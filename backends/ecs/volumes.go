package ecs

import (
	"context"

	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	awscommon "github.com/sockerless/aws-common"
)

// EFS-backed named-volume + bind-mount provisioning for ECS.
//
// Docker volume semantics on Fargate map to EFS access points: one
// filesystem per backend instance, one access point per named volume.
// The access point's RootDirectory gives each volume an isolated
// subdirectory on the shared filesystem.
//
// Implementation lives in backends/aws-common/volumes.go as
// awscommon.EFSManager so Lambda can share it.

// volumeState embeds the shared EFSManager. Initialised by NewServer
// once the EFS client and config are available.
type volumeState struct {
	efs *awscommon.EFSManager
}

func (s *Server) ensureEFSFilesystem(ctx context.Context) (string, error) {
	return s.efs.EnsureFilesystem(ctx)
}

func (s *Server) accessPointForVolume(ctx context.Context, volName string) (string, error) {
	return s.efs.AccessPointForVolume(ctx, volName)
}

func (s *Server) deleteAccessPointForVolume(ctx context.Context, volName string) error {
	return s.efs.DeleteAccessPointForVolume(ctx, volName)
}

func (s *Server) listManagedAccessPoints(ctx context.Context) ([]efstypes.AccessPointDescription, error) {
	return s.efs.ListManagedAccessPoints(ctx)
}
