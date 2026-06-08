package aws_sdk_test

import (
	"fmt"
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

	_, err = client.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: aws.String(svcID)})
	assertAWSAPIErrorCode(t, err, "ResourceInUse")

	// Cleanup
	_, _ = client.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
		ServiceId:  aws.String(svcID),
		InstanceId: aws.String("inst-002"),
	})
	_, _ = client.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: aws.String(svcID)})
	_, _ = client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{Id: aws.String(nsID)})
}

// TestECS_CrossTaskDNS exercises cross-task DNS: Cloud Map registrations for
// awsvpc/Fargate-style tasks resolve to the registered task ENI address, so
// tasks in the namespace can resolve each other by service name.
func TestECS_CrossTaskDNS(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker CLI required for cross-task DNS test (no fallback): %v", err)
	}

	cm := cmClient()
	ecsCli := ecsClient()
	vpcID, subnetID := createECSTestVPCSubnet(t, "xtask-dns")

	// Namespace control-plane CRUD is independent from local Docker resources.
	createNs, err := cm.CreatePrivateDnsNamespace(ctx, &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name: aws.String("xtask-dns.local"),
		Vpc:  aws.String(vpcID),
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
	alphaCID := strings.Repeat("a", 64)
	betaCID := strings.Repeat("b", 64)
	t.Cleanup(func() {
		_, _ = cm.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
			ServiceId: svcAlpha.Service.Id, InstanceId: aws.String(alphaCID[:12]),
		})
		_, _ = cm.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
			ServiceId: svcBeta.Service.Id, InstanceId: aws.String(betaCID[:12]),
		})
		_, _ = cm.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: svcAlpha.Service.Id})
		_, _ = cm.DeleteService(ctx, &servicediscovery.DeleteServiceInput{Id: svcBeta.Service.Id})
		_, _ = cm.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{Id: aws.String(nsID)})
	})

	// Cluster + task def running long enough for real container startup and
	// Cloud Map resolver updates on slower runtimes.
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
			Command:    []string{"sleep 120"},
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
				AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{Subnets: []string{subnetID}},
			},
			Tags: []ecstypes.Tag{
				{Key: aws.String("sockerless-container-id"), Value: aws.String(containerID)},
			},
		})
		require.NoError(t, err)
		require.Len(t, runOut.Tasks, 1)
		return *runOut.Tasks[0].TaskArn
	}

	waitTasksRunning := func(tasks ...string) {
		t.Helper()
		var taskStatuses []string
		require.Eventually(t, func() bool {
			out, err := ecsCli.DescribeTasks(ctx, &ecs.DescribeTasksInput{
				Cluster: aws.String("xtask-dns"),
				Tasks:   tasks,
			})
			if err != nil {
				taskStatuses = []string{"DescribeTasks: " + err.Error()}
				return false
			}
			if len(out.Tasks) < len(tasks) {
				taskStatuses = []string{fmt.Sprintf("got %d tasks", len(out.Tasks))}
				return false
			}
			taskStatuses = taskStatuses[:0]
			for _, tk := range out.Tasks {
				status := aws.ToString(tk.LastStatus)
				taskStatuses = append(taskStatuses, aws.ToString(tk.TaskArn)+"="+status)
				if status != "RUNNING" {
					return false
				}
			}
			return true
		}, 45*time.Second, 500*time.Millisecond, "tasks should reach RUNNING: %s", strings.Join(taskStatuses, ", "))
	}
	containerName := func(taskArn string) string {
		taskID := taskArn[strings.LastIndex(taskArn, "/")+1:]
		return "sockerless-sim-aws-task-" + taskID[:12]
	}
	waitContainer := func(name string) {
		t.Helper()
		var inspect []byte
		var inspectErr error
		require.Eventually(t, func() bool {
			inspect, inspectErr = exec.Command("docker", "inspect", name).CombinedOutput()
			return inspectErr == nil
		}, 45*time.Second, 500*time.Millisecond, "task container %s should exist before Cloud Map registration: err=%v output=%q", name, inspectErr, inspect)
	}

	alphaTask := runTask(alphaCID)
	cleanupECSTask(t, ecsCli, "xtask-dns", alphaTask)
	waitTasksRunning(alphaTask)
	alphaContainer := containerName(alphaTask)
	waitContainer(alphaContainer)

	betaTask := runTask(betaCID)
	cleanupECSTask(t, ecsCli, "xtask-dns", betaTask)
	waitTasksRunning(alphaTask, betaTask)
	waitContainer(containerName(betaTask))

	// Register each task as an instance in its service. The registered
	// AWS_INSTANCE_IPV4 is the task ENI address clients should resolve.
	alphaIP := ecsTaskPrivateIPv4(t, ecsCli, "xtask-dns", alphaTask)
	betaIP := ecsTaskPrivateIPv4(t, ecsCli, "xtask-dns", betaTask)
	registerInstance := func(serviceID, instanceID, ip string) {
		t.Helper()
		var registerErr error
		require.Eventually(t, func() bool {
			_, registerErr = cm.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
				ServiceId:  aws.String(serviceID),
				InstanceId: aws.String(instanceID),
				Attributes: map[string]string{"AWS_INSTANCE_IPV4": ip},
			})
			return registerErr == nil
		}, 45*time.Second, 500*time.Millisecond, "RegisterInstance should update task DNS state: %v", registerErr)
	}
	registerInstance(aws.ToString(svcAlpha.Service.Id), alphaCID[:12], alphaIP)
	registerInstance(aws.ToString(svcBeta.Service.Id), betaCID[:12], betaIP)

	// Exec into alpha's container and resolve "beta" through the task's
	// normal libc resolver.
	var getent []byte
	var execErr error
	var hosts []byte
	require.Eventually(t, func() bool {
		cmd := exec.Command("docker", "exec", alphaContainer, "getent", "hosts", "beta")
		getent, execErr = cmd.CombinedOutput()
		hosts, _ = exec.Command("docker", "exec", alphaContainer, "cat", "/etc/hosts").CombinedOutput()
		return execErr == nil && len(getent) > 0
	}, 10*time.Second, 500*time.Millisecond, "alpha task should resolve 'beta' via Cloud Map DNS: getent=%q err=%v hosts=%q", getent, execErr, hosts)

	assert.Contains(t, string(getent), "beta", "getent output should mention beta hostname: %s", getent)

}

func ecsTaskPrivateIPv4(t *testing.T, client *ecs.Client, clusterName, taskArn string) string {
	t.Helper()
	out, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, out.Tasks, 1)
	for _, attachment := range out.Tasks[0].Attachments {
		if aws.ToString(attachment.Type) != "ElasticNetworkInterface" {
			continue
		}
		for _, detail := range attachment.Details {
			if aws.ToString(detail.Name) == "privateIPv4Address" {
				ip := aws.ToString(detail.Value)
				require.NotEmpty(t, ip)
				return ip
			}
		}
	}
	t.Fatalf("task %s did not include an ElasticNetworkInterface privateIPv4Address", taskArn)
	return ""
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

	_, err = client.DeleteNamespace(ctx, &servicediscovery.DeleteNamespaceInput{
		Id: aws.String(nsID),
	})
	assertAWSAPIErrorCode(t, err, "ResourceInUse")

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
