package gitlabhub

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArtifactUploadDownload(t *testing.T) {
	s := newTestServer(t)

	// Create a job manually
	s.store.mu.Lock()
	jobID := s.store.NextJob
	s.store.NextJob++
	s.store.Jobs[jobID] = &PipelineJob{
		ID:     jobID,
		Name:   "build",
		Stage:  "build",
		Status: "running",
		Token:  "test-token",
	}
	s.store.mu.Unlock()

	// Upload artifact as multipart
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "artifacts.zip")
	if err != nil {
		t.Fatal(err)
	}
	testData := []byte("PK\x03\x04fake-zip-data-here")
	part.Write(testData)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v4/jobs/1/artifacts", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Download
	rr2 := doRequest(s, "GET", "/api/v4/jobs/1/artifacts", nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d", rr2.Code)
	}

	downloaded, _ := io.ReadAll(rr2.Body)
	if !bytes.Equal(downloaded, testData) {
		t.Fatalf("downloaded data mismatch: got %d bytes", len(downloaded))
	}
}

func TestArtifactNotFound(t *testing.T) {
	s := newTestServer(t)

	// Create job without artifacts
	s.store.mu.Lock()
	s.store.Jobs[1] = &PipelineJob{ID: 1, Status: "running"}
	s.store.NextJob = 2
	s.store.mu.Unlock()

	rr := doRequest(s, "GET", "/api/v4/jobs/1/artifacts", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestCachePutGet(t *testing.T) {
	s := newTestServer(t)

	// PUT cache
	data := []byte("cached-data-12345")
	req := httptest.NewRequest("PUT", "/cache/mykey", bytes.NewReader(data))
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("cache put: expected 200, got %d", rr.Code)
	}

	// GET cache
	rr2 := doRequest(s, "GET", "/cache/mykey", nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("cache get: expected 200, got %d", rr2.Code)
	}
	got, _ := io.ReadAll(rr2.Body)
	if !bytes.Equal(got, data) {
		t.Fatalf("cache data mismatch")
	}
}

func TestCacheMiss(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "GET", "/cache/nonexistent", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestCacheHead(t *testing.T) {
	s := newTestServer(t)

	// HEAD without data
	rr := doRequest(s, "HEAD", "/cache/mykey", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}

	// Store data
	s.store.SetCache("mykey", []byte("data"))

	// HEAD with data
	req := httptest.NewRequest("HEAD", "/cache/mykey", nil)
	rr2 := httptest.NewRecorder()
	s.mux.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}
}

func TestArtifactDependencies(t *testing.T) {
	s := newTestServer(t)

	// Submit pipeline with artifacts
	yaml := `
stages:
  - build
  - test

build:
  stage: build
  script:
    - echo build
  artifacts:
    paths:
      - output/

test:
  stage: test
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

	// Complete build and store artifact
	jobID := s.store.DequeueJob()
	s.store.StoreArtifact(jobID, []byte("fake-zip"))
	job := s.store.GetJob(jobID)
	s.store.mu.Lock()
	job.Status = "success"
	job.Result = "success"
	s.store.mu.Unlock()
	s.onJobCompleted(context.Background(),jobID, "success")

	// Now test job should be pending
	testJobID := s.store.DequeueJob()
	if testJobID == 0 {
		t.Fatal("expected test job to be enqueued")
	}

	pipeline := s.store.GetPipeline(pipelineID)
	testJob := s.store.GetJob(testJobID)
	testJobDef := pipeline.Def.Jobs[testJob.Name]

	resp2 := s.buildJobResponse(pipeline, testJob, testJobDef)
	if len(resp2.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(resp2.Dependencies))
	}
	if resp2.Dependencies[0].Name != "build" {
		t.Fatalf("expected dependency name=build, got %s", resp2.Dependencies[0].Name)
	}
}
