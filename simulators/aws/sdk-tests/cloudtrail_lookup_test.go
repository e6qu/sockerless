package aws_sdk_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cttypes "github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedtypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ctLookup(t *testing.T, ct *cloudtrail.Client, key cttypes.LookupAttributeKey, value string) []cttypes.Event {
	t.Helper()
	out, err := ct.LookupEvents(ctx, &cloudtrail.LookupEventsInput{
		LookupAttributes: []cttypes.LookupAttribute{{AttributeKey: key, AttributeValue: aws.String(value)}},
	})
	require.NoError(t, err)
	return out.Events
}

func ctHasEventNamed(events []cttypes.Event, name string) bool {
	for _, e := range events {
		if aws.ToString(e.EventName) == name {
			return true
		}
	}
	return false
}

// TestCloudTrailLookupFilterKeysSDK covers issue #496: LookupEvents must filter
// on all eight AttributeKey values, not just EventName/EventSource/Username
// (the other five were silently ignored, so any filter using them returned
// every event). Creates an ECS cluster — a resource-bearing management event —
// and exercises the EventId / ResourceName / ResourceType / ReadOnly filters.
func TestCloudTrailLookupFilterKeysSDK(t *testing.T) {
	ct := cloudTrailClient()
	ecsc := ecsClient()
	const cluster = "ct-filter-cluster"

	_, err := ecsc.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(cluster)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = ecsc.DeleteCluster(ctx, &ecs.DeleteClusterInput{Cluster: aws.String(cluster)})
	})

	// By EventName — locate the CreateCluster event and capture its id.
	byName := ctLookup(t, ct, cttypes.LookupAttributeKeyEventName, "CreateCluster")
	require.True(t, ctHasEventNamed(byName, "CreateCluster"),
		"LookupEvents by EventName must return the CreateCluster event")
	var eventID, src string
	for _, e := range byName {
		if aws.ToString(e.EventName) == "CreateCluster" {
			eventID = aws.ToString(e.EventId)
			src = aws.ToString(e.EventSource)
		}
	}
	assert.Equal(t, "ecs.amazonaws.com", src)
	require.NotEmpty(t, eventID)

	// By EventId — exact match returns that one event (previously ignored).
	byID := ctLookup(t, ct, cttypes.LookupAttributeKeyEventId, eventID)
	require.Len(t, byID, 1, "EventId filter must return exactly the one matching event")
	assert.Equal(t, eventID, aws.ToString(byID[0].EventId))

	// By ResourceName — the cluster name the call acted on.
	byResName := ctLookup(t, ct, cttypes.LookupAttributeKeyResourceName, cluster)
	require.True(t, ctHasEventNamed(byResName, "CreateCluster"),
		"ResourceName filter must return events touching the named resource")

	// By ResourceType — AWS::ECS::Cluster.
	byResType := ctLookup(t, ct, cttypes.LookupAttributeKeyResourceType, "AWS::ECS::Cluster")
	require.True(t, ctHasEventNamed(byResType, "CreateCluster"),
		"ResourceType filter must return events of that resource type")

	// By ReadOnly=false — a mutating call; ReadOnly=true must exclude it.
	rw := ctLookup(t, ct, cttypes.LookupAttributeKeyReadOnly, "false")
	assert.True(t, ctHasEventNamed(rw, "CreateCluster"), "CreateCluster is a write event (ReadOnly=false)")
	ro := ctLookup(t, ct, cttypes.LookupAttributeKeyReadOnly, "true")
	assert.False(t, ctHasEventNamed(ro, "CreateCluster"), "CreateCluster must not appear under ReadOnly=true")

	// Negative: a non-matching ResourceName returns nothing — proves the filter
	// is applied, not silently ignored (the #496 defect returned all events).
	none := ctLookup(t, ct, cttypes.LookupAttributeKeyResourceName, "no-such-resource-xyz")
	assert.False(t, ctHasEventNamed(none, "CreateCluster"),
		"a non-matching ResourceName filter must not return the CreateCluster event")
}

// TestCloudTrailRecordsSchedulerAPICallsSDK covers issue #498: EventBridge
// Scheduler is a REST service registered off the central POST / router, so its
// API calls bypassed the CloudTrail recording middleware. CreateSchedule (and
// the other Scheduler operations) must now appear in LookupEvents against
// scheduler.amazonaws.com.
func TestCloudTrailRecordsSchedulerAPICallsSDK(t *testing.T) {
	ct := cloudTrailClient()
	sched := schedulerClient()
	const name = "ct-api-schedule"

	_, err := sched.CreateSchedule(ctx, &scheduler.CreateScheduleInput{
		Name:               aws.String(name),
		ScheduleExpression: aws.String("rate(1 hour)"),
		FlexibleTimeWindow: &schedtypes.FlexibleTimeWindow{Mode: schedtypes.FlexibleTimeWindowModeOff},
		Target: &schedtypes.Target{
			Arn:     aws.String("arn:aws:sqs:us-east-1:123456789012:ct-api-queue"),
			RoleArn: aws.String("arn:aws:iam::123456789012:role/scheduler"),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = sched.DeleteSchedule(ctx, &scheduler.DeleteScheduleInput{Name: aws.String(name)})
	})

	byName := ctLookup(t, ct, cttypes.LookupAttributeKeyEventName, "CreateSchedule")
	require.True(t, ctHasEventNamed(byName, "CreateSchedule"),
		"Scheduler CreateSchedule must be recorded in CloudTrail")
	for _, e := range byName {
		if aws.ToString(e.EventName) == "CreateSchedule" {
			assert.Equal(t, "scheduler.amazonaws.com", aws.ToString(e.EventSource))
		}
	}

	// The schedule resource is filterable by name.
	byRes := ctLookup(t, ct, cttypes.LookupAttributeKeyResourceName, name)
	assert.True(t, ctHasEventNamed(byRes, "CreateSchedule"),
		"the CreateSchedule event must carry the schedule as a ResourceName")
}

// TestCloudTrailRecordsSchedulerFiredTargetSDK covers issue #497: when the
// Scheduler firing loop invokes a target it calls the target handler directly,
// bypassing the POST / recording middleware. The downstream call (here SQS
// SendMessage) must be recorded with userIdentity.invokedBy =
// scheduler.amazonaws.com, as real CloudTrail records scheduler-driven calls.
func TestCloudTrailRecordsSchedulerFiredTargetSDK(t *testing.T) {
	ct := cloudTrailClient()
	sched := schedulerClient()
	sqsc := sqsClient()
	const queue = "ct-fire-queue"

	cq, err := sqsc.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(queue)})
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = sqsc.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: cq.QueueUrl}) })

	fireAt := time.Now().UTC().Add(3 * time.Second).Format("2006-01-02T15:04:05")
	_, err = sched.CreateSchedule(ctx, &scheduler.CreateScheduleInput{
		Name:               aws.String("ct-fire-sqs"),
		ScheduleExpression: aws.String("at(" + fireAt + ")"),
		FlexibleTimeWindow: &schedtypes.FlexibleTimeWindow{Mode: schedtypes.FlexibleTimeWindowModeOff},
		Target: &schedtypes.Target{
			Arn:     aws.String("arn:aws:sqs:us-east-1:123456789012:" + queue),
			RoleArn: aws.String("arn:aws:iam::123456789012:role/scheduler"),
			Input:   aws.String("scheduled-payload"),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = sched.DeleteSchedule(ctx, &scheduler.DeleteScheduleInput{Name: aws.String("ct-fire-sqs")})
	})

	require.Eventually(t, func() bool {
		events := ctLookup(t, ct, cttypes.LookupAttributeKeyEventName, "SendMessage")
		for _, e := range events {
			if aws.ToString(e.EventName) != "SendMessage" {
				continue
			}
			if aws.ToString(e.EventSource) != "sqs.amazonaws.com" {
				continue
			}
			// invokedBy lives in the embedded full event record.
			if strings.Contains(aws.ToString(e.CloudTrailEvent), "scheduler.amazonaws.com") {
				return true
			}
		}
		return false
	}, 20*time.Second, 1*time.Second,
		"scheduler-fired SendMessage must be recorded with invokedBy=scheduler.amazonaws.com")
}
