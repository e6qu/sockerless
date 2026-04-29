package lambda

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	awscommon "github.com/sockerless/aws-common"
)

// EFS-backed named-volume provisioning for Lambda.
//
// Lambda supports cross-invocation durable storage via
// `Function.FileSystemConfigs[]` — each entry pairs an EFS access-point
// ARN with a local mount path inside the function container. The
// requirement is Lambda-in-VPC + EFS mount targets in the function's
// subnets.
//
// Implementation reuses awscommon.EFSManager (shared with ECS) so both
// backends agree on the sockerless-managed filesystem / access-point
// tagging conventions. Volume CRUD delegates to the manager; the
// ContainerCreate path translates `HostConfig.Binds` into
// `FileSystemConfigs[]` entries appended to the CreateFunctionInput.

// volumeState embeds the shared EFSManager. Initialised by NewServer
// once the EFS client + VPC config are available.
type volumeState struct {
	efs *awscommon.EFSManager
}

func (s *Server) accessPointForVolume(ctx context.Context, volName string) (string, error) {
	// If the volume matches a pre-configured shared volume (declared
	// via SOCKERLESS_LAMBDA_SHARED_VOLUMES), use its EFS access point
	// directly — that's how the runner-Lambda and the spawned sub-task
	// share a workspace via EFS without sockerless having to provision
	// a fresh access point per docker run. Mirror of the ECS backend's
	// same-named function.
	if sv := s.config.LookupSharedVolumeByName(volName); sv != nil {
		return sv.AccessPointID, nil
	}
	return s.efs.AccessPointForVolume(ctx, volName)
}

func (s *Server) deleteAccessPointForVolume(ctx context.Context, volName string) error {
	return s.efs.DeleteAccessPointForVolume(ctx, volName)
}

func (s *Server) listManagedAccessPoints(ctx context.Context) ([]efstypes.AccessPointDescription, error) {
	return s.efs.ListManagedAccessPoints(ctx)
}

// fileSystemConfigsForBinds parses Docker bind specs
// (`volName:/mnt[:ro]`), provisions an access point per unique named
// volume, and returns a slice of Lambda FileSystemConfig entries. Host-
// path binds are rejected. Lambda's FileSystemConfigs requires
// Lambda-in-VPC with matching subnets — caller must validate.
func (s *Server) fileSystemConfigsForBinds(ctx context.Context, binds []string) ([]lambdatypes.FileSystemConfig, error) {
	if len(binds) == 0 {
		return nil, nil
	}
	if len(s.config.SubnetIDs) == 0 {
		return nil, fmt.Errorf("lambda named volumes require the function to run in a VPC; set SOCKERLESS_LAMBDA_SUBNETS")
	}

	// Build FileSystemConfigs[] for the bound volumes.
	//
	// Three special cases for host-path binds (mirror of the ECS
	// backend's bind-mount handling — same semantics, same env-var
	// shape, just routed through Lambda's FileSystemConfigs instead
	// of ECS's EFSVolumeConfiguration):
	//
	//   1. `/var/run/docker.sock` — drop silently (no socket on Lambda).
	//   2. Source path matches a configured SharedVolume.ContainerPath
	//      → translate to that SharedVolume's named-volume reference;
	//      the spawned sub-task (typically dispatched cross-backend to
	//      ECS) mounts the same EFS access point.
	//   3. Source path is a sub-path of a mapped SharedVolume → drop
	//      (the parent EFS mount already exposes it via subdirectory).
	seen := make(map[string]struct{})
	out := make([]lambdatypes.FileSystemConfig, 0, len(binds))
	for _, b := range binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid bind %q", b)
		}
		volName, mountPath := parts[0], parts[1]
		if volName == "/var/run/docker.sock" {
			continue
		}
		if strings.HasPrefix(volName, "/") || strings.HasPrefix(volName, ".") {
			if sv := s.config.LookupSharedVolumeBySourcePath(volName); sv != nil {
				volName = sv.Name
			} else if isSubPathOfSharedVolume(volName, s.config.SharedVolumes) {
				continue
			} else {
				return nil, fmt.Errorf("host-path binds are not supported on Lambda; use a named volume or configure SOCKERLESS_LAMBDA_SHARED_VOLUMES")
			}
		}
		if _, dup := seen[volName]; dup {
			// One FileSystemConfig per volume; Lambda rejects duplicates.
			continue
		}
		seen[volName] = struct{}{}
		apID, err := s.accessPointForVolume(ctx, volName)
		if err != nil {
			return nil, fmt.Errorf("provision access point for %q: %w", volName, err)
		}
		apARN, err := s.accessPointARN(ctx, apID)
		if err != nil {
			return nil, fmt.Errorf("resolve access point ARN %s: %w", apID, err)
		}
		out = append(out, lambdatypes.FileSystemConfig{
			Arn:            aws.String(apARN),
			LocalMountPath: aws.String(mountPath),
		})
	}
	return out, nil
}

// accessPointARN resolves an access-point ID to its ARN via a single
// DescribeAccessPoints call. Lambda's FileSystemConfigs wants the full
// ARN, not just the ID. Operator-provisioned access points (passed via
// SOCKERLESS_LAMBDA_SHARED_VOLUMES) don't carry the sockerless-managed
// tag, so we look them up by ID directly rather than filtering through
// the managed list.
func (s *Server) accessPointARN(ctx context.Context, apID string) (string, error) {
	ap, err := s.efs.DescribeAccessPoint(ctx, apID)
	if err != nil {
		return "", err
	}
	return aws.ToString(ap.AccessPointArn), nil
}
