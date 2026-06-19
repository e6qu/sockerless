package aca

import (
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	azurecommon "github.com/sockerless/azure-common"
)

// pollExecutionExit monitors an ACA Job execution and updates container state when it completes.
func (s *Server) pollExecutionExit(containerID, jobName, executionName string, exitCh chan struct{}) {
	ticker := time.NewTicker(s.config.PollInterval * 2)
	defer ticker.Stop()

	gone := 0
	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			pager := s.azure.Executions.NewListPager(s.config.ResourceGroup, jobName, nil)
			found := false
			enumErr := false
			for pager.More() {
				page, err := pager.NextPage(s.ctx())
				if err != nil {
					enumErr = true
					break
				}
				for _, exec := range page.Value {
					// Guard against empty executionName
					if executionName != "" && (exec.Name == nil || *exec.Name != executionName) {
						continue
					}
					found = true
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
			if enumErr {
				continue
			}
			if !found {
				// Execution (or its Job) GC'd / deleted out-of-band — after a
				// few consecutive successful enumerations that don't list it,
				// treat it as terminal so a blocked ContainerWait unblocks
				// instead of polling forever.
				if gone++; gone >= pollGoneThreshold {
					if ch, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
						close(ch.(chan struct{}))
					}
					return
				}
			} else {
				gone = 0
			}
		}
	}
}

// pollGoneThreshold is the number of consecutive polls in which the backing
// cloud resource is absent before a poller treats the container as terminally
// gone and unblocks waiters (mirrors core.WaitGoneThreshold).
const pollGoneThreshold = 5

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
// when the job is already gone. Errors propagate. Typed not-found
// detection goes through azurecommon.IsNotFound.
func (s *Server) deleteJobStrict(jobName string) error {
	poller, err := s.azure.Jobs.BeginDelete(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		if azurecommon.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete ACA job %q: %w", jobName, err)
	}
	if _, werr := poller.PollUntilDone(s.ctx(), nil); werr != nil {
		return fmt.Errorf("await delete ACA job %q: %w", jobName, werr)
	}
	return nil
}
