package gcp_cli_test

import (
	"encoding/json"
	"fmt"
	"io"
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

	project  = "test-project"
	location = "us-central1"
)

func TestMain(m *testing.M) {
	// Check if gcloud CLI is installed
	if _, err := exec.LookPath("gcloud"); err != nil {
		fmt.Println("gcloud CLI not found, skipping CLI tests")
		os.Exit(0)
	}

	// Build simulator
	binaryPath, _ = filepath.Abs("../simulator-gcp")
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

// gcloudCLI creates a gcloud command with config isolation and endpoint overrides.
func gcloudCLI(args ...string) *exec.Cmd {
	cmd := exec.Command("gcloud", args...)
	cmd.Env = append(os.Environ(),
		"CLOUDSDK_CONFIG="+filepath.Join(tmpDir, "gcloud-config"),
		"CLOUDSDK_AUTH_ACCESS_TOKEN=fake-gcp-token",
		"CLOUDSDK_CORE_PROJECT="+project,
		"CLOUDSDK_CORE_DISABLE_PROMPTS=1",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_DNS="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_LOGGING="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDFUNCTIONS="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_SERVICEUSAGE="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_VPCACCESS="+baseURL+"/",
	)
	return cmd
}

// httpDo performs a direct HTTP request to the simulator REST API.
// Used when gcloud commands don't support endpoint overrides well.
func httpDo(method, url string, body string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer fake-gcp-token")
	return http.DefaultClient.Do(req)
}

func httpDoJSON(t *testing.T, method, url, body string) string {
	t.Helper()
	resp, err := httpDo(method, url, body)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return string(data)
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
