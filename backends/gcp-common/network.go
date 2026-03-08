package gcpcommon

import "context"

// VPCNetworkDriver provides cloud-native networking via GCP VPC.
// Maps Docker networks to VPC subnets + firewall rules, and container
// connections to Serverless VPC Connectors for Cloud Run.
// TODO: Implement with Compute/VPC SDK when cloud-native networking is needed.
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
	return "gcp-vpc"
}
