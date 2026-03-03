package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BootstrapSimulator performs cloud-specific setup after simulator starts.
// ECS requires CreateCluster; all others are no-ops.
func BootstrapSimulator(cloud CloudType, backend BackendType, simAddr, projectName string, client *http.Client) error {
	if backend != BackendECS {
		return nil
	}

	// ECS: CreateCluster
	clusterName := projectName + "-cluster"
	payload := map[string]string{"clusterName": clusterName}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cluster request: %w", err)
	}

	req, err := http.NewRequest("POST", simAddr+"/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create cluster request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerServiceV20141113.CreateCluster")

	c := client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("create cluster: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("create cluster returned %d: %s", resp.StatusCode, string(errBody))
	}
	return nil
}

// SimulatorEnv returns environment variables for starting a simulator.
func SimulatorEnv(cloud CloudType, port int, logLevel string) []string {
	env := []string{
		fmt.Sprintf("SIM_LISTEN_ADDR=:%d", port),
	}
	if logLevel != "" {
		env = append(env, "SIM_LOG_LEVEL="+logLevel)
	}
	return env
}

// BackendEnv returns environment variables for a backend connecting to a simulator.
func BackendEnv(cloud CloudType, backend BackendType, simPort int, projectName string) []string {
	endpoint := fmt.Sprintf("http://localhost:%d", simPort)
	env := []string{
		"SOCKERLESS_ENDPOINT_URL=" + endpoint,
	}

	switch backend {
	case BackendECS:
		env = append(env,
			"AWS_REGION=us-east-1",
			"SOCKERLESS_ECS_CLUSTER="+projectName+"-cluster",
			"SOCKERLESS_ECS_SUBNETS=subnet-sim",
			"SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::0:role/sim",
		)
	case BackendLambda:
		env = append(env,
			"AWS_REGION=us-east-1",
			"SOCKERLESS_LAMBDA_ROLE_ARN=arn:aws:iam::0:role/sim",
		)
	case BackendCloudRun:
		env = append(env,
			"SOCKERLESS_GCR_PROJECT=sim-project",
		)
	case BackendGCF:
		env = append(env,
			"SOCKERLESS_GCF_PROJECT=sim-project",
		)
	case BackendACA:
		env = append(env,
			"SOCKERLESS_ACA_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000",
			"SOCKERLESS_ACA_RESOURCE_GROUP=sim-rg",
			"SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE=default",
		)
	case BackendAZF:
		env = append(env,
			"SOCKERLESS_AZF_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000",
			"SOCKERLESS_AZF_RESOURCE_GROUP=sim-rg",
			"SOCKERLESS_AZF_STORAGE_ACCOUNT=simstorage",
		)
	}

	return env
}

// BackendArgs returns command-line arguments for starting a backend.
func BackendArgs(port int, logLevel string) []string {
	args := []string{"-addr", fmt.Sprintf(":%d", port)}
	if logLevel != "" {
		args = append(args, "-log-level", logLevel)
	}
	return args
}

// FrontendArgs returns command-line arguments for starting the Docker frontend.
func FrontendArgs(frontendPort, backendPort, mgmtPort int, logLevel string) []string {
	args := []string{
		"-addr", fmt.Sprintf(":%d", frontendPort),
		"-backend", fmt.Sprintf("http://localhost:%d", backendPort),
		"-mgmt-addr", fmt.Sprintf(":%d", mgmtPort),
	}
	if logLevel != "" {
		args = append(args, "-log-level", logLevel)
	}
	return args
}
