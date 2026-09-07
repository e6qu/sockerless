//go:build integration

package ecs

import (
	"bytes"
	"context"
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

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	core "github.com/sockerless/backend-core"
)

var dockerClient *client.Client
var evalImageName string

// backendBinaryPath + backendBaseEnv are set in TestMain so tests can
// spawn additional backend instances with extra process-level config
// (e.g. SOCKERLESS_ECS_SHARED_VOLUMES, which can't be set per-request).
var backendBinaryPath string
var backendBaseEnv []string

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
//	SOCKERLESS_ECS_CPU_ARCHITECTURE,
//	SOCKERLESS_ECS_EVAL_IMAGE) and fails
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

	repoRoot, repoRootErr := filepath.Abs(findModuleDir("."))
	if repoRootErr != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve repository root: %v\n", repoRootErr)
		os.Exit(1)
	}
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

	if target == "sim" {
		evalDir := repoRoot + "/tests/testdata/eval-arithmetic"
		evalImageName = "sockerless-eval-arithmetic:test"
		fmt.Printf("[setup] Building %s (linux/arm64)...\n", evalImageName)
		evalDockerfile := `FROM public.ecr.aws/docker/library/golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /eval-arithmetic .
FROM public.ecr.aws/docker/library/alpine:latest
COPY --from=build /eval-arithmetic /usr/local/bin/eval-arithmetic
ENTRYPOINT ["/usr/local/bin/eval-arithmetic"]
`
		evalImageBuild := exec.Command("docker", "build",
			"--load",
			"--platform", "linux/arm64",
			"-t", evalImageName, "-f", "-", evalDir)
		evalImageBuild.Stdin = strings.NewReader(evalDockerfile)
		if out, err := evalImageBuild.CombinedOutput(); err != nil {
			failClean("ERROR: docker build eval-arithmetic image: %v\n%s", err, out)
		}
	} else {
		evalImageName = requireEnv("SOCKERLESS_ECS_EVAL_IMAGE")
	}

	var endpointURL, cluster, subnets, executionRoleARN, cpuArch string
	switch target {
	case "sim":
		// The simulator lives in the sockerless-cloud repository; the tests
		// module pins its version (see the tool directives in tests/go.mod),
		// so build it through that module for a single source of truth.
		simBinary := repoRoot + "/tests/.build/simulator-aws"
		if err := os.MkdirAll(repoRoot+"/tests/.build", 0o755); err != nil {
			failClean("ERROR: create tests/.build: %v\n", err)
		}
		fmt.Println("[sim] Building simulator-aws...")
		build := exec.Command("go", "build", "-tags", "noui", "-o", simBinary,
			"github.com/e6qu/sockerless-cloud/simulator-aws")
		build.Dir = repoRoot + "/tests"
		build.Env = filterBuildEnv(os.Environ())
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			failClean("ERROR: build simulator-aws: %v\n", err)
		}
		cleanups = append(cleanups, func() { os.Remove(simBinary) })

		// Both of the simulator's ports come from one reservation, so the
		// DNS listener cannot be handed the port just chosen for the API.
		ports := core.NewPortReservation()
		simPort := ports.TCP()
		// The Amazon Route 53 resolver's DNS listener. Its default port is
		// the one a host's own mDNS responder already owns, so the simulator
		// is given a coordinate the operating system picked.
		simDNSPort := ports.TCPAndUDP()
		ports.Release()
		simAddr := fmt.Sprintf(":%d", simPort)
		simURL := fmt.Sprintf("http://127.0.0.1:%d", simPort)
		fmt.Printf("[sim] Starting simulator-aws on %s...\n", simAddr)
		simCmd := exec.Command(simBinary)
		simCmd.Env = append(os.Environ(),
			"SIM_LISTEN_ADDR="+simAddr,
			fmt.Sprintf("SIM_DNS_PORT=%d", simDNSPort),
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

		// Create ECS cluster in simulator (sim fixture). This is a direct
		// control-plane call to the AWS simulator, so it is signed with SigV4
		// exactly as the ECS backend's SDK client signs — same seeded bootstrap
		// credential, differing only in the endpoint coordinate.
		body := []byte(fmt.Sprintf(`{"clusterName":"%s"}`, cluster))
		req, _ := http.NewRequest("POST", simURL+"/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "AmazonEC2ContainerServiceV20141113.CreateCluster")
		if err := signAWSControlPlane(req, body, "ecs"); err != nil {
			failClean("ERROR: sign sim ECS CreateCluster: %v\n", err)
		}
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
	backendBinaryPath = backendBinary
	backendBaseEnv = []string{
		"SOCKERLESS_ENDPOINT_URL=" + endpointURL,
		"SOCKERLESS_POLL_INTERVAL=500ms",
		"SOCKERLESS_ECS_CLUSTER=" + cluster,
		"SOCKERLESS_ECS_SUBNETS=" + subnets,
		"SOCKERLESS_ECS_EXECUTION_ROLE_ARN=" + executionRoleARN,
		"SOCKERLESS_ECS_CPU_ARCHITECTURE=" + cpuArch,
	}
	backendCmd := exec.Command(backendBinary, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(), backendBaseEnv...)
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
	dockerClient, err = client.New(
		client.WithHost(fmt.Sprintf("tcp://localhost:%d", backendPort)),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		failClean("ERROR: docker client: %v\n", err)
	}
	if target == "sim" {
		// Provision the simulator coordinate through the same Docker Image Load
		// API an operator uses. Building the fixture directly in the host
		// daemon is not enough: the Amazon ECS backend owns its image catalog,
		// and an image absent from that catalog is correctly treated as a
		// Docker Hub reference.
		save := exec.Command("docker", "save", evalImageName)
		savedImage, err := save.StdoutPipe()
		if err != nil {
			failClean("ERROR: create docker save pipe for %s: %v\n", evalImageName, err)
		}
		var saveStderr bytes.Buffer
		save.Stderr = &saveStderr
		if err := save.Start(); err != nil {
			failClean("ERROR: start docker save for %s: %v\n", evalImageName, err)
		}
		loaded, err := dockerClient.ImageLoad(context.Background(), savedImage)
		if err != nil {
			_ = save.Process.Kill()
			_ = save.Wait()
			failClean("ERROR: load %s through Amazon ECS backend: %v\n", evalImageName, err)
		}
		loadOutput, readErr := io.ReadAll(loaded)
		_ = loaded.Close()
		saveErr := save.Wait()
		if readErr != nil {
			failClean("ERROR: read Amazon ECS backend image-load response for %s: %v\n", evalImageName, readErr)
		}
		if saveErr != nil {
			failClean("ERROR: docker save %s: %v\n%s", evalImageName, saveErr, saveStderr.String())
		}
		fmt.Printf("[setup] Loaded %s through Amazon ECS backend: %s", evalImageName, loadOutput)
	}

	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestECSContainerLifecycle(t *testing.T) {
	ctx := context.Background()

	// Pull image
	rc, err := dockerClient.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
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
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "hello from ecs"},
		Tty:   false,
	}, Name: "ecs-lifecycle-" + generateTestID()})
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	// Start
	if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Wait
	waited := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, errCh := waited.Result, waited.Error
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
	inspected, err := dockerClient.ContainerInspect(ctx, resp.ID, client.ContainerInspectOptions{})
	info := inspected.Container
	if err != nil {
		t.Fatalf("container inspect failed: %v", err)
	}
	if info.State.Status != "exited" {
		t.Errorf("expected status 'exited', got %q", info.State.Status)
	}
}

func TestECSContainerLogs(t *testing.T) {
	ctx := context.Background()

	pullRC, _ := dockerClient.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "log-test-output"},
	}, Name: "ecs-logs-" + generateTestID()})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	_, _ = dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})

	// Wait for exit
	waited2 := dockerClient.ContainerWait(ctx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	waitCh, _ := waited2.Result, waited2.Error
	select {
	case <-waitCh:
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout")
	}

	// Get logs
	logRC, err := dockerClient.ContainerLogs(ctx, resp.ID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		t.Fatalf("logs failed: %v", err)
	}
	defer logRC.Close()

	// Read the FULL demuxed stream: a single Read() can return only the first
	// multiplexed frame and miss the payload in a later frame (CI-flaky).
	var logBuf bytes.Buffer
	_, _ = stdcopy.StdCopy(&logBuf, &logBuf, logRC)
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "log-test-output") {
		t.Errorf("expected logs to contain 'log-test-output', got %q", logOutput)
	}
}

func TestECSAttachedContainerRunsTwoCompleteCycles(t *testing.T) {
	ctx := context.Background()
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image:        "alpine:latest",
		Cmd:          []string{"sh"},
		OpenStdin:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}, Name: "ecs-attached-restart-" + generateTestID()})
	if err != nil {
		t.Fatalf("create attached container: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	runCycle := func(marker string) {
		t.Helper()
		attached, err := dockerClient.ContainerAttach(ctx, resp.ID, client.ContainerAttachOptions{
			Stream: true,
			Stdin:  true,
			Stdout: true,
			Stderr: true,
		})
		if err != nil {
			t.Fatalf("attach cycle %q: %v", marker, err)
		}
		defer attached.Close()

		var stdout, stderr bytes.Buffer
		readDone := make(chan error, 1)
		go func() {
			_, err := stdcopy.StdCopy(&stdout, &stderr, attached.Reader)
			readDone <- err
		}()

		if _, err := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
			t.Fatalf("start cycle %q: %v", marker, err)
		}
		if _, err := io.WriteString(attached.Conn, "echo "+marker+"\n"); err != nil {
			t.Fatalf("write cycle %q stdin: %v", marker, err)
		}
		if err := attached.CloseWrite(); err != nil {
			t.Fatalf("close cycle %q stdin: %v", marker, err)
		}

		select {
		case err := <-readDone:
			if err != nil {
				t.Fatalf("read cycle %q attach: %v", marker, err)
			}
		case <-time.After(5 * time.Minute):
			t.Fatalf("cycle %q attach did not end with its Amazon ECS task", marker)
		}
		if output := stdout.String() + stderr.String(); !strings.Contains(output, marker) {
			t.Fatalf("cycle %q output %q did not contain its marker", marker, output)
		}
	}

	runCycle("first-cycle-complete")
	runCycle("second-cycle-complete")
}

func TestECSContainerExec(t *testing.T) {
	ctx := context.Background()

	pullRC, _ := dockerClient.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image:     "alpine:latest",
		Cmd:       []string{"tail", "-f", "/dev/null"},
		OpenStdin: true,
		Tty:       true,
	}, Name: "ecs-exec-" + generateTestID()})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	_, _ = dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})

	// Create exec
	execResp, err := dockerClient.ExecCreate(ctx, resp.ID, client.ExecCreateOptions{
		Cmd:          []string{"echo", "exec-output"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}

	// Start exec
	hijacked, err := dockerClient.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{})
	if err != nil {
		t.Fatalf("exec start failed: %v", err)
	}
	output, _ := io.ReadAll(hijacked.Reader)
	hijacked.Close()

	if !strings.Contains(string(output), "exec-output") {
		t.Errorf("expected exec output to contain 'exec-output', got %q", string(output))
	}
	if strings.Contains(string(output), "__SOCKEXIT") {
		t.Errorf("exit marker reached the client: %q", string(output))
	}
	inspect, err := dockerClient.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}
	if inspect.Running || inspect.ExitCode != 0 {
		t.Errorf("echo exec: running=%v exit=%d, want finished with 0", inspect.Running, inspect.ExitCode)
	}

	// A failing command's status must reach ExecInspect, as it does on
	// Docker: CI runners decide a step's outcome from it.
	failing, err := dockerClient.ExecCreate(ctx, resp.ID, client.ExecCreateOptions{
		Cmd:          []string{"sh", "-c", "echo before-failure; exit 7"},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		t.Fatalf("exec create failed: %v", err)
	}
	hijacked, err = dockerClient.ExecAttach(ctx, failing.ID, client.ExecAttachOptions{})
	if err != nil {
		t.Fatalf("exec start failed: %v", err)
	}
	output, _ = io.ReadAll(hijacked.Reader)
	hijacked.Close()
	if !strings.Contains(string(output), "before-failure") || strings.Contains(string(output), "__SOCKEXIT") {
		t.Errorf("failing exec output %q", string(output))
	}
	inspect, err = dockerClient.ExecInspect(ctx, failing.ID, client.ExecInspectOptions{})
	if err != nil {
		t.Fatalf("exec inspect failed: %v", err)
	}
	if inspect.Running || inspect.ExitCode != 7 {
		t.Errorf("failing exec: running=%v exit=%d, want finished with 7", inspect.Running, inspect.ExitCode)
	}

	// Stop container
	timeout := 5
	_, _ = dockerClient.ContainerStop(ctx, resp.ID, client.ContainerStopOptions{Timeout: &timeout})
}

func TestECSContainerList(t *testing.T) {
	ctx := context.Background()

	pullRC, _ := dockerClient.ImagePull(ctx, "alpine:latest", client.ImagePullOptions{})
	if pullRC != nil {
		buf := make([]byte, 4096)
		for {
			if _, err := pullRC.Read(buf); err != nil {
				break
			}
		}
		pullRC.Close()
	}

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{
		Image:  "alpine:latest",
		Cmd:    []string{"sleep", "30"},
		Labels: map[string]string{"test": "ecs-list"},
	}, Name: "ecs-list-" + generateTestID()})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})

	_, _ = dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})

	// List running containers
	listed, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{})
	containers := listed.Items
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
	_, _ = dockerClient.ContainerStop(ctx, resp.ID, client.ContainerStopOptions{Timeout: &timeout})
}

func TestECSNetworkOperations(t *testing.T) {
	ctx := context.Background()

	// Create network
	netName := "ecs-test-net-" + generateTestID()
	netResp, err := dockerClient.NetworkCreate(ctx, netName, client.NetworkCreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		t.Fatalf("network create failed: %v", err)
	}
	defer dockerClient.NetworkRemove(ctx, netResp.ID, client.

		// Inspect
		NetworkRemoveOptions{})

	netInspected, err := dockerClient.NetworkInspect(ctx, netResp.ID, client.NetworkInspectOptions{})
	netInfo := netInspected.Network
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}
	if netInfo.Name != netName {
		t.Errorf("expected name %q, got %q", netName, netInfo.Name)
	}

	// List
	netListed, err := dockerClient.NetworkList(ctx, client.NetworkListOptions{})
	networks := netListed.Items
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
	volCreated, err := dockerClient.VolumeCreate(ctx, client.VolumeCreateOptions{Name: volName})
	vol := volCreated.Volume
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

	volInspected, err := dockerClient.VolumeInspect(ctx, volName, client.VolumeInspectOptions{})
	inspected := volInspected.Volume
	if err != nil {
		t.Fatalf("VolumeInspect failed: %v", err)
	}
	if inspected.Name != volName {
		t.Errorf("inspect Name: got %q, want %q", inspected.Name, volName)
	}

	listed, err := dockerClient.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		t.Fatalf("VolumeList failed: %v", err)
	}
	found := false
	for _, v := range listed.Items {
		if v.Name == volName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("VolumeList did not include %q; got %d volumes", volName, len(listed.Items))
	}

	if _, err := dockerClient.VolumeRemove(ctx, volName, client.VolumeRemoveOptions{Force: false}); err != nil {
		t.Fatalf("VolumeRemove failed: %v", err)
	}

	if _, err := dockerClient.VolumeInspect(ctx, volName, client.VolumeInspectOptions{}); err == nil {
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
