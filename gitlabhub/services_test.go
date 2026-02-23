package gitlabhub

import (
	"encoding/json"
	"testing"
)

func TestBuildServiceDefs(t *testing.T) {
	services := []ServiceEntry{
		{Name: "postgres:15-alpine"},
		{Name: "redis:7", Alias: "cache"},
		{Name: "registry.example.com/my-service:latest"},
	}

	defs := buildServiceDefs(services)

	if len(defs) != 3 {
		t.Fatalf("expected 3 services, got %d", len(defs))
	}

	// postgres alias should be "postgres"
	if defs[0].Alias != "postgres" {
		t.Errorf("expected alias=postgres, got %s", defs[0].Alias)
	}

	// explicit alias preserved
	if defs[1].Alias != "cache" {
		t.Errorf("expected alias=cache, got %s", defs[1].Alias)
	}

	// registry image alias is last path component
	if defs[2].Alias != "my-service" {
		t.Errorf("expected alias=my-service, got %s", defs[2].Alias)
	}
}

func TestServiceAlias(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"postgres:15", "postgres"},
		{"redis", "redis"},
		{"registry.example.com/my-img:v1", "my-img"},
		{"docker.io/library/mysql:8", "mysql"},
	}

	for _, tc := range tests {
		got := serviceAlias(tc.image)
		if got != tc.expected {
			t.Errorf("serviceAlias(%q): expected %q, got %q", tc.image, tc.expected, got)
		}
	}
}

func TestServiceDefsInJobResponse(t *testing.T) {
	s := newTestServer(t)

	yaml := `
test:
  services:
    - postgres:15
    - name: redis:7
      alias: cache
  script:
    - echo testing
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	if resp.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	// Get the enqueued job
	jobID := s.store.DequeueJob()
	job := s.store.GetJob(jobID)
	pipeline := s.store.GetPipeline(pipelineID)
	jobDef := pipeline.Def.Jobs[job.Name]

	jobResp := s.buildJobResponse(pipeline, job, jobDef)

	if len(jobResp.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(jobResp.Services))
	}
	if jobResp.Services[0].Name != "postgres:15" {
		t.Errorf("expected postgres:15, got %s", jobResp.Services[0].Name)
	}
	if jobResp.Services[1].Alias != "cache" {
		t.Errorf("expected alias=cache, got %s", jobResp.Services[1].Alias)
	}
}
