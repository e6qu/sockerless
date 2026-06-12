package azf

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// translateSharedVolumeBinds rewrites HostConfig.Binds for Azure
// Functions. Named-volume binds (`-v volName:/mnt[:ro]`) pass through
// and attach to the function site via
// WebApps.UpdateAzureStorageAccounts (see volumes.go). Host-path binds
// (`/h:/c`) translate via SharedVolumes (config-driven map from
// caller-side mount path → operator-provisioned Azure Files share).
// Mirrors the ECS / Lambda / ACA translators:
//
//   - source matches a SharedVolume.ContainerPath → rewrite to that
//     volume's named-volume reference (`<volume>:/container[:ro]`);
//   - source is a sub-path of a mapped SharedVolume → drop (the parent
//     volume's mount already exposes the sub-path);
//   - `/var/run/docker.sock` → drop (no docker socket on Azure
//     Functions; the github-runner adds this unconditionally);
//   - anything else → reject loudly (no host filesystem on Azure
//     Functions).
//
// Returns the rewritten bind list plus the binds that were dropped so
// the caller can log them.
func translateSharedVolumeBinds(cfg Config, binds []string) (translated, dropped []string, err error) {
	translated = make([]string, 0, len(binds))
	for _, bind := range binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			return nil, nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid bind %q: expected src:dst[:mode]", bind)}
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
		if strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") {
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
				"host-path binds are not supported on Azure Functions (%q); use a named volume (docker volume create + -v name:%s) — volumes are backed by sockerless-managed Azure Files shares. Configure SOCKERLESS_AZF_SHARED_VOLUMES to translate runner-task bind mounts to shared Azure Files shares.",
				bind, dst,
			)}
		}
		// Already a named volume — pass through.
		translated = append(translated, bind)
	}
	return translated, dropped, nil
}
