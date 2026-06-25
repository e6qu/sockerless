package azure_cli_test

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
	baseURL          string
	simCmd           *exec.Cmd
	binaryPath       string
	evalImageName    string
	commandImageName string
	tmpDir           string

	subscriptionID = "00000000-0000-0000-0000-000000000001"
	resourceGroup  = "cli-test-rg"
)

func TestMain(m *testing.M) {
	// Check if az CLI is installed
	if _, err := exec.LookPath("az"); err != nil {
		fmt.Println("az CLI not found, skipping CLI tests")
		os.Exit(0)
	}

	// Build simulator
	binaryPath, _ = filepath.Abs("../simulator-azure")
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

	commandDir, _ := filepath.Abs("../../testdata/container-command")
	commandImageName = "sockerless-container-command:test"
	buildGoScratchImage(commandImageName, commandDir, "container-command", workloadPlatform)

	// Find free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Start simulator
	simCmd = exec.Command(binaryPath)
	advertisedEndpoints := fmt.Sprintf(
		`{"storage":{"blob":"http://{account}.blob.cli-shim.localhost:%d/","file":"http://{account}.file.cli-shim.localhost:%d/","queue":"http://{account}.queue.cli-shim.localhost:%d/","table":"http://{account}.table.cli-shim.localhost:%d/"},"keyVault":"https://{vault}.vault.cli-shim.localhost:%d/","serviceBus":"https://{namespace}.servicebus.cli-shim.localhost:%d/"}`,
		port, port, port, port, port, port)
	simCmd.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
		"SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON="+advertisedEndpoints,
	)
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

	// Create tmp dir
	tmpDir, _ = filepath.Abs("tmp")
	os.MkdirAll(tmpDir, 0755)

	// Create resource group (needed by most tests)
	rgURL := fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s?api-version=2021-04-01",
		baseURL, subscriptionID, resourceGroup)
	cmd := azRest("PUT", rgURL, `{"location":"eastus"}`)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Failed to create resource group: %v\n%s", err, out)
	}

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

// azRest creates an "az rest" command with config isolation.
// Uses az rest to bypass cloud registration issues with HTTP endpoints.
// Extra args are appended verbatim — used by callers that need --headers.
func azRest(method, url, body string, extra ...string) *exec.Cmd {
	args := []string{"rest", "--method", method, "--url", url, "--output", "json"}
	if body != "" {
		args = append(args, "--body", body)
	}
	args = append(args, extra...)
	cmd := exec.Command("az", args...)
	cmd.Env = append(os.Environ(),
		"AZURE_CONFIG_DIR="+filepath.Join(tmpDir, "azure-config"),
		"AZURE_CORE_NO_COLOR=1",
	)
	return cmd
}

// armURL constructs the full ARM resource URL
func armURL(provider, resourcePath, apiVersion string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/%s/%s?api-version=%s",
		baseURL, subscriptionID, resourceGroup, provider, resourcePath, apiVersion)
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
		t.Fatalf("CLI command failed: %v\nCommand: %s\nOutput: %s", err, strings.Join(cmd.Args, " "), combined.String())
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
	buildDir, err := os.MkdirTemp("", "sockerless-azure-image-*")
	if err != nil {
		log.Fatalf("Failed to create image build dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	binaryPath := filepath.Join(buildDir, binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = sourceDir
	build.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOWORK=off",
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
	)
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s workload binary: %v\n%s", binaryName, err, out)
	}

	dockerfile := fmt.Sprintf(`FROM scratch
COPY %s /usr/local/bin/%s
ENTRYPOINT ["/usr/local/bin/%s"]
`, binaryName, binaryName, binaryName)
	dockerBuild := exec.Command("docker", "build",
		"--platform", platform,
		"-t", imageName,
		"-f", "-", buildDir)
	dockerBuild.Stdin = strings.NewReader(dockerfile)
	if out, err := dockerBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build %s Docker image: %v\n%s", imageName, err, out)
	}
}

func waitForCLIJSON(t *testing.T, url string, ready func(string) bool) string {
	t.Helper()
	// Generous deadline: each `az rest` poll pays Python startup cost, so a tight
	// window allows only a few attempts and races on a loaded CI runner.
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for {
		out, err := azRest("GET", url, "").CombinedOutput()
		last = string(out)
		if err == nil && ready(last) {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for CLI resource state at %s; last output: %s", url, last)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
