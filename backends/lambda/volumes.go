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
	seen := make(map[string]struct{})
	out := make([]lambdatypes.FileSystemConfig, 0, len(binds))
	for _, b := range binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid bind %q", b)
		}
		volName, mountPath := parts[0], parts[1]
		if strings.HasPrefix(volName, "/") || strings.HasPrefix(volName, ".") {
			return nil, fmt.Errorf("host-path binds are not supported on Lambda; use a named volume")
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
// ARN, not just the ID.
func (s *Server) accessPointARN(ctx context.Context, apID string) (string, error) {
	fsID, err := s.efs.EnsureFilesystem(ctx)
	if err != nil {
		return "", err
	}
	aps, err := s.efs.ListManagedAccessPoints(ctx)
	if err != nil {
		return "", err
	}
	for _, ap := range aps {
		if aws.ToString(ap.AccessPointId) == apID && aws.ToString(ap.FileSystemId) == fsID {
			return aws.ToString(ap.AccessPointArn), nil
		}
	}
	return "", fmt.Errorf("access point %s not found on filesystem %s", apID, fsID)
}
