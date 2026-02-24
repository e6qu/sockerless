package agent

import (
	"testing"
	"time"
)

func TestHealthCheckHealthy(t *testing.T) {
	hc := NewHealthChecker(HealthcheckConfig{
		Test:     []string{"CMD", "true"},
		Interval: 50 * time.Millisecond,
		Timeout:  5 * time.Second,
		Retries:  3,
	}, testLogger())
	hc.Start()
	defer hc.Stop()

	// Wait for at least one check
	time.Sleep(200 * time.Millisecond)

	if status := hc.Status(); status != "healthy" {
		t.Fatalf("expected healthy, got %q", status)
	}
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

	// Wait for enough checks to hit retries threshold
	time.Sleep(400 * time.Millisecond)

	if status := hc.Status(); status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q", status)
	}
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

	// Wait for the timeout + a check
	time.Sleep(500 * time.Millisecond)

	if status := hc.Status(); status != "unhealthy" {
		t.Fatalf("expected unhealthy after timeout, got %q", status)
	}

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

	// Wait for more than 5 checks
	time.Sleep(400 * time.Millisecond)

	logs := hc.Log()
	if len(logs) > 5 {
		t.Fatalf("expected at most 5 log entries, got %d", len(logs))
	}
}
