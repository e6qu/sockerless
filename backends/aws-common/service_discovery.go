package awscommon

import "context"

// CloudMapServiceDiscovery provides AWS Cloud Map service discovery for ECS tasks.
// TODO: Implement with AWS Cloud Map SDK calls when service discovery is needed.
type CloudMapServiceDiscovery struct{}

func (d *CloudMapServiceDiscovery) Register(_ context.Context, _ any) error {
	return nil
}

func (d *CloudMapServiceDiscovery) Deregister(_ context.Context, _ string) error {
	return nil
}

func (d *CloudMapServiceDiscovery) Resolve(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (d *CloudMapServiceDiscovery) DriverName() string {
	return "cloud-map"
}
