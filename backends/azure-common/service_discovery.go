package azurecommon

import "context"

// AzureDNSServiceDiscovery provides Azure DNS private zone service discovery.
// TODO: Implement with Azure DNS SDK calls when DNS-based service discovery is needed.
type AzureDNSServiceDiscovery struct{}

func (d *AzureDNSServiceDiscovery) Register(_ context.Context, _ any) error {
	return nil
}

func (d *AzureDNSServiceDiscovery) Deregister(_ context.Context, _ string) error {
	return nil
}

func (d *AzureDNSServiceDiscovery) Resolve(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (d *AzureDNSServiceDiscovery) DriverName() string {
	return "azure-dns"
}
