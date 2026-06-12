package aca

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// translateSharedVolumeBinds rewrites HostConfig.Binds for ACA.
// Named-volume binds (`volName:/mnt`) pass through and map to Azure
// Files shares provisioned by VolumeCreate (see volumes.go). Host-path
// binds (`/h:/c`) translate via SharedVolumes (config-driven map from
// caller-side mount path → operator-provisioned Azure Files share).
// Mirrors the ECS / Lambda / Cloud Run translators:
//
//   - source matches a SharedVolume.ContainerPath → rewrite to that
//     volume's named-volume reference (`<volume>:/container[:ro]`);
//   - source is a sub-path of a mapped SharedVolume → drop (the parent
//     volume's mount already exposes the sub-path);
//   - `/var/run/docker.sock` → drop (no docker socket on ACA; the
//     github-runner adds this unconditionally);
//   - anything else → reject loudly (no host filesystem on ACA).
//
// Returns the rewritten bind list plus the binds that were dropped so
// the caller can log them.
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
		if src == "/var/run/docker.sock" {
			dropped = append(dropped, bind)
			continue
		}
		if strings.HasPrefix(src, "/") {
			if sv := cfg.LookupSharedVolumeBySourcePath(src); sv != nil {
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
				"host bind mounts are not supported on ACA backend (%q); use a named volume (`docker volume create <name> && docker run -v <name>:/path`) — volumes are backed by sockerless-managed Azure Files shares. Configure SOCKERLESS_ACA_SHARED_VOLUMES to translate runner-task bind mounts to shared Azure Files shares.",
				bind,
			)}
		}
		// Already a named volume — pass through.
		translated = append(translated, bind)
	}
	return translated, dropped, nil
}
