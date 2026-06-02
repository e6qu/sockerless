package main

import (
	"fmt"
	"testing"
)

func TestCloudTrailLookupEventsReturnsNewestMatchesFirst(t *testing.T) {
	events := make([]CloudTrailEvent, 0, 61)
	for i := 0; i < 60; i++ {
		events = append(events, CloudTrailEvent{
			EventId:     fmt.Sprintf("event-old-%02d", i),
			EventName:   "DescribeInstances",
			EventSource: "ec2.amazonaws.com",
			EventTime:   "2026-06-02T15:00:00Z",
			Username:    "sockerless",
		})
	}
	events = append(events, CloudTrailEvent{
		EventId:     "event-new",
		EventName:   "CreateVpc",
		EventSource: "ec2.amazonaws.com",
		EventTime:   "2026-06-02T15:01:00Z",
		Username:    "sockerless",
	})

	out := cloudTrailLookupEvents(events, nil, 50)
	if got := out[0]["EventName"]; got != "CreateVpc" {
		t.Fatalf("newest event must be first; got %v", got)
	}

	out = cloudTrailLookupEvents(events, []cloudTrailLookupAttribute{
		{AttributeKey: "EventName", AttributeValue: "CreateVpc"},
	}, 50)
	if len(out) != 1 {
		t.Fatalf("expected one filtered CreateVpc event, got %d", len(out))
	}
}
