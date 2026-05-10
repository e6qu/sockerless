// Per-backend translator: cloud-agnostic core.BackingSpec → ECS
// taskdef volume protobuf. Each named-volume bind in HostConfig.Binds
// resolves through s.storageBackings (registered with
// awscommon.EFSEphemeralDriver at startup), the driver returns a
// BackingSpec, and this translator materialises the cloud-native
// ecstypes.Volume + MountPoint pair.

package ecs

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	core "github.com/sockerless/backend-core"
)

// resolveBindToVolume turns a docker bind spec ("volName:/mnt[:ro]")
// into an ECS Volume + MountPoint pair via the storage-backing
// registry. Returns (nil, nil, nil) for invalid binds the caller
// should skip silently (matches the prior inline behaviour at
// taskdef.go:139). Empty Backing on the SharedVolume defaults to
// `efs-ephemeral` since that's the only backing ECS supports today.
func (s *Server) resolveBindToVolume(ctx context.Context, bind, fsID string) (*ecstypes.Volume, *ecstypes.MountPoint, error) {
	parts := strings.SplitN(bind, ":", 3)
	if len(parts) < 2 {
		return nil, nil, nil
	}
	volName := parts[0]
	apID, err := s.accessPointForVolume(ctx, volName)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve access point for volume %q: %w", volName, err)
	}
	readOnly := len(parts) == 3 && parts[2] == "ro"

	backing := core.BackingEFSEphemeral
	if sv := s.config.LookupSharedVolumeByName(volName); sv != nil && sv.Backing != "" {
		backing = core.StorageBacking(sv.Backing)
	}
	driver, err := s.storageBackings.Resolve(backing)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve storage backing for volume %q: %w", volName, err)
	}
	spec, err := driver.CloudSpec(core.SharedVolumeRef{
		Name:             volName,
		ContainerPath:    parts[1],
		Backing:          backing,
		EFSFileSystemID:  fsID,
		EFSAccessPointID: apID,
		ReadOnly:         readOnly,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("CloudSpec for volume %q: %w", volName, err)
	}
	vol, err := translateBackingSpecToECSVolume(volName, spec)
	if err != nil {
		return nil, nil, err
	}
	mount := &ecstypes.MountPoint{
		SourceVolume:  aws.String(volName),
		ContainerPath: aws.String(parts[1]),
		ReadOnly:      aws.Bool(readOnly),
	}
	return vol, mount, nil
}

// translateBackingSpecToECSVolume materialises the cloud-agnostic
// BackingSpec into the ECS task-definition volume shape.
func translateBackingSpecToECSVolume(name string, spec core.BackingSpec) (*ecstypes.Volume, error) {
	switch spec.Kind {
	case core.BackingEFSEphemeral:
		if spec.EFSEphemeral == nil {
			return nil, fmt.Errorf("ecs translator: efs-ephemeral spec missing payload")
		}
		return &ecstypes.Volume{
			Name: aws.String(name),
			EfsVolumeConfiguration: &ecstypes.EFSVolumeConfiguration{
				FileSystemId:      aws.String(spec.EFSEphemeral.FileSystemID),
				TransitEncryption: ecstypes.EFSTransitEncryptionEnabled,
				AuthorizationConfig: &ecstypes.EFSAuthorizationConfig{
					AccessPointId: aws.String(spec.EFSEphemeral.AccessPointID),
					Iam:           ecstypes.EFSAuthorizationConfigIAMEnabled,
				},
			},
		}, nil

	case core.BackingMemory:
		// ECS doesn't expose a RAM-backed volume primitive at the
		// task-def Volumes layer — tmpfs lives at the
		// ContainerDefinition.LinuxParameters.Tmpfs[] layer instead.
		// Fail loudly rather than silently substitute a Host{}
		// (disk-backed) volume; operators wanting RAM-backed scratch
		// should configure tmpfs on the container def directly, or
		// pick `Backing: emptyDir` if they want disk-backed
		// task-scoped scratch space.
		return nil, fmt.Errorf(
			"ecs translator: backing %q not supported via task-def Volumes — "+
				"ECS expresses RAM-backed mounts via "+
				"ContainerDefinition.LinuxParameters.Tmpfs[] (container-def layer), "+
				"not Volumes (task-def layer). For task-scoped disk scratch use "+
				"`Backing: emptyDir` instead",
			spec.Kind)
	}
	return nil, fmt.Errorf("ecs translator: backing %q not supported on ECS", spec.Kind)
}
