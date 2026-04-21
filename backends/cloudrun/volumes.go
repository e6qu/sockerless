package cloudrun

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

// Phase 92 — GCS-backed named-volume + named-volume bind-mount
// provisioning for Cloud Run (Jobs today; Services when the v2 sim
// route ships).
//
// Docker volume semantics on Cloud Run map to GCS buckets:
//
//  - One GCS bucket per named volume, named `sockerless-volume-<id>`.
//    Labels (`sockerless-managed=true` + `sockerless-volume-name=<docker-name>`)
//    identify sockerless-owned buckets so VolumeList filters correctly.
//  - Bind specs `volName:/mnt[:ro]` translate at task-launch time into
//    a RevisionTemplate Volume with a `Gcs{Bucket}` source + a
//    Container VolumeMount at `/mnt`.
//
// Host-path bind specs (`/h:/c`) stay rejected — Cloud Run containers
// have no host filesystem to bind from.
//
// GCS's flat-namespace + eventual-consistency semantics are weaker
// than EFS's POSIX semantics; operators needing `O_APPEND` / POSIX
// locking should wait for Filestore (a future Phase 92.1).

const (
	gcsVolumeLabel        = "sockerless-managed"
	gcsVolumeLabelValue   = "true"
	gcsVolumeNameLabelKey = "sockerless-volume-name"
	gcsVolumeBucketPrefix = "sockerless-volume-"
)

// gcsVolumeState caches volume-name → bucket-name lookups. Lives on
// Server; initialised in NewServer.
type gcsVolumeState struct {
	gcsBucketMu    sync.Mutex
	gcsBucketCache map[string]string // volume-name → bucket-name
}

// bucketForVolume returns the bucket name bound to a Docker volume
// name, provisioning a new bucket on first call. Safe for concurrent
// callers.
func (s *Server) bucketForVolume(ctx context.Context, volName string) (string, error) {
	s.gcsBucketMu.Lock()
	defer s.gcsBucketMu.Unlock()

	if name, ok := s.gcsBucketCache[volName]; ok {
		return name, nil
	}

	if name, ok, err := s.findBucketByVolumeName(ctx, volName); err != nil {
		return "", err
	} else if ok {
		s.gcsBucketCache[volName] = name
		return name, nil
	}

	bucket := gcsBucketName(s.config.Project, volName)
	attrs := &storage.BucketAttrs{
		Location: s.config.Region,
		Labels: map[string]string{
			gcsVolumeLabel:        gcsVolumeLabelValue,
			gcsVolumeNameLabelKey: sanitiseGCSLabelValue(volName),
		},
	}
	if err := s.gcp.Storage.Bucket(bucket).Create(ctx, s.config.Project, attrs); err != nil {
		return "", fmt.Errorf("create bucket %q for volume %q: %w", bucket, volName, err)
	}
	s.gcsBucketCache[volName] = bucket
	return bucket, nil
}

func (s *Server) findBucketByVolumeName(ctx context.Context, volName string) (string, bool, error) {
	it := s.gcp.Storage.Buckets(ctx, s.config.Project)
	for {
		b, err := it.Next()
		if err == iterator.Done {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("list buckets: %w", err)
		}
		if bucketMatchesVolumeName(b, volName) {
			return b.Name, true, nil
		}
	}
}

func bucketMatchesVolumeName(b *storage.BucketAttrs, volName string) bool {
	if b.Labels == nil {
		return false
	}
	if b.Labels[gcsVolumeLabel] != gcsVolumeLabelValue {
		return false
	}
	return b.Labels[gcsVolumeNameLabelKey] == sanitiseGCSLabelValue(volName)
}

func bucketIsManaged(b *storage.BucketAttrs) bool {
	return b.Labels != nil && b.Labels[gcsVolumeLabel] == gcsVolumeLabelValue
}

func bucketVolumeName(b *storage.BucketAttrs) string {
	if b.Labels == nil {
		return ""
	}
	return b.Labels[gcsVolumeNameLabelKey]
}

// gcsBucketName returns a globally-unique bucket name for a Docker
// volume. GCS bucket names have tight constraints (3-63 chars,
// lowercase, digits, dashes, no leading/trailing dashes, no `..`).
// The project prefix + docker-name-sanitise keeps per-project
// uniqueness without exposing the user's volume name verbatim.
func gcsBucketName(project, volName string) string {
	safe := sanitiseGCSLabelValue(volName)
	// 30 chars of sanitised volume-name + 23-ish for project prefix
	// keeps us under the 63 char limit for realistic project IDs.
	if len(safe) > 30 {
		safe = safe[:30]
	}
	return fmt.Sprintf("%s%s-%s", gcsVolumeBucketPrefix, sanitiseGCSLabelValue(project), safe)
}

// sanitiseGCSLabelValue returns a GCS-label-safe variant of the input
// (lowercase letters, digits, dashes, underscores only; max 63 chars).
func sanitiseGCSLabelValue(s string) string {
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

// deleteBucketForVolume removes the GCS bucket backing a Docker
// volume. If `force` is true, all objects are deleted first;
// otherwise a non-empty bucket returns the underlying GCS error.
func (s *Server) deleteBucketForVolume(ctx context.Context, volName string, force bool) error {
	s.gcsBucketMu.Lock()
	defer s.gcsBucketMu.Unlock()

	name, ok, err := s.findBucketByVolumeName(ctx, volName)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	bucket := s.gcp.Storage.Bucket(name)
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
	delete(s.gcsBucketCache, volName)
	return nil
}

// listManagedBuckets returns every sockerless-managed GCS bucket
// with a volume-name label. VolumeList / VolumePrune iterate these.
func (s *Server) listManagedBuckets(ctx context.Context) ([]*storage.BucketAttrs, error) {
	var out []*storage.BucketAttrs
	it := s.gcp.Storage.Buckets(ctx, s.config.Project)
	for {
		b, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list buckets: %w", err)
		}
		if bucketIsManaged(b) && bucketVolumeName(b) != "" {
			out = append(out, b)
		}
	}
	return out, nil
}
