package core

import (
	"strings"

	"github.com/sockerless/api"
)

// Containers that join the same user-defined Docker network expect to
// share a network namespace and resolve each other by alias. A backend
// whose cloud primitive runs several containers on one loopback (a
// Google Cloud Run multi-container revision, an Azure App Service
// multi-container site) materialises such a network as one pod. The
// signals are pure Docker API: network membership and
// Container.Config.OpenStdin, which a client sets on the container it is
// about to attach stdin to, i.e. the script-runner.

// IsBuiltinNetwork reports whether name is one of Docker's built-in
// networks, which never receive the pod treatment.
func IsBuiltinNetwork(name string) bool {
	switch strings.ToLower(name) {
	case "", "default", "bridge", "host", "none":
		return true
	}
	return false
}

// UserDefinedNetworkID returns the ID of the first user-defined network
// the container has joined. An endpoint that already carries its network
// ID wins; otherwise the network name is resolved through the store, and
// HostConfig.NetworkMode is consulted last because a client may name a
// network there before any NetworkSettings entry exists.
func (s *BaseServer) UserDefinedNetworkID(c api.Container) (string, bool) {
	for netName, ep := range c.NetworkSettings.Networks {
		if IsBuiltinNetwork(netName) {
			continue
		}
		if ep != nil && ep.NetworkID != "" {
			return ep.NetworkID, true
		}
		if net, ok := s.Store.ResolveNetwork(netName); ok {
			return net.ID, true
		}
	}
	if !IsBuiltinNetwork(c.HostConfig.NetworkMode) {
		if net, ok := s.Store.ResolveNetwork(c.HostConfig.NetworkMode); ok {
			return net.ID, true
		}
	}
	return "", false
}

// PendingMembersOfNetwork returns every created-but-unstarted container
// on the network other than excludeID. skip, when non-nil, drops members
// the caller has already materialised: a CI runner spawns a new
// script-runner per stage, and the previous stage's main must not be
// pulled into the next stage's pod.
func (s *BaseServer) PendingMembersOfNetwork(netID, excludeID string, skip func(api.Container) bool) []api.Container {
	var out []api.Container
	for _, pc := range s.PendingCreates.List() {
		if pc.ID == excludeID {
			continue
		}
		if mid, ok := s.UserDefinedNetworkID(pc); !ok || mid != netID {
			continue
		}
		if skip != nil && skip(pc) {
			continue
		}
		out = append(out, pc)
	}
	return out
}

// DeferOrMaterializeNetworkPod decides what a container's start does on a
// network-pod backend. siblings are the other pending members of its
// network; pinned are service-style members already deployed that a new
// script-runner should re-bundle (nil when the backend does not track
// them).
//
//   - A service-style container (no OpenStdin) with no siblings is
//     deferred: it may be a service waiting for its script-runner, or a
//     job container waiting for its services, and only the next arrival
//     tells which. With siblings, the first sibling is the main (it was
//     created first) and this container joins as a sidecar.
//   - A script-runner (OpenStdin) is the main; every sibling and pinned
//     service joins as a sidecar. Alone, it falls through to the
//     single-container path.
func DeferOrMaterializeNetworkPod(c api.Container, siblings, pinned []api.Container) (shouldDefer bool, members []api.Container) {
	if !c.Config.OpenStdin {
		if len(siblings) == 0 {
			return true, nil
		}
		all := make([]api.Container, 0, len(siblings)+1)
		all = append(all, siblings[0])
		all = append(all, siblings[1:]...)
		all = append(all, c)
		return false, all
	}
	all := make([]api.Container, 0, len(siblings)+len(pinned)+1)
	all = append(all, c)
	all = append(all, siblings...)
	for _, p := range pinned {
		dup := false
		for _, existing := range all {
			if existing.ID == p.ID {
				dup = true
				break
			}
		}
		if !dup {
			all = append(all, p)
		}
	}
	if len(all) <= 1 {
		return false, nil
	}
	return false, all
}

// HostAliasesForMembers returns every name a pod member is reachable by
// on the network: its endpoint aliases, its hostname, and its container
// name when that looks like a user-chosen alias. The bootstrap writes each
// as a `127.0.0.1` line in /etc/hosts so siblings resolve to the shared
// loopback.
func HostAliasesForMembers(members []api.Container, netID string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(a string) {
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	for _, c := range members {
		for netName, ep := range c.NetworkSettings.Networks {
			if ep == nil || (ep.NetworkID != netID && netName != netID) {
				continue
			}
			for _, a := range ep.Aliases {
				add(a)
			}
		}
		add(c.Config.Hostname)
		if bare := strings.TrimPrefix(c.Name, "/"); IsLikelyAlias(bare) {
			add(bare)
		}
	}
	return out
}

// IsLikelyAlias reports whether s looks like a user-supplied alias (short,
// no path or port separators, not a hexadecimal generated ID) rather than
// a generated container name.
func IsLikelyAlias(s string) bool {
	if len(s) == 0 || len(s) > 63 || strings.ContainsAny(s, "/:") {
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
