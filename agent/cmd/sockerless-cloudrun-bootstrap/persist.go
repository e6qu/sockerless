package main

// Bind-volume persistence for the Cloud Run bootstrap.
//
// Each gitlab-runner stage runs in a separate Cloud Run Service revision
// instance. In-memory `emptyDir` volumes don't survive across instances,
// and Cloud Run's `Volume.Gcs` (gcsfuse) lacks POSIX hard-link / flock /
// atomic-rename — git checkout times out at ~200x the speed of tmpfs
// (measured: 211s vs 1s on the same workload).
//
// Workaround that keeps the runner unmodified: at container startup,
// download a single tar object per bind volume from GCS into the tmpfs
// mountpoint; after every exec, repack and upload. Single-object GCS
// round-trip (~2-5 sec) replaces N per-file round trips (~minutes).
//
// Auth uses the metadata server (Cloud Run service account ADC). The
// SA needs roles/storage.objectAdmin on each bucket — already granted
// for the gcsfuse mount path the buckets were originally created for.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// envPersistVolumes lists tmpfs mountpoints to back up to GCS at every
// exec boundary. Comma-separated `name=mountpath=bucket` triples; same
// shape as SOCKERLESS_GCP_SHARED_VOLUMES so the operator-config grammar
// stays consistent. Empty / missing → no persistence.
const envPersistVolumes = "SOCKERLESS_PERSIST_VOLUMES"

// persistObjectName is the single object key used per bucket. One
// tarball per (bucket, volume) — independent volumes get independent
// buckets per the existing bucketForVolume scheme.
const persistObjectName = "sockerless-volume.tar"

// metadataTokenURL returns a fresh ADC token for the Cloud Run service
// account. The metadata server bakes the audience for storage.googleapis
// .com correctly when scope=cloud-platform.
const metadataTokenURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

// persistVolume describes one mountpoint to back up.
type persistVolume struct {
	Name      string // logical volume name (for logging only)
	MountPath string // absolute path in container
	Bucket    string // GCS bucket name (no gs:// prefix)
}

// parsePersistVolumes parses SOCKERLESS_PERSIST_VOLUMES into a slice.
// Tolerant of empty input + extra whitespace; rejects malformed
// entries (logs + skips them).
func parsePersistVolumes(env string) []persistVolume {
	env = strings.TrimSpace(env)
	if env == "" {
		return nil
	}
	var out []persistVolume
	for _, entry := range strings.Split(env, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "=")
		if len(parts) != 3 {
			fmt.Fprintf(os.Stderr, "sockerless-bootstrap: persist volume entry malformed (want name=path=bucket): %q\n", entry)
			continue
		}
		name := strings.TrimSpace(parts[0])
		mp := strings.TrimSpace(parts[1])
		bucket := strings.TrimSpace(parts[2])
		if name == "" || mp == "" || bucket == "" {
			fmt.Fprintf(os.Stderr, "sockerless-bootstrap: persist volume entry has empty field: %q\n", entry)
			continue
		}
		out = append(out, persistVolume{Name: name, MountPath: mp, Bucket: bucket})
	}
	return out
}

// restoreAll downloads each volume's tarball from GCS and untars into
// its mountpoint. A 404 (object missing) is the first-stage path and
// is logged but not an error. Other failures (auth, corrupt tar) are
// fatal — restore is a correctness primitive, not best-effort.
func restoreAll(ctx context.Context, vols []persistVolume) error {
	for _, v := range vols {
		if err := restoreOne(ctx, v); err != nil {
			return fmt.Errorf("restore %s: %w", v.Name, err)
		}
	}
	return nil
}

func restoreOne(ctx context.Context, v persistVolume) error {
	if err := os.MkdirAll(v.MountPath, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", v.MountPath, err)
	}
	body, status, err := gcsGet(ctx, v.Bucket, persistObjectName)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "sockerless-bootstrap: persist restore %s: no prior object (first stage)\n", v.Name)
		return nil
	}
	if status != http.StatusOK {
		return fmt.Errorf("GET gs://%s/%s status %d: %s", v.Bucket, persistObjectName, status, truncate(body, 200))
	}
	if err := untarInto(bytes.NewReader(body), v.MountPath); err != nil {
		return fmt.Errorf("untar into %s: %w", v.MountPath, err)
	}
	fmt.Fprintf(os.Stderr, "sockerless-bootstrap: persist restore %s: %d bytes -> %s\n", v.Name, len(body), v.MountPath)
	return nil
}

// saveAll repacks each mountpoint and uploads to GCS. Failure is fatal
// — silent data loss between stages would surface as confusing build
// errors downstream.
func saveAll(ctx context.Context, vols []persistVolume) error {
	for _, v := range vols {
		if err := saveOne(ctx, v); err != nil {
			return fmt.Errorf("save %s: %w", v.Name, err)
		}
	}
	return nil
}

func saveOne(ctx context.Context, v persistVolume) error {
	info, err := os.Stat(v.MountPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "sockerless-bootstrap: persist save %s: mountpoint %s missing, skipping\n", v.Name, v.MountPath)
			return nil
		}
		return fmt.Errorf("stat %s: %w", v.MountPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("mountpoint %s is not a directory", v.MountPath)
	}
	var buf bytes.Buffer
	if err := tarFrom(&buf, v.MountPath); err != nil {
		return fmt.Errorf("tar %s: %w", v.MountPath, err)
	}
	if err := gcsPut(ctx, v.Bucket, persistObjectName, buf.Bytes()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sockerless-bootstrap: persist save %s: %d bytes -> gs://%s/%s\n", v.Name, buf.Len(), v.Bucket, persistObjectName)
	return nil
}

// tarFrom writes a tar archive of `root`'s contents (entries are
// relative paths under root) to w. Skips device files; preserves
// regular files, directories, and symlinks with mode + mtime.
func tarFrom(w io.Writer, root string) error {
	tw := tar.NewWriter(w)
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
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
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", path, err)
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return fmt.Errorf("file info header %s: %w", path, err)
		}
		hdr.Name = rel
		// Skip char/block/socket/fifo entries.
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
	}); err != nil {
		return err
	}
	return tw.Close()
}

// untarInto extracts r (a tar stream) into root. Refuses paths that
// escape root (../) and silently ignores unsupported entry types.
func untarInto(r io.Reader, root string) error {
	tr := tar.NewReader(r)
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
			_ = os.Remove(dst) // overwrite stale symlink
			if err := os.Symlink(hdr.Linkname, dst); err != nil {
				return err
			}
		default:
			// device/socket/fifo — skip
		}
	}
}

// gcsGet fetches the object via the JSON-API media download endpoint.
// Returns the body, HTTP status, and any transport error.
func gcsGet(ctx context.Context, bucket, object string) ([]byte, int, error) {
	tok, err := metadataToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	u := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s/o/%s?alt=media",
		url.PathEscape(bucket), url.PathEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// gcsPut uploads an object via the JSON-API simple-upload endpoint.
// Always overwrites; no precondition.
func gcsPut(ctx context.Context, bucket, object string, data []byte) error {
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
	req.Header.Set("Content-Type", "application/x-tar")
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

// metadataToken fetches a fresh OAuth bearer from the GCE metadata
// server. Cloud Run resolves this to the configured service-account
// identity automatically.
func metadataToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataTokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("metadata token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("metadata token status %d: %s", resp.StatusCode, truncate(body, 200))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decode metadata token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("metadata token: empty access_token in response")
	}
	return tok.AccessToken, nil
}

// httpClient is a single shared client. Timeout per request is generous
// — large tarballs can take seconds at GCS ingress speeds; failures
// surface as exec errors via saveOne / restoreOne.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}
