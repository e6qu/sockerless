package ecs

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

var dockerClient *client.Client
var evalBinaryPath string
var evalImageName string

func TestMain(m *testing.M) {
	if os.Getenv("SOCKERLESS_INTEGRATION") != "1" {
		// In CI, silent short-circuit would let integration tests "pass" by
		// not running. Require the env var explicitly so a missing CI config
		// fails loudfollow-up).
		if os.Getenv("GITHUB_ACTIONS") == "true" || os.Getenv("CI") == "true" {
			fmt.Fprintln(os.Stderr, "ERROR: SOCKERLESS_INTEGRATION must be set to 1 in CI — integration tests would otherwise be silently skipped.")
			os.Exit(1)
		}
		// Local dev: run whatever unit tests exist and exit.
		os.Exit(m.Run())
	}

	repoRoot := findModuleDir(".")
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// Build eval-arithmetic binary (static linux/amd64, to be embedded
	// in a Docker image the container runtime can actually execute).
	evalDir := repoRoot + "/simulators/testdata/eval-arithmetic"
	evalBinaryPath = evalDir + "/eval-arithmetic"
	fmt.Println("[sim] Building eval-arithmetic (linux/amd64)...")
	evalBuild := exec.Command("go", "build", "-o", "eval-arithmetic", ".")
	evalBuild.Dir = evalDir
	evalBuild.Env = filterBuildEnv(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOOS=linux", "GOARCH=amd64")
	evalBuild.Stdout = os.Stderr
	evalBuild.Stderr = os.Stderr
	if err := evalBuild.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build eval-arithmetic: %v\n", err)
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(evalBinaryPath) })

	// Bake the binary into a local Docker image so the container can
	// actually run it. Matches the pattern in simulators/aws/sdk-tests.
	evalImageName = "sockerless-eval-arithmetic:test"
	fmt.Printf("[sim] Building %s...\n", evalImageName)
	evalDockerfile := "FROM alpine:latest\nCOPY eval-arithmetic /usr/local/bin/eval-arithmetic\nENTRYPOINT [\"/usr/local/bin/eval-arithmetic\"]\n"
	evalImageBuild := exec.Command("docker", "build", "-t", evalImageName, "-f", "-", evalDir)
	evalImageBuild.Stdin = strings.NewReader(evalDockerfile)
	if out, err := evalImageBuild.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build eval-arithmetic image: %v\n%s", err, out)
		os.Exit(1)
	}

	// Build simulator
	simDir := repoRoot + "/simulators/aws"
	simBinary := simDir + "/simulator-aws"
	fmt.Println("[sim] Building simulator-aws...")
	build := exec.Command("go", "build", "-tags", "noui", "-o", "simulator-aws", ".")
	build.Dir = simDir
	build.Env = filterBuildEnv(os.Environ(), "GOWORK=off")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build simulator-aws: %v\n", err)
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(simBinary) })

	// Start simulator
	simPort := findFreePort()
	simAddr := fmt.Sprintf(":%d", simPort)
	simURL := fmt.Sprintf("http://127.0.0.1:%d", simPort)
	fmt.Printf("[sim] Starting simulator-aws on %s...\n", simAddr)
	simCmd := exec.Command(simBinary)
	simCmd.Env = append(os.Environ(),
		"SIM_LISTEN_ADDR="+simAddr,
		"PATH="+os.Getenv("PATH"),
	)
	simCmd.Stdout = os.Stderr
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start simulator-aws: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { simCmd.Process.Kill(); simCmd.Wait() })

	if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "simulator-aws not ready: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	fmt.Printf("[sim] simulator-aws is ready at %s\n", simURL)

	// Create ECS cluster in simulator
	clusterName := "sim-cluster"
	body := fmt.Sprintf(`{"clusterName":"%s"}`, clusterName)
	req, _ := http.NewRequest("POST", simURL+"/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerServiceV20141113.CreateCluster")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create ECS cluster: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	resp.Body.Close()
	fmt.Printf("[sim] Created ECS cluster %q in simulator\n", clusterName)

	// Build backend
	backendDir := repoRoot + "/backends/ecs"
	backendBinary := backendDir + "/sockerless-backend-ecs"
	fmt.Println("[sim] Building sockerless-backend-ecs...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-ecs", "./cmd/sockerless-backend-ecs")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build backend: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(backendBinary) })

	// Start backend
	backendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	fmt.Printf("[sim] Starting sockerless-backend-ecs on %s...\n", backendAddr)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+simURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_ECS_CLUSTER=sim-cluster",
		"SOCKERLESS_ECS_SUBNETS=subnet-sim",
		"SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::000000000000:role/sim",
	)
	backendCmd.Stdout = os.Stderr
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start backend: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { backendCmd.Process.Kill(); backendCmd.Wait() })

	backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
	if err := waitForReady(backendURL, 15*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "backend not ready: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	fmt.Printf("[sim] backend is ready on %s\n", backendAddr)

	// The ECS backend serves the Docker API directly (no separate
	// frontend binary — in-process wiring per post-P67 architecture).
	// Point the docker SDK at the backend's TCP address.
	dockerClient, err = client.NewClientWithOpts(
		client.WithHost(fmt.Sprintf("tcp://localhost:%d", backendPort)),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create docker client: %v\n", err)
		cleanup()
		os.Exit(1)
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("SOCKERLESS_INTEGRATION") != "1" {
		t.Skip("skipping integration test (SOCKERLESS_INTEGRATION != 1)")
	}
}

func TestECSContainerLifecycle(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	// Pull image
	rc, err := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	defer rc.Close()
	buf := make([]byte, 4096)
	for {
		if _, err := rc.Read(buf); err != nil {
			break
		}
	}

	// Create container
	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "hello from ecs"},
		Tty:   false,
	}, nil, nil, nil, "ecs-lifecycle-"+generateTestID())
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	// Start
	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Wait
	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
	}

	// Inspect
	info, err := dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Status != "exited" {
		t.Errorf("expected status 'exited', got %q", info.State.Status)
	}
}

func TestECSContainerLogs(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	pullRC, _ := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "log-test-output"},
	}, nil, nil, nil, "ecs-logs-"+generateTestID())
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Wait for exit
	waitCh, _ := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout")
	}

	// Get logs
	logRC, err := dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}
	defer logRC.Close()

	logBuf := make([]byte, 4096)
	n, _ := logRC.Read(logBuf)
	logOutput := string(logBuf[:n])
	if !strings.Contains(logOutput, "log-test-output") {
		t.Errorf("expected logs to contain 'log-test-output', got %q", logOutput)
	}
}

func TestECSContainerExec(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	pullRC, _ := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:     "alpine:latest",
		Cmd:       []string{"tail", "-f", "/dev/null"},
		OpenStdin: true,
		Tty:       true,
	}, nil, nil, nil, "ecs-exec-"+generateTestID())
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Create exec
	execResp, err := dockerClient.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
		Cmd:          []string{"echo", "exec-output"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	// Start exec
	hijacked, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		t.Fatalf("exec start failed: %v", err)
	}
	output, _ := io.ReadAll(hijacked.Reader)
	hijacked.Close()

	if !strings.Contains(string(output), "exec-output") {
		t.Errorf("expected exec output to contain 'exec-output', got %q", string(output))
	}

	// Stop container
	timeout := 5
	dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout})
}

func TestECSContainerList(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	pullRC, _ := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:  "alpine:latest",
		Cmd:    []string{"sleep", "30"},
		Labels: map[string]string{"test": "ecs-list"},
	}, nil, nil, nil, "ecs-list-"+generateTestID())
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// List running containers
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	found := false
	for _, ctr := range containers {
		if ctr.ID == resp.ID {
			found = true
			if ctr.Labels["test"] != "ecs-list" {
				t.Errorf("expected label test=ecs-list")
			}
			break
		}
	}
	if !found {
		t.Error("container not found in list")
	}

	timeout := 5
	dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout})
}

func TestECSNetworkOperations(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	// Create network
	netName := "ecs-test-net-" + generateTestID()
	netResp, err := dockerClient.NetworkCreate(ctx, netName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer dockerClient.NetworkRemove(ctx, netResp.ID)

	// Inspect
	netInfo, err := dockerClient.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if netInfo.Name != netName {
		t.Errorf("expected name %q, got %q", netName, netInfo.Name)
	}

	// List
	networks, err := dockerClient.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		t.Fatalf("network list failed: %v", err)
	}
	found := false
	for _, n := range networks {
		if n.ID == netResp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("network not found in list")
	}
}

// TestECSVolumeOperations pins BUG-731 — named volumes are not
// supported on ECS (Fargate tasks are ephemeral; real persistence
// requires EFS volume configuration on the task def). Real per-cloud
// volume provisioning tracked as Phase 91.
func TestECSVolumeOperations(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	_, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: "ecs-test-vol-" + generateTestID()})
	if err == nil {
		t.Fatal("expected VolumeCreate to fail with NotImplemented")
	}
	if !strings.Contains(err.Error(), "does not support named volumes") {
		t.Errorf("expected NotImplemented error, got: %v", err)
	}
}

// --- helpers ---

func findModuleDir(rel string) string {
	// We're in backends/ecs, repo root is../..
	candidates := []string{
		"../..",
		"../../..",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c + "/go.work"); err == nil {
			return c
		}
	}
	return "../.."
}

func findFreePort() int {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func waitForReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func generateTestID(parts ...string) string {
	id := time.Now().Format("150405")
	for _, p := range parts {
		id += "-" + p
	}
	return id
}

func filterBuildEnv(env []string, extra ...string) []string {
	var filtered []string
	for _, e := range env {
		if strings.HasPrefix(e, "GOOS=") || strings.HasPrefix(e, "GOARCH=") {
			continue
		}
		filtered = append(filtered, e)
	}
	return append(filtered, extra...)
}
