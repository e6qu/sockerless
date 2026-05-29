package aws_cli_test

import (
	"strings"
	"testing"
)

func TestELBv2LoadBalancerCLI(t *testing.T) {
	out := runCLI(t, awsCLI("ec2", "create-vpc",
		"--cidr-block", "10.91.0.0/16",
		"--query", "Vpc.VpcId",
		"--output", "text"))
	vpcID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "create-subnet",
		"--vpc-id", vpcID,
		"--cidr-block", "10.91.1.0/24",
		"--availability-zone", "us-east-1a",
		"--query", "Subnet.SubnetId",
		"--output", "text"))
	subnet1 := strings.TrimSpace(out)
	out = runCLI(t, awsCLI("ec2", "create-subnet",
		"--vpc-id", vpcID,
		"--cidr-block", "10.91.2.0/24",
		"--availability-zone", "us-east-1b",
		"--query", "Subnet.SubnetId",
		"--output", "text"))
	subnet2 := strings.TrimSpace(out)
	out = runCLI(t, awsCLI("ec2", "create-security-group",
		"--group-name", "cli-elbv2-sg",
		"--description", "cli elbv2",
		"--vpc-id", vpcID,
		"--query", "GroupId",
		"--output", "text"))
	sgID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("elbv2", "create-load-balancer",
		"--name", "cli-lb",
		"--type", "application",
		"--scheme", "internet-facing",
		"--subnets", subnet1, subnet2,
		"--security-groups", sgID,
		"--tags", "Key=env,Value=cli",
		"--query", "LoadBalancers[0].LoadBalancerArn",
		"--output", "text"))
	lbArn := strings.TrimSpace(out)
	if !strings.Contains(lbArn, ":loadbalancer/app/cli-lb/") {
		t.Fatalf("expected ELBv2 load balancer ARN, got %q", lbArn)
	}

	out = runCLI(t, awsCLI("elbv2", "create-target-group",
		"--name", "cli-tg",
		"--protocol", "HTTP",
		"--port", "80",
		"--vpc-id", vpcID,
		"--target-type", "ip",
		"--query", "TargetGroups[0].TargetGroupArn",
		"--output", "text"))
	tgArn := strings.TrimSpace(out)
	if !strings.Contains(tgArn, ":targetgroup/cli-tg/") {
		t.Fatalf("expected ELBv2 target group ARN, got %q", tgArn)
	}

	runCLI(t, awsCLI("elbv2", "register-targets",
		"--target-group-arn", tgArn,
		"--targets", "Id=10.91.1.25,Port=80"))
	out = runCLI(t, awsCLI("elbv2", "describe-target-health",
		"--target-group-arn", tgArn,
		"--query", "TargetHealthDescriptions[0].TargetHealth.State",
		"--output", "text"))
	if strings.TrimSpace(out) != "healthy" {
		t.Fatalf("expected healthy target, got %q", out)
	}

	out = runCLI(t, awsCLI("elbv2", "create-listener",
		"--load-balancer-arn", lbArn,
		"--protocol", "HTTP",
		"--port", "80",
		"--default-actions", "Type=forward,TargetGroupArn="+tgArn,
		"--query", "Listeners[0].ListenerArn",
		"--output", "text"))
	listenerArn := strings.TrimSpace(out)
	if !strings.Contains(listenerArn, ":listener/app/cli-lb/") {
		t.Fatalf("expected ELBv2 listener ARN, got %q", listenerArn)
	}

	out = runCLI(t, awsCLI("elbv2", "describe-load-balancers",
		"--load-balancer-arns", lbArn,
		"--query", "LoadBalancers[0].State.Code",
		"--output", "text"))
	if strings.TrimSpace(out) != "active" {
		t.Fatalf("expected active load balancer, got %q", out)
	}

	out = runCLI(t, awsCLI("elbv2", "describe-listener-attributes",
		"--listener-arn", listenerArn,
		"--query", "Attributes[?Key=='routing.http.response.server.enabled'].Value|[0]",
		"--output", "text"))
	if strings.TrimSpace(out) != "true" {
		t.Fatalf("expected default listener server header attribute, got %q", out)
	}
	out = runCLI(t, awsCLI("elbv2", "modify-listener-attributes",
		"--listener-arn", listenerArn,
		"--attributes", "Key=routing.http.response.server.enabled,Value=false",
		"--query", "Attributes[?Key=='routing.http.response.server.enabled'].Value|[0]",
		"--output", "text"))
	if strings.TrimSpace(out) != "false" {
		t.Fatalf("expected modified listener server header attribute, got %q", out)
	}

	runCLI(t, awsCLI("elbv2", "add-tags",
		"--resource-arns", lbArn,
		"--tags", "Key=phase,Value=cli"))
	out = runCLI(t, awsCLI("elbv2", "describe-tags",
		"--resource-arns", lbArn,
		"--query", "TagDescriptions[0].Tags[?Key=='phase'].Value|[0]",
		"--output", "text"))
	if strings.TrimSpace(out) != "cli" {
		t.Fatalf("expected phase tag from describe-tags, got %q", out)
	}

	runCLI(t, awsCLI("elbv2", "delete-listener", "--listener-arn", listenerArn))
	runCLI(t, awsCLI("elbv2", "deregister-targets",
		"--target-group-arn", tgArn,
		"--targets", "Id=10.91.1.25,Port=80"))
	runCLI(t, awsCLI("elbv2", "delete-target-group", "--target-group-arn", tgArn))
	runCLI(t, awsCLI("elbv2", "delete-load-balancer", "--load-balancer-arn", lbArn))
}
