package cloudrun

import (
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
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

// maybeAutoRemove deletes the cloud-side Cloud Run Job (and clears the Store/CloudRun
// entries) for a container created with HostConfig.AutoRemove=true. Called from the
// execution-exit poller after the wait channel has been signaled. Cloud Run is a
// stateless backend (Store.Containers is empty), so AutoRemove rides on the
// SOCKERLESS_AUTOREMOVE annotation that buildJobSpec writes onto the cloud-side Job.
func (s *Server) maybeAutoRemove(containerID string) {
	c, ok := s.ResolveContainerAuto(s.ctx(), containerID)
	if !ok {
		return
	}
	if !c.HostConfig.AutoRemove {
		return
	}
	if err := s.ContainerRemove(containerID, false); err != nil {
		s.Logger.Warn().Err(err).Str("container", containerID[:12]).Msg("auto-remove failed")
	}
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

// deleteJob deletes a Cloud Run Job (best-effort), waiting for completion.
func (s *Server) deleteJob(jobName string) {
	op, err := s.gcp.Jobs.DeleteJob(s.ctx(), &runpb.DeleteJobRequest{
		Name: jobName,
	})
	if err != nil {
		s.Logger.Debug().Err(err).Str("job", jobName).Msg("failed to delete job")
		return
	}
	_, _ = op.Wait(s.ctx())
}
