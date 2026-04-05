package core

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	"gopkg.in/yaml.v3"
)

// specPath is the OpenAPI spec that drives this test.
type specDoc struct {
	Paths map[string]map[string]specOperation `yaml:"paths"`
}

type specOperation struct {
	OperationID     string          `yaml:"operationId"`
	GoMethod        string          `yaml:"x-sockerless-go-method"`
	StateTransition *specTransition `yaml:"x-sockerless-state-transition"`
	Events          []specEvent     `yaml:"x-sockerless-events"`
}

type specTransition struct {
	From      []string `yaml:"from"`
	To        string   `yaml:"to"`
	ForceFrom []string `yaml:"force-from"`
}

type specEvent struct {
	Type   string `yaml:"type"`
	Action string `yaml:"action"`
}

func newSpecTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:          store,
		Logger:         zerolog.Nop(),
		Mux:            http.NewServeMux(),
		AgentRegistry:  NewAgentRegistry(),
		EventBus:       NewEventBus(),
		PendingCreates: NewStateStore[api.Container](),
	}
	s.InitDrivers()
	s.self = s
	store.RestartHook = s.handleRestartPolicy
	return s
}

func seedImage(s *BaseServer) {
	s.Store.Images.Put("sha256:test", api.Image{
		ID:       "sha256:test",
		RepoTags: []string{"alpine:latest"},
	})
}

// createContainerInState creates a container and sets its state directly in the store.
// This avoids process lifecycle side effects (immediate die events from synthetic drivers).
func createContainerInState(s *BaseServer, state string) (string, error) {
	seedImage(s)

	id := GenerateID()
	name := "/" + GenerateName()

	c := api.Container{
		ID:   id,
		Name: name,
		Config: api.ContainerConfig{
			Image: "alpine:latest",
			Cmd:   []string{"echo", "test"},
		},
		HostConfig: api.HostConfig{NetworkMode: "default"},
		State: api.ContainerState{
			Status: "created",
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{},
		},
	}

	// Set state based on desired precondition
	switch state {
	case "created":
		c.State.Status = "created"
	case "running":
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = 42
		c.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	case "paused":
		c.State.Status = "paused"
		c.State.Running = true
		c.State.Paused = true
		c.State.Pid = 42
		c.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	case "exited":
		c.State.Status = "exited"
		c.State.ExitCode = 0
		c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	s.Store.Containers.Put(id, c)
	s.Store.ContainerNames.Put(name, id)

	// Set up a wait channel (some operations need it)
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	return id, nil
}

// verifyState checks that a container's state matches the expected spec state.
// It checks the Status field directly, which is the canonical state indicator.
func verifyState(t *testing.T, s *BaseServer, id, expectedState string) {
	t.Helper()
	c, ok := s.Store.ResolveContainer(id)
	if expectedState == "removed" {
		if ok {
			t.Errorf("expected container %s to be removed, but it still exists", id)
		}
		return
	}
	if !ok {
		t.Fatalf("container %s not found in store", id)
	}
	if c.State.Status != expectedState {
		t.Errorf("expected Status=%q, got %q (Running=%v, Paused=%v)",
			expectedState, c.State.Status, c.State.Running, c.State.Paused)
	}
}

// drainEvents reads all pending events from the channel within a timeout.
func drainEvents(ch <-chan api.Event, expected int) []api.Event {
	var events []api.Event
	timeout := time.After(500 * time.Millisecond)
	for i := 0; i < expected; i++ {
		select {
		case ev := <-ch:
			events = append(events, ev)
		case <-timeout:
			return events
		}
	}
	return events
}

// flushEvents drains all pending events without blocking long.
func flushEvents(ch <-chan api.Event) {
	for {
		select {
		case <-ch:
		case <-time.After(50 * time.Millisecond):
			return
		}
	}
}

// callMethod invokes the typed Backend method corresponding to a spec operation.
func callMethod(s *BaseServer, method string, id string) error {
	switch method {
	case "ContainerStart":
		return s.ContainerStart(id)
	case "ContainerStop":
		return s.ContainerStop(id, nil)
	case "ContainerRestart":
		return s.ContainerRestart(id, nil)
	case "ContainerKill":
		return s.ContainerKill(id, "SIGKILL")
	case "ContainerRemove":
		return s.ContainerRemove(id, false)
	case "ContainerPause":
		return s.ContainerPause(id)
	case "ContainerUnpause":
		return s.ContainerUnpause(id)
	default:
		return nil
	}
}

func TestSpecStateTransitions(t *testing.T) {
	specData, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc specDoc
	if err := yaml.Unmarshal(specData, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	// Collect operations with state transitions (excluding ContainerCreate which has no from-state).
	type transitionTest struct {
		method    string
		fromState string
		toState   string
		events    []specEvent
	}

	var tests []transitionTest
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.StateTransition == nil || op.GoMethod == "" {
				continue
			}
			if op.GoMethod == "ContainerCreate" {
				continue // tested separately
			}
			for _, from := range op.StateTransition.From {
				tests = append(tests, transitionTest{
					method:    op.GoMethod,
					fromState: from,
					toState:   op.StateTransition.To,
					events:    op.Events,
				})
			}
		}
	}

	if len(tests) == 0 {
		t.Fatal("no state transition tests found in spec")
	}

	for _, tt := range tests {
		t.Run(tt.method+"/from_"+tt.fromState, func(t *testing.T) {
			s := newSpecTestServer()
			ch := s.EventBus.Subscribe("spec-test")
			defer s.EventBus.Unsubscribe("spec-test")

			// Create container in precondition state
			id, err := createContainerInState(s, tt.fromState)
			if err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			// Flush all setup events
			flushEvents(ch)

			// Execute the operation under test
			if err := callMethod(s, tt.method, id); err != nil {
				t.Fatalf("%s failed: %v", tt.method, err)
			}

			// Verify postcondition state
			verifyState(t, s, id, tt.toState)

			// Verify emitted events — collect all events emitted
			got := drainEvents(ch, len(tt.events)+4) // read generously

			// Build expected events, accounting for conditional emission:
			// - ContainerRestart emits die+stop only when was running/paused
			expectedEvents := tt.events
			if tt.method == "ContainerRestart" && tt.fromState != "running" && tt.fromState != "paused" {
				var filtered []specEvent
				for _, e := range expectedEvents {
					if e.Action == "die" || e.Action == "stop" {
						continue
					}
					filtered = append(filtered, e)
				}
				expectedEvents = filtered
			}

			// Verify that all expected events appear in order (allowing extra
			// events from process lifecycle, e.g. die from immediate process exit).
			ei := 0
			for _, ev := range got {
				if ei < len(expectedEvents) && ev.Type == expectedEvents[ei].Type && ev.Action == expectedEvents[ei].Action {
					ei++
				}
			}
			if ei != len(expectedEvents) {
				t.Errorf("expected events not found in order: want %v, got:", expectedEvents)
				for i, ev := range got {
					t.Logf("  event[%d]: type=%q action=%q", i, ev.Type, ev.Action)
				}
			}
		})
	}
}

func TestSpecStateTransition_ContainerCreate(t *testing.T) {
	specData, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc specDoc
	if err := yaml.Unmarshal(specData, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	// Find ContainerCreate operation
	var createOp *specOperation
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.GoMethod == "ContainerCreate" {
				copy := op
				createOp = &copy
				break
			}
		}
	}
	if createOp == nil {
		t.Fatal("ContainerCreate not found in spec")
	}
	if createOp.StateTransition == nil {
		t.Fatal("ContainerCreate has no state transition in spec")
	}

	s := newSpecTestServer()
	ch := s.EventBus.Subscribe("spec-test")
	defer s.EventBus.Unsubscribe("spec-test")

	seedImage(s)
	resp, err := s.ContainerCreate(&api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{
			Image: "alpine:latest",
			Cmd:   []string{"echo", "hi"},
		},
	})
	if err != nil {
		t.Fatalf("ContainerCreate failed: %v", err)
	}

	// Verify postcondition: state = created
	verifyState(t, s, resp.ID, createOp.StateTransition.To)

	// Verify events
	got := drainEvents(ch, len(createOp.Events))
	if len(got) != len(createOp.Events) {
		t.Fatalf("expected %d events, got %d", len(createOp.Events), len(got))
	}
	for i, want := range createOp.Events {
		if got[i].Type != want.Type || got[i].Action != want.Action {
			t.Errorf("event[%d]: expected type=%q action=%q, got type=%q action=%q",
				i, want.Type, want.Action, got[i].Type, got[i].Action)
		}
	}
}

func TestSpecForceRemove(t *testing.T) {
	specData, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc specDoc
	if err := yaml.Unmarshal(specData, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	// Find ContainerRemove with force-from
	var removeOp *specOperation
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.GoMethod == "ContainerRemove" {
				copy := op
				removeOp = &copy
				break
			}
		}
	}
	if removeOp == nil || removeOp.StateTransition == nil || len(removeOp.StateTransition.ForceFrom) == 0 {
		t.Fatal("ContainerRemove force-from not found in spec")
	}

	// Test force-remove from each force-from state
	for _, fromState := range removeOp.StateTransition.ForceFrom {
		t.Run("force_from_"+fromState, func(t *testing.T) {
			s := newSpecTestServer()

			id, err := createContainerInState(s, fromState)
			if err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			// Force remove
			if err := s.ContainerRemove(id, true); err != nil {
				t.Fatalf("force remove from %s failed: %v", fromState, err)
			}

			// Verify removed
			verifyState(t, s, id, "removed")
		})
	}
}

func TestSpecOperationCoverage(t *testing.T) {
	specData, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc specDoc
	if err := yaml.Unmarshal(specData, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	// All operations with state transitions should be callable
	supported := map[string]bool{
		"ContainerCreate":  true,
		"ContainerStart":   true,
		"ContainerStop":    true,
		"ContainerRestart": true,
		"ContainerKill":    true,
		"ContainerRemove":  true,
		"ContainerPause":   true,
		"ContainerUnpause": true,
	}

	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.StateTransition == nil || op.GoMethod == "" {
				continue
			}
			if !supported[op.GoMethod] {
				t.Errorf("spec declares state transition for %s but test has no handler for it", op.GoMethod)
			}
		}
	}

	// Verify all supported methods actually appear in the spec
	found := make(map[string]bool)
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.StateTransition != nil {
				found[op.GoMethod] = true
			}
		}
	}
	for method := range supported {
		if !found[method] {
			t.Errorf("test supports %s but spec has no state transition for it", method)
		}
	}
}

// TestSpecEventTypes verifies that all event types in the spec match Docker conventions.
func TestSpecEventTypes(t *testing.T) {
	specData, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}

	var doc specDoc
	if err := yaml.Unmarshal(specData, &doc); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}

	validTypes := map[string]bool{
		"container": true, "image": true, "network": true, "volume": true,
		"daemon": true, "plugin": true, "node": true, "service": true,
		"secret": true, "config": true,
	}

	for path, methods := range doc.Paths {
		for httpMethod, op := range methods {
			for _, ev := range op.Events {
				if !validTypes[ev.Type] {
					t.Errorf("%s %s: invalid event type %q", strings.ToUpper(httpMethod), path, ev.Type)
				}
				if ev.Action == "" {
					t.Errorf("%s %s: empty event action", strings.ToUpper(httpMethod), path)
				}
			}
		}
	}
}
