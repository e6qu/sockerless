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

// LambdaSharedMountPath is the canonical local mount path for the
// single FileSystemConfig sockerless attaches to every sub-task Lambda
// function. Lambda enforces `/mnt/[A-Za-z0-9_.\-]+` on
// FileSystemConfig.LocalMountPath.
const LambdaSharedMountPath = "/mnt/sockerless-shared"

// fileSystemConfigsForBinds translates Docker bind specs
// (`volName-or-source:dst[:ro]`) into Lambda primitives, subject to two
// hard Lambda constraints: at most one FileSystemConfig per function,
// and `localMountPath` matching `/mnt/[A-Za-z0-9_.\-]+`. See
// `specs/CLOUD_RESOURCE_MAPPING.md` § "Lambda bind-mount translation"
// for the canonical mapping.
//
// Returns the (≤ 1) FileSystemConfig + a slice of `dst=mnt-target`
// strings that the caller emits as `SOCKERLESS_LAMBDA_BIND_LINKS` for
// the in-Lambda bootstrap to materialise as symlinks before the user
// entrypoint runs.
//
// Host-path binds are handled by:
//
//  1. `/var/run/docker.sock` — silently dropped (no docker socket on Lambda).
//  2. Source path matches a configured SharedVolume.ContainerPath →
//     translate to that SharedVolume's named-volume reference (same as ECS).
//  3. Source path is a sub-path of a mapped SharedVolume's source → drop
//     (the parent EFS mount already exposes it via subdirectory).
//  4. Anything else → reject (no host fs on Lambda).
//
// Multiple Docker volumes that share an access point ARN collapse to one
// FileSystemConfig. Multiple distinct access points in a single
// CreateFunction call are rejected with a clear error pointing at the
// Lambda single-FSC constraint.
func (s *Server) fileSystemConfigsForBinds(ctx context.Context, binds []string) ([]lambdatypes.FileSystemConfig, []string, error) {
	if len(binds) == 0 {
		return nil, nil, nil
	}
	if len(s.config.SubnetIDs) == 0 {
		return nil, nil, fmt.Errorf("lambda named volumes require the function to run in a VPC; set SOCKERLESS_LAMBDA_SUBNETS")
	}

	type resolvedBind struct {
		volName string
		dst     string
		apID    string
		apARN   string
		subpath string
	}

	seen := make(map[string]struct{})
	resolved := make([]resolvedBind, 0, len(binds))
	for _, b := range binds {
		parts := strings.SplitN(b, ":", 3)
		if len(parts) < 2 {
			return nil, nil, fmt.Errorf("invalid bind %q", b)
		}
		volName, dst := parts[0], parts[1]
		if volName == "/var/run/docker.sock" {
			continue
		}
		var subpath string
		if strings.HasPrefix(volName, "/") || strings.HasPrefix(volName, ".") {
			if sv := s.config.LookupSharedVolumeBySourcePath(volName); sv != nil {
				volName = sv.Name
				subpath = sv.EFSSubpath
			} else if isSubPathOfSharedVolume(volName, s.config.SharedVolumes) {
				continue
			} else {
				return nil, nil, fmt.Errorf("host-path binds are not supported on Lambda; use a named volume or configure SOCKERLESS_LAMBDA_SHARED_VOLUMES")
			}
		} else if sv := s.config.LookupSharedVolumeByName(volName); sv != nil {
			subpath = sv.EFSSubpath
		}
		if _, dup := seen[volName+":"+dst]; dup {
			continue
		}
		seen[volName+":"+dst] = struct{}{}
		apID, err := s.accessPointForVolume(ctx, volName)
		if err != nil {
			return nil, nil, fmt.Errorf("provision access point for %q: %w", volName, err)
		}
		apARN, err := s.accessPointARN(ctx, apID)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve access point ARN %s: %w", apID, err)
		}
		resolved = append(resolved, resolvedBind{volName: volName, dst: dst, apID: apID, apARN: apARN, subpath: subpath})
	}
	if len(resolved) == 0 {
		return nil, nil, nil
	}

	// Lambda permits at most one FileSystemConfig per function. All
	// resolved binds must share an access point ARN.
	canonicalARN := resolved[0].apARN
	for _, r := range resolved[1:] {
		if r.apARN != canonicalARN {
			return nil, nil, fmt.Errorf("lambda allows at most one FileSystemConfig per function, but binds reference multiple access points (%s and %s); collapse the volumes onto a single sockerless-managed EFS access point with sub-paths", canonicalARN, r.apARN)
		}
	}

	fsc := lambdatypes.FileSystemConfig{
		Arn:            aws.String(canonicalARN),
		LocalMountPath: aws.String(LambdaSharedMountPath),
	}

	// Build BIND_LINKS entries as `<dst>=<mount-target>`. Each Docker
	// bind's destination becomes a symlink the bootstrap creates before
	// the user entrypoint runs.
	links := make([]string, 0, len(resolved))
	linkSeen := make(map[string]struct{}, len(resolved))
	for _, r := range resolved {
		target := LambdaSharedMountPath
		if r.subpath != "" {
			target = LambdaSharedMountPath + "/" + r.subpath
		}
		key := r.dst + "=" + target
		if _, dup := linkSeen[key]; dup {
			continue
		}
		linkSeen[key] = struct{}{}
		links = append(links, key)
	}
	return []lambdatypes.FileSystemConfig{fsc}, links, nil
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
