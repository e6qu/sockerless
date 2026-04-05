package cloudrun

import (
	"context"
	"fmt"
	"net/http"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
)

// waitForExecutionRunning polls a Cloud Run execution until it reaches RUNNING state.
// Returns (agentAddr, -1, nil) if the execution is running.
// Returns ("", exitCode, nil) if the execution completed before the agent was reachable.
// Returns ("", -1, err) on failure.
func (s *Server) waitForExecutionRunning(ctx context.Context, executionName string) (string, int, error) {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", -1, ctx.Err()
		case <-timeout:
			return "", -1, fmt.Errorf("timeout waiting for execution to reach RUNNING state")
		case <-ticker.C:
			exec, err := s.gcp.Executions.GetExecution(ctx, &runpb.GetExecutionRequest{
				Name: executionName,
			})
			if err != nil {
				s.Logger.Debug().Err(err).Msg("polling execution status")
				continue
			}

			if exec.RunningCount > 0 {
				return "reverse", -1, nil
			}

			if exec.CancelledCount > 0 {
				return "", -1, fmt.Errorf("execution was cancelled")
			}

			if exec.FailedCount > 0 {
				// Execution failed — treat as fast exit with code 1
				return "", 1, nil
			}

			if exec.SucceededCount > 0 {
				// Execution completed before agent was reachable — fast exit with code 0
				return "", 0, nil
			}
		}
	}
}

// waitForAgentHealth polls the agent's /health endpoint.
func (s *Server) waitForAgentHealth(ctx context.Context, healthURL string) error {
	timeout := time.After(s.config.AgentTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for agent health")
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
	}
}

// waitForExecutionComplete blocks until the Cloud Run execution completes or exitCh is closed.
// Used in reverse agent mode where the goroutine needs to wait for the cloud job to finish.
func (s *Server) waitForExecutionComplete(executionName string, exitCh chan struct{}) {
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
				return
			}
		}
	}
}

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
