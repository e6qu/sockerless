package aws_sdk_test

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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

var (
	baseURL                string
	simCmd                 *exec.Cmd
	binaryPath             string
	evalImageName          string // Docker image containing eval-arithmetic binary
	lambdaHandlerImageName string // Docker image for Lambda Runtime API test handler
	ctx                    = context.Background()
)

func sdkConfig() aws.Config {
	return aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
}

func TestMain(m *testing.M) {
	binaryPath, _ = filepath.Abs("../simulator-aws")

	simDir, _ := filepath.Abs("..")
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
	build.Dir = simDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build simulator: %v\n%s", err, out)
	}

	// Build the Docker image hosting eval-arithmetic. Multi-stage Docker
	// build forced to linux/arm64 — sim's primary capacity contract.
	// CI on amd64 hosts uses QEMU. Phase 135.
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
	dockerBuild := exec.Command("docker", "build",
		"--platform", "linux/arm64",
		"-t", evalImageName, "-f", "-", evalDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic Docker image: %v\n%s", err, out)
	}

	// lambda-runtime-handler image — multi-stage Docker build forced to
	// linux/arm64 (matches eval-arithmetic; sim's primary capacity).
	lambdaHandlerDir, _ := filepath.Abs("../../testdata/lambda-runtime-handler")
	lambdaHandlerImageName = "sockerless-lambda-runtime-handler:test"
	lhDockerfile := `FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /lambda-runtime-handler .
FROM alpine:latest
COPY --from=build /lambda-runtime-handler /usr/local/bin/lambda-runtime-handler
ENTRYPOINT ["/usr/local/bin/lambda-runtime-handler"]
`
	lhDockerBuild := exec.Command("docker", "build",
		"--platform", "linux/arm64",
		"-t", lambdaHandlerImageName, "-f", "-", lambdaHandlerDir)
	lhDockerBuild.Stdin = strings.NewReader(lhDockerfile)
	if out, err := lhDockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build lambda-runtime-handler Docker image: %v\n%s", err, out)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(), fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port))
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)

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
