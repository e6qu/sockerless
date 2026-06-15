package azf

import (
	"strings"

	"github.com/sockerless/api"
)

// shouldDeferOrMaterializeNetworkPod implements the "docker user-defined
// network → App Service multi-container (sitecontainers) site" mapping.
//
// Standard-Docker signal: containers that join the SAME user-defined
// network are expected to share a network namespace and resolve each other
// by alias. On App Service that maps to one Function App site whose
// sitecontainers share one loopback (`127.0.0.1`) — the faithful Azure
// analog of a Cloud Run multi-container revision or an ECS multi-container
// task.
//
// The materialization trigger is the standard Docker signal
// `Container.Config.OpenStdin` — set true on containers the docker client
// is about to ATTACH stdin to (script-runners). Service containers
// (postgres, redis, etc.) created with their image's default CMD do NOT set
// OpenStdin.
//
// Returns:
//   - shouldDefer=true, members=nil: service-style container with no sibling
//     yet — defer until a peer (script-runner or job container) arrives to
//     materialize the pod. ContainerStart returns success (eventually-true).
//   - shouldDefer=false, members!=nil (len>1): this container completes a
//     pod; return all members (members[0] is the main) so ContainerStart can
//     deploy them as one multi-container site.
//   - shouldDefer=false, members=nil: not in a multi-member network — fall
//     through to the single-container path.
//
// No runner-specific labels are read: the signals are pure Docker API —
// network membership + Container.Config.OpenStdin. This mirrors the gcf /
// cloudrun network-pod path.
func (s *Server) shouldDeferOrMaterializeNetworkPod(c api.Container) (shouldDefer bool, members []api.Container) {
	netID, ok := s.userDefinedNetworkID(c)
	if !ok {
		return false, nil
	}

	siblings := s.pendingMembersOfNetwork(netID, c.ID)
	if !c.Config.OpenStdin {
		// Service-style (postgres/redis) OR the GH actions/runner job
		// container (OpenStdin=false but long-lived; runner execs each step).
		//   - No siblings yet: defer; the disambiguating peer hasn't arrived.
		//   - Siblings exist: the FIRST sibling is the main (created first);
		//     this just-arrived container is a sidecar.
		if len(siblings) == 0 {
			return true, nil
		}
		all := make([]api.Container, 0, len(siblings)+1)
		all = append(all, siblings[0])
		all = append(all, siblings[1:]...)
		all = append(all, c)
		return false, all
	}

	// Script-runner (gitlab-runner pattern): this container is the main +
	// every pending sibling is a sidecar.
	if len(siblings) == 0 {
		return false, nil
	}
	all := make([]api.Container, 0, len(siblings)+1)
	all = append(all, c)
	all = append(all, siblings...)
	return false, all
}

// userDefinedNetworkID returns the ID of the first user-defined network the
// container has joined, or false if none. Built-in networks (bridge, host,
// none, default) are excluded.
func (s *Server) userDefinedNetworkID(c api.Container) (string, bool) {
	for netName, ep := range c.NetworkSettings.Networks {
		if isBuiltinNetwork(netName) {
			continue
		}
		if ep != nil && ep.NetworkID != "" {
			return ep.NetworkID, true
		}
		if net, ok := s.Store.ResolveNetwork(netName); ok {
			return net.ID, true
		}
	}
	if !isBuiltinNetwork(c.HostConfig.NetworkMode) {
		if net, ok := s.Store.ResolveNetwork(c.HostConfig.NetworkMode); ok {
			return net.ID, true
		}
	}
	return "", false
}

func isBuiltinNetwork(name string) bool {
	switch strings.ToLower(name) {
	case "", "default", "bridge", "host", "none":
		return true
	}
	return false
}

// pendingMembersOfNetwork returns every container in PendingCreates that has
// joined the given network ID, excluding `excludeID`. Already-materialized
// MAIN containers (OpenStdin=true with a populated FunctionURL) are filtered
// out: gitlab-runner spawns a new build container per stage, and the new
// stage's container must materialize as the main of its own site rather than
// dragging the previous stage's main into a fresh site. Sidecars
// (OpenStdin=false) are NOT filtered — each stage's site needs its own copy
// of the services on its shared loopback.
func (s *Server) pendingMembersOfNetwork(netID, excludeID string) []api.Container {
	var out []api.Container
	for _, pc := range s.PendingCreates.List() {
		if pc.ID == excludeID {
			continue
		}
		mid, ok := s.userDefinedNetworkID(pc)
		if !ok || mid != netID {
			continue
		}
		if pc.Config.OpenStdin {
			if state, ok := s.AZF.Get(pc.ID); ok && state.FunctionURL != "" {
				continue
			}
		}
		out = append(out, pc)
	}
	return out
}

// hostAliasesForMembers returns the alias names registered by every member
// of the given network (each container's NetworkingConfig.EndpointsConfig
// .<net>.Aliases, plus hostname and a service-style container name). Used to
// source SOCKERLESS_HOST_ALIASES so the main bootstrap can write
// `127.0.0.1 <alias>` lines to /etc/hosts and a sibling resolves by name to
// the shared loopback.
func hostAliasesForMembers(members []api.Container, netID string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, c := range members {
		for netName, ep := range c.NetworkSettings.Networks {
			if ep == nil {
				continue
			}
			if ep.NetworkID != netID && netName != netID {
				continue
			}
			for _, a := range ep.Aliases {
				if a == "" || seen[a] {
					continue
				}
				seen[a] = true
				out = append(out, a)
			}
		}
		if c.Config.Hostname != "" && !seen[c.Config.Hostname] {
			seen[c.Config.Hostname] = true
			out = append(out, c.Config.Hostname)
		}
		bareName := strings.TrimPrefix(c.Name, "/")
		if bareName != "" && !seen[bareName] && isLikelyAlias(bareName) {
			seen[bareName] = true
			out = append(out, bareName)
		}
	}
	return out
}

// isLikelyAlias returns true if `s` looks like a user-supplied alias (short,
// no separators) rather than a generated container name.
func isLikelyAlias(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	if strings.ContainsAny(s, "/:") {
		return false
	}
	if len(s) > 12 {
		hex := true
		for _, r := range s {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				hex = false
				break
			}
		}
		if hex {
			return false
		}
	}
	return true
}
