package gcpcommon

import (
	"context"
	"io"
)

// CloudRunExecDriver provides cloud-native exec for Cloud Run services.
// Cloud Run Jobs do not support interactive exec; Cloud Run services have
// limited support via gcloud run services proxy.
// TODO: Implement when agent-free exec is needed for Cloud Run services.
type CloudRunExecDriver struct{}

func (d *CloudRunExecDriver) Exec(_ context.Context, _ string, _ []string, _ bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *CloudRunExecDriver) Attach(_ context.Context, _ string, _ bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *CloudRunExecDriver) Supported(_ context.Context, _ string) bool {
	return false
}

func (d *CloudRunExecDriver) DriverName() string {
	return "cloudrun-exec"
}
