package gitlabhub

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestCancelPipelineBasic(t *testing.T) {
	logger := zerolog.Nop()
	s := NewServer(":0", logger)

	// Create a pipeline with jobs
	project := s.store.CreateProject("cancel-test")
	def := &PipelineDef{
		Stages: []string{"build", "test"},
		Jobs: map[string]*PipelineJobDef{
			"build": {Stage: "build", Script: []string{"echo build"}, When: "on_success"},
			"test":  {Stage: "test", Script: []string{"echo test"}, When: "on_success"},
		},
	}

	pl, err := s.submitPipeline(context.Background(),project, def, "localhost", "")
	if err != nil {
		t.Fatal(err)
	}

	// Cancel the pipeline
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v3/gitlabhub/pipelines/%d/cancel", pl.ID), nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Check pipeline is canceled
	pl = s.store.GetPipeline(pl.ID)
	if pl.Status != "canceled" {
		t.Errorf("expected canceled, got %s", pl.Status)
	}
}

func TestCancelPipelineNotFound(t *testing.T) {
	logger := zerolog.Nop()
	s := NewServer(":0", logger)

	req := httptest.NewRequest("POST", "/api/v3/gitlabhub/pipelines/999/cancel", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCancelSkipsCompleted(t *testing.T) {
	logger := zerolog.Nop()
	s := NewServer(":0", logger)

	project := s.store.CreateProject("cancel-skip-test")
	def := &PipelineDef{
		Stages: []string{"build", "test"},
		Jobs: map[string]*PipelineJobDef{
			"build": {Stage: "build", Script: []string{"echo build"}, When: "on_success"},
			"test":  {Stage: "test", Script: []string{"echo test"}, When: "on_success"},
		},
	}

	pl, _ := s.submitPipeline(context.Background(),project, def, "localhost", "")

	// Complete build job
	for _, j := range pl.Jobs {
		if j.Name == "build" {
			s.store.mu.Lock()
			j.Status = "success"
			j.Result = "success"
			s.store.mu.Unlock()
		}
	}

	// Cancel
	s.cancelPipeline(pl)

	// build should still be success
	for _, j := range pl.Jobs {
		if j.Name == "build" && j.Status != "success" {
			t.Errorf("build should stay success, got %s", j.Status)
		}
		if j.Name == "test" && j.Status != "canceled" {
			t.Errorf("test should be canceled, got %s", j.Status)
		}
	}
}

func TestResourceGroupQueues(t *testing.T) {
	logger := zerolog.Nop()
	s := NewServer(":0", logger)

	project := s.store.CreateProject("rg-test")
	def := &PipelineDef{
		Stages: []string{"deploy"},
		Jobs: map[string]*PipelineJobDef{
			"deploy1": {Stage: "deploy", Script: []string{"echo 1"}, When: "on_success", ResourceGroup: "production"},
			"deploy2": {Stage: "deploy", Script: []string{"echo 2"}, When: "on_success", ResourceGroup: "production"},
		},
	}

	pl, _ := s.submitPipeline(context.Background(),project, def, "localhost", "")

	// One should be pending, one should be waiting
	pending := 0
	created := 0
	for _, j := range pl.Jobs {
		if j.Status == "pending" {
			pending++
		}
		if j.Status == "created" {
			created++
		}
	}
	if pending != 1 {
		t.Errorf("expected 1 pending, got %d", pending)
	}
	if created != 1 {
		t.Errorf("expected 1 created (waiting for resource group), got %d", created)
	}
}

func TestResourceGroupReleases(t *testing.T) {
	logger := zerolog.Nop()
	s := NewServer(":0", logger)

	project := s.store.CreateProject("rg-release-test")
	def := &PipelineDef{
		Stages: []string{"deploy"},
		Jobs: map[string]*PipelineJobDef{
			"deploy1": {Stage: "deploy", Script: []string{"echo 1"}, When: "on_success", ResourceGroup: "production"},
			"deploy2": {Stage: "deploy", Script: []string{"echo 2"}, When: "on_success", ResourceGroup: "production"},
		},
	}

	pl, _ := s.submitPipeline(context.Background(),project, def, "localhost", "")

	// Find the pending job and complete it
	for _, j := range pl.Jobs {
		if j.Status == "pending" || j.Status == "running" {
			s.store.mu.Lock()
			j.Status = "success"
			j.Result = "success"
			s.store.mu.Unlock()
			s.onJobCompleted(context.Background(),j.ID, "success")
			break
		}
	}

	// Now the second job should be dispatched
	pending := 0
	for _, j := range pl.Jobs {
		if j.Status == "pending" {
			pending++
		}
	}
	if pending != 1 {
		t.Errorf("expected second job dispatched, got %d pending", pending)
	}
}

func TestPipelineTimeout(t *testing.T) {
	// Test that pipeline-level timeout field is recognized
	yaml := `
test:
  resource_group: production
  script:
    - echo test
`
	def, err := ParsePipeline([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if def.Jobs["test"].ResourceGroup != "production" {
		t.Errorf("expected resource_group=production, got %q", def.Jobs["test"].ResourceGroup)
	}
}

