package cloudrun

import (
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
)

// pollExecutionExit monitors a Cloud Run execution and updates container state when it completes.
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
				return
			}
		}
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
