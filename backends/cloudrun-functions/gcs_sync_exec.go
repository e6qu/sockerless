package gcf

import (
	"github.com/sockerless/api"
	gcpcommon "github.com/sockerless/gcp-common"
)

// gcsSyncPreExec stages every gcs-sync shared volume for the exec and
// returns the sync-back to run when the exec stream closes; nil when no
// gcs-sync volume is declared.
func (s *Server) gcsSyncPreExec(execID string) (func(), error) {
	return gcpcommon.GCSSyncPreExec(s.ctx(), s.config.SharedVolumes, s.storageBackings, execID, func(entry string) {
		s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
			e.ProcessConfig.Env = append(e.ProcessConfig.Env, entry)
		})
	}, s.Logger)
}

// materializeDeferredNetworkPodForExec lazily deploys a network-pod container
// that ContainerStart deferred (see shouldDeferOrMaterializeNetworkPod): the
// container was marked running in PendingCreates but the Cloud Run Service was
// never deployed because it was alone in its user-defined network and no later
// sibling arrived to trigger materialization. A GH actions/runner job container
// with no `services:` is exactly this case; the runner then `docker exec`s it,
// which is the trigger. Any service containers that DID arrive (and were
// themselves deferred + tracked) are bundled as sidecars so a job + its
// services deploy as one revision with shared loopback. No-op when the
// container was already materialized (not in PendingCreates).
func (s *Server) materializeDeferredNetworkPodForExec(id string) error {
	c, ok := s.PendingCreates.Get(id)
	if !ok {
		return nil // already materialized, or not a deferred container
	}
	netID, _ := s.UserDefinedNetworkID(c)
	// The job container is main (index 0); deferred service siblings on the
	// same network become sidecars in the same revision.
	members := []api.Container{c}
	members = append(members, s.pendingMembersOfNetwork(netID, id)...)
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)
	s.Logger.Info().Str("container", id).Int("members", len(members)).
		Msg("ExecStart: materializing deferred network-pod on first exec")
	if err := s.materializePodService(id, members, exitCh); err != nil {
		s.PendingCreates.Delete(id)
		return err
	}
	return nil
}
