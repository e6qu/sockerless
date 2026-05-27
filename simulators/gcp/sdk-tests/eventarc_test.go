package gcp_sdk_test

import (
	"testing"

	eventarc "cloud.google.com/go/eventarc/apiv1"
	"cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func eventarcClient(t *testing.T) *eventarc.Client {
	t.Helper()
	client, err := eventarc.NewRESTClient(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	return client
}

func TestEventarc_TriggerLifecycleSDK(t *testing.T) {
	client := eventarcClient(t)
	parent := "projects/test-project/locations/us-central1"
	name := parent + "/triggers/sdk-trigger"

	create, err := client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: "sdk-trigger",
		Trigger: &eventarcpb.Trigger{
			EventFilters: []*eventarcpb.EventFilter{{
				Attribute: "type",
				Value:     "google.cloud.pubsub.topic.v1.messagePublished",
			}},
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_CloudRun{
					CloudRun: &eventarcpb.CloudRun{Service: "svc", Region: "us-central1"},
				},
			},
			Transport: &eventarcpb.Transport{
				Intermediary: &eventarcpb.Transport_Pubsub{
					Pubsub: &eventarcpb.Pubsub{Topic: "projects/test-project/topics/eventarc-topic"},
				},
			},
			Labels: map[string]string{"env": "test"},
		},
	})
	require.NoError(t, err)
	created, err := create.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, name, created.GetName())
	assert.Equal(t, "test", created.GetLabels()["env"])
	t.Cleanup(func() {
		op, err := client.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{Name: name})
		if err == nil {
			_, _ = op.Wait(ctx)
		}
	})

	got, err := client.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{Name: name})
	require.NoError(t, err)
	assert.Equal(t, name, got.GetName())
	assert.Equal(t, "svc", got.GetDestination().GetCloudRun().GetService())

	iter := client.ListTriggers(ctx, &eventarcpb.ListTriggersRequest{Parent: parent})
	listed, err := iter.Next()
	require.NoError(t, err)
	assert.Equal(t, name, listed.GetName())
	_, err = iter.Next()
	assert.ErrorIs(t, err, iterator.Done)
}
