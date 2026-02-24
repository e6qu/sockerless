package gitlabhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestSingleJobPipeline(t *testing.T) {
	s := newTestServer(t)

	yaml := `
test:
  script:
    - echo hello
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	// Job should be enqueued
	s.store.mu.RLock()
	pendingCount := len(s.store.PendingJobs)
	s.store.mu.RUnlock()

	if pendingCount != 1 {
		t.Fatalf("expected 1 pending job, got %d", pendingCount)
	}

	// Simulate completion
	jobID := s.store.DequeueJob()
	job := s.store.GetJob(jobID)
	s.store.mu.Lock()
	job.Status = "success"
	job.Result = "success"
	s.store.mu.Unlock()
	s.onJobCompleted(context.Background(),jobID, "success")

	pipeline := s.store.GetPipeline(pipelineID)
	if pipeline.Status != "success" {
		t.Fatalf("expected pipeline status=success, got %s", pipeline.Status)
	}
}

func TestMultiStagePipeline(t *testing.T) {
	s := newTestServer(t)

	yaml := `
stages:
  - build
  - test
  - deploy

build:
  stage: build
  script:
    - echo build

test:
  stage: test
  script:
    - echo test

deploy:
  stage: deploy
  script:
    - echo deploy
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	// Only first stage should be pending
	s.store.mu.RLock()
	pendingCount := len(s.store.PendingJobs)
	s.store.mu.RUnlock()
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending job (build stage), got %d", pendingCount)
	}

	// Complete build stage
	jobID := s.store.DequeueJob()
	job := s.store.GetJob(jobID)
	s.store.mu.Lock()
	job.Status = "success"
	job.Result = "success"
	s.store.mu.Unlock()
	s.onJobCompleted(context.Background(),jobID, "success")

	// Test stage should now be pending
	s.store.mu.RLock()
	pendingCount = len(s.store.PendingJobs)
	s.store.mu.RUnlock()
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending job (test stage), got %d", pendingCount)
	}

	// Complete test stage
	jobID = s.store.DequeueJob()
	job = s.store.GetJob(jobID)
	s.store.mu.Lock()
	job.Status = "success"
	job.Result = "success"
	s.store.mu.Unlock()
	s.onJobCompleted(context.Background(),jobID, "success")

	// Deploy stage should now be pending
	s.store.mu.RLock()
	pendingCount = len(s.store.PendingJobs)
	s.store.mu.RUnlock()
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending job (deploy stage), got %d", pendingCount)
	}

	// Complete deploy stage
	jobID = s.store.DequeueJob()
	job = s.store.GetJob(jobID)
	s.store.mu.Lock()
	job.Status = "success"
	job.Result = "success"
	s.store.mu.Unlock()
	s.onJobCompleted(context.Background(),jobID, "success")

	pipeline := s.store.GetPipeline(pipelineID)
	if pipeline.Status != "success" {
		t.Fatalf("expected pipeline status=success, got %s", pipeline.Status)
	}
}

func TestDAGNeedsOverride(t *testing.T) {
	s := newTestServer(t)

	yaml := `
stages:
  - build
  - test

build_a:
  stage: build
  script:
    - echo a

build_b:
  stage: build
  script:
    - echo b

test:
  stage: test
  needs: [build_a]
  script:
    - echo test
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	// Both build jobs should be pending
	s.store.mu.RLock()
	pendingCount := len(s.store.PendingJobs)
	s.store.mu.RUnlock()
	if pendingCount != 2 {
		t.Fatalf("expected 2 pending jobs (both builds), got %d", pendingCount)
	}

	// Complete build_a only
	pipeline := s.store.GetPipeline(pipelineID)
	var buildA *PipelineJob
	for _, j := range pipeline.Jobs {
		if j.Name == "build_a" {
			buildA = j
			break
		}
	}
	if buildA == nil {
		t.Fatal("build_a not found")
	}

	// Dequeue build_a and build_b
	for i := 0; i < 2; i++ {
		jobID := s.store.DequeueJob()
		job := s.store.GetJob(jobID)
		s.store.mu.Lock()
		job.Status = "success"
		job.Result = "success"
		s.store.mu.Unlock()
		if job.Name == "build_a" {
			s.onJobCompleted(context.Background(),jobID, "success")
		}
	}

	// Test job should be pending now (DAG: needs only build_a, not all of build stage)
	s.store.mu.RLock()
	pendingCount = len(s.store.PendingJobs)
	s.store.mu.RUnlock()
	if pendingCount != 1 {
		t.Fatalf("expected 1 pending job (test via DAG), got %d", pendingCount)
	}
}

func TestFailedJobStopsLaterStages(t *testing.T) {
	s := newTestServer(t)

	yaml := `
stages:
  - build
  - test

build:
  stage: build
  script:
    - echo build

test:
  stage: test
  script:
    - echo test
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	// Complete build with failure
	jobID := s.store.DequeueJob()
	job := s.store.GetJob(jobID)
	s.store.mu.Lock()
	job.Status = "failed"
	job.Result = "failed"
	s.store.mu.Unlock()
	s.onJobCompleted(context.Background(),jobID, "failed")

	pipeline := s.store.GetPipeline(pipelineID)

	// Test job should be skipped
	testJob, ok := pipeline.Jobs["test"]
	if !ok {
		t.Fatal("test job not found")
	}
	if testJob.Status != "skipped" {
		t.Fatalf("expected test status=skipped, got %s", testJob.Status)
	}

	// Pipeline should be failed
	if pipeline.Status != "failed" {
		t.Fatalf("expected pipeline status=failed, got %s", pipeline.Status)
	}
}

func TestPipelineStatusAPI(t *testing.T) {
	s := newTestServer(t)

	yaml := `
test:
  script:
    - echo hello
`
	submitResp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	var submitResult map[string]interface{}
	json.NewDecoder(submitResp.Body).Decode(&submitResult)
	pipelineID := int(submitResult["pipelineId"].(float64))

	// Get status
	statusResp := doRequest(s, "GET", fmt.Sprintf("/api/v3/gitlabhub/pipelines/%d", pipelineID), nil)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", statusResp.Code)
	}

	var status PipelineStatusResponse
	json.NewDecoder(statusResp.Body).Decode(&status)
	if status.Status != "running" {
		t.Fatalf("expected status=running, got %s", status.Status)
	}
	if len(status.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(status.Jobs))
	}
}

func TestPipelineNotFound(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "GET", "/api/v3/gitlabhub/pipelines/99999", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestAllStagesCompleteSuccess(t *testing.T) {
	s := newTestServer(t)

	yaml := `
stages:
  - build
  - test

build:
  stage: build
  script: [echo build]

test:
  stage: test
  script: [echo test]
`
	resp := doRequest(s, "POST", "/api/v3/gitlabhub/pipeline", PipelineSubmitRequest{
		Pipeline: yaml, Image: "alpine:latest",
	})
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	pipelineID := int(result["pipelineId"].(float64))

	// Complete both jobs sequentially
	for i := 0; i < 2; i++ {
		// Wait for job to be enqueued
		var jobID int
		for attempt := 0; attempt < 10; attempt++ {
			jobID = s.store.DequeueJob()
			if jobID != 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if jobID == 0 {
			t.Fatalf("no job available on iteration %d", i)
		}
		job := s.store.GetJob(jobID)
		s.store.mu.Lock()
		job.Status = "success"
		job.Result = "success"
		s.store.mu.Unlock()
		s.onJobCompleted(context.Background(),jobID, "success")
	}

	pipeline := s.store.GetPipeline(pipelineID)
	if pipeline.Status != "success" {
		t.Fatalf("expected pipeline status=success, got %s", pipeline.Status)
	}
	if pipeline.Result != "success" {
		t.Fatalf("expected result=success, got %s", pipeline.Result)
	}
}

