package aca

import (
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
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
					if exec.Properties.Status == nil {
						continue
					}
					switch *exec.Properties.Status {
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

// deleteJob deletes an ACA Job (best-effort, error logged).
// Used by rollback paths inside ContainerStart. ContainerRemove
// uses deleteJobStrict which propagates errors per the no-fallback
// contract.
func (s *Server) deleteJob(jobName string) {
	if err := s.deleteJobStrict(jobName); err != nil {
		s.Logger.Warn().Err(err).Str("job", jobName).Msg("deleteJob: cloud delete failed (rollback path)")
	}
}

// deleteJobStrict deletes an ACA Job and returns nil on success or
// when the job is already gone. Errors propagate.
func (s *Server) deleteJobStrict(jobName string) error {
	poller, err := s.azure.Jobs.BeginDelete(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "ResourceNotFound") {
			return nil
		}
		return fmt.Errorf("delete ACA job %q: %w", jobName, err)
	}
	if _, werr := poller.PollUntilDone(s.ctx(), nil); werr != nil {
		return fmt.Errorf("await delete ACA job %q: %w", jobName, werr)
	}
	return nil
}
