package simulator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// ContainerConfig describes a container to run.
type ContainerConfig struct {
	Image     string            // container image (e.g., "alpine:latest")
	Command   []string          // entrypoint override (empty = use image default)
	Args      []string          // command/args (empty = use image default)
	Env       map[string]string // environment variables
	Timeout   time.Duration     // max execution time (0 = no limit)
	Labels    map[string]string // container labels for tracking
	Network   string            // Docker network to join (optional)
	Name      string            // container name (optional, auto-generated if empty)
	Tty       bool              // allocate a pseudo-TTY
	OpenStdin bool              // keep stdin open
	Binds     []string          // bind mounts (e.g., "vol:/path")
}

// ContainerHandle manages a running container.
type ContainerHandle struct {
	ContainerID string
	cancel      context.CancelFunc
	done        <-chan ProcessResult
	cli         *client.Client
}

// Wait blocks until the container exits.
func (h *ContainerHandle) Wait() ProcessResult { return <-h.done }

// Cancel stops and removes the container.
func (h *ContainerHandle) Cancel() { h.cancel() }

// dockerClient is the shared Docker client. Initialized once at startup.
var (
	dockerClient     *client.Client
	dockerClientOnce sync.Once
	dockerClientErr  error
)

// InitDocker initializes the shared Docker client and verifies connectivity.
// Must be called at simulator startup. Fatally exits if Docker is not available.
func InitDocker() *client.Client {
	dockerClientOnce.Do(func() {
		dockerClient, dockerClientErr = client.NewClientWithOpts(
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		)
		if dockerClientErr != nil {
			return
		}
		// Verify connectivity
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, dockerClientErr = dockerClient.Ping(ctx)
	})
	if dockerClientErr != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Docker/Podman not available: %v\n", dockerClientErr)
		fmt.Fprintf(os.Stderr, "Simulators require a container runtime. Install Docker or Podman.\n")
		os.Exit(1)
	}
	return dockerClient
}

// DockerClient returns the shared Docker client. InitDocker must have been called first.
func DockerClient() *client.Client {
	return dockerClient
}

// managedContainers tracks containers created by this simulator instance for cleanup.
var managedContainers sync.Map // containerID -> true

// CleanupContainers stops and removes all simulator-managed containers.
// Called on simulator shutdown.
func CleanupContainers() {
	if dockerClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	managedContainers.Range(func(key, _ any) bool {
		id := key.(string)
		timeout := 5
		_ = dockerClient.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
		_ = dockerClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
		return true
	})
}

// StartContainer pulls the image (if needed), creates and starts a container.
// Returns a ContainerHandle immediately. Call handle.Wait() to block until exit.
// Stdout/stderr are streamed to the LogSink.
func StartContainer(cfg ContainerConfig, sink LogSink) *ContainerHandle {
	resultCh := make(chan ProcessResult, 1)

	cli := DockerClient()
	if cli == nil {
		resultCh <- ProcessResult{
			ExitCode:  -1,
			StartedAt: time.Now(),
			StoppedAt: time.Now(),
			Error:     fmt.Errorf("docker client not initialized"),
		}
		return &ContainerHandle{cancel: func() {}, done: resultCh, cli: cli}
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		result := runContainer(ctx, cli, cfg, sink)
		resultCh <- result
	}()

	// Wait briefly for the container to start so we can capture the ID
	// The ContainerHandle is returned immediately; the goroutine runs in background
	return &ContainerHandle{cancel: cancel, done: resultCh, cli: cli}
}

// StartContainerSync is like StartContainer but returns the handle with ContainerID populated.
// Blocks until the container is created and started (but not until it exits).
func StartContainerSync(cfg ContainerConfig, sink LogSink) (*ContainerHandle, error) {
	cli := DockerClient()
	if cli == nil {
		return nil, fmt.Errorf("docker client not initialized")
	}

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan ProcessResult, 1)

	containerID, err := createAndStartContainer(ctx, cli, cfg)
	if err != nil {
		cancel()
		return nil, err
	}

	managedContainers.Store(containerID, true)

	// Stream logs and wait for exit in background
	go func() {
		result := waitAndCaptureLogs(ctx, cli, containerID, cfg, sink)
		managedContainers.Delete(containerID)
		// Remove container after exit
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rmCancel()
		_ = cli.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true})
		resultCh <- result
	}()

	handle := &ContainerHandle{
		ContainerID: containerID,
		cancel:      cancel,
		done:        resultCh,
		cli:         cli,
	}
	return handle, nil
}

// StopContainer stops a running container by ID.
func StopContainer(containerID string) {
	if dockerClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	timeout := 10
	_ = dockerClient.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

func runContainer(ctx context.Context, cli *client.Client, cfg ContainerConfig, sink LogSink) ProcessResult {
	containerID, err := createAndStartContainer(ctx, cli, cfg)
	if err != nil {
		return ProcessResult{
			ExitCode:  -1,
			StartedAt: time.Now(),
			StoppedAt: time.Now(),
			Error:     err,
		}
	}

	managedContainers.Store(containerID, true)
	defer func() {
		managedContainers.Delete(containerID)
		// Remove container after exit
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rmCancel()
		_ = cli.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true})
	}()

	return waitAndCaptureLogs(ctx, cli, containerID, cfg, sink)
}

func createAndStartContainer(ctx context.Context, cli *client.Client, cfg ContainerConfig) (string, error) {
	// Pull image
	pullPolicy := os.Getenv("SIM_PULL_POLICY")
	if pullPolicy == "" {
		pullPolicy = "if-not-present"
	}

	shouldPull := pullPolicy == "always"
	if pullPolicy == "if-not-present" {
		_, _, err := cli.ImageInspectWithRaw(ctx, cfg.Image)
		if err != nil {
			shouldPull = true
		}
	}

	if shouldPull {
		reader, err := cli.ImagePull(ctx, cfg.Image, image.PullOptions{})
		if err != nil {
			return "", fmt.Errorf("image pull %s: %w", cfg.Image, err)
		}
		// Drain pull output
		_, _ = io.Copy(io.Discard, reader)
		_ = reader.Close()
	}

	// Build container config
	var env []string
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	labels := map[string]string{
		"sockerless-sim": "true",
	}
	for k, v := range cfg.Labels {
		labels[k] = v
	}

	containerCfg := &container.Config{
		Image:       cfg.Image,
		Env:         env,
		Labels:      labels,
		Tty:         cfg.Tty,
		OpenStdin:   cfg.OpenStdin,
		AttachStdin: cfg.OpenStdin,
	}

	// Set entrypoint and command separately
	if len(cfg.Command) > 0 {
		containerCfg.Entrypoint = cfg.Command
	}
	if len(cfg.Args) > 0 {
		containerCfg.Cmd = cfg.Args
	}

	hostCfg := &container.HostConfig{
		Binds: cfg.Binds,
	}

	var networkCfg *network.NetworkingConfig
	if cfg.Network != "" {
		networkCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				cfg.Network: {},
			},
		}
	}

	resp, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, cfg.Name)
	if err != nil {
		return "", fmt.Errorf("container create: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Cleanup on start failure
		_ = cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("container start: %w", err)
	}

	return resp.ID, nil
}

func waitAndCaptureLogs(ctx context.Context, cli *client.Client, containerID string, cfg ContainerConfig, sink LogSink) ProcessResult {
	startedAt := time.Now()

	// Attach to logs — must complete before we return the result
	var logDone chan struct{}
	logReader, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	if err == nil {
		logDone = make(chan struct{})
		go func() {
			streamDockerLogs(logReader, sink)
			close(logDone)
		}()
	}

	// Enforce timeout via a separate goroutine
	if cfg.Timeout > 0 {
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(cfg.Timeout):
				timeout := 5
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = cli.ContainerStop(stopCtx, containerID, container.StopOptions{Timeout: &timeout})
			}
		}()
	}

	// Wait for container to exit
	var result ProcessResult
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			result = ProcessResult{
				ExitCode:  -1,
				StartedAt: startedAt,
				StoppedAt: time.Now(),
				Error:     err,
			}
		}
	case status := <-statusCh:
		result = ProcessResult{
			ExitCode:  int(status.StatusCode),
			StartedAt: startedAt,
			StoppedAt: time.Now(),
		}
	case <-ctx.Done():
		timeout := 5
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = cli.ContainerStop(stopCtx, containerID, container.StopOptions{Timeout: &timeout})
		result = ProcessResult{
			ExitCode:  137,
			StartedAt: startedAt,
			StoppedAt: time.Now(),
		}
	}

	// Wait for log drain to complete before returning — ensures all container
	// output reaches the LogSink (and thus Cloud Logging) before the execution
	// is marked as completed.
	if logDone != nil {
		select {
		case <-logDone:
		case <-time.After(5 * time.Second):
			// Safety timeout — don't hang forever if log reader is stuck
		}
	}

	return result
}

// streamDockerLogs demuxes Docker log output and sends lines to the sink.
func streamDockerLogs(reader io.ReadCloser, sink LogSink) {
	defer reader.Close()

	// Docker multiplexed output: use stdcopy to demux
	stdoutPR, stdoutPW := io.Pipe()
	stderrPR, stderrPW := io.Pipe()

	go func() {
		_, _ = stdcopy.StdCopy(stdoutPW, stderrPW, reader)
		_ = stdoutPW.Close()
		_ = stderrPW.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	scanStream := func(r io.Reader, stream string) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			sink.WriteLog(LogLine{
				Stream:    stream,
				Text:      scanner.Text(),
				Timestamp: time.Now(),
			})
		}
	}

	go scanStream(stdoutPR, "stdout")
	go scanStream(stderrPR, "stderr")

	wg.Wait()
}

// ResolveLocalImage maps cloud registry URIs back to Docker Hub images for local execution.
// Cloud backends resolve "alpine:latest" to cloud-specific registries:
//   - GCP AR: "us-central1-docker.pkg.dev/project/docker-hub/library/alpine:latest"
//   - AWS ECR: "123456789.dkr.ecr.eu-west-1.amazonaws.com/alpine:latest"
//   - Azure ACR: "myacr.azurecr.io/library/alpine:latest"
//
// The simulator runs containers locally where only Docker Hub images exist,
// so these URIs must be resolved back to their original form.
func ResolveLocalImage(image string) string {
	// GCP Artifact Registry pull-through cache
	if strings.Contains(image, "-docker.pkg.dev/") && strings.Contains(image, "/docker-hub/") {
		idx := strings.Index(image, "/docker-hub/")
		dockerPath := image[idx+len("/docker-hub/"):]
		dockerPath = strings.TrimPrefix(dockerPath, "library/")
		return dockerPath
	}
	// AWS ECR pull-through cache
	if strings.Contains(image, ".dkr.ecr.") && strings.Contains(image, ".amazonaws.com/") {
		idx := strings.Index(image, ".amazonaws.com/")
		dockerPath := image[idx+len(".amazonaws.com/"):]
		dockerPath = strings.TrimPrefix(dockerPath, "library/")
		dockerPath = strings.TrimPrefix(dockerPath, "docker-hub/")
		return dockerPath
	}
	// Azure ACR
	if strings.Contains(image, ".azurecr.io/") {
		idx := strings.Index(image, ".azurecr.io/")
		dockerPath := image[idx+len(".azurecr.io/"):]
		dockerPath = strings.TrimPrefix(dockerPath, "library/")
		return dockerPath
	}
	return image
}

// RuntimeInfo returns the container runtime name and version for display.
func RuntimeInfo() string {
	if dockerClient == nil {
		return "not initialized"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := dockerClient.ServerVersion(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	name := "Docker"
	for _, c := range info.Components {
		if strings.EqualFold(c.Name, "Podman Engine") {
			name = "Podman"
			break
		}
	}
	return fmt.Sprintf("%s %s", name, info.Version)
}
