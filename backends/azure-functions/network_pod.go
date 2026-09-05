package azf

import (
	core "github.com/sockerless/backend-core"

	"github.com/sockerless/api"
)

// shouldDeferOrMaterializeNetworkPod decides what this container's start
// does on the network-pod path (see core.DeferOrMaterializeNetworkPod).
// On App Service the pod is one Function App site whose sitecontainers
// share one loopback, the faithful Azure analog of a Cloud Run
// multi-container revision.
func (s *Server) shouldDeferOrMaterializeNetworkPod(c api.Container) (shouldDefer bool, members []api.Container) {
	netID, ok := s.UserDefinedNetworkID(c)
	if !ok {
		return false, nil
	}
	return core.DeferOrMaterializeNetworkPod(c, s.pendingMembersOfNetwork(netID, c.ID), nil)
}

// pendingMembersOfNetwork returns every created-but-unstarted container on
// the network other than excludeID. A main container that already has a
// function URL is skipped: gitlab-runner spawns a new build container per
// stage, and the new stage's container must materialize as the main of
// its own site rather than drag the previous stage's main into a fresh
// site. Sidecars (OpenStdin=false) are not skipped: each stage's site
// needs its own copy of the services on its shared loopback.
func (s *Server) pendingMembersOfNetwork(netID, excludeID string) []api.Container {
	return s.PendingMembersOfNetwork(netID, excludeID, func(pc api.Container) bool {
		if !pc.Config.OpenStdin {
			return false
		}
		state, ok := s.AZF.Get(pc.ID)
		return ok && state.FunctionURL != ""
	})
}
