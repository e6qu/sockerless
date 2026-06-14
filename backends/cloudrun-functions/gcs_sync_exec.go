package gcf

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// gcsSyncPreExec runs GCSSyncDriver.PreExec for every configured gcs-sync
// SharedVolume: it tars the runner-side workspace (the volume's host source)
// to a per-exec GCS object and injects a SOCKERLESS_SYNC_VOLUMES hint into the
// exec instance's env so the bootstrap restores it into the job container's
// tmpfs before running the command (the workspace mount is otherwise an empty
// tmpfs). It returns a closure that pulls each volume's modifications back to
// the runner-side path after the exec, or nil when no gcs-sync volume applies.
func (s *Server) gcsSyncPreExec(execID string) (func(), error) {
	var vols []SharedVolume
	for _, sv := range s.config.SharedVolumes {
		if core.StorageBacking(sv.Backing) == core.BackingGCSSync {
			vols = append(vols, sv)
		}
	}
	if len(vols) == 0 {
		return nil, nil
	}
	driver, err := s.storageBackings.Resolve(core.BackingGCSSync)
	if err != nil {
		return nil, err
	}
	var pairs []string
	for _, sv := range vols {
		hints, err := driver.PreExec(s.ctx(), sv.AsRef(), execID, sv.ContainerPath, "")
		if err != nil {
			return nil, fmt.Errorf("volume %q: %w", sv.Name, err)
		}
		pairs = append(pairs, hints["SOCKERLESS_SYNC_VOLUMES"]...)
	}
	if len(pairs) > 0 {
		entry := "SOCKERLESS_SYNC_VOLUMES=" + strings.Join(pairs, ",")
		s.Store.Execs.Update(execID, func(e *api.ExecInstance) {
			e.ProcessConfig.Env = append(e.ProcessConfig.Env, entry)
		})
	}
	volsCopy := vols
	return func() {
		for _, sv := range volsCopy {
			if perr := driver.PostExec(s.ctx(), sv.AsRef(), execID, sv.ContainerPath); perr != nil {
				s.Logger.Warn().Err(perr).Str("volume", sv.Name).Str("exec", execID).
					Msg("gcs-sync PostExec failed — runner-side workspace may be stale for this step")
			}
		}
	}, nil
}

// execPostHook wraps an exec stream so the gcs-sync PostExec sync-back runs
// exactly once when the caller closes the stream — by which point the exec has
// finished and the bootstrap has uploaded its workspace modifications.
type execPostHook struct {
	io.ReadWriteCloser
	once *sync.Once
	hook func()
}

func (e *execPostHook) Close() error {
	err := e.ReadWriteCloser.Close()
	e.once.Do(e.hook)
	return err
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
	netID, _ := s.userDefinedNetworkID(c)
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
