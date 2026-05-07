package gcf

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
		// Service-style container (postgres, redis, etc.) OR the GH
		// actions/runner job container which is also OpenStdin=false but
		// long-lived (entrypoint=`tail -f /dev/null`-style; runner uses
		// `docker exec` for each step). Two cases:
		//
		//  - No siblings yet: defer — we don't know if a script-runner
		//    (gitlab-runner OpenStdin=true) is coming, or if this IS the
		//    GH job container that will be materialized when its services
		//    arrive. Either way, single-container materialize is wrong;
		//    wait for the second arrival to disambiguate.
		//  - Siblings exist: GH actions/runner pattern — the JOB
		//    container was created FIRST (it's siblings[0]), services
		//    after (this container). Materialize the pod with siblings[0]
		//    as main + this container as sidecar. The script-runner-with-
		//    OpenStdin=true gitlab-runner case never lands here because it
		//    has OpenStdin=true and falls through to the next branch.
		if len(siblings) == 0 {
			return true, nil
		}
		all := make([]api.Container, 0, len(siblings)+1)
		all = append(all, siblings[0])     // FIRST sibling = main (GH job container)
		all = append(all, siblings[1:]...) // other siblings (additional services, if any)
		all = append(all, c)               // self — sidecar (this just-arrived service)
		return false, all
	}

	// Script-runner (gitlab-runner pattern): materialize the pod with
	// this container as main + every pending sibling as sidecar.
	if len(siblings) == 0 {
		return false, nil
	}
	all := make([]api.Container, 0, len(siblings)+1)
	all = append(all, c) // main first — materializePodService uses members[0].ID
	all = append(all, siblings...)
	return false, all
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
// has joined the given network ID, excluding `excludeID`. Already-materialized
// MAIN containers (OpenStdin=true with a populated FunctionURL) are filtered
// out — gitlab-runner v17 spawns a NEW build container per stage with a
// different image (helper for prep/get_sources, user image for step_script).
// The new stage's container must materialize as the MAIN of its own pod-Service
// revision; pulling the previous stage's main into the new revision would
// either collide on container names or, after the materialize cleanup step
// runs DeleteFunction on the previous main's allocation, leave the previous
// pod-Service unreachable for cleanup_file_variables (the v25 "no service
// URL" failure mode).
//
// Sidecars (OpenStdin=false: postgres, redis, etc.) are NOT filtered. Each
// stage's pod-Service revision needs its own copy of the sidecars — the
// runner expects them on `127.0.0.1` from inside the build container, which
// requires they share the same Cloud Run revision (loopback is per-revision).
// They stay in PendingCreates across stages.
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
			if state, ok := s.resolveGCFFromCloud(s.ctx(), pc.ID); ok && state.FunctionURL != "" {
				continue
			}
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
