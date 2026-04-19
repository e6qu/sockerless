package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Build v1 slice. Sockerless's GCP backends (`backends/cloudrun/`
// and `backends/cloudrun-functions/`) submit docker builds via Cloud
// Build whenever sockerless handles `docker build`. Without this slice
// the GCP simulator can't cover the image-build path. BUG-704 fix.
//
// Real API: https://cloud.google.com/build/docs/api/reference/rest

// Build represents a Cloud Build build resource.
type Build struct {
	ID               string            `json:"id"`
	Name             string            `json:"name,omitempty"`
	ProjectID        string            `json:"projectId"`
	Status           string            `json:"status"`
	StatusDetail     string            `json:"statusDetail,omitempty"`
	Source           *BuildSource      `json:"source,omitempty"`
	Steps            []*BuildStep      `json:"steps,omitempty"`
	Images           []string          `json:"images,omitempty"`
	CreateTime       string            `json:"createTime,omitempty"`
	StartTime        string            `json:"startTime,omitempty"`
	FinishTime       string            `json:"finishTime,omitempty"`
	LogsBucket       string            `json:"logsBucket,omitempty"`
	AvailableSecrets *AvailableSecrets `json:"availableSecrets,omitempty"`
	Substitutions    map[string]string `json:"substitutions,omitempty"`
	Options          map[string]any    `json:"options,omitempty"`
}

type BuildSource struct {
	StorageSource *StorageSource `json:"storageSource,omitempty"`
}

type StorageSource struct {
	Bucket string `json:"bucket"`
	Object string `json:"object"`
}

type BuildStep struct {
	Name       string   `json:"name"`
	Args       []string `json:"args,omitempty"`
	Env        []string `json:"env,omitempty"`
	SecretEnv  []string `json:"secretEnv,omitempty"`
	Dir        string   `json:"dir,omitempty"`
	Entrypoint string   `json:"entrypoint,omitempty"`
	ID         string   `json:"id,omitempty"`
}

// AvailableSecrets binds Secret Manager references to environment
// variable names usable by steps via `secretEnv`.
type AvailableSecrets struct {
	SecretManager []*SecretManagerSecret `json:"secretManager,omitempty"`
}

type SecretManagerSecret struct {
	VersionName string `json:"versionName"`
	Env         string `json:"env"`
}

// Operation is the LRO wrapper Cloud Build returns from CreateBuild.
type CloudBuildOperation struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Done     bool           `json:"done"`
	Response map[string]any `json:"response,omitempty"`
	Error    *BuildError    `json:"error,omitempty"`
}

type BuildError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var cbBuilds sim.Store[Build]

func registerCloudBuild(srv *sim.Server) {
	cbBuilds = sim.MakeStore[Build](srv.DB(), "cloudbuild_builds")

	// CreateBuild: POST /v1/projects/{project}/builds
	srv.HandleFunc("POST /v1/projects/{project}/builds", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")

		var build Build
		if err := sim.ReadJSON(r, &build); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid build body: %v", err)
			return
		}
		build.ID = generateUUID()
		build.ProjectID = project
		build.Status = "QUEUED"
		build.CreateTime = time.Now().UTC().Format(time.RFC3339)
		build.Name = fmt.Sprintf("projects/%s/locations/global/builds/%s", project, build.ID)

		cbBuilds.Put(build.ID, build)

		// Execute synchronously — real Cloud Build is async with
		// status transitions QUEUED → WORKING → SUCCESS/FAILURE; the
		// simulator compresses this into one call so `op.Wait()` on
		// the backend returns the final state immediately.
		result := executeBuild(r.Context(), build)
		cbBuilds.Put(result.ID, result)

		// Return LRO wrapper with done=true so `op.Wait(ctx)` resolves.
		op := CloudBuildOperation{
			Name:     fmt.Sprintf("operations/build/%s/%s", project, result.ID),
			Done:     true,
			Metadata: map[string]any{"@type": "type.googleapis.com/google.devtools.cloudbuild.v1.BuildOperationMetadata", "build": result},
		}
		if result.Status == "SUCCESS" {
			op.Response = map[string]any{"@type": "type.googleapis.com/google.devtools.cloudbuild.v1.Build"}
			for k, v := range structToMap(result) {
				op.Response[k] = v
			}
		} else {
			op.Error = &BuildError{Code: 13, Message: result.StatusDetail}
		}
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// GetBuild: GET /v1/projects/{project}/builds/{id}
	srv.HandleFunc("GET /v1/projects/{project}/builds/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "id")
		build, ok := cbBuilds.Get(id)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "build %s not found", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, build)
	})

	// CancelBuild: POST /v1/projects/{project}/builds/{id}:cancel.
	// Go ServeMux doesn't allow `{id}:cancel`; use a single wildcard
	// and parse the colon suffix in the handler.
	srv.HandleFunc("POST /v1/projects/{project}/builds/{idAction}", func(w http.ResponseWriter, r *http.Request) {
		idAction := sim.PathParam(r, "idAction")
		id, action, found := strings.Cut(idAction, ":")
		if !found || action != "cancel" {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown build action %q", idAction)
			return
		}
		cbBuilds.Update(id, func(b *Build) {
			if b.Status == "QUEUED" || b.Status == "WORKING" {
				b.Status = "CANCELLED"
				b.FinishTime = time.Now().UTC().Format(time.RFC3339)
			}
		})
		build, ok := cbBuilds.Get(id)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "build %s not found", id)
			return
		}
		sim.WriteJSON(w, http.StatusOK, build)
	})

	// GetOperation for cloudbuild LROs:
	// GET /v1/{name=operations/**}  — Go SDK uses this path.
	srv.HandleFunc("GET /v1/operations/build/{project}/{id}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		id := sim.PathParam(r, "id")
		build, ok := cbBuilds.Get(id)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "operation for build %s not found", id)
			return
		}
		op := CloudBuildOperation{
			Name: fmt.Sprintf("operations/build/%s/%s", project, id),
			Done: build.Status == "SUCCESS" || build.Status == "FAILURE" || build.Status == "CANCELLED",
		}
		if op.Done {
			if build.Status == "SUCCESS" {
				op.Response = map[string]any{"@type": "type.googleapis.com/google.devtools.cloudbuild.v1.Build"}
				for k, v := range structToMap(build) {
					op.Response[k] = v
				}
			} else {
				op.Error = &BuildError{Code: 13, Message: build.StatusDetail}
			}
		}
		sim.WriteJSON(w, http.StatusOK, op)
	})
}

// executeBuild runs the build steps against the source context and
// returns the final build record with status + finishTime populated.
// Matches the real Cloud Build behavior: downloads source from GCS,
// extracts it, executes each step (currently only gcr.io/cloud-builders/docker),
// expands secretEnv via AvailableSecrets → Secret Manager (BUG-707).
func executeBuild(ctx context.Context, b Build) Build {
	b.StartTime = time.Now().UTC().Format(time.RFC3339)
	b.Status = "WORKING"

	fail := func(msg string) Build {
		b.Status = "FAILURE"
		b.StatusDetail = msg
		b.FinishTime = time.Now().UTC().Format(time.RFC3339)
		return b
	}

	if b.Source == nil || b.Source.StorageSource == nil {
		return fail("source.storageSource is required")
	}

	// Fetch the tarball from gcsObjects (populated by gcs.go).
	objKey := b.Source.StorageSource.Bucket + "/" + b.Source.StorageSource.Object
	obj, ok := gcsObjects.Get(objKey)
	if !ok {
		return fail(fmt.Sprintf("source object %s not found in bucket %s",
			b.Source.StorageSource.Object, b.Source.StorageSource.Bucket))
	}

	// Extract to a temp dir.
	workDir, err := os.MkdirTemp("", "sim-cloudbuild-*")
	if err != nil {
		return fail(fmt.Sprintf("tempdir: %v", err))
	}
	defer os.RemoveAll(workDir)

	if err := extractTarball(obj.data, workDir); err != nil {
		return fail(fmt.Sprintf("extract source: %v", err))
	}

	// Resolve Secret Manager references for secretEnv expansion (BUG-707).
	secretValues := map[string]string{}
	if b.AvailableSecrets != nil {
		for _, sm := range b.AvailableSecrets.SecretManager {
			payload, err := resolveSecretManagerReference(sm.VersionName)
			if err != nil {
				return fail(fmt.Sprintf("resolve secret %s: %v", sm.VersionName, err))
			}
			secretValues[sm.Env] = string(payload)
		}
	}

	// Execute each build step. For Phase 86 scope, only
	// gcr.io/cloud-builders/docker is supported — it's the only
	// builder sockerless uses.
	for i, step := range b.Steps {
		if step == nil {
			continue
		}
		if !strings.HasPrefix(step.Name, "gcr.io/cloud-builders/docker") {
			return fail(fmt.Sprintf("step %d: builder %q not supported by this simulator (only gcr.io/cloud-builders/docker)",
				i, step.Name))
		}
		if err := runDockerStep(ctx, workDir, step, secretValues); err != nil {
			return fail(fmt.Sprintf("step %d (%s %v): %v", i, step.Name, step.Args, err))
		}
	}

	b.Status = "SUCCESS"
	b.FinishTime = time.Now().UTC().Format(time.RFC3339)
	return b
}

// extractTarball unpacks a gzip-compressed tar archive into dir.
// Cloud Build context uploads use .tar.gz convention.
func extractTarball(data []byte, dir string) error {
	var r io.Reader = bytesReader(data)
	// Best-effort gzip detection: Cloud Build uploads are typically
	// gzipped; skip the gzip layer if magic doesn't match.
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		path := filepath.Join(dir, hdr.Name)
		// Prevent path traversal.
		if !strings.HasPrefix(path, dir) {
			return fmt.Errorf("tarball contains path traversal: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
}

// runDockerStep executes one `gcr.io/cloud-builders/docker` step.
// Args are the docker sub-command args (e.g. ["build","-t","img","."]).
// secretValues map env-var-name → resolved secret payload; these are
// added to the subprocess env when the step's secretEnv references them.
func runDockerStep(ctx context.Context, workDir string, step *BuildStep, secretValues map[string]string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not available: %w", err)
	}
	cmd := exec.CommandContext(ctx, "docker", step.Args...)
	cmd.Dir = workDir
	if step.Dir != "" {
		cmd.Dir = filepath.Join(workDir, step.Dir)
	}
	env := os.Environ()
	for _, e := range step.Env {
		env = append(env, e)
	}
	for _, secEnvName := range step.SecretEnv {
		if v, ok := secretValues[secEnvName]; ok {
			env = append(env, secEnvName+"="+v)
		}
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// bytesReader wraps a byte slice as an io.Reader. Avoids importing
// bytes just for the NewReader call; a tiny shim.
type bytesReader []byte

func (b bytesReader) Read(p []byte) (int, error) {
	if len(b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, b)
	return n, nil
}

// structToMap converts a Build to a generic map[string]any for
// embedding inside the LRO's response envelope. The real API wraps
// `Build` as a protobuf Any with the full proto shape; our JSON
// structure is close enough for the SDK's unmarshal.
func structToMap(b Build) map[string]any {
	return map[string]any{
		"id":         b.ID,
		"name":       b.Name,
		"projectId":  b.ProjectID,
		"status":     b.Status,
		"source":     b.Source,
		"steps":      b.Steps,
		"images":     b.Images,
		"createTime": b.CreateTime,
		"startTime":  b.StartTime,
		"finishTime": b.FinishTime,
		"logsBucket": b.LogsBucket,
	}
}
