package aws_sdk_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cmClient() *servicediscovery.Client {
	cfg := sdkConfig()
	// Use the legacy EndpointResolver with HostnameImmutable=true to prevent
	// the SDK from prepending "data-" to the hostname for DiscoverInstances.
	return servicediscovery.NewFromConfig(cfg, func(o *servicediscovery.Options) {
		o.BaseEndpoint = aws.String(baseURL)
		//nolint:staticcheck // EndpointResolver is deprecated but needed to make hostname immutable
		o.EndpointResolver = servicediscovery.EndpointResolverFromURL(baseURL, func(e *aws.Endpoint) {
			e.HostnameImmutable = true
		})
	})
}

func TestCloudMap_CreateNamespaceAndGetOperation(t *testing.T) {
	client := cmClient()

	// Create a private DNS namespace
	createOut, err := client.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name:        aws.String("test-ns.local"),
		Vpc:         aws.String("vpc-12345"),
		Description: aws.String("test namespace"),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.OperationId)

	// Get operation to retrieve namespace ID
	opOut, err := client.GetOperation(ctx, &servicediscovery.GetOperationInput{
		OperationId: createOut.OperationId,
	})
	require.NoError(t, err)
	require.NotNil(t, opOut.Operation)
	assert.Equal(t, sdtypes.OperationStatusSuccess, opOut.Operation.Status)

	// Extract namespace ID from operation targets
	nsID, ok := opOut.Operation.Targets["NAMESPACE"]
	require.True(t, ok, "operation should have NAMESPACE target")
	assert.NotEmpty(t, nsID)

	// Get the namespace
	nsOut, err := client.GetNamespace(ctx, &servicediscovery.GetNamespaceInput{
		Id: aws.String(nsID),
	})
	require.NoError(t, err)
	require.NotNil(t, nsOut.Namespace)
	assert.Equal(t, "test-ns.local", *nsOut.Namespace.Name)
	assert.Equal(t, sdtypes.NamespaceTypeDnsPrivate, nsOut.Namespace.Type)

	// Cleanup
	_, err = client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{
		Id: aws.String(nsID),
	})
	require.NoError(t, err)
}

func TestCloudMap_CreateServiceAndListWithFilter(t *testing.T) {
	client := cmClient()

	// Create namespace
	createOut, err := client.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name: aws.String("svc-filter-test.local"),
		Vpc:  aws.String("vpc-12345"),
	})
	require.NoError(t, err)

	opOut, err := client.GetOperation(ctx, &servicediscovery.GetOperationInput{
		OperationId: createOut.OperationId,
	})
	require.NoError(t, err)
	nsID := opOut.Operation.Targets["NAMESPACE"]

	// Create two namespaces — second for filtering test
	createOut2, err := client.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name: aws.String("svc-filter-other.local"),
		Vpc:  aws.String("vpc-12345"),
	})
	require.NoError(t, err)
	opOut2, err := client.GetOperation(ctx, &servicediscovery.GetOperationInput{
		OperationId: createOut2.OperationId,
	})
	require.NoError(t, err)
	nsID2 := opOut2.Operation.Targets["NAMESPACE"]

	// Create service in first namespace
	svcOut, err := client.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String("my-service"),
		NamespaceId: aws.String(nsID),
		DnsConfig: &sdtypes.DnsConfig{
			DnsRecords: []sdtypes.DnsRecord{
				{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, svcOut.Service)
	assert.Equal(t, "my-service", *svcOut.Service.Name)
	svcID := *svcOut.Service.Id

	// Create service in second namespace
	svcOut2, err := client.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String("other-service"),
		NamespaceId: aws.String(nsID2),
	})
	require.NoError(t, err)

	// List all services (no filter) — should include both
	listAll, err := client.ListServices(ctx, &servicediscovery.ListServicesInput{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(listAll.Services), 2)

	// List with NAMESPACE_ID filter — should return only first namespace's service
	listFiltered, err := client.ListServices(ctx, &servicediscovery.ListServicesInput{
		Filters: []sdtypes.ServiceFilter{
			{
				Name:      sdtypes.ServiceFilterNameNamespaceId,
				Values:    []string{nsID},
				Condition: sdtypes.FilterConditionEq,
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, listFiltered.Services, 1)
	assert.Equal(t, "my-service", *listFiltered.Services[0].Name)

	// Cleanup
	_, _ = client.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: aws.String(svcID)})
	_, _ = client.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: svcOut2.Service.Id})
	_, _ = client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{Id: aws.String(nsID)})
	_, _ = client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{Id: aws.String(nsID2)})
}

func TestCloudMap_RegisterAndDiscoverInstances(t *testing.T) {
	client := cmClient()

	// Create namespace
	createOut, err := client.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name: aws.String("discover-test.local"),
		Vpc:  aws.String("vpc-12345"),
	})
	require.NoError(t, err)
	opOut, err := client.GetOperation(ctx, &servicediscovery.GetOperationInput{
		OperationId: createOut.OperationId,
	})
	require.NoError(t, err)
	nsID := opOut.Operation.Targets["NAMESPACE"]

	// Create service
	svcOut, err := client.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String("web"),
		NamespaceId: aws.String(nsID),
		DnsConfig: &sdtypes.DnsConfig{
			DnsRecords: []sdtypes.DnsRecord{
				{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)},
			},
		},
	})
	require.NoError(t, err)
	svcID := *svcOut.Service.Id

	// Register two instances
	_, err = client.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
		ServiceId:  aws.String(svcID),
		InstanceId: aws.String("inst-001"),
		Attributes: map[string]string{
			"AWS_INSTANCE_IPV4": "10.0.0.1",
			"HOSTNAME":          "web-1",
		},
	})
	require.NoError(t, err)

	_, err = client.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
		ServiceId:  aws.String(svcID),
		InstanceId: aws.String("inst-002"),
		Attributes: map[string]string{
			"AWS_INSTANCE_IPV4": "10.0.0.2",
			"HOSTNAME":          "web-2",
		},
	})
	require.NoError(t, err)

	// Discover instances
	discoverOut, err := client.DiscoverInstances(ctx, &servicediscovery.DiscoverInstancesInput{
		NamespaceName: aws.String("discover-test.local"),
		ServiceName:   aws.String("web"),
	})
	require.NoError(t, err)
	require.Len(t, discoverOut.Instances, 2)

	// Verify attributes are present
	ips := map[string]bool{}
	for _, inst := range discoverOut.Instances {
		if ip, ok := inst.Attributes["AWS_INSTANCE_IPV4"]; ok {
			ips[ip] = true
		}
	}
	assert.True(t, ips["10.0.0.1"])
	assert.True(t, ips["10.0.0.2"])

	// Deregister one instance
	_, err = client.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
		ServiceId:  aws.String(svcID),
		InstanceId: aws.String("inst-001"),
	})
	require.NoError(t, err)

	// Discover again — should only have one
	discoverOut2, err := client.DiscoverInstances(ctx, &servicediscovery.DiscoverInstancesInput{
		NamespaceName: aws.String("discover-test.local"),
		ServiceName:   aws.String("web"),
	})
	require.NoError(t, err)
	require.Len(t, discoverOut2.Instances, 1)
	assert.Equal(t, "10.0.0.2", discoverOut2.Instances[0].Attributes["AWS_INSTANCE_IPV4"])

	// Cleanup
	_, _ = client.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
		ServiceId:  aws.String(svcID),
		InstanceId: aws.String("inst-002"),
	})
	_, _ = client.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: aws.String(svcID)})
	_, _ = client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{Id: aws.String(nsID)})
}

// TestECS_CrossTaskDNS exercises cross-task DNS: the Cloud Map simulator
// creates a real Docker network backing each private DNS namespace and
// connects each registered instance's ECS task container to it with the
// service name as alias. Two tasks on the same namespace must resolve
// each other by service name via Docker's embedded DNS.
func TestECS_CrossTaskDNS(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI required for cross-task DNS test")
	}

	cm := cmClient()
	ecsCli := ecsClient()

	// Namespace — simulator creates Docker network sim-<nsId>
	createNs, err := cm.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name: aws.String("xtask-dns.local"),
		Vpc:  aws.String("vpc-sim"),
	})
	require.NoError(t, err)
	opOut, err := cm.GetOperation(ctx, &servicediscovery.GetOperationInput{OperationId: createNs.OperationId})
	require.NoError(t, err)
	nsID := opOut.Operation.Targets["NAMESPACE"]

	// Two services — alpha, beta
	svcAlpha, err := cm.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String("alpha"),
		NamespaceId: aws.String(nsID),
		DnsConfig: &sdtypes.DnsConfig{
			DnsRecords: []sdtypes.DnsRecord{{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)}},
		},
	})
	require.NoError(t, err)
	svcBeta, err := cm.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String("beta"),
		NamespaceId: aws.String(nsID),
		DnsConfig: &sdtypes.DnsConfig{
			DnsRecords: []sdtypes.DnsRecord{{Type: sdtypes.RecordTypeA, TTL: aws.Int64(10)}},
		},
	})
	require.NoError(t, err)

	// Cluster + task def running `sleep 30`
	_, err = ecsCli.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: aws.String("xtask-dns")})
	require.NoError(t, err)
	tdOut, err := ecsCli.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("xtask-dns-td"),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:       aws.String("app"),
			Image:      aws.String("alpine:latest"),
			EntryPoint: []string{"sh", "-c"},
			Command:    []string{"sleep 30"},
			LogConfiguration: &ecstypes.LogConfiguration{
				LogDriver: ecstypes.LogDriverAwslogs,
				Options: map[string]string{
					"awslogs-group":         "/ecs/xtask-dns",
					"awslogs-stream-prefix": "ecs",
				},
			},
		}},
	})
	require.NoError(t, err)

	runTask := func(containerID string) string {
		runOut, err := ecsCli.RunTask(ctx, &ecs.RunTaskInput{
			Cluster:        aws.String("xtask-dns"),
			TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
			Count:          aws.Int32(1),
			LaunchType:     ecstypes.LaunchTypeFargate,
			NetworkConfiguration: &ecstypes.NetworkConfiguration{
				AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{Subnets: []string{"subnet-sim"}},
			},
			Tags: []ecstypes.Tag{
				{Key: aws.String("sockerless-container-id"), Value: aws.String(containerID)},
			},
		})
		require.NoError(t, err)
		require.Len(t, runOut.Tasks, 1)
		return *runOut.Tasks[0].TaskArn
	}

	alphaCID := strings.Repeat("a", 64)
	betaCID := strings.Repeat("b", 64)
	alphaTask := runTask(alphaCID)
	betaTask := runTask(betaCID)
	_ = alphaTask

	// Wait for both to reach RUNNING (sim transitions in ~500ms) and
	// ensure Docker containers are up before registering instances.
	require.Eventually(t, func() bool {
		out, _ := ecsCli.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String("xtask-dns"),
			Tasks:   []string{alphaTask, betaTask},
		})
		if len(out.Tasks) < 2 {
			return false
		}
		for _, tk := range out.Tasks {
			if aws.ToString(tk.LastStatus) != "RUNNING" {
				return false
			}
		}
		return true
	}, 15*time.Second, 200*time.Millisecond, "tasks should reach RUNNING")

	// RUNNING in the simulator is set before StartContainerSync returns
	// — wait for the Docker container itself so RegisterInstance can
	// find it and connect it to the namespace's network.
	alphaTaskID := alphaTask[strings.LastIndex(alphaTask, "/")+1:]
	betaTaskID := betaTask[strings.LastIndex(betaTask, "/")+1:]
	alphaContainer := "sockerless-sim-aws-task-" + alphaTaskID[:12]
	betaContainer := "sockerless-sim-aws-task-" + betaTaskID[:12]
	require.Eventually(t, func() bool {
		for _, name := range []string{alphaContainer, betaContainer} {
			if err := exec.Command("docker", "inspect", name).Run(); err != nil {
				return false
			}
		}
		return true
	}, 30*time.Second, 200*time.Millisecond, "Docker containers should exist before RegisterInstance")

	// Register each task as an instance in its service — simulator
	// connects the matching Docker container to the namespace's network.
	_, err = cm.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
		ServiceId:  svcAlpha.Service.Id,
		InstanceId: aws.String(alphaCID[:12]),
		Attributes: map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.10"},
	})
	require.NoError(t, err)
	_, err = cm.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
		ServiceId:  svcBeta.Service.Id,
		InstanceId: aws.String(betaCID[:12]),
		Attributes: map[string]string{"AWS_INSTANCE_IPV4": "10.0.0.20"},
	})
	require.NoError(t, err)

	// Exec into alpha's container and resolve "beta" via Docker's DNS.
	// Retry for a short window — Docker network connect is async.
	_ = betaContainer
	var getent []byte
	var execErr error
	require.Eventually(t, func() bool {
		cmd := exec.Command("docker", "exec", alphaContainer, "getent", "hosts", "beta")
		getent, execErr = cmd.CombinedOutput()
		return execErr == nil && len(getent) > 0
	}, 10*time.Second, 500*time.Millisecond, "alpha task should resolve 'beta' via Cloud Map DNS: %s", getent)

	assert.Contains(t, string(getent), "beta", "getent output should mention beta hostname: %s", getent)

	// Cleanup: deregister + delete services + delete namespace (removes Docker network)
	_, _ = cm.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
		ServiceId: svcAlpha.Service.Id, InstanceId: aws.String(alphaCID[:12]),
	})
	_, _ = cm.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
		ServiceId: svcBeta.Service.Id, InstanceId: aws.String(betaCID[:12]),
	})
	_, _ = cm.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: svcAlpha.Service.Id})
	_, _ = cm.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: svcBeta.Service.Id})
	_, _ = cm.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{Id: aws.String(nsID)})
	_, _ = ecsCli.StopTask(ctx, &ecs.StopTaskInput{Cluster: aws.String("xtask-dns"), Task: aws.String(alphaTask)})
	_, _ = ecsCli.StopTask(ctx, &ecs.StopTaskInput{Cluster: aws.String("xtask-dns"), Task: aws.String(betaTask)})
}

func TestCloudMap_DeleteServiceAndNamespace(t *testing.T) {
	client := cmClient()

	// Create namespace
	createOut, err := client.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name: aws.String("delete-test.local"),
		Vpc:  aws.String("vpc-12345"),
	})
	require.NoError(t, err)
	opOut, err := client.GetOperation(ctx, &servicediscovery.GetOperationInput{
		OperationId: createOut.OperationId,
	})
	require.NoError(t, err)
	nsID := opOut.Operation.Targets["NAMESPACE"]

	// Create service
	svcOut, err := client.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String("deletable-svc"),
		NamespaceId: aws.String(nsID),
	})
	require.NoError(t, err)
	svcID := *svcOut.Service.Id

	// Delete service
	delSvcOut, err := client.DeleteService(ctx, &servicediscovery.DeleteServiceInput{
		Id: aws.String(svcID),
	})
	require.NoError(t, err)
	_ = delSvcOut

	// Verify service is gone — GetService should fail
	_, err = client.GetService(ctx, &servicediscovery.GetServiceInput{
		Id: aws.String(svcID),
	})
	assert.Error(t, err, "GetService should fail after deletion")

	// Delete namespace
	delNsOut, err := client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{
		Id: aws.String(nsID),
	})
	require.NoError(t, err)
	require.NotNil(t, delNsOut.OperationId)

	// Verify namespace is gone
	_, err = client.GetNamespace(ctx, &servicediscovery.GetNamespaceInput{
		Id: aws.String(nsID),
	})
	assert.Error(t, err, "GetNamespace should fail after deletion")
}
