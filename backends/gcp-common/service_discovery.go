package gcpcommon

import "context"

// CloudDNSServiceDiscovery provides GCP Cloud DNS private zone service discovery.
// TODO: Implement with Cloud DNS SDK calls when DNS-based service discovery is needed.
type CloudDNSServiceDiscovery struct{}

func (d *CloudDNSServiceDiscovery) Register(_ context.Context, _ any) error {
	return nil
}

func (d *CloudDNSServiceDiscovery) Deregister(_ context.Context, _ string) error {
	return nil
}

func (d *CloudDNSServiceDiscovery) Resolve(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (d *CloudDNSServiceDiscovery) DriverName() string {
	return "cloud-dns"
}
