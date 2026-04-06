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
	baseURL        string
	simCmd         *exec.Cmd
	binaryPath     string
	evalBinaryPath string
	evalImageName  string // Docker image containing eval-arithmetic binary
	ctx            = context.Background()
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

	// Build eval-arithmetic binary (static, for embedding in a Docker image)
	evalDir, _ := filepath.Abs("../../testdata/eval-arithmetic")
	evalBinaryPath = filepath.Join(evalDir, "eval-arithmetic")
	evalBuild := exec.Command("go", "build", "-o", evalBinaryPath, ".")
	evalBuild.Dir = evalDir
	evalBuild.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOOS=linux")
	if out, err := evalBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic: %v\n%s", err, out)
	}

	// Build Docker image containing the eval binary
	evalImageName = "sockerless-eval-arithmetic:test"
	dockerfile := fmt.Sprintf("FROM alpine:latest\nCOPY %s /usr/local/bin/eval-arithmetic\nENTRYPOINT [\"/usr/local/bin/eval-arithmetic\"]\n", "eval-arithmetic")
	dockerBuild := exec.Command("docker", "build", "-t", evalImageName, "-f", "-", evalDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic Docker image: %v\n%s", err, out)
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
