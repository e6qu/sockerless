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

// BUG-833 — sim was missing v2 Cloud Run Services routes; the
// cloudrun backend uses run.NewServicesRESTClient (v2 REST) when
// Config.UseService=true and silently 404'd against the sim. These
// tests pin the v2 contract using the same client the backend uses.

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
					{Image: "gcr.io/test-project/hello"},
				},
				Scaling: &runpb.RevisionScaling{
					MinInstanceCount: 1,
					MaxInstanceCount: 1,
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

	got, err := client.GetService(ctx, &runpb.GetServiceRequest{Name: svc.Name})
	require.NoError(t, err)
	assert.Equal(t, svc.Name, got.Name)
	assert.Equal(t, "true", got.Labels["sockerless_managed"])

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
