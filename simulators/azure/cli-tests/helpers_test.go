package azure_cli_test

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
	baseURL        string
	simCmd         *exec.Cmd
	binaryPath     string
	evalBinaryPath string
	tmpDir         string

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
	build := exec.Command("go", "build", "-o", binaryPath, ".")
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
	evalBuild.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off")
	if out, err := evalBuild.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build eval-arithmetic: %v\n%s", err, out)
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
func azRest(method, url, body string) *exec.Cmd {
	args := []string{"rest", "--method", method, "--url", url, "--output", "json"}
	if body != "" {
		args = append(args, "--body", body)
	}
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

// subURL constructs a subscription-scoped URL
func subURL(provider, resourcePath, apiVersion string) string {
	return fmt.Sprintf("%s/subscriptions/%s/providers/%s/%s?api-version=%s",
		baseURL, subscriptionID, provider, resourcePath, apiVersion)
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
