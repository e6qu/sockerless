package aws_cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBridgeCLI_RuleTargetPutEvents(t *testing.T) {
	runCLI(t, awsCLI("sqs", "create-queue", "--queue-name", "eb-cli-q"))
	t.Cleanup(func() {
		out := runCLI(t, awsCLI("sqs", "get-queue-url", "--queue-name", "eb-cli-q"))
		var q struct {
			QueueUrl string `json:"QueueUrl"`
		}
		parseJSON(t, out, &q)
		runCLI(t, awsCLI("sqs", "delete-queue", "--queue-url", q.QueueUrl))
	})

	out := runCLI(t, awsCLI("sqs", "get-queue-url", "--queue-name", "eb-cli-q"))
	var q struct {
		QueueUrl string `json:"QueueUrl"`
	}
	parseJSON(t, out, &q)
	out = runCLI(t, awsCLI("sqs", "get-queue-attributes",
		"--queue-url", q.QueueUrl,
		"--attribute-names", "QueueArn"))
	var attrs struct {
		Attributes map[string]string `json:"Attributes"`
	}
	parseJSON(t, out, &attrs)
	queueARN := attrs.Attributes["QueueArn"]
	require.NotEmpty(t, queueARN)

	runCLI(t, awsCLI("events", "put-rule",
		"--name", "eb-cli-rule",
		"--event-pattern", `{"source":["sockerless.cli"]}`))
	t.Cleanup(func() {
		runCLI(t, awsCLI("events", "remove-targets", "--rule", "eb-cli-rule", "--ids", "queue"))
		runCLI(t, awsCLI("events", "delete-rule", "--name", "eb-cli-rule"))
	})

	runCLI(t, awsCLI("events", "put-targets",
		"--rule", "eb-cli-rule",
		"--targets", `[{"Id":"queue","Arn":"`+queueARN+`"}]`))

	out = runCLI(t, awsCLI("events", "list-targets-by-rule", "--rule", "eb-cli-rule"))
	var targets struct {
		Targets []struct {
			ID  string `json:"Id"`
			Arn string `json:"Arn"`
		} `json:"Targets"`
	}
	parseJSON(t, out, &targets)
	require.Len(t, targets.Targets, 1)
	assert.Equal(t, queueARN, targets.Targets[0].Arn)

	entries, err := json.Marshal([]map[string]string{{
		"Source":     "sockerless.cli",
		"DetailType": "example",
		"Detail":     `{"cli":true}`,
	}})
	require.NoError(t, err)
	runCLI(t, awsCLI("events", "put-events", "--entries", string(entries)))

	out = runCLI(t, awsCLI("sqs", "receive-message", "--queue-url", q.QueueUrl))
	var recv struct {
		Messages []struct {
			Body string `json:"Body"`
		} `json:"Messages"`
	}
	parseJSON(t, out, &recv)
	require.Len(t, recv.Messages, 1)
	assert.JSONEq(t, `{"cli":true}`, recv.Messages[0].Body)
}
