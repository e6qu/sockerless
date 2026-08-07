package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Elastic Load Balancing, Amazon CloudWatch and AWS Certificate Manager address
// their resources three different ways, and each is pinned here against a
// request shaped the way a real client sends it.

func iamDeriveQuery(operation, version, form string) []string {
	r := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader("Action="+operation+"&Version="+version+"&"+form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return iamDerivedResourceARNs(r, "elasticloadbalancing", operation, "us-east-1", "123456789012")
}

func iamDeriveJSON(service, operation, body string) []string {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-amz-json-1.1")
	return iamDerivedResourceARNs(r, service, operation, "us-east-1", "123456789012")
}

func iamAssertDerived(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("derived %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("derived %v, want %v", got, want)
		}
	}
}

// A rule's ARN arrives inside a priority entry rather than under a member of
// its own, so the parameter name is a path. The coverage probe fills each
// member with one scalar and therefore cannot express that shape at all — it
// counts this operation as underived — which is why the real shape is pinned
// here rather than by tuning the probe to know about one operation.
func TestIAMResourceARNs_ELBReadsARuleARNNestedInAPriority(t *testing.T) {
	const rule = "arn:aws:elasticloadbalancing:us-east-1:123456789012:listener-rule/app/lb/0123456789abcdef/fedcba9876543210/1122334455667788"
	iamAssertDerived(t, iamDeriveQuery("SetRulePriorities", "2015-12-01",
		"RulePriorities.member.1.RuleArn="+rule+"&RulePriorities.member.1.Priority=1"), rule)
}

// A request that names several resources is authorized against all of them:
// creating a listener on a load balancer, forwarding to a target group, is a
// call about both.
func TestIAMResourceARNs_ELBAuthorizesEveryResourceARequestNames(t *testing.T) {
	const lb = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/probe/0123456789abcdef"
	const tg = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/probe/fedcba9876543210"
	iamAssertDerived(t, iamDeriveQuery("CreateListener", "2015-12-01",
		"LoadBalancerArn="+lb+"&TargetGroupArn="+tg), lb, tg)
}

// The previous-generation load balancer is addressed by name, and its ARN
// carries nothing else.
func TestIAMResourceARNs_ELBBuildsTheClassicLoadBalancerFromItsName(t *testing.T) {
	iamAssertDerived(t, iamDeriveQuery("ConfigureHealthCheck", "2012-06-01",
		"LoadBalancerName=classic-lb"),
		"arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/classic-lb")
}

// Amazon CloudWatch names its resources, and each type's name arrives under a
// member of its own.
func TestIAMResourceARNs_CloudWatchFillsTheNamedResource(t *testing.T) {
	iamAssertDerived(t, iamDeriveJSON("cloudwatch", "DescribeAlarms", `{"AlarmNames":["cpu-high"]}`),
		"arn:aws:cloudwatch:us-east-1:123456789012:alarm:cpu-high")
	iamAssertDerived(t, iamDeriveJSON("cloudwatch", "GetDashboard", `{"DashboardName":"ops"}`),
		"arn:aws:cloudwatch::123456789012:dashboard/ops")
	// An insight rule's ${InsightRuleName} arrives as RuleName.
	iamAssertDerived(t, iamDeriveJSON("cloudwatch", "DeleteInsightRules", `{"RuleNames":["contributors"]}`),
		"arn:aws:cloudwatch:us-east-1:123456789012:insight-rule/contributors")
}

// A tagging call names its target by ARN, and the reference lists every
// taggable type for it — which one the call is about is what the ARN says, so
// there is nothing to choose between them.
func TestIAMResourceARNs_TaggingTakesTheARNTheRequestNames(t *testing.T) {
	const alarm = "arn:aws:cloudwatch:us-east-1:123456789012:alarm:cpu-high"
	iamAssertDerived(t, iamDeriveJSON("cloudwatch", "TagResource",
		`{"ResourceARN":"`+alarm+`","Tags":[]}`), alarm)

	const certificate = "arn:aws:acm:us-east-1:123456789012:certificate/0123abcd-ef45-6789-abcd-ef0123456789"
	iamAssertDerived(t, iamDeriveJSON("acm", "AddTagsToCertificate",
		`{"CertificateArn":"`+certificate+`","Tags":[]}`), certificate)
}
