package tests

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// simBackendInfo describes how to start a cloud backend in simulator mode.
type simBackendInfo struct {
	// Name is the backend identifier (e.g. "ecs", "lambda").
	Name string
	// Cloud is the simulator cloud (e.g. "aws", "gcp", "azure").
	Cloud string
	// BackendDir is the relative path from repo root to the backend module.
	BackendDir string
	// CmdDir is the relative path from the backend module to the cmd directory.
	CmdDir string
	// BinaryName is the output binary name.
	BinaryName string
	// EnvVarSocket is the env var name for the Unix socket path.
	EnvVarSocket string
	// ExtraEnv are additional env vars for the backend process.
	ExtraEnv map[string]string
}

var simBackends = map[string]simBackendInfo{
	"ecs": {
		Name:         "ecs",
		Cloud:        "aws",
		BackendDir:   "backends/ecs",
		CmdDir:       "./cmd/sockerless-backend-ecs",
		BinaryName:   "sockerless-backend-ecs",
		EnvVarSocket: "SOCKERLESS_ECS_SOCKET",
		ExtraEnv: map[string]string{
			"SOCKERLESS_ECS_CLUSTER":            "sim-cluster",
			"SOCKERLESS_ECS_SUBNETS":            "subnet-0123456789abcdef0",
			"SOCKERLESS_ECS_EXECUTION_ROLE_ARN": "arn:aws:iam::000000000000:role/sim",
			// BUG-848 made arch mandatory; no default.
			"SOCKERLESS_ECS_CPU_ARCHITECTURE": "X86_64",
		},
	},
	"lambda": {
		Name:         "lambda",
		Cloud:        "aws",
		BackendDir:   "backends/lambda",
		CmdDir:       "./cmd/sockerless-backend-lambda",
		BinaryName:   "sockerless-backend-lambda",
		EnvVarSocket: "SOCKERLESS_LAMBDA_SOCKET",
		ExtraEnv: map[string]string{
			"SOCKERLESS_LAMBDA_ROLE_ARN":     "arn:aws:iam::000000000000:role/sim",
			"SOCKERLESS_LAMBDA_ARCHITECTURE": "x86_64",
		},
	},
	"cloudrun": {
		Name:         "cloudrun",
		Cloud:        "gcp",
		BackendDir:   "backends/cloudrun",
		CmdDir:       "./cmd/sockerless-backend-cloudrun",
		BinaryName:   "sockerless-backend-cloudrun",
		EnvVarSocket: "SOCKERLESS_CLOUDRUN_SOCKET",
		ExtraEnv: map[string]string{
			"SOCKERLESS_GCR_PROJECT": "sim-project",
		},
	},
	"gcf": {
		Name:         "gcf",
		Cloud:        "gcp",
		BackendDir:   "backends/cloudrun-functions",
		CmdDir:       "./cmd/sockerless-backend-gcf",
		BinaryName:   "sockerless-backend-gcf",
		EnvVarSocket: "SOCKERLESS_GCF_SOCKET",
		ExtraEnv: map[string]string{
			"SOCKERLESS_GCF_PROJECT": "sim-project",
		},
	},
	"aca": {
		Name:         "aca",
		Cloud:        "azure",
		BackendDir:   "backends/aca",
		CmdDir:       "./cmd/sockerless-backend-aca",
		BinaryName:   "sockerless-backend-aca",
		EnvVarSocket: "SOCKERLESS_ACA_SOCKET",
		ExtraEnv: map[string]string{
			"SOCKERLESS_ACA_SUBSCRIPTION_ID": "00000000-0000-0000-0000-000000000001",
			"SOCKERLESS_ACA_RESOURCE_GROUP":  "sim-rg",
		},
	},
	"azf": {
		Name:         "azf",
		Cloud:        "azure",
		BackendDir:   "backends/azure-functions",
		CmdDir:       "./cmd/sockerless-backend-azf",
		BinaryName:   "sockerless-backend-azf",
		EnvVarSocket: "SOCKERLESS_AZF_SOCKET",
		ExtraEnv: map[string]string{
			"SOCKERLESS_AZF_SUBSCRIPTION_ID": "00000000-0000-0000-0000-000000000001",
			"SOCKERLESS_AZF_RESOURCE_GROUP":  "sim-rg",
			"SOCKERLESS_AZF_STORAGE_ACCOUNT": "simstorage",
		},
	},
}

// simProcess tracks a running process for cleanup.
type simProcess struct {
	cmd        *exec.Cmd
	binaryPath string
}

// parseSimBackends parses the SOCKERLESS_SIM env var into a list of backend names.
func parseSimBackends(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// cloudsForBackends returns the unique set of simulator clouds needed.
func cloudsForBackends(backendNames []string) []string {
	seen := map[string]bool{}
	var clouds []string
	for _, name := range backendNames {
		info, ok := simBackends[name]
		if !ok {
			continue
		}
		if !seen[info.Cloud] {
			seen[info.Cloud] = true
			clouds = append(clouds, info.Cloud)
		}
	}
	return clouds
}

// buildSimulator builds the simulator binary for the given cloud and returns the path.
func buildSimulator(cloud string) (string, error) {
	simDir := findModuleDir("simulators/" + cloud)
	binaryPath := simDir + "/simulator-" + cloud

	binaryName := "simulator-" + cloud
	fmt.Printf("[sim] Building %s...\n", binaryName)
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryName, ".")
	build.Dir = simDir
	// GOWORK=off because simulators are not in the workspace.
	// Filter out GOOS/GOARCH from env to ensure we build for the host platform,
	// since Docker test configs may set GOOS=linux.
	var filteredEnv []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOOS=") || strings.HasPrefix(e, "GOARCH=") {
			continue
		}
		filteredEnv = append(filteredEnv, e)
	}
	build.Env = append(filteredEnv, "GOWORK=off")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return "", fmt.Errorf("failed to build simulator-%s: %w", cloud, err)
	}
	return binaryPath, nil
}

// startSimulator starts a simulator process and returns the process info and its URL.
func startSimulator(binaryPath string, cloud string) (*simProcess, string, error) {
	port := findFreePort()
	addr := fmt.Sprintf(":%d", port)

	fmt.Printf("[sim] Starting simulator-%s on %s...\n", cloud, addr)
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "SIM_LISTEN_ADDR="+addr)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("failed to start simulator-%s: %w", cloud, err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	return &simProcess{cmd: cmd, binaryPath: binaryPath}, url, nil
}

// buildBackendBinary builds a cloud backend binary and returns the path.
func buildBackendBinary(info simBackendInfo) (string, error) {
	backendDir := findModuleDir(info.BackendDir)
	binaryPath := backendDir + "/" + info.BinaryName

	fmt.Printf("[sim] Building %s...\n", info.BinaryName)
	build := exec.Command("go", "build", "-tags", "noui", "-o", info.BinaryName, info.CmdDir)
	build.Dir = backendDir
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return "", fmt.Errorf("failed to build %s: %w", info.BinaryName, err)
	}
	return binaryPath, nil
}

// startBackend starts a cloud backend process pointing at a simulator.
func startBackend(binaryPath string, info simBackendInfo, simURL string) (*simProcess, int, error) {
	port := findFreePort()
	addr := fmt.Sprintf(":%d", port)

	fmt.Printf("[sim] Starting %s on %s (sim=%s)...\n", info.BinaryName, addr, simURL)
	cmd := exec.Command(binaryPath, "--addr", addr, "--log-level", "debug")

	// Build environment: start with current env, add endpoint URL and extra env
	env := append(os.Environ(), "SOCKERLESS_ENDPOINT_URL="+simURL)
	for k, v := range info.ExtraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, 0, fmt.Errorf("failed to start %s: %w", info.BinaryName, err)
	}

	return &simProcess{cmd: cmd, binaryPath: binaryPath}, port, nil
}

// setupSimulatorState creates initial resources in the simulator that backends expect.
func setupSimulatorState(cloud string, simURL string, backendNames []string) error {
	switch cloud {
	case "aws":
		return setupAWSSimulator(simURL, backendNames)
	}
	return nil
}

func setupAWSSimulator(simURL string, backendNames []string) error {
	for _, name := range backendNames {
		info := simBackends[name]
		if info.Cloud != "aws" {
			continue
		}
		if name == "ecs" {
			// Create the ECS cluster
			clusterName := info.ExtraEnv["SOCKERLESS_ECS_CLUSTER"]
			body := fmt.Sprintf(`{"clusterName":"%s"}`, clusterName)
			req, _ := http.NewRequest("POST", simURL+"/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/x-amz-json-1.1")
			req.Header.Set("X-Amz-Target", "AmazonEC2ContainerServiceV20141113.CreateCluster")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("failed to create ECS cluster: %w", err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				return fmt.Errorf("failed to create ECS cluster: status %d", resp.StatusCode)
			}
			fmt.Printf("[sim] Created ECS cluster %q in simulator\n", clusterName)
		}
	}
	return nil
}

// startSimBackends orchestrates building and starting simulators and cloud backends
// for all backends specified in the SOCKERLESS_SIM env var.
// Backends serve the Docker API directly (in-process wiring), so no separate frontend is needed.
// Returns all processes for cleanup, socket paths for cleanup, and any error.
func startSimBackends(simVal string) ([]*simProcess, []string, error) {
	backendNames := parseSimBackends(simVal)
	if len(backendNames) == 0 {
		return nil, nil, nil
	}

	var allProcesses []*simProcess
	var allSocketPaths []string

	// cleanup kills all started processes on error
	cleanup := func() {
		for i := len(allProcesses) - 1; i >= 0; i-- {
			allProcesses[i].cmd.Process.Kill()
			allProcesses[i].cmd.Wait()
			if allProcesses[i].binaryPath != "" {
				os.Remove(allProcesses[i].binaryPath)
			}
		}
		for _, s := range allSocketPaths {
			os.Remove(s)
		}
	}

	// Step 1: Build and start simulators (one per cloud)
	clouds := cloudsForBackends(backendNames)
	simURLs := map[string]string{} // cloud -> simulator URL

	for _, cloud := range clouds {
		binaryPath, err := buildSimulator(cloud)
		if err != nil {
			cleanup()
			return nil, nil, err
		}

		proc, url, err := startSimulator(binaryPath, cloud)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		allProcesses = append(allProcesses, proc)

		// Wait for simulator health
		if err := waitForReady(url+"/health", 10*time.Second); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("simulator-%s not ready: %w", cloud, err)
		}
		fmt.Printf("[sim] simulator-%s is ready at %s\n", cloud, url)
		simURLs[cloud] = url

		// Create initial resources in the simulator
		if err := setupSimulatorState(cloud, url, backendNames); err != nil {
			cleanup()
			return nil, nil, err
		}
	}

	// Step 2: For each backend, build binary, start backend, start frontend, set env var
	for _, name := range backendNames {
		info, ok := simBackends[name]
		if !ok {
			cleanup()
			return nil, nil, fmt.Errorf("unknown sim backend: %s", name)
		}

		simURL, ok := simURLs[info.Cloud]
		if !ok {
			cleanup()
			return nil, nil, fmt.Errorf("no simulator for cloud %s (backend %s)", info.Cloud, name)
		}

		// Build backend binary
		binaryPath, err := buildBackendBinary(info)
		if err != nil {
			cleanup()
			return nil, nil, err
		}

		// Start backend
		backendProc, backendPort, err := startBackend(binaryPath, info, simURL)
		if err != nil {
			cleanup()
			return nil, nil, err
		}
		allProcesses = append(allProcesses, backendProc)

		// Wait for backend readiness
		backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
		if err := waitForReady(backendURL, 15*time.Second); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("%s backend not ready: %w", name, err)
		}
		fmt.Printf("[sim] %s backend is ready on :%d (serving Docker API)\n", name, backendPort)

		// Backend serves Docker API directly — set env var to TCP address
		os.Setenv(info.EnvVarSocket, fmt.Sprintf("tcp://localhost:%d", backendPort))
	}

	return allProcesses, allSocketPaths, nil
}
