package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// InstanceLifecycle shells out to the per-component make targets (see
// make/components.mk) to start / stop / rebuild a single instance.
// Stays out of the in-process topology lock — callers grab the
// instance via TopologyManager first, then drive lifecycle here.
//
// repoRoot is the directory the make commands run in. Empty string
// uses the process's current working dir (the typical case when
// admin is launched from the repo root via `make stack-up`).
type InstanceLifecycle struct {
	repoRoot string
	timeout  time.Duration
}

// NewInstanceLifecycle constructs a lifecycle shell. timeout caps any
// single make invocation; 0 means no timeout.
func NewInstanceLifecycle(repoRoot string, timeout time.Duration) *InstanceLifecycle {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &InstanceLifecycle{repoRoot: repoRoot, timeout: timeout}
}

// Start invokes `make start-component` with the per-instance
// arguments derived from inst + simPort (resolved by caller from the
// instance's Sim ref, when applicable). project scopes admin-managed
// per-instance state directories.
func (l *InstanceLifecycle) Start(ctx context.Context, project string, inst Instance, simPort int) error {
	args := []string{
		"start-component",
		"KIND=" + string(inst.Kind),
		"NAME=" + inst.Name,
		"PORT=" + strconv.Itoa(inst.Port),
	}
	if inst.Cloud != "" {
		args = append(args, "CLOUD="+string(inst.Cloud))
	}
	if inst.Backend != "" {
		args = append(args, "BACKEND="+string(inst.Backend))
	}
	if inst.Kind == InstanceKindBackend && simPort > 0 {
		args = append(args, "SIM_PORT="+strconv.Itoa(simPort))
	}

	// Compose the env map admin actually writes to .stack-pids/<n>.env.
	// Operator config wins over admin-managed entries — this lets an
	// operator override SIM_DATA_DIR if they want a non-default path,
	// e.g. mounting state on a separate volume. Admin only fills in
	// fields the operator hasn't set.
	managed := managedEnvFor(project, inst, l.stateRoot())
	cfg := mergeConfig(managed, inst.Config)

	if len(cfg) > 0 {
		envFile := envFilePath(l.repoRoot, inst.Name)
		if err := writeEnvFile(envFile, cfg); err != nil {
			return fmt.Errorf("write env file: %w", err)
		}
		args = append(args, "ENV_FILE="+envFile)
	}
	return l.runMake(ctx, args...)
}

// stateRoot returns the directory under which per-instance state
// directories live. Resolves to <repoRoot>/.sockerless-state/, or
// <cwd>/.sockerless-state/ when repoRoot is empty (the typical case
// where admin runs from the repo root).
func (l *InstanceLifecycle) stateRoot() string {
	root := l.repoRoot
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	return filepath.Join(root, ".sockerless-state")
}

// managedEnvFor returns the env entries admin synthesises per kind.
// Sim instances get SIM_DATA_DIR pointing at
// <stateRoot>/<project>/<instance>/ so multiple sim instances of the
// same cloud coexist with isolated state. Operator opts into
// persistence by adding SIM_PERSIST=true to the instance Config —
// admin doesn't force it; per the components-decoupled invariant,
// persistence is a behaviour choice, not an admin-imposed default.
func managedEnvFor(project string, inst Instance, stateRoot string) map[string]string {
	if inst.Kind != InstanceKindSim {
		return nil
	}
	return map[string]string{
		"SIM_DATA_DIR": filepath.Join(stateRoot, project, inst.Name),
	}
}

// mergeConfig overlays operator-provided values on top of
// admin-managed defaults. nil maps are treated as empty.
func mergeConfig(managed, operator map[string]string) map[string]string {
	if len(managed) == 0 && len(operator) == 0 {
		return nil
	}
	out := make(map[string]string, len(managed)+len(operator))
	for k, v := range managed {
		out[k] = v
	}
	for k, v := range operator {
		out[k] = v
	}
	return out
}

// Stop invokes `make stop-component NAME=<inst.Name>`.
func (l *InstanceLifecycle) Stop(ctx context.Context, inst Instance) error {
	return l.runMake(ctx, "stop-component", "NAME="+inst.Name)
}

// Reload re-renders the env file and signals the component via
// `make reload-component NAME=…`. The make target sends SIGHUP to
// the recorded PID; component-side handling of SIGHUP is the
// component's concern. No-op components (i.e. those that ignore
// SIGHUP) silently succeed at the make level — admin's contract is
// "signal sent", not "config absorbed".
func (l *InstanceLifecycle) Reload(ctx context.Context, inst Instance) error {
	// Re-render the env file so a subsequent restart picks up the
	// latest values. Reload doesn't fork the process, so the running
	// component still sees the env it was started with — but the
	// next start-component will see the updated file.
	if len(inst.Config) > 0 {
		envFile := envFilePath(l.repoRoot, inst.Name)
		if err := writeEnvFile(envFile, inst.Config); err != nil {
			return fmt.Errorf("write env file: %w", err)
		}
	}
	return l.runMake(ctx, "reload-component", "NAME="+inst.Name)
}

// Rebuild invokes `make rebuild-component KIND=… [CLOUD=…] [BACKEND=…]`.
func (l *InstanceLifecycle) Rebuild(ctx context.Context, inst Instance) error {
	args := []string{"rebuild-component", "KIND=" + string(inst.Kind)}
	if inst.Cloud != "" {
		args = append(args, "CLOUD="+string(inst.Cloud))
	}
	if inst.Backend != "" {
		args = append(args, "BACKEND="+string(inst.Backend))
	}
	return l.runMake(ctx, args...)
}

func (l *InstanceLifecycle) runMake(ctx context.Context, args ...string) error {
	if l.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "make", args...)
	if l.repoRoot != "" {
		cmd.Dir = l.repoRoot
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make %s: %w (output: %s)",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func envFilePath(repoRoot, instanceName string) string {
	root := repoRoot
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	return filepath.Join(root, ".stack-pids", instanceName+".env")
}

// writeEnvFile renders cfg as a `KEY=VALUE` line per entry, sorted
// for deterministic output (admin Replace stays idempotent on the
// env-file write path).
func writeEnvFile(path string, cfg map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys := sortedConfigKeys(cfg)
	var b strings.Builder
	b.WriteString("# generated by sockerless-admin; do not edit by hand\n")
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(cfg[k])
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func sortedConfigKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Avoid pulling sort into this file's deps unnecessarily; small N.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
