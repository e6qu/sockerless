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

func TestEventBridgeCLI_BusArchiveReplay(t *testing.T) {
	out := runCLI(t, awsCLI("events", "create-event-bus",
		"--name", "eb-cli-bus",
		"--description", "cli bus"))
	var bus struct {
		EventBusArn string `json:"EventBusArn"`
	}
	parseJSON(t, out, &bus)
	require.NotEmpty(t, bus.EventBusArn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("events", "delete-archive", "--archive-name", "eb-cli-archive"))
		runCLI(t, awsCLI("events", "delete-event-bus", "--name", "eb-cli-bus"))
	})

	out = runCLI(t, awsCLI("events", "describe-event-bus", "--name", "eb-cli-bus"))
	var described struct {
		Name string `json:"Name"`
		Arn  string `json:"Arn"`
	}
	parseJSON(t, out, &described)
	assert.Equal(t, "eb-cli-bus", described.Name)
	assert.Equal(t, bus.EventBusArn, described.Arn)

	out = runCLI(t, awsCLI("events", "list-event-buses", "--name-prefix", "eb-cli"))
	var buses struct {
		EventBuses []struct {
			Name string `json:"Name"`
		} `json:"EventBuses"`
	}
	parseJSON(t, out, &buses)
	require.Len(t, buses.EventBuses, 1)

	runCLI(t, awsCLI("events", "put-permission",
		"--event-bus-name", "eb-cli-bus",
		"--statement-id", "cli-permission",
		"--action", "events:PutEvents",
		"--principal", "123456789012"))
	out = runCLI(t, awsCLI("events", "describe-event-bus", "--name", "eb-cli-bus"))
	var policyBus struct {
		Policy string `json:"Policy"`
	}
	parseJSON(t, out, &policyBus)
	require.Contains(t, policyBus.Policy, "cli-permission")
	runCLI(t, awsCLI("events", "remove-permission",
		"--event-bus-name", "eb-cli-bus",
		"--statement-id", "cli-permission"))

	out = runCLI(t, awsCLI("events", "create-archive",
		"--archive-name", "eb-cli-archive",
		"--event-source-arn", bus.EventBusArn,
		"--event-pattern", `{"source":["sockerless.cli.archive"]}`))
	var archive struct {
		ArchiveArn string `json:"ArchiveArn"`
		State      string `json:"State"`
	}
	parseJSON(t, out, &archive)
	require.NotEmpty(t, archive.ArchiveArn)
	assert.Equal(t, "ENABLED", archive.State)

	out = runCLI(t, awsCLI("events", "describe-archive", "--archive-name", "eb-cli-archive"))
	var describedArchive struct {
		ArchiveName    string `json:"ArchiveName"`
		EventSourceArn string `json:"EventSourceArn"`
	}
	parseJSON(t, out, &describedArchive)
	assert.Equal(t, "eb-cli-archive", describedArchive.ArchiveName)
	assert.Equal(t, bus.EventBusArn, describedArchive.EventSourceArn)

	out = runCLI(t, awsCLI("events", "list-archives", "--event-source-arn", bus.EventBusArn))
	var archives struct {
		Archives []struct {
			ArchiveName string `json:"ArchiveName"`
		} `json:"Archives"`
	}
	parseJSON(t, out, &archives)
	require.Len(t, archives.Archives, 1)

	runCLI(t, awsCLI("events", "put-events", "--entries", `[{"EventBusName":"eb-cli-bus","Source":"sockerless.cli.archive","DetailType":"example","Detail":"{\"cli\":true}"}]`))
	out = runCLI(t, awsCLI("events", "start-replay",
		"--replay-name", "eb-cli-replay",
		"--event-source-arn", archive.ArchiveArn,
		"--event-start-time", "2026-05-27T00:00:00Z",
		"--event-end-time", "2026-05-29T00:00:00Z",
		"--destination", `{"Arn":"`+bus.EventBusArn+`"}`))
	var replay struct {
		ReplayArn string `json:"ReplayArn"`
		State     string `json:"State"`
	}
	parseJSON(t, out, &replay)
	require.NotEmpty(t, replay.ReplayArn)
	assert.Equal(t, "COMPLETED", replay.State)

	out = runCLI(t, awsCLI("events", "describe-replay", "--replay-name", "eb-cli-replay"))
	var describedReplay struct {
		ReplayName string `json:"ReplayName"`
		State      string `json:"State"`
	}
	parseJSON(t, out, &describedReplay)
	assert.Equal(t, "eb-cli-replay", describedReplay.ReplayName)
	assert.Equal(t, "COMPLETED", describedReplay.State)

	out = runCLI(t, awsCLI("events", "list-replays", "--event-source-arn", archive.ArchiveArn))
	var replays struct {
		Replays []struct {
			ReplayName string `json:"ReplayName"`
		} `json:"Replays"`
	}
	parseJSON(t, out, &replays)
	require.Len(t, replays.Replays, 1)
}
