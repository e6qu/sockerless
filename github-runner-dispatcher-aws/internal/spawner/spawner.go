// Package spawner runs the runner image via the local `docker` CLI
// (`docker run --rm -d …`). Kept dependency-free — talks to whichever
// daemon `DOCKER_HOST` points at, including local Podman, Docker
// Desktop, or sockerless. Doesn't import the docker SDK because the
// dispatcher's stated coupling is "Docker public API / CLI" (see
// PLAN.md Phase 110a).
//
// One container per queued job. Container lifecycle:
//  1. `docker run -d --pull never <image> …`  (returns container ID)
//  2. The runner image's entrypoint registers the runner with GitHub
//     using `RUNNER_REG_TOKEN`, runs the job, exits.
//  3. The 60-s idle timeout inside the entrypoint kills the container
//     if no job arrives — duplicate-spawn races become benign.
//
// `--rm` is preferred so successful runs auto-clean; `--pull never`
// avoids surprise registry traffic on every spawn (operator pre-pulls).
package spawner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Labels stamped on every spawned container so a restarted dispatcher
// can rediscover its state from the docker daemon (no on-disk
// dispatcher state). Docker's `--filter label=KEY=VALUE` is the lookup
// key; the JobID label is what gets parsed back into the seen-set.
const (
	LabelJobID      = "sockerless.dispatcher.job_id"
	LabelRunnerName = "sockerless.dispatcher.runner_name"
	LabelManagedBy  = "sockerless.dispatcher.managed_by"
	LabelManagedVal = "github-runner-dispatcher"
)

// Request is one spawn directive.
type Request struct {
	DockerHost  string // tcp://host:port or unix:///var/run/docker.sock
	Image       string // runner image URI
	RegToken    string // GitHub ephemeral runner registration token
	Repo        string // owner/repo for runner registration
	RunnerName  string // unique name; logs / Actions UI uses it
	Labels      []string
	IdleSeconds int   // seconds to wait for the runner to register; 0 → 60 s default
	JobID       int64 // GitHub workflow_job ID — written to LabelJobID for restart recovery
}

// Spawn shells out to `docker run -d`. Returns the container ID on
// success (12-char short ID is fine).
func Spawn(ctx context.Context, req Request) (string, error) {
	if req.DockerHost == "" {
		return "", fmt.Errorf("docker host required")
	}
	if req.Image == "" {
		return "", fmt.Errorf("image required")
	}
	if req.RegToken == "" {
		return "", fmt.Errorf("registration token required")
	}
	if req.Repo == "" {
		return "", fmt.Errorf("repo required")
	}
	if req.RunnerName == "" {
		return "", fmt.Errorf("runner name required")
	}
	idle := req.IdleSeconds
	if idle <= 0 {
		idle = 60
	}
	args := []string{
		"run", "--rm", "-d",
		"--pull", "never",
		"--name", req.RunnerName,
		"--label", LabelManagedBy + "=" + LabelManagedVal,
		"--label", fmt.Sprintf("%s=%d", LabelJobID, req.JobID),
		"--label", LabelRunnerName + "=" + req.RunnerName,
		"-e", "RUNNER_REG_TOKEN=" + req.RegToken,
		"-e", "RUNNER_REPO=" + req.Repo,
		"-e", "RUNNER_NAME=" + req.RunnerName,
		"-e", "RUNNER_LABELS=" + strings.Join(req.Labels, ","),
		"-e", fmt.Sprintf("RUNNER_IDLE_SECONDS=%d", idle),
		req.Image,
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+req.DockerHost)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("docker run returned empty container ID")
	}
	return id, nil
}

// Liveness reports whether the docker daemon at DockerHost answers
// `docker info`. Used to skip a poll cycle when the daemon is down.
// Doesn't crash the dispatcher — the next poll re-checks.
func Liveness(ctx context.Context, dockerHost string) error {
	cmd := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+dockerHost)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker info on %s: %v: %s", dockerHost, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Managed describes a running container the dispatcher previously
// spawned (matched via LabelManagedBy). Used by state recovery on
// startup and the cleanup sweep.
type Managed struct {
	ContainerID string
	JobID       int64
	RunnerName  string
	State       string // "running", "exited", "created", …
	DockerHost  string
}

// ListManaged returns every container on the daemon at DockerHost that
// carries the dispatcher's managed-by label, regardless of state. The
// dispatcher uses this on startup to rebuild the seen-set without
// on-disk state, and on graceful shutdown to clean up.
func ListManaged(ctx context.Context, dockerHost string) ([]Managed, error) {
	args := []string{
		"ps", "-a",
		"--filter", "label=" + LabelManagedBy + "=" + LabelManagedVal,
		"--format", "{{.ID}}|{{.State}}|{{.Label \"" + LabelJobID + "\"}}|{{.Label \"" + LabelRunnerName + "\"}}",
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+dockerHost)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps on %s: %v: %s", dockerHost, err, strings.TrimSpace(string(out)))
	}
	var managed []Managed
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		var jobID int64
		if v := strings.TrimSpace(parts[2]); v != "" {
			fmt.Sscanf(v, "%d", &jobID)
		}
		managed = append(managed, Managed{
			ContainerID: parts[0],
			State:       parts[1],
			JobID:       jobID,
			RunnerName:  parts[3],
			DockerHost:  dockerHost,
		})
	}
	return managed, nil
}

// StopAndRemove stops a container (timeout 10 s) and removes it.
// Tolerates already-gone (`docker stop` on an exited container is a
// no-op; `docker rm` on a non-existent ID returns a recognised
// error). Used by the cleanup sweep + graceful shutdown.
func StopAndRemove(ctx context.Context, dockerHost, containerID string) error {
	stop := exec.CommandContext(ctx, "docker", "stop", "-t", "10", containerID)
	stop.Env = append(os.Environ(), "DOCKER_HOST="+dockerHost)
	if out, err := stop.CombinedOutput(); err != nil {
		// If the container's already gone, `docker stop` returns an
		// error; we still attempt rm so the call is idempotent.
		_ = out
	}
	rm := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
	rm.Env = append(os.Environ(), "DOCKER_HOST="+dockerHost)
	if out, err := rm.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		// `--rm` containers self-remove on stop, so a "no such container"
		// error is expected; treat as success.
		if strings.Contains(msg, "No such container") {
			return nil
		}
		return fmt.Errorf("docker rm %s: %v: %s", containerID, err, msg)
	}
	return nil
}
