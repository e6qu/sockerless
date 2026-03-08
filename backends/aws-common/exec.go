package awscommon

import (
	"context"
	"io"
)

// ECSExecDriver provides cloud-native exec via ECS ExecuteCommand + SSM Session Manager.
// TODO: Implement with ECS SDK ExecuteCommand + SSM WebSocket when agent-free exec is needed.
type ECSExecDriver struct{}

func (d *ECSExecDriver) Exec(_ context.Context, _ string, _ []string, _ bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *ECSExecDriver) Attach(_ context.Context, _ string, _ bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func (d *ECSExecDriver) Supported(_ context.Context, _ string) bool {
	return false
}

func (d *ECSExecDriver) DriverName() string {
	return "ecs-exec"
}
