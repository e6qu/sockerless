package test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var simulatorURL string
var simulator *exec.Cmd

func TestMain(m *testing.M) {
	simDir, err := filepath.Abs("../../../simulators/aws")
	if err != nil {
		panic(err)
	}
	binary := filepath.Join(os.TempDir(), "simulator-aws-bleephub-ecs-test")
	build := exec.Command("go", "build", "-tags", "noui", "-o", binary, ".")
	build.Dir = simDir
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOCACHE=/private/tmp/sockerless-go-cache")
	if out, err := build.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("build AWS simulator: %v\n%s", err, out))
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	simulator = exec.Command(binary)
	simulator.Env = append(os.Environ(), fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port))
	simulator.Stdout, simulator.Stderr = io.Discard, io.Discard
	if err := simulator.Start(); err != nil {
		panic(err)
	}
	simulatorURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		response, err := http.Get(simulatorURL + "/health") // #nosec G107 -- test-local simulator coordinate
		if err == nil && response.StatusCode == http.StatusOK {
			_ = response.Body.Close()
			break
		}
		if response != nil {
			_ = response.Body.Close()
		}
	}
	code := m.Run()
	_ = simulator.Process.Kill()
	_, _ = simulator.Process.Wait()
	_ = os.Remove(binary)
	os.Exit(code)
}

func TestBleephubECSApplyDestroy(t *testing.T) {
	moduleDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	wakeZip := filepath.Join(t.TempDir(), "wake.zip")
	buildWake(t, wakeZip)
	startupPage := filepath.Join(t.TempDir(), "startup.html")
	buildStartupPage(t, startupPage)
	dir := t.TempDir()
	root := fmt.Sprintf(`
terraform {
  required_providers { aws = { source = "hashicorp/aws", version = "~> 6.0" } }
}
provider "aws" {
  region = "eu-west-1"
  access_key = "test"
  secret_key = "test"
  skip_credentials_validation = true
  skip_metadata_api_check = true
  skip_requesting_account_id = true
  endpoints {
    acm = "%[1]s"
    apigatewayv2 = "%[1]s"
    autoscaling = "%[1]s"
    cloudwatch = "%[1]s"
    cloudwatchlogs = "%[1]s"
    ec2 = "%[1]s"
    ecs = "%[1]s"
    efs = "%[1]s"
    elasticloadbalancing = "%[1]s"
    iam = "%[1]s"
    lambda = "%[1]s"
    route53 = "%[1]s"
    scheduler = "%[1]s"
    s3 = "%[1]s"
    secretsmanager = "%[1]s"
    ssm = "%[1]s"
    sts = "%[1]s"
  }
}
resource "aws_route53_zone" "bleephub" {
  name = "bleephub.example.test"
}
module "bleephub" {
  source = %q
  name = "bleephub-test"
  region = "eu-west-1"
  availability_zones = ["eu-west-1a", "eu-west-1b"]
  hosted_zone_id = aws_route53_zone.bleephub.zone_id
  domain_name = "bleephub.example.test"
  container_image = "public.ecr.aws/docker/library/alpine:3.20"
  admin_token = "test-administrator-token"
  wake_listener_zip_path = %q
  startup_page_path = %q
}
`, simulatorURL, moduleDir, wakeZip, startupPage)
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(root), 0o600); err != nil {
		t.Fatal(err)
	}
	runTerraform(t, dir, "init", "-backend=false")
	runTerraform(t, dir, "apply", "-auto-approve")
	idleShutdown := runTerraformOutput(t, dir, "state", "show", "module.bleephub.aws_lambda_function.idle_shutdown")
	if !strings.Contains(idleShutdown, "timeout                         = 300") {
		t.Fatalf("idle shutdown Lambda did not retain the 300-second control-plane timeout:\n%s", idleShutdown)
	}
	wake := runTerraformOutput(t, dir, "state", "show", "module.bleephub.aws_lambda_function.wake")
	if !strings.Contains(wake, "ADMIN_TOKEN_SECRET_ARN") {
		t.Fatalf("wake Lambda did not receive the administrator startup-dashboard credential coordinate:\n%s", wake)
	}
	startup := runTerraformOutput(t, dir, "state", "show", "module.bleephub.aws_s3_object.startup_page")
	if !strings.Contains(startup, "startup/index.html") || !strings.Contains(startup, "text/html; charset=utf-8") {
		t.Fatalf("S3 startup document was not uploaded with its explicit browser content type:\n%s", startup)
	}
	runTerraform(t, dir, "plan", "-detailed-exitcode")
	runTerraform(t, dir, "destroy", "-auto-approve")
}

func buildStartupPage(t *testing.T, destination string) {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(repo, "scripts", "build-bleephub-startup.sh"), filepath.Dir(destination))
	command.Env = append(os.Environ(), "BLEEPHUB_VERSION=test-build", "BLEEPHUB_PUBLISHED_AT=2026-07-14T00:00:00Z")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build startup page: %v\n%s", err, out)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(destination), "startup", "index.html"), destination); err != nil {
		t.Fatal(err)
	}
}

func buildWake(t *testing.T, destination string) {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(repo, "scripts", "build-bleephub-wake.sh"), filepath.Dir(destination))
	command.Env = append(os.Environ(), "GOCACHE=/private/tmp/sockerless-go-cache")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build wake listener: %v\n%s", err, out)
	}
	if err := os.Rename(filepath.Join(filepath.Dir(destination), "bleephub-wake.zip"), destination); err != nil {
		t.Fatal(err)
	}
}

func runTerraform(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	_ = runTerraformOutput(t, directory, arguments...)
}

func runTerraformOutput(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("terraform", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "AWS_ENDPOINT_URL="+simulatorURL, "AWS_ACCESS_KEY_ID=test", "AWS_SECRET_ACCESS_KEY=test", "AWS_DEFAULT_REGION=eu-west-1", "TF_LOG=")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("terraform %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
