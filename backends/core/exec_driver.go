package core

import (
	"context"
	"io"
)

// CloudExecDriver provides cloud-native command execution and stream attachment
// for containers running on managed cloud services. This is the fallback when
// no sockerless agent is connected to the container.
//
// Each cloud provider implements this with its own exec mechanism:
//   - AWS: ECS ExecuteCommand (SSM Session Manager)
//   - GCP: Cloud Run Jobs don't support exec; Cloud Run services have limited support
//   - Azure: Container Apps console connect (az containerapp exec)
//
// The default NoOpCloudExecDriver returns not-implemented errors, which is the
// current behavior when no agent is connected. Cloud backends can plug in
// their cloud-native driver to enable exec without an agent.
type CloudExecDriver interface {
	// Exec runs a command in a running container via cloud-native APIs and
	// returns a bidirectional stream for stdin/stdout.
	// Returns the stream and any error. The caller reads/writes the stream
	// and the exit code is delivered when the stream closes.
	Exec(ctx context.Context, containerID string, cmd []string, tty bool) (io.ReadWriteCloser, error)

	// Attach connects to a running container's primary process via
	// cloud-native APIs and returns a bidirectional stream.
	Attach(ctx context.Context, containerID string, tty bool) (io.ReadWriteCloser, error)

	// Supported returns true if cloud-native exec is available for the
	// given container. Some clouds may not support exec for certain
	// container types (e.g., GCP Cloud Run Jobs).
	Supported(ctx context.Context, containerID string) bool

	// DriverName returns the cloud exec driver name
	// (e.g., "ecs-exec", "aca-console", "none").
	DriverName() string
}

// NoOpCloudExecDriver is the default that does not support cloud-native exec.
// When no agent is connected, exec/attach operations return not-implemented errors.
type NoOpCloudExecDriver struct{}

func (d *NoOpCloudExecDriver) Exec(_ context.Context, _ string, _ []string, _ bool) (io.ReadWriteCloser, error) {
	return nil, &ErrCloudExecNotSupported{}
}

func (d *NoOpCloudExecDriver) Attach(_ context.Context, _ string, _ bool) (io.ReadWriteCloser, error) {
	return nil, &ErrCloudExecNotSupported{}
}

func (d *NoOpCloudExecDriver) Supported(_ context.Context, _ string) bool {
	return false
}

func (d *NoOpCloudExecDriver) DriverName() string {
	return "none"
}

// ErrCloudExecNotSupported is returned when cloud-native exec is not available.
type ErrCloudExecNotSupported struct{}

func (e *ErrCloudExecNotSupported) Error() string {
	return "cloud-native exec is not supported; connect an agent or configure a cloud exec driver"
}
