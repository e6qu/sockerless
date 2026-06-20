// Package spawner runs the runner image via the local `docker` CLI
// (`docker run --rm -d …`). Kept dependency-free — talks to whichever
// daemon `DOCKER_HOST` points at, including local Podman, Docker
// Desktop, or sockerless. Doesn't import the docker SDK because the
// dispatcher's stated coupling is "Docker public API / CLI" (see
// PLAN.md ).
//
// One container per queued job. Container lifecycle:
//  1. `docker run -d --pull never <image> …`  (returns container ID)
//  2. The runner image's entrypoint registers the runner with GitHub
//     using `RUNNER_REG_TOKEN`, runs the job, exits.
//  3. The entrypoint's idle gate (RUNNER_IDLE_SECONDS) bounds only the
//     pre-pickup window: a runner that never gets a job exits cleanly
//     — duplicate-spawn races become benign — while a picked-up job
//     runs unbounded by the gate.
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
	"time"
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
	}
	args = append(args, hostGatewayArgs(ctx, req.DockerHost)...)
	args = append(args,
		"--label", LabelManagedBy+"="+LabelManagedVal,
		"--label", fmt.Sprintf("%s=%d", LabelJobID, req.JobID),
		"--label", LabelRunnerName+"="+req.RunnerName,
		// The registration token rides as a plain container env var. It is
		// visible in `docker inspect` to any local user who can reach this
		// daemon — a bounded risk: the token is a GitHub-ephemeral,
		// single-use, 1h registration token (not a PAT), consumed the
		// moment the runner registers. The cloud dispatchers (GCP Secret
		// Manager, Azure secret binding) keep it out of the resource's
		// plain env because their control plane offers a secret store; a
		// local docker daemon has no equivalent that also preserves the
		// runner image's `RUNNER_REG_TOKEN`-from-env bootstrap contract
		// (an env-file's value still resolves in `docker inspect`, and
		// stdin/file delivery would require changing the runner image's
		// entrypoint). So this stays as the documented bounded-risk path.
		"-e", "RUNNER_REG_TOKEN="+req.RegToken,
		"-e", "RUNNER_REPO="+req.Repo,
		"-e", "RUNNER_NAME="+req.RunnerName,
		"-e", "RUNNER_LABELS="+strings.Join(req.Labels, ","),
		"-e", fmt.Sprintf("RUNNER_IDLE_SECONDS=%d", idle),
		req.Image,
	)
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

// hostGatewayArgs returns the --add-host flag that makes
// host.docker.internal resolve inside spawned runner containers on
// Linux Docker (runner images dial back to host-published services —
// bleephub, sockerless — through it). Docker Desktop and Podman 4+
// provide the alias natively, and Podman REJECTS the host-gateway
// magic value, so the flag is engine-conditional — the same runtime
// detection the simulators use for their workload containers.
func hostGatewayArgs(ctx context.Context, dockerHost string) []string {
	cmd := exec.CommandContext(ctx, "docker", "version", "--format", "{{range .Server.Components}}{{.Name}} {{end}}")
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+dockerHost)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Engine unknown — don't guess a flag that can break the spawn;
		// the next Liveness check will surface a daemon problem.
		return nil
	}
	if strings.Contains(strings.ToLower(string(out)), "podman") {
		return nil
	}
	return []string{"--add-host", "host.docker.internal:host-gateway"}
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
	CreatedAt   time.Time // zero when the daemon's timestamp didn't parse
}

// dockerCreatedAtLayout matches `docker ps --format {{.CreatedAt}}`
// output (e.g. "2026-06-12 10:30:00 +0000 UTC").
const dockerCreatedAtLayout = "2006-01-02 15:04:05 -0700 MST"

// ListManaged returns every container on the daemon at DockerHost that
// carries the dispatcher's managed-by label, regardless of state. The
// dispatcher uses this on startup to rebuild the seen-set without
// on-disk state, and on the cleanup sweep (including the
// runner_job_timeout age bound) and graceful shutdown.
func ListManaged(ctx context.Context, dockerHost string) ([]Managed, error) {
	args := []string{
		"ps", "-a",
		"--filter", "label=" + LabelManagedBy + "=" + LabelManagedVal,
		"--format", "{{.ID}}|{{.State}}|{{.CreatedAt}}|{{.Label \"" + LabelJobID + "\"}}|{{.Label \"" + LabelRunnerName + "\"}}",
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
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		var jobID int64
		if v := strings.TrimSpace(parts[3]); v != "" {
			fmt.Sscanf(v, "%d", &jobID)
		}
		createdAt, parseErr := time.Parse(dockerCreatedAtLayout, strings.TrimSpace(parts[2]))
		if parseErr != nil {
			createdAt = time.Time{} // age-based reap skips unknown ages
		}
		managed = append(managed, Managed{
			ContainerID: parts[0],
			State:       parts[1],
			CreatedAt:   createdAt,
			JobID:       jobID,
			RunnerName:  parts[4],
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
