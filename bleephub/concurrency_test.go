package bleephub

import (
	"context"
	"testing"
)

func TestParseConcurrencyString(t *testing.T) {
	yaml := `
name: test
concurrency: deploy-group
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo build
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if wf.Concurrency == nil {
		t.Fatal("concurrency should be set")
	}
	if wf.Concurrency.Group != "deploy-group" {
		t.Errorf("group = %q, want deploy-group", wf.Concurrency.Group)
	}
	if wf.Concurrency.CancelInProgress {
		t.Error("cancel-in-progress should default to false")
	}
}

func TestParseConcurrencyObject(t *testing.T) {
	yaml := `
name: test
concurrency:
  group: deploy-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo build
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if wf.Concurrency == nil {
		t.Fatal("concurrency should be set")
	}
	if wf.Concurrency.Group != "deploy-${{ github.ref }}" {
		t.Errorf("group = %q", wf.Concurrency.Group)
	}
	if !wf.Concurrency.CancelInProgress {
		t.Error("cancel-in-progress should be true")
	}
}

func TestConcurrencyCancelInProgress(t *testing.T) {
	s := newTestServer()
	s.metrics = NewMetrics()

	// Submit first workflow with concurrency group
	wf1 := &WorkflowDef{
		Name:        "wf1",
		Concurrency: &ConcurrencyDef{Group: "deploy", CancelInProgress: true},
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "sleep 999"}}},
		},
	}
	workflow1, err := s.submitWorkflow(context.Background(), "http://localhost", wf1, "alpine:latest")
	if err != nil {
		t.Fatalf("submit wf1: %v", err)
	}
	if workflow1.Status != "running" {
		t.Fatalf("wf1 status = %q, want running", workflow1.Status)
	}

	// Submit second workflow in same group with cancel-in-progress
	wf2 := &WorkflowDef{
		Name:        "wf2",
		Concurrency: &ConcurrencyDef{Group: "deploy", CancelInProgress: true},
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "echo build"}}},
		},
	}
	workflow2, err := s.submitWorkflow(context.Background(), "http://localhost", wf2, "alpine:latest")
	if err != nil {
		t.Fatalf("submit wf2: %v", err)
	}

	// First workflow should be cancelled
	if workflow1.Status != "completed" || workflow1.Result != "cancelled" {
		t.Errorf("wf1 = %s/%s, want completed/cancelled", workflow1.Status, workflow1.Result)
	}

	// Second workflow should be running
	if workflow2.Status != "running" {
		t.Errorf("wf2 status = %q, want running", workflow2.Status)
	}
}

func TestConcurrencyGroupIsolation(t *testing.T) {
	s := newTestServer()
	s.metrics = NewMetrics()

	// Submit workflow in group A
	wf1 := &WorkflowDef{
		Name:        "wf-a",
		Concurrency: &ConcurrencyDef{Group: "group-a", CancelInProgress: true},
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "sleep 999"}}},
		},
	}
	workflow1, err := s.submitWorkflow(context.Background(), "http://localhost", wf1, "alpine:latest")
	if err != nil {
		t.Fatalf("submit wf-a: %v", err)
	}

	// Submit workflow in group B — should NOT affect group A
	wf2 := &WorkflowDef{
		Name:        "wf-b",
		Concurrency: &ConcurrencyDef{Group: "group-b", CancelInProgress: true},
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "echo build"}}},
		},
	}
	_, err = s.submitWorkflow(context.Background(), "http://localhost", wf2, "alpine:latest")
	if err != nil {
		t.Fatalf("submit wf-b: %v", err)
	}

	// First workflow should still be running
	if workflow1.Status != "running" {
		t.Errorf("wf-a status = %q, want running (different group)", workflow1.Status)
	}
}

func TestConcurrencyQueueWhenNotCancel(t *testing.T) {
	s := newTestServer()
	s.metrics = NewMetrics()

	// Submit first workflow
	wf1 := &WorkflowDef{
		Name:        "wf1",
		Concurrency: &ConcurrencyDef{Group: "serial", CancelInProgress: false},
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "sleep 999"}}},
		},
	}
	workflow1, err := s.submitWorkflow(context.Background(), "http://localhost", wf1, "alpine:latest")
	if err != nil {
		t.Fatalf("submit wf1: %v", err)
	}
	workflow1.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}

	// Submit second workflow in same group (no cancel-in-progress)
	wf2 := &WorkflowDef{
		Name:        "wf2",
		Concurrency: &ConcurrencyDef{Group: "serial", CancelInProgress: false},
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "echo build"}}},
		},
		Env: map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"},
	}
	workflow2, err := s.submitWorkflow(context.Background(), "http://localhost", wf2, "alpine:latest")
	if err != nil {
		t.Fatalf("submit wf2: %v", err)
	}

	// First workflow still running
	if workflow1.Status != "running" {
		t.Errorf("wf1 = %q, want running", workflow1.Status)
	}
	// Second workflow should be pending
	if workflow2.Status != "pending_concurrency" {
		t.Errorf("wf2 = %q, want pending_concurrency", workflow2.Status)
	}

	// Complete first workflow
	for _, j := range workflow1.Jobs {
		s.onJobCompleted(context.Background(),j.JobID, "Succeeded")
	}

	// Second workflow should now be running
	if workflow2.Status != "running" {
		t.Errorf("wf2 after wf1 done = %q, want running", workflow2.Status)
	}
}

func TestNoConcurrency(t *testing.T) {
	yaml := `
name: no-concurrency
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo build
`
	wf, err := ParseWorkflow([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if wf.Concurrency != nil {
		t.Error("concurrency should be nil when not specified")
	}
}
