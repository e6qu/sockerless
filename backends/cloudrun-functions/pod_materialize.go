package gcf

// Pod-mode legacy helpers. The previous merged-rootfs supervisor
// approach (materializePodFunction + invokePodFunction +
// ensurePodOverlayImage) was replaced by direct multi-container
// Cloud Run Service deploy in pod_service.go. The helper types +
// sanitisers below are still used by the new path and by
// image_inject.go's pod-overlay rendering tests.

import (
	"strings"

	"github.com/sockerless/api"
)

// materializePodFunction collapses a multi-container pod into a single
// Cloud Run Function backed by a merged-rootfs overlay image. Called
// from ContainerStart on the LAST pod member's start (when
// PodDeferredStart returns shouldDefer=false and len(podContainers) > 1).
//
// Per spec § "Podman pods on FaaS backends — supervisor-in-overlay":
//
//  1. The overlay bakes each pod member's rootfs into /containers/<name>/.
//  2. The bootstrap (PID 1) parses SOCKERLESS_POD_CONTAINERS and forks
//     one chroot'd subprocess per member; the main member's stdout
//     becomes the HTTP response body, sidecars run in background.
//  3. Net+IPC+UTS namespaces are shared per podman pod default (matches);
//     mount-ns degraded to chroot path-isolation, PID-ns shared (no
//     CAP_SYS_ADMIN in Cloud Run sandbox). Surfaced to operators via
//     `docker inspect <pod-member>.HostConfig.MountNamespaceMode`.
//
// Per-member ContainerCreate already created throwaway single-container
// Functions; this materialisation deletes them atomically before
// creating the merged pod Function so the pod Function is the only
// cloud-side resource that maps back to any of the member containers.
//
// All members share the InvocationResult: the pod is one Function;
// `docker wait <any-member>` returns when the pod's HTTP invoke completes.

// containersToPodOverlaySpec converts the live container set into the
// build-time PodOverlaySpec the renderer consumes. Member order is
// preserved so the pod's main container (the one whose ContainerStart
// triggered materialisation) lands as MainName.
func containersToPodOverlaySpec(bootstrapPath, podName, mainID string, containers []api.Container) PodOverlaySpec {
	members := make([]PodMemberSpec, 0, len(containers))
	mainName := ""
	for _, c := range containers {
		// Member name = container's docker name (without leading slash)
		// or its short ID when unnamed. Both round-trip cleanly through
		// /containers/<name>/ chroot subdirs and the supervisor log prefix.
		name := strings.TrimPrefix(c.Name, "/")
		if name == "" {
			name = c.ID[:12]
		}
		// Sanitise to lowercase alnum + dash so it's safe for both the
		// chroot path and the GCP label slot if we end up writing it.
		name = sanitizePodMemberName(name)

		members = append(members, PodMemberSpec{
			Name:         name,
			ContainerID:  c.ID,
			BaseImageRef: c.Config.Image,
			Entrypoint:   c.Config.Entrypoint,
			Cmd:          c.Config.Cmd,
			Workdir:      c.Config.WorkingDir,
			Env:          c.Config.Env,
		})
		if c.ID == mainID {
			mainName = name
		}
	}
	return PodOverlaySpec{
		PodName:             podName,
		MainName:            mainName,
		BootstrapBinaryPath: bootstrapPath,
		Members:             members,
	}
}

// sanitizePodMemberName lowercases and strips characters outside
// [a-z0-9-]. The result is safe for use as a chroot subdir AND a GCP
// label fragment. Empty results fall back to "x" so we never end up
// with /containers// or an empty member identifier.
func sanitizePodMemberName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-':
			b.WriteRune(r)
		case r == '_', r == '.':
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "x"
	}
	return out
}

// sanitizePodLabelValue applies the GCP label-value charset to a pod
// name (lowercase letters + digits + `_-` only). Pods named with chars
// outside that set get the unsafe chars stripped; if the result would
// be empty we drop the label entirely (callers check for "").
func sanitizePodLabelValue(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}
