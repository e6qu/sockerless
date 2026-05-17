package gcp_sdk_test

import (
	"testing"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// v2 Cloud Run Services routes contract. The cloudrun backend uses
// run.NewServicesRESTClient (v2 REST) when Config.UseService=true.
// These tests pin the v2 contract using the same client the backend
// uses.

func newServicesClient(t *testing.T) *run.ServicesClient {
	t.Helper()
	client, err := run.NewServicesRESTClient(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

func TestSDK_CloudRunV2Services_CreateGetListDelete(t *testing.T) {
	client := newServicesClient(t)

	createOp, err := client.CreateService(ctx, &runpb.CreateServiceRequest{
		Parent:    "projects/test-project/locations/us-central1",
		ServiceId: "v2-svc-roundtrip",
		Service: &runpb.Service{
			Labels: map[string]string{
				"sockerless_managed":      "true",
				"sockerless_container_id": "abc123",
			},
			Annotations: map[string]string{
				"sockerless_name": "my-svc",
			},
			Ingress: runpb.IngressTraffic_INGRESS_TRAFFIC_INTERNAL_ONLY,
			Template: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{
						Image: "gcr.io/test-project/hello",
						Env: []*runpb.EnvVar{
							{Name: "SOCKERLESS_CALLBACK_URL", Values: &runpb.EnvVar_Value{Value: "ws://host.docker.internal:3375/v1/cloudrun/reverse"}},
							{Name: "SOCKERLESS_CONTAINER_ID", Values: &runpb.EnvVar_Value{Value: "abc123"}},
						},
					},
				},
				Scaling: &runpb.RevisionScaling{
					MinInstanceCount: 1,
					MaxInstanceCount: 1,
				},
				VpcAccess: &runpb.VpcAccess{
					Connector: "projects/test-project/locations/us-central1/connectors/test-connector",
					Egress:    runpb.VpcAccess_ALL_TRAFFIC,
				},
			},
		},
	})
	require.NoError(t, err)

	svc, err := createOp.Wait(ctx)
	require.NoError(t, err, "CreateService LRO must complete")
	require.NotNil(t, svc)
	assert.Contains(t, svc.Name, "v2-svc-roundtrip")
	assert.NotEmpty(t, svc.Uid)
	assert.Equal(t, int64(1), svc.Generation)
	assert.Equal(t, "true", svc.Labels["sockerless_managed"])
	assert.Equal(t, "my-svc", svc.Annotations["sockerless_name"])
	require.NotNil(t, svc.TerminalCondition)
	assert.Equal(t, runpb.Condition_CONDITION_SUCCEEDED, svc.TerminalCondition.State)
	assert.NotEmpty(t, svc.LatestReadyRevision, "LatestReadyRevision must be set so backend's serviceContainerState reads 'running'")
	require.NotNil(t, svc.Template)
	require.NotNil(t, svc.Template.VpcAccess)
	assert.Equal(t, "projects/test-project/locations/us-central1/connectors/test-connector", svc.Template.VpcAccess.Connector)
	assert.Equal(t, runpb.VpcAccess_ALL_TRAFFIC, svc.Template.VpcAccess.Egress)
	require.Len(t, svc.Template.Containers, 1)
	require.Len(t, svc.Template.Containers[0].Env, 2)
	assert.Equal(t, "SOCKERLESS_CALLBACK_URL", svc.Template.Containers[0].Env[0].Name)
	assert.Equal(t, "ws://host.docker.internal:3375/v1/cloudrun/reverse", svc.Template.Containers[0].Env[0].GetValue())
	assert.Equal(t, "SOCKERLESS_CONTAINER_ID", svc.Template.Containers[0].Env[1].Name)
	assert.Equal(t, "abc123", svc.Template.Containers[0].Env[1].GetValue())

	got, err := client.GetService(ctx, &runpb.GetServiceRequest{Name: svc.Name})
	require.NoError(t, err)
	assert.Equal(t, svc.Name, got.Name)
	assert.Equal(t, "true", got.Labels["sockerless_managed"])
	require.Len(t, got.Template.Containers, 1)
	require.Len(t, got.Template.Containers[0].Env, 2)
	assert.Equal(t, "ws://host.docker.internal:3375/v1/cloudrun/reverse", got.Template.Containers[0].Env[0].GetValue())
	assert.Equal(t, "abc123", got.Template.Containers[0].Env[1].GetValue())

	it := client.ListServices(ctx, &runpb.ListServicesRequest{
		Parent: "projects/test-project/locations/us-central1",
	})
	found := false
	for {
		s, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		if s.Name == svc.Name {
			found = true
			break
		}
	}
	assert.True(t, found, "ListServices must return the service we just created")

	deleteOp, err := client.DeleteService(ctx, &runpb.DeleteServiceRequest{Name: svc.Name})
	require.NoError(t, err)
	_, err = deleteOp.Wait(ctx)
	require.NoError(t, err)

	_, err = client.GetService(ctx, &runpb.GetServiceRequest{Name: svc.Name})
	assert.Error(t, err, "GetService after delete must 404")
}

func TestSDK_CloudRunV2Services_UpdateBumpsGeneration(t *testing.T) {
	client := newServicesClient(t)

	createOp, err := client.CreateService(ctx, &runpb.CreateServiceRequest{
		Parent:    "projects/test-project/locations/us-central1",
		ServiceId: "v2-svc-update",
		Service: &runpb.Service{
			Template: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{{Image: "gcr.io/test-project/v1"}},
			},
		},
	})
	require.NoError(t, err)
	created, err := createOp.Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), created.Generation)

	updateOp, err := client.UpdateService(ctx, &runpb.UpdateServiceRequest{
		Service: &runpb.Service{
			Name: created.Name,
			Template: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{{Image: "gcr.io/test-project/v2"}},
			},
		},
	})
	require.NoError(t, err)
	updated, err := updateOp.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.Generation, "Update should bump generation")
	require.NotNil(t, updated.TerminalCondition)
	assert.Equal(t, runpb.Condition_CONDITION_SUCCEEDED, updated.TerminalCondition.State)

	// Cleanup.
	_, _ = client.DeleteService(ctx, &runpb.DeleteServiceRequest{Name: created.Name})
}
