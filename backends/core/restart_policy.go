package core

import (
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

	// Increment restart count
	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.RestartCount++
	})

	// Clean up old process
	s.Drivers.ProcessLifecycle.Cleanup(containerID)

	// Re-spawn
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(containerID, exitCh)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = 42
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
	})

	cmd := append([]string{c.Path}, c.Args...)
	binds := s.resolveBindMounts(c.HostConfig.Binds, c.HostConfig.Mounts)
	_, err := s.Drivers.ProcessLifecycle.Start(containerID, cmd, c.Config.Env, binds)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", containerID).Msg("failed to restart container")
		return false
	}

	return true
}
