package bleephub

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestListWorkflowsEmpty(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/internal/workflows")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var workflows []workflowView
	if err := json.NewDecoder(resp.Body).Decode(&workflows); err != nil {
		t.Fatal(err)
	}
	// May not be empty if other tests have run — just verify shape
	_ = workflows
}

func TestListWorkflowsWithData(t *testing.T) {
	// Seed a workflow
	testServer.store.mu.Lock()
	testServer.store.Workflows["test-wf-1"] = &Workflow{
		ID:        "test-wf-1",
		Name:      "CI Pipeline",
		RunID:     42,
		Status:    "completed",
		Result:    "success",
		CreatedAt: time.Now(),
		EventName: "push",
		Jobs: map[string]*WorkflowJob{
			"build": {Key: "build", JobID: "j1", DisplayName: "Build", Status: "completed", Result: "success"},
		},
	}
	testServer.store.mu.Unlock()

	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Workflows, "test-wf-1")
		testServer.store.mu.Unlock()
	}()

	resp, err := http.Get(testBaseURL + "/internal/workflows")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var workflows []workflowView
	if err := json.NewDecoder(resp.Body).Decode(&workflows); err != nil {
		t.Fatal(err)
	}
	if len(workflows) < 1 {
		t.Fatal("expected at least 1 workflow")
	}

	found := false
	for _, wf := range workflows {
		if wf.ID == "test-wf-1" {
			found = true
			if wf.Name != "CI Pipeline" {
				t.Errorf("expected name 'CI Pipeline', got %q", wf.Name)
			}
			if wf.Status != "completed" {
				t.Errorf("expected status 'completed', got %q", wf.Status)
			}
			if len(wf.Jobs) != 1 {
				t.Errorf("expected 1 job, got %d", len(wf.Jobs))
			}
		}
	}
	if !found {
		t.Error("test-wf-1 not found in response")
	}
}

func TestListSessions(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/internal/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sessions []sessionView
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	_ = sessions
}

func TestListRepos(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/internal/repos")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var repos []repoView
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatal(err)
	}
	_ = repos
}

func TestGetWorkflowNotFound(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/internal/workflows/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetWorkflowLogs(t *testing.T) {
	// Seed a workflow with log lines
	testServer.store.mu.Lock()
	testServer.store.Workflows["test-wf-logs"] = &Workflow{
		ID:        "test-wf-logs",
		Name:      "Log Test",
		RunID:     99,
		Status:    "completed",
		Result:    "success",
		CreatedAt: time.Now(),
		Jobs: map[string]*WorkflowJob{
			"test": {Key: "test", JobID: "j-log-1", DisplayName: "Test", Status: "completed", Result: "success"},
		},
	}
	testServer.store.LogLines["j-log-1"] = []string{"line 1", "line 2"}
	testServer.store.mu.Unlock()

	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Workflows, "test-wf-logs")
		delete(testServer.store.LogLines, "j-log-1")
		testServer.store.mu.Unlock()
	}()

	resp, err := http.Get(testBaseURL + "/internal/workflows/test-wf-logs/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var logs map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		t.Fatal(err)
	}
	if lines, ok := logs["j-log-1"]; !ok || len(lines) != 2 {
		t.Errorf("expected 2 log lines for j-log-1, got %v", logs)
	}
}
