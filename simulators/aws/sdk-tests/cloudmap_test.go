package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
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
