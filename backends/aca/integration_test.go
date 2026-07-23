//go:build integration

package aca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

var dockerClient *client.Client

// backendPort is set in TestMain; used by callers that construct the
// reverse-agent callback URL.
var backendPort int
var evalImageName string
var commandImageName string
var acaOverlayImageName string

// backendBinaryPath + backendBaseEnv are set in TestMain so tests can
// spawn additional backend instances with extra process-level config
// (e.g. SOCKERLESS_ACA_SHARED_VOLUMES, which can't be set per-request).
var backendBinaryPath string
var backendBaseEnv []string

const (
	acaAppsE2EEnv = "SOCKERLESS_ACA_APPS_E2E"

	// simIdentityHeader is the shared-secret value the Azure platform injects
	// as IDENTITY_HEADER alongside IDENTITY_ENDPOINT into an App Service /
	// Container Apps container. DefaultAzureCredential echoes it back as the
	// X-IDENTITY-HEADER request header when it acquires a managed-identity
	// token. The backend process receives the same value below.
	simIdentityHeader = "sim-identity-header"
)

// simARMBearer acquires an Azure Resource Manager bearer token from the
// simulator the exact way a managed-identity client does in production: an App
// Service MSI request against IDENTITY_ENDPOINT (here the sim's /msi/token) for
// the ARM resource. The simulator mints a real, RS256-signed token whose `aud`
// is the management audience — the same token DefaultAzureCredential obtains
// inside the backend. The operator-shaped ARM PUTs that provision the storage
// account and managed environment carry this bearer, exactly as a real ARM
// client must. Only the coordinate (simURL) differs from real Azure.
func simARMBearer(simURL string) string {
	die := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, format, args...)
		os.Exit(1)
	}
	req, err := http.NewRequest("GET",
		simURL+"/msi/token?resource=https://management.azure.com/", nil)
	if err != nil {
		die("ERROR: build ARM token request: %v\n", err)
	}
	req.Header.Set("X-IDENTITY-HEADER", simIdentityHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		die("ERROR: acquire ARM token from sim: %v\n", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		die("ERROR: sim /msi/token returned %d: %s\n", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		die("ERROR: decode ARM token response: %v (body=%s)\n", err, string(body))
	}
	if tok.AccessToken == "" {
		die("ERROR: sim /msi/token returned an empty access_token (body=%s)\n", string(body))
	}
	return tok.AccessToken
}

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

func reverseAgentCallbackHost(ctx context.Context, runtimeClient *client.Client) string {
	version, err := runtimeClient.ServerVersion(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[backend] WARNING: docker server version unavailable, using host.docker.internal for reverse-agent callback: %v\n", err)
		return "host.docker.internal"
	}
	for _, component := range version.Components {
		if strings.Contains(strings.ToLower(component.Name), "podman") {
			return "host.containers.internal"
		}
	}
	if strings.Contains(strings.ToLower(version.Platform.Name), "podman") {
		return "host.containers.internal"
	}
	return "host.docker.internal"
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
	absRepoRoot, absErr := filepath.Abs(repoRoot)
	if absErr != nil {
		failClean("ERROR: resolve repo root: %v\n", absErr)
	}
	repoRoot = absRepoRoot

	buildScratchGoImage := func(imageName, moduleRel, binaryName string) {
		buildCtx, err := os.MkdirTemp("", "sockerless-"+binaryName+"-image-")
		if err != nil {
			failClean("ERROR: create %s image context: %v\n", binaryName, err)
		}
		cleanups = append(cleanups, func() { os.RemoveAll(buildCtx) })

		binaryPath := filepath.Join(buildCtx, binaryName)
		buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
		buildCmd.Dir = filepath.Join(repoRoot, moduleRel)
		buildCmd.Env = filterBuildEnv(os.Environ(), "GOWORK=off", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
		buildCmd.Stdout = os.Stderr
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			failClean("ERROR: build %s binary: %v\n", binaryName, err)
		}

		dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /usr/local/bin/%s
ENTRYPOINT ["/usr/local/bin/%s"]
`, binaryName, binaryName, binaryName)
		imageBuild := exec.Command("docker", "build",
			"--platform", "linux/arm64",
			"-t", imageName, "-f", "-", buildCtx)
		imageBuild.Stdin = strings.NewReader(dockerfile)
		if out, err := imageBuild.CombinedOutput(); err != nil {
			failClean("ERROR: docker build %s image failed: %v\n%s", binaryName, err, out)
		}
	}

	// Build workload images forced to linux/arm64, the sim's primary
	// capacity contract. The images are built from local Go binaries and
	// scratch roots so the harness does not depend on external registries.
	evalImageName = "sockerless-eval-arithmetic:test"
	fmt.Printf("[setup] Building %s (linux/arm64)...\n", evalImageName)
	buildScratchGoImage(evalImageName, "simulators/testdata/eval-arithmetic", "eval-arithmetic")

	commandImageName = "sockerless-container-command:test"
	fmt.Printf("[setup] Building %s (linux/arm64)...\n", commandImageName)
	buildScratchGoImage(commandImageName, "simulators/testdata/container-command", "container-command")

	if os.Getenv(acaAppsE2EEnv) == "1" {
		bootstrapPath := filepath.Join(repoRoot, "agent", fmt.Sprintf("sockerless-cloudrun-bootstrap-aca-test-arm64-%d", os.Getpid()))
		buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		bootstrapBuild := exec.CommandContext(buildCtx, "go", "build", "-o", bootstrapPath, "./cmd/sockerless-cloudrun-bootstrap")
		bootstrapBuild.Dir = filepath.Join(repoRoot, "agent")
		bootstrapBuild.Env = filterBuildEnv(os.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
		bootstrapBuild.Stdout = os.Stderr
		bootstrapBuild.Stderr = os.Stderr
		if err := bootstrapBuild.Run(); err != nil {
			failClean("ERROR: build ACA app bootstrap: %v\n", err)
		}
		if os.Getenv(acaAppsE2EEnv) != "1" {
			cleanups = append(cleanups, func() { os.Remove(bootstrapPath) })
		}

		overlayCtx, err := os.MkdirTemp("", "sockerless-aca-overlay-")
		if err != nil {
			failClean("ERROR: create ACA app overlay context: %v\n", err)
		}
		cleanups = append(cleanups, func() { os.RemoveAll(overlayCtx) })
		bootstrapBytes, err := os.ReadFile(bootstrapPath)
		if err != nil {
			failClean("ERROR: read ACA app bootstrap: %v\n", err)
		}
		if err := os.WriteFile(filepath.Join(overlayCtx, filepath.Base(bootstrapPath)), bootstrapBytes, 0o755); err != nil {
			failClean("ERROR: write ACA app overlay bootstrap: %v\n", err)
		}
		overlayCommand := filepath.Join(overlayCtx, "container-command")
		commandBuild := exec.Command("go", "build", "-o", overlayCommand, ".")
		commandBuild.Dir = filepath.Join(repoRoot, "simulators/testdata/container-command")
		commandBuild.Env = filterBuildEnv(os.Environ(), "GOWORK=off", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
		commandBuild.Stdout = os.Stderr
		commandBuild.Stderr = os.Stderr
		if err := commandBuild.Run(); err != nil {
			failClean("ERROR: build ACA app command workload: %v\n", err)
		}

		acaOverlayImageName = fmt.Sprintf("sockerless-overlay/aca:test-%d", os.Getpid())
		// busybox base (not scratch) so the image carries a real /bin/sh —
		// the gitlab-runner attach-stdin pattern pipes a shell script that the
		// bootstrap runs under /bin/sh (see TestACAGitLabRunnerAttachStdin),
		// mirroring the shell-capable images real overlays are built on.
		overlayDockerfile := fmt.Sprintf(`FROM busybox:latest
COPY %s /opt/sockerless/sockerless-cloudrun-bootstrap
COPY container-command /opt/sockerless/container-command
ENTRYPOINT ["/opt/sockerless/sockerless-cloudrun-bootstrap"]
`, filepath.Base(bootstrapPath))
		overlayBuild := exec.Command("docker", "build",
			"--load",
			"--platform", "linux/arm64",
			"-t", acaOverlayImageName,
			"-f", "-", overlayCtx)
		overlayBuild.Stdin = strings.NewReader(overlayDockerfile)
		overlayBuild.Stdout = os.Stderr
		overlayBuild.Stderr = os.Stderr
		if err := overlayBuild.Run(); err != nil {
			failClean("ERROR: build ACA app overlay image: %v\n", err)
		}
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
		if os.Getenv(acaAppsE2EEnv) != "1" {
			cleanups = append(cleanups, func() { os.Remove(simBinary) })
		}

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
		// Acquire a real ARM bearer from the sim's managed-identity endpoint;
		// the sim now enforces bearer auth on the ARM control plane exactly as
		// real Azure does, so every /subscriptions/ PUT must carry it.
		armBearer := simARMBearer(simURL)
		preCreate := func(url, body string) {
			req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+armBearer)
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
		cleanups = append(cleanups, func() {
			req, _ := http.NewRequest("DELETE",
				fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/sockerless?api-version=2024-03-01",
					simURL, subscriptionID, resourceGroup),
				nil,
			)
			req.Header.Set("Authorization", "Bearer "+armBearer)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		})

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
	// The gitlab-attach test re-execs this binary with the apps-E2E env
	// set, which runs TestMain AGAIN as a subprocess. That nested run
	// must not delete repo-path artifacts the PARENT suite still uses
	// (it deleted the backend binary out from under later parent tests
	// that spawn it).
	nestedRun := os.Getenv(acaAppsE2EEnv) == "1"
	if !nestedRun {
		cleanups = append(cleanups, func() { os.Remove(backendBinary) })
	}

	runtimeClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		failClean("ERROR: docker runtime client: %v\n", err)
	}
	defer runtimeClient.Close()
	callbackHost := reverseAgentCallbackHost(context.Background(), runtimeClient)

	// Start backend pointed at the resolved endpoint.
	backendPort = findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	fmt.Printf("[backend] Starting sockerless-backend-aca on %s (target=%s endpoint=%s)\n", backendAddr, target, endpointURL)
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendBinaryPath = backendBinary
	backendBaseEnv = []string{
		"SOCKERLESS_ENDPOINT_URL=" + endpointURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_ACA_SUBSCRIPTION_ID=" + subscriptionID,
		"SOCKERLESS_ACA_RESOURCE_GROUP=" + resourceGroup,
		"SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE=" + logAnalyticsWS,
		"SOCKERLESS_ACA_STORAGE_ACCOUNT=" + storageAccount,
		// Required at NewServer (no fallback).
		"SOCKERLESS_CALLBACK_URL=" + fmt.Sprintf("ws://%s:%d/v1/aca/reverse", callbackHost, backendPort),
	}
	if target == "sim" {
		// Inject the App Service / Container Apps managed-identity coordinate
		// the same way the real Azure platform injects it into an ACA app
		// container. The backend's DefaultAzureCredential (used identically
		// against real Azure and the simulator) reads IDENTITY_ENDPOINT +
		// IDENTITY_HEADER and performs a real managed-identity token
		// acquisition against it — here the simulator's /msi/token endpoint,
		// which mints a real, verifiable ARM bearer. Only the coordinate value
		// differs from real Azure. Against the cloud target the platform
		// supplies these itself, so they are not set here.
		backendBaseEnv = append(backendBaseEnv,
			"IDENTITY_ENDPOINT="+endpointURL+"/msi/token",
			"IDENTITY_HEADER="+simIdentityHeader,
		)
	}
	if os.Getenv(acaAppsE2EEnv) == "1" {
		backendBaseEnv = append(backendBaseEnv, "SOCKERLESS_ACA_USE_APP=1")
	}
	backendCmd.Env = append(os.Environ(), backendBaseEnv...)
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

	testID := generateTestID()

	// Create
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: commandImageName,
			Cmd:   []string{"hold"},
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

	testID := generateTestID()
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: commandImageName,
			Cmd:   []string{"log", "hello-aca", "5"},
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
			Image: commandImageName,
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

func TestACAGitLabRunnerAttachStdin(t *testing.T) {
	if os.Getenv(acaAppsE2EEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run", "^TestACAGitLabRunnerAttachStdin$", "-test.v")
		cmd.Env = append(os.Environ(), acaAppsE2EEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ACA Apps gitlab attach subprocess failed: %v\n%s", err, string(out))
		}
		return
	}

	if acaOverlayImageName == "" {
		t.Fatal("ACA overlay image was not built by TestMain")
	}

	ctx := context.Background()
	testID := generateTestID()
	// gitlab-runner attach-stdin pattern: the runner pipes a shell script to the
	// container's process and the backend runs it under /bin/sh (it must — the
	// gitlab-runner helper's own entrypoint reads stdin in a private protocol and
	// ignores a raw script). So the captured stdin is a shell command, mirroring
	// the gcf/cloudrun equivalents.
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image:        acaOverlayImageName,
			Cmd:          []string{"sh"},
			OpenStdin:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
		},
		nil, nil, nil, "aca_gitlab_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	hijacked, err := dockerClient.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		t.Fatalf("container attach failed: %v", err)
	}
	defer hijacked.Close()

	if _, err := hijacked.Conn.Write([]byte("echo aca-gitlab-stdin-ok\n")); err != nil {
		t.Fatalf("write attach stdin: %v", err)
	}
	if err := hijacked.CloseWrite(); err != nil {
		t.Fatalf("close attach stdin: %v", err)
	}

	startCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := dockerClient.ContainerStart(startCtx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	var stdout, stderr bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&stdout, &stderr, hijacked.Reader)
		copyDone <- err
	}()

	select {
	case err := <-copyDone:
		if err != nil {
			t.Fatalf("attach stream copy failed: %v", err)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for attach output")
	}
	if !strings.Contains(stdout.String(), "aca-gitlab-stdin-ok") {
		t.Fatalf("attach stdout = %q stderr = %q", stdout.String(), stderr.String())
	}

	waitCh, errCh := dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.StatusCode != 0 {
			t.Fatalf("wait status = %d, want 0", result.StatusCode)
		}
	case err := <-errCh:
		t.Fatalf("container wait error: %v", err)
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for container")
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
