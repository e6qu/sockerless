package cloudrun

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	"google.golang.org/api/iterator"
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
		// Service-style container OR the GH actions/runner job container
		// (which is also OpenStdin=false but long-lived; the runner uses
		// `docker exec` per step). Two cases:
		//
		//  - No siblings yet: defer — could be a service waiting for a
		//    script-runner (gitlab-runner, OpenStdin=true) OR the GH job
		//    container that will be materialized when its services arrive.
		//  - Siblings exist: GH actions/runner pattern — the JOB container
		//    was created FIRST (siblings[0]), services after (current).
		//    Materialize with siblings[0] as main + this container as
		//    sidecar. The gitlab-runner case never lands here because the
		//    script-runner has OpenStdin=true and falls through.
		if len(siblings) == 0 {
			return true, nil
		}
		all := make([]api.Container, 0, len(siblings)+1)
		all = append(all, siblings[0])     // FIRST sibling = main (GH job container)
		all = append(all, siblings[1:]...) // additional siblings
		all = append(all, c)               // self — sidecar
		return false, all
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
	if _, ok := s.networkServices.Load(netID); !ok {
		// Cache miss — typically after a backend restart, where the map was
		// lost but the members survive as bundled sidecars on the network's
		// latest Service revision. Rebuild once per network (a service-less
		// network must not re-list Services on every stage).
		if _, attempted := s.networkRebuilt.LoadOrStore(netID, true); !attempted {
			s.rebuildNetworkServicesFromCloud(netID)
		}
	}
	v, ok := s.networkServices.Load(netID)
	if !ok {
		return nil
	}
	ids := asStringSlice(v)
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

// rebuildNetworkServicesFromCloud reconstructs networkServices[netID] after a
// restart by reading the service-style members persisted on the network's
// latest Cloud Run Service revision (servicespec.go writes them as
// annotations). Each recovered member is re-seeded into PendingCreates so the
// next script-runner stage re-bundles it as a sidecar, exactly as it would
// have within a single process lifetime. Best-effort: a list/decode failure
// is logged and leaves the map empty (the stage proceeds without the sidecar
// rather than crashing) — the cloud Service revision remains the source of
// truth.
func (s *Server) rebuildNetworkServicesFromCloud(netID string) {
	if netID == "" || s.gcp == nil || s.gcp.Services == nil {
		return
	}
	ctx := s.ctx()
	it := s.gcp.Services.ListServices(ctx, &runpb.ListServicesRequest{
		Parent: s.buildServiceParent(),
	})
	var latestBlob string
	var latest time.Time
	for {
		svc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.Logger.Warn().Err(err).Str("net_id", netID).Msg("rebuildNetworkServices: list Services failed")
			return
		}
		if svc.Annotations[networkIDAnnotation] != netID {
			continue
		}
		blob := svc.Annotations[networkServiceMembersAnnotation]
		if blob == "" {
			continue
		}
		ct := time.Time{}
		if svc.CreateTime != nil {
			ct = svc.CreateTime.AsTime()
		}
		if latestBlob == "" || ct.After(latest) {
			latestBlob, latest = blob, ct
		}
	}
	if latestBlob == "" {
		return
	}
	n := s.applyNetworkServiceMembers(netID, latestBlob)
	if n > 0 {
		s.Logger.Info().Str("net_id", netID).Int("members", n).
			Msg("rebuildNetworkServices: restored network service members from cloud after restart")
	}
}

// applyNetworkServiceMembers decodes the base64-JSON member blob persisted on
// a network's Service revision and re-seeds each member into PendingCreates +
// networkServices, so the next script-runner stage re-bundles it as a sidecar.
// Returns the number of members applied.
func (s *Server) applyNetworkServiceMembers(netID, blob string) int {
	decoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		s.Logger.Warn().Err(err).Str("net_id", netID).Msg("rebuildNetworkServices: decode members failed")
		return 0
	}
	var members []api.Container
	if err := json.Unmarshal(decoded, &members); err != nil {
		s.Logger.Warn().Err(err).Str("net_id", netID).Msg("rebuildNetworkServices: unmarshal members failed")
		return 0
	}
	n := 0
	for i := range members {
		m := members[i]
		if m.ID == "" {
			continue
		}
		if _, ok := s.PendingCreates.Get(m.ID); !ok {
			s.PendingCreates.Put(m.ID, m)
		}
		s.trackNetworkService(netID, m.ID)
		n++
	}
	return n
}

// trackNetworkService records a service-style container under the
// network ID so subsequent script-runners on the same network can
// re-bundle it as a sidecar.
func (s *Server) trackNetworkService(netID, containerID string) {
	if netID == "" || containerID == "" {
		return
	}
	// Atomic read-modify-write: LoadOrStore cannot update an existing key (it
	// returns the present value without storing, so the old code livelocked on
	// the 2nd container of any network) — a plain Load→append→Store under a
	// mutex is required.
	s.networkServicesMu.Lock()
	defer s.networkServicesMu.Unlock()
	var existing []string
	if v, ok := s.networkServices.Load(netID); ok {
		existing = asStringSlice(v)
	}
	for _, id := range existing {
		if id == containerID {
			return
		}
	}
	updated := append(append([]string{}, existing...), containerID)
	s.networkServices.Store(netID, updated)
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
