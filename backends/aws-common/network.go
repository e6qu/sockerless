package awscommon

import "context"

// VPCNetworkDriver provides cloud-native networking via AWS VPC.
// Maps Docker networks to VPC subnets + security groups, and container
// connections to ENI attachments in ECS awsvpc mode.
// TODO: Implement with EC2/ECS SDK when cloud-native networking is needed.
type VPCNetworkDriver struct{}

func (d *VPCNetworkDriver) EnsureNetwork(_ context.Context, _ any) (map[string]string, error) {
	return nil, nil
}

func (d *VPCNetworkDriver) DeleteNetwork(_ context.Context, _ string) error {
	return nil
}

func (d *VPCNetworkDriver) AttachContainer(_ context.Context, _, _ string) (any, error) {
	return nil, nil
}

func (d *VPCNetworkDriver) DetachContainer(_ context.Context, _, _ string) error {
	return nil
}

func (d *VPCNetworkDriver) DriverName() string {
	return "aws-vpc"
}
