package tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

var (
	dockerClient *client.Client
	frontendAddr string
	ctx          = context.Background()
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

	// Build binaries
	fmt.Println("Building backend...")
	buildBackend := exec.Command("go", "build", "-o", "sockerless-backend-memory", "./cmd/")
	buildBackend.Dir = findModuleDir("backends/memory")
	buildBackend.Stdout = os.Stderr
	buildBackend.Stderr = os.Stderr
	if err := buildBackend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build backend: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Building frontend...")
	buildFrontend := exec.Command("go", "build", "-o", "sockerless-docker-frontend", "./cmd/")
	buildFrontend.Dir = findModuleDir("frontends/docker")
	buildFrontend.Stdout = os.Stderr
	buildFrontend.Stderr = os.Stderr
	if err := buildFrontend.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build frontend: %v\n", err)
		os.Exit(1)
	}

	// Find free ports
	backendPort := findFreePort()
	frontendPort := findFreePort()
	backendAddr := fmt.Sprintf(":%d", backendPort)
	frontendAddr = fmt.Sprintf("localhost:%d", frontendPort)

	// Start backend
	fmt.Printf("Starting backend on %s...\n", backendAddr)
	backendBin := findModuleDir("backends/memory") + "/sockerless-backend-memory"
	backendCmd := exec.Command(backendBin, "--addr", backendAddr, "--log-level", "debug")
	backendCmd.Stdout = os.Stderr
	backendCmd.Stderr = os.Stderr
	if err := backendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start backend: %v\n", err)
		os.Exit(1)
	}

	// Wait for backend to be ready
	backendURL := fmt.Sprintf("http://localhost:%d/internal/v1/info", backendPort)
	if err := waitForReady(backendURL, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "backend not ready: %v\n", err)
		backendCmd.Process.Kill()
		os.Exit(1)
	}
	fmt.Println("Backend is ready")

	// Start frontend
	fmt.Printf("Starting frontend on :%d...\n", frontendPort)
	frontendBin := findModuleDir("frontends/docker") + "/sockerless-docker-frontend"
	frontendCmd := exec.Command(frontendBin,
		"--addr", fmt.Sprintf(":%d", frontendPort),
		"--backend", fmt.Sprintf("http://localhost:%d", backendPort),
		"--log-level", "debug",
	)
	frontendCmd.Stdout = os.Stderr
	frontendCmd.Stderr = os.Stderr
	if err := frontendCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start frontend: %v\n", err)
		backendCmd.Process.Kill()
		os.Exit(1)
	}

	// Wait for frontend to be ready
	frontendURL := fmt.Sprintf("http://%s/_ping", frontendAddr)
	if err := waitForReady(frontendURL, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "frontend not ready: %v\n", err)
		frontendCmd.Process.Kill()
		backendCmd.Process.Kill()
		os.Exit(1)
	}
	fmt.Println("Frontend is ready")

	// Create Docker SDK client
	var err error
	dockerClient, err = client.NewClientWithOpts(
		client.WithHost("tcp://"+frontendAddr),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create docker client: %v\n", err)
		frontendCmd.Process.Kill()
		backendCmd.Process.Kill()
		os.Exit(1)
	}

	// Start simulator backends if SOCKERLESS_SIM is set
	var simProcesses []*simProcess
	var simSocketPaths []string
	if simVal := os.Getenv("SOCKERLESS_SIM"); simVal != "" {
		frontendBin := findModuleDir("frontends/docker") + "/sockerless-docker-frontend"
		procs, sockets, err := startSimBackends(simVal, frontendBin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start sim backends: %v\n", err)
			frontendCmd.Process.Kill()
			backendCmd.Process.Kill()
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
	frontendCmd.Process.Kill()
	backendCmd.Process.Kill()
	frontendCmd.Wait()
	backendCmd.Wait()

	// Clean up binaries
	os.Remove(findModuleDir("backends/memory") + "/sockerless-backend-memory")
	os.Remove(findModuleDir("frontends/docker") + "/sockerless-docker-frontend")

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
