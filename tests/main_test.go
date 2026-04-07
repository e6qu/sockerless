package tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

var (
	dockerClient   *client.Client
	serverAddr     string
	evalBinaryPath string
	evalImageName  string
	ctx            = context.Background()
)

func TestMain(m *testing.M) {
	// If --socket flag is provided, connect to an external system
	if socket := os.Getenv("SOCKERLESS_SOCKET"); socket != "" {
		var err error
		dockerClient, err = client.NewClientWithOpts(
			client.WithHost("unix://"+socket),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create docker client: %v\n", err)
			os.Exit(1)
		}
		os.Exit(m.Run())
	}

	// Build eval-arithmetic binary
	evalDir := findModuleDir("simulators/testdata/eval-arithmetic")
	evalBinaryPath = evalDir + "/eval-arithmetic"
	fmt.Println("Building eval-arithmetic...")
	evalBuild := exec.Command("go", "build", "-o", "eval-arithmetic", ".")
	evalBuild.Dir = evalDir
	evalBuild.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOOS=linux")
	evalBuild.Stdout = os.Stderr
	evalBuild.Stderr = os.Stderr
	if err := evalBuild.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build eval-arithmetic: %v\n", err)
		os.Exit(1)
	}

	// Build Docker image containing the eval binary
	evalImageName = "sockerless-eval-arithmetic:test"
	dockerfile := "FROM alpine:latest\nCOPY eval-arithmetic /usr/local/bin/eval-arithmetic\nENTRYPOINT [\"/usr/local/bin/eval-arithmetic\"]\n"
	dockerBuild := exec.Command("docker", "build", "-t", evalImageName, "-f", "-", evalDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build eval-arithmetic Docker image: %v\n%s", err, out)
		os.Exit(1)
	}

	// Build AWS simulator (independent module, not in go.work)
	simDir := findModuleDir("simulators/aws")
	fmt.Println("Building AWS simulator...")
	buildSim := exec.Command("go", "build", "-tags", "noui", "-o", "simulator-aws", ".")
	buildSim.Dir = simDir
	// Filter out GOOS/GOARCH from env
	var filteredEnv []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOOS=") || strings.HasPrefix(e, "GOARCH=") {
			continue
		}
		filteredEnv = append(filteredEnv, e)
	}
	buildSim.Env = append(filteredEnv, "GOWORK=off")
	buildSim.Stdout = os.Stderr
	buildSim.Stderr = os.Stderr
	if err := buildSim.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build simulator: %v\n", err)
		os.Exit(1)
	}
	simBin := simDir + "/simulator-aws"

	// Build ECS backend
	fmt.Println("Building ECS backend...")
	backendDir := findModuleDir("backends/ecs")
	buildBackend := exec.Command("go", "build", "-tags", "noui", "-o", "sockerless-backend-ecs", "./cmd/sockerless-backend-ecs")
	buildBackend.Dir = backendDir
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build backend: %v\n", err)
		os.Exit(1)
	}
	backendBin := backendDir + "/sockerless-backend-ecs"

	// Find free ports
	simPort := findFreePort()
	backendPort := findFreePort()

	// Start AWS simulator
	fmt.Printf("Starting AWS simulator on :%d...\n", simPort)
	simCmd := exec.Command(simBin)
	simCmd.Env = append(os.Environ(), fmt.Sprintf("SIM_LISTEN_ADDR=:%d", simPort))
	simCmd.Stdout = os.Stderr
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start simulator: %v\n", err)
		os.Exit(1)
	}

	simURL := fmt.Sprintf("http://127.0.0.1:%d", simPort)
	if err := waitForReady(simURL+"/health", 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "simulator not ready: %v\n", err)
		simCmd.Process.Kill()
		os.Exit(1)
	}
	fmt.Println("AWS simulator ready")

	// Create ECS cluster in simulator
	clusterBody := `{"clusterName":"sim-cluster"}`
	req, _ := http.NewRequest("POST", simURL+"/", strings.NewReader(clusterBody))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerServiceV20141113.CreateCluster")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create ECS cluster: %v\n", err)
		simCmd.Process.Kill()
		os.Exit(1)
	}
	resp.Body.Close()
	fmt.Println("ECS cluster created")

	// Start ECS backend (serves Docker API directly via in-process wiring)
	backendAddr := fmt.Sprintf(":%d", backendPort)
	serverAddr = fmt.Sprintf("localhost:%d", backendPort)
	fmt.Printf("Starting ECS backend on %s...\n", backendAddr)
	backendCmd := exec.Command(backendBin, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Env = append(os.Environ(),
		"SOCKERLESS_ENDPOINT_URL="+simURL,
		"SOCKERLESS_ECS_CLUSTER=sim-cluster",
		"SOCKERLESS_ECS_SUBNETS=subnet-sim",
		"SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::000000000000:role/sim",
		"SOCKERLESS_AGENT_TIMEOUT=2s",
		"SOCKERLESS_POLL_INTERVAL=500ms",
	)
	backendCmd.Stdout = os.Stderr
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start backend: %v\n", err)
		simCmd.Process.Kill()
		os.Exit(1)
	}

	// Wait for backend to be ready (serves both internal API and Docker API)
	backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
	if err := waitForReady(backendURL, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "backend not ready: %v\n", err)
		backendCmd.Process.Kill()
		simCmd.Process.Kill()
		os.Exit(1)
	}
	fmt.Println("Backend is ready (serving Docker API)")

	// Create Docker SDK client pointing directly at backend
	dockerClient, err = client.NewClientWithOpts(
		client.WithHost("tcp://"+serverAddr),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create docker client: %v\n", err)
		backendCmd.Process.Kill()
		simCmd.Process.Kill()
		os.Exit(1)
	}

	// Start simulator backends if SOCKERLESS_SIM is set
	var simProcesses []*simProcess
	var simSocketPaths []string
	if simVal := os.Getenv("SOCKERLESS_SIM"); simVal != "" {
		procs, sockets, err := startSimBackends(simVal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start sim backends: %v\n", err)
			backendCmd.Process.Kill()
			simCmd.Process.Kill()
			os.Exit(1)
		}
		simProcesses = procs
		simSocketPaths = sockets
	}

	// Run tests
	code := m.Run()

	// Cleanup sim processes (reverse order)
	for i := len(simProcesses) - 1; i >= 0; i-- {
		simProcesses[i].cmd.Process.Kill()
		simProcesses[i].cmd.Wait()
		if simProcesses[i].binaryPath != "" {
			os.Remove(simProcesses[i].binaryPath)
		}
	}
	for _, s := range simSocketPaths {
		os.Remove(s)
	}

	// Cleanup
	backendCmd.Process.Kill()
	simCmd.Process.Kill()
	backendCmd.Wait()
	simCmd.Wait()

	// Clean up binaries
	os.Remove(backendBin)
	os.Remove(simBin)

	os.Exit(code)
}

func findModuleDir(rel string) string {
	// Try relative to current directory, then parent directories
	candidates := []string{
		"../" + rel,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "../" + rel
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
