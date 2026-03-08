package azurecommon

import (
	"context"
	"io"
)

// ACAExecDriver provides cloud-native exec via Azure Container Apps console connect.
// TODO: Implement with Container Apps SDK exec/console API when agent-free exec is needed.
type ACAExecDriver struct{}

func (d *ACAExecDriver) Exec(_ context.Context, _ string, _ []string, _ bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *ACAExecDriver) Attach(_ context.Context, _ string, _ bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *ACAExecDriver) Supported(_ context.Context, _ string) bool {
	return false
}

func (d *ACAExecDriver) DriverName() string {
	return "aca-console"
}
