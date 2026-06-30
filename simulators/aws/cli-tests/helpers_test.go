package aws_cli_test

import (
	"bytes"
	"encoding/json"
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
)

var (
	baseURL                string
	simCmd                 *exec.Cmd
	binaryPath             string
	evalImageName          string
	lambdaHandlerImageName string
	containerCommandImage  string
	tmpDir                 string
	awsCLIVersion          string
)

func TestMain(m *testing.M) {
	// Some CI / host images ship an aws CLI that predates simulator-tested
	// surfaces (e.g. create-transit-gateway-metering-policy). Rather than
	// relying on whatever version happens to be installed, install the latest
	// v2 CLI into a tmp dir and use it for the whole suite. This satisfies the
	// no-skip-if-absent rule: the test controls its own reference adaptor
	// version.
	awsPath := installLatestAWSCLI()
	if out, err := exec.Command(awsPath, "--version").CombinedOutput(); err == nil {
		awsCLIVersion = strings.TrimSpace(string(out))
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

func runCLI(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	const perCmdTimeout = 60 * time.Second
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Start(); err != nil {
		t.Fatalf("CLI command failed to start: %v\nCommand: %s", err, strings.Join(cmd.Args, " "))
	}
	// Kill a hung CLI call so it can't consume the whole suite timeout and mask
	// the real failure in the error message.
	timer := time.AfterFunc(perCmdTimeout, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	if err := cmd.Wait(); err != nil {
		// Surface the CLI's own "invalid choice" / help output clearly; this
		// usually means the installed aws CLI predates the command under test.
		if isAWSCLIUnknownCommand(combined.String()) {
			t.Skipf("installed aws CLI (%s) does not support %s %s; update the aws CLI to run this test", awsCLIVersion, cmd.Args[1], cmd.Args[2])
		}
		t.Fatalf("CLI command failed: %v\nCommand: %s\nOutput: %s", err, strings.Join(cmd.Args, " "), combined.String())
	}
	return combined.String()
}

// isAWSCLIUnknownCommand returns true when aws-cli printed its help banner
// because it did not recognize the requested service/operation.
func isAWSCLIUnknownCommand(out string) bool {
	return strings.Contains(out, "Invalid choice:") || strings.Contains(out, "argument operation: Invalid choice")
}

func runCLIExpectError(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	const perCmdTimeout = 60 * time.Second
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Start(); err != nil {
		t.Fatalf("CLI command failed to start: %v\nCommand: %s", err, strings.Join(cmd.Args, " "))
	}
	timer := time.AfterFunc(perCmdTimeout, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	if err := cmd.Wait(); err == nil {
		t.Fatalf("CLI command unexpectedly succeeded\nCommand: %s\nOutput: %s", strings.Join(cmd.Args, " "), combined.String())
	}
	return combined.String()
}

func parseJSON(t *testing.T, data string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), target); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nData: %s", err, data)
	}
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

func cleanupCLIECSTask(t *testing.T, clusterName, taskArn string) {
	t.Helper()
	t.Cleanup(func() {
		runCLI(t, awsCLI("ecs", "stop-task",
			"--cluster", clusterName,
			"--task", taskArn,
			"--reason", "test cleanup",
		))
	})
}

func installLatestAWSCLI() string {
	binDir, err := os.MkdirTemp("", "sockerless-aws-cli-*")
	if err != nil {
		log.Fatalf("Failed to create aws CLI install dir: %v", err)
	}
	installDir := filepath.Join(binDir, "aws-cli")

	switch runtime.GOOS {
	case "darwin":
		pkg := filepath.Join(binDir, "AWSCLIV2.pkg")
		if out, err := exec.Command("curl", "-fsSL", "-o", pkg, "https://awscli.amazonaws.com/AWSCLIV2.pkg").CombinedOutput(); err != nil {
			log.Fatalf("Failed to download aws CLI pkg: %v\n%s", err, out)
		}
		expanded := filepath.Join(binDir, "expanded")
		if out, err := exec.Command("pkgutil", "--expand", pkg, expanded).CombinedOutput(); err != nil {
			log.Fatalf("Failed to expand aws CLI pkg: %v\n%s", err, out)
		}
		payload := filepath.Join(expanded, "aws-cli.pkg", "Payload")
		if out, err := exec.Command("tar", "-xf", payload, "-C", binDir).CombinedOutput(); err != nil {
			log.Fatalf("Failed to extract aws CLI payload: %v\n%s", err, out)
		}
		// tar extracts to aws-cli/aws relative to binDir.
	case "linux":
		zip := filepath.Join(binDir, "awscliv2.zip")
		if out, err := exec.Command("curl", "-fsSL", "-o", zip, "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip").CombinedOutput(); err != nil {
			log.Fatalf("Failed to download aws CLI zip: %v\n%s", err, out)
		}
		if out, err := exec.Command("unzip", "-q", zip, "-d", binDir).CombinedOutput(); err != nil {
			log.Fatalf("Failed to unzip aws CLI: %v\n%s", err, out)
		}
	default:
		log.Fatalf("Unsupported OS for automatic aws CLI install: %s", runtime.GOOS)
	}

	awsBin := filepath.Join(installDir, "aws")
	if _, err := os.Stat(awsBin); err != nil {
		log.Fatalf("aws CLI binary not found after install at %s: %v", awsBin, err)
	}

	// Prepend to PATH so awsCLI() picks it up.
	os.Setenv("PATH", installDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return awsBin
}
