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

func TestMain(m *testing.M) {
	if os.Getenv("SOCKERLESS_INTEGRATION") != "1" {
		fmt.Println("skipping integration tests (SOCKERLESS_INTEGRATION != 1)")
		os.Exit(0)
	}

	repoRoot := findModuleDir(".")
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// Build simulator
	simDir := repoRoot + "/simulators/aws"
	simBinary := simDir + "/simulator-aws"
	fmt.Println("[sim] Building simulator-aws...")
	build := exec.Command("go", "build", "-o", "simulator-aws", ".")
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
	simCmd.Env = append(os.Environ(), "SIM_LISTEN_ADDR="+simAddr)
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
	buildBackend := exec.Command("go", "build", "-o", "sockerless-backend-ecs", "./cmd/sockerless-backend-ecs")
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

	// Build frontend
	frontendDir := repoRoot + "/frontends/docker"
	frontendBinary := frontendDir + "/sockerless-docker-frontend"
	fmt.Println("[sim] Building sockerless-docker-frontend...")
	buildFrontend := exec.Command("go", "build", "-o", "sockerless-docker-frontend", "./cmd/")
	buildFrontend.Dir = frontendDir
	buildFrontend.Stdout = os.Stderr
	buildFrontend.Stderr = os.Stderr
	if err := buildFrontend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build frontend: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(frontendBinary) })

	// Start frontend on Unix socket
	socketPath := fmt.Sprintf("/tmp/sockerless-ecs-inttest-%d.sock", os.Getpid())
	os.Remove(socketPath)
	fmt.Printf("[sim] Starting frontend on %s...\n", socketPath)
	frontendCmd := exec.Command(frontendBinary,
		"--addr", socketPath,
		"--backend", fmt.Sprintf("http://localhost:%d", backendPort),
		"--log-level", "debug",
	)
	frontendCmd.Stdout = os.Stderr
	frontendCmd.Stderr = os.Stderr
	if err := frontendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start frontend: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { frontendCmd.Process.Kill(); frontendCmd.Wait(); os.Remove(socketPath) })

	if err := waitForUnixSocket(socketPath, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "frontend not ready: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	fmt.Printf("[sim] frontend is ready on %s\n", socketPath)

	// Create Docker client
	dockerClient, err = client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
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

func TestECSContainerLifecycle(t *testing.T) {
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
	}, nil, nil, nil, "ecs-lifecycle-test")
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
	}, nil, nil, nil, "ecs-logs-test")
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
	}, nil, nil, nil, "ecs-exec-test")
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
	}, nil, nil, nil, "ecs-list-test")
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
	ctx := context.Background()

	// Create network
	netResp, err := dockerClient.NetworkCreate(ctx, "ecs-test-net", network.CreateOptions{
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
	if netInfo.Name != "ecs-test-net" {
		t.Errorf("expected name 'ecs-test-net', got %q", netInfo.Name)
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

func TestECSVolumeOperations(t *testing.T) {
	ctx := context.Background()

	// Create volume
	vol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: "ecs-test-vol"})
	if err != nil {
		t.Fatalf("volume create failed: %v", err)
	}
	defer dockerClient.VolumeRemove(ctx, vol.Name, true)

	// Inspect
	volInfo, err := dockerClient.VolumeInspect(ctx, vol.Name)
	if err != nil {
		t.Fatalf("volume inspect failed: %v", err)
	}
	if volInfo.Name != "ecs-test-vol" {
		t.Errorf("expected name 'ecs-test-vol', got %q", volInfo.Name)
	}

	// List
	volList, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("volume list failed: %v", err)
	}
	found := false
	for _, v := range volList.Volumes {
		if v.Name == "ecs-test-vol" {
			found = true
			break
		}
	}
	if !found {
		t.Error("volume not found in list")
	}
}

// --- helpers ---

func findModuleDir(rel string) string {
	// We're in backends/ecs, repo root is ../..
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

func waitForUnixSocket(socketPath string, timeout time.Duration) error {
	c := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := c.Get("http://localhost/_ping")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for socket %s", socketPath)
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
