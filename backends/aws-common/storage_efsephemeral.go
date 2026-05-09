// Package awscommon — efs-ephemeral storage backing driver.
//
// EFS access point on a sockerless-managed filesystem, attached as an
// ephemeral mount to an ECS task / Lambda function. Lifecycle is bound
// to the task — the access point survives across tasks (its data is
// the durable workspace), but each task's mount is per-task.
//
// PreExec / PostExec are no-ops: the cloud attaches the access point
// during task startup; sockerless does no data sync (that's what gcs-sync
// is for on the GCP side).
package awscommon

import (
	"context"
	"fmt"

	core "github.com/sockerless/backend-core"
)

// EFSEphemeralDriver implements core.StorageBackingDriver for the
// efs-ephemeral mount type.
type EFSEphemeralDriver struct {
	// Manager resolves volume name → access point ID and ensures the
	// underlying filesystem exists. Backends share a single EFSManager
	// instance across the registry.
	Manager *EFSManager
}

// NewEFSEphemeralDriver constructs the driver wrapping the supplied
// EFSManager. Callers must have already ensured the manager's
// underlying filesystem (or set AgentEFSID for an operator-owned one).
func NewEFSEphemeralDriver(mgr *EFSManager) *EFSEphemeralDriver {
	return &EFSEphemeralDriver{Manager: mgr}
}

func (d *EFSEphemeralDriver) Backing() core.StorageBacking { return core.BackingEFSEphemeral }

func (d *EFSEphemeralDriver) CloudSpec(vol core.SharedVolumeRef) (core.BackingSpec, error) {
	if vol.EFSAccessPointID == "" {
		return core.BackingSpec{}, fmt.Errorf("efs-ephemeral: AccessPointID required (vol=%q)", vol.Name)
	}
	if vol.EFSFileSystemID == "" {
		return core.BackingSpec{}, fmt.Errorf("efs-ephemeral: FileSystemID required (vol=%q)", vol.Name)
	}
	return core.BackingSpec{
		Kind: core.BackingEFSEphemeral,
		EFSEphemeral: &core.EFSEphemeralSpec{
			FileSystemID:  vol.EFSFileSystemID,
			AccessPointID: vol.EFSAccessPointID,
			ReadOnly:      vol.ReadOnly,
		},
	}, nil
}

func (d *EFSEphemeralDriver) PreExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}

func (d *EFSEphemeralDriver) PostExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath string) error {
	return nil
}
