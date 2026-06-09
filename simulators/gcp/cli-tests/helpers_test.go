package gcp_cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	baseURL       string
	simCmd        *exec.Cmd
	binaryPath    string
	evalImageName string
	tmpDir        string

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

	// Find free ports for HTTP and gRPC
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free gRPC port: %v", err)
	}
	grpcPort := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()

	// Start simulator
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
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_APIGATEWAY="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_API_GATEWAY="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDBUILD="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDRESOURCEMANAGER="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_IAM="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_PUBSUB="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_LOGGING="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDFUNCTIONS="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_SERVICEUSAGE="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_SECRETMANAGER="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDKMS="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_VPCACCESS="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_COMPUTE="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_ARTIFACTREGISTRY="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_EVENTARC="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_STORAGE="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_BIGQUERY="+baseURL+"/bigquery/v2/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_FIRESTORE="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_REDIS="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_SQL="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_SPANNER="+baseURL+"/spanner/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_DATAFLOW="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_BIGTABLE="+baseURL+"/",
		"CLOUDSDK_API_ENDPOINT_OVERRIDES_BIGTABLEADMIN="+baseURL+"/",
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

	const perCmdTimeout = 30 * time.Second

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	if err := cmd.Start(); err != nil {
		t.Fatalf("CLI command failed to start: %v\nCommand: %s", err, strings.Join(cmd.Args, " "))
	}

	// Kill the process if it has not exited within the per-command budget.
	// This prevents a single hanging gcloud call from consuming the whole
	// suite timeout and masking the actual failure in the error message.
	timer := time.AfterFunc(perCmdTimeout, func() {
		_ = cmd.Process.Kill()
	})
	defer timer.Stop()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("CLI command failed: %v\nCommand: %s\nOutput: %s", err, strings.Join(cmd.Args, " "), combined.String())
	}
	return combined.String()
}

func nativeDockerPlatform() string {
	return "linux/" + runtime.GOARCH
}

func buildGoScratchImage(imageName, sourceDir, binaryName, platform string) {
	buildDir, err := os.MkdirTemp("", "sockerless-gcp-image-*")
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

func parseJSON(t *testing.T, data string, target any) {
	t.Helper()
	// gcloud may prefix JSON with status text (e.g. "Created [URL].\n").
	// Try each plausible JSON delimiter until the remaining output decodes.
	for i, r := range data {
		if r != '[' && r != '{' {
			continue
		}
		if err := json.Unmarshal([]byte(data[i:]), target); err == nil {
			return
		}
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nData: %s", err, data)
	}
}
