package cloudrun

import (
	"fmt"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	gcpcommon "github.com/sockerless/gcp-common"
)

// pollExecutionExit monitors a Cloud Run execution and updates container state when it completes.
// On completion, if the container was created with HostConfig.AutoRemove=true (i.e. `docker run --rm`),
// the backend self-deletes the Cloud Run Job here so cleanup doesn't depend on the docker CLI's
// post-wait DELETE step (which doesn't reliably fire when /attach holds the connection open).
func (s *Server) pollExecutionExit(containerID, executionName string, exitCh chan struct{}) {
	ticker := time.NewTicker(s.config.PollInterval * 2)
	defer ticker.Stop()

	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			exec, err := s.gcp.Executions.GetExecution(s.ctx(), &runpb.GetExecutionRequest{
				Name: executionName,
			})
			if err != nil {
				continue
			}

			if exec.CompletionTime != nil {
				// Close wait channel so ContainerWait unblocks
				if ch, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
					close(ch.(chan struct{}))
				}
				s.maybeAutoRemove(containerID)
				return
			}
		}
	}
}

// maybeAutoRemove was the auto-remove-on-exit hook for HostConfig.AutoRemove=true
// containers. gitlab-runner / github-runner expect to `docker exec` into the
// SAME container ID across N stages even AFTER the cmd has exited (the docker
// daemon model keeps stopped containers around for inspect/exec until explicit
// remove). Cloud Run Job auto-removal on execution completion broke this —
// once the helper container's first command exited, sockerless removed the
// Job, the next exec failed with "No such container". Rule: NEVER
// auto-remove; require explicit ContainerRemove from the client.
func (s *Server) maybeAutoRemove(containerID string) {
	// No-op: auto-remove disabled to preserve docker semantics for runners.
	_ = containerID
}

// cancelExecution cancels a Cloud Run execution (best-effort), waiting for completion.
func (s *Server) cancelExecution(executionName string) {
	op, err := s.gcp.Executions.CancelExecution(s.ctx(), &runpb.CancelExecutionRequest{
		Name: executionName,
	})
	if err != nil {
		s.Logger.Debug().Err(err).Str("execution", executionName).Msg("failed to cancel execution")
		return
	}
	_, _ = op.Wait(s.ctx())
}

// deleteJob deletes a Cloud Run Job (best-effort, error logged).
// Used by rollback paths inside ContainerStart where the primary
// error already carries the operator-visible context. ContainerRemove
// uses deleteJobStrict instead — it propagates errors so `docker rm`
// only succeeds when the cloud is actually clean.
func (s *Server) deleteJob(jobName string) {
	if err := s.deleteJobStrict(jobName); err != nil {
		s.Logger.Warn().Err(err).Str("job", jobName).Msg("deleteJob: cloud delete failed (rollback path)")
	}
}

// deleteJobStrict deletes a Cloud Run Job and returns nil on success
// or when the job is already gone. Errors propagate. Used by
// ContainerRemove for the no-fallback cleanup contract. Typed
// not-found detection via gcpcommon.IsNotFound (BUG-1063).
func (s *Server) deleteJobStrict(jobName string) error {
	op, err := s.gcp.Jobs.DeleteJob(s.ctx(), &runpb.DeleteJobRequest{
		Name: jobName,
	})
	if err != nil {
		if gcpcommon.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete cloud run job %q: %w", jobName, err)
	}
	if _, werr := op.Wait(s.ctx()); werr != nil {
		return fmt.Errorf("await delete cloud run job %q: %w", jobName, werr)
	}
	return nil
}
