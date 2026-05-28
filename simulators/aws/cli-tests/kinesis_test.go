package aws_cli_test

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKinesisCLI_StreamAndRecords(t *testing.T) {
	stream := "cli-kinesis-stream"

	runCLI(t, awsCLI("kinesis", "create-stream",
		"--stream-name", stream,
		"--shard-count", "1",
		"--tags", "env=cli"))
	t.Cleanup(func() {
		_ = awsCLI("kinesis", "delete-stream", "--stream-name", stream).Run()
	})

	describe := runCLI(t, awsCLI("kinesis", "describe-stream", "--stream-name", stream))
	var desc struct {
		StreamDescription struct {
			StreamName   string `json:"StreamName"`
			StreamStatus string `json:"StreamStatus"`
			Shards       []struct {
				ShardID string `json:"ShardId"`
			} `json:"Shards"`
		} `json:"StreamDescription"`
	}
	parseJSON(t, describe, &desc)
	assert.Equal(t, stream, desc.StreamDescription.StreamName)
	assert.Equal(t, "ACTIVE", desc.StreamDescription.StreamStatus)
	require.Len(t, desc.StreamDescription.Shards, 1)

	runCLI(t, awsCLI("kinesis", "put-record",
		"--stream-name", stream,
		"--partition-key", "cli-pk",
		"--data", "cli-body",
		"--cli-binary-format", "raw-in-base64-out"))

	iterJSON := runCLI(t, awsCLI("kinesis", "get-shard-iterator",
		"--stream-name", stream,
		"--shard-id", desc.StreamDescription.Shards[0].ShardID,
		"--shard-iterator-type", "TRIM_HORIZON"))
	var iter struct {
		ShardIterator string `json:"ShardIterator"`
	}
	parseJSON(t, iterJSON, &iter)
	require.NotEmpty(t, iter.ShardIterator)

	recordsJSON := runCLI(t, awsCLI("kinesis", "get-records",
		"--shard-iterator", iter.ShardIterator,
		"--limit", "10"))
	var records struct {
		Records []struct {
			Data string `json:"Data"`
		} `json:"Records"`
	}
	parseJSON(t, recordsJSON, &records)
	require.Len(t, records.Records, 1)
	body, err := base64.StdEncoding.DecodeString(records.Records[0].Data)
	require.NoError(t, err)
	assert.Equal(t, "cli-body", string(body))
}
