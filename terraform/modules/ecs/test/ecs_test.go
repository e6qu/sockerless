package test

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
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	simURL     string
	simCmd     *exec.Cmd
	binaryPath string
)

func TestMain(m *testing.M) {
	// Build the AWS simulator from its pinned sockerless-cloud module (the
	// tests module pins the version via the tool directives in tests/go.mod).
	testsDir, err := filepath.Abs("../../../../tests")
	if err != nil {
		log.Fatalf("Failed to resolve tests module dir: %v", err)
	}

	binaryPath = filepath.Join(testsDir, ".build", "simulator-aws-tf-test")
	if err := os.MkdirAll(filepath.Join(testsDir, ".build"), 0o755); err != nil {
		log.Fatalf("Failed to create tests/.build: %v", err)
	}
	build := exec.Command("go", "build", "-tags", "noui", "-o", binaryPath,
		"github.com/e6qu/sockerless-cloud/simulator-aws")
	build.Dir = testsDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("Failed to build simulator: %v\n%s", err, out)
	}

	// Find a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// The Route 53 DNS listener gets its own free port: the default :5353 is
	// mDNS, which desktop browsers hold open, and a simulator that cannot
	// bind it refuses to start.
	dnsConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Failed to find free DNS port: %v", err)
	}
	dnsPort := dnsConn.LocalAddr().(*net.UDPAddr).Port
	dnsConn.Close()

	// Start the simulator
	simCmd = exec.Command(binaryPath)
	simCmd.Env = append(os.Environ(),
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
		fmt.Sprintf("SIM_DNS_PORT=%d", dnsPort))
	simCmd.Stdout = os.Stderr
	simCmd.Stderr = os.Stderr
	if err := simCmd.Start(); err != nil {
		log.Fatalf("Failed to start simulator: %v", err)
	}

	simURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for health
	if err := waitForHealth(simURL + "/health"); err != nil {
		simCmd.Process.Kill()
		log.Fatalf("Simulator did not become healthy: %v", err)
	}

	code := m.Run()

	simCmd.Process.Kill()
	simCmd.Wait()
	os.Remove(binaryPath)
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

// tfDir returns the absolute path to a temporary directory containing a
// Terraform root module that references the ECS module under test.
func setupTerraformDir(t *testing.T) string {
	t.Helper()

	moduleDir, err := filepath.Abs("..")
	require.NoError(t, err)

	tmpDir := t.TempDir()

	// Write a root module that calls our ECS module
	rootTF := fmt.Sprintf(`
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    ec2              = "%s"
    ecs              = "%s"
    efs              = "%s"
    iam              = "%s"
    cloudwatchlogs   = "%s"
    servicediscovery = "%s"
    ecr              = "%s"
    sts              = "%s"
    s3               = "%s"
  }
}

module "ecs" {
  source      = "%s"
  environment = "tftest"
}
`, simURL, simURL, simURL, simURL, simURL, simURL, simURL, simURL, simURL, moduleDir)

	err = os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(rootTF), 0644)
	require.NoError(t, err)

	return tmpDir
}

func runTerraform(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"AWS_ENDPOINT_URL="+simURL,
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
		"TF_LOG=", // disable verbose logging unless debugging
	)
	// Own process group so the watchdog below can reap terraform + its
	// provider-plugin grandchildren with one kill(-pgid). Without this, a
	// timed-out command leaves orphaned, spinning plugins that starve later
	// runs into cascading timeouts.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	require.NoError(t, cmd.Start(), "terraform %s: failed to start", strings.Join(args, " "))

	killGroup := func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	t.Cleanup(killGroup)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var watchdog <-chan time.Time
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) - 20*time.Second; d > 0 {
			timer := time.NewTimer(d)
			defer timer.Stop()
			watchdog = timer.C
		}
	}

	select {
	case err := <-done:
		return buf.String(), err
	case <-watchdog:
		killGroup()
		<-done
		return buf.String(), fmt.Errorf("terraform %s timed out near the test deadline", strings.Join(args, " "))
	}
}

func TestECSModule_Validate(t *testing.T) {
	tfDir := setupTerraformDir(t)

	// terraform init
	out, err := runTerraform(t, tfDir, "init", "-no-color")
	require.NoError(t, err, "terraform init failed:\n%s", out)

	// terraform validate
	out, err = runTerraform(t, tfDir, "validate", "-no-color")
	require.NoError(t, err, "terraform validate failed:\n%s", out)
	assert.Contains(t, out, "Success")
}

func TestECSModule_Plan(t *testing.T) {
	tfDir := setupTerraformDir(t)

	// terraform init
	out, err := runTerraform(t, tfDir, "init", "-no-color")
	require.NoError(t, err, "terraform init failed:\n%s", out)

	// terraform plan with JSON output for machine-readable verification
	planFile := filepath.Join(tfDir, "tfplan")
	out, err = runTerraform(t, tfDir, "plan", "-no-color", "-out="+planFile)
	require.NoError(t, err, "terraform plan failed:\n%s", out)

	// Verify key resources appear in the plan
	assert.Contains(t, out, "aws_ecs_cluster")
	assert.Contains(t, out, "aws_efs_file_system")
	assert.Contains(t, out, "aws_cloudwatch_log_group")
	assert.Contains(t, out, "aws_iam_role")
	assert.Contains(t, out, "aws_ecr_repository")

	// terraform show -json for structured verification
	showOut, err := runTerraform(t, tfDir, "show", "-json", planFile)
	require.NoError(t, err, "terraform show failed:\n%s", showOut)

	var plan map[string]interface{}
	err = json.Unmarshal([]byte(showOut), &plan)
	require.NoError(t, err, "failed to parse plan JSON")

	// Verify planned resource types
	changes, ok := plan["resource_changes"].([]interface{})
	require.True(t, ok, "plan should have resource_changes")

	resourceTypes := map[string]bool{}
	for _, change := range changes {
		rc := change.(map[string]interface{})
		resourceTypes[rc["type"].(string)] = true
	}

	expectedTypes := []string{
		"aws_vpc",
		"aws_ecs_cluster",
		"aws_efs_file_system",
		"aws_cloudwatch_log_group",
		"aws_iam_role",
		"aws_ecr_repository",
		"aws_service_discovery_private_dns_namespace",
	}
	for _, rt := range expectedTypes {
		assert.True(t, resourceTypes[rt], "plan should include resource type %s, got: %v", rt, keysOf(resourceTypes))
	}
}

func TestECSModule_PlanResourceCount(t *testing.T) {
	tfDir := setupTerraformDir(t)

	out, err := runTerraform(t, tfDir, "init", "-no-color")
	require.NoError(t, err, "terraform init failed:\n%s", out)

	planFile := filepath.Join(tfDir, "tfplan")
	out, err = runTerraform(t, tfDir, "plan", "-no-color", "-out="+planFile)
	require.NoError(t, err, "terraform plan failed:\n%s", out)

	// Parse the "Plan: N to add" line
	lines := strings.Split(out, "\n")
	var planLine string
	for _, line := range lines {
		if strings.Contains(line, "Plan:") && strings.Contains(line, "to add") {
			planLine = line
			break
		}
	}
	require.NotEmpty(t, planLine, "should find 'Plan: N to add' in output:\n%s", out)

	// The ECS module creates a substantial number of resources:
	// VPC, subnets, IGW, NAT, route tables, ECS cluster, EFS, CW logs,
	// Cloud Map, ECR, IAM roles, security groups, etc.
	// We just verify it plans to create *something* reasonable.
	// Use a regex match that requires the literal "0 to add" with a
	// word boundary on the left so "40 to add" doesn't trigger a
	// false positive.
	assert.Contains(t, planLine, "to add")
	assert.NotRegexp(t, `(^|\s)0 to add`, planLine)
}

func keysOf(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
