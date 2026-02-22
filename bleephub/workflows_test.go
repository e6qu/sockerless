package bleephub

import (
	"encoding/json"
	"testing"
)

func TestWorkflowSingleJobSubmit(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "test",
		Jobs: map[string]*JobDef{
			"build": {
				Steps: []StepDef{{Run: "echo hello"}},
			},
		},
	}

	workflow, err := s.submitWorkflow("http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if workflow.Status != "running" {
		t.Errorf("status = %q, want running", workflow.Status)
	}
	if len(workflow.Jobs) != 1 {
		t.Errorf("jobs = %d, want 1", len(workflow.Jobs))
	}
	job := workflow.Jobs["build"]
	if job.Status != "queued" {
		t.Errorf("job status = %q, want queued", job.Status)
	}
}

func TestWorkflowTwoJobsWithNeeds(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "test",
		Jobs: map[string]*JobDef{
			"build": {
				Steps: []StepDef{{Run: "make build"}},
			},
			"test": {
				Needs: []string{"build"},
				Steps: []StepDef{{Run: "make test"}},
			},
		},
	}

	workflow, err := s.submitWorkflow("http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// build should be dispatched, test should be pending
	buildJob := workflow.Jobs["build"]
	testJob := workflow.Jobs["test"]

	if buildJob.Status != "queued" {
		t.Errorf("build status = %q, want queued", buildJob.Status)
	}
	if testJob.Status != "pending" {
		t.Errorf("test status = %q, want pending", testJob.Status)
	}

	// Simulate build completion — store serverURL in env for re-dispatch
	workflow.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}
	s.onJobCompleted(buildJob.JobID, "Succeeded")

	if testJob.Status != "queued" {
		t.Errorf("test status after build = %q, want queued", testJob.Status)
	}
}

func TestWorkflowDiamondDependency(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "diamond",
		Jobs: map[string]*JobDef{
			"a": {Steps: []StepDef{{Run: "echo a"}}},
			"b": {Needs: []string{"a"}, Steps: []StepDef{{Run: "echo b"}}},
			"c": {Needs: []string{"a"}, Steps: []StepDef{{Run: "echo c"}}},
			"d": {Needs: []string{"b", "c"}, Steps: []StepDef{{Run: "echo d"}}},
		},
	}

	workflow, err := s.submitWorkflow("http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	workflow.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}

	// Only A should be dispatched
	if workflow.Jobs["a"].Status != "queued" {
		t.Errorf("a = %q, want queued", workflow.Jobs["a"].Status)
	}
	if workflow.Jobs["b"].Status != "pending" {
		t.Errorf("b = %q, want pending", workflow.Jobs["b"].Status)
	}
	if workflow.Jobs["d"].Status != "pending" {
		t.Errorf("d = %q, want pending", workflow.Jobs["d"].Status)
	}

	// Complete A → B and C should dispatch
	s.onJobCompleted(workflow.Jobs["a"].JobID, "Succeeded")
	if workflow.Jobs["b"].Status != "queued" {
		t.Errorf("b after a = %q, want queued", workflow.Jobs["b"].Status)
	}
	if workflow.Jobs["c"].Status != "queued" {
		t.Errorf("c after a = %q, want queued", workflow.Jobs["c"].Status)
	}
	if workflow.Jobs["d"].Status != "pending" {
		t.Errorf("d after a = %q, want pending", workflow.Jobs["d"].Status)
	}

	// Complete B → D still pending (C not done)
	s.onJobCompleted(workflow.Jobs["b"].JobID, "Succeeded")
	if workflow.Jobs["d"].Status != "pending" {
		t.Errorf("d after b = %q, want pending", workflow.Jobs["d"].Status)
	}

	// Complete C → D dispatches, workflow complete
	s.onJobCompleted(workflow.Jobs["c"].JobID, "Succeeded")
	if workflow.Jobs["d"].Status != "queued" {
		t.Errorf("d after c = %q, want queued", workflow.Jobs["d"].Status)
	}
}

func TestWorkflowFailedJobSkipsDependents(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "fail-test",
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "exit 1"}}},
			"test":  {Needs: []string{"build"}, Steps: []StepDef{{Run: "echo test"}}},
		},
	}

	workflow, err := s.submitWorkflow("http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	workflow.Env = map[string]string{"__serverURL": "http://localhost", "__defaultImage": "alpine:latest"}

	// Build fails
	s.onJobCompleted(workflow.Jobs["build"].JobID, "Failed")

	if workflow.Jobs["test"].Status != "skipped" {
		t.Errorf("test status = %q, want skipped", workflow.Jobs["test"].Status)
	}
	if workflow.Status != "completed" {
		t.Errorf("workflow status = %q, want completed", workflow.Status)
	}
	if workflow.Result != "failure" {
		t.Errorf("workflow result = %q, want failure", workflow.Result)
	}
}

func TestWorkflowNeedsContextPropagation(t *testing.T) {
	wf := &Workflow{
		ID:   "test-wf",
		Name: "test",
		Jobs: map[string]*WorkflowJob{
			"build": {
				Key:    "build",
				JobID:  "j1",
				Status: "completed",
				Result: "success",
				Outputs: map[string]string{
					"version": "1.0.0",
				},
			},
			"deploy": {
				Key:   "deploy",
				JobID: "j2",
				Needs: []string{"build"},
			},
		},
	}

	ctx := buildNeedsContext(wf, wf.Jobs["deploy"])
	dict, ok := ctx.(map[string]interface{})
	if !ok {
		t.Fatalf("needs context is not a dict: %T", ctx)
	}
	if dict["t"] != 2 {
		t.Errorf("needs context type = %v, want 2", dict["t"])
	}
	entries, ok := dict["d"].([]map[string]interface{})
	if !ok || len(entries) != 1 {
		t.Fatalf("needs context entries = %v", dict["d"])
	}
	if entries[0]["k"] != "build" {
		t.Errorf("needs[0].k = %v, want build", entries[0]["k"])
	}
}

func TestWorkflowUsesStepReference(t *testing.T) {
	s := newTestServer()
	wf := &WorkflowDef{
		Name: "uses-test",
		Jobs: map[string]*JobDef{
			"build": {
				Steps: []StepDef{
					{Uses: "actions/checkout@v4"},
					{Run: "echo done"},
				},
			},
		},
	}

	workflow, err := s.submitWorkflow("http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Verify the job message contains a repository reference
	s.store.mu.RLock()
	job := s.store.Jobs[workflow.Jobs["build"].JobID]
	s.store.mu.RUnlock()

	if job == nil {
		t.Fatal("job not found in store")
	}
	if job.Message == "" {
		t.Fatal("job message is empty")
	}

	// Parse the message and check the first step's reference
	var msg map[string]interface{}
	if err := jsonUnmarshal([]byte(job.Message), &msg); err != nil {
		t.Fatalf("parse message: %v", err)
	}

	steps, ok := msg["steps"].([]interface{})
	if !ok || len(steps) < 1 {
		t.Fatal("no steps in message")
	}

	step0 := steps[0].(map[string]interface{})
	ref := step0["reference"].(map[string]interface{})
	if ref["type"] != "repository" {
		t.Errorf("ref type = %v, want repository", ref["type"])
	}
	if ref["name"] != "actions/checkout" {
		t.Errorf("ref name = %v, want actions/checkout", ref["name"])
	}
	if ref["ref"] != "v4" {
		t.Errorf("ref ref = %v, want v4", ref["ref"])
	}
}

func TestValidateJobGraphCycle(t *testing.T) {
	wf := &WorkflowDef{
		Jobs: map[string]*JobDef{
			"a": {Needs: []string{"b"}},
			"b": {Needs: []string{"a"}},
		},
	}
	err := validateJobGraph(wf)
	if err == nil {
		t.Error("expected cycle error")
	}
}

func TestValidateJobGraphUnknownDep(t *testing.T) {
	wf := &WorkflowDef{
		Jobs: map[string]*JobDef{
			"a": {Needs: []string{"nonexistent"}},
		},
	}
	err := validateJobGraph(wf)
	if err == nil {
		t.Error("expected unknown dependency error")
	}
}

func TestNormalizeResult(t *testing.T) {
	tests := map[string]string{
		"Succeeded": "success",
		"succeeded": "success",
		"Failed":    "failure",
		"failed":    "failure",
		"Cancelled": "cancelled",
		"":          "success",
		"custom":    "custom",
	}
	for input, expected := range tests {
		if got := normalizeResult(input); got != expected {
			t.Errorf("normalizeResult(%q) = %q, want %q", input, got, expected)
		}
	}
}

// jsonUnmarshal is a test helper.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
