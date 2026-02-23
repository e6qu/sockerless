package core

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sockerless/api"
)

// concurrentExecDriver is a test exec driver that tracks concurrent access.
type concurrentExecDriver struct {
	exitCode int
	output   string
	mu       sync.Mutex
	calls    int
}

func (d *concurrentExecDriver) Exec(_ context.Context, _ string, _ string,
	_ []string, _ []string, _ string, _ bool, conn net.Conn) int {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	if d.output != "" {
		conn.Write([]byte(d.output))
	}
	return d.exitCode
}

func (d *concurrentExecDriver) CallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func TestHealthCheckHealthyRace(t *testing.T) {
	driver := &concurrentExecDriver{exitCode: 0, output: "OK"}
	s := newTestServer(driver)
	createTestContainer(s, "hc-race-healthy", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "echo OK"},
		Interval: int64(50 * time.Millisecond),
		Timeout:  int64(5 * time.Second),
		Retries:  3,
	})

	s.StartHealthCheck("hc-race-healthy")
	defer s.StopHealthCheck("hc-race-healthy")

	// Concurrent reads while health check goroutine is writing
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				c, ok := s.Store.Containers.Get("hc-race-healthy")
				if !ok {
					continue
				}
				// Access Health fields — should not race
				if c.State.Health != nil {
					_ = c.State.Health.Status
					_ = c.State.Health.FailingStreak
					_ = len(c.State.Health.Log)
					for _, entry := range c.State.Health.Log {
						_ = entry.ExitCode
						_ = entry.Output
					}
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	// Wait for health check to become healthy
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, _ := s.Store.Containers.Get("hc-race-healthy")
		if c.State.Health != nil && c.State.Health.Status == "healthy" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	wg.Wait()

	c, _ := s.Store.Containers.Get("hc-race-healthy")
	if c.State.Health == nil || c.State.Health.Status != "healthy" {
		t.Fatalf("expected healthy status, got %+v", c.State.Health)
	}
}

func TestHealthCheckUnhealthyRace(t *testing.T) {
	driver := &concurrentExecDriver{exitCode: 1, output: "FAIL"}
	s := newTestServer(driver)
	createTestContainer(s, "hc-race-unhealthy", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "exit 1"},
		Interval: int64(50 * time.Millisecond),
		Timeout:  int64(5 * time.Second),
		Retries:  2,
	})

	s.StartHealthCheck("hc-race-unhealthy")
	defer s.StopHealthCheck("hc-race-unhealthy")

	// Concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				c, ok := s.Store.Containers.Get("hc-race-unhealthy")
				if ok && c.State.Health != nil {
					_ = c.State.Health.Status
					_ = c.State.Health.FailingStreak
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, _ := s.Store.Containers.Get("hc-race-unhealthy")
		if c.State.Health != nil && c.State.Health.Status == "unhealthy" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	wg.Wait()

	c, _ := s.Store.Containers.Get("hc-race-unhealthy")
	if c.State.Health == nil || c.State.Health.Status != "unhealthy" {
		t.Fatalf("expected unhealthy status, got %+v", c.State.Health)
	}
}

func TestHealthCheckStopCancelsGoroutine(t *testing.T) {
	driver := &concurrentExecDriver{exitCode: 0, output: "OK"}
	s := newTestServer(driver)
	createTestContainer(s, "hc-stop-cancel", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "echo OK"},
		Interval: int64(50 * time.Millisecond),
		Timeout:  int64(5 * time.Second),
		Retries:  3,
	})

	s.StartHealthCheck("hc-stop-cancel")

	// Wait for at least one health check to run
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if driver.CallCount() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Stop health check
	s.StopHealthCheck("hc-stop-cancel")

	// Record call count after stop
	countAfterStop := driver.CallCount()

	// Wait a bit and verify no more calls
	time.Sleep(200 * time.Millisecond)
	countLater := driver.CallCount()

	if countLater > countAfterStop+1 {
		t.Errorf("expected no more health checks after stop, but count went from %d to %d",
			countAfterStop, countLater)
	}
}
