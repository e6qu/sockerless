package aws_cli_test

import (
	"strings"
	"testing"
)

func TestEC2InstanceLifecycleCLI(t *testing.T) {
	out := runCLI(t, awsCLI("ec2", "create-vpc",
		"--cidr-block", "10.77.0.0/16",
		"--query", "Vpc.VpcId",
		"--output", "text"))
	vpcID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "create-subnet",
		"--vpc-id", vpcID,
		"--cidr-block", "10.77.1.0/24",
		"--query", "Subnet.SubnetId",
		"--output", "text"))
	subnetID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "create-security-group",
		"--group-name", "cli-instance-sg",
		"--description", "cli instance lifecycle",
		"--vpc-id", vpcID,
		"--query", "GroupId",
		"--output", "text"))
	sgID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "run-instances",
		"--image-id", "ami-cli1234",
		"--instance-type", "t3.micro",
		"--subnet-id", subnetID,
		"--security-group-ids", sgID,
		"--query", "Instances[0].InstanceId",
		"--output", "text"))
	instanceID := strings.TrimSpace(out)
	if instanceID == "" || !strings.HasPrefix(instanceID, "i-") {
		t.Fatalf("expected EC2 instance id, got %q", instanceID)
	}

	out = runCLI(t, awsCLI("ec2", "describe-instances",
		"--instance-ids", instanceID,
		"--query", "Reservations[0].Instances[0].State.Name",
		"--output", "text"))
	if strings.TrimSpace(out) != "running" {
		t.Fatalf("expected running instance, got %q", out)
	}

	runCLI(t, awsCLI("ec2", "stop-instances", "--instance-ids", instanceID))
	runCLI(t, awsCLI("ec2", "start-instances", "--instance-ids", instanceID))
	runCLI(t, awsCLI("ec2", "terminate-instances", "--instance-ids", instanceID))
}
