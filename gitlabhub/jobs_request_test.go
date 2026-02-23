package gitlabhub

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestJobRequestNoJob204(t *testing.T) {
	s := newTestServer(t)

	// Register a runner
	regResp := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token: "reg-token", Description: "test",
	})
	var reg RunnerRegistrationResponse
	json.NewDecoder(regResp.Body).Decode(&reg)

	// Request a job with short timeout (should get 204)
	done := make(chan int, 1)
	go func() {
		rr := doRequest(s, "POST", "/api/v4/jobs/request", JobRequestBody{
			Token: reg.Token,
		})
		done <- rr.Code
	}()

	select {
	case code := <-done:
		if code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", code)
		}
	case <-time.After(35 * time.Second):
		t.Fatal("timeout waiting for job request response")
	}
}

func TestJobRequestForbiddenToken(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "POST", "/api/v4/jobs/request", JobRequestBody{
		Token: "bad-token",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestJobRequestReturnsPendingJob(t *testing.T) {
	s := newTestServer(t)

	// Register runner
	regResp := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token: "reg-token", Description: "test",
	})
	var reg RunnerRegistrationResponse
	json.NewDecoder(regResp.Body).Decode(&reg)

	// Submit a pipeline
	yaml := `
test:
  script:
    - echo hello
`
	submitResp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml,
		Image:    "alpine:latest",
	})
	if submitResp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", submitResp.Code, submitResp.Body.String())
	}

	// Request a job — should get the pending job
	rr := doRequest(s, "POST", "/api/v4/jobs/request", JobRequestBody{
		Token: reg.Token,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var job JobResponse
	json.NewDecoder(rr.Body).Decode(&job)

	if job.ID == 0 {
		t.Fatal("expected non-zero job ID")
	}
	if job.Token == "" {
		t.Fatal("expected non-empty job token")
	}
	if job.Image.Name != "alpine:latest" {
		t.Fatalf("expected image=alpine:latest, got %s", job.Image.Name)
	}
	if len(job.Steps) == 0 {
		t.Fatal("expected at least one step")
	}
}

func TestJobResponseVariables(t *testing.T) {
	s := newTestServer(t)

	// Register runner
	doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token: "reg-token", Description: "test",
	})

	// Submit a pipeline with variables
	yaml := `
variables:
  MY_VAR: hello

test:
  script:
    - echo $MY_VAR
`
	submitResp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml,
		Image:    "alpine:latest",
	})
	if submitResp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", submitResp.Code, submitResp.Body.String())
	}

	var submitResult map[string]interface{}
	json.NewDecoder(submitResp.Body).Decode(&submitResult)
	pipelineID := int(submitResult["pipelineId"].(float64))

	// Get the job
	pipeline := s.store.GetPipeline(pipelineID)
	if pipeline == nil {
		t.Fatal("pipeline not found")
	}

	// Find first job
	var firstJob *PipelineJob
	for _, j := range pipeline.Jobs {
		firstJob = j
		break
	}
	if firstJob == nil {
		t.Fatal("no jobs in pipeline")
	}

	// Build variables
	jobDef := pipeline.Def.Jobs[firstJob.Name]
	vars := s.buildJobVariables(pipeline, firstJob, jobDef)

	// Check for CI variables
	varMap := make(map[string]string)
	for _, v := range vars {
		varMap[v.Key] = v.Value
	}

	requiredVars := []string{"CI", "GITLAB_CI", "CI_JOB_ID", "CI_JOB_NAME", "CI_PIPELINE_ID", "CI_REPOSITORY_URL"}
	for _, key := range requiredVars {
		if _, ok := varMap[key]; !ok {
			t.Errorf("missing required CI variable: %s", key)
		}
	}

	if varMap["CI"] != "true" {
		t.Errorf("expected CI=true, got %s", varMap["CI"])
	}
	if varMap["MY_VAR"] != "hello" {
		t.Errorf("expected MY_VAR=hello, got %s", varMap["MY_VAR"])
	}
}
