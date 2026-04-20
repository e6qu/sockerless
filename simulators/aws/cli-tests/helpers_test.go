package aws_cli_test

import (
	"encoding/json"
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
	baseURL                 string
	simCmd                  *exec.Cmd
	binaryPath              string
	evalBinaryPath          string
	evalImageName           string
	lambdaHandlerBinaryPath string
	lambdaHandlerImageName  string
	tmpDir                  string
)

func TestMain(m *testing.M) {
	// Check if aws CLI is installed
	if _, err := exec.LookPath("aws"); err != nil {
		fmt.Println("aws CLI not found, skipping CLI tests")
		os.Exit(0)
	}

	// Build simulator
	binaryPath, _ = filepath.Abs("../simulator-aws")
	simDir, _ := filepath.Abs("..")
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath, ".")
	build.Dir = simDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build simulator: %v\n%s", err, out)
	}

	// Build eval-arithmetic binary
	evalDir, _ := filepath.Abs("../../testdata/eval-arithmetic")
	evalBinaryPath = filepath.Join(evalDir, "eval-arithmetic")
	evalBuild := exec.Command("go", "build", "-o", evalBinaryPath, ".")
	evalBuild.Dir = evalDir
	evalBuild.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOWORK=off")
	if out, err := evalBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic: %v\n%s", err, out)
	}

	// Build Docker image for eval-arithmetic
	evalImageName = "sockerless-eval-arithmetic:test"
	dockerfile := fmt.Sprintf("FROM alpine:latest\nCOPY %s /usr/local/bin/eval-arithmetic\nENTRYPOINT [\"/usr/local/bin/eval-arithmetic\"]\n", "eval-arithmetic")
	dockerBuild := exec.Command("docker", "build", "-t", evalImageName, "-f", "-", evalDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic Docker image: %v\n%s", err, out)
	}

	// Build lambda-runtime-handler binary + Docker image for
	// Runtime API Invoke tests.
	lambdaHandlerDir, _ := filepath.Abs("../../testdata/lambda-runtime-handler")
	lambdaHandlerBinaryPath = filepath.Join(lambdaHandlerDir, "lambda-runtime-handler")
	lhBuild := exec.Command("go", "build", "-o", lambdaHandlerBinaryPath, ".")
	lhBuild.Dir = lambdaHandlerDir
	lhBuild.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOWORK=off")
	if out, err := lhBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build lambda-runtime-handler: %v\n%s", err, out)
	}
	lambdaHandlerImageName = "sockerless-lambda-runtime-handler:test"
	lhDockerfile := "FROM alpine:latest\nCOPY lambda-runtime-handler /usr/local/bin/lambda-runtime-handler\nENTRYPOINT [\"/usr/local/bin/lambda-runtime-handler\"]\n"
	lhDockerBuild := exec.Command("docker", "build", "-t", lambdaHandlerImageName, "-f", "-", lambdaHandlerDir)
	lhDockerBuild.Stdin = strings.NewReader(lhDockerfile)
	if out, err := lhDockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build lambda-runtime-handler Docker image: %v\n%s", err, out)
	}

	// Find free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Start simulator
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

	// Create tmp dir for test files
	tmpDir, _ = filepath.Abs("tmp")
	os.MkdirAll(tmpDir, 0755)

	code := m.Run()

	simCmd.Process.Kill()
	simCmd.Wait()
	os.RemoveAll(tmpDir)
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

func awsCLI(args ...string) *exec.Cmd {
	cmd := exec.Command("aws", args...)
	cmd.Env = append(os.Environ(),
		"AWS_ENDPOINT_URL="+baseURL,
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_PAGER=",
	)
	return cmd
}

func awsS3CLI(args ...string) *exec.Cmd {
	// S3 routes are under /s3/ prefix in the simulator
	cmd := exec.Command("aws", args...)
	cmd.Env = append(os.Environ(),
		"AWS_ENDPOINT_URL="+baseURL+"/s3",
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_PAGER=",
	)
	return cmd
}

func runCLI(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI command failed: %v\nCommand: %s\nOutput: %s", err, strings.Join(cmd.Args, " "), string(out))
	}
	return string(out)
}

func parseJSON(t *testing.T, data string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), target); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nData: %s", err, data)
	}
}
