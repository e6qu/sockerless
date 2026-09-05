package gcf

import (
	core "github.com/sockerless/backend-core"

	"github.com/sockerless/api"
)

// shouldDeferOrMaterializeNetworkPod decides what this container's start
// does on the network-pod path (see core.DeferOrMaterializeNetworkPod).
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
// stage with a different image, and each stage's container must
// materialize as the main of its own pod-Service revision rather than
// pull the previous stage's main into the new revision, which would
// collide on container names or leave the previous pod-Service
// unreachable for cleanup. Sidecars (OpenStdin=false: postgres, redis)
// are not skipped: each stage's revision needs its own copy on loopback.
func (s *Server) pendingMembersOfNetwork(netID, excludeID string) []api.Container {
	return s.PendingMembersOfNetwork(netID, excludeID, func(pc api.Container) bool {
		if !pc.Config.OpenStdin {
			return false
		}
		state, ok := s.resolveGCFFromCloud(s.ctx(), pc.ID)
		return ok && state.FunctionURL != ""
	})
}
