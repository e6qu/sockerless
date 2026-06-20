package cloudrun

import (
	"fmt"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	gcpcommon "github.com/sockerless/gcp-common"
)

// pollExecutionExit monitors a Cloud Run execution and closes the container's
// wait channel when it completes. The backend deliberately does NOT auto-remove
// the Cloud Run Job on completion (even for HostConfig.AutoRemove=true): the
// gitlab-runner / github-runner docker-executor model `docker exec`s into the
// SAME container ID across N stages after its first command has exited, so the
// Job must persist until an explicit ContainerRemove from the client.
func (s *Server) pollExecutionExit(containerID, executionName string, exitCh chan struct{}) {
	ticker := time.NewTicker(s.config.PollInterval * 2)
	defer ticker.Stop()

	gone := 0
	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			exec, err := s.gcp.Executions.GetExecution(s.ctx(), &runpb.GetExecutionRequest{
				Name: executionName,
			})
			if err != nil {
				// Execution deleted out-of-band (NotFound) or a sustained
				// outage — after a few consecutive failures treat it as
				// terminal so a blocked ContainerWait unblocks instead of
				// polling forever.
				if gone++; gone >= pollGoneThreshold {
					if ch, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
						close(ch.(chan struct{}))
					}
					return
				}
				continue
			}
			gone = 0

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

// pollGoneThreshold is the number of consecutive polls in which the backing
// cloud resource is absent before a poller treats the container as terminally
// gone and unblocks waiters (mirrors core.WaitGoneThreshold).
const pollGoneThreshold = 5

// cancelExecution cancels a Cloud Run execution (best-effort), waiting for
// completion. Used by rollback paths inside ContainerStart where the primary
// error already carries the operator-visible context. Stop/Kill/Remove use
// cancelExecutionStrict so a failed teardown surfaces instead of leaving a
// billable execution running while the docker op reports success.
func (s *Server) cancelExecution(executionName string) {
	if err := s.cancelExecutionStrict(executionName); err != nil {
		s.Logger.Warn().Err(err).Str("execution", executionName).Msg("cancelExecution: cloud cancel failed (rollback path)")
	}
}

// cancelExecutionStrict cancels a Cloud Run execution and returns nil on
// success or when the execution is already gone. Errors propagate so the
// no-fallback teardown contract holds — `docker stop/kill/rm` only succeeds
// when the cloud execution is actually cancelled.
func (s *Server) cancelExecutionStrict(executionName string) error {
	op, err := s.gcp.Executions.CancelExecution(s.ctx(), &runpb.CancelExecutionRequest{
		Name: executionName,
	})
	if err != nil {
		if gcpcommon.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("cancel cloud run execution %q: %w", executionName, err)
	}
	if _, werr := op.Wait(s.ctx()); werr != nil {
		return fmt.Errorf("await cancel cloud run execution %q: %w", executionName, werr)
	}
	return nil
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
// not-found detection via gcpcommon.IsNotFound.
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
