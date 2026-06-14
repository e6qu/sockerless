package bleeplab

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := NewServer(":0", zerolog.Nop())
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, ts *httptest.Server, method, path string, body any, hdr map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

// TestFullPipelineLifecycle drives the control-plane + runner API exactly as
// the orchestrator + a real gitlab-runner would: create project, commit a
// .gitlab-ci.yml, create a runner, trigger a pipeline, then act as the runner
// (claim job, stream trace, report success) and assert the pipeline succeeds.
func TestFullPipelineLifecycle(t *testing.T) {
	ts := newTestServer(t)

	// 1. Project.
	resp, body := do(t, ts, "POST", "/api/v4/projects", map[string]any{"name": "demo"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: status %d body %s", resp.StatusCode, body)
	}
	var proj struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(body, &proj)

	// 2. Commit a .gitlab-ci.yml.
	ci := strings.Join([]string{
		"stages: [build, test]",
		"build-job:",
		"  stage: build",
		"  image: alpine:3.20",
		"  script:",
		"    - echo building",
		"test-job:",
		"  stage: test",
		"  image: alpine:3.20",
		"  script:",
		"    - echo testing",
	}, "\n")
	resp, body = do(t, ts, "POST", "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/repository/commits", map[string]any{
		"branch": "main",
		"actions": []map[string]string{
			{"file_path": ".gitlab-ci.yml", "content": ci},
		},
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("commit: status %d body %s", resp.StatusCode, body)
	}

	// 3. Runner.
	resp, body = do(t, ts, "POST", "/api/v4/user/runners", map[string]any{"runner_type": "project_type"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create runner: status %d body %s", resp.StatusCode, body)
	}
	var runner struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(body, &runner)

	// 4. Trigger pipeline.
	resp, body = do(t, ts, "POST", "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/pipeline", map[string]any{"ref": "main"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create pipeline: status %d body %s", resp.StatusCode, body)
	}
	var pl struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(body, &pl)

	// 5. Act as the runner: claim + run both jobs (build then test). Stage
	// gating means the test job only appears after build succeeds.
	ranStages := claimAndRun(t, ts, runner.Token)
	if ranStages != 2 {
		t.Fatalf("expected to run 2 jobs across stages, ran %d", ranStages)
	}

	// 6. Pipeline must be success.
	resp, body = do(t, ts, "GET", "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/pipelines/"+strconv.Itoa(pl.ID), nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get pipeline: status %d", resp.StatusCode)
	}
	var got struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(body, &got)
	if got.Status != "success" {
		t.Fatalf("pipeline status = %q, want success (trace/jobs: %s)", got.Status, body)
	}
}

// claimAndRun polls /jobs/request like a runner until it gets 204, running
// each claimed job to success (trace + PUT state). Returns the job count.
// Stage gating means the second stage only appears after the first succeeds —
// proving the stage engine enqueues incrementally.
func claimAndRun(t *testing.T, ts *httptest.Server, runnerToken string) int {
	t.Helper()
	ran := 0
	for i := 0; i < 10; i++ {
		resp, body := do(t, ts, "POST", "/api/v4/jobs/request", map[string]any{"token": runnerToken}, nil)
		if resp.StatusCode == http.StatusNoContent {
			break
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("jobs/request: status %d body %s", resp.StatusCode, body)
		}
		var jr struct {
			ID    int    `json:"id"`
			Token string `json:"token"`
			Steps []struct {
				Script []string `json:"script"`
			} `json:"steps"`
			Image *struct {
				Name string `json:"name"`
			} `json:"image"`
		}
		if err := json.Unmarshal(body, &jr); err != nil {
			t.Fatalf("decode job: %v body %s", err, body)
		}
		if jr.Image == nil || jr.Image.Name != "alpine:3.20" {
			t.Fatalf("job %d image = %+v, want alpine:3.20", jr.ID, jr.Image)
		}
		if len(jr.Steps) == 0 || len(jr.Steps[0].Script) == 0 {
			t.Fatalf("job %d has no script steps: %s", jr.ID, body)
		}
		// Stream a trace chunk.
		traceReq, _ := http.NewRequest("PATCH", ts.URL+"/api/v4/jobs/"+strconv.Itoa(jr.ID)+"/trace",
			strings.NewReader("Running step\n"))
		traceReq.Header.Set("JOB-TOKEN", jr.Token)
		traceReq.Header.Set("Content-Range", "0-12")
		tr, _ := ts.Client().Do(traceReq)
		if tr.StatusCode != http.StatusAccepted {
			t.Fatalf("trace: status %d", tr.StatusCode)
		}
		tr.Body.Close()
		// Report success.
		resp, _ = do(t, ts, "PUT", "/api/v4/jobs/"+strconv.Itoa(jr.ID), map[string]any{
			"token": jr.Token, "state": "success",
		}, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("job update: status %d", resp.StatusCode)
		}
		ran++
	}
	return ran
}
