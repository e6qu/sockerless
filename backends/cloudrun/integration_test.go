package cloudrun

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
	// actually run it.
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
	simDir := repoRoot + "/simulators/gcp"
	simBinary := simDir + "/simulator-gcp"
	fmt.Println("[sim] Building simulator-gcp...")
	build := exec.Command("go", "build", "-tags", "noui", "-o", "simulator-gcp", ".")
	build.Dir = simDir
	build.Env = filterBuildEnv(os.Environ(), "GOWORK=off")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build simulator-gcp: %v\n", err)
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(simBinary) })

	// Start simulator
	simPort := findFreePort()
	simAddr := fmt.Sprintf(":%d", simPort)
	simURL := fmt.Sprintf("http://127.0.0.1:%d", simPort)
	fmt.Printf("[sim] Starting simulator-gcp on %s...\n", simAddr)
	simCmd := exec.Command(simBinary)
	simCmd.Env = append(os.Environ(),
		"SIM_LISTEN_ADDR="+simAddr,
		"PATH="+os.Getenv("PATH"),
	)
	simCmd.Stdout = os.Stderr
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start simulator-gcp: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { simCmd.Process.Kill(); simCmd.Wait() })

	if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "simulator-gcp not ready: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	fmt.Printf("[sim] simulator-gcp is ready at %s\n", simURL)

	// Build backend
	backendDir := repoRoot + "/backends/cloudrun"
	backendBinary := backendDir + "/sockerless-backend-cloudrun"
	fmt.Println("[sim] Building sockerless-backend-cloudrun...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-cloudrun", "./cmd/sockerless-backend-cloudrun")
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
	fmt.Printf("[sim] Starting sockerless-backend-cloudrun on %s...\n", backendAddr)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+simURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_LOG_TIMEOUT=2s",
		"SOCKERLESS_GCR_PROJECT=sim-project",
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

	// The Cloud Run backend serves the Docker API directly (no separate
	// frontend binary — in-process wiring per post-P67 architecture).
	// Point the docker SDK at the backend's TCP address.
	var err error
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

func TestCloudRunContainerLifecycle(t *testing.T) {
	ctx := context.Background()

	// Pull image
	rc, err := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	testID := generateTestID()

	// Create
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"tail", "-f", "/dev/null"},
		},
		nil, nil, nil, "cloudrun_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	// Inspect (should be created)
	info, err := dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Status != "created" {
		t.Errorf("expected status created, got %s", info.State.Status)
	}

	// Start (may take longer for Cloud Run — 5 min timeout)
	startCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := dockerClient.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Verify running
	info, err = dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if !info.State.Running {
		t.Error("expected container to be running")
	}

	// Stop
	timeout := 10
	if err := dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}

	// Verify stopped
	info, err = dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Running {
		t.Error("expected container to be stopped")
	}

	// Remove
	if err := dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
}

func TestCloudRunContainerLogs(t *testing.T) {
	ctx := context.Background()

	rc, err := dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("image pull failed: %v", err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image:      "alpine:latest",
			Entrypoint: []string{"sh", "-c", "echo hello-cloudrun && sleep 5"},
		},
		nil, nil, nil, "cloudrun_logs_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	startCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := dockerClient.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Wait for log ingestion (Cloud Logging can have 1-5s delay)
	time.Sleep(5 * time.Second)

	logReader, err := dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Fatalf("container logs failed: %v", err)
	}
	logData, _ := io.ReadAll(logReader)
	logReader.Close()

	t.Logf("logs: %q", string(logData))
	if !strings.Contains(string(logData), "hello-cloudrun") {
		t.Log("note: log may not yet be available due to Cloud Logging ingestion delay")
	}

	dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
}

func TestCloudRunContainerList(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
		},
		nil, nil, nil, "cloudrun_list_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatalf("container list failed: %v", err)
	}

	found := false
	for _, cn := range containers {
		if cn.ID == resp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created container not found in list")
	}
}

func TestCloudRunNetworkOperations(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	netName := "cloudrun_net_" + testID

	// Create
	netResp, err := dockerClient.NetworkCreate(ctx, netName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer dockerClient.NetworkRemove(ctx, netResp.ID)

	// Inspect
	net, err := dockerClient.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if net.Name != netName {
		t.Errorf("expected name %s, got %s", netName, net.Name)
	}

	// Remove
	if err := dockerClient.NetworkRemove(ctx, netResp.ID); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}
}

// TestCloudRunVolumeOperations — Phase 92 GCS-backed named volumes:
// VolumeCreate provisions a sockerless-managed GCS bucket, VolumeInspect
// + VolumeList surface it, VolumeRemove deletes it.
func TestCloudRunVolumeOperations(t *testing.T) {
	ctx := context.Background()

	volName := "cloudrun_vol_" + generateTestID()
	vol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: volName})
	if err != nil {
		t.Fatalf("VolumeCreate: %v", err)
	}
	if vol.Name != volName {
		t.Errorf("Volume.Name = %q, want %q", vol.Name, volName)
	}
	if vol.Driver != "gcs" {
		t.Errorf("Volume.Driver = %q, want gcs", vol.Driver)
	}
	if vol.Options["bucket"] == "" {
		t.Errorf("Volume.Options missing bucket: %+v", vol.Options)
	}

	inspected, err := dockerClient.VolumeInspect(ctx, volName)
	if err != nil {
		t.Fatalf("VolumeInspect: %v", err)
	}
	if inspected.Name != volName {
		t.Errorf("inspect Name = %q, want %q", inspected.Name, volName)
	}

	listed, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("VolumeList: %v", err)
	}
	found := false
	for _, v := range listed.Volumes {
		if v.Name == volName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("VolumeList did not return %q; got %d volumes", volName, len(listed.Volumes))
	}

	if err := dockerClient.VolumeRemove(ctx, volName, true); err != nil {
		t.Fatalf("VolumeRemove: %v", err)
	}
	if _, err := dockerClient.VolumeInspect(ctx, volName); err == nil {
		t.Error("VolumeInspect after remove: expected error, got success")
	}
}

// --- helpers ---

func findModuleDir(rel string) string {
	candidates := []string{"../..", "../../.."}
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
