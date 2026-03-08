package azurecommon

import "context"

// VNetNetworkDriver provides cloud-native networking via Azure VNet.
// Maps Docker networks to VNet subnets + NSGs, and container connections
// to Container Apps Environment networking.
// TODO: Implement with Network SDK when cloud-native networking is needed.
type VNetNetworkDriver struct{}

func (d *VNetNetworkDriver) EnsureNetwork(_ context.Context, _ any) (map[string]string, error) {
	return nil, nil
}

func (d *VNetNetworkDriver) DeleteNetwork(_ context.Context, _ string) error {
	return nil
}

func (d *VNetNetworkDriver) AttachContainer(_ context.Context, _, _ string) (any, error) {
	return nil, nil
}

func (d *VNetNetworkDriver) DetachContainer(_ context.Context, _, _ string) error {
	return nil
}

func (d *VNetNetworkDriver) DriverName() string {
	return "azure-vnet"
}
