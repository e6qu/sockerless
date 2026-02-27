package gcp_sdk_test

import (
	"testing"

	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logadminClient(t *testing.T) *logadmin.Client {
	t.Helper()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client, err := logadmin.NewClient(ctx, "test-project", option.WithGRPCConn(conn))
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	return client
}

func TestLogging_WriteAndListEntries(t *testing.T) {
	client := logadminClient(t)

	// Write entries via gRPC using the logging.Client (write client)
	writeClient, err := newLoggingWriteClient(t)
	require.NoError(t, err)

	err = writeClient.Logger("test-log-write").LogSync(ctx, writeEntry("hello from gRPC"))
	require.NoError(t, err)
	err = writeClient.Logger("test-log-write").LogSync(ctx, writeEntry("second entry"))
	require.NoError(t, err)
	err = writeClient.Close()
	require.NoError(t, err)

	// List entries via logadmin
	it := client.Entries(ctx, logadmin.Filter(`logName="projects/test-project/logs/test-log-write"`))

	var messages []string
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		if entry.Payload != nil {
			if s, ok := entry.Payload.(string); ok {
				messages = append(messages, s)
			}
		}
	}

	require.Len(t, messages, 2)
	assert.Equal(t, "hello from gRPC", messages[0])
	assert.Equal(t, "second entry", messages[1])
}

func TestLogging_FilterByResourceType(t *testing.T) {
	client := logadminClient(t)

	writeClient, err := newLoggingWriteClient(t)
	require.NoError(t, err)

	// Write entries with different resource types
	err = writeClient.Logger("filter-test").LogSync(ctx, writeEntryWithResource("cloud_run_job", "job-1", "cloud run entry"))
	require.NoError(t, err)
	err = writeClient.Logger("filter-test").LogSync(ctx, writeEntryWithResource("cloud_run_revision", "svc-1", "cloud function entry"))
	require.NoError(t, err)
	err = writeClient.Close()
	require.NoError(t, err)

	// Filter by resource.type
	it := client.Entries(ctx, logadmin.Filter(`resource.type="cloud_run_job"`))

	var count int
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		assert.Equal(t, "cloud_run_job", entry.Resource.Type)
		count++
	}

	assert.GreaterOrEqual(t, count, 1)
}

func TestLogging_FilterByTimestamp(t *testing.T) {
	client := logadminClient(t)

	writeClient, err := newLoggingWriteClient(t)
	require.NoError(t, err)

	err = writeClient.Logger("ts-filter-test").LogSync(ctx, writeEntry("old entry"))
	require.NoError(t, err)
	err = writeClient.Logger("ts-filter-test").LogSync(ctx, writeEntry("new entry"))
	require.NoError(t, err)
	err = writeClient.Close()
	require.NoError(t, err)

	// Use a filter that should match all (timestamp >= a very old date)
	it := client.Entries(ctx, logadmin.Filter(`timestamp>="2020-01-01T00:00:00Z"`))

	var count int
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		count++
	}

	assert.GreaterOrEqual(t, count, 2, "should return entries with timestamps >= 2020")
}
