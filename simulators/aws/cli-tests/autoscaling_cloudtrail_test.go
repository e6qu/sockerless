package aws_cli_test

import (
	"strings"
	"testing"
)

func TestAutoScalingGroupLifecycleCLI(t *testing.T) {
	out := runCLI(t, awsCLI("ec2", "create-vpc",
		"--cidr-block", "10.82.0.0/16",
		"--query", "Vpc.VpcId",
		"--output", "text"))
	vpcID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "create-subnet",
		"--vpc-id", vpcID,
		"--cidr-block", "10.82.1.0/24",
		"--availability-zone", "us-east-1a",
		"--query", "Subnet.SubnetId",
		"--output", "text"))
	subnetID := strings.TrimSpace(out)

	runCLI(t, awsCLI("autoscaling", "create-launch-configuration",
		"--launch-configuration-name", "cli-lc",
		"--image-id", "ami-cli-asg",
		"--instance-type", "t3.micro"))
	runCLI(t, awsCLI("autoscaling", "create-auto-scaling-group",
		"--auto-scaling-group-name", "cli-asg",
		"--launch-configuration-name", "cli-lc",
		"--min-size", "1",
		"--max-size", "2",
		"--desired-capacity", "1",
		"--vpc-zone-identifier", subnetID))

	out = runCLI(t, awsCLI("autoscaling", "describe-auto-scaling-groups",
		"--auto-scaling-group-names", "cli-asg",
		"--query", "AutoScalingGroups[0].Instances[0].InstanceId",
		"--output", "text"))
	if !strings.HasPrefix(strings.TrimSpace(out), "i-") {
		t.Fatalf("expected materialized EC2 instance id, got %q", out)
	}

	runCLI(t, awsCLI("autoscaling", "set-desired-capacity",
		"--auto-scaling-group-name", "cli-asg",
		"--desired-capacity", "2"))
	out = runCLI(t, awsCLI("autoscaling", "describe-scaling-activities",
		"--auto-scaling-group-name", "cli-asg",
		"--query", "Activities[0].StatusCode",
		"--output", "text"))
	if strings.TrimSpace(out) != "Successful" {
		t.Fatalf("expected successful scaling activity, got %q", out)
	}

	runCLI(t, awsCLI("autoscaling", "delete-auto-scaling-group",
		"--auto-scaling-group-name", "cli-asg",
		"--force-delete"))
	runCLI(t, awsCLI("autoscaling", "delete-launch-configuration",
		"--launch-configuration-name", "cli-lc"))
}

func TestCloudTrailRecordsAPICallsCLI(t *testing.T) {
	runCLI(t, awsCLI("s3api", "create-bucket", "--bucket", "cli-cloudtrail-bucket"))
	runCLI(t, awsCLI("cloudtrail", "create-trail",
		"--name", "cli-trail",
		"--s3-bucket-name", "cli-cloudtrail-bucket",
		"--s3-key-prefix", "trail-logs"))
	runCLI(t, awsCLI("cloudtrail", "start-logging", "--name", "cli-trail"))
	runCLI(t, awsCLI("ec2", "create-vpc", "--cidr-block", "10.83.0.0/16"))

	out := runCLI(t, awsCLI("cloudtrail", "lookup-events",
		"--lookup-attributes", "AttributeKey=EventName,AttributeValue=CreateVpc",
		"--query", "Events[0].EventName",
		"--output", "text"))
	if strings.TrimSpace(out) != "CreateVpc" {
		t.Fatalf("expected CreateVpc CloudTrail event, got %q", out)
	}

	out = runCLI(t, awsCLI("s3api", "list-objects-v2",
		"--bucket", "cli-cloudtrail-bucket",
		"--prefix", "trail-logs/AWSLogs/123456789012/CloudTrail/us-east-1/",
		"--query", "Contents[0].Key",
		"--output", "text"))
	if !strings.Contains(strings.TrimSpace(out), "cli-trail_") {
		t.Fatalf("expected delivered CloudTrail log object, got %q", out)
	}

	runCLI(t, awsCLI("cloudtrail", "stop-logging", "--name", "cli-trail"))
	runCLI(t, awsCLI("cloudtrail", "delete-trail", "--name", "cli-trail"))
}
