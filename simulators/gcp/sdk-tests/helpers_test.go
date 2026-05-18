package gcp_sdk_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	baseURL            string
	grpcAddr           string // host:port for gRPC Cloud Logging
	simCmd             *exec.Cmd
	binaryPath         string
	evalImageName      string // Docker image containing eval-arithmetic binary
	httpProbeImageName string // Docker image containing localhost probe/server binary
	ctx                = context.Background()
)

func TestMain(m *testing.M) {
	binaryPath, _ = filepath.Abs("../simulator-gcp")

	simDir, _ := filepath.Abs("..")
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
	build.Dir = simDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build simulator: %v\n%s", err, out)
	}

	// Build the Docker image hosting eval-arithmetic. The build is a
	// multi-stage Docker build so the workload binary's architecture
	// matches the image's (never the sim host's).
	evalDir, _ := filepath.Abs("../../testdata/eval-arithmetic")
	evalImageName = "sockerless-eval-arithmetic:test"
	dockerfile := `FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /eval-arithmetic .
FROM alpine:latest
COPY --from=build /eval-arithmetic /usr/local/bin/eval-arithmetic
ENTRYPOINT ["/usr/local/bin/eval-arithmetic"]
`
	// Build for linux/arm64 explicitly — sim's primary capacity contract.
	// Requires QEMU on amd64 hosts (CI sets it up via docker/setup-qemu-action).
	dockerBuild := exec.Command("docker", "build",
		"--platform", "linux/arm64",
		"-t", evalImageName, "-f", "-", evalDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic Docker image: %v\n%s", err, out)
	}

	probeDir, _ := filepath.Abs("../../testdata/http-localhost-probe")
	httpProbeImageName = "sockerless-http-localhost-probe:test"
	probeDockerfile := `FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /http-localhost-probe .
FROM alpine:latest
COPY --from=build /http-localhost-probe /usr/local/bin/http-localhost-probe
ENTRYPOINT ["/usr/local/bin/http-localhost-probe"]
`
	probeBuild := exec.Command("docker", "build",
		"--platform", "linux/arm64",
		"-t", httpProbeImageName, "-f", "-", probeDir)
	probeBuild.Stdin = strings.NewReader(probeDockerfile)
	if out, err := probeBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build http-localhost-probe Docker image: %v\n%s", err, out)
	}

	// Allocate both ports while both listeners are open. Closing the first
	// before allocating the second lets the OS re-assign the just-freed
	// port to the second listener, causing the sim's HTTP and gRPC servers
	// to collide on the same port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free gRPC port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	grpcPort := ln2.Addr().(*net.TCPAddr).Port
	ln.Close()
	ln2.Close()

	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
		fmt.Sprintf("SIM_GCP_GRPC_PORT=%d", grpcPort),
	)
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	grpcAddr = fmt.Sprintf("127.0.0.1:%d", grpcPort)

	if err := waitForHealth(baseURL + "/health"); err != nil {
		simCmd.Process.Kill()
		log.Fatalf("Simulator did not become healthy: %v", err)
	}

	code := m.Run()
	simCmd.Process.Kill()
	simCmd.Wait()
	os.Exit(code)
}

func waitForHealth(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}
