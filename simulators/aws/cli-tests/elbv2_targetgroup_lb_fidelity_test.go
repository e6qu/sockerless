package aws_cli_test

import (
	"strings"
	"testing"
)

// TestELBv2TargetGroupLBFidelityCLI covers the target-group Matcher/protocol_version
// round-trip and the load-balancer SetIpAddressType via the aws CLI.
func TestELBv2TargetGroupLBFidelityCLI(t *testing.T) {
	q := func(args ...string) string { return strings.TrimSpace(runCLI(t, awsCLI(args...))) }

	vpcID := q("ec2", "create-vpc", "--cidr-block", "10.152.0.0/16", "--query", "Vpc.VpcId", "--output", "text")
	subnetID := q("ec2", "create-subnet", "--vpc-id", vpcID, "--cidr-block", "10.152.1.0/24",
		"--availability-zone", "us-east-1a", "--query", "Subnet.SubnetId", "--output", "text")

	// HTTP target group with an explicit matcher.
	tgArn := q("elbv2", "create-target-group", "--name", "cli-tg-fid", "--protocol", "HTTP", "--port", "80",
		"--vpc-id", vpcID, "--target-type", "ip", "--matcher", "HttpCode=200-299",
		"--query", "TargetGroups[0].TargetGroupArn", "--output", "text")
	out := q("elbv2", "describe-target-groups", "--target-group-arns", tgArn,
		"--query", "TargetGroups[0].[Matcher.HttpCode,ProtocolVersion]", "--output", "text")
	if f := strings.Fields(out); len(f) != 2 || f[0] != "200-299" || f[1] != "HTTP1" {
		t.Fatalf("target group matcher/protocol_version: got %q, want '200-299 HTTP1'", out)
	}

	// NLB: enforce-SG default + SetIpAddressType.
	lbArn := q("elbv2", "create-load-balancer", "--name", "cli-nlb-fid", "--type", "network",
		"--subnets", subnetID, "--query", "LoadBalancers[0].LoadBalancerArn", "--output", "text")
	if v := q("elbv2", "describe-load-balancers", "--load-balancer-arns", lbArn,
		"--query", "LoadBalancers[0].EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic", "--output", "text"); v != "on" {
		t.Fatalf("NLB enforce-sg default: got %q, want on", v)
	}
	runCLI(t, awsCLI("elbv2", "set-ip-address-type", "--load-balancer-arn", lbArn, "--ip-address-type", "dualstack"))
	if v := q("elbv2", "describe-load-balancers", "--load-balancer-arns", lbArn,
		"--query", "LoadBalancers[0].IpAddressType", "--output", "text"); v != "dualstack" {
		t.Fatalf("ip_address_type after set: got %q, want dualstack", v)
	}
}
