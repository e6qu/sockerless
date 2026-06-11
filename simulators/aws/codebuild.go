package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sim "github.com/sockerless/simulator"
	"gopkg.in/yaml.v3"
)

// AWS CodeBuild — AWS JSON 1.1 protocol (X-Amz-Target: CodeBuild_20161006.<Op>).
// Builds execute project buildspec commands and record terminal state from exit status.

type CBProject struct {
	Name         string         `json:"name"`
	Arn          string         `json:"arn"`
	Description  string         `json:"description,omitempty"`
	Source       map[string]any `json:"source"`
	Artifacts    map[string]any `json:"artifacts"`
	Environment  map[string]any `json:"environment"`
	ServiceRole  string         `json:"serviceRole"`
	Created      float64        `json:"created"`
	LastModified float64        `json:"lastModified"`
	Tags         []CBTag        `json:"tags,omitempty"`
}

type CBBuild struct {
	ID          string         `json:"id"`
	Arn         string         `json:"arn"`
	ProjectName string         `json:"projectName"`
	BuildStatus string         `json:"buildStatus"`
	StartTime   float64        `json:"startTime"`
	EndTime     float64        `json:"endTime"`
	Phases      []CBPhase      `json:"phases"`
	Logs        map[string]any `json:"logs"`
	Environment map[string]any `json:"environment,omitempty"`
}

type CBPhase struct {
	PhaseType         string           `json:"phaseType"`
	PhaseStatus       string           `json:"phaseStatus"`
	StartTime         float64          `json:"startTime"`
	EndTime           float64          `json:"endTime"`
	DurationInSeconds float64          `json:"durationInSeconds"`
	Contexts          []CBPhaseContext `json:"contexts,omitempty"`
}

type CBPhaseContext struct {
	StatusCode string `json:"statusCode"`
	Message    string `json:"message"`
}

// cbLogsLocation is the LogsLocation the sim reports: builds run as local
// processes without a CloudWatch log sink, so log enablement is the real
// member cloudWatchLogs.status (not an invented boolean).
func cbLogsLocation() map[string]any {
	return map[string]any{
		"cloudWatchLogs": map[string]any{"status": "DISABLED"},
	}
}

type CBTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var (
	cbProjects sim.Store[CBProject]
	cbBuilds   sim.Store[CBBuild]
	cbMu       sync.Mutex
)

func registerCodeBuild(r *sim.AWSRouter, srv *sim.Server) {
	cbProjects = sim.MakeStore[CBProject](srv.DB(), "codebuild_projects")
	cbBuilds = sim.MakeStore[CBBuild](srv.DB(), "codebuild_builds")

	r.Register("CodeBuild_20161006.CreateProject", handleCBCreateProject)
	r.Register("CodeBuild_20161006.BatchGetProjects", handleCBBatchGetProjects)
	r.Register("CodeBuild_20161006.ListProjects", handleCBListProjects)
	r.Register("CodeBuild_20161006.UpdateProject", handleCBUpdateProject)
	r.Register("CodeBuild_20161006.DeleteProject", handleCBDeleteProject)
	r.Register("CodeBuild_20161006.StartBuild", handleCBStartBuild)
	r.Register("CodeBuild_20161006.BatchGetBuilds", handleCBBatchGetBuilds)
	r.Register("CodeBuild_20161006.ListBuildsForProject", handleCBListBuildsForProject)
	r.Register("CodeBuild_20161006.ListBuilds", handleCBListBuilds)
}

func cbARN(resource string) string {
	return fmt.Sprintf("arn:aws:codebuild:us-east-1:123456789012:%s", resource)
}

func cbEpochNow() float64 {
	return float64(time.Now().UTC().Unix())
}

func cbWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cbWriteError(w http.ResponseWriter, code string, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"__type":  code,
		"message": msg,
	})
}

func handleCBCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Source      map[string]any `json:"source"`
		Artifacts   map[string]any `json:"artifacts"`
		Environment map[string]any `json:"environment"`
		ServiceRole string         `json:"serviceRole"`
		Tags        []CBTag        `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}
	if req.Name == "" {
		cbWriteError(w, "InvalidInputException", "name is required")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	if _, ok := cbProjects.Get(req.Name); ok {
		cbWriteError(w, "ResourceAlreadyExistsException", "Project already exists: "+req.Name)
		return
	}

	now := cbEpochNow()
	if req.Source == nil {
		req.Source = map[string]any{"type": "NO_SOURCE"}
	}
	if req.Artifacts == nil {
		req.Artifacts = map[string]any{"type": "NO_ARTIFACTS"}
	}
	if req.Environment == nil {
		req.Environment = map[string]any{"type": "LINUX_CONTAINER", "image": "aws/codebuild/standard:7.0", "computeType": "BUILD_GENERAL1_SMALL"}
	}
	p := CBProject{
		Name:         req.Name,
		Arn:          cbARN("project/" + req.Name),
		Description:  req.Description,
		Source:       req.Source,
		Artifacts:    req.Artifacts,
		Environment:  req.Environment,
		ServiceRole:  req.ServiceRole,
		Created:      now,
		LastModified: now,
		Tags:         req.Tags,
	}
	cbProjects.Put(req.Name, p)
	cbWriteJSON(w, http.StatusOK, map[string]any{"project": p})
}

func handleCBBatchGetProjects(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []CBProject
	var notFound []string
	for _, nameOrARN := range req.Names {
		p, ok := cbProjects.Get(nameOrARN)
		if !ok {
			p, ok = cbProjects.Get(cbNameFromARN(nameOrARN))
		}
		if ok {
			found = append(found, p)
		} else {
			notFound = append(notFound, nameOrARN)
		}
	}
	if found == nil {
		found = []CBProject{}
	}
	if notFound == nil {
		notFound = []string{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{
		"projects":         found,
		"projectsNotFound": notFound,
	})
}

func handleCBListProjects(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SortBy    string `json:"sortBy"`
		SortOrder string `json:"sortOrder"`
		NextToken string `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := cbProjects.List()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Name)
	}
	page, nextTok := awsPage(names, req.NextToken, 0, 100)
	resp := map[string]any{"projects": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBUpdateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Source      map[string]any `json:"source"`
		Artifacts   map[string]any `json:"artifacts"`
		Environment map[string]any `json:"environment"`
		ServiceRole string         `json:"serviceRole"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	p, ok := cbProjects.Get(req.Name)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Project not found: "+req.Name)
		return
	}
	if req.Description != "" {
		p.Description = req.Description
	}
	if req.Source != nil {
		p.Source = req.Source
	}
	if req.Artifacts != nil {
		p.Artifacts = req.Artifacts
	}
	if req.Environment != nil {
		p.Environment = req.Environment
	}
	if req.ServiceRole != "" {
		p.ServiceRole = req.ServiceRole
	}
	p.LastModified = cbEpochNow()
	cbProjects.Put(req.Name, p)
	cbWriteJSON(w, http.StatusOK, map[string]any{"project": p})
}

func handleCBDeleteProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	cbProjects.Delete(req.Name)
	cbWriteJSON(w, http.StatusOK, map[string]any{})
}

func handleCBStartBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectName                  string           `json:"projectName"`
		BuildspecOverride            string           `json:"buildspecOverride"`
		EnvironmentVariablesOverride []map[string]any `json:"environmentVariablesOverride"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	p, ok := cbProjects.Get(req.ProjectName)
	if !ok {
		cbWriteError(w, "ResourceNotFoundException", "Project not found: "+req.ProjectName)
		return
	}
	commands, err := cbBuildCommands(p, req.BuildspecOverride)
	if err != nil {
		cbWriteError(w, "InvalidInputException", err.Error())
		return
	}

	cbMu.Lock()
	defer cbMu.Unlock()

	buildID := req.ProjectName + ":" + uuid.New().String()
	now := cbEpochNow()
	build := CBBuild{
		ID:          buildID,
		Arn:         cbARN("build/" + buildID),
		ProjectName: req.ProjectName,
		BuildStatus: "IN_PROGRESS",
		StartTime:   now,
		EndTime:     now,
		Environment: p.Environment,
		Phases: []CBPhase{
			{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: now, EndTime: now, DurationInSeconds: 0},
			{PhaseType: "BUILD", PhaseStatus: "IN_PROGRESS", StartTime: now},
		},
		Logs: cbLogsLocation(),
	}
	cbBuilds.Put(buildID, build)
	go cbRunBuild(buildID, commands, cbEnvironment(p.Environment, req.EnvironmentVariablesOverride))
	cbWriteJSON(w, http.StatusOK, map[string]any{"build": build})
}

type cbBuildspec struct {
	Phases map[string]struct {
		Commands []string `yaml:"commands"`
	} `yaml:"phases"`
}

func cbBuildCommands(p CBProject, override string) ([]string, error) {
	buildspec := override
	if buildspec == "" {
		buildspec = cbString(p.Source["buildspec"])
	}
	if buildspec == "" {
		return nil, fmt.Errorf("source.buildspec is required for NO_SOURCE builds")
	}
	var spec cbBuildspec
	if err := yaml.Unmarshal([]byte(buildspec), &spec); err != nil {
		return nil, fmt.Errorf("invalid buildspec: %w", err)
	}
	var commands []string
	for _, phase := range []string{"install", "pre_build", "build", "post_build"} {
		commands = append(commands, spec.Phases[phase].Commands...)
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("buildspec must contain at least one command")
	}
	return commands, nil
}

func cbRunBuild(buildID string, commands []string, env map[string]string) {
	workDir, err := os.MkdirTemp("", "sockerless-codebuild-*")
	if err != nil {
		cbCompleteBuild(buildID, -1, err.Error())
		return
	}
	defer os.RemoveAll(workDir)

	exitCode := 0
	var reason string
	for _, command := range commands {
		handle := sim.StartProcess(sim.ProcessConfig{
			Command: []string{"/bin/sh", "-c", command},
			Dir:     filepath.Clean(workDir),
			Env:     env,
		}, sim.NoopSink{})
		result := handle.Wait()
		if result.Error != nil {
			exitCode = -1
			reason = result.Error.Error()
			break
		}
		if result.ExitCode != 0 {
			exitCode = result.ExitCode
			reason = fmt.Sprintf("Build command exited with status %d", result.ExitCode)
			break
		}
	}
	cbCompleteBuild(buildID, exitCode, reason)
}

func cbCompleteBuild(buildID string, exitCode int, reason string) {
	cbMu.Lock()
	defer cbMu.Unlock()

	build, ok := cbBuilds.Get(buildID)
	if !ok {
		return
	}
	now := cbEpochNow()
	status := "SUCCEEDED"
	if exitCode != 0 {
		status = "FAILED"
	}
	build.BuildStatus = status
	build.EndTime = now
	buildPhase := CBPhase{PhaseType: "BUILD", PhaseStatus: status, StartTime: build.StartTime, EndTime: now, DurationInSeconds: now - build.StartTime}
	if reason != "" {
		// Failure detail rides the phase's contexts (the real Build
		// shape's per-phase PhaseContext), not the LogsLocation.
		// COMMAND_EXECUTION_ERROR is real CodeBuild's status code for a
		// failed buildspec command.
		buildPhase.Contexts = []CBPhaseContext{{
			StatusCode: "COMMAND_EXECUTION_ERROR",
			Message:    reason,
		}}
	}
	build.Phases = []CBPhase{
		{PhaseType: "SUBMITTED", PhaseStatus: "SUCCEEDED", StartTime: build.StartTime, EndTime: build.StartTime, DurationInSeconds: 0},
		buildPhase,
		{PhaseType: "COMPLETED", PhaseStatus: status, StartTime: now, EndTime: now, DurationInSeconds: 0},
	}
	cbBuilds.Put(buildID, build)
}

func handleCBBatchGetBuilds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	var found []CBBuild
	var notFound []string
	for _, id := range req.IDs {
		if b, ok := cbBuilds.Get(id); ok {
			found = append(found, b)
		} else {
			notFound = append(notFound, id)
		}
	}
	if found == nil {
		found = []CBBuild{}
	}
	if notFound == nil {
		notFound = []string{}
	}
	cbWriteJSON(w, http.StatusOK, map[string]any{
		"builds":         found,
		"buildsNotFound": notFound,
	})
}

func handleCBListBuildsForProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectName string `json:"projectName"`
		SortOrder   string `json:"sortOrder"`
		NextToken   string `json:"nextToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		cbWriteError(w, "InvalidInputException", "invalid JSON")
		return
	}

	all := cbBuilds.List()
	var ids []string
	for _, b := range all {
		if b.ProjectName == req.ProjectName {
			ids = append(ids, b.ID)
		}
	}
	page, nextTok := awsPage(ids, req.NextToken, 0, 100)
	resp := map[string]any{"ids": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func handleCBListBuilds(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SortOrder string `json:"sortOrder"`
		NextToken string `json:"nextToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	all := cbBuilds.List()
	ids := make([]string, 0, len(all))
	for _, b := range all {
		ids = append(ids, b.ID)
	}
	page, nextTok := awsPage(ids, req.NextToken, 0, 100)
	resp := map[string]any{"ids": page}
	if nextTok != "" {
		resp["nextToken"] = nextTok
	}
	cbWriteJSON(w, http.StatusOK, resp)
}

func cbNameFromARN(arn string) string {
	// arn:aws:codebuild:us-east-1:123456789012:project/name
	parts := strings.Split(arn, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return arn
}

func cbString(v any) string {
	s, _ := v.(string)
	return s
}

func cbEnvironment(projectEnv map[string]any, overrides []map[string]any) map[string]string {
	env := map[string]string{
		"PATH": os.Getenv("PATH"),
		"HOME": os.Getenv("HOME"),
	}
	for k, v := range cbEnvironmentValues(projectEnv["environmentVariables"]) {
		env[k] = v
	}
	for _, item := range overrides {
		name := cbString(item["name"])
		if name == "" {
			name = cbString(item["Name"])
		}
		if name == "" {
			continue
		}
		value := cbString(item["value"])
		if value == "" {
			value = cbString(item["Value"])
		}
		env[name] = value
	}
	return env
}

func cbEnvironmentValues(v any) map[string]string {
	env := map[string]string{}
	values, ok := v.([]any)
	if !ok {
		return env
	}
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name := cbString(item["name"])
		if name == "" {
			name = cbString(item["Name"])
		}
		if name == "" {
			continue
		}
		value := cbString(item["value"])
		if value == "" {
			value = cbString(item["Value"])
		}
		env[name] = value
	}
	return env
}
