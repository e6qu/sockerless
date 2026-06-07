package aws_cli_test

import (
	"strings"
	"testing"
)

// TestCloudTrailLookupFilterKeysCLI covers issue #496: `cloudtrail
// lookup-events` must honour the ResourceName / ResourceType / EventId /
// ReadOnly attribute keys, not only EventName (the others were silently ignored
// and returned every event).
func TestCloudTrailLookupFilterKeysCLI(t *testing.T) {
	const cluster = "cli-ct-filter-cluster"
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", cluster))
	t.Cleanup(func() { _ = awsCLI("ecs", "delete-cluster", "--cluster", cluster).Run() })

	// EventId filter — capture the id via EventName, then look it up exactly.
	eventID := strings.TrimSpace(runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=EventName,AttributeValue=CreateCluster",
		"--query", "Events[0].EventId", "--output", "text")))
	if eventID == "" {
		t.Fatal("CreateCluster event not recorded")
	}
	got := strings.TrimSpace(runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=EventId,AttributeValue="+eventID,
		"--query", "Events[0].EventName", "--output", "text")))
	if got != "CreateCluster" {
		t.Fatalf("EventId filter: got %q, want CreateCluster", got)
	}

	// ResourceName filter — the cluster the call acted on.
	got = strings.TrimSpace(runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=ResourceName,AttributeValue="+cluster,
		"--query", "Events[0].EventName", "--output", "text")))
	if got != "CreateCluster" {
		t.Fatalf("ResourceName filter: got %q, want CreateCluster", got)
	}

	// ResourceType filter — AWS::ECS::Cluster.
	got = strings.TrimSpace(runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=ResourceType,AttributeValue=AWS::ECS::Cluster",
		"--query", "length(Events[?EventName=='CreateCluster'])", "--output", "text")))
	if got == "0" || got == "" {
		t.Fatalf("ResourceType filter returned no CreateCluster event (got %q)", got)
	}

	// Negative: a non-matching ResourceName must return no CreateCluster event —
	// proves the filter is applied, not silently ignored.
	got = strings.TrimSpace(runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=ResourceName,AttributeValue=cli-no-such-resource",
		"--query", "length(Events[?EventName=='CreateCluster'])", "--output", "text")))
	if got != "0" {
		t.Fatalf("non-matching ResourceName must return 0 CreateCluster events, got %q", got)
	}
}

// TestCloudTrailRecordsSchedulerAPICallCLI covers issue #498: EventBridge
// Scheduler API calls must be recorded in CloudTrail against
// scheduler.amazonaws.com.
func TestCloudTrailRecordsSchedulerAPICallCLI(t *testing.T) {
	const name = "cli-ct-schedule"
	runCLI(t, awsCLI("scheduler", "create-schedule",
		"--name", name,
		"--schedule-expression", "rate(1 hour)",
		"--flexible-time-window", `{"Mode":"OFF"}`,
		"--target", `{"Arn":"arn:aws:sqs:us-east-1:123456789012:cli-ct-q","RoleArn":"arn:aws:iam::123456789012:role/scheduler-role"}`,
	))
	t.Cleanup(func() { _ = awsCLI("scheduler", "delete-schedule", "--name", name).Run() })

	src := strings.TrimSpace(runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=EventName,AttributeValue=CreateSchedule",
		"--query", "Events[0].EventSource", "--output", "text")))
	if src != "scheduler.amazonaws.com" {
		t.Fatalf("CreateSchedule EventSource: got %q, want scheduler.amazonaws.com", src)
	}
}
