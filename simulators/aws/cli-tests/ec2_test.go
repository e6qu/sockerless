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

func TestEC2NatGatewayCLI(t *testing.T) {
	out := runCLI(t, awsCLI("ec2", "create-vpc",
		"--cidr-block", "10.78.0.0/16",
		"--query", "Vpc.VpcId",
		"--output", "text"))
	vpcID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "create-subnet",
		"--vpc-id", vpcID,
		"--cidr-block", "10.78.1.0/24",
		"--availability-zone", "us-east-1a",
		"--query", "Subnet.SubnetId",
		"--output", "text"))
	subnetID := strings.TrimSpace(out)

	out = runCLI(t, awsCLI("ec2", "allocate-address",
		"--domain", "vpc",
		"--query", "AllocationId",
		"--output", "text"))
	allocationID := strings.TrimSpace(out)
	if allocationID == "" || !strings.HasPrefix(allocationID, "eipalloc-") {
		t.Fatalf("expected EIP allocation id, got %q", allocationID)
	}

	out = runCLI(t, awsCLI("ec2", "describe-addresses",
		"--allocation-ids", allocationID,
		"--query", "Addresses[0].PublicIp",
		"--output", "text"))
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected allocated public IP, got %q", out)
	}

	out = runCLI(t, awsCLI("ec2", "create-nat-gateway",
		"--subnet-id", subnetID,
		"--allocation-id", allocationID,
		"--query", "NatGateway.NatGatewayId",
		"--output", "text"))
	natID := strings.TrimSpace(out)
	if natID == "" || !strings.HasPrefix(natID, "nat-") {
		t.Fatalf("expected NAT gateway id, got %q", natID)
	}

	out = runCLI(t, awsCLI("ec2", "describe-nat-gateways",
		"--nat-gateway-ids", natID,
		"--query", "NatGateways[0].State",
		"--output", "text"))
	if strings.TrimSpace(out) != "available" {
		t.Fatalf("expected available NAT gateway, got %q", out)
	}

	out = runCLI(t, awsCLI("ec2", "create-route-table",
		"--vpc-id", vpcID,
		"--query", "RouteTable.RouteTableId",
		"--output", "text"))
	routeTableID := strings.TrimSpace(out)

	runCLI(t, awsCLI("ec2", "create-route",
		"--route-table-id", routeTableID,
		"--destination-cidr-block", "0.0.0.0/0",
		"--nat-gateway-id", natID))

	out = runCLI(t, awsCLI("ec2", "describe-route-tables",
		"--route-table-ids", routeTableID,
		"--query", "RouteTables[0].Routes[?NatGatewayId=='"+natID+"'].NatGatewayId | [0]",
		"--output", "text"))
	if strings.TrimSpace(out) != natID {
		t.Fatalf("expected NAT gateway route, got %q", out)
	}

	runCLI(t, awsCLI("ec2", "delete-nat-gateway", "--nat-gateway-id", natID))
	runCLI(t, awsCLI("ec2", "release-address", "--allocation-id", allocationID))
}
