// Package gcpcommon — gcs-sync storage backing driver (DEFAULT for shared workspaces, Phase 123).
//
// Architecture: the JOB pod-Service mounts an in-memory emptyDir at the
// volume's container path. The runner-task's sockerless-backend (the
// PreExec caller) tars the equivalent local path on its side and uploads
// to a GCS object before forwarding the exec POST. The bootstrap-side
// envelope handler (in agent/cmd/sockerless-{cloudrun,gcf}-bootstrap)
// recognises the SOCKERLESS_WORKSPACE_OBJECT env hint, downloads the tar,
// untars to localPath, runs the subprocess, then tars + uploads back to
// the same object. Sockerless-backend's PostExec downloads + untars to
// pick up changes on the runner-task side and deletes the object.
//
// Why this and not GCSFuse: GCSFuse invalidates open file handles when
// an object is rewritten while a holder has it open (BUG-965). GH
// actions/runner rewrites _temp/event.json every step, hitting that
// path. gcs-sync sidesteps FUSE entirely — it's pure GCS SDK calls
// against a single tar object per exec, which has strong consistency.
//
// Why this and not always-on shared FS (NFS/Filestore/JuiceFS+Redis):
// user directive 2026-05-07 — zero-scaling, no-cost-when-not-in-use.
// GCS satisfies; persistent hardware does not.
//
// Cost shape: $0.02/GiB/mo for stored bytes; same-region ingress/egress
// free. For a CI workspace ≤ 100 MB, that's pennies a month — only
// charged when work is happening.
package gcpcommon

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// GCSSyncDriver implements core.StorageBackingDriver for the no-FUSE
// per-exec sync pattern. Use this as the workspace backing for any
// SharedVolume that's read/written by two different Cloud Run resources
// (e.g. github-runner-dispatcher's runner-task + JOB pod-Service).
//
// Construction: backends do `gcpcommon.NewGCSSyncDriver(ctx)`; the
// returned driver holds a long-lived storage client and is safe for
// concurrent use.
type GCSSyncDriver struct {
	client *storage.Client
}

// NewGCSSyncDriver constructs the driver with a storage client using
// Application Default Credentials. The client is reused across PreExec
// + PostExec calls.
func NewGCSSyncDriver(ctx context.Context) (*GCSSyncDriver, error) {
	cli, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("gcs-sync: storage client: %w", err)
	}
	return &GCSSyncDriver{client: cli}, nil
}

func (d *GCSSyncDriver) Backing() core.StorageBacking { return core.BackingGCSSync }

// CloudSpec emits an in-memory emptyDir spec — the JOB pod-Service mounts
// tmpfs at the volume's container path. Sync of that tmpfs against GCS
// happens at exec boundaries via PreExec/PostExec (this driver) and the
// matching bootstrap-side envelope handler.
func (d *GCSSyncDriver) CloudSpec(vol core.SharedVolumeRef) (core.BackingSpec, error) {
	if vol.GCSBucket == "" {
		return core.BackingSpec{}, errMissingBucket("gcs-sync", vol.Name)
	}
	return core.BackingSpec{
		Kind:     core.BackingGCSSync,
		EmptyDir: &core.EmptyDirSpec{Medium: "Memory"},
	}, nil
}

// PreExec tars localPath and uploads to gs://<bucket>/<objectName(volName, execID)>.
// Returns a list-valued env hint under SOCKERLESS_SYNC_VOLUMES with one
// per-volume triple (`name=path=gs://bucket/object`). The translator
// concatenates per-volume hints across multiple SharedVolumes before
// serialising so multi-volume execs (e.g. cells 5+6 with runner-workspace
// + runner-externals) don't clobber.
//
// localPath is the runner-task's local mount of the SharedVolume (e.g.
// /tmp/runner-work). If localPath does not exist, an empty tar is
// uploaded — the bootstrap will see an empty volume on restore, which
// is the correct behaviour for a job's first exec.
func (d *GCSSyncDriver) PreExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath string) (map[string][]string, error) {
	if err := validateRefForSync(vol); err != nil {
		return nil, err
	}
	obj := objectName(vol.Name, execID)

	w := d.client.Bucket(vol.GCSBucket).Object(obj).NewWriter(ctx)
	w.ContentType = "application/x-tar+gzip"
	if err := writeTarGzFromDir(w, localPath); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("gcs-sync PreExec: tar+upload %s → gs://%s/%s: %w", localPath, vol.GCSBucket, obj, err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gcs-sync PreExec: finalize gs://%s/%s: %w", vol.GCSBucket, obj, err)
	}

	triple := fmt.Sprintf("%s=%s=gs://%s/%s", vol.Name, vol.ContainerPath, vol.GCSBucket, obj)
	return map[string][]string{
		"SOCKERLESS_SYNC_VOLUMES": {triple},
	}, nil
}

// PostExec downloads gs://<bucket>/<objectName> and untars to localPath,
// then deletes the object. The download brings any modifications the
// bootstrap made during the subprocess back to the runner-task's local
// view; the delete keeps the bucket from accumulating per-exec tar
// objects (operators can also use a bucket lifecycle policy as belt-
// and-suspenders).
func (d *GCSSyncDriver) PostExec(ctx context.Context, vol core.SharedVolumeRef, execID, localPath string) error {
	if err := validateRefForSync(vol); err != nil {
		return err
	}
	obj := objectName(vol.Name, execID)

	r, err := d.client.Bucket(vol.GCSBucket).Object(obj).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			// Bootstrap didn't write a result back — possibly the
			// subprocess crashed or didn't modify the workspace. Treat as
			// a no-op rather than failing the exec response handling.
			return nil
		}
		return fmt.Errorf("gcs-sync PostExec: open gs://%s/%s: %w", vol.GCSBucket, obj, err)
	}
	defer func() { _ = r.Close() }()

	if err := readTarGzIntoDir(r, localPath); err != nil {
		return fmt.Errorf("gcs-sync PostExec: untar gs://%s/%s → %s: %w", vol.GCSBucket, obj, localPath, err)
	}

	// Best-effort cleanup. Failure here is non-fatal — the data plane
	// already returned.
	if err := d.client.Bucket(vol.GCSBucket).Object(obj).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		// log via the storage client's own diagnostics; caller sees nil.
		_ = err
	}
	return nil
}

// PruneStaleObjects removes workspace tar objects older than maxAge from
// the bucket. Operators can call this opportunistically (e.g. on
// dispatcher startup) to clean up after crashed jobs whose PostExec
// never ran. Object naming carries no timestamp so we filter by GCS
// Updated metadata.
func (d *GCSSyncDriver) PruneStaleObjects(ctx context.Context, bucket string, prefix string) error {
	it := d.client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("gcs-sync prune: list gs://%s: %w", bucket, err)
		}
		_ = d.client.Bucket(bucket).Object(attrs.Name).Delete(ctx)
	}
}

// objectName is the deterministic object key for a (volume, execID) pair.
// Path-style so operators can use bucket lifecycle policies scoped to
// `workspace/` prefix to clean up orphans.
func objectName(volName, execID string) string {
	// Sanitize volume name to allowed GCS object characters. Underscores
	// and most printable ASCII are fine; we only collapse path separators
	// to keep the prefix structure predictable.
	clean := strings.ReplaceAll(volName, "/", "-")
	return fmt.Sprintf("workspace/%s/%s.tar.gz", clean, execID)
}

func validateRefForSync(vol core.SharedVolumeRef) error {
	if vol.Name == "" {
		return fmt.Errorf("gcs-sync: SharedVolume.Name required")
	}
	if vol.ContainerPath == "" {
		return fmt.Errorf("gcs-sync: SharedVolume.ContainerPath required (volume %q)", vol.Name)
	}
	if vol.GCSBucket == "" {
		return errMissingBucket("gcs-sync", vol.Name)
	}
	return nil
}

func errMissingBucket(driver, vol string) error {
	return fmt.Errorf("storage backing %s: SharedVolume.GCSBucket required (volume %q)", driver, vol)
}

// writeTarGzFromDir tars + gzips dir's contents (entries are relative
// paths under dir) into w. If dir does not exist, writes an empty tar
// header — the recipient will see an empty volume, which is the correct
// first-exec behaviour. Identical entry-walk shape to the bootstrap-side
// `tarFrom` helper in agent/cmd/sockerless-{cloudrun,gcf}-bootstrap/
// persist.go (kept in sync deliberately so PreExec output is bit-for-bit
// readable by the bootstrap's untar path).
func writeTarGzFromDir(w io.Writer, dir string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	defer func() {
		_ = tw.Close()
		_ = gz.Close()
	}()

	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		// Empty tar — first exec on a fresh volume.
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	return filepath.Walk(dir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		var link string
		if fi.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", path, err)
			}
		}
		hdr, err := tar.FileInfoHeader(fi, link)
		if err != nil {
			return fmt.Errorf("file info header %s: %w", path, err)
		}
		hdr.Name = rel
		// Skip char/block/socket/fifo entries (not portable across
		// filesystems; bootstraps don't expect them either).
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir && hdr.Typeflag != tar.TypeSymlink {
			return nil
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

// readTarGzIntoDir is the inverse of writeTarGzFromDir. Refuses paths
// that escape root (../). Same shape as bootstrap-side `untarInto`.
func readTarGzIntoDir(r io.Reader, root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", root, err)
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		// Empty or non-gzip stream — treat as empty tar (no-op restore).
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("gunzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("tar entry %q escapes root", hdr.Name)
		}
		dst := filepath.Join(root, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.Remove(dst)
			if err := os.Symlink(hdr.Linkname, dst); err != nil {
				return err
			}
		default:
			// device/socket/fifo — skip
		}
	}
}
