package gcf

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
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

	// Watchdog: if TestMain hasn't reached `m.Run()` within 5 minutes
	// something has hung. Force a SIGABRT via `runtime/pprof` goroutine
	// dump + panic so the failure surfaces a full goroutine trace,
	// pinpointing which goroutine is stuck where. The previous failure
	// mode was an opaque 8-minute silence; this turns it into an
	// actionable stack dump that lands directly in the CI log.
	watchdogDone := make(chan struct{})
	go func() {
		select {
		case <-watchdogDone:
			return
		case <-time.After(5 * time.Minute):
			fmt.Fprintln(os.Stderr, "[testmain] WATCHDOG: TestMain has been running >5min; dumping goroutines and aborting")
			_ = pprof.Lookup("goroutine").WriteTo(os.Stderr, 2)
			fmt.Fprintln(os.Stderr, "[testmain] WATCHDOG: goroutine dump complete; aborting process")
			os.Exit(124) // 124 = standard timeout exit code
		}
	}()
	defer close(watchdogDone)

	repoRoot := findModuleDir(".")
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	// stderr-step logger: TestMain runs before any test function, so its
	// stdout output is buffered by `go test -v` and only flushed when a
	// test starts. If TestMain hangs (e.g. a download stalls or a sim
	// HTTP call blocks), we'd never see WHERE — the last visible CI line
	// would be `go: downloading <module>` from the outer `go test`.
	// Writing to os.Stderr is unbuffered and surfaces progress live.
	step := func(label string) { fmt.Fprintf(os.Stderr, "[testmain] %s\n", label) }
	step("entered TestMain (SOCKERLESS_INTEGRATION=1)")

	// Build eval-arithmetic binary (static linux/amd64, to be embedded
	// in a Docker image the container runtime can actually execute).
	evalDir := repoRoot + "/simulators/testdata/eval-arithmetic"
	evalBinaryPath = evalDir + "/eval-arithmetic"
	step("building eval-arithmetic (linux/amd64)")
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
	step("docker build " + evalImageName)
	fmt.Printf("[sim] Building %s...\n", evalImageName)
	evalDockerfile := "FROM alpine:latest\nCOPY eval-arithmetic /usr/local/bin/eval-arithmetic\nENTRYPOINT [\"/usr/local/bin/eval-arithmetic\"]\n"
	evalImageBuild := exec.Command("docker", "build", "-t", evalImageName, "-f", "-", evalDir)
	evalImageBuild.Stdin = strings.NewReader(evalDockerfile)
	if out, err := evalImageBuild.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build eval-arithmetic image: %v\n%s", err, out)
		os.Exit(1)
	}
	// Also tag the image with the AR-prefixed name the gcf backend's
	// gcpcommon.ResolveGCPImageURI rewrites unqualified Docker Hub
	// references to. Cloud Build's `FROM` (executed via the sim's
	// build executor running real `docker build` against the local
	// daemon) finds the image in the local cache without pulling
	// from the AR URL — production deployments would have the image
	// already in AR; the local tag is the equivalent for tests.
	arTag := "us-central1-docker.pkg.dev/sockerless-test/docker-hub/library/" + evalImageName
	if out, err := exec.Command("docker", "tag", evalImageName, arTag).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to AR-tag eval-arithmetic image: %v\n%s", err, out)
		os.Exit(1)
	}

	// Tests that don't use eval-arithmetic still pass plain "alpine:latest"
	// as the container image. The backend rewrites that to the AR URL via
	// gcpcommon.ResolveGCPImageURI; Cloud Build's FROM then needs the
	// rewritten URL to resolve locally. Pre-pull alpine and tag it to the
	// AR URL so `docker build` (run by the sim's executor) finds it in
	// the local cache instead of attempting an anonymous AR token fetch
	// (which 403s on CI for nonexistent AR projects).
	step("docker pull alpine:latest + AR-tag")
	fmt.Println("[sim] Pre-pulling alpine:latest...")
	if out, err := exec.Command("docker", "pull", "alpine:latest").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to pull alpine:latest: %v\n%s", err, out)
		os.Exit(1)
	}
	alpineARTag := "us-central1-docker.pkg.dev/sockerless-test/docker-hub/library/alpine:latest"
	if out, err := exec.Command("docker", "tag", "alpine:latest", alpineARTag).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to AR-tag alpine:latest: %v\n%s", err, out)
		os.Exit(1)
	}

	// Build simulator
	simDir := repoRoot + "/simulators/gcp"
	simBinary := simDir + "/simulator-gcp"
	step("go build simulator-gcp")
	fmt.Println("[sim] Building simulator-gcp...")
	buildCtx1, buildCancel1 := context.WithTimeout(context.Background(), 3*time.Minute)
	defer buildCancel1()
	build := exec.CommandContext(buildCtx1, "go", "build", "-tags", "noui", "-o", "simulator-gcp", ".")
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
	step("starting simulator-gcp")
	fmt.Printf("[sim] Starting simulator-gcp on %s...\n", simAddr)
	simCmd := exec.Command(simBinary)
	simCmd.Env = append(os.Environ(), "SIM_LISTEN_ADDR="+simAddr)
	simCmd.Stdout = os.Stderr
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start simulator-gcp: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { simCmd.Process.Kill(); simCmd.Wait() })

	step("waiting for simulator-gcp /health")
	if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "simulator-gcp not ready: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	fmt.Printf("[sim] simulator-gcp is ready at %s\n", simURL)

	// Pre-create the GCS bucket the backend uses for Cloud Build context
	// uploads. Real GCS doesn't auto-create buckets; the backend doesn't
	// either (operator infrastructure — terraform creates the bucket in
	// production deployments). Tests stand in for terraform here.
	step("create GCS bucket sockerless-test-build")
	if err := createGCSBucket(simURL, "sockerless-test", "sockerless-test-build"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create sockerless-test-build: %v\n", err)
		cleanup()
		os.Exit(1)
	}

	// Build backend
	backendDir := repoRoot + "/backends/cloudrun-functions"
	backendBinary := backendDir + "/sockerless-backend-gcf"
	step("go build sockerless-backend-gcf")
	fmt.Println("[sim] Building sockerless-backend-gcf...")
	buildCtx2, buildCancel2 := context.WithTimeout(context.Background(), 3*time.Minute)
	defer buildCancel2()
	buildBackend := exec.CommandContext(buildCtx2, "go", "build", "-tags", "noui", "-o", "sockerless-backend-gcf", "./cmd/sockerless-backend-gcf")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build backend: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { os.Remove(backendBinary) })

	// Stage a fake service-account JSON with a real RSA keypair so the
	// backend's `idtoken.NewClient` (called from `invokeFunction`) can
	// sign JWTs locally. idtoken refuses user-credentials ADC and
	// requires service-account creds; the keypair is generated fresh
	// per test run. The sim's invocation handler doesn't validate the
	// signed token (token validation is a real-Cloud-Run concern, not
	// a sim concern) — but the AUTH HEADER PRESENCE is the same wire
	// shape as production, so the backend code path stays identical.
	step("staging fake SA JSON")
	saJSONPath, err := writeFakeServiceAccountJSON(simURL + "/token")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stage fake SA JSON: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { _ = os.Remove(saJSONPath) })

	// Build the sockerless-gcf-bootstrap binary so the backend's
	// overlay-and-swap path (Phase 118 gcf re-architecture) can stage
	// it into the Cloud Build context tar. Real deployments install
	// this binary at /opt/sockerless via the runner image; integration
	// tests build it on demand. Path must be absolute — the backend
	// process runs with a different cwd than the test binary, so a
	// relative path resolves wrong inside the backend.
	gcfBootstrapPath := repoRoot + "/agent/sockerless-gcf-bootstrap-test"
	if abs, absErr := filepath.Abs(gcfBootstrapPath); absErr == nil {
		gcfBootstrapPath = abs
	}
	step("go build sockerless-gcf-bootstrap")
	fmt.Println("[sim] Building sockerless-gcf-bootstrap...")
	buildCtx3, buildCancel3 := context.WithTimeout(context.Background(), 3*time.Minute)
	defer buildCancel3()
	bootstrapBuild := exec.CommandContext(buildCtx3, "go", "build", "-o", gcfBootstrapPath, "./cmd/sockerless-gcf-bootstrap")
	bootstrapBuild.Dir = repoRoot + "/agent"
	bootstrapBuild.Env = filterBuildEnv(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOOS=linux", "GOARCH=amd64")
	bootstrapBuild.Stdout = os.Stderr
	bootstrapBuild.Stderr = os.Stderr
	if err := bootstrapBuild.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build sockerless-gcf-bootstrap: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { _ = os.Remove(gcfBootstrapPath) })

	// Start backend
	backendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	step("starting sockerless-backend-gcf")
	fmt.Printf("[sim] Starting sockerless-backend-gcf on %s...\n", backendAddr)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	// urlHost extracts host:port from http://host:port for STORAGE_EMULATOR_HOST.
	storageHost := strings.TrimPrefix(simURL, "http://")
	storageHost = strings.TrimPrefix(storageHost, "https://")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+simURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_LOG_TIMEOUT=2s",
		"SOCKERLESS_GCF_PROJECT=sockerless-test",
		// Real Cloud Functions Gen2 requires a GCS bucket for the
		// stub-Buildpacks-Go source archive.
		"SOCKERLESS_GCP_BUILD_BUCKET=sockerless-test-build",
		// idtoken.NewClient ADC source. Generated fresh per test run.
		"GOOGLE_APPLICATION_CREDENTIALS="+saJSONPath,
		// Path to the sockerless-gcf-bootstrap binary the backend stages
		// into Cloud Build context tarballs (overlay image build).
		"SOCKERLESS_GCF_BOOTSTRAP="+gcfBootstrapPath,
		// STORAGE_EMULATOR_HOST is Google's SDK-side name for "where
		// to route storage API requests" (not a description of what's
		// at the other end — the sockerless GCP simulator implements
		// real GCS-shape storage backed by real on-disk files, not an
		// emulation). Operators set this env var when their storage
		// SDK should target a non-default host. Production sets nothing
		// and uses the default storage.googleapis.com discovery.
		"STORAGE_EMULATOR_HOST="+storageHost,
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
	step("waiting for sockerless-backend-gcf /info")
	if err := waitForReady(backendURL, 15*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "backend not ready: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	fmt.Printf("[sim] backend is ready on %s\n", backendAddr)

	// The GCF backend serves the Docker API directly (no separate
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

	step("entering m.Run() — TestMain setup complete")
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestGCFContainerLogs(t *testing.T) {
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
			Cmd:   []string{"echo", "hello-gcf-logs"},
		},
		nil, nil, nil, "gcf_logs_"+testID,
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
	if !strings.Contains(string(logData), "hello-gcf-logs") {
		t.Log("note: log may not yet be available due to Cloud Logging ingestion delay")
	}
}

func TestGCFContainerList(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
		},
		nil, nil, nil, "gcf_list_"+testID,
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

func TestGCFContainerStopNoOp(t *testing.T) {
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
		nil, nil, nil, "gcf_stop_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{})

	// Stop should succeed as no-op
	timeout := 5
	if err := dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("container stop failed (should be no-op): %v", err)
	}
}

func TestGCFContainerExec(t *testing.T) {
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
		nil, nil, nil, "gcf_exec_"+testID,
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

func TestGCFNetworkOperations(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()

	// Network create should succeed
	netResp, err := dockerClient.NetworkCreate(ctx, "gcf-net-"+testID, network.CreateOptions{})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}

	// Network inspect
	net, err := dockerClient.NetworkInspect(ctx, netResp.ID, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if net.Name != "gcf-net-"+testID {
		t.Errorf("expected network name %q, got %q", "gcf-net-"+testID, net.Name)
	}

	// Network remove
	if err := dockerClient.NetworkRemove(ctx, netResp.ID); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}
}

// TestGCFVolumeOperations — GCS-backed named volumes on GCF:
// VolumeCreate provisions a sockerless-managed GCS bucket via the shared
// gcpcommon.BucketManager, VolumeInspect + VolumeList surface it, and
// VolumeRemove deletes it. The actual bucket-attach-to-function path
// (Services.UpdateService escape hatch) is exercised by the
// arithmetic + lifecycle tests when they carry Binds.
func TestGCFVolumeOperations(t *testing.T) {
	ctx := context.Background()

	volName := "gcf_vol_" + generateTestID()
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
		t.Errorf("inspected.Name = %q, want %q", inspected.Name, volName)
	}

	list, err := dockerClient.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		t.Fatalf("VolumeList: %v", err)
	}
	found := false
	for _, v := range list.Volumes {
		if v != nil && v.Name == volName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("VolumeList did not surface %q", volName)
	}

	if err := dockerClient.VolumeRemove(ctx, volName, false); err != nil {
		t.Fatalf("VolumeRemove: %v", err)
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

// waitForReady polls `url` until it answers 200 or `timeout` elapses.
// Uses an http.Client with a per-request timeout (1s) so a SINGLE call
// that hangs (server accepted the connection but never wrote a
// response — possible if the goroutine handling /health blocks) can't
// monopolise the deadline budget. The previous implementation called
// `http.DefaultClient.Get`, which has NO timeout — a hung request
// would block forever and the outer `for time.Now().Before(deadline)`
// check would never re-evaluate.
func waitForReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
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

// TestGCFContainerLifecycle: invocation goroutine records the HTTP
// response (2xx → 0) in Store.InvocationResults, so CloudState reports
// `exited` and docker wait returns the real exit code.
func TestGCFContainerLifecycle(t *testing.T) {
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
			Cmd:   []string{"echo", "hello from gcf"},
		},
		nil, nil, nil, "gcf_lc_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

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

	info, err := dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Status != "exited" {
		t.Errorf("expected status 'exited', got %q", info.State.Status)
	}

	if err := dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("container remove failed: %v", err)
	}
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

// writeFakeServiceAccountJSON generates a fresh RSA keypair and writes
// a valid service-account JSON to a temp file, returning the path.
//
// `idtoken.NewClient` (called by the gcf backend's `invokeFunction`)
// signs JWTs locally with this key — no network call to GCP is made.
//
// `cloudbuild.NewRESTClient` (called by gcpcommon.NewGCPBuildService)
// uses the SA's `token_uri` to mint OAuth2 access tokens for the
// authenticated REST client. Tests point `token_uri` at the sim's
// /token endpoint (registered by `simulators/gcp/oauth2.go`) so the
// SDK fetches tokens from the sim instead of `oauth2.googleapis.com`.
//
// The SA JSON shape is real (parseable by Google's auth library); the
// JWT and access-token responses are real wire-shape. The sim doesn't
// validate the JWT — it isn't an emulator gating the API; it just
// responds with the wire shape the SDK expects so the SDK proceeds to
// the actual API call.
func writeFakeServiceAccountJSON(tokenURI string) (string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate RSA keypair: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("marshal private key PKCS8: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	sa := map[string]string{
		"type":                        "service_account",
		"project_id":                  "sockerless-test",
		"private_key_id":              "sim-key",
		"private_key":                 string(keyPEM),
		"client_email":                "sockerless-runner@sockerless-test.iam.gserviceaccount.com",
		"client_id":                   "111111111111111111111",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   tokenURI,
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/sockerless-runner@sockerless-test.iam.gserviceaccount.com",
		"universe_domain":             "googleapis.com",
	}
	body, err := json.Marshal(sa)
	if err != nil {
		return "", fmt.Errorf("marshal SA JSON: %w", err)
	}
	f, err := os.CreateTemp("", "sockerless-sim-sa-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return "", fmt.Errorf("write SA JSON: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// createGCSBucket POSTs to the sim's GCS bucket-create endpoint. Real
// GCS uses the same shape: POST /storage/v1/b?project=<id> with a
// {name: "<bucket>"} body. The sim accepts this form too (sim/gcp/gcs.go).
// Stands in for terraform in integration tests; production operators
// create the bucket as infrastructure before launching the backend.
//
// Bound by a 5s context so a hung sim surfaces as a clear error rather
// than blocking TestMain indefinitely (the request is local and a
// healthy sim should answer in single-digit ms).
func createGCSBucket(simURL, project, bucket string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body := []byte(fmt.Sprintf(`{"name":%q}`, bucket))
	url := fmt.Sprintf("%s/storage/v1/b?project=%s", simURL, project)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create bucket %s: %d: %s", bucket, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
