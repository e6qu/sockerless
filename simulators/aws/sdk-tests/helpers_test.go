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
	"runtime"
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
	containerCommandImage  string // Docker image containing container-command binary
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

	workloadPlatform := nativeDockerPlatform()

	evalDir, _ := filepath.Abs("../../testdata/eval-arithmetic")
	evalImageName = "sockerless-eval-arithmetic:test"
	buildGoScratchImage(evalImageName, evalDir, "eval-arithmetic", workloadPlatform)

	lambdaHandlerDir, _ := filepath.Abs("../../testdata/lambda-runtime-handler")
	lambdaHandlerImageName = "sockerless-lambda-runtime-handler:test"
	buildGoScratchImage(lambdaHandlerImageName, lambdaHandlerDir, "lambda-runtime-handler", workloadPlatform)

	containerCommandDir, _ := filepath.Abs("../../testdata/container-command")
	containerCommandImage = "sockerless-container-command:test"
	buildGoScratchImage(containerCommandImage, containerCommandDir, "container-command", workloadPlatform)

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

func nativeDockerPlatform() string {
	return "linux/" + runtime.GOARCH
}

func buildGoScratchImage(imageName, sourceDir, binaryName, platform string) {
	buildDir, err := os.MkdirTemp("", "sockerless-aws-image-*")
	if err != nil {
		log.Fatalf("Failed to create image build dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	binaryPath := filepath.Join(buildDir, binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = sourceDir
	build.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
	)
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s binary: %v\n%s", binaryName, err, out)
	}

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /usr/local/bin/%s
ENTRYPOINT ["/usr/local/bin/%s"]
`, binaryName, binaryName, binaryName)
	dockerBuild := exec.Command("docker", "build",
		"--platform", platform,
		"-t", imageName, "-f", "-", buildDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s Docker image: %v\n%s", binaryName, err, out)
	}
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
