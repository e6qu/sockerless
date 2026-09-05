package gcf

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	core "github.com/sockerless/backend-core"
)

const overlayBootstrapContextName = "sockerless-gcf-bootstrap"

// PodOverlaySpec describes the merged-rootfs pod overlay built when
// sockerless materialises a multi-container pod into one Cloud Run
// Function. Per spec § "Podman pods on FaaS backends — supervisor-in-overlay",
// each pod member's rootfs is COPY --from'd into a per-name subdir
// (`/containers/<name>`) of a single image; the bootstrap (PID 1) then
// chroots and exec's each member.
//
// Stateless invariant: the manifest itself is encoded as base64-JSON
// into the SOCKERLESS_POD_CONTAINERS env var on the Function so the
// supervisor can rebuild the per-member exec table at runtime, and so
// `docker pod ps` round-trips via cloud labels + env without any local
// state.
type PodOverlaySpec struct {
	// PodName is the human-readable pod name. Used as the base of the
	// Function name (sockerless-pod-<podName>-<hash>) and tagged on
	// the Function via label `sockerless_pod=<podName>` for round-trip.
	PodName string
	// MainName identifies which pod member's stdout becomes the HTTP
	// response body (the "step" container in CI runner terms). Empty
	// → bootstrap defaults to the last entry in Members.
	MainName string
	// BootstrapBinaryPath is the host path of the
	// sockerless-gcf-bootstrap binary that should be COPYed into
	// /opt/sockerless inside the overlay.
	BootstrapBinaryPath string
	// Members lists the pod's containers in the order sockerless saw
	// them join the pod. Last entry defaults as the main.
	Members []core.PodMemberSpec
}

// RenderPodOverlayDockerfile returns the Dockerfile content for a pod
// overlay: scratch base + per-member `COPY --from=<image> / /containers/<name>/`
// + bootstrap binary + SOCKERLESS_POD_CONTAINERS env. The bootstrap
// (already PID 1 via ENTRYPOINT) runs in supervisor mode at startup.
//
// Build-time collision detection is deliberately not done here — Cloud
// Build will surface duplicate-path warnings during the COPY step, and
// the per-container subdir naming guarantees no actual file collision
// (each member's rootfs lives in its own `/containers/<name>/` subtree).
func RenderPodOverlayDockerfile(spec PodOverlaySpec) (string, error) {
	if spec.BootstrapBinaryPath == "" {
		return "", fmt.Errorf("BootstrapBinaryPath is required")
	}
	if len(spec.Members) == 0 {
		return "", fmt.Errorf("members must be non-empty")
	}
	for i, m := range spec.Members {
		if m.Name == "" {
			return "", fmt.Errorf("member %d: Name is required", i)
		}
		if m.BaseImageRef == "" {
			return "", fmt.Errorf("member %q: BaseImageRef is required", m.Name)
		}
	}
	manifest, err := core.EncodePodManifest(spec.Members)
	if err != nil {
		return "", fmt.Errorf("encode pod manifest: %w", err)
	}

	var b strings.Builder
	// scratch is too minimal — the supervisor needs /lib + dynamic
	// loader to start. Use the FIRST pod member's image as the base
	// rootfs so the supervisor has /bin/sh, libc, and the standard
	// utilities; subsequent members layer in under /containers/<name>/.
	// The first member's full rootfs lives at both / and
	// /containers/<firstName>/ so its chroot still points into a
	// complete tree.
	first := spec.Members[0]
	fmt.Fprintf(&b, "FROM %s\n", first.BaseImageRef)
	// Snapshot the base rootfs into the first member's chroot subdir
	// before the COPY layers from the remaining members would clobber
	// any shared paths under /. `cp -a` preserves symlinks, perms,
	// times — the chroot must look identical to the unwrapped base
	// from the member's perspective.
	fmt.Fprintf(&b, "RUN mkdir -p /containers/%s && cp -a /. /containers/%s/ 2>/dev/null || true\n", first.Name, first.Name)
	for _, m := range spec.Members[1:] {
		fmt.Fprintf(&b, "COPY --from=%s / /containers/%s/\n", m.BaseImageRef, m.Name)
	}
	fmt.Fprintf(&b, "COPY %s /opt/sockerless/sockerless-gcf-bootstrap\n", overlayBootstrapContextName)
	fmt.Fprintln(&b, `RUN chmod +x /opt/sockerless/sockerless-gcf-bootstrap`)
	fmt.Fprintf(&b, "ENV %s=%s\n", "SOCKERLESS_POD_CONTAINERS", manifest)
	if spec.MainName != "" {
		fmt.Fprintf(&b, "ENV %s=%s\n", "SOCKERLESS_POD_MAIN", spec.MainName)
	}
	fmt.Fprintln(&b, `ENTRYPOINT ["/opt/sockerless/sockerless-gcf-bootstrap"]`)
	return b.String(), nil
}

// PodOverlayContentTag returns a stable content-addressed tag for the
// pod overlay image. Identical pod manifests (same set of member
// images + entrypoints + cmds, in the same order) reuse the same AR
// image. Format: `gcf-pod-<sha256[:16]>`.
func PodOverlayContentTag(spec PodOverlaySpec) string {
	h := sha256.New()
	fmt.Fprintln(h, spec.BootstrapBinaryPath)
	fmt.Fprintln(h, spec.MainName)
	for _, m := range spec.Members {
		fmt.Fprintln(h, m.Name)
		fmt.Fprintln(h, m.BaseImageRef)
		if epb, err := json.Marshal(m.Entrypoint); err == nil {
			h.Write(epb)
		}
		if cmdb, err := json.Marshal(m.Cmd); err == nil {
			h.Write(cmdb)
		}
		fmt.Fprintln(h, m.Workdir)
		if envb, err := json.Marshal(m.Env); err == nil {
			h.Write(envb)
		}
	}
	sum := h.Sum(nil)
	return "gcf-pod-" + hex.EncodeToString(sum[:8])
}

// TarPodOverlayContext packages the pod-overlay Dockerfile + bootstrap
// binary into a gzipped tar archive suitable as the build context for
// gcpcommon.GCPBuildService.Build. Per-member images are pulled by
// Cloud Build via the Dockerfile `COPY --from=<image>` references; no
// member rootfs is staged in the local context.
func TarPodOverlayContext(spec PodOverlaySpec) ([]byte, error) {
	dockerfile, err := RenderPodOverlayDockerfile(spec)
	if err != nil {
		return nil, err
	}
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	if err := core.WriteTarEntry(tw, "Dockerfile", []byte(dockerfile), 0o644); err != nil {
		return nil, err
	}
	if err := core.WriteTarFile(tw, spec.BootstrapBinaryPath, overlayBootstrapContextName); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return raw.Bytes(), nil
}

// — stub Buildpacks-Go source ———————————————————————————————————————
//
// Cloud Functions Gen2's CreateFunction requires a Source pointing at
// Buildpacks-compatible source code. There is no documented path to
// deploy a generic OCI image. The stub source below is the documented
// escape hatch: a no-op Go function that satisfies the Buildpacks-Go
// detector. After CreateFunction succeeds, the gcf backend swaps the
// underlying Cloud Run service's image (`Run.Services.UpdateService`) to
// the real overlay built above. The stub never serves user traffic.
//
// The source is identical for every sockerless deployment in a project,
// so Buildpacks caches it after the first deploy — repeat CreateFunction
// calls are ~30s instead of ~120s.

const stubGoMain = `package p

import (
	"net/http"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("Stub", stub)
}

// stub is a no-op handler used only to satisfy Cloud Functions Gen2's
// Buildpacks API gate at CreateFunction time. The gcf backend replaces
// the underlying Cloud Run service's image with sockerless's real
// overlay via Run.Services.UpdateService immediately after the function
// reaches ACTIVE, so this handler never serves user traffic.
func stub(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
`

const stubGoMod = `module sockerless.io/gcf-stub

go 1.22

require github.com/GoogleCloudPlatform/functions-framework-go v1.9.0
`

// ZipStubBuildpacksSource returns a ZIP archive containing the no-op
// Go function source that Cloud Functions Gen2's Buildpacks detector
// accepts (Runtime="go124", EntryPoint="Stub"). The zip is identical
// for every sockerless deployment, so it can be staged once per project
// and reused across CreateFunction calls.
func ZipStubBuildpacksSource() ([]byte, error) {
	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	if err := writeZipEntry(zw, "main.go", []byte(stubGoMain)); err != nil {
		return nil, err
	}
	if err := writeZipEntry(zw, "go.mod", []byte(stubGoMod)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return raw.Bytes(), nil
}

// stageStubSourceIfMissing uploads the stub source to GCS at
// gs://<bucket>/<object> if not already present. Used once per project
// to populate the source pointer that CreateFunction expects.
func stageStubSourceIfMissing(ctx context.Context, sc *storage.Client, bucket, object string) error {
	if _, err := sc.Bucket(bucket).Object(object).Attrs(ctx); err == nil {
		return nil // already staged
	}
	zipData, err := ZipStubBuildpacksSource()
	if err != nil {
		return fmt.Errorf("build stub source zip: %w", err)
	}
	w := sc.Bucket(bucket).Object(object).NewWriter(ctx)
	w.ContentType = "application/zip"
	if _, err := w.Write(zipData); err != nil {
		_ = w.Close()
		return fmt.Errorf("write gs://%s/%s: %w", bucket, object, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close gs://%s/%s: %w", bucket, object, err)
	}
	return nil
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
