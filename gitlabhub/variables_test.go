package gitlabhub

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildJobVariables(t *testing.T) {
	s := newTestServer(t)

	// Create project and pipeline
	project := s.store.CreateProject("myproject")
	_, _ = s.createProjectRepo("myproject", map[string]string{
		".gitlab-ci.yml": "test:\n  script: [echo hi]",
	})

	pipeline := &Pipeline{
		ID:        1,
		ProjectID: project.ID,
		Status:    "running",
		Ref:       "main",
		Sha:       "abc123def456",
		ServerURL: "localhost:8080",
	}

	job := &PipelineJob{
		ID:    1,
		Name:  "test",
		Stage: "test",
		Token: "job-token-123",
	}

	jobDef := &PipelineJobDef{
		Variables: map[string]string{
			"MY_VAR": "hello",
		},
	}

	vars := s.buildJobVariables(pipeline, job, jobDef)

	varMap := make(map[string]string)
	for _, v := range vars {
		varMap[v.Key] = v.Value
	}

	// Required CI vars
	checks := map[string]string{
		"CI":                "true",
		"GITLAB_CI":         "true",
		"CI_JOB_ID":         "1",
		"CI_JOB_NAME":       "test",
		"CI_JOB_STAGE":      "test",
		"CI_PIPELINE_ID":    "1",
		"CI_PROJECT_ID":     "1",
		"CI_PROJECT_NAME":   "myproject",
		"CI_COMMIT_SHA":     "abc123def456",
		"CI_COMMIT_BRANCH":  "main",
		"CI_BUILDS_DIR":     "/builds",
		"MY_VAR":            "hello",
	}

	for key, expected := range checks {
		got, ok := varMap[key]
		if !ok {
			t.Errorf("missing CI variable: %s", key)
			continue
		}
		if got != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, got)
		}
	}
}

func TestCIRepositoryURLFormat(t *testing.T) {
	s := newTestServer(t)

	project := s.store.CreateProject("test-repo")
	pipeline := &Pipeline{
		ID:        1,
		ProjectID: project.ID,
		ServerURL: "gitlab.example.com",
	}
	job := &PipelineJob{
		ID:    1,
		Token: "my-job-token",
	}

	vars := s.buildJobVariables(pipeline, job, nil)

	var repoURL string
	for _, v := range vars {
		if v.Key == "CI_REPOSITORY_URL" {
			repoURL = v.Value
			break
		}
	}

	if repoURL == "" {
		t.Fatal("CI_REPOSITORY_URL not found")
	}

	expected := "http://gitlab-ci-token:my-job-token@gitlab.example.com/test-repo.git"
	if repoURL != expected {
		t.Errorf("CI_REPOSITORY_URL: expected %q, got %q", expected, repoURL)
	}
}

func TestMaskedVariables(t *testing.T) {
	s := newTestServer(t)

	project := s.store.CreateProject("proj")

	// Add a masked variable
	s.store.mu.Lock()
	project.Variables["SECRET_KEY"] = &Variable{
		Key:    "SECRET_KEY",
		Value:  "super-secret",
		Masked: true,
	}
	s.store.mu.Unlock()

	pipeline := &Pipeline{
		ID:        1,
		ProjectID: project.ID,
		ServerURL: "localhost",
	}
	job := &PipelineJob{ID: 1, Token: "tok"}

	vars := s.buildJobVariables(pipeline, job, nil)

	var found bool
	for _, v := range vars {
		if v.Key == "SECRET_KEY" {
			found = true
			if v.Value != "super-secret" {
				t.Errorf("expected value super-secret, got %s", v.Value)
			}
			if !v.Masked {
				t.Error("expected masked=true")
			}
			if v.Public {
				t.Error("expected public=false for masked variable")
			}
		}
	}
	if !found {
		t.Error("SECRET_KEY not found in variables")
	}
}

func TestShortSha(t *testing.T) {
	if shortSha("abcdef1234567890") != "abcdef12" {
		t.Error("unexpected short sha")
	}
	if shortSha("abc") != "abc" {
		t.Error("short sha should handle short input")
	}
}

func TestUserVariablesOverride(t *testing.T) {
	s := newTestServer(t)

	yaml := `
variables:
  SHARED: global
test:
  variables:
    SHARED: job-level
  script: [echo]
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	pipeline := s.store.GetPipeline(pipelineID)
	var firstJob *PipelineJob
	for _, j := range pipeline.Jobs {
		firstJob = j
		break
	}
	jobDef := pipeline.Def.Jobs[firstJob.Name]
	vars := s.buildJobVariables(pipeline, firstJob, jobDef)

	// Find SHARED var
	var val string
	for _, v := range vars {
		if v.Key == "SHARED" {
			val = v.Value
		}
	}
	if val != "job-level" {
		t.Errorf("expected SHARED=job-level, got %s", val)
	}
}

func TestVariablesContainServerInfo(t *testing.T) {
	s := newTestServer(t)

	project := s.store.CreateProject("proj")
	pipeline := &Pipeline{
		ID:        1,
		ProjectID: project.ID,
		ServerURL: "myhost:9000",
	}
	job := &PipelineJob{ID: 1, Token: "tok"}

	vars := s.buildJobVariables(pipeline, job, nil)

	varMap := make(map[string]string)
	for _, v := range vars {
		varMap[v.Key] = v.Value
	}

	if !strings.Contains(varMap["CI_SERVER_URL"], "myhost:9000") {
		t.Errorf("CI_SERVER_URL should contain host: %s", varMap["CI_SERVER_URL"])
	}
	if varMap["CI_SERVER_HOST"] != "myhost:9000" {
		t.Errorf("CI_SERVER_HOST expected myhost:9000, got %s", varMap["CI_SERVER_HOST"])
	}
}
