package core

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// HostBindPolicy names the backend in the rejection a host-path bind
// receives when no shared volume covers it. Cloud workloads have no host
// filesystem, so the only host paths a backend can honour are the ones an
// operator declared as shared volumes.
type HostBindPolicy struct {
	// Platform is the proper name of the execution platform, e.g.
	// "Amazon ECS" or "Azure Container Apps".
	Platform string
	// Backing names what named volumes are stored on, e.g.
	// "sockerless-managed EFS access points".
	Backing string
	// EnvVar is the shared-volume variable the operator configures, e.g.
	// "SOCKERLESS_ECS_SHARED_VOLUMES".
	EnvVar string
}

// TranslateSharedVolumeBinds rewrites HostConfig.Binds for a cloud
// backend. For each `src:dst[:mode]` bind:
//
//   - a source that is not a host path (a named volume) passes through;
//   - `/var/run/docker.sock` is dropped — CI runners add it unconditionally
//     and there is no daemon socket in a cloud workload;
//   - a host path equal to a shared volume's ContainerPath is rewritten to
//     `<volume>:dst[:mode]`;
//   - a host path underneath a shared volume is dropped, because the
//     parent volume's mount already exposes it;
//   - any other host path is rejected with an InvalidParameterError that
//     names the platform, the backing, and the variable to configure.
//
// Docker treats a source starting with `/` or `.` as a path; anything
// else is a volume name. The dropped binds are returned so the caller can
// log them.
func TranslateSharedVolumeBinds(vols SharedVolumes, binds []string, policy HostBindPolicy) (translated, dropped []string, err error) {
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
		if !IsHostBindSource(src) {
			translated = append(translated, bind)
			continue
		}
		if sv := vols.BySourcePath(src); sv != nil {
			rewritten := sv.Name + ":" + dst
			if mode != "" {
				rewritten += ":" + mode
			}
			translated = append(translated, rewritten)
			continue
		}
		if vols.IsSubPath(src) {
			dropped = append(dropped, bind)
			continue
		}
		return nil, nil, &api.InvalidParameterError{Message: fmt.Sprintf(
			"host bind mounts are not supported on %s (%q); use a named volume (`docker volume create <name> && docker run -v <name>:%s`) — volumes are backed by %s. Configure %s to translate runner-task bind mounts to shared volumes.",
			policy.Platform, bind, dst, policy.Backing, policy.EnvVar,
		)}
	}
	return translated, dropped, nil
}

// IsHostBindSource reports whether a bind source is a host path rather
// than a volume name. Docker resolves relative sources to absolute paths
// before they reach the API, and a volume name may not begin with `.`, so
// a leading `/` or `.` is the path signal.
func IsHostBindSource(src string) bool {
	return strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".")
}
