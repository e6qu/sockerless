package ecs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
)

// Phase 91 — EFS-backed named-volume + bind-mount provisioning for ECS.
//
// Docker volume semantics on Fargate map to EFS access points:
//
//  - One EFS filesystem per backend instance, tagged `sockerless-managed=true`.
//    The first VolumeCreate / bind-mount provision call lazily creates it
//    (and a mount target per configured subnet) and reuses it thereafter.
//  - One EFS access point per named volume (or per bind-mount path). The
//    access point's RootDirectory gives each volume an isolated subdirectory
//    on the shared filesystem. The `Name` tag carries the Docker volume
//    name so VolumeInspect / VolumeList / VolumeRemove can look up by name.
//
// Operators who already own an EFS filesystem can pre-set
// `SOCKERLESS_ECS_AGENT_EFS_ID` to skip the ensure-filesystem step; the
// access-point provisioning still runs against that filesystem.

// efsVolumeTag and efsManagedTag identify sockerless-owned EFS resources.
const (
	efsManagedTagKey    = "sockerless-managed"
	efsManagedTagValue  = "true"
	efsVolumeNameTagKey = "sockerless-volume-name"
)

// ensureEFSFilesystem returns the ID of a sockerless-owned EFS filesystem,
// creating one (and its per-subnet mount targets) on first call. Safe for
// concurrent callers.
func (s *Server) ensureEFSFilesystem(ctx context.Context) (string, error) {
	// Operator override wins.
	if s.config.AgentEFSID != "" {
		return s.config.AgentEFSID, nil
	}
	s.efsOnce.Do(func() {
		fsID, err := s.discoverOrCreateFilesystem(ctx)
		if err != nil {
			s.efsEnsureErr = err
			return
		}
		if err := s.ensureMountTargets(ctx, fsID); err != nil {
			s.efsEnsureErr = err
			return
		}
		s.efsCachedID = fsID
	})
	if s.efsEnsureErr != nil {
		return "", s.efsEnsureErr
	}
	return s.efsCachedID, nil
}

func (s *Server) discoverOrCreateFilesystem(ctx context.Context) (string, error) {
	listed, err := s.aws.EFS.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{})
	if err != nil {
		return "", fmt.Errorf("describe filesystems: %w", err)
	}
	for _, fs := range listed.FileSystems {
		for _, t := range fs.Tags {
			if aws.ToString(t.Key) == efsManagedTagKey && aws.ToString(t.Value) == efsManagedTagValue {
				return aws.ToString(fs.FileSystemId), nil
			}
		}
	}

	created, err := s.aws.EFS.CreateFileSystem(ctx, &efs.CreateFileSystemInput{
		CreationToken: aws.String(fmt.Sprintf("sockerless-%s", s.Desc.InstanceID)),
		Tags: []efstypes.Tag{
			{Key: aws.String(efsManagedTagKey), Value: aws.String(efsManagedTagValue)},
			{Key: aws.String("Name"), Value: aws.String("sockerless-volumes")},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create filesystem: %w", err)
	}
	fsID := aws.ToString(created.FileSystemId)

	// Poll until the filesystem is available. Fargate rejects mount
	// targets on a filesystem still in `creating`.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := s.aws.EFS.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{FileSystemId: aws.String(fsID)})
		if err == nil && len(out.FileSystems) > 0 && out.FileSystems[0].LifeCycleState == efstypes.LifeCycleStateAvailable {
			return fsID, nil
		}
		time.Sleep(s.config.PollInterval)
	}
	return fsID, fmt.Errorf("filesystem %s did not reach available state", fsID)
}

func (s *Server) ensureMountTargets(ctx context.Context, fsID string) error {
	out, err := s.aws.EFS.DescribeMountTargets(ctx, &efs.DescribeMountTargetsInput{FileSystemId: aws.String(fsID)})
	if err != nil {
		return fmt.Errorf("describe mount targets: %w", err)
	}
	existing := make(map[string]struct{}, len(out.MountTargets))
	for _, mt := range out.MountTargets {
		existing[aws.ToString(mt.SubnetId)] = struct{}{}
	}

	for _, subnet := range s.config.Subnets {
		if _, ok := existing[subnet]; ok {
			continue
		}
		in := &efs.CreateMountTargetInput{
			FileSystemId: aws.String(fsID),
			SubnetId:     aws.String(subnet),
		}
		if len(s.config.SecurityGroups) > 0 {
			in.SecurityGroups = s.config.SecurityGroups
		}
		if _, err := s.aws.EFS.CreateMountTarget(ctx, in); err != nil {
			return fmt.Errorf("create mount target in %s: %w", subnet, err)
		}
	}
	return nil
}

// accessPointForVolume returns the access-point ID bound to a Docker
// volume name, creating one if needed. Safe for concurrent callers.
func (s *Server) accessPointForVolume(ctx context.Context, volName string) (string, error) {
	s.efsAPMu.Lock()
	defer s.efsAPMu.Unlock()

	if id, ok := s.efsAPCache[volName]; ok {
		return id, nil
	}

	fsID, err := s.ensureEFSFilesystem(ctx)
	if err != nil {
		return "", err
	}

	if id, ok, err := s.findAccessPointByVolumeName(ctx, fsID, volName); err != nil {
		return "", err
	} else if ok {
		s.efsAPCache[volName] = id
		return id, nil
	}

	rootDir := "/sockerless/volumes/" + sanitiseVolumePath(volName)
	created, err := s.aws.EFS.CreateAccessPoint(ctx, &efs.CreateAccessPointInput{
		FileSystemId: aws.String(fsID),
		RootDirectory: &efstypes.RootDirectory{
			Path: aws.String(rootDir),
			CreationInfo: &efstypes.CreationInfo{
				OwnerUid:    aws.Int64(1000),
				OwnerGid:    aws.Int64(1000),
				Permissions: aws.String("0777"),
			},
		},
		Tags: []efstypes.Tag{
			{Key: aws.String(efsManagedTagKey), Value: aws.String(efsManagedTagValue)},
			{Key: aws.String(efsVolumeNameTagKey), Value: aws.String(volName)},
			{Key: aws.String("Name"), Value: aws.String(volName)},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create access point for %q: %w", volName, err)
	}
	id := aws.ToString(created.AccessPointId)
	s.efsAPCache[volName] = id
	return id, nil
}

func (s *Server) findAccessPointByVolumeName(ctx context.Context, fsID, volName string) (string, bool, error) {
	out, err := s.aws.EFS.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{FileSystemId: aws.String(fsID)})
	if err != nil {
		return "", false, fmt.Errorf("describe access points: %w", err)
	}
	for _, ap := range out.AccessPoints {
		if apMatchesVolumeName(ap, volName) {
			return aws.ToString(ap.AccessPointId), true, nil
		}
	}
	return "", false, nil
}

func apMatchesVolumeName(ap efstypes.AccessPointDescription, volName string) bool {
	if !apIsManaged(ap) {
		return false
	}
	for _, t := range ap.Tags {
		if aws.ToString(t.Key) == efsVolumeNameTagKey && aws.ToString(t.Value) == volName {
			return true
		}
	}
	return false
}

func apIsManaged(ap efstypes.AccessPointDescription) bool {
	for _, t := range ap.Tags {
		if aws.ToString(t.Key) == efsManagedTagKey && aws.ToString(t.Value) == efsManagedTagValue {
			return true
		}
	}
	return false
}

func apVolumeName(ap efstypes.AccessPointDescription) string {
	for _, t := range ap.Tags {
		if aws.ToString(t.Key) == efsVolumeNameTagKey {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

// sanitiseVolumePath returns a filesystem-safe variant of a Docker
// volume name or host bind path for use as an EFS RootDirectory path
// segment. Only `a-zA-Z0-9._-` survive; other characters become `_`.
func sanitiseVolumePath(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "_"
	}
	return out
}

// deleteAccessPointForVolume removes the access point bound to a volume
// name. The filesystem is left in place so other volumes keep working.
func (s *Server) deleteAccessPointForVolume(ctx context.Context, volName string) error {
	s.efsAPMu.Lock()
	defer s.efsAPMu.Unlock()

	fsID, err := s.ensureEFSFilesystem(ctx)
	if err != nil {
		return err
	}
	id, ok, err := s.findAccessPointByVolumeName(ctx, fsID, volName)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if _, err := s.aws.EFS.DeleteAccessPoint(ctx, &efs.DeleteAccessPointInput{AccessPointId: aws.String(id)}); err != nil {
		return fmt.Errorf("delete access point %s: %w", id, err)
	}
	delete(s.efsAPCache, volName)
	return nil
}

// listManagedAccessPoints returns every access point on the sockerless
// filesystem whose tags mark it as a Docker-volume-owned access point.
func (s *Server) listManagedAccessPoints(ctx context.Context) ([]efstypes.AccessPointDescription, error) {
	fsID, err := s.ensureEFSFilesystem(ctx)
	if err != nil {
		return nil, err
	}
	out, err := s.aws.EFS.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{FileSystemId: aws.String(fsID)})
	if err != nil {
		return nil, err
	}
	var managed []efstypes.AccessPointDescription
	for _, ap := range out.AccessPoints {
		if apIsManaged(ap) && apVolumeName(ap) != "" {
			managed = append(managed, ap)
		}
	}
	return managed, nil
}

// volumeState bundles the lazy filesystem ensure + per-volume access
// point cache. Lives on Server; initialised in NewServer.
type volumeState struct {
	efsOnce      sync.Once
	efsCachedID  string
	efsEnsureErr error

	efsAPMu    sync.Mutex
	efsAPCache map[string]string
}
