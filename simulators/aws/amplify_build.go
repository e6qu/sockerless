package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	sim "github.com/sockerless/simulator"
	"gopkg.in/yaml.v3"
)

// Real Amplify build pipeline. A job runs a real build when the app has a
// clonable HTTP(S) git repository. A branch/app buildSpec wins when configured;
// otherwise the build host reads amplify.yml from the checked-out repository,
// matching Amplify Hosting's source-controlled build settings. The sim host clones the repository in-process
// (go-git — the host prepares the workspace the way Amplify's build host
// does), then executes the buildSpec's frontend phases inside a node
// container on the host Docker daemon, collects the artifact baseDirectory
// into a zip stored in the sim's S3, and reports SUCCEED/FAILED from the
// container exit code. A source that cannot be cloned is rejected by StartJob;
// it never produces a successful job without executing work.

// ---------- buildSpec ----------

// amplifyBuildSpec is the parsed amplify.yml surface the sim executes:
// frontend preBuild/build commands + the artifacts collection rule.
type amplifyBuildSpec struct {
	Version          string
	PreBuildCommands []string
	BuildCommands    []string
	BaseDirectory    string
	Files            []string
}

type amplifyBuildSpecYAML struct {
	Version  any `yaml:"version"`
	Frontend struct {
		Phases struct {
			PreBuild struct {
				Commands []string `yaml:"commands"`
			} `yaml:"preBuild"`
			Build struct {
				Commands []string `yaml:"commands"`
			} `yaml:"build"`
		} `yaml:"phases"`
		Artifacts struct {
			BaseDirectory string   `yaml:"baseDirectory"`
			Files         []string `yaml:"files"`
		} `yaml:"artifacts"`
	} `yaml:"frontend"`
}

// amplifyParseBuildSpec parses an amplify.yml buildSpec. It requires at
// least one frontend build-phase command — a spec with nothing to execute
// is a configuration error the build surfaces as FAILED, not a silent
// success.
func amplifyParseBuildSpec(text string) (amplifyBuildSpec, error) {
	var raw amplifyBuildSpecYAML
	if err := yaml.Unmarshal([]byte(text), &raw); err != nil {
		return amplifyBuildSpec{}, fmt.Errorf("invalid buildSpec YAML: %w", err)
	}
	spec := amplifyBuildSpec{
		Version:          fmt.Sprintf("%v", raw.Version),
		PreBuildCommands: raw.Frontend.Phases.PreBuild.Commands,
		BuildCommands:    raw.Frontend.Phases.Build.Commands,
		BaseDirectory:    raw.Frontend.Artifacts.BaseDirectory,
		Files:            raw.Frontend.Artifacts.Files,
	}
	if len(spec.PreBuildCommands)+len(spec.BuildCommands) == 0 {
		return amplifyBuildSpec{}, fmt.Errorf("buildSpec has no frontend.phases.preBuild/build commands")
	}
	if spec.BaseDirectory == "" {
		return amplifyBuildSpec{}, fmt.Errorf("buildSpec has no frontend.artifacts.baseDirectory")
	}
	return spec, nil
}

// amplifyRealBuildPlan resolves the source and optional configured buildSpec.
// An empty spec is valid at this stage: the provision phase reads amplify.yml
// from the cloned repository. ok is false only when the source cannot be
// cloned through the supported HTTPS Git transport.
func amplifyRealBuildPlan(app AmplifyApp, br AmplifyBranch) (spec string, repo string, ok bool) {
	repo = app.Repository
	if !strings.HasPrefix(repo, "http://") && !strings.HasPrefix(repo, "https://") {
		return "", "", false
	}
	spec = br.BuildSpec
	if spec == "" {
		spec = app.BuildSpec
	}
	return spec, repo, true
}

// ---------- running builds (StopJob cancellation) ----------

var (
	amplifyBuildMu      sync.Mutex
	amplifyBuildCancels = map[string]func(){} // jobID → cancel running build container
)

func amplifyRegisterBuildCancel(jobID string, cancel func()) {
	amplifyBuildMu.Lock()
	defer amplifyBuildMu.Unlock()
	amplifyBuildCancels[jobID] = cancel
}

func amplifyUnregisterBuildCancel(jobID string) {
	amplifyBuildMu.Lock()
	defer amplifyBuildMu.Unlock()
	delete(amplifyBuildCancels, jobID)
}

// amplifyCancelRunningBuild stops the build container of an in-flight real
// build, if any. Called by StopJob after it marks the job CANCELLED.
func amplifyCancelRunningBuild(jobID string) {
	amplifyBuildMu.Lock()
	cancel := amplifyBuildCancels[jobID]
	amplifyBuildMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ---------- build image ----------

// amplifyBuildImage is the managed build image used by this Amazon Amplify
// Hosting runtime generation. AWS selects its build image service-side; it is
// therefore fixed by the cloud implementation rather than caller-configurable
// simulator state.
func amplifyBuildImage() string {
	return "public.ecr.aws/docker/library/node:20-alpine"
}

// ---------- step/log plumbing ----------

// amplifyStepLog accumulates one job step's log lines and lands them in the
// sim's S3 so the step's logUrl resolves (sim-emitted-url-roundtrip).
type amplifyStepLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *amplifyStepLog) WriteLog(line sim.LogLine) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line.Text)
}

func (l *amplifyStepLog) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *amplifyStepLog) Text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n") + "\n"
}

// amplifyStoreStepLog writes a step's log to the sim's S3 and returns the
// presigned URL that becomes the step's logUrl.
func amplifyStoreStepLog(urlBase, appID, branch, jobID, step string, log *amplifyStepLog) string {
	key := "logs/" + appID + "/" + branch + "/" + jobID + "/" + step + ".log"
	amplifyPutS3Object(key, "text/plain", []byte(log.Text()))
	return amplifyPresignedS3URLBase(urlBase, key, http.MethodGet)
}

// amplifyUpdateJobStep mutates one step of a stored job. Steps that already
// reached a terminal state are left alone — StopJob marks every step
// CANCELLED, and the build goroutine's late completion must not rewrite
// that to FAILED.
func amplifyUpdateJobStep(jobID, stepName string, mutate func(*AmplifyJobStep)) {
	amplifyJobs.Update(jobID, func(j *amplifyStoredJob) {
		for i := range j.Job.Steps {
			if j.Job.Steps[i].StepName == stepName && !j.Job.Steps[i].Status.Terminal() {
				mutate(&j.Job.Steps[i])
			}
		}
	})
}

// amplifyFinishJob lands a real-build job in a terminal state, refusing to
// clobber a job that already left RUNNING (StopJob marked it CANCELLED).
// Remaining non-terminal steps land in the same state.
func amplifyFinishJob(jobID string, to AmplifyJobStatus) bool {
	finished := false
	amplifyJobs.Update(jobID, func(j *amplifyStoredJob) {
		if j.Job.Summary.Status != AmplifyJobStatusRunning {
			return
		}
		now := amplifyEpoch()
		j.Job.Summary.Status = to
		j.Job.Summary.EndTime = now
		for i := range j.Job.Steps {
			if !j.Job.Steps[i].Status.Terminal() {
				j.Job.Steps[i].Status = to
				j.Job.Steps[i].EndTime = now
			}
		}
		finished = true
	})
	return finished
}

// ---------- the build itself ----------

const amplifyBuildTimeout = 15 * time.Minute

// amplifyScheduleRealBuild runs a real build for a StartJob whose app has a
// clonable repository + buildSpec. Job state machine: PENDING → RUNNING at
// clone start → SUCCEED/FAILED honestly from the build container's exit.
func amplifyScheduleRealBuild(appID, branch, jobID, urlBase, repo, specText string, env map[string]string, commitID string) {
	go func() {
		if !amplifyAdvanceJob(jobID, AmplifyJobStatusPending, AmplifyJobStatusRunning) {
			return // stopped before it started
		}
		status := amplifyRunRealBuild(appID, branch, jobID, urlBase, repo, specText, env, commitID)
		if amplifyFinishJob(jobID, status) && status == AmplifyJobStatusSucceed {
			amplifyMarkProductionDeploy(appID, branch, jobID)
		}
	}()
}

func amplifyRunRealBuild(appID, branch, jobID, urlBase, repo, specText string, env map[string]string, commitID string) AmplifyJobStatus {
	provisionLog := &amplifyStepLog{}
	finishStep := func(step string, log *amplifyStepLog, status AmplifyJobStatus) {
		logURL := amplifyStoreStepLog(urlBase, appID, branch, jobID, step, log)
		now := amplifyEpoch()
		amplifyUpdateJobStep(jobID, step, func(s *AmplifyJobStep) {
			s.Status = status
			s.EndTime = now
			s.LogUrl = logURL
		})
	}
	failProvision := func(format string, args ...any) AmplifyJobStatus {
		provisionLog.Printf(format, args...)
		finishStep("PROVISION", provisionLog, AmplifyJobStatusFailed)
		return AmplifyJobStatusFailed
	}

	// PROVISION: workspace + clone.
	workDir, err := os.MkdirTemp("", "sockerless-amplify-build-*")
	if err != nil {
		return failProvision("workspace: %v", err)
	}
	defer os.RemoveAll(workDir)
	provisionLog.Printf("# Cloning repository: %s (branch %s)", repo, branch)
	cloneOpts := &git.CloneOptions{
		URL:           repo,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	}
	gitRepo, err := git.PlainClone(workDir, false, cloneOpts)
	if err != nil {
		return failProvision("git clone %s (branch %s): %v", repo, branch, err)
	}
	if head, err := gitRepo.Head(); err == nil {
		provisionLog.Printf("# HEAD %s", head.Hash())
		if commitID == "" || commitID == "HEAD" {
			amplifyJobs.Update(jobID, func(j *amplifyStoredJob) {
				j.Job.Summary.CommitId = head.Hash().String()
			})
		}
	}
	if strings.TrimSpace(specText) == "" {
		checkedIn, readErr := os.ReadFile(filepath.Join(workDir, "amplify.yml"))
		if readErr != nil {
			return failProvision("buildSpec error: repository has no readable amplify.yml: %v", readErr)
		}
		specText = string(checkedIn)
		provisionLog.Printf("# Build specification: amplify.yml")
	}
	spec, err := amplifyParseBuildSpec(specText)
	if err != nil {
		return failProvision("buildSpec error: %v", err)
	}
	provisionLog.Printf("# Build image: %s", amplifyBuildImage())
	finishStep("PROVISION", provisionLog, AmplifyJobStatusSucceed)

	// BUILD: preBuild + build commands in one shell (env exports persist
	// across phases, the way Amplify's build container runs them).
	buildLog := &amplifyStepLog{}
	var script strings.Builder
	script.WriteString("set -e\n")
	for _, phase := range [][]string{spec.PreBuildCommands, spec.BuildCommands} {
		for _, command := range phase {
			script.WriteString(command + "\n")
		}
	}
	handle, err := sim.StartContainerSync(sim.ContainerConfig{
		Image:        amplifyBuildImage(),
		Architecture: "linux/" + runtime.GOARCH,
		Command:      []string{"/bin/sh", "-c", script.String()},
		WorkingDir:   "/workspace",
		// The build writes real artifacts back into this workspace. A shared
		// SELinux relabel gives the confined build container that access on
		// enforcing hosts and is accepted as a no-op by Docker elsewhere.
		Binds:   []string{workDir + ":/workspace:z"},
		Env:     env,
		Timeout: amplifyBuildTimeout,
		Labels:  map[string]string{"sockerless-amplify-job": jobID},
		Sandbox: sim.SandboxFargate,
	}, buildLog)
	if err != nil {
		buildLog.Printf("# start build container: %v", err)
		finishStep("BUILD", buildLog, AmplifyJobStatusFailed)
		return AmplifyJobStatusFailed
	}
	amplifyRegisterBuildCancel(jobID, handle.Cancel)
	result := handle.Wait()
	amplifyUnregisterBuildCancel(jobID)
	if result.Error != nil {
		buildLog.Printf("# build container error: %v", result.Error)
		finishStep("BUILD", buildLog, AmplifyJobStatusFailed)
		return AmplifyJobStatusFailed
	}
	if result.ExitCode != 0 {
		buildLog.Printf("# build exited with status %d", result.ExitCode)
		finishStep("BUILD", buildLog, AmplifyJobStatusFailed)
		return AmplifyJobStatusFailed
	}
	finishStep("BUILD", buildLog, AmplifyJobStatusSucceed)

	// DEPLOY: collect baseDirectory into the job's artifact zip.
	deployLog := &amplifyStepLog{}
	zipBytes, fileCount, err := amplifyZipArtifacts(filepath.Join(workDir, spec.BaseDirectory), spec.Files, deployLog)
	if err != nil {
		deployLog.Printf("# artifact collection: %v", err)
		finishStep("DEPLOY", deployLog, AmplifyJobStatusFailed)
		return AmplifyJobStatusFailed
	}
	key := "artifacts/" + appID + "/" + branch + "/" + jobID + "/artifacts.zip"
	amplifyPutS3Object(key, "application/zip", zipBytes)
	amplifyRegisterJobArtifact(urlBase, appID, branch, jobID, amplifyArtifactID(jobID), "artifacts.zip", key)
	deployLog.Printf("# deployed %d files (%d bytes) from %s", fileCount, len(zipBytes), spec.BaseDirectory)
	finishStep("DEPLOY", deployLog, AmplifyJobStatusSucceed)
	return AmplifyJobStatusSucceed
}

// amplifyZipArtifacts zips baseDir's contents filtered by the buildSpec's
// files patterns ('**/*' or empty = everything; otherwise path.Match against
// the slash-separated relative path).
func amplifyZipArtifacts(baseDir string, patterns []string, log *amplifyStepLog) ([]byte, int, error) {
	info, err := os.Stat(baseDir)
	if err != nil || !info.IsDir() {
		return nil, 0, fmt.Errorf("artifacts baseDirectory %s not found after build", filepath.Base(baseDir))
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	count := 0
	err = filepath.WalkDir(baseDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(baseDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !amplifyArtifactMatch(patterns, rel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := zw.Create(rel)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
		log.Printf("# artifact: %s (%d bytes)", rel, len(data))
		count++
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, 0, fmt.Errorf("no files matched artifacts.files in %s", filepath.Base(baseDir))
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), count, nil
}

func amplifyArtifactMatch(patterns []string, rel string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if pattern == "**/*" || pattern == "**" {
			return true
		}
		if ok, _ := path.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

// amplifyBuildEnv merges the app- and branch-level environment variables
// (branch wins) plus the standard variables real Amplify injects into every
// build.
func amplifyBuildEnv(app AmplifyApp, br AmplifyBranch, jobID string) map[string]string {
	env := map[string]string{}
	for k, v := range app.EnvironmentVariables {
		env[k] = v
	}
	for k, v := range br.EnvironmentVariables {
		env[k] = v
	}
	env["AWS_APP_ID"] = app.AppId
	env["AWS_BRANCH"] = br.BranchName
	env["AWS_JOB_ID"] = jobID
	return env
}
