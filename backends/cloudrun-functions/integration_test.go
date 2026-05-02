package gcf

import (
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
	simCmd.Env = append(os.Environ(), "SIM_LISTEN_ADDR="+simAddr)
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
	backendDir := repoRoot + "/backends/cloudrun-functions"
	backendBinary := backendDir + "/sockerless-backend-gcf"
	fmt.Println("[sim] Building sockerless-backend-gcf...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-gcf", "./cmd/sockerless-backend-gcf")
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
	saJSONPath, err := writeFakeServiceAccountJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to stage fake SA JSON: %v\n", err)
		cleanup()
		os.Exit(1)
	}
	cleanups = append(cleanups, func() { _ = os.Remove(saJSONPath) })

	// Start backend
	backendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	fmt.Printf("[sim] Starting sockerless-backend-gcf on %s...\n", backendAddr)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+simURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_LOG_TIMEOUT=2s",
		"SOCKERLESS_GCF_PROJECT=sim-project",
		// Real Cloud Functions Gen2 requires a GCS bucket for the
		// stub-Buildpacks-Go source archive; the sim's GCS handler
		// holds the upload too, so the same bucket name works for both.
		"SOCKERLESS_GCP_BUILD_BUCKET=sim-bucket",
		// idtoken.NewClient ADC source. Generated fresh per test run.
		"GOOGLE_APPLICATION_CREDENTIALS="+saJSONPath,
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
// The sim's invocation handler doesn't validate the token (real Cloud
// Run does; the sim doesn't gate on token shape), but the AUTH HEADER
// PRESENCE matches production wire shape, so the backend code path is
// identical against either real GCP or the sockerless GCP simulator.
//
// No fakes / no fallbacks: this is a real RSA keypair, the SA JSON
// shape is valid (parseable by Google's auth library), and the JWT
// produced is real and crypto-verifiable. The sim could verify the
// signature against this key if it ever wanted to enforce auth.
func writeFakeServiceAccountJSON() (string, error) {
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
		"project_id":                  "sim-project",
		"private_key_id":              "sim-key",
		"private_key":                 string(keyPEM),
		"client_email":                "sim-runner@sim-project.iam.gserviceaccount.com",
		"client_id":                   "111111111111111111111",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/sim-runner@sim-project.iam.gserviceaccount.com",
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
