package bleephub

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestCancellationSignalsRunningJob covers BUG-1745: cancel sends
// JobCancellation to the runner executing a job, leaves always()-gated
// jobs runnable, and the run concludes cancelled.
func TestCancellationSignalsRunningJob(t *testing.T) {
	repoKey := "cancelowner/cancel-repo"
	cancelRepoRunsCleanup(t, repoKey)
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", `name: cancel-ci
on: [push]
jobs:
  slow:
    runs-on: self-hosted
    steps:
      - run: sleep 300
  cleanup:
    needs: [slow]
    if: always()
    runs-on: self-hosted
    steps:
      - run: echo cleanup
`)
	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	var wf *Workflow
	waitUntil(t, "run", func() bool {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		for _, w := range testServer.store.Workflows {
			if w.RepoFullName == repoKey {
				wf = w
				return true
			}
		}
		return false
	})

	// A runner session pulls the slow job and starts running it.
	sess := &Session{
		SessionID: "cancel-sess",
		Agent:     &Agent{ID: 7001, Labels: []Label{{Name: "self-hosted"}}},
		MsgCh:     make(chan *TaskAgentMessage, 10),
	}
	testServer.store.mu.Lock()
	testServer.store.Sessions["cancel-sess"] = sess
	testServer.store.mu.Unlock()
	// The pending queue is shared across tests — keep only this run's
	// job so the pull below deterministically takes it.
	testServer.store.mu.Lock()
	slowJobID := wf.Jobs["slow"].JobID
	kept := testServer.store.PendingMessages[:0]
	for _, m := range testServer.store.PendingMessages {
		if m.JobID == slowJobID {
			kept = append(kept, m)
		}
	}
	testServer.store.PendingMessages = kept
	testServer.store.mu.Unlock()

	msg := testServer.pullPendingMessage(sess)
	if msg == nil || msg.JobID != slowJobID {
		t.Fatalf("runner did not pull the slow job: %v", msg)
	}
	testServer.store.mu.Lock()
	slowID := wf.Jobs["slow"].JobID
	testServer.store.Jobs[slowID].Status = "running"
	wf.Jobs["slow"].Status = JobStatusRunning
	testServer.store.mu.Unlock()

	// Cancel via the REST API.
	resp := ghPost(t, fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/cancel", repoKey, wf.RunID), defaultToken, map[string]interface{}{})
	resp.Body.Close()

	// The runner receives a JobCancellation for the running job.
	var cancelMsg *TaskAgentMessage
	select {
	case cancelMsg = <-sess.MsgCh:
	default:
		t.Fatal("no JobCancellation pushed to the runner's open poll")
	}
	if cancelMsg.MessageType != "JobCancellation" {
		t.Fatalf("message type = %q, want JobCancellation", cancelMsg.MessageType)
	}
	var body struct {
		JobID   string `json:"jobId"`
		Timeout string `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(cancelMsg.Body), &body); err != nil || body.JobID != slowID {
		t.Fatalf("cancellation body = %s (err %v), want jobId %s", cancelMsg.Body, err, slowID)
	}

	testServer.store.mu.RLock()
	slowStatus := wf.Jobs["slow"].Status
	cleanupStatus := wf.Jobs["cleanup"].Status
	testServer.store.mu.RUnlock()
	if slowStatus != JobStatusRunning {
		t.Errorf("running job force-completed server-side: %q (the runner reports the cancel)", slowStatus)
	}
	if cleanupStatus == JobStatusCompleted && wf.Jobs["cleanup"].Result == ResultCancelled {
		t.Error("always() job was cancelled instead of left runnable")
	}

	// The runner aborts and reports; the always() job then dispatches.
	testServer.onJobCompleted(context.Background(), slowID, "Canceled")
	waitUntil(t, "cleanup dispatched", func() bool {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		return wf.Jobs["cleanup"].Status == JobStatusQueued
	})

	// Cleanup completes; run concludes cancelled (not failure).
	testServer.onJobCompleted(context.Background(), wf.Jobs["cleanup"].JobID, "Succeeded")
	waitUntil(t, "run cancelled", func() bool {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		return wf.Status == WorkflowStatusCompleted && wf.Result == ResultCancelled
	})
}

// TestCancelPurgesUndeliveredJobs: cancelling drops queued-but-undelivered
// job messages so a runner can't pull a cancelled job later.
func TestCancelPurgesUndeliveredJobs(t *testing.T) {
	repoKey := "cancelq/cq-repo"
	cancelRepoRunsCleanup(t, repoKey)
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", `name: cq-ci
on: [push]
jobs:
  a:
    runs-on: self-hosted
    steps:
      - run: echo a
`)
	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)
	var wf *Workflow
	waitUntil(t, "run", func() bool {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		for _, w := range testServer.store.Workflows {
			if w.RepoFullName == repoKey {
				wf = w
				return true
			}
		}
		return false
	})

	testServer.cancelWorkflow(wf)

	testServer.store.mu.RLock()
	defer testServer.store.mu.RUnlock()
	for _, msg := range testServer.store.PendingMessages {
		if msg.JobID == wf.Jobs["a"].JobID {
			t.Fatal("cancelled job's message still pending")
		}
	}
	if wf.Jobs["a"].Result != ResultCancelled {
		t.Errorf("job result = %q, want cancelled", wf.Jobs["a"].Result)
	}
	if wf.Result != ResultCancelled {
		t.Errorf("run result = %q, want cancelled", wf.Result)
	}
}

// TestStartupFailureRunShell covers BUG-1747: a matched workflow that
// can't start yields a run with conclusion startup_failure, visible on
// the runs API, with no jobs.
func TestStartupFailureRunShell(t *testing.T) {
	repoKey := "startfail/sf-repo"
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/broken.yml", `name: broken-call
on: [push]
jobs:
  call:
    uses: ./.github/workflows/does-not-exist.yml
`)
	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	var wf *Workflow
	waitUntil(t, "startup_failure run", func() bool {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		for _, w := range testServer.store.Workflows {
			if w.RepoFullName == repoKey {
				wf = w
				return true
			}
		}
		return false
	})
	if wf.Result != ResultStartupFailure || wf.Status != WorkflowStatusCompleted {
		t.Fatalf("run = %q/%q, want completed/startup_failure", wf.Status, wf.Result)
	}
	if len(wf.Jobs) != 0 {
		t.Errorf("startup_failure run has %d jobs, want 0", len(wf.Jobs))
	}

	// Visible through the runs API with the real conclusion.
	resp, err := http.Get(fmt.Sprintf("http://%s/api/v3/repos/%s/actions/runs/%d", testServer.addr, repoKey, wf.RunID))
	if err != nil {
		t.Fatal(err)
	}
	var run map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&run)
	resp.Body.Close()
	if run["conclusion"] != "startup_failure" {
		t.Errorf("API conclusion = %v, want startup_failure", run["conclusion"])
	}
	if run["name"] != "broken-call" {
		t.Errorf("API name = %v, want broken-call", run["name"])
	}
}

// TestRunnerGroupsCRUD covers BUG-1746.
func TestRunnerGroupsCRUD(t *testing.T) {
	resp := ghPost(t, "/api/v3/admin/organizations", defaultToken,
		map[string]interface{}{"login": "rg-org", "admin": "admin"})
	resp.Body.Close()

	testServer.store.mu.Lock()
	agentID := testServer.store.NextAgent
	testServer.store.NextAgent++
	testServer.store.Agents[agentID] = &Agent{ID: agentID, Name: "rg-agent", Status: "online"}
	testServer.store.mu.Unlock()

	do := func(method, path string, body interface{}) (int, map[string]interface{}) {
		var payload []byte
		if body != nil {
			payload, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, "http://"+testServer.addr+path, bytesReader(payload))
		req.Header.Set("Authorization", "Bearer "+defaultToken)
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var out map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&out)
		return r.StatusCode, out
	}

	// Create
	code, created := do("POST", "/api/v3/orgs/rg-org/actions/runner-groups",
		map[string]interface{}{"name": "gpu-pool", "visibility": "selected"})
	if code != http.StatusCreated {
		t.Fatalf("create group = %d", code)
	}
	gid := int(created["id"].(float64))
	if created["default"] != false || created["visibility"] != "selected" {
		t.Errorf("created group shape: %v", created)
	}

	// List includes Default + new
	code, list := do("GET", "/api/v3/orgs/rg-org/actions/runner-groups", nil)
	if code != http.StatusOK || int(list["total_count"].(float64)) < 2 {
		t.Fatalf("list = %d, %v", code, list["total_count"])
	}

	// Membership
	code, _ = do("PUT", fmt.Sprintf("/api/v3/orgs/rg-org/actions/runner-groups/%d/runners/%d", gid, agentID), nil)
	if code != http.StatusNoContent {
		t.Fatalf("add runner = %d", code)
	}
	code, members := do("GET", fmt.Sprintf("/api/v3/orgs/rg-org/actions/runner-groups/%d/runners", gid), nil)
	if code != http.StatusOK || int(members["total_count"].(float64)) != 1 {
		t.Fatalf("group runners = %d, %v", code, members["total_count"])
	}

	// Runner JSON reflects the group
	code, runner := do("GET", fmt.Sprintf("/api/v3/orgs/rg-org/actions/runners/%d", agentID), nil)
	if code != http.StatusOK || int(runner["runner_group_id"].(float64)) != gid {
		t.Errorf("runner_group_id = %v, want %d", runner["runner_group_id"], gid)
	}

	// Rename
	code, patched := do("PATCH", fmt.Sprintf("/api/v3/orgs/rg-org/actions/runner-groups/%d", gid),
		map[string]interface{}{"name": "gpu-pool-2"})
	if code != http.StatusOK || patched["name"] != "gpu-pool-2" {
		t.Errorf("patch = %d, %v", code, patched["name"])
	}

	// Default group is undeletable
	code, _ = do("DELETE", "/api/v3/orgs/rg-org/actions/runner-groups/1", nil)
	if code != http.StatusBadRequest {
		t.Errorf("delete default = %d, want 400", code)
	}

	// Delete: members fall back to Default
	code, _ = do("DELETE", fmt.Sprintf("/api/v3/orgs/rg-org/actions/runner-groups/%d", gid), nil)
	if code != http.StatusNoContent {
		t.Fatalf("delete group = %d", code)
	}
	code, runner = do("GET", fmt.Sprintf("/api/v3/orgs/rg-org/actions/runners/%d", agentID), nil)
	if code != http.StatusOK || int(runner["runner_group_id"].(float64)) != 1 {
		t.Errorf("post-delete runner_group_id = %v, want 1", runner["runner_group_id"])
	}

	// Unknown org 404s
	code, _ = do("GET", "/api/v3/orgs/no-such/actions/runner-groups", nil)
	if code != http.StatusNotFound {
		t.Errorf("unknown org = %d, want 404", code)
	}
}

// bytesReader tolerates nil bodies for request construction.
func bytesReader(b []byte) *bytes.Reader {
	if b == nil {
		return bytes.NewReader(nil)
	}
	return bytes.NewReader(b)
}

// TestLocalActionTarball covers BUG-1748: actions hosted on bleephub
// itself serve GitHub-layout tarballs from their own git storage.
func TestLocalActionTarball(t *testing.T) {
	commitFilesToStorage(t, testServer, "actowner/hello-action", map[string]string{
		"action.yml": `name: hello
runs:
  using: composite
  steps:
    - run: echo "from composite"
      shell: bash
`,
		"README.md": "composite test action",
	})

	resp, err := http.Get("http://" + testServer.addr + "/_apis/v1/actions/tarball/actowner/hello-action/main")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadGateway {
		// Default branch may be master in the test storage.
		resp2, err2 := http.Get("http://" + testServer.addr + "/_apis/v1/actions/tarball/actowner/hello-action/master")
		if err2 != nil {
			t.Fatal(err2)
		}
		defer resp2.Body.Close()
		resp = resp2
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tarball status = %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	found := map[string]bool{}
	var topPrefix string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.SplitN(hdr.Name, "/", 2)
		if topPrefix == "" {
			topPrefix = parts[0]
		} else if parts[0] != topPrefix {
			t.Errorf("multiple top-level dirs: %q vs %q", parts[0], topPrefix)
		}
		if len(parts) == 2 {
			found[parts[1]] = true
		}
		if parts[len(parts)-1] == "action.yml" {
			content, _ := io.ReadAll(tr)
			if !strings.Contains(string(content), "using: composite") {
				t.Error("action.yml content mangled")
			}
		}
	}
	if !found["action.yml"] || !found["README.md"] {
		t.Errorf("tarball entries = %v, want action.yml + README.md under one prefix", found)
	}
	if !strings.HasPrefix(topPrefix, "actowner-hello-action-") {
		t.Errorf("top-level dir = %q, want <owner>-<repo>-<sha> layout", topPrefix)
	}
}

func TestNormalizeResultRunnerSpellings(t *testing.T) {
	// The official runner reports the US spelling "Canceled".
	for _, in := range []string{"Canceled", "canceled", "Cancelled", "cancelled"} {
		if got := normalizeResult(in); got != "cancelled" {
			t.Errorf("normalizeResult(%q) = %q, want cancelled", in, got)
		}
	}
}
