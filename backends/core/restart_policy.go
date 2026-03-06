package core

import (
	"strings"
	"time"

	"github.com/sockerless/api"
)

// ShouldRestart determines whether a container should be restarted based on
// the restart policy, the exit code, and how many times it has already restarted.
func ShouldRestart(policy api.RestartPolicy, exitCode int, restartCount int) bool {
	switch policy.Name {
	case "", "no":
		return false
	case "on-failure":
		if exitCode == 0 {
			return false
		}
		if policy.MaximumRetryCount > 0 && restartCount >= policy.MaximumRetryCount {
			return false
		}
		return true
	case "always":
		return true
	case "unless-stopped":
		// Explicit stop is handled externally (handleContainerStop sets Running=false
		// before StopContainer, so the restart hook isn't invoked for explicit stops).
		return true
	default:
		return false
	}
}

// RestartDelay computes the exponential backoff delay for the given restart count.
// Formula: min(100ms * 2^restartCount, 60s).
func RestartDelay(restartCount int) time.Duration {
	base := 100 * time.Millisecond
	delay := base
	for i := 0; i < restartCount; i++ {
		delay *= 2
		if delay > 60*time.Second {
			return 60 * time.Second
		}
	}
	return delay
}

// handleRestartPolicy checks if a container should be restarted and, if so,
// re-spawns the process. Returns true if the restart was handled (caller should
// not close the wait channel).
func (s *BaseServer) handleRestartPolicy(containerID string, exitCode int) bool {
	c, ok := s.Store.Containers.Get(containerID)
	if !ok {
		return false
	}

	if !ShouldRestart(c.HostConfig.RestartPolicy, exitCode, c.RestartCount) {
		return false
	}

	// Increment restart count and apply exponential backoff delay
	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.RestartCount++
	})

	delay := RestartDelay(c.RestartCount)
	time.Sleep(delay)

	// Stop old health check before cleanup (BUG-147)
	s.StopHealthCheck(containerID)

	// Close old wait channel before creating new one (BUG-149)
	if old, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
		close(old.(chan struct{}))
	}

	// Create new wait channel
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(containerID, exitCh)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pid := s.Store.NextPID() // BUG-549
	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = pid
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
	})

	// Re-fetch fresh container after state update (BUG-148)
	c, _ = s.Store.Containers.Get(containerID)

	// Re-start health check if configured (BUG-147)
	if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
		(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
		s.StartHealthCheck(containerID)
	}

	return true
}
