package aws_cli_test

import (
	"testing"

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
