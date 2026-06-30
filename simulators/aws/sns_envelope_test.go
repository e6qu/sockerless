package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSNSNotificationEnvelopeValidJSON verifies snsNotificationEnvelope
// always emits parseable JSON, even when the embedded CloudWatch alarm
// message contains control characters that fmt.Sprintf("%q") would render
// as \x escapes (invalid in JSON).
func TestSNSNotificationEnvelopeValidJSON(t *testing.T) {
	// Simulate a CloudWatch alarm JSON payload whose AlarmName contains a
	// control character. json.Marshal escapes it as \u0001, so the message
	// string itself is valid JSON.
	message := `{"AlarmName":"bad\u0001char","Description":"line1\nline2"}`
	envelopeStr := snsNotificationEnvelope("arn:aws:sns:us-east-1:123456789012:t", "msg-id", "subj", message)

	var envelope map[string]any
	if err := json.Unmarshal([]byte(envelopeStr), &envelope); err != nil {
		t.Fatalf("envelope is not valid JSON: %v\nenvelope: %s", err, envelopeStr)
	}
	if envelope["Type"] != "Notification" {
		t.Errorf("expected Type=Notification, got %v", envelope["Type"])
	}
	inner, ok := envelope["Message"].(string)
	if !ok {
		t.Fatalf("Message should be a string, got %T", envelope["Message"])
	}
	var alarm map[string]any
	if err := json.Unmarshal([]byte(inner), &alarm); err != nil {
		t.Fatalf("inner Message is not valid JSON: %v\ninner: %s", err, inner)
	}
	if alarm["AlarmName"] != "bad\x01char" {
		t.Errorf("alarm name did not round-trip: %v", alarm["AlarmName"])
	}
	if !strings.Contains(envelopeStr, `"Timestamp"`) {
		t.Error("envelope should include a Timestamp field")
	}
}
