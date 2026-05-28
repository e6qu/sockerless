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

func TestEventarc_ChannelProviderConnectionSDK(t *testing.T) {
	client := eventarcClient(t)
	parent := "projects/test-project/locations/us-central1"
	channelName := parent + "/channels/sdk-channel"
	connectionName := parent + "/channelConnections/sdk-connection"

	providers := client.ListProviders(ctx, &eventarcpb.ListProvidersRequest{Parent: parent})
	provider, err := providers.Next()
	require.NoError(t, err)
	assert.Equal(t, parent+"/providers/cloud.pubsub", provider.GetName())
	require.NotEmpty(t, provider.GetEventTypes())

	gotProvider, err := client.GetProvider(ctx, &eventarcpb.GetProviderRequest{Name: parent + "/providers/cloud.pubsub"})
	require.NoError(t, err)
	assert.Equal(t, "Cloud Pub/Sub", gotProvider.GetDisplayName())

	createChannel, err := client.CreateChannel(ctx, &eventarcpb.CreateChannelRequest{
		Parent:    parent,
		ChannelId: "sdk-channel",
		Channel: &eventarcpb.Channel{
			Provider: parent + "/providers/cloud.pubsub",
			Transport: &eventarcpb.Channel_PubsubTopic{
				PubsubTopic: "projects/test-project/topics/sdk-channel-topic",
			},
			Labels: map[string]string{"env": "test"},
		},
	})
	require.NoError(t, err)
	channel, err := createChannel.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, channelName, channel.GetName())
	assert.Equal(t, eventarcpb.Channel_ACTIVE, channel.GetState())
	require.NotEmpty(t, channel.GetActivationToken())
	t.Cleanup(func() {
		op, err := client.DeleteChannel(ctx, &eventarcpb.DeleteChannelRequest{Name: channelName})
		if err == nil {
			_, _ = op.Wait(ctx)
		}
	})

	gotChannel, err := client.GetChannel(ctx, &eventarcpb.GetChannelRequest{Name: channelName})
	require.NoError(t, err)
	assert.Equal(t, "projects/test-project/topics/sdk-channel-topic", gotChannel.GetPubsubTopic())

	channels := client.ListChannels(ctx, &eventarcpb.ListChannelsRequest{Parent: parent})
	listedChannel, err := channels.Next()
	require.NoError(t, err)
	assert.Equal(t, channelName, listedChannel.GetName())
	_, err = channels.Next()
	assert.ErrorIs(t, err, iterator.Done)

	createConnection, err := client.CreateChannelConnection(ctx, &eventarcpb.CreateChannelConnectionRequest{
		Parent:              parent,
		ChannelConnectionId: "sdk-connection",
		ChannelConnection: &eventarcpb.ChannelConnection{
			Channel:         channelName,
			ActivationToken: channel.GetActivationToken(),
			Labels:          map[string]string{"env": "test"},
		},
	})
	require.NoError(t, err)
	connection, err := createConnection.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, connectionName, connection.GetName())
	assert.Equal(t, channelName, connection.GetChannel())
	t.Cleanup(func() {
		op, err := client.DeleteChannelConnection(ctx, &eventarcpb.DeleteChannelConnectionRequest{Name: connectionName})
		if err == nil {
			_, _ = op.Wait(ctx)
		}
	})

	gotConnection, err := client.GetChannelConnection(ctx, &eventarcpb.GetChannelConnectionRequest{Name: connectionName})
	require.NoError(t, err)
	assert.Equal(t, channelName, gotConnection.GetChannel())

	connections := client.ListChannelConnections(ctx, &eventarcpb.ListChannelConnectionsRequest{Parent: parent})
	listedConnection, err := connections.Next()
	require.NoError(t, err)
	assert.Equal(t, connectionName, listedConnection.GetName())
	_, err = connections.Next()
	assert.ErrorIs(t, err, iterator.Done)
}
