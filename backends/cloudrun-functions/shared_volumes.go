package gcf

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// translateSharedVolumeBinds rewrites HostConfig.Binds for Cloud Run
// Functions. Named-volume binds (`-v volName:/mnt[:ro]`) pass through
// and land on sockerless-managed GCS buckets via the underlying Cloud
// Run Service's ServiceV2.Template.Volumes. Host-path binds translate
// via SharedVolumes (config-driven). Mirror of the
// `cloudrun.translateSharedVolumeBinds` + `lambda.fileSystemConfigsForBinds`
// shape:
//
//   - source matches a SharedVolume.ContainerPath → rewrite to that
//     volume's named-volume reference (`<volume>:/container[:ro]`);
//   - source is a sub-path of a mapped SharedVolume → drop (the parent
//     volume's mount already exposes the sub-path);
//   - `/var/run/docker.sock` → drop (no docker socket on Cloud
//     Functions; the github-runner adds this unconditionally);
//   - anything else → reject loudly (no host filesystem on Cloud
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
			return nil, nil, &api.InvalidParameterError{Message: fmt.Sprintf("host-path binds are not supported on Cloud Functions (%q); use a named volume (docker volume create + -v name:%s) — volumes are backed by sockerless-managed GCS buckets. Configure SOCKERLESS_GCP_SHARED_VOLUMES to translate runner-task bind mounts.", bind, dst)}
		}
		// Already a named volume — pass through.
		translated = append(translated, bind)
	}
	return translated, dropped, nil
}
