package ecs

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// translateSharedVolumeBinds rewrites HostConfig.Binds for Fargate. On
// Fargate there's no host filesystem to bind from, so host-path bind
// mounts (`/h:/c`) are rejected — only named Docker volumes
// (`volname:/c`) are supported; those land on the sockerless-managed
// EFS filesystem via per-volume access points. Rejecting host binds
// (rather than silently substituting an empty scratch volume or an EFS
// subdir named after the host path) keeps `docker run -v /host:/container`
// failures explicit.
//
// Exception 1 — shared-volume translation. When sockerless runs inside
// an ECS task that has EFS access points mounted at configured paths
// (declared via SOCKERLESS_ECS_SHARED_VOLUMES), a bind mount whose
// source matches a shared volume's ContainerPath is rewritten to a
// named-volume reference. The spawned sub-task mounts the same EFS
// access point, so caller and sub-task share the workspace via EFS —
// this is what makes GitHub Actions `container:` bind-mounts work
// end-to-end on Fargate.
//
// Exception 2 — sub-paths of a mapped shared volume are dropped. The
// parent volume's EFS mount already exposes the sub-path inside the
// spawned container at the parent's containerPath + the sub-path's
// relative offset. The runner uses redundant per-sub-path bind mounts
// (e.g. `/_work/_temp`, `/_work/_actions`) when the parent `/_work` is
// already mapped — those land inside the same EFS naturally.
//
// Exception 3 — `/var/run/docker.sock`. The runner unconditionally
// adds this for nested-docker support. On Fargate there's no docker
// daemon socket; sockerless drops the mount. Sub-task containers that
// try to use docker.sock will fail at the step level, surfacing the
// limitation.
//
// Returns the rewritten bind list plus the binds that were dropped
// (docker.sock + shared-volume sub-paths) so the caller can log them.
func translateSharedVolumeBinds(cfg Config, binds []string) (translated, dropped []string, err error) {
	translated = make([]string, 0, len(binds))
	for _, bind := range binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			return nil, nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind mount spec %q", bind)}
		}
		src, dst := parts[0], parts[1]
		mode := ""
		if len(parts) == 3 {
			mode = parts[2]
		}
		if !strings.HasPrefix(src, "/") {
			// Already a named-volume reference (`volname:/path`) — keep as-is.
			translated = append(translated, bind)
			continue
		}
		if src == "/var/run/docker.sock" {
			dropped = append(dropped, bind)
			continue
		}
		if sv := cfg.LookupSharedVolumeBySourcePath(src); sv != nil {
			// Rewrite `/host:/container[:ro]` → `<volume>:/container[:ro]`.
			rewritten := sv.Name + ":" + dst
			if mode != "" {
				rewritten += ":" + mode
			}
			translated = append(translated, rewritten)
			continue
		}
		if isSubPathOfSharedVolume(src, cfg.SharedVolumes) {
			dropped = append(dropped, bind)
			continue
		}
		return nil, nil, &api.InvalidParameterError{Message: fmt.Sprintf(
			"host bind mounts are not supported on ECS backend (%q); use a named volume (`docker volume create <name> && docker run -v <name>:/path`) — volumes are backed by sockerless-managed EFS access points. Configure SOCKERLESS_ECS_SHARED_VOLUMES to translate runner-task bind mounts to shared EFS access points.",
			bind,
		)}
	}
	return translated, dropped, nil
}
