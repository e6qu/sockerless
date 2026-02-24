package core

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// Default health check timing (Docker defaults, in nanoseconds).
const (
	defaultHealthInterval    = 30 * time.Second
	defaultHealthTimeout     = 30 * time.Second
	defaultHealthStartPeriod = 0
	defaultHealthRetries     = 3
	maxHealthLogEntries      = 5
)

// StartHealthCheck begins periodic health checking for a container.
// It parses the container's Healthcheck config and spawns a goroutine
// that runs the check command on the configured interval.
func (s *BaseServer) StartHealthCheck(containerID string) {
	c, ok := s.Store.Containers.Get(containerID)
	if !ok || c.Config.Healthcheck == nil {
		return
	}

	hc := c.Config.Healthcheck
	if len(hc.Test) == 0 {
		return
	}

	// Parse test command
	cmd := parseHealthcheckCmd(hc.Test)
	if cmd == nil {
		return // NONE or invalid
	}

	// Parse timing with defaults
	interval := defaultHealthInterval
	if hc.Interval > 0 {
		interval = time.Duration(hc.Interval)
	}
	timeout := defaultHealthTimeout
	if hc.Timeout > 0 {
		timeout = time.Duration(hc.Timeout)
	}
	startPeriod := time.Duration(defaultHealthStartPeriod)
	if hc.StartPeriod > 0 {
		startPeriod = time.Duration(hc.StartPeriod)
	}
	retries := defaultHealthRetries
	if hc.Retries > 0 {
		retries = hc.Retries
	}

	// Initialize health state
	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.State.Health = &api.HealthState{
			Status: "starting",
			Log:    []api.HealthLog{},
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	s.Store.HealthChecks.Store(containerID, cancel)

	go s.runHealthCheckLoop(ctx, containerID, cmd, interval, timeout, startPeriod, retries)
}

// StopHealthCheck cancels the health check goroutine for a container.
func (s *BaseServer) StopHealthCheck(containerID string) {
	if cancel, ok := s.Store.HealthChecks.LoadAndDelete(containerID); ok {
		cancel.(context.CancelFunc)()
	}
}

// runHealthCheckLoop is the health check goroutine that periodically
// executes the health check command and updates container state.
func (s *BaseServer) runHealthCheckLoop(ctx context.Context, containerID string, cmd []string,
	interval, timeout, startPeriod time.Duration, retries int) {

	// Wait for start period
	if startPeriod > 0 {
		select {
		case <-time.After(startPeriod):
		case <-ctx.Done():
			return
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run first check immediately, then on interval
	for {
		exitCode, output := s.execHealthCheck(ctx, containerID, cmd, timeout)
		if ctx.Err() != nil {
			return
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		s.Store.Containers.Update(containerID, func(c *api.Container) {
			if c.State.Health == nil {
				return
			}

			entry := api.HealthLog{
				Start:    now,
				End:      time.Now().UTC().Format(time.RFC3339Nano),
				ExitCode: exitCode,
				Output:   truncateOutput(output, 4096),
			}

			// Deep-copy HealthState to avoid racing with concurrent Get() callers
			// that hold a reference to the old *HealthState pointer.
			newHealth := *c.State.Health
			newLog := make([]api.HealthLog, len(newHealth.Log))
			copy(newLog, newHealth.Log)
			newLog = append(newLog, entry)
			if len(newLog) > maxHealthLogEntries {
				newLog = newLog[len(newLog)-maxHealthLogEntries:]
			}
			newHealth.Log = newLog

			if exitCode == 0 {
				newHealth.Status = "healthy"
				newHealth.FailingStreak = 0
			} else {
				newHealth.FailingStreak++
				if newHealth.FailingStreak >= retries {
					newHealth.Status = "unhealthy"
				}
			}
			c.State.Health = &newHealth
		})

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// execHealthCheck runs a single health check command via the exec driver.
func (s *BaseServer) execHealthCheck(ctx context.Context, containerID string, cmd []string, timeout time.Duration) (int, string) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	clientConn, serverConn := net.Pipe()
	var exitCode int
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer serverConn.Close()
		exitCode = s.Drivers.Exec.Exec(execCtx, containerID, "healthcheck-"+GenerateID()[:8],
			cmd, nil, "", true, serverConn)
	}()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, clientConn)
	clientConn.Close()
	<-done

	return exitCode, buf.String()
}

// parseHealthcheckCmd converts a Healthcheck.Test array into an exec command.
// Returns nil for NONE or invalid formats.
func parseHealthcheckCmd(test []string) []string {
	if len(test) == 0 {
		return nil
	}
	switch strings.ToUpper(test[0]) {
	case "NONE":
		return nil
	case "CMD":
		if len(test) < 2 {
			return nil
		}
		return test[1:]
	case "CMD-SHELL":
		if len(test) < 2 {
			return nil
		}
		return []string{"sh", "-c", strings.Join(test[1:], " ")}
	default:
		// Treat as raw command (legacy format)
		return test
	}
}

// truncateOutput truncates output to maxLen bytes.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
