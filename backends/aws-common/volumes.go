package awscommon

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

// EFSManager owns sockerless-managed EFS filesystem + access points
// backing Docker named volumes for every AWS backend that can mount
// EFS (ECS today, Lambda planned). One filesystem per backend
// instance, one access point per Docker volume, tagged:
//
//   - sockerless-managed=true       → identifies sockerless-owned resources
//   - sockerless-volume-name=<name> → maps access point back to Docker name
//
// Operators who already own an EFS filesystem can pre-set `AgentEFSID`
// to skip the ensure-filesystem step; the access-point provisioning
// still runs against that filesystem.
type EFSManager struct {
	Client         *efs.Client
	AgentEFSID     string
	Subnets        []string
	SecurityGroups []string
	PollInterval   time.Duration
	InstanceID     string

	efsOnce      sync.Once
	efsCachedID  string
	efsEnsureErr error

	apMu    sync.Mutex
	apCache map[string]string // docker-name → access-point-id
}

// EFSManagerConfig wires per-backend knobs into a manager. Callers pass
// either an `AgentEFSID` to reuse an operator-owned filesystem, or
// `Subnets` + `SecurityGroups` so the manager provisions one itself.
type EFSManagerConfig struct {
	AgentEFSID     string
	Subnets        []string
	SecurityGroups []string
	PollInterval   time.Duration
	InstanceID     string
}

// NewEFSManager wires an EFSManager against an EFS client.
func NewEFSManager(client *efs.Client, cfg EFSManagerConfig) *EFSManager {
	poll := cfg.PollInterval
	if poll == 0 {
		poll = 2 * time.Second
	}
	return &EFSManager{
		Client:         client,
		AgentEFSID:     cfg.AgentEFSID,
		Subnets:        cfg.Subnets,
		SecurityGroups: cfg.SecurityGroups,
		PollInterval:   poll,
		InstanceID:     cfg.InstanceID,
		apCache:        make(map[string]string),
	}
}

const (
	VolumeManagedTagKey   = "sockerless-managed"
	VolumeManagedTagValue = "true"
	VolumeNameTagKey      = "sockerless-volume-name"
)

// EnsureFilesystem returns the ID of a sockerless-owned EFS filesystem,
// creating one (and its per-subnet mount targets) on first call. Safe
// for concurrent callers.
func (m *EFSManager) EnsureFilesystem(ctx context.Context) (string, error) {
	if m.AgentEFSID != "" {
		return m.AgentEFSID, nil
	}
	m.efsOnce.Do(func() {
		fsID, err := m.discoverOrCreateFilesystem(ctx)
		if err != nil {
			m.efsEnsureErr = err
			return
		}
		if err := m.ensureMountTargets(ctx, fsID); err != nil {
			m.efsEnsureErr = err
			return
		}
		m.efsCachedID = fsID
	})
	if m.efsEnsureErr != nil {
		return "", m.efsEnsureErr
	}
	return m.efsCachedID, nil
}

func (m *EFSManager) discoverOrCreateFilesystem(ctx context.Context) (string, error) {
	listed, err := m.Client.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{})
	if err != nil {
		return "", fmt.Errorf("describe filesystems: %w", err)
	}
	for _, fs := range listed.FileSystems {
		for _, t := range fs.Tags {
			if aws.ToString(t.Key) == VolumeManagedTagKey && aws.ToString(t.Value) == VolumeManagedTagValue {
				return aws.ToString(fs.FileSystemId), nil
			}
		}
	}

	created, err := m.Client.CreateFileSystem(ctx, &efs.CreateFileSystemInput{
		CreationToken: aws.String(fmt.Sprintf("sockerless-%s", m.InstanceID)),
		Tags: []efstypes.Tag{
			{Key: aws.String(VolumeManagedTagKey), Value: aws.String(VolumeManagedTagValue)},
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
		out, err := m.Client.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{FileSystemId: aws.String(fsID)})
		if err == nil && len(out.FileSystems) > 0 && out.FileSystems[0].LifeCycleState == efstypes.LifeCycleStateAvailable {
			return fsID, nil
		}
		time.Sleep(m.PollInterval)
	}
	return fsID, fmt.Errorf("filesystem %s did not reach available state", fsID)
}

func (m *EFSManager) ensureMountTargets(ctx context.Context, fsID string) error {
	out, err := m.Client.DescribeMountTargets(ctx, &efs.DescribeMountTargetsInput{FileSystemId: aws.String(fsID)})
	if err != nil {
		return fmt.Errorf("describe mount targets: %w", err)
	}
	existing := make(map[string]struct{}, len(out.MountTargets))
	for _, mt := range out.MountTargets {
		existing[aws.ToString(mt.SubnetId)] = struct{}{}
	}

	for _, subnet := range m.Subnets {
		if _, ok := existing[subnet]; ok {
			continue
		}
		in := &efs.CreateMountTargetInput{
			FileSystemId: aws.String(fsID),
			SubnetId:     aws.String(subnet),
		}
		if len(m.SecurityGroups) > 0 {
			in.SecurityGroups = m.SecurityGroups
		}
		if _, err := m.Client.CreateMountTarget(ctx, in); err != nil {
			return fmt.Errorf("create mount target in %s: %w", subnet, err)
		}
	}
	return nil
}

// AccessPointForVolume returns the access-point ID bound to a Docker
// volume name, creating one if needed. Safe for concurrent callers.
func (m *EFSManager) AccessPointForVolume(ctx context.Context, volName string) (string, error) {
	m.apMu.Lock()
	defer m.apMu.Unlock()

	if id, ok := m.apCache[volName]; ok {
		return id, nil
	}

	fsID, err := m.EnsureFilesystem(ctx)
	if err != nil {
		return "", err
	}

	if id, ok, err := m.findAccessPointByVolumeNameLocked(ctx, fsID, volName); err != nil {
		return "", err
	} else if ok {
		m.apCache[volName] = id
		return id, nil
	}

	rootDir := "/sockerless/volumes/" + SanitiseVolumePath(volName)
	created, err := m.Client.CreateAccessPoint(ctx, &efs.CreateAccessPointInput{
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
			{Key: aws.String(VolumeManagedTagKey), Value: aws.String(VolumeManagedTagValue)},
			{Key: aws.String(VolumeNameTagKey), Value: aws.String(volName)},
			{Key: aws.String("Name"), Value: aws.String(volName)},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create access point for %q: %w", volName, err)
	}
	id := aws.ToString(created.AccessPointId)
	m.apCache[volName] = id
	return id, nil
}

func (m *EFSManager) findAccessPointByVolumeNameLocked(ctx context.Context, fsID, volName string) (string, bool, error) {
	out, err := m.Client.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{FileSystemId: aws.String(fsID)})
	if err != nil {
		return "", false, fmt.Errorf("describe access points: %w", err)
	}
	for _, ap := range out.AccessPoints {
		if APMatchesVolumeName(ap, volName) {
			return aws.ToString(ap.AccessPointId), true, nil
		}
	}
	return "", false, nil
}

// DeleteAccessPointForVolume removes the access point bound to a volume
// name. The filesystem is left in place so other volumes keep working.
func (m *EFSManager) DeleteAccessPointForVolume(ctx context.Context, volName string) error {
	m.apMu.Lock()
	defer m.apMu.Unlock()

	fsID, err := m.EnsureFilesystem(ctx)
	if err != nil {
		return err
	}
	id, ok, err := m.findAccessPointByVolumeNameLocked(ctx, fsID, volName)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if _, err := m.Client.DeleteAccessPoint(ctx, &efs.DeleteAccessPointInput{AccessPointId: aws.String(id)}); err != nil {
		return fmt.Errorf("delete access point %s: %w", id, err)
	}
	delete(m.apCache, volName)
	return nil
}

// ListManagedAccessPoints returns every access point on the sockerless
// filesystem whose tags mark it as a Docker-volume-owned access point.
func (m *EFSManager) ListManagedAccessPoints(ctx context.Context) ([]efstypes.AccessPointDescription, error) {
	fsID, err := m.EnsureFilesystem(ctx)
	if err != nil {
		return nil, err
	}
	out, err := m.Client.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{FileSystemId: aws.String(fsID)})
	if err != nil {
		return nil, err
	}
	var managed []efstypes.AccessPointDescription
	for _, ap := range out.AccessPoints {
		if APIsManaged(ap) && APVolumeName(ap) != "" {
			managed = append(managed, ap)
		}
	}
	return managed, nil
}

// APMatchesVolumeName reports whether the access point belongs to the
// given sockerless-managed Docker volume.
func APMatchesVolumeName(ap efstypes.AccessPointDescription, volName string) bool {
	if !APIsManaged(ap) {
		return false
	}
	for _, t := range ap.Tags {
		if aws.ToString(t.Key) == VolumeNameTagKey && aws.ToString(t.Value) == volName {
			return true
		}
	}
	return false
}

// APIsManaged reports whether the access point carries the
// sockerless-managed tag.
func APIsManaged(ap efstypes.AccessPointDescription) bool {
	for _, t := range ap.Tags {
		if aws.ToString(t.Key) == VolumeManagedTagKey && aws.ToString(t.Value) == VolumeManagedTagValue {
			return true
		}
	}
	return false
}

// APVolumeName returns the Docker volume name encoded in the access
// point tags, or empty if unmanaged.
func APVolumeName(ap efstypes.AccessPointDescription) string {
	for _, t := range ap.Tags {
		if aws.ToString(t.Key) == VolumeNameTagKey {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

// SanitiseVolumePath returns a filesystem-safe variant of a Docker
// volume name or host bind path for use as an EFS RootDirectory path
// segment. Only `a-zA-Z0-9._-` survive; other characters become `_`.
func SanitiseVolumePath(s string) string {
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
