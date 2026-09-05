package cloudrun

import (
	"encoding/base64"
	"encoding/json"
	"time"

	core "github.com/sockerless/backend-core"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	"google.golang.org/api/iterator"
)

// shouldDeferOrMaterializeNetworkPod decides what this container's start
// does on the network-pod path (see core.DeferOrMaterializeNetworkPod).
// Beyond the pending siblings, a script-runner also re-bundles every
// service-style container tracked on its network: gitlab-runner creates a
// new script-runner per stage (get_sources / step_script / after_script)
// in the same user-defined network, and each stage's revision needs its
// own copy of the services on loopback.
func (s *Server) shouldDeferOrMaterializeNetworkPod(c api.Container) (shouldDefer bool, members []api.Container) {
	netID, ok := s.UserDefinedNetworkID(c)
	if !ok {
		return false, nil
	}
	siblings := s.PendingMembersOfNetwork(netID, c.ID, nil)
	var pinned []api.Container
	if c.Config.OpenStdin {
		pinned = s.serviceMembersOfNetwork(netID)
	}
	return core.DeferOrMaterializeNetworkPod(c, siblings, pinned)
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
