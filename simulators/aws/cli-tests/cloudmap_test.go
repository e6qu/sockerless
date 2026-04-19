package aws_cli_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudMap_CreateAndListNamespaces(t *testing.T) {
	out := runCLI(t, awsCLI("servicediscovery", "create-private-dns-namespace",
		"--name", "cli-test.local",
		"--vpc", "vpc-12345678",
		"--output", "json",
	))

	var createResult struct {
		OperationId string `json:"OperationId"`
	}
	parseJSON(t, out, &createResult)
	require.NotEmpty(t, createResult.OperationId)

	// List namespaces to find the created one
	out = runCLI(t, awsCLI("servicediscovery", "list-namespaces", "--output", "json"))

	var listResult struct {
		Namespaces []struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
			Type string `json:"Type"`
		} `json:"Namespaces"`
	}
	parseJSON(t, out, &listResult)

	var nsId string
	for _, ns := range listResult.Namespaces {
		if ns.Name == "cli-test.local" {
			nsId = ns.Id
			assert.Equal(t, "DNS_PRIVATE", ns.Type)
		}
	}
	require.NotEmpty(t, nsId, "Expected to find namespace cli-test.local")

	// Cleanup
	runCLI(t, awsCLI("servicediscovery", "delete-namespace", "--id", nsId))
}

func TestCloudMap_CreateService(t *testing.T) {
	// Create namespace first
	runCLI(t, awsCLI("servicediscovery", "create-private-dns-namespace",
		"--name", "svc-test.local",
		"--vpc", "vpc-12345678",
	))

	// Get namespace ID
	out := runCLI(t, awsCLI("servicediscovery", "list-namespaces", "--output", "json"))
	var nsList struct {
		Namespaces []struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"Namespaces"`
	}
	parseJSON(t, out, &nsList)
	var nsId string
	for _, ns := range nsList.Namespaces {
		if ns.Name == "svc-test.local" {
			nsId = ns.Id
		}
	}
	require.NotEmpty(t, nsId)

	// Create service
	out = runCLI(t, awsCLI("servicediscovery", "create-service",
		"--name", "my-service",
		"--namespace-id", nsId,
		"--dns-config", `NamespaceId=`+nsId+`,RoutingPolicy=MULTIVALUE,DnsRecords=[{Type=A,TTL=60}]`,
		"--output", "json",
	))

	var svcResult struct {
		Service struct {
			Id          string `json:"Id"`
			Name        string `json:"Name"`
			NamespaceId string `json:"NamespaceId"`
		} `json:"Service"`
	}
	parseJSON(t, out, &svcResult)
	assert.Equal(t, "my-service", svcResult.Service.Name)
	assert.Equal(t, nsId, svcResult.Service.NamespaceId)

	// Cleanup
	runCLI(t, awsCLI("servicediscovery", "delete-namespace", "--id", nsId))
}

func TestCloudMap_RegisterAndListInstances(t *testing.T) {
	// Setup: namespace + service
	runCLI(t, awsCLI("servicediscovery", "create-private-dns-namespace",
		"--name", "discover-test.local",
		"--vpc", "vpc-12345678",
	))

	out := runCLI(t, awsCLI("servicediscovery", "list-namespaces", "--output", "json"))
	var nsList struct {
		Namespaces []struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"Namespaces"`
	}
	parseJSON(t, out, &nsList)
	var nsId string
	for _, ns := range nsList.Namespaces {
		if ns.Name == "discover-test.local" {
			nsId = ns.Id
		}
	}
	require.NotEmpty(t, nsId)

	out = runCLI(t, awsCLI("servicediscovery", "create-service",
		"--name", "web",
		"--namespace-id", nsId,
		"--output", "json",
	))
	var svcResult struct {
		Service struct {
			Id string `json:"Id"`
		} `json:"Service"`
	}
	parseJSON(t, out, &svcResult)
	svcId := svcResult.Service.Id

	// Register instance
	out = runCLI(t, awsCLI("servicediscovery", "register-instance",
		"--service-id", svcId,
		"--instance-id", "instance-1",
		"--attributes", "AWS_INSTANCE_IPV4=10.0.0.1,AWS_INSTANCE_PORT=8080",
		"--output", "json",
	))
	var regResult struct {
		OperationId string `json:"OperationId"`
	}
	parseJSON(t, out, &regResult)
	require.NotEmpty(t, regResult.OperationId)

	// List instances (discover-instances uses a separate data-plane endpoint
	// with a data- hostname prefix, so we use list-instances instead)
	out = runCLI(t, awsCLI("servicediscovery", "list-instances",
		"--service-id", svcId,
		"--output", "json",
	))

	var listResult struct {
		Instances []struct {
			Id         string            `json:"Id"`
			Attributes map[string]string `json:"Attributes"`
		} `json:"Instances"`
	}
	parseJSON(t, out, &listResult)
	require.Len(t, listResult.Instances, 1)
	assert.Equal(t, "instance-1", listResult.Instances[0].Id)
	assert.Equal(t, "10.0.0.1", listResult.Instances[0].Attributes["AWS_INSTANCE_IPV4"])

	// Cleanup
	runCLI(t, awsCLI("servicediscovery", "deregister-instance",
		"--service-id", svcId,
		"--instance-id", "instance-1",
	))
	runCLI(t, awsCLI("servicediscovery", "delete-namespace", "--id", nsId))
}

// TestCloudMap_CrossTaskDNS_CLI exercises BUG-701's fix end-to-end
// through the aws CLI — creating a namespace backs a real Docker
// network, running two ECS tasks + registering them in per-hostname
// services connects both containers to that network with DNS aliases,
// and one container can resolve the other's alias via Docker's
// embedded DNS.
func TestCloudMap_CrossTaskDNS_CLI(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI required for cross-task DNS test")
	}

	// Namespace — backs a real Docker network sim-<nsId>
	out := runCLI(t, awsCLI("servicediscovery", "create-private-dns-namespace",
		"--name", "cli-xtask-dns.local",
		"--vpc", "vpc-sim",
		"--output", "json",
	))
	var createNs struct {
		OperationId string `json:"OperationId"`
	}
	parseJSON(t, out, &createNs)

	out = runCLI(t, awsCLI("servicediscovery", "list-namespaces", "--output", "json"))
	var nsList struct {
		Namespaces []struct{ Id, Name string }
	}
	parseJSON(t, out, &nsList)
	var nsId string
	for _, ns := range nsList.Namespaces {
		if ns.Name == "cli-xtask-dns.local" {
			nsId = ns.Id
		}
	}
	require.NotEmpty(t, nsId)

	// Services alpha + beta
	createService := func(name string) string {
		out := runCLI(t, awsCLI("servicediscovery", "create-service",
			"--name", name,
			"--namespace-id", nsId,
			"--dns-config", "NamespaceId="+nsId+",DnsRecords=[{Type=A,TTL=10}]",
			"--output", "json",
		))
		var svcResult struct {
			Service struct{ Id string } `json:"Service"`
		}
		parseJSON(t, out, &svcResult)
		require.NotEmpty(t, svcResult.Service.Id)
		return svcResult.Service.Id
	}
	alphaSvc := createService("alpha")
	betaSvc := createService("beta")

	// Cluster + task def with awslogs config (sim only spawns Docker
	// containers when awslogs is configured) + sleep command so the
	// container stays alive.
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-xtask-dns"))
	containerDef := `[{"name":"app","image":"alpine:latest","entryPoint":["sh","-c"],"command":["sleep 30"],"logConfiguration":{"logDriver":"awslogs","options":{"awslogs-group":"/ecs/cli-xtask-dns","awslogs-stream-prefix":"ecs"}}}]`
	out = runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-xtask-dns-td",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--container-definitions", containerDef,
		"--output", "json",
	))
	var regTd struct {
		TaskDefinition struct{ TaskDefinitionArn string } `json:"taskDefinition"`
	}
	parseJSON(t, out, &regTd)

	runTask := func(cid string) string {
		out := runCLI(t, awsCLI("ecs", "run-task",
			"--cluster", "cli-xtask-dns",
			"--task-definition", regTd.TaskDefinition.TaskDefinitionArn,
			"--count", "1",
			"--launch-type", "FARGATE",
			"--network-configuration", "awsvpcConfiguration={subnets=[subnet-sim]}",
			"--tags", "key=sockerless-container-id,value="+cid,
			"--output", "json",
		))
		var runRes struct {
			Tasks []struct{ TaskArn string } `json:"tasks"`
		}
		parseJSON(t, out, &runRes)
		require.Len(t, runRes.Tasks, 1)
		return runRes.Tasks[0].TaskArn
	}
	alphaCID := strings.Repeat("a", 64)
	betaCID := strings.Repeat("b", 64)
	alphaTask := runTask(alphaCID)
	betaTask := runTask(betaCID)

	// Wait for Docker containers to exist
	alphaName := "sockerless-sim-aws-task-" + alphaTask[strings.LastIndex(alphaTask, "/")+1:][:12]
	betaName := "sockerless-sim-aws-task-" + betaTask[strings.LastIndex(betaTask, "/")+1:][:12]
	require.Eventually(t, func() bool {
		for _, n := range []string{alphaName, betaName} {
			if err := exec.Command("docker", "inspect", n).Run(); err != nil {
				return false
			}
		}
		return true
	}, 30*time.Second, 300*time.Millisecond, "Docker containers should be running")

	// Register instances — sim connects containers to the namespace's
	// Docker network with the service name as alias.
	runCLI(t, awsCLI("servicediscovery", "register-instance",
		"--service-id", alphaSvc,
		"--instance-id", alphaCID[:12],
		"--attributes", "AWS_INSTANCE_IPV4=10.0.0.10",
	))
	runCLI(t, awsCLI("servicediscovery", "register-instance",
		"--service-id", betaSvc,
		"--instance-id", betaCID[:12],
		"--attributes", "AWS_INSTANCE_IPV4=10.0.0.20",
	))

	// Resolve beta from alpha via Docker's embedded DNS
	var getent []byte
	require.Eventually(t, func() bool {
		var err error
		getent, err = exec.Command("docker", "exec", alphaName, "getent", "hosts", "beta").CombinedOutput()
		return err == nil && len(getent) > 0
	}, 10*time.Second, 500*time.Millisecond, "alpha should resolve 'beta' via Cloud Map DNS; last output: %s", getent)
	assert.Contains(t, string(getent), "beta", "getent output should mention beta: %s", getent)

	// Cleanup
	runCLI(t, awsCLI("servicediscovery", "deregister-instance",
		"--service-id", alphaSvc, "--instance-id", alphaCID[:12]))
	runCLI(t, awsCLI("servicediscovery", "deregister-instance",
		"--service-id", betaSvc, "--instance-id", betaCID[:12]))
	runCLI(t, awsCLI("servicediscovery", "delete-service", "--id", alphaSvc))
	runCLI(t, awsCLI("servicediscovery", "delete-service", "--id", betaSvc))
	runCLI(t, awsCLI("servicediscovery", "delete-namespace", "--id", nsId))
	runCLI(t, awsCLI("ecs", "stop-task", "--cluster", "cli-xtask-dns", "--task", alphaTask))
	runCLI(t, awsCLI("ecs", "stop-task", "--cluster", "cli-xtask-dns", "--task", betaTask))
}

func TestCloudMap_DeregisterInstance(t *testing.T) {
	// Setup
	runCLI(t, awsCLI("servicediscovery", "create-private-dns-namespace",
		"--name", "dereg-test.local",
		"--vpc", "vpc-12345678",
	))

	out := runCLI(t, awsCLI("servicediscovery", "list-namespaces", "--output", "json"))
	var nsList struct {
		Namespaces []struct {
			Id   string `json:"Id"`
			Name string `json:"Name"`
		} `json:"Namespaces"`
	}
	parseJSON(t, out, &nsList)
	var nsId string
	for _, ns := range nsList.Namespaces {
		if ns.Name == "dereg-test.local" {
			nsId = ns.Id
		}
	}
	require.NotEmpty(t, nsId)

	out = runCLI(t, awsCLI("servicediscovery", "create-service",
		"--name", "api",
		"--namespace-id", nsId,
		"--output", "json",
	))
	var svcResult struct {
		Service struct {
			Id string `json:"Id"`
		} `json:"Service"`
	}
	parseJSON(t, out, &svcResult)
	svcId := svcResult.Service.Id

	// Register then deregister
	runCLI(t, awsCLI("servicediscovery", "register-instance",
		"--service-id", svcId,
		"--instance-id", "temp-instance",
		"--attributes", "AWS_INSTANCE_IPV4=10.0.0.2",
	))

	runCLI(t, awsCLI("servicediscovery", "deregister-instance",
		"--service-id", svcId,
		"--instance-id", "temp-instance",
	))

	// Verify it's gone
	out = runCLI(t, awsCLI("servicediscovery", "list-instances",
		"--service-id", svcId,
		"--output", "json",
	))

	var listResult struct {
		Instances []struct {
			Id string `json:"Id"`
		} `json:"Instances"`
	}
	parseJSON(t, out, &listResult)
	assert.Empty(t, listResult.Instances)

	// Cleanup
	runCLI(t, awsCLI("servicediscovery", "delete-namespace", "--id", nsId))
}
