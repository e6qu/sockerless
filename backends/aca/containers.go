package aca

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
)

// pollExecutionExit monitors an ACA Job execution and updates container state when it completes.
func (s *Server) pollExecutionExit(containerID, jobName, executionName string, exitCh chan struct{}) {
	ticker := time.NewTicker(s.config.PollInterval * 2)
	defer ticker.Stop()

	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			pager := s.azure.Executions.NewListPager(s.config.ResourceGroup, jobName, nil)
			for pager.More() {
				page, err := pager.NextPage(s.ctx())
				if err != nil {
					break
				}
				for _, exec := range page.Value {
					// Guard against empty executionName
					if executionName != "" && (exec.Name == nil || *exec.Name != executionName) {
						continue
					}
					if exec.Status == nil {
						continue
					}
					switch *exec.Status {
					case armappcontainers.JobExecutionRunningStateSucceeded,
						armappcontainers.JobExecutionRunningStateFailed,
						armappcontainers.JobExecutionRunningStateDegraded,
						armappcontainers.JobExecutionRunningStateStopped:
						// Close wait channel so ContainerWait unblocks
						if ch, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
							close(ch.(chan struct{}))
						}
						return
					}
				}
			}
		}
	}
}

// stopExecution stops an ACA Job execution (best-effort), waiting for completion.
func (s *Server) stopExecution(jobName, executionName string) {
	poller, err := s.azure.Jobs.BeginStopExecution(s.ctx(), s.config.ResourceGroup, jobName, executionName, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Str("execution", executionName).Msg("failed to stop execution")
		return
	}
	_, _ = poller.PollUntilDone(s.ctx(), nil)
}

// deleteJob deletes an ACA Job (best-effort), waiting for completion.
func (s *Server) deleteJob(jobName string) {
	poller, err := s.azure.Jobs.BeginDelete(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Str("job", jobName).Msg("failed to delete job")
		return
	}
	_, _ = poller.PollUntilDone(s.ctx(), nil)
}
