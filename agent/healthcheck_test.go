package agent

import (
	"testing"
	"time"
)

// waitForStatus polls hc.Status() until it matches expected or the deadline expires.
func waitForStatus(t *testing.T, hc *HealthChecker, expected string, deadline time.Duration) {
	t.Helper()
	timeout := time.After(deadline)
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for status %q, got %q", expected, hc.Status())
		default:
			if hc.Status() == expected {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestHealthCheckHealthy(t *testing.T) {
	hc := NewHealthChecker(HealthcheckConfig{
		Test:     []string{"CMD", "true"},
		Interval: 50 * time.Millisecond,
		Timeout:  5 * time.Second,
		Retries:  3,
	}, testLogger())
	hc.Start()
	defer hc.Stop()

	waitForStatus(t, hc, "healthy", 10*time.Second)

	if streak := hc.FailingStreak(); streak != 0 {
		t.Fatalf("expected 0 failing streak, got %d", streak)
	}
}

func TestHealthCheckUnhealthy(t *testing.T) {
	hc := NewHealthChecker(HealthcheckConfig{
		Test:     []string{"CMD", "false"},
		Interval: 50 * time.Millisecond,
		Timeout:  5 * time.Second,
		Retries:  2,
	}, testLogger())
	hc.Start()
	defer hc.Stop()

	waitForStatus(t, hc, "unhealthy", 10*time.Second)

	if streak := hc.FailingStreak(); streak < 2 {
		t.Fatalf("expected failing streak >= 2, got %d", streak)
	}
}

func TestHealthCheckTimeout(t *testing.T) {
	hc := NewHealthChecker(HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "sleep 60"},
		Interval: 50 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
		Retries:  1,
	}, testLogger())
	hc.Start()
	defer hc.Stop()

	waitForStatus(t, hc, "unhealthy", 10*time.Second)

	logs := hc.Log()
	if len(logs) == 0 {
		t.Fatal("expected at least one log entry")
	}
	found := false
	for _, l := range logs {
		if l.Output == "health check timed out" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'health check timed out' in logs")
	}
}

func TestHealthCheckLogRetention(t *testing.T) {
	hc := NewHealthChecker(HealthcheckConfig{
		Test:     []string{"CMD", "true"},
		Interval: 30 * time.Millisecond,
		Timeout:  5 * time.Second,
		Retries:  3,
	}, testLogger())
	hc.Start()
	defer hc.Stop()

	// Poll until we have at least 6 checks worth of time, then verify retention cap
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for enough health check log entries")
		default:
			if len(hc.Log()) >= 5 {
				// Enough checks have run — verify cap
				if len(hc.Log()) > 5 {
					t.Fatalf("expected at most 5 log entries, got %d", len(hc.Log()))
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
