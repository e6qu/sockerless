package core

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// testExecDriver is a mock ExecDriver that returns a configurable exit code.
type testExecDriver struct {
	exitCode int
	output   string
}

func (d *testExecDriver) Exec(_ context.Context, _ string, _ string,
	_ []string, _ []string, _ string, _ bool, conn net.Conn) int {
	if d.output != "" {
		conn.Write([]byte(d.output))
	}
	return d.exitCode
}

// newTestServer creates a minimal BaseServer for health check testing.
func newTestServer(execDriver ExecDriver) *BaseServer {
	store := NewStore()
	logger := zerolog.Nop()
	s := &BaseServer{
		Store:         store,
		Logger:        logger,
		AgentRegistry: NewAgentRegistry(),
		Desc:          BackendDescriptor{Driver: "test"},
	}
	s.InitDrivers()
	// Override the exec driver with our test driver
	s.Drivers.Exec = execDriver
	return s
}

func createTestContainer(s *BaseServer, id string, healthcheck *api.HealthcheckConfig) {
	c := api.Container{
		ID:   id,
		Name: "/" + id,
		Config: api.ContainerConfig{
			Healthcheck: healthcheck,
		},
		State: api.ContainerState{
			Status:  "running",
			Running: true,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	s.Store.Containers.Put(id, c)
}

func TestHealthCheckNoConfig(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})
	createTestContainer(s, "no-hc", nil)

	s.StartHealthCheck("no-hc")

	c, _ := s.Store.Containers.Get("no-hc")
	if c.State.Health != nil {
		t.Fatalf("expected State.Health to be nil, got %+v", c.State.Health)
	}
}

func TestHealthCheckNone(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})
	createTestContainer(s, "hc-none", &api.HealthcheckConfig{
		Test: []string{"NONE"},
	})

	s.StartHealthCheck("hc-none")

	c, _ := s.Store.Containers.Get("hc-none")
	if c.State.Health != nil {
		t.Fatalf("expected State.Health to be nil for NONE, got %+v", c.State.Health)
	}
}

func TestHealthCheckInit(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})
	createTestContainer(s, "hc-init", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "exit 0"},
		Interval: int64(1 * time.Hour), // long interval so first check doesn't complete
	})

	s.StartHealthCheck("hc-init")
	defer s.StopHealthCheck("hc-init")

	// Health state should be initialized immediately
	c, _ := s.Store.Containers.Get("hc-init")
	if c.State.Health == nil {
		t.Fatal("expected State.Health to be initialized")
	}
	if c.State.Health.Status != "starting" {
		t.Fatalf("expected status 'starting', got %q", c.State.Health.Status)
	}
}

func TestHealthCheckHealthy(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0, output: "OK"})
	createTestContainer(s, "hc-healthy", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "echo OK"},
		Interval: int64(50 * time.Millisecond),
		Timeout:  int64(5 * time.Second),
		Retries:  3,
	})

	s.StartHealthCheck("hc-healthy")
	defer s.StopHealthCheck("hc-healthy")

	// Wait for health check to transition to healthy
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, _ := s.Store.Containers.Get("hc-healthy")
		if c.State.Health != nil && c.State.Health.Status == "healthy" {
			// Verify log entry
			if len(c.State.Health.Log) == 0 {
				t.Fatal("expected at least one log entry")
			}
			if c.State.Health.Log[0].ExitCode != 0 {
				t.Fatalf("expected exit code 0, got %d", c.State.Health.Log[0].ExitCode)
			}
			if c.State.Health.Log[0].Output != "OK" {
				t.Fatalf("expected output 'OK', got %q", c.State.Health.Log[0].Output)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("health check did not transition to healthy within 2s")
}

func TestHealthCheckUnhealthy(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 1, output: "FAIL"})
	createTestContainer(s, "hc-unhealthy", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "exit 1"},
		Interval: int64(50 * time.Millisecond),
		Timeout:  int64(5 * time.Second),
		Retries:  2,
	})

	s.StartHealthCheck("hc-unhealthy")
	defer s.StopHealthCheck("hc-unhealthy")

	// Wait for health check to transition to unhealthy
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, _ := s.Store.Containers.Get("hc-unhealthy")
		if c.State.Health != nil && c.State.Health.Status == "unhealthy" {
			if c.State.Health.FailingStreak < 2 {
				t.Fatalf("expected FailingStreak >= 2, got %d", c.State.Health.FailingStreak)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("health check did not transition to unhealthy within 2s")
}

func TestHealthCheckCleanup(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})
	createTestContainer(s, "hc-cleanup", &api.HealthcheckConfig{
		Test:     []string{"CMD-SHELL", "exit 0"},
		Interval: int64(50 * time.Millisecond),
		Timeout:  int64(5 * time.Second),
		Retries:  3,
	})

	s.StartHealthCheck("hc-cleanup")

	// Verify cancel func is stored
	if _, ok := s.Store.HealthChecks.Load("hc-cleanup"); !ok {
		t.Fatal("expected HealthChecks entry to exist")
	}

	s.StopHealthCheck("hc-cleanup")

	// Verify cancel func is removed
	if _, ok := s.Store.HealthChecks.Load("hc-cleanup"); ok {
		t.Fatal("expected HealthChecks entry to be removed after StopHealthCheck")
	}

	// Give goroutine time to exit cleanly
	time.Sleep(100 * time.Millisecond)

	// Double stop should not panic
	s.StopHealthCheck("hc-cleanup")
}
