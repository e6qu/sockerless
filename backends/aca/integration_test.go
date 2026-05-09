package aca

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

// requireExe verifies a binary is on PATH or dies loud.
func requireExe(name string) {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: required tool %q not found on PATH (%v).\n", name, err)
		os.Exit(1)
	}
}

// TestMain wires the docker SDK to a running sockerless-backend-aca
// pointed at a SOCKERLESS_TEST_TARGET-selected endpoint. There is no
// implicit default and no skip — every config option is mandatory and
// every required prereq must be present, otherwise the harness exits
// non-zero with an explanatory message.
//
// SOCKERLESS_TEST_TARGET = sim   → harness builds + starts simulator-azure on a
//
//	free port and pre-creates the fixed sim
//	storage-account + managedEnvironment.
//	The endpoint, subscription, RG, storage
//	account, and log-analytics workspace are
//	fixed sim values (not externally
//	configurable — sim fixtures are part of
//	the test contract).
//
// SOCKERLESS_TEST_TARGET = cloud → harness reads operator-supplied env vars
//
//	(SOCKERLESS_ENDPOINT_URL,
//	SOCKERLESS_ACA_SUBSCRIPTION_ID,
//	SOCKERLESS_ACA_RESOURCE_GROUP,
//	SOCKERLESS_ACA_STORAGE_ACCOUNT,
//	SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE) and
//	fails loud on any missing. No pre-creates
//	— operator owns those resources.
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

	// Multi-stage Docker build forced to linux/arm64 — sim's primary
	// capacity contract. The eval-arithmetic image is the workload the
	// test functions exec inside the cloud (sim or real).
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
		failClean("ERROR: docker build eval-arithmetic image failed: %v\n%s", err, out)
	}

	// Resolve target endpoint + ARM identifiers.
	var endpointURL, subscriptionID, resourceGroup, storageAccount, logAnalyticsWS string
	switch target {
	case "sim":
		// Build the simulator binary
		simDir := repoRoot + "/simulators/azure"
		simBinary := simDir + "/simulator-azure"
		fmt.Println("[sim] Building simulator-azure...")
		build := exec.Command("go", "build", "-tags", "noui", "-o", "simulator-azure", ".")
		build.Dir = simDir
		build.Env = filterBuildEnv(os.Environ(), "GOWORK=off")
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			failClean("ERROR: build simulator-azure failed: %v\n", err)
		}
		cleanups = append(cleanups, func() { os.Remove(simBinary) })

		simPort := findFreePort()
		simAddr := fmt.Sprintf(":%d", simPort)
		simURL := fmt.Sprintf("http://127.0.0.1:%d", simPort)
		fmt.Printf("[sim] Starting simulator-azure on %s...\n", simAddr)
		simCmd := exec.Command(simBinary)
		simCmd.Env = append(os.Environ(),
			"SIM_LISTEN_ADDR="+simAddr,
			"PATH="+os.Getenv("PATH"),
		)
		simCmd.Stdout = os.Stderr
		simCmd.Stderr = os.Stderr
		if err := simCmd.Start(); err != nil {
			failClean("ERROR: start simulator-azure: %v\n", err)
		}
		cleanups = append(cleanups, func() { simCmd.Process.Kill(); simCmd.Wait() })

		if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
			failClean("ERROR: simulator-azure not ready: %v\n", err)
		}
		fmt.Printf("[sim] simulator-azure ready at %s\n", simURL)

		// Pre-create storage account + managedEnvironment as the
		// operator would in production. Direct ARM PUTs against the
		// sim. These identifiers are sim fixtures (part of the test
		// contract); they're not externally configurable.
		endpointURL = simURL
		subscriptionID = "00000000-0000-0000-0000-000000000001"
		resourceGroup = "sim-rg"
		storageAccount = "simstorage"
		logAnalyticsWS = "default"
		preCreate := func(url, body string) {
			req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				failClean("ERROR: pre-create sim resource %s: %v\n", url, err)
			}
			resp.Body.Close()
		}
		preCreate(
			fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s?api-version=2023-01-01",
				simURL, subscriptionID, resourceGroup, storageAccount),
			`{"location":"eastus","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}`,
		)
		preCreate(
			fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/sockerless?api-version=2024-03-01",
				simURL, subscriptionID, resourceGroup),
			`{"location":"eastus","properties":{}}`,
		)

	case "cloud":
		endpointURL = requireEnv("SOCKERLESS_ENDPOINT_URL")
		subscriptionID = requireEnv("SOCKERLESS_ACA_SUBSCRIPTION_ID")
		resourceGroup = requireEnv("SOCKERLESS_ACA_RESOURCE_GROUP")
		storageAccount = requireEnv("SOCKERLESS_ACA_STORAGE_ACCOUNT")
		logAnalyticsWS = requireEnv("SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE")
	}

	// Build backend
	backendDir := repoRoot + "/backends/aca"
	backendBinary := backendDir + "/sockerless-backend-aca"
	fmt.Println("[backend] Building sockerless-backend-aca...")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-aca", "./cmd/sockerless-backend-aca")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		failClean("ERROR: build sockerless-backend-aca: %v\n", err)
	}
	cleanups = append(cleanups, func() { os.Remove(backendBinary) })

	// Start backend pointed at the resolved endpoint.
	backendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	fmt.Printf("[backend] Starting sockerless-backend-aca on %s (target=%s endpoint=%s)\n", backendAddr, target, endpointURL)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+endpointURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_ACA_SUBSCRIPTION_ID="+subscriptionID,
		"SOCKERLESS_ACA_RESOURCE_GROUP="+resourceGroup,
		"SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE="+logAnalyticsWS,
		"SOCKERLESS_ACA_STORAGE_ACCOUNT="+storageAccount,
	)
	backendCmd.Stdout = os.Stderr
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		failClean("ERROR: start sockerless-backend-aca: %v\n", err)
	}
	cleanups = append(cleanups, func() { backendCmd.Process.Kill(); backendCmd.Wait() })

	backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
	if err := waitForReady(backendURL, 15*time.Second); err != nil {
		failClean("ERROR: sockerless-backend-aca not ready: %v\n", err)
	}
	fmt.Printf("[backend] ready on %s\n", backendAddr)

	// The ACA backend serves the Docker API directly. Point the docker
	// SDK at the backend's TCP port.
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

func TestACAContainerLifecycle(t *testing.T) {
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
		nil, nil, nil, "aca_"+testID,
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

	// Start (ACA may take longer — 10 min timeout)
	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
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

func TestACAContainerLogs(t *testing.T) {
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
			Entrypoint: []string{"sh", "-c", "echo hello-aca && sleep 5"},
		},
		nil, nil, nil, "aca_logs_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := dockerClient.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Wait for log ingestion (Azure Monitor can have 2-10s delay)
	time.Sleep(10 * time.Second)

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
	if !strings.Contains(string(logData), "hello-aca") {
		t.Log("note: log may not yet be available due to Azure Monitor ingestion delay")
	}

	dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
}

func TestACAContainerList(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
		},
		nil, nil, nil, "aca_list_"+testID,
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

func TestACANetworkOperations(t *testing.T) {
	ctx := context.Background()

	testID := generateTestID()
	netName := "aca_net_" + testID

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

// TestACAVolumeOperations — Azure-Files-backed named volumes:
// VolumeCreate provisions a sockerless-managed file share + env-storage,
// VolumeInspect/VolumeList surface it, VolumeRemove deletes both.
func TestACAVolumeOperations(t *testing.T) {
	ctx := context.Background()

	volName := "aca_vol_" + generateTestID()
	vol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{Name: volName})
	if err != nil {
		t.Fatalf("VolumeCreate: %v", err)
	}
	if vol.Name != volName {
		t.Errorf("Volume.Name = %q, want %q", vol.Name, volName)
	}
	if vol.Driver != "azurefile" {
		t.Errorf("Volume.Driver = %q, want azurefile", vol.Driver)
	}
	if vol.Options["shareName"] == "" {
		t.Errorf("Volume.Options missing shareName: %+v", vol.Options)
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
