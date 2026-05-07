package main

// gcs-sync per-exec restore/save. Distinct from persist.go, which
// handles the legacy SOCKERLESS_PERSIST_VOLUMES tar-pack pattern.
//
// Two env vars define the data plane:
//
//   - SOCKERLESS_SYNC_MOUNTS (set at JOB pod-Service materialize time):
//     `name=mountpath[,...]` — maps logical SharedVolume name to the
//     bind target on this container. Read once at startup.
//
//   - SOCKERLESS_SYNC_VOLUMES (set per-exec via the envelope.Env from
//     the runner-task's PreExec hook): `name=gs://bucket/object[,...]`
//     — for each volume in this exec, where the freshly-tarred
//     snapshot lives. Object format is .tar.gz (matches
//     GCSSyncDriver.writeTarGzFromDir on the runner-task side).
//
// At restore time the bootstrap joins the two by name: download the
// GCS object, untar to the mount path. At save time it tars the mount
// path and uploads to the same GCS object so the runner-task can pull
// the modifications back via PostExec.
//
// Why split: the runner-task's stateless cloud_state lookup returns
// api.Container with empty HostConfig.Binds, so the runner-task can't
// reliably know the JOB-side bind target. The materializer DOES know
// it (it sets the runpb.VolumeMount), so it bakes the map into the
// container's startup env.
//
// No-fallbacks: malformed entries fail loudly. This data plane is
// load-bearing for the per-step shared-workspace path.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	envSyncVolumes = "SOCKERLESS_SYNC_VOLUMES"
	envSyncMounts  = "SOCKERLESS_SYNC_MOUNTS"
)

// syncVolume joins a per-exec SOCKERLESS_SYNC_VOLUMES entry (name + GCS
// object) with the boot-time SOCKERLESS_SYNC_MOUNTS entry (name +
// mount path) so each exec knows where to restore to.
type syncVolume struct {
	Name      string
	MountPath string
	Bucket    string
	Object    string
}

// parseSyncMounts reads SOCKERLESS_SYNC_MOUNTS — set at materialize
// time — into a map of `volumeName -> mountPath`. Format: comma-
// separated `name=path` pairs.
func parseSyncMounts(env string) (map[string]string, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, raw := range strings.Split(env, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("sync mount entry malformed (want name=path): %q", entry)
		}
		name := strings.TrimSpace(parts[0])
		mp := strings.TrimSpace(parts[1])
		if name == "" || mp == "" {
			return nil, fmt.Errorf("sync mount entry has empty field: %q", entry)
		}
		out[name] = mp
	}
	return out, nil
}

// parseSyncVolumes reads SOCKERLESS_SYNC_VOLUMES from a string of
// comma-separated `name=gs://bucket/object` pairs and joins each pair
// with mounts (from parseSyncMounts) so the bootstrap knows where to
// restore. Returns an error on any malformed entry or unmatched name —
// silent skip would mask a configuration mismatch.
func parseSyncVolumes(env string, mounts map[string]string) ([]syncVolume, error) {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil, nil
	}
	var out []syncVolume
	for _, raw := range strings.Split(env, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("sync volume entry malformed (want name=gs://bucket/object): %q", entry)
		}
		name := strings.TrimSpace(parts[0])
		gs := strings.TrimSpace(parts[1])
		if name == "" || gs == "" {
			return nil, fmt.Errorf("sync volume entry has empty field: %q", entry)
		}
		bucket, object, err := splitGSURL(gs)
		if err != nil {
			return nil, fmt.Errorf("sync volume %q: %w", name, err)
		}
		mp, ok := mounts[name]
		if !ok {
			return nil, fmt.Errorf("sync volume %q: no mount path in SOCKERLESS_SYNC_MOUNTS (got mounts=%v)", name, mounts)
		}
		out = append(out, syncVolume{Name: name, MountPath: mp, Bucket: bucket, Object: object})
	}
	return out, nil
}

func splitGSURL(s string) (bucket, object string, err error) {
	const prefix = "gs://"
	if !strings.HasPrefix(s, prefix) {
		return "", "", fmt.Errorf("expected gs:// URL, got %q", s)
	}
	rest := s[len(prefix):]
	idx := strings.IndexByte(rest, '/')
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", fmt.Errorf("expected gs://bucket/object, got %q", s)
	}
	return rest[:idx], rest[idx+1:], nil
}

// restoreSyncAll downloads each volume's tar.gz from GCS and untars into
// its mountpoint. A 404 (object missing) is the first-stage path and is
// non-fatal. Other failures fail the exec — restore is a correctness
// primitive, not best-effort.
func restoreSyncAll(ctx context.Context, vols []syncVolume) error {
	for _, v := range vols {
		if err := restoreSyncOne(ctx, v); err != nil {
			return fmt.Errorf("restore sync %s: %w", v.Name, err)
		}
	}
	return nil
}

func restoreSyncOne(ctx context.Context, v syncVolume) error {
	if err := os.MkdirAll(v.MountPath, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", v.MountPath, err)
	}
	body, status, err := gcsGet(ctx, v.Bucket, v.Object)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "sockerless-bootstrap: sync restore %s: no object at gs://%s/%s (first stage)\n", v.Name, v.Bucket, v.Object)
		return nil
	}
	if status != http.StatusOK {
		return fmt.Errorf("GET gs://%s/%s status %d: %s", v.Bucket, v.Object, status, truncate(body, 200))
	}
	if err := untarGzInto(body, v.MountPath); err != nil {
		return fmt.Errorf("untar.gz into %s: %w", v.MountPath, err)
	}
	fmt.Fprintf(os.Stderr, "sockerless-bootstrap: sync restore %s: %d bytes -> %s\n", v.Name, len(body), v.MountPath)
	return nil
}

// saveSyncAll repacks each mountpoint as tar.gz and uploads to its
// per-exec object. Failure is fatal — silent data loss between
// runner-task and JOB pod-Service surfaces as missing files downstream.
func saveSyncAll(ctx context.Context, vols []syncVolume) error {
	for _, v := range vols {
		if err := saveSyncOne(ctx, v); err != nil {
			return fmt.Errorf("save sync %s: %w", v.Name, err)
		}
	}
	return nil
}

func saveSyncOne(ctx context.Context, v syncVolume) error {
	info, err := os.Stat(v.MountPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", v.MountPath, err)
		}
		fmt.Fprintf(os.Stderr, "sockerless-bootstrap: sync save %s: mountpoint %s missing, uploading empty tar.gz\n", v.Name, v.MountPath)
	} else if !info.IsDir() {
		return fmt.Errorf("mountpoint %s is not a directory", v.MountPath)
	}
	var buf bytes.Buffer
	if err := tarGzFrom(&buf, v.MountPath); err != nil {
		return fmt.Errorf("tar.gz %s: %w", v.MountPath, err)
	}
	if err := gcsPutContentType(ctx, v.Bucket, v.Object, buf.Bytes(), "application/x-tar+gzip"); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sockerless-bootstrap: sync save %s: %d bytes -> gs://%s/%s\n", v.Name, buf.Len(), v.Bucket, v.Object)
	return nil
}

// tarGzFrom is the gzipped-tar variant of persist.go::tarFrom. Kept
// separate so the legacy plain-tar persist path stays untouched.
func tarGzFrom(w io.Writer, root string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	defer func() {
		_ = tw.Close()
		_ = gz.Close()
	}()
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil // empty archive
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}
	return filepath.Walk(root, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
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

func untarGzInto(data []byte, root string) error {
	if len(data) == 0 {
		return nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", root, err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
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

// gcsPutContentType uploads with an explicit content-type. Variant of
// the persist.go::gcsPut helper, which hardcodes "application/x-tar".
// We want "application/x-tar+gzip" for sync objects so cloud-side
// tooling (gsutil ls -L, console preview) recognises the encoding.
func gcsPutContentType(ctx context.Context, bucket, object string, data []byte, contentType string) error {
	tok, err := metadataToken(ctx)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("https://storage.googleapis.com/upload/storage/v1/b/%s/o?uploadType=media&name=%s",
		url.PathEscape(bucket), url.QueryEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(data))
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT gs://%s/%s status %d: %s", bucket, object, resp.StatusCode, truncate(body, 200))
	}
	return nil
}
