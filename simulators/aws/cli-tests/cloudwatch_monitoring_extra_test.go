package aws_cli_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cwCLIUsesAwsJSON reports whether the installed aws CLI / botocore sends the
// CloudWatch monitoring operations over awsJson1.0 (X-Amz-Target
// GraniteServiceVersion20100801.<Op>) rather than the legacy awsQuery protocol.
//
// The dataset / OTel-enrichment / managed-rule / report / widget operations are
// recent additions. A current botocore sends them as awsJson1.0, which the
// simulator serves from its awsJson router (and the Go SDK reaches the matching
// rpc-v2-cbor router). The macOS-bundled aws CLI 2.26.6 predates the migration
// and still emits these as awsQuery (Action=<Op>, form params) — a protocol the
// simulator routes through a separate query router these recent ops are not
// registered on, so the call would 404 "InvalidAction". The SDK test covers the
// contract hook for every operation; the CLI tests below skip on a query-only
// CLI rather than asserting against a protocol the local CLI does not speak.
func cwCLIUsesAwsJSON() bool {
	// list-metrics needs no required arguments, so botocore builds and logs the
	// request (we point at an unreachable port and only read the --debug log of
	// the request it constructed). All CloudWatch operations share one protocol,
	// so list-metrics is a faithful probe for the whole service.
	cmd := exec.Command("aws", "cloudwatch", "list-metrics",
		"--endpoint-url", "http://127.0.0.1:1", "--region", "us-east-1", "--debug")
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID=test", "AWS_SECRET_ACCESS_KEY=test", "AWS_PAGER=")
	out, _ := cmd.CombinedOutput()
	return strings.Contains(string(out), "GraniteServiceVersion20100801.")
}

// TestCloudWatchCLI_ManagedInsightRules covers put-managed-insight-rules creating
// a managed rule over a resource ARN and list-managed-insight-rules reading it
// back with its template name and rule state.
func TestCloudWatchCLI_ManagedInsightRules(t *testing.T) {
	if !cwCLIHasSubcommand("put-managed-insight-rules") {
		t.Skip("aws CLI lacks put-managed-insight-rules")
	}
	if !cwCLIUsesAwsJSON() {
		t.Skip("aws CLI uses the legacy awsQuery protocol for these ops; SDK test covers the hook")
	}
	resourceARN := "arn:aws:ec2:us-east-1:000000000000:vpc/vpc-cli12345"
	rules := []map[string]any{{
		"TemplateName": "VPC-Flow-Logs-By-Source",
		"ResourceARN":  resourceARN,
	}}
	b, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("marshal managed rules: %v", err)
	}
	f := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(f, b, 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	runCLI(t, awsCLI("cloudwatch", "put-managed-insight-rules", "--managed-rules", "file://"+f))

	template := strings.TrimSpace(runCLI(t, awsCLI("cloudwatch", "list-managed-insight-rules",
		"--resource-arn", resourceARN,
		"--query", "ManagedRules[0].TemplateName", "--output", "text")))
	if template != "VPC-Flow-Logs-By-Source" {
		t.Fatalf("ManagedRules[0].TemplateName = %q, want VPC-Flow-Logs-By-Source", template)
	}
	state := strings.TrimSpace(runCLI(t, awsCLI("cloudwatch", "list-managed-insight-rules",
		"--resource-arn", resourceARN,
		"--query", "ManagedRules[0].RuleState.State", "--output", "text")))
	if state != "ENABLED" {
		t.Fatalf("ManagedRules[0].RuleState.State = %q, want ENABLED", state)
	}
}

// TestCloudWatchCLI_InsightRuleReport covers get-insight-rule-report over an
// existing insight rule: an honestly-empty report (zero aggregate, no
// contributors) with the real shape.
func TestCloudWatchCLI_InsightRuleReport(t *testing.T) {
	if !cwCLIHasSubcommand("get-insight-rule-report") {
		t.Skip("aws CLI lacks get-insight-rule-report")
	}
	if !cwCLIUsesAwsJSON() {
		t.Skip("aws CLI uses the legacy awsQuery protocol for these ops; SDK test covers the hook")
	}
	name := "cli-report-rule"
	def := `{"Schema":{"Name":"CloudWatchLogRule","Version":1},"LogGroupNames":["/aws/lambda/x"],"Contribution":{"Keys":["$.requestId"]},"AggregateOn":"Count"}`
	runCLI(t, awsCLI("cloudwatch", "put-insight-rule", "--rule-name", name, "--rule-definition", def))
	defer func() { _ = awsCLI("cloudwatch", "delete-insight-rules", "--rule-names", name).Run() }()

	agg := strings.TrimSpace(runCLI(t, awsCLI("cloudwatch", "get-insight-rule-report",
		"--rule-name", name,
		"--start-time", "2024-01-01T00:00:00Z",
		"--end-time", "2024-01-01T00:05:00Z",
		"--period", "60",
		"--query", "AggregateValue", "--output", "text")))
	if agg != "0.0" && agg != "0" {
		t.Fatalf("AggregateValue = %q, want 0", agg)
	}
	count := strings.TrimSpace(runCLI(t, awsCLI("cloudwatch", "get-insight-rule-report",
		"--rule-name", name,
		"--start-time", "2024-01-01T00:00:00Z",
		"--end-time", "2024-01-01T00:05:00Z",
		"--period", "60",
		"--query", "length(MetricDatapoints)", "--output", "text")))
	if count != "5" {
		t.Fatalf("MetricDatapoints length = %q, want 5", count)
	}
}

// TestCloudWatchCLI_MetricWidgetImage covers get-metric-widget-image returning a
// base64-encoded PNG that decodes to the expected PNG magic bytes.
func TestCloudWatchCLI_MetricWidgetImage(t *testing.T) {
	if !cwCLIHasSubcommand("get-metric-widget-image") {
		t.Skip("aws CLI lacks get-metric-widget-image")
	}
	if !cwCLIUsesAwsJSON() {
		t.Skip("aws CLI uses the legacy awsQuery protocol for these ops; SDK test covers the hook")
	}
	widget := `{"metrics":[["AWS/EC2","CPUUtilization","InstanceId","i-1234567890abcdef0"]],"width":250,"height":150,"stat":"Average"}`
	// --output text emits the MetricWidgetImage blob as base64 text.
	b64 := strings.TrimSpace(runCLI(t, awsCLI("cloudwatch", "get-metric-widget-image",
		"--metric-widget", widget, "--output-format", "png",
		"--query", "MetricWidgetImage", "--output", "text")))
	if b64 == "" {
		t.Fatal("get-metric-widget-image returned no image bytes")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode base64 image: %v", err)
	}
	if len(raw) < 8 || string(raw[1:4]) != "PNG" {
		t.Fatalf("image is not a PNG (first bytes %v)", raw)
	}
}
