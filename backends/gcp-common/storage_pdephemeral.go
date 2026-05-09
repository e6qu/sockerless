// Package gcpcommon — pd-ephemeral storage backing driver.
//
// Compute Engine Persistent Disk attached to a Cloud Run / Cloud Run
// Jobs instance as an ephemeral volume — lifecycle bound to the task,
// underlying PD owned by sockerless. Used when the workload needs a
// real POSIX filesystem (e.g. a build cache that doesn't survive the
// task) without paying for an always-on NFS / Filestore.
//
// PreExec / PostExec are no-ops: the cloud attaches the disk during
// task startup and detaches at task end; no sockerless-side data sync
// is required.
package gcpcommon

import (
	"context"
	"fmt"

	core "github.com/sockerless/backend-core"
)

// PDEphemeralDriver implements core.StorageBackingDriver for the
// pd-ephemeral mount type.
type PDEphemeralDriver struct {
	// DefaultZone is used when SharedVolumeRef.PDZone is empty. Set by
	// backend startup to the configured Cloud Run region's primary
	// zone (typically `<region>-a`).
	DefaultZone string

	// DefaultDiskSizeGB is used when SharedVolumeRef.PDDiskSizeGB is
	// zero. Set by backend startup; conservative default = 10 GiB.
	DefaultDiskSizeGB int
}

// NewPDEphemeralDriver constructs the driver with backend-supplied
// defaults. Either default may be zero/empty; CloudSpec rejects a
// vol that resolves to zero size.
func NewPDEphemeralDriver(defaultZone string, defaultSizeGB int) *PDEphemeralDriver {
	return &PDEphemeralDriver{DefaultZone: defaultZone, DefaultDiskSizeGB: defaultSizeGB}
}

func (d *PDEphemeralDriver) Backing() core.StorageBacking { return core.BackingPDEphemeral }

func (d *PDEphemeralDriver) CloudSpec(vol core.SharedVolumeRef) (core.BackingSpec, error) {
	zone := vol.PDZone
	if zone == "" {
		zone = d.DefaultZone
	}
	size := vol.PDDiskSizeGB
	if size == 0 {
		size = d.DefaultDiskSizeGB
	}
	if size <= 0 {
		return core.BackingSpec{}, fmt.Errorf("pd-ephemeral: disk size must be > 0 (vol=%q)", vol.Name)
	}
	return core.BackingSpec{
		Kind: core.BackingPDEphemeral,
		PDEphemeral: &core.PDEphemeralSpec{
			DiskSizeGB: size,
			Zone:       zone,
		},
	}, nil
}

func (d *PDEphemeralDriver) PreExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath, remotePath string) (map[string][]string, error) {
	return nil, nil
}

func (d *PDEphemeralDriver) PostExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath string) error {
	return nil
}
