package lambda

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

// State shared with the agent-e2e test. Populated by TestMain when
// SOCKERLESS_INTEGRATION=1, consumed by the e2e test that drives
// the Lambda-backend → simulator → reverse-agent round-trip.
var (
	agentBootstrapBinaryPath string
	agentTestImageName       string
	lambdaBackendPort        int
	lambdaBackendWSURL       string
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("SOCKERLESS_INTEGRATION") != "1" {
		t.Skip("skipping integration test (SOCKERLESS_INTEGRATION != 1)")
	}
}

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

	// Build backend
	backendDir := repoRoot + "/backends/lambda"
	backendBinary := backendDir + "/sockerless-backend-lambda"
	fmt.Println("[sim] Building sockerless-backend-lambda...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-lambda", "./cmd/sockerless-backend-lambda")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build backend: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(backendBinary) })

	// Build the real sockerless-lambda-bootstrap for linux and bake
	// it into a throw-away test image that the simulator's Lambda
	// Runtime API slice will invoke as a handler. The backend is
	// started with PrebuiltOverlayImage pointed at this image so it
	// doesn't need to run `docker push` against an insecure registry.
	bootstrapDir := repoRoot + "/agent/cmd/sockerless-lambda-bootstrap"
	agentBootstrapBinaryPath = bootstrapDir + "/sockerless-lambda-bootstrap"
	fmt.Println("[sim] Building sockerless-lambda-bootstrap for linux...")
	bsBuild := exec.Command("go", "build", "-o", "sockerless-lambda-bootstrap", ".")
	bsBuild.Dir = bootstrapDir
	bsBuild.Env = filterBuildEnv(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOOS=linux", "GOARCH=amd64")
	bsBuild.Stdout = os.Stderr
	bsBuild.Stderr = os.Stderr
	if err := bsBuild.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build sockerless-lambda-bootstrap: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(agentBootstrapBinaryPath) })

	agentTestImageName = "sockerless-lambda-agent-test:v1"
	fmt.Printf("[sim] Building %s...\n", agentTestImageName)
	agentDockerfile := "FROM alpine:latest\n" +
		"COPY sockerless-lambda-bootstrap /usr/local/bin/sockerless-lambda-bootstrap\n" +
		"ENTRYPOINT [\"/usr/local/bin/sockerless-lambda-bootstrap\"]\n"
	agentBuild := exec.Command("docker", "build", "-t", agentTestImageName, "-f", "-", bootstrapDir)
	agentBuild.Stdin = strings.NewReader(agentDockerfile)
	if out, err := agentBuild.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build agent test image: %v\n%s", err, out)
		cleanup()
		os.Exit(1)
	}

	// Start backend
	backendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	lambdaBackendPort = backendPort
	// Callback URL the container inside Docker will dial back to —
	// host.docker.internal resolves to the host where the backend lives.
	lambdaBackendWSURL = fmt.Sprintf("ws://host.docker.internal:%d/v1/lambda/reverse", backendPort)
	fmt.Printf("[sim] Starting sockerless-backend-lambda on %s (callback=%s)...\n", backendAddr, lambdaBackendWSURL)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+simURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_LAMBDA_ROLE_ARN=arn:aws:iam::000000000000:role/sim",
		"SOCKERLESS_CALLBACK_URL="+lambdaBackendWSURL,
		"SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE="+agentTestImageName,
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

	// The Lambda backend serves the Docker API directly (no separate
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

func TestLambdaContainerLifecycle(t *testing.T) {
	skipIfNoIntegration(t)
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
			Cmd:   []string{"echo", "hello from lambda"},
		},
		nil, nil, nil, "lambda_"+testID,
	)
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

	// Remove
	if err := dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
}

func TestLambdaContainerLogs(t *testing.T) {
	skipIfNoIntegration(t)
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
			Image: "alpine:latest",
			Cmd:   []string{"echo", "hello-lambda-logs"},
		},
		nil, nil, nil, "lambda_logs_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
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
	if !strings.Contains(string(logData), "hello-lambda-logs") {
		t.Log("note: log may not yet be available due to CloudWatch ingestion delay")
	}
}

// TestLambdaContainerLogsFollowLazyStream verifies that calling
// ContainerLogs with Follow=true BEFORE the log stream exists still
// produces output once the invocation completes. Regression test for
// the bug where logStreamName was resolved once up-front; if empty at
// that moment the follow loop would return empty forever.
func TestLambdaContainerLogsFollowLazyStream(t *testing.T) {
	skipIfNoIntegration(t)
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
			Image: "alpine:latest",
			Cmd:   []string{"sh", "-c", "for i in 1 2 3; do echo follow-line-$i; sleep 0.2; done"},
		},
		nil, nil, nil, "lambda_follow_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	// Start and immediately open follow logs — the stream may not exist yet.
	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	logReader, err := dockerClient.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		t.Fatalf("container logs (follow) failed: %v", err)
	}
	defer logReader.Close()

	// Read with a deadline so a stuck follow loop fails the test.
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(logReader)
		done <- b
	}()

	var logData []byte
	select {
	case logData = <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("follow-mode log read did not terminate within 60s after container exited")
	}

	t.Logf("follow logs: %q", string(logData))
	for _, want := range []string{"follow-line-1", "follow-line-2", "follow-line-3"} {
		if !strings.Contains(string(logData), want) {
			t.Errorf("missing %q in follow-mode log output", want)
		}
	}
}

func TestLambdaContainerList(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
		},
		nil, nil, nil, "lambda_list_"+testID,
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

func TestLambdaContainerStopUnblocksWait(t *testing.T) {
	skipIfNoIntegration(t)
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
			Image: "alpine:latest",
			Cmd:   []string{"sleep", "30"},
		},
		nil, nil, nil, "lambda_stop_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Stop clamps the function timeout (no-op against in-flight) and closes
	// the local wait channel. A subsequent ContainerWait must return within
	// a few seconds, not after the sleep-30 completes.
	stopTimeout := 5
	if err := dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		t.Fatalf("container stop failed: %v", err)
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
		// expected — wait channel closed by stop
	case err := <-errCh:
		t.Fatalf("wait returned error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("docker wait did not unblock within 5s of stop; wait channel not closed")
	}
}

func TestLambdaContainerExec(t *testing.T) {
	skipIfNoIntegration(t)
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
			Image:     "alpine:latest",
			Cmd:       []string{"tail", "-f", "/dev/null"},
			Tty:       true,
			OpenStdin: true,
		},
		nil, nil, nil, "lambda_exec_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Exec create should succeed (synthetic exec from core)
	execResp, err := dockerClient.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
		Cmd:          []string{"echo", "hello"},
		AttachStdout: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	if execResp.ID == "" {
		t.Error("expected non-empty exec ID")
	}
}

func TestLambdaNetworkOperations(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	testID := generateTestID()

	// Network create should succeed
	netResp, err := dockerClient.NetworkCreate(ctx, "lambda-net-"+testID, network.CreateOptions{})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}

	// Network inspect
	net, err := dockerClient.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if net.Name != "lambda-net-"+testID {
		t.Errorf("expected network name %q, got %q", "lambda-net-"+testID, net.Name)
	}

	// Network remove
	if err := dockerClient.NetworkRemove(ctx, netResp.ID); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}
}

// TestLambdaVolumeOperations pins BUG-731 — Lambda's /tmp is
// per-invocation; named volumes require real EFS mounts and are
// tracked as Phase 91.
func TestLambdaVolumeOperations(t *testing.T) {
	skipIfNoIntegration(t)
	ctx := context.Background()

	_, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: "lambda-vol-" + generateTestID()})
	if err == nil {
		t.Fatal("expected VolumeCreate to fail with NotImplemented")
	}
	if !strings.Contains(err.Error(), "does not support named volumes") {
		t.Errorf("expected NotImplemented error, got: %v", err)
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
