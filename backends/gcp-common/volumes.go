package gcpcommon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// BucketManager owns sockerless-managed GCS buckets backing Docker
// named volumes for every GCP backend (Cloud Run Jobs + Services,
// Cloud Functions). One bucket per Docker volume, named
// `sockerless-volume-<project>-<volume>`, with these labels:
//
//   - sockerless-managed=true  → identifies sockerless-owned buckets
//   - sockerless-volume-name=<sanitised-docker-name> → round-trip mapping
//
// Callers pass the storage client + project + region once; per-volume
// lookups are cached in-memory. Concurrent callers are safe.
type BucketManager struct {
	Client  *storage.Client
	Project string
	Region  string

	mu    sync.Mutex
	cache map[string]string // docker-name → bucket-name
}

// NewBucketManager wires a BucketManager against a storage client.
func NewBucketManager(client *storage.Client, project, region string) *BucketManager {
	return &BucketManager{
		Client:  client,
		Project: project,
		Region:  region,
		cache:   make(map[string]string),
	}
}

const (
	VolumeLabel        = "sockerless-managed"
	VolumeLabelValue   = "true"
	VolumeNameLabelKey = "sockerless-volume-name"
	VolumeBucketPrefix = "sockerless-volume-"
)

// ForVolume returns the bucket name bound to a Docker volume, provisioning
// a new bucket on first call.
func (m *BucketManager) ForVolume(ctx context.Context, volName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name, ok := m.cache[volName]; ok {
		return name, nil
	}

	if name, ok, err := m.findByVolumeNameLocked(ctx, volName); err != nil {
		return "", err
	} else if ok {
		m.cache[volName] = name
		return name, nil
	}

	bucket := BucketName(m.Project, volName)
	attrs := &storage.BucketAttrs{
		Location: m.Region,
		Labels: map[string]string{
			VolumeLabel:        VolumeLabelValue,
			VolumeNameLabelKey: SanitiseLabelValue(volName),
		},
	}
	if err := m.Client.Bucket(bucket).Create(ctx, m.Project, attrs); err != nil {
		return "", fmt.Errorf("create bucket %q for volume %q: %w", bucket, volName, err)
	}
	m.cache[volName] = bucket
	return bucket, nil
}

func (m *BucketManager) findByVolumeNameLocked(ctx context.Context, volName string) (string, bool, error) {
	it := m.Client.Buckets(ctx, m.Project)
	for {
		b, err := it.Next()
		if err == iterator.Done {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("list buckets: %w", err)
		}
		if BucketMatchesVolumeName(b, volName) {
			return b.Name, true, nil
		}
	}
}

// DeleteForVolume removes the bucket backing a Docker volume. If `force`
// is true, all objects are deleted first; otherwise a non-empty bucket
// returns the underlying GCS error.
func (m *BucketManager) DeleteForVolume(ctx context.Context, volName string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name, ok, err := m.findByVolumeNameLocked(ctx, volName)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	bucket := m.Client.Bucket(name)
	if force {
		it := bucket.Objects(ctx, nil)
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("list objects in bucket %q: %w", name, err)
			}
			if err := bucket.Object(attrs.Name).Delete(ctx); err != nil {
				return fmt.Errorf("delete object %q: %w", attrs.Name, err)
			}
		}
	}
	if err := bucket.Delete(ctx); err != nil {
		return fmt.Errorf("delete bucket %q: %w", name, err)
	}
	delete(m.cache, volName)
	return nil
}

// ListManaged returns every sockerless-managed bucket with a volume-name
// label. VolumeList / VolumePrune iterate these.
func (m *BucketManager) ListManaged(ctx context.Context) ([]*storage.BucketAttrs, error) {
	var out []*storage.BucketAttrs
	it := m.Client.Buckets(ctx, m.Project)
	for {
		b, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list buckets: %w", err)
		}
		if BucketIsManaged(b) && BucketVolumeName(b) != "" {
			out = append(out, b)
		}
	}
	return out, nil
}

// BucketMatchesVolumeName reports whether the bucket attrs belong to the
// given sockerless-managed Docker volume.
func BucketMatchesVolumeName(b *storage.BucketAttrs, volName string) bool {
	if b.Labels == nil {
		return false
	}
	if b.Labels[VolumeLabel] != VolumeLabelValue {
		return false
	}
	return b.Labels[VolumeNameLabelKey] == SanitiseLabelValue(volName)
}

// BucketIsManaged reports whether the bucket attrs carry the
// sockerless-managed label.
func BucketIsManaged(b *storage.BucketAttrs) bool {
	return b.Labels != nil && b.Labels[VolumeLabel] == VolumeLabelValue
}

// BucketVolumeName returns the Docker volume name encoded in the bucket
// labels, or empty if unmanaged.
func BucketVolumeName(b *storage.BucketAttrs) string {
	if b.Labels == nil {
		return ""
	}
	return b.Labels[VolumeNameLabelKey]
}

// BucketName returns a globally-unique bucket name for a Docker volume.
// GCS bucket names have tight constraints (3-63 chars, lowercase, digits,
// dashes, no leading/trailing dashes, no `..`). BUG-916: long volume
// names (e.g. gitlab-runner cache: `runner-tkopdswuw-project-81023556-
// concurrent-0-...-cache-...`) overflowed the 63-char limit even with
// a 30-char safe truncation, AND truncation produced names GCS rejected
// as "restricted" (truncated tail looks like a project ID prefix).
// Fix: use a stable sha256 hash for any volume name >12 chars, so
// total length is always predictable: 18 + project (capped 26) + 1 +
// hash[:16] = 61 chars max. Round-trip via VolumeNameLabelKey label.
func BucketName(project, volName string) string {
	proj := SanitiseLabelValue(project)
	if len(proj) > 26 {
		proj = proj[:26]
	}
	safe := SanitiseLabelValue(volName)
	if len(safe) > 12 {
		sum := sha256.Sum256([]byte(volName))
		safe = hex.EncodeToString(sum[:8]) // 16 hex chars
	}
	return fmt.Sprintf("%s%s-%s", VolumeBucketPrefix, proj, safe)
}

// SanitiseLabelValue returns a GCS-label-safe variant of the input
// (lowercase letters, digits, dashes, underscores only; max 63 chars).
func SanitiseLabelValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 63 {
		out = out[:63]
	}
	if out == "" {
		return "_"
	}
	return out
}
