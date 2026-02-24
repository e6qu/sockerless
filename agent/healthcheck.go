package agent

import (
	"bytes"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// HealthcheckConfig defines a health check to run periodically.
type HealthcheckConfig struct {
	Test        []string
	Interval    time.Duration
	Timeout     time.Duration
	StartPeriod time.Duration
	Retries     int
}

// HealthLog records a single health check execution.
type HealthLog struct {
	Start    time.Time `json:"Start"`
	End      time.Time `json:"End"`
	ExitCode int       `json:"ExitCode"`
	Output   string    `json:"Output"`
}

// HealthChecker runs periodic health checks.
type HealthChecker struct {
	config HealthcheckConfig
	logger zerolog.Logger

	mu            sync.RWMutex
	status        string // "starting", "healthy", "unhealthy"
	failingStreak int
	log           []HealthLog
	stopCh        chan struct{}
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config HealthcheckConfig, logger zerolog.Logger) *HealthChecker {
	if config.Interval == 0 {
		config.Interval = 30 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Retries == 0 {
		config.Retries = 3
	}

	return &HealthChecker{
		config: config,
		logger: logger,
		status: "starting",
		stopCh: make(chan struct{}),
	}
}

// Start begins periodic health checking.
func (hc *HealthChecker) Start() {
	go hc.run()
}

// Stop stops the health checker.
func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
}

// Status returns the current health status.
func (hc *HealthChecker) Status() string {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.status
}

// FailingStreak returns the current number of consecutive failures.
func (hc *HealthChecker) FailingStreak() int {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.failingStreak
}

// Log returns recent health check results.
func (hc *HealthChecker) Log() []HealthLog {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	result := make([]HealthLog, len(hc.log))
	copy(result, hc.log)
	return result
}

func (hc *HealthChecker) run() {
	// Wait for start period
	if hc.config.StartPeriod > 0 {
		select {
		case <-time.After(hc.config.StartPeriod):
		case <-hc.stopCh:
			return
		}
	}

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	// Run an initial check immediately
	hc.check()

	for {
		select {
		case <-ticker.C:
			hc.check()
		case <-hc.stopCh:
			return
		}
	}
}

func (hc *HealthChecker) check() {
	if len(hc.config.Test) == 0 {
		return
	}

	start := time.Now()

	// Build command — Test[0] is the type (CMD, CMD-SHELL, etc.)
	var cmd *exec.Cmd
	switch hc.config.Test[0] {
	case "CMD-SHELL":
		if len(hc.config.Test) < 2 {
			return
		}
		cmd = exec.Command("/bin/sh", "-c", hc.config.Test[1])
	case "CMD":
		if len(hc.config.Test) < 2 {
			return
		}
		cmd = exec.Command(hc.config.Test[1], hc.config.Test[2:]...)
	default:
		// Treat as raw command
		cmd = exec.Command(hc.config.Test[0], hc.config.Test[1:]...)
	}

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	// Run with timeout
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		hc.recordResult(start, 1, err.Error())
		return
	}

	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		hc.recordResult(start, exitCode, outBuf.String())
	case <-time.After(hc.config.Timeout):
		_ = cmd.Process.Kill()
		<-done
		hc.recordResult(start, 1, "health check timed out")
	}
}

func (hc *HealthChecker) recordResult(start time.Time, exitCode int, output string) {
	entry := HealthLog{
		Start:    start,
		End:      time.Now(),
		ExitCode: exitCode,
		Output:   truncateOutput(output, 4096),
	}

	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.log = append(hc.log, entry)
	// Keep last 5 entries
	if len(hc.log) > 5 {
		hc.log = hc.log[len(hc.log)-5:]
	}

	if exitCode == 0 {
		hc.failingStreak = 0
		hc.status = "healthy"
	} else {
		hc.failingStreak++
		if hc.failingStreak >= hc.config.Retries {
			hc.status = "unhealthy"
		}
	}

	hc.logger.Debug().
		Str("status", hc.status).
		Int("exitCode", exitCode).
		Int("failingStreak", hc.failingStreak).
		Msg("health check completed")
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
