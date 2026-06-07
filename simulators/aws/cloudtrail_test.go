package main

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
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

// TestAWSEventSourceCoversAllServiceSlices pins the CloudTrail eventSource the
// sim records for every awsJson and query-protocol service slice it implements.
// Real CloudTrail labels each management event with the service's
// `<service>.amazonaws.com` endpoint; LookupEvents supports filtering by
// EventSource, so a wrong/generic source makes those filters silently miss.
func TestAWSEventSourceCoversAllServiceSlices(t *testing.T) {
	jsonCases := map[string]string{
		// X-Amz-Target service prefix -> eventSource
		"DynamoDB_20120810.PutItem":                           "dynamodb.amazonaws.com",
		"AmazonSQS.SendMessage":                               "sqs.amazonaws.com",
		"AmazonSSM.GetParameter":                              "ssm.amazonaws.com",
		"TrentService.Encrypt":                                "kms.amazonaws.com",
		"Kinesis_20131202.PutRecord":                          "kinesis.amazonaws.com",
		"Logs_20140328.FilterLogEvents":                       "logs.amazonaws.com",
		"GraniteServiceVersion20100801.PutMetricData":         "monitoring.amazonaws.com",
		"AWSEvents.PutEvents":                                 "events.amazonaws.com",
		"AWSGlue.GetDatabase":                                 "glue.amazonaws.com",
		"AWSStepFunctions.StartExecution":                     "states.amazonaws.com",
		"AWSWAF_20190729.GetWebACL":                           "wafv2.amazonaws.com",
		"CertificateManager.DescribeCertificate":              "acm.amazonaws.com",
		"CodeBuild_20161006.StartBuild":                       "codebuild.amazonaws.com",
		"AmazonEC2ContainerServiceV20141113.RunTask":          "ecs.amazonaws.com",
		"AmazonEC2ContainerRegistry_V20150921.DescribeImages": "ecr.amazonaws.com",
		"AnyScaleFrontendService.RegisterScalableTarget":      "application-autoscaling.amazonaws.com",
		"Route53AutoNaming_v20170314.CreateService":           "servicediscovery.amazonaws.com",
		"CloudTrail_20131101.LookupEvents":                    "cloudtrail.amazonaws.com",
		"secretsmanager.GetSecretValue":                       "secretsmanager.amazonaws.com",
	}
	for target, want := range jsonCases {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("X-Amz-Target", target)
		got, ok := awsEventSource(r)
		if !ok || got != want {
			t.Errorf("awsEventSource(X-Amz-Target=%q) = (%q, %v), want (%q, true)", target, got, ok, want)
		}
	}

	queryCases := map[string]string{
		"2016-11-15": "ec2.amazonaws.com",
		"2011-01-01": "autoscaling.amazonaws.com",
		"2010-03-31": "sns.amazonaws.com",
		"2015-12-01": "elasticloadbalancing.amazonaws.com",
		"2014-10-31": "rds.amazonaws.com",
		"2010-05-08": "iam.amazonaws.com",
		"2011-06-15": "sts.amazonaws.com",
	}
	for version, want := range queryCases {
		form := url.Values{"Action": {"X"}, "Version": {version}}
		r := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		got, ok := awsEventSource(r)
		if !ok || got != want {
			t.Errorf("awsEventSource(Version=%q) = (%q, %v), want (%q, true)", version, got, ok, want)
		}
	}

	// No fabrication: an unmapped service slice must report ok=false rather than
	// fall back to a generic source (the defect a default would reintroduce).
	unknownTarget := httptest.NewRequest("POST", "/", nil)
	unknownTarget.Header.Set("X-Amz-Target", "SomeNewServiceV2.DoThing")
	if src, ok := awsEventSource(unknownTarget); ok {
		t.Errorf("awsEventSource(unknown target) = (%q, true), want ok=false", src)
	}
	unknownQuery := httptest.NewRequest("POST", "/", strings.NewReader("Action=X&Version=1999-01-01"))
	unknownQuery.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if src, ok := awsEventSource(unknownQuery); ok {
		t.Errorf("awsEventSource(unknown version) = (%q, true), want ok=false", src)
	}
}
