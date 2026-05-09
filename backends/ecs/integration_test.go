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
var evalImageName string

// requireEnv reads a required env var or dies loud.
func requireEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "ERROR: required env var %s is not set.\n", name)
		fmt.Fprintln(os.Stderr, "       The integration test harness has no fallbacks — every config option is mandatory.")
		fmt.Fprintln(os.Stderr, "       Use `make test-integration` from this directory; it sets up the sim target.")
		os.Exit(1)
	}
	return v
}

func requireExe(name string) {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: required tool %q not found on PATH (%v).\n", name, err)
		os.Exit(1)
	}
}

// TestMain wires the docker SDK to a running sockerless-backend-ecs
// pointed at a SOCKERLESS_TEST_TARGET-selected endpoint. There is no
// implicit default and no skip — every config option is mandatory and
// every required prereq must be present, otherwise the harness exits
// non-zero with an explanatory message.
//
// SOCKERLESS_TEST_TARGET = sim   → harness builds + starts simulator-aws on a
//
//	free port, creates the fixed sim ECS
//	cluster, and runs the backend against it.
//	Cluster + subnet + execution role + CPU
//	arch are sim fixtures.
//
// SOCKERLESS_TEST_TARGET = cloud → harness reads explicit env vars
//
//	(SOCKERLESS_ENDPOINT_URL,
//	SOCKERLESS_ECS_CLUSTER,
//	SOCKERLESS_ECS_SUBNETS,
//	SOCKERLESS_ECS_EXECUTION_ROLE_ARN,
//	SOCKERLESS_ECS_CPU_ARCHITECTURE) and fails
//	loud on any missing.
//
// The Test* functions don't know which target they're running against.
func TestMain(m *testing.M) {
	target := requireEnv("SOCKERLESS_TEST_TARGET")
	if target != "sim" && target != "cloud" {
		fmt.Fprintf(os.Stderr, "ERROR: SOCKERLESS_TEST_TARGET=%q is invalid (want \"sim\" or \"cloud\").\n", target)
		os.Exit(1)
	}
	requireExe("docker")
	requireExe("go")

	repoRoot := findModuleDir(".")
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	failClean := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format, args...)
		cleanup()
		os.Exit(1)
	}

	evalDir := repoRoot + "/simulators/testdata/eval-arithmetic"
	evalImageName = "sockerless-eval-arithmetic:test"
	fmt.Printf("[setup] Building %s (linux/arm64)...\n", evalImageName)
	evalDockerfile := `FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /eval-arithmetic .
FROM alpine:latest
COPY --from=build /eval-arithmetic /usr/local/bin/eval-arithmetic
ENTRYPOINT ["/usr/local/bin/eval-arithmetic"]
`
	evalImageBuild := exec.Command("docker", "build",
		"--platform", "linux/arm64",
		"-t", evalImageName, "-f", "-", evalDir)
	evalImageBuild.Stdin = strings.NewReader(evalDockerfile)
	if out, err := evalImageBuild.CombinedOutput(); err != nil {
		failClean("ERROR: docker build eval-arithmetic image: %v\n%s", err, out)
	}

	var endpointURL, cluster, subnets, executionRoleARN, cpuArch string
	switch target {
	case "sim":
		simDir := repoRoot + "/simulators/aws"
		simBinary := simDir + "/simulator-aws"
		fmt.Println("[sim] Building simulator-aws...")
		build := exec.Command("go", "build", "-tags", "noui", "-o", "simulator-aws", ".")
		build.Dir = simDir
		build.Env = filterBuildEnv(os.Environ(), "GOWORK=off")
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			failClean("ERROR: build simulator-aws: %v\n", err)
		}
		cleanups = append(cleanups, func() { os.Remove(simBinary) })

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
			failClean("ERROR: start simulator-aws: %v\n", err)
		}
		cleanups = append(cleanups, func() { simCmd.Process.Kill(); simCmd.Wait() })

		if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
			failClean("ERROR: simulator-aws not ready: %v\n", err)
		}
		fmt.Printf("[sim] simulator-aws ready at %s\n", simURL)

		endpointURL = simURL
		cluster = "sim-cluster"
		subnets = "subnet-0123456789abcdef0"
		executionRoleARN = "arn:aws:iam::000000000000:role/sim"
		cpuArch = "ARM64"

		// Create ECS cluster in simulator (sim fixture).
		body := fmt.Sprintf(`{"clusterName":"%s"}`, cluster)
		req, _ := http.NewRequest("POST", simURL+"/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "AmazonEC2ContainerServiceV20141113.CreateCluster")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			failClean("ERROR: create sim ECS cluster: %v\n", err)
		}
		resp.Body.Close()
		fmt.Printf("[sim] Created ECS cluster %q\n", cluster)

	case "cloud":
		endpointURL = requireEnv("SOCKERLESS_ENDPOINT_URL")
		cluster = requireEnv("SOCKERLESS_ECS_CLUSTER")
		subnets = requireEnv("SOCKERLESS_ECS_SUBNETS")
		executionRoleARN = requireEnv("SOCKERLESS_ECS_EXECUTION_ROLE_ARN")
		cpuArch = requireEnv("SOCKERLESS_ECS_CPU_ARCHITECTURE")
	}

	backendDir := repoRoot + "/backends/ecs"
	backendBinary := backendDir + "/sockerless-backend-ecs"
	fmt.Println("[backend] Building sockerless-backend-ecs...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-ecs", "./cmd/sockerless-backend-ecs")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		failClean("ERROR: build sockerless-backend-ecs: %v\n", err)
	}
	cleanups = append(cleanups, func() { os.Remove(backendBinary) })

	backendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	fmt.Printf("[backend] Starting sockerless-backend-ecs on %s (target=%s endpoint=%s)\n", backendAddr, target, endpointURL)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+endpointURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_ECS_CLUSTER="+cluster,
		"SOCKERLESS_ECS_SUBNETS="+subnets,
		"SOCKERLESS_ECS_EXECUTION_ROLE_ARN="+executionRoleARN,
		"SOCKERLESS_ECS_CPU_ARCHITECTURE="+cpuArch,
	)
	backendCmd.Stdout = os.Stderr
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		failClean("ERROR: start sockerless-backend-ecs: %v\n", err)
	}
	cleanups = append(cleanups, func() { backendCmd.Process.Kill(); backendCmd.Wait() })

	backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
	if err := waitForReady(backendURL, 15*time.Second); err != nil {
		failClean("ERROR: sockerless-backend-ecs not ready: %v\n", err)
	}
	fmt.Printf("[backend] ready on %s\n", backendAddr)

	var err error
	dockerClient, err = client.NewClientWithOpts(
		client.WithHost(fmt.Sprintf("tcp://localhost:%d", backendPort)),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		failClean("ERROR: docker client: %v\n", err)
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

// TestECSVolumeOperations exercises the EFS-backed named volume path:
// VolumeCreate provisions a sockerless-owned EFS access
// point, VolumeInspect and VolumeList surface it, and VolumeRemove
// deletes it. The simulator's EFS slice backs each access point with
// a host-side directory so tasks bind-mount a real path.
func TestECSVolumeOperations(t *testing.T) {
	ctx := context.Background()

	volName := "ecs-test-vol-" + generateTestID()
	vol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: volName})
	if err != nil {
		t.Fatalf("VolumeCreate failed: %v", err)
	}
	if vol.Name != volName {
		t.Errorf("Volume.Name: got %q, want %q", vol.Name, volName)
	}
	if vol.Driver != "efs" {
		t.Errorf("Volume.Driver: got %q, want efs", vol.Driver)
	}
	if vol.Options["accessPointId"] == "" {
		t.Errorf("Volume.Options missing accessPointId: %+v", vol.Options)
	}

	inspected, err := dockerClient.VolumeInspect(ctx, volName)
	if err != nil {
		t.Fatalf("VolumeInspect failed: %v", err)
	}
	if inspected.Name != volName {
		t.Errorf("inspect Name: got %q, want %q", inspected.Name, volName)
	}

	listed, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("VolumeList failed: %v", err)
	}
	found := false
	for _, v := range listed.Volumes {
		if v.Name == volName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("VolumeList did not include %q; got %d volumes", volName, len(listed.Volumes))
	}

	if err := dockerClient.VolumeRemove(ctx, volName, false); err != nil {
		t.Fatalf("VolumeRemove failed: %v", err)
	}

	if _, err := dockerClient.VolumeInspect(ctx, volName); err == nil {
		t.Errorf("expected VolumeInspect to 404 after remove, got success")
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
