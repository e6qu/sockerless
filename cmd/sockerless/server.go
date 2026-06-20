package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func cmdServer(args []string) {
	if len(args) < 1 {
		serverUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "start":
		serverStart(args[1:])
	case "stop":
		serverStop()
	case "restart":
		serverStop()
		time.Sleep(500 * time.Millisecond)
		serverStart(args[1:])
	default:
		serverUsage()
		os.Exit(1)
	}
}

func serverUsage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless server <subcommand>

Subcommands:
  start     Start the backend server
  stop      Stop running server
  restart   Restart server`)
}

func serverStart(args []string) {
	fs := flag.NewFlagSet("server start", flag.ExitOnError)
	backendBin := fs.String("backend-bin", "", "path to backend binary (default: sockerless-backend-{type})")
	addr := fs.String("addr", ":3375", "listen address (Docker API + management)")
	_ = fs.Parse(args)

	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context; run 'sockerless context use <name>' first")
		os.Exit(1)
	}

	// config.yaml is the only runtime config shape. The legacy
	// contexts/<name>/config.json layout is no longer consulted at
	// runtime (operators on older state run `sockerless config migrate`).
	if !configFileExists() {
		fmt.Fprintln(os.Stderr, "error: no config.yaml present. Run `sockerless config init` or `sockerless config migrate` first.")
		os.Exit(1)
	}
	ucfg, err := loadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	env, ok := ucfg.Environments[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: context %q not found in config.yaml\n", name)
		os.Exit(1)
	}

	runDir := serverRunDir(name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	bBin := *backendBin
	if bBin == "" {
		bBin = "sockerless-backend-" + env.Backend
	}
	bBinPath, err := exec.LookPath(bBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: backend binary %q not found in PATH\n", bBin)
		os.Exit(1)
	}

	cmd := exec.Command(bBinPath, "-addr", *addr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Translate the structured environment config back into the
	// SOCKERLESS_*/AWS_REGION env vars the backend reads (the exact inverse
	// of `config migrate`). Without this the spawned backend inherits only
	// the parent process's env and a migrated context's region/cluster/agent
	// settings are silently never applied.
	cmd.Env = append(os.Environ(), environmentEnvVars(env)...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(runDir, "backend.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write PID file: %v\n", err)
	}
	fmt.Printf("Server started (PID %d) on %s\n", cmd.Process.Pid, *addr)

	// Liveness gate: poll the management /healthz so we don't report success
	// for a backend that died immediately (bad config, port already in use).
	if err := waitServerHealthy(env.Addr, cmd, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "error: server did not become healthy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Server started. Use 'sockerless status' to verify.")
}

// waitServerHealthy polls the management /healthz endpoint until it
// answers 200 or the timeout elapses, failing fast if the child process
// has already exited. mgmtAddr is the context's configured management
// base URL (env.Addr); when it's empty there's no reachable address to
// poll, so we fall back to confirming the process is still alive.
func waitServerHealthy(mgmtAddr string, cmd *exec.Cmd, timeout time.Duration) error {
	// Detect immediate child death independently of the HTTP probe.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.Now().Add(timeout)
	for {
		select {
		case werr := <-exited:
			if werr != nil {
				return fmt.Errorf("backend exited: %w", werr)
			}
			return fmt.Errorf("backend exited immediately")
		default:
		}
		if mgmtAddr == "" {
			// No management address to poll. Give the process a moment and
			// report alive only if it hasn't exited — we can't do better
			// without a reachable endpoint.
			time.Sleep(1 * time.Second)
			select {
			case werr := <-exited:
				if werr != nil {
					return fmt.Errorf("backend exited: %w", werr)
				}
				return fmt.Errorf("backend exited immediately")
			default:
				return nil
			}
		}
		if _, err := mgmtGet(mgmtAddr, "/internal/v1/healthz"); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for %s/internal/v1/healthz", timeout, mgmtAddr)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// environmentEnvVars renders a context environment into the
// SOCKERLESS_*/AWS_REGION env vars the backend reads — the inverse of the
// `config migrate` mapping. Only set keys are emitted (empty structured
// fields produce no env var) so the backend's own defaults still apply.
func environmentEnvVars(env *environment) []string {
	var out []string
	set := func(k, v string) {
		if v != "" {
			out = append(out, k+"="+v)
		}
	}
	setInt := func(k string, v int) {
		if v != 0 {
			out = append(out, k+"="+strconv.Itoa(v))
		}
	}
	setCSV := func(k string, vs []string) {
		if len(vs) > 0 {
			out = append(out, k+"="+strings.Join(vs, ","))
		}
	}

	if env.LogLevel != "" {
		set("SOCKERLESS_LOG_LEVEL", env.LogLevel)
	}

	// Common (shared) config.
	set("SOCKERLESS_AGENT_IMAGE", env.Common.AgentImage)
	set("SOCKERLESS_AGENT_TOKEN", env.Common.AgentToken)
	set("SOCKERLESS_CALLBACK_URL", env.Common.CallbackURL)
	set("SOCKERLESS_ENDPOINT_URL", env.Common.EndpointURL)
	set("SOCKERLESS_POLL_INTERVAL", env.Common.PollInterval)
	set("SOCKERLESS_AGENT_TIMEOUT", env.Common.AgentTimeout)

	if env.AWS != nil {
		set("AWS_REGION", env.AWS.Region)
		if ecs := env.AWS.ECS; ecs != nil {
			set("SOCKERLESS_ECS_CLUSTER", ecs.Cluster)
			setCSV("SOCKERLESS_ECS_SUBNETS", ecs.Subnets)
			setCSV("SOCKERLESS_ECS_SECURITY_GROUPS", ecs.SecurityGroups)
			set("SOCKERLESS_ECS_TASK_ROLE_ARN", ecs.TaskRoleARN)
			set("SOCKERLESS_ECS_EXECUTION_ROLE_ARN", ecs.ExecutionRoleARN)
			set("SOCKERLESS_ECS_LOG_GROUP", ecs.LogGroup)
			if ecs.AssignPublicIP {
				set("SOCKERLESS_ECS_PUBLIC_IP", "true")
			}
			set("SOCKERLESS_AGENT_EFS_ID", ecs.AgentEFSID)
		}
		if l := env.AWS.Lambda; l != nil {
			set("SOCKERLESS_LAMBDA_ROLE_ARN", l.RoleARN)
			set("SOCKERLESS_LAMBDA_LOG_GROUP", l.LogGroup)
			setInt("SOCKERLESS_LAMBDA_MEMORY_SIZE", l.MemorySize)
			setInt("SOCKERLESS_LAMBDA_TIMEOUT", l.Timeout)
			setCSV("SOCKERLESS_LAMBDA_SUBNETS", l.Subnets)
			setCSV("SOCKERLESS_LAMBDA_SECURITY_GROUPS", l.SecurityGroups)
		}
	}

	if env.GCP != nil {
		set("SOCKERLESS_GCP_BUILD_BUCKET", env.GCP.BuildBucket)
		set("SOCKERLESS_GCP_BUILD_PLATFORM", env.GCP.BuildPlatform)
		if cr := env.GCP.CloudRun; cr != nil {
			set("SOCKERLESS_GCR_PROJECT", env.GCP.Project)
			set("SOCKERLESS_GCR_REGION", cr.Region)
			set("SOCKERLESS_GCR_VPC_CONNECTOR", cr.VPCConnector)
			set("SOCKERLESS_GCR_LOG_ID", cr.LogID)
			set("SOCKERLESS_LOG_TIMEOUT", cr.LogTimeout)
		}
		if g := env.GCP.GCF; g != nil {
			set("SOCKERLESS_GCF_PROJECT", env.GCP.Project)
			set("SOCKERLESS_GCF_REGION", g.Region)
			set("SOCKERLESS_GCF_SERVICE_ACCOUNT", g.ServiceAccount)
			setInt("SOCKERLESS_GCF_TIMEOUT", g.Timeout)
			set("SOCKERLESS_GCF_MEMORY", g.Memory)
			set("SOCKERLESS_GCF_CPU", g.CPU)
			set("SOCKERLESS_LOG_TIMEOUT", g.LogTimeout)
		}
	}

	if env.Azure != nil {
		if aca := env.Azure.ACA; aca != nil {
			set("SOCKERLESS_ACA_SUBSCRIPTION_ID", env.Azure.SubscriptionID)
			set("SOCKERLESS_ACA_RESOURCE_GROUP", aca.ResourceGroup)
			set("SOCKERLESS_ACA_ENVIRONMENT", aca.Environment)
			set("SOCKERLESS_ACA_LOCATION", aca.Location)
			set("SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE", aca.LogAnalyticsWorkspace)
			set("SOCKERLESS_ACA_STORAGE_ACCOUNT", aca.StorageAccount)
		}
		if azf := env.Azure.AZF; azf != nil {
			set("SOCKERLESS_AZF_SUBSCRIPTION_ID", env.Azure.SubscriptionID)
			set("SOCKERLESS_AZF_RESOURCE_GROUP", azf.ResourceGroup)
			set("SOCKERLESS_AZF_LOCATION", azf.Location)
			set("SOCKERLESS_AZF_STORAGE_ACCOUNT", azf.StorageAccount)
			set("SOCKERLESS_AZF_REGISTRY", azf.Registry)
			set("SOCKERLESS_AZF_APP_SERVICE_PLAN", azf.AppServicePlan)
			setInt("SOCKERLESS_AZF_TIMEOUT", azf.Timeout)
			set("SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE", azf.LogAnalyticsWorkspace)
		}
	}

	return out
}

func serverStop() {
	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context")
		os.Exit(1)
	}

	runDir := serverRunDir(name)

	pidFile := filepath.Join(runDir, "backend.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("No running server found")
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(pidFile)
		fmt.Println("No running server found")
		return
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidFile)
		fmt.Println("No running server found")
		return
	}
	// os.FindProcess never errors on Unix even for a dead/recycled PID, so
	// probe liveness with signal 0 before sending SIGTERM — otherwise a
	// recycled PID belonging to an unrelated process would get killed.
	if err := p.Signal(syscall.Signal(0)); err != nil {
		os.Remove(pidFile)
		fmt.Printf("No running server found (PID %d not alive); cleaned up stale pidfile\n", pid)
		return
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not signal server (PID %d): %v\n", pid, err)
	} else {
		fmt.Printf("Sent SIGTERM to server (PID %d)\n", pid)
	}
	os.Remove(pidFile)
}

func serverRunDir(contextName string) string {
	return filepath.Join(sockerlessDir(), "run", contextName)
}
