package cloudrun

import (
	"strings"

	"github.com/sockerless/api"
)

// shouldDeferOrMaterializeNetworkPod implements the "docker user-defined
// network → Cloud Run multi-container Service revision" mapping.
//
// Standard-Docker signal: containers that join the SAME user-defined
// network (NetworkingConfig.EndpointsConfig.<net>.Aliases populated) are
// expected to share a network namespace and resolve each other by alias.
// On Cloud Run that maps cleanly to a multi-container Service revision
// where sidecars share loopback (`127.0.0.1`).
//
// The materialization trigger is the standard Docker signal
// `Container.Config.OpenStdin` — set true on containers the docker
// client is about to ATTACH stdin to (script-runners). Service
// containers (postgres, redis, etc.) created with their image's default
// CMD do NOT set OpenStdin.
//
// Returns:
//   - shouldDefer=true, members=nil: this container is service-style
//     (no OpenStdin) and there are sibling pending creates in the same
//     network — defer the actual deploy until a script-runner triggers
//     materialization. ContainerStart returns success (eventually-true:
//     the sidecar WILL be running shortly when the pod materializes).
//   - shouldDefer=false, members!=nil (len>1): this container is a
//     script-runner (OpenStdin) and has sibling deferrals — return all
//     pod members so ContainerStart can deploy them as one
//     multi-container Cloud Run Service revision.
//   - shouldDefer=false, members=nil: not in a multi-member network or
//     no sibling deferrals — fall through to the single-container path.
//
// No runner-specific labels are read. The signals are pure Docker API:
// network membership + Container.Config.OpenStdin.
func (s *Server) shouldDeferOrMaterializeNetworkPod(c api.Container) (shouldDefer bool, members []api.Container) {
	netID, ok := s.userDefinedNetworkID(c)
	if !ok {
		return false, nil
	}

	siblings := s.pendingMembersOfNetwork(netID, c.ID)
	if !c.Config.OpenStdin {
		// Service-style container. Defer if siblings exist (so we can
		// bundle with a script-runner later) or if the network has been
		// reserved for multi-container deployment.
		if len(siblings) > 0 {
			return true, nil
		}
		// Lone service container, no siblings yet. Defer — gitlab/github
		// runners always create the script-runner AFTER services. If
		// nothing arrives we'll detect the orphan on network teardown.
		return true, nil
	}

	// Script-runner. gitlab-runner v17.5 creates a NEW script-runner
	// container per stage (get_sources / step_script / after_script),
	// each in the SAME user-defined network as the service container(s).
	// To keep service containers reachable across stages, also pull in
	// any service containers tracked under this network ID — even if
	// they were already deployed in a prior materialization. The cloud-
	// run multi-container revision per stage redeploys the postgres
	// sidecar from scratch (postgres is stateless across job stages,
	// matching the docker-compose scoping that gitlab-runner emulates).
	pinned := s.serviceMembersOfNetwork(netID)
	all := make([]api.Container, 0, len(siblings)+len(pinned)+1)
	all = append(all, c) // main first — startMultiContainerServiceTyped uses index 0 as IsMain
	all = append(all, siblings...)
	for _, p := range pinned {
		// Skip duplicates if a sibling is also in the pinned set.
		found := false
		for _, existing := range all {
			if existing.ID == p.ID {
				found = true
				break
			}
		}
		if !found {
			all = append(all, p)
		}
	}
	if len(all) <= 1 {
		return false, nil
	}
	return false, all
}

// serviceMembersOfNetwork returns service-style containers (no
// OpenStdin) that have ever been members of this network — i.e.
// containers we *deferred* via shouldDeferOrMaterializeNetworkPod
// and which are tracked in s.networkServices. These get re-bundled
// into every script-runner's revision so subsequent stages of the
// same gitlab-runner job can still reach them on loopback.
func (s *Server) serviceMembersOfNetwork(netID string) []api.Container {
	v, ok := s.networkServices.Load(netID)
	if !ok {
		return nil
	}
	ids := v.([]string)
	var out []api.Container
	for _, id := range ids {
		if c, ok := s.PendingCreates.Get(id); ok {
			out = append(out, c)
			continue
		}
		// Not in PendingCreates — look up via cloud state.
		if c, ok := s.ResolveContainerAuto(s.ctx(), id); ok {
			out = append(out, c)
		}
	}
	return out
}

// trackNetworkService records a service-style container under the
// network ID so subsequent script-runners on the same network can
// re-bundle it as a sidecar.
func (s *Server) trackNetworkService(netID, containerID string) {
	if netID == "" || containerID == "" {
		return
	}
	for {
		var existing []string
		if v, ok := s.networkServices.Load(netID); ok {
			existing = v.([]string)
		}
		for _, id := range existing {
			if id == containerID {
				return
			}
		}
		updated := append([]string{}, existing...)
		updated = append(updated, containerID)
		if v, loaded := s.networkServices.LoadOrStore(netID, updated); !loaded {
			_ = v
			return
		}
		// Race with concurrent writer — retry.
	}
}

// userDefinedNetworkID returns the ID of the first user-defined network
// the container has joined, or false if none. Built-in networks (bridge,
// host, none, default) are excluded — they don't get the multi-container
// treatment.
func (s *Server) userDefinedNetworkID(c api.Container) (string, bool) {
	for netName, ep := range c.NetworkSettings.Networks {
		if isBuiltinNetwork(netName) {
			continue
		}
		if ep != nil && ep.NetworkID != "" {
			return ep.NetworkID, true
		}
		// Resolve via the network store.
		if net, ok := s.Store.ResolveNetwork(netName); ok {
			return net.ID, true
		}
	}
	// HostConfig.NetworkMode may name a network without a corresponding
	// NetworkSettings entry yet.
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

// pendingMembersOfNetwork returns every container in PendingCreates that
// has joined the given network ID, excluding `excludeID`.
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
		out = append(out, pc)
	}
	return out
}

// hostAliasesForNetwork returns the alias names registered by every
// member of the given network (each container's NetworkingConfig
// .EndpointsConfig.<net>.Aliases). Used to source SOCKERLESS_HOST_ALIASES
// at deploy time so the bootstrap can write `127.0.0.1 <alias>` lines
// to /etc/hosts.
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
		// Container hostname is also a valid alias on the network.
		if c.Config.Hostname != "" && !seen[c.Config.Hostname] {
			seen[c.Config.Hostname] = true
			out = append(out, c.Config.Hostname)
		}
		// gitlab-runner's `services:` directive also auto-aliases the
		// container by its `name` field (without leading slash). Most
		// docker clients populate Aliases explicitly so this is rarely
		// needed, but some clients only set the container name.
		bareName := strings.TrimPrefix(c.Name, "/")
		if bareName != "" && !seen[bareName] {
			// Heuristic: only consider a name an alias if it looks like a
			// service alias (no slashes, no colons, short). Skip when the
			// name is a long randomized container ID.
			if isLikelyAlias(bareName) {
				seen[bareName] = true
				out = append(out, bareName)
			}
		}
	}
	return out
}

// isLikelyAlias returns true if `s` looks like a user-supplied alias
// (short, no separators) rather than a generated container name.
func isLikelyAlias(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	if strings.ContainsAny(s, "/:") {
		return false
	}
	// Reject names that look like generated container IDs (pure hex,
	// >12 chars).
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
