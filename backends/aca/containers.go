package aca

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
)

// signalToExitCode maps a signal name or number to the corresponding
// exit code (128 + signal number), matching Docker's behavior.
func signalToExitCode(signal string) int {
	signalMap := map[string]int{
		"SIGHUP": 129, "HUP": 129, "1": 129,
		"SIGINT": 130, "INT": 130, "2": 130,
		"SIGQUIT": 131, "QUIT": 131, "3": 131,
		"SIGABRT": 134, "ABRT": 134, "6": 134,
		"SIGKILL": 137, "KILL": 137, "9": 137,
		"SIGUSR1": 138, "USR1": 138, "10": 138,
		"SIGUSR2": 140, "USR2": 140, "12": 140,
		"SIGTERM": 143, "TERM": 143, "15": 143,
	}
	signal = strings.ToUpper(strings.TrimSpace(signal))
	if code, ok := signalMap[signal]; ok {
		return code
	}
	return 137 // default to SIGKILL
}

// waitForExecutionRunning polls until the execution reaches RUNNING state.
// Returns (agentAddr, -1, nil) if the execution is running.
// Returns ("", exitCode, nil) if the execution completed before the agent was reachable.
// Returns ("", -1, err) on failure.
func (s *Server) waitForExecutionRunning(ctx context.Context, jobName, executionName string) (string, int, error) {
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
			pager := s.azure.Executions.NewListPager(s.config.ResourceGroup, jobName, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					s.Logger.Debug().Err(err).Msg("polling execution status")
					break
				}
				for _, exec := range page.Value {
					if executionName != "" && (exec.Name == nil || *exec.Name != executionName) {
						continue
					}
					if exec.Status == nil {
						continue
					}
					switch *exec.Status {
					case armappcontainers.JobExecutionRunningStateRunning:
						return "reverse", -1, nil
					case armappcontainers.JobExecutionRunningStateFailed,
						armappcontainers.JobExecutionRunningStateDegraded:
						// Execution failed — treat as fast exit with code 1
						return "", 1, nil
					case armappcontainers.JobExecutionRunningStateStopped:
						return "", -1, fmt.Errorf("execution stopped")
					case armappcontainers.JobExecutionRunningStateSucceeded:
						// Execution completed before agent was reachable — fast exit with code 0
						return "", 0, nil
					}
				}
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

// waitForExecutionComplete blocks until the ACA Job execution completes or exitCh is closed.
// Used in reverse agent mode where the goroutine needs to wait for the cloud job to finish.
func (s *Server) waitForExecutionComplete(jobName, executionName string, exitCh chan struct{}) {
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
					// BUG-569: Guard against empty executionName
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
						return
					}
				}
			}
		}
	}
}

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
					// BUG-569: Guard against empty executionName
					if executionName != "" && (exec.Name == nil || *exec.Name != executionName) {
						continue
					}
					if exec.Status == nil {
						continue
					}
					switch *exec.Status {
					case armappcontainers.JobExecutionRunningStateSucceeded:
						if c, ok := s.Store.Containers.Get(containerID); ok && c.State.Running {
							s.Store.StopContainer(containerID, 0)
						}
						return
					case armappcontainers.JobExecutionRunningStateFailed,
						armappcontainers.JobExecutionRunningStateDegraded:
						if c, ok := s.Store.Containers.Get(containerID); ok && c.State.Running {
							s.Store.StopContainer(containerID, 1)
						}
						return
					case armappcontainers.JobExecutionRunningStateStopped:
						if c, ok := s.Store.Containers.Get(containerID); ok && c.State.Running {
							s.Store.StopContainer(containerID, 137)
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

// mergeEnvByKey merges base env vars with override env vars by key.
// Override values replace base values with the same key; order is preserved.
func mergeEnvByKey(base, override []string) []string {
	if len(override) == 0 {
		return base
	}
	if len(base) == 0 {
		return override
	}
	keys := make(map[string]string)
	order := make([]string, 0, len(base)+len(override))
	for _, e := range base {
		k, _, _ := strings.Cut(e, "=")
		keys[k] = e
		order = append(order, k)
	}
	for _, e := range override {
		k, _, _ := strings.Cut(e, "=")
		if _, exists := keys[k]; !exists {
			order = append(order, k)
		}
		keys[k] = e
	}
	result := make([]string, 0, len(order))
	for _, k := range order {
		result = append(result, keys[k])
	}
	return result
}
