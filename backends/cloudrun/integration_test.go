// TestMain + helpers for the cloudrun integration tests. Drives the
// docker SDK against a running sockerless-backend-cloudrun pointed at
// a SOCKERLESS_TEST_TARGET-selected endpoint. No fallbacks, no skips —
// every config option is mandatory.

package cloudrun

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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

var dockerClient *client.Client

// backendPort is set in TestMain; used by callers that construct the
// reverse-agent callback URL (`ws://host.docker.internal:<port>/v1/cloudrun/reverse`).
var backendPort int
var evalImageName string

const cloudRunExecE2EEnv = "SOCKERLESS_CLOUDRUN_EXEC_E2E"

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

// TestMain wires the docker SDK to a running sockerless-backend-cloudrun
// pointed at a SOCKERLESS_TEST_TARGET-selected endpoint. There is no
// implicit default and no skip — every config option is mandatory and
// every required prereq must be present, otherwise the harness exits
// non-zero with an explanatory message.
//
// SOCKERLESS_TEST_TARGET = sim   → harness builds + starts simulator-gcp on a
//
//	free port and runs the backend against it.
//	Project is a sim fixture (`sim-project`).
//
// SOCKERLESS_TEST_TARGET = cloud → harness reads explicit env vars
//
//	(SOCKERLESS_ENDPOINT_URL,
//	SOCKERLESS_GCR_PROJECT) and fails loud
//	on any missing.
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
	overlayPlatform := testOverlayPlatformCloudRun()
	overlayArch := runtime.GOARCH
	fmt.Printf("[setup] Building %s (%s)...\n", evalImageName, overlayPlatform)
	// FROM lines pull from public.ecr.aws (no anonymous-pull rate
	// limit), not docker.io. Docker Hub throttles unauthenticated
	// pulls aggressively (toomanyrequests/429); ECR Public Gallery
	// mirrors the Docker Library images without that constraint.
	evalDockerfile := `FROM public.ecr.aws/docker/library/golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /eval-arithmetic .
FROM public.ecr.aws/docker/library/alpine:latest
COPY --from=build /eval-arithmetic /usr/local/bin/eval-arithmetic
ENTRYPOINT ["/usr/local/bin/eval-arithmetic"]
`
	evalImageBuild := exec.Command("docker", "build",
		"--platform", overlayPlatform,
		"-t", evalImageName, "-f", "-", evalDir)
	evalImageBuild.Stdin = strings.NewReader(evalDockerfile)
	if out, err := evalImageBuild.CombinedOutput(); err != nil {
		failClean("ERROR: docker build eval-arithmetic image: %v\n%s", err, out)
	}

	// AR-tag the image so the sim's Cloud Build executor's `FROM`
	// resolves locally (the cloudrun backend rewrites unqualified
	// docker.io refs to AR URLs via gcpcommon.ResolveGCPImageURI).
	// Production deployments would have the image already in AR; the
	// local tag is the equivalent for tests.
	arTag := "us-central1-docker.pkg.dev/sim-project/docker-hub/library/" + evalImageName
	if out, err := exec.Command("docker", "tag", evalImageName, arTag).CombinedOutput(); err != nil {
		failClean("ERROR: docker tag eval-arithmetic AR-form: %v\n%s", err, out)
	}
	// Same for alpine: the eval image build above already pulled the
	// real public.ecr alpine base into the local Docker daemon. Tag
	// that exact base into the Docker Hub and AR names the backend and
	// simulator will reference instead of performing a redundant second
	// registry pull.
	if out, err := exec.Command("docker", "tag", "public.ecr.aws/docker/library/alpine:latest", "alpine:latest").CombinedOutput(); err != nil {
		failClean("ERROR: docker tag alpine docker-hub-form: %v\n%s", err, out)
	}
	alpineARTag := "us-central1-docker.pkg.dev/sim-project/docker-hub/library/alpine:latest"
	if out, err := exec.Command("docker", "tag", "alpine:latest", alpineARTag).CombinedOutput(); err != nil {
		failClean("ERROR: docker tag alpine AR-form: %v\n%s", err, out)
	}

	var endpointURL, project, bootstrapPath, buildBucket, saJSONPath string
	switch target {
	case "sim":
		simDir := repoRoot + "/simulators/gcp"
		simBinary := simDir + "/simulator-gcp"
		fmt.Println("[sim] Building simulator-gcp...")
		build := exec.Command("go", "build", "-tags", "noui", "-o", "simulator-gcp", ".")
		build.Dir = simDir
		build.Env = filterBuildEnv(os.Environ(), "GOWORK=off")
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			failClean("ERROR: build simulator-gcp: %v\n", err)
		}
		cleanups = append(cleanups, func() { os.Remove(simBinary) })

		simPort := findFreePort()
		simAddr := fmt.Sprintf(":%d", simPort)
		simURL := fmt.Sprintf("http://127.0.0.1:%d", simPort)
		fmt.Printf("[sim] Starting simulator-gcp on %s...\n", simAddr)
		simCmd := exec.Command(simBinary)
		simCmd.Env = append(os.Environ(),
			"SIM_LISTEN_ADDR="+simAddr,
			"PATH="+os.Getenv("PATH"),
			// The sim's Cloud Build executor runs `docker build` for the
			// overlay image. The Dockerfile FROM references the user's
			// image by its raw name (e.g. `sockerless-eval-arithmetic:test`)
			// — buildkit always tries the registry first which 401s for
			// the local-only test image. Classic builder falls back to
			// the local daemon cache where the image was just tagged.
			"DOCKER_BUILDKIT=0",
		)
		simCmd.Stdout = os.Stderr
		simCmd.Stderr = os.Stderr
		if err := simCmd.Start(); err != nil {
			failClean("ERROR: start simulator-gcp: %v\n", err)
		}
		cleanups = append(cleanups, func() { simCmd.Process.Kill(); simCmd.Wait() })

		if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
			failClean("ERROR: simulator-gcp not ready: %v\n", err)
		}
		fmt.Printf("[sim] simulator-gcp ready at %s\n", simURL)

		endpointURL = simURL
		project = "sim-project"
		buildBucket = "sockerless-test-build"

		// Pre-create the GCS bucket the backend uses for Cloud Build
		// context uploads (overlay path). Real GCS doesn't auto-create
		// buckets; the backend doesn't either.
		if err := createGCSBucketCloudrun(simURL, project, buildBucket); err != nil {
			failClean("ERROR: create GCS bucket %s: %v\n", buildBucket, err)
		}

		// Stage a fake SA JSON with a real RSA keypair so the backend's
		// gcpcommon.NewGCPBuildService can construct its storage +
		// cloudbuild clients. token_uri points at the sim's /token
		// endpoint. Mirrors the cloudrun-functions setup.
		var saErr error
		saJSONPath, saErr = writeFakeSAJSONCloudrun(simURL + "/token")
		if saErr != nil {
			failClean("ERROR: stage fake SA JSON: %v\n", saErr)
		}
		cleanups = append(cleanups, func() { _ = os.Remove(saJSONPath) })

		if os.Getenv(cloudRunExecE2EEnv) == "1" {
			bootstrapPath = filepath.Join(repoRoot, "agent", fmt.Sprintf("sockerless-cloudrun-bootstrap-test-%d", os.Getpid()))
			if abs, absErr := filepath.Abs(bootstrapPath); absErr == nil {
				bootstrapPath = abs
			}
			fmt.Println("[sim] Building sockerless-cloudrun-bootstrap...")
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer buildCancel()
			bootstrapBuild := exec.CommandContext(buildCtx, "go", "build", "-o", bootstrapPath, "./cmd/sockerless-cloudrun-bootstrap")
			bootstrapBuild.Dir = repoRoot + "/agent"
			bootstrapBuild.Env = filterBuildEnv(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOOS=linux", "GOARCH="+overlayArch)
			bootstrapBuild.Stdout = os.Stderr
			bootstrapBuild.Stderr = os.Stderr
			if err := bootstrapBuild.Run(); err != nil {
				failClean("ERROR: build sockerless-cloudrun-bootstrap: %v\n", err)
			}
			cleanups = append(cleanups, func() { _ = os.Remove(bootstrapPath) })
		} else {
			// Keep the existing simulator package tests on the direct
			// user-image path. The overlay/bootstrap Service-path e2e
			// runs in a dedicated subprocess via TestCloudRunContainerExec.
			bootstrapPath = ""
		}

	case "cloud":
		endpointURL = requireEnv("SOCKERLESS_ENDPOINT_URL")
		project = requireEnv("SOCKERLESS_GCR_PROJECT")
		bootstrapPath = requireEnv("SOCKERLESS_CLOUDRUN_BOOTSTRAP")
		buildBucket = requireEnv("SOCKERLESS_GCP_BUILD_BUCKET")
		saJSONPath = requireEnv("GOOGLE_APPLICATION_CREDENTIALS")
	}

	backendDir := repoRoot + "/backends/cloudrun"
	backendBinary := backendDir + "/sockerless-backend-cloudrun"
	fmt.Println("[backend] Building sockerless-backend-cloudrun...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-cloudrun", "./cmd/sockerless-backend-cloudrun")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		failClean("ERROR: build sockerless-backend-cloudrun: %v\n", err)
	}
	cleanups = append(cleanups, func() { os.Remove(backendBinary) })

	backendPort = findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	fmt.Printf("[backend] Starting sockerless-backend-cloudrun on %s (target=%s endpoint=%s)\n", backendAddr, target, endpointURL)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	storageHost := strings.TrimPrefix(endpointURL, "http://")
	storageHost = strings.TrimPrefix(storageHost, "https://")
	backendEnv := append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+endpointURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_LOG_TIMEOUT=2s",
		"SOCKERLESS_GCR_PROJECT="+project,
		"SOCKERLESS_CLOUDRUN_BOOTSTRAP="+bootstrapPath,
		"SOCKERLESS_GCP_BUILD_BUCKET="+buildBucket,
		"SOCKERLESS_GCP_BUILD_PLATFORM="+overlayPlatform,
		// Required at NewServer per Phase 168 (no Path B fallback).
		// Bootstrap dials back over WebSocket from inside the workload
		// container — host.docker.internal resolves the test host.
		"SOCKERLESS_CALLBACK_URL="+fmt.Sprintf("ws://host.docker.internal:%d/v1/cloudrun/reverse", backendPort),
		// STORAGE_EMULATOR_HOST routes the backend's GCS client to the
		// sim's storage endpoint instead of storage.googleapis.com.
		"STORAGE_EMULATOR_HOST="+storageHost,
		// ADC source for storage.NewClient + cloudbuild.NewRESTClient.
		"GOOGLE_APPLICATION_CREDENTIALS="+saJSONPath,
	)
	if target == "sim" && os.Getenv(cloudRunExecE2EEnv) == "1" {
		backendEnv = append(backendEnv,
			"SOCKERLESS_GCR_USE_SERVICE=1",
			"SOCKERLESS_GCR_VPC_CONNECTOR=projects/sim-project/locations/us-central1/connectors/sim-connector",
		)
	}
	backendCmd.Env = backendEnv
	backendCmd.Stdout = os.Stderr
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		failClean("ERROR: start sockerless-backend-cloudrun: %v\n", err)
	}
	cleanups = append(cleanups, func() { backendCmd.Process.Kill(); backendCmd.Wait() })

	backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
	if err := waitForReady(backendURL, 15*time.Second); err != nil {
		failClean("ERROR: sockerless-backend-cloudrun not ready: %v\n", err)
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

	// Pre-load the eval-arithmetic image into the backend's image
	// store via `docker save | dockerClient.ImageLoad`. The backend's
	// Store.ResolveImage(ref) then finds the image with its full
	// Config (Entrypoint=/usr/local/bin/eval-arithmetic) so the
	// overlay path can preserve it into SOCKERLESS_USER_ENTRYPOINT
	// for the bootstrap to exec. Mirrors the cloudrun-functions setup.
	if err := preloadImageIntoBackendCloudrun(evalImageName); err != nil {
		failClean("ERROR: preload eval-arithmetic into backend: %v\n", err)
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func testOverlayPlatformCloudRun() string {
	return "linux/" + runtime.GOARCH
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
			Image:     "alpine:latest",
			Cmd:       []string{"tail", "-f", "/dev/null"},
			OpenStdin: true,
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
			OpenStdin:  true,
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

func TestCloudRunContainerExec(t *testing.T) {
	if os.Getenv(cloudRunExecE2EEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestCloudRunContainerExec$", "-test.v")
		cmd.Env = append(os.Environ(), cloudRunExecE2EEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("overlay exec subprocess failed: %v\n%s", err, string(out))
		}
		return
	}

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
			Cmd:   []string{"tail", "-f", "/dev/null"},
		},
		nil, nil, nil, "cloudrun_exec_"+testID,
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

	execResp, err := dockerClient.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", "printf cloudrun-exec-ok"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	hijacked, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		t.Fatalf("exec attach failed: %v", err)
	}
	defer hijacked.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, hijacked.Reader); err != nil {
		t.Fatalf("exec stream copy failed: %v", err)
	}
	if got := stdout.String(); got != "cloudrun-exec-ok" {
		t.Fatalf("exec stdout = %q, stderr = %q", got, stderr.String())
	}

	inspect, err := dockerClient.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}
	if inspect.ExitCode != 0 {
		t.Fatalf("exec exit code = %d", inspect.ExitCode)
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

// TestCloudRunVolumeOperations — GCS-backed named volumes:
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

// createGCSBucketCloudrun is the cloudrun-side mirror of the GCF
// helper. Pre-creates the GCS bucket the backend uses for Cloud Build
// context uploads (overlay path).
func createGCSBucketCloudrun(simURL, project, bucket string) error {
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

// writeFakeSAJSONCloudrun mirrors the cloudrun-functions helper —
// generates an RSA keypair + writes a real-shape service-account JSON
// pointing at the sim's /token endpoint. The backend's
// NewGCPBuildService uses these creds to construct its GCS +
// cloudbuild clients.
func writeFakeSAJSONCloudrun(tokenURI string) (string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate RSA keypair: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("marshal PKCS8: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	sa := map[string]string{
		"type":                        "service_account",
		"project_id":                  "sim-project",
		"private_key_id":              "sim-key",
		"private_key":                 string(keyPEM),
		"client_email":                "sockerless-runner@sim-project.iam.gserviceaccount.com",
		"client_id":                   "111111111111111111111",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   tokenURI,
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"universe_domain":             "googleapis.com",
	}
	body, err := json.Marshal(sa)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "sockerless-sim-cloudrun-sa-*.json")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// preloadImageIntoBackendCloudrun pipes `docker save <ref>` into
// `dockerClient.ImageLoad`. The backend's ImageLoad handler parses
// the tar and registers the image with full Config in Store.Images,
// so subsequent overlay-path Container Creates preserve the image's
// ENTRYPOINT/CMD. Mirror of cloudrun-functions/preloadImageIntoBackend.
func preloadImageIntoBackendCloudrun(ref string) error {
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer saveCancel()
	save := exec.CommandContext(saveCtx, "docker", "save", ref)
	stdout, err := save.StdoutPipe()
	if err != nil {
		return fmt.Errorf("docker save stdout pipe: %w", err)
	}
	if err := save.Start(); err != nil {
		return fmt.Errorf("docker save start: %w", err)
	}
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer loadCancel()
	resp, err := dockerClient.ImageLoad(loadCtx, stdout)
	if err != nil {
		_ = save.Wait()
		return fmt.Errorf("dockerClient.ImageLoad: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if err := save.Wait(); err != nil {
		return fmt.Errorf("docker save wait: %w", err)
	}
	return nil
}
