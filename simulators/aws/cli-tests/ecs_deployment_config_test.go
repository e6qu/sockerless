package aws_cli_test

import (
	"strings"
	"testing"
)

// TestECSCLI_DeploymentConfiguration drives the deployment-configuration
// round-trip via the aws CLI: create a service with a circuit breaker, then
// read it back from describe-services.
func TestECSCLI_DeploymentConfiguration(t *testing.T) {
	cluster := "cli-deploycfg-cluster"
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", cluster))
	runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-deploycfg-task",
		"--container-definitions", `[{"name":"app","image":"`+containerCommandImage+`","command":["hold"],"memory":128}]`))
	runCLI(t, awsCLI("ecs", "create-service",
		"--cluster", cluster, "--service-name", "cli-deploycfg-svc",
		"--task-definition", "cli-deploycfg-task:1", "--desired-count", "1",
		"--deployment-configuration", "deploymentCircuitBreaker={enable=true,rollback=true},maximumPercent=200,minimumHealthyPercent=100"))
	cleanupCLIService(t, cluster, "cli-deploycfg-svc")

	out := strings.TrimSpace(runCLI(t, awsCLI("ecs", "describe-services",
		"--cluster", cluster, "--services", "cli-deploycfg-svc",
		"--query", "services[0].deploymentConfiguration.{CB:deploymentCircuitBreaker.enable,Max:maximumPercent}",
		"--output", "text")))
	if !strings.Contains(strings.ToLower(out), "true") || !strings.Contains(out, "200") {
		t.Fatalf("deploymentConfiguration did not round-trip: %q", out)
	}
}
