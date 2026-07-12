package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestAgentSatisfiesLabels(t *testing.T) {
	agent := &Agent{Labels: []Label{{Name: "self-hosted"}, {Name: "Linux"}, {Name: "gpu"}}}
	cases := []struct {
		required []string
		want     bool
	}{
		{nil, true},
		{[]string{"self-hosted"}, true},
		{[]string{"self-hosted", "gpu"}, true},
		{[]string{"SELF-HOSTED", "linux"}, true}, // case-insensitive
		{[]string{"self-hosted", "windows"}, false},
		{[]string{"ubuntu-latest"}, true}, // hosted alias: any agent
		{[]string{"ubuntu-22.04"}, true},  // hosted alias family
		{[]string{"macos-14"}, true},      // hosted alias family
		{[]string{"ubuntu-latest", "gpu"}, true},
		{[]string{"ubuntu-latest", "tpu"}, false}, // custom label still strict
	}
	for _, tc := range cases {
		if got := agentSatisfiesLabels(agent, tc.required); got != tc.want {
			t.Errorf("agentSatisfiesLabels(%v) = %v, want %v", tc.required, got, tc.want)
		}
	}
	if agentSatisfiesLabels(nil, []string{"self-hosted"}) {
		t.Error("nil agent must not satisfy strict labels")
	}
	if !agentSatisfiesLabels(nil, []string{"ubuntu-latest"}) {
		t.Error("nil agent satisfies hosted aliases")
	}
}

func TestBusyRunnerNeverReceivesJobs(t *testing.T) {
	s := newTestServer()
	sess := &Session{
		SessionID: "busy-sess",
		Agent:     &Agent{ID: 901, Labels: []Label{{Name: "self-hosted"}}},
		MsgCh:     make(chan *TaskAgentMessage, 10),
	}
	s.store.mu.Lock()
	s.store.Sessions["busy-sess"] = sess
	// An assigned, unfinished job marks the agent busy.
	s.store.Jobs["job-1"] = &Job{ID: "job-1", AgentID: 901, Status: "running"}
	s.store.Jobs["job-2"] = &Job{ID: "job-2", Status: "queued"}
	s.store.mu.Unlock()

	s.queueJobMessage(&TaskAgentMessage{MessageID: 7, JobID: "job-2", Labels: []string{"self-hosted"}})

	// While busy, polls must not pull the queued job.
	if got := s.pullPendingMessage(sess); got != nil {
		t.Fatal("busy runner's poll pulled a job message")
	}

	// Job finishes → the next poll pulls the pending message.
	s.store.mu.Lock()
	s.store.Jobs["job-1"].Status = "completed"
	s.store.mu.Unlock()
	got := s.pullPendingMessage(sess)
	if got == nil || got.MessageID != 7 {
		t.Fatalf("free runner's poll did not pull the pending job: %v", got)
	}
	s.store.mu.RLock()
	agentID := s.store.Jobs["job-2"].AgentID
	pending := len(s.store.PendingMessages)
	s.store.mu.RUnlock()
	if agentID != 901 {
		t.Errorf("pulled job not associated with the agent: AgentID=%d", agentID)
	}
	if pending != 0 {
		t.Errorf("pending queue not drained: %d left", pending)
	}
}

func TestLabelRoutingQueuesUntilMatch(t *testing.T) {
	s := newTestServer()

	mkSession := func(id string, labels ...string) *Session {
		ls := make([]Label, 0, len(labels))
		for _, l := range labels {
			ls = append(ls, Label{Name: l})
		}
		sess := &Session{
			SessionID: id,
			Agent:     &Agent{ID: len(id), Labels: ls},
			MsgCh:     make(chan *TaskAgentMessage, 10),
		}
		s.store.mu.Lock()
		s.store.Sessions[id] = sess
		s.store.mu.Unlock()
		return sess
	}
	plain := mkSession("a-plain", "self-hosted", "linux")

	s.queueJobMessage(&TaskAgentMessage{MessageID: 1, Labels: []string{"self-hosted", "gpu"}})

	// A poll from a non-matching runner must not pull the job.
	if got := s.pullPendingMessage(plain); got != nil {
		t.Fatal("job pulled by a runner without the required labels")
	}

	// A matching runner's poll pulls it.
	gpu := mkSession("b-gpu", "self-hosted", "linux", "gpu")
	got := s.pullPendingMessage(gpu)
	if got == nil || got.MessageID != 1 {
		t.Fatalf("matching runner's poll did not pull the job: %v", got)
	}
	if again := s.pullPendingMessage(gpu); again != nil {
		t.Fatal("message pulled twice")
	}
}

// cancelRepoRunsCleanup cancels every in-flight run a test created on
// the shared testServer, keeping the global max-concurrent-workflows
// budget free for unrelated tests.
func cancelRepoRunsCleanup(t *testing.T, repoKey string) {
	t.Helper()
	t.Cleanup(func() {
		testServer.store.mu.RLock()
		var runs []*Workflow
		for _, w := range testServer.store.Workflows {
			if w.RepoFullName == repoKey && w.Status != WorkflowStatusCompleted {
				runs = append(runs, w)
			}
		}
		testServer.store.mu.RUnlock()
		for _, w := range runs {
			testServer.cancelWorkflow(w)
		}
	})
}

func seedRerunRepo(t *testing.T, repoKey, yaml string) *Workflow {
	t.Helper()
	cancelRepoRunsCleanup(t, repoKey)
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", yaml)
	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)
	var wf *Workflow
	waitUntil(t, "triggered run", func() bool {
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
	return wf
}

func assertWorkflowJobsUseHostMode(t *testing.T, wf *Workflow, keys ...string) {
	t.Helper()
	if len(keys) == 0 {
		for key := range wf.Jobs {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		job := wf.Jobs[key]
		if job == nil {
			t.Fatalf("workflow job %q not found", key)
		}
		testServer.store.mu.RLock()
		queued := testServer.store.Jobs[job.JobID]
		testServer.store.mu.RUnlock()
		if queued == nil {
			t.Fatalf("workflow job %q has no stored runner job %q", key, job.JobID)
		}
		if queued.Message == "" {
			t.Fatalf("workflow job %q has no runner message", key)
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(queued.Message), &msg); err != nil {
			t.Fatalf("workflow job %q message JSON: %v", key, err)
		}
		if msg["jobContainer"] != nil {
			t.Fatalf("workflow job %q jobContainer = %#v, want nil for a workflow without container", key, msg["jobContainer"])
		}
	}
}

const twoJobYAML = `name: ci
on: [push]
jobs:
  good:
    runs-on: ubuntu-latest
    steps:
      - run: echo ok
  bad:
    runs-on: ubuntu-latest
    steps:
      - run: exit 1
`

func TestRerunKeepsRunIDAndBumpsAttempt(t *testing.T) {
	repoKey := "rerunowner/rerun-repo"
	wf := seedRerunRepo(t, repoKey, twoJobYAML)
	origRunID := wf.RunID
	assertWorkflowJobsUseHostMode(t, wf)

	// Finish both jobs (one failure) so the run completes.
	testServer.onJobCompleted(context.Background(), wf.Jobs["good"].JobID, "Succeeded")
	testServer.onJobCompleted(context.Background(), wf.Jobs["bad"].JobID, "Failed")

	resp := ghPost(t, fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/rerun", repoKey, origRunID), defaultToken, map[string]interface{}{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("rerun status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Same run id, attempt 2, both jobs fresh.
	resp2, err := http.Get(fmt.Sprintf("http://%s/api/v3/repos/%s/actions/runs/%d", testServer.addr, repoKey, origRunID))
	if err != nil {
		t.Fatal(err)
	}
	var run map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&run)
	resp2.Body.Close()
	if int(run["run_attempt"].(float64)) != 2 {
		t.Errorf("run_attempt = %v, want 2", run["run_attempt"])
	}
	if int(run["id"].(float64)) != origRunID {
		t.Errorf("rerun id = %v, want %d (same run id)", run["id"], origRunID)
	}
	testServer.store.mu.RLock()
	var attempt2 *Workflow
	for _, w := range testServer.store.Workflows {
		if w.RepoFullName == repoKey && w.RunID == origRunID {
			attempt2 = w
			break
		}
	}
	testServer.store.mu.RUnlock()
	if attempt2 == nil {
		t.Fatal("rerun attempt 2 not found")
	}
	assertWorkflowJobsUseHostMode(t, attempt2)

	// The first attempt is retrievable.
	resp3, err := http.Get(fmt.Sprintf("http://%s/api/v3/repos/%s/actions/runs/%d/attempts/1", testServer.addr, repoKey, origRunID))
	if err != nil {
		t.Fatal(err)
	}
	var att map[string]interface{}
	_ = json.NewDecoder(resp3.Body).Decode(&att)
	resp3.Body.Close()
	if int(att["run_attempt"].(float64)) != 1 {
		t.Errorf("attempt 1 run_attempt = %v", att["run_attempt"])
	}
	if att["conclusion"] != "failure" {
		t.Errorf("attempt 1 conclusion = %v, want failure", att["conclusion"])
	}

	// Attempt 1 jobs endpoint serves the archived jobs.
	resp4, err := http.Get(fmt.Sprintf("http://%s/api/v3/repos/%s/actions/runs/%d/attempts/1/jobs", testServer.addr, repoKey, origRunID))
	if err != nil {
		t.Fatal(err)
	}
	var jobs struct {
		TotalCount int `json:"total_count"`
	}
	_ = json.NewDecoder(resp4.Body).Decode(&jobs)
	resp4.Body.Close()
	if jobs.TotalCount != 2 {
		t.Errorf("attempt 1 jobs = %d, want 2", jobs.TotalCount)
	}
}

func TestRerunFailedJobsCarriesSuccesses(t *testing.T) {
	repoKey := "rerunfail/rf-repo"
	wf := seedRerunRepo(t, repoKey, twoJobYAML)
	runID := wf.RunID
	assertWorkflowJobsUseHostMode(t, wf)

	testServer.store.mu.Lock()
	wf.Jobs["good"].Outputs["artifact"] = "kept"
	testServer.store.mu.Unlock()
	testServer.onJobCompleted(context.Background(), wf.Jobs["good"].JobID, "Succeeded")
	testServer.onJobCompleted(context.Background(), wf.Jobs["bad"].JobID, "Failed")

	resp := ghPost(t, fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/rerun-failed-jobs", repoKey, runID), defaultToken, map[string]interface{}{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("rerun-failed-jobs status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	var attempt2 *Workflow
	waitUntil(t, "attempt 2", func() bool {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		for _, w := range testServer.store.Workflows {
			if w.RepoFullName == repoKey && w.RunID == runID {
				attempt2 = w
				return w.AttemptNumber() == 2
			}
		}
		return false
	})

	testServer.store.mu.RLock()
	good := attempt2.Jobs["good"]
	bad := attempt2.Jobs["bad"]
	goodStatus, goodResult, goodOut := good.Status, good.Result, good.Outputs["artifact"]
	badStatus := bad.Status
	testServer.store.mu.RUnlock()

	if goodStatus != JobStatusCompleted || goodResult != ResultSuccess {
		t.Errorf("good carried over: status=%q result=%q", goodStatus, goodResult)
	}
	if goodOut != "kept" {
		t.Errorf("good outputs not carried: %q", goodOut)
	}
	if badStatus != JobStatusQueued {
		t.Errorf("bad job should re-dispatch (queued), got %q", badStatus)
	}
	assertWorkflowJobsUseHostMode(t, attempt2, "bad")
}

func TestWorkflowEnableDisable(t *testing.T) {
	repoKey := "disowner/dis-repo"
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", `name: dis-ci
on: [push, workflow_dispatch]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)
	testServer.store.DiscoverWorkflowFilesFromGit(repoKey)

	disable := func(verb string) int {
		req, _ := http.NewRequest("PUT",
			fmt.Sprintf("http://%s/api/v3/repos/%s/actions/workflows/ci.yml/%s", testServer.addr, repoKey, verb), nil)
		req.Header.Set("Authorization", "Bearer "+defaultToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code := disable("disable"); code != http.StatusNoContent {
		t.Fatalf("disable status = %d", code)
	}
	wfFile := testServer.resolveWorkflowFile(repoKey, "ci.yml")
	if wfFile.State != "disabled_manually" {
		t.Errorf("state = %q, want disabled_manually", wfFile.State)
	}

	// Dispatch while disabled → 403.
	resp := ghPost(t, fmt.Sprintf("/api/v3/repos/%s/actions/workflows/ci.yml/dispatches", repoKey), defaultToken,
		map[string]interface{}{"ref": "refs/heads/main"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("dispatch while disabled = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Push trigger while disabled → no run.
	before := countRepoRuns(repoKey)
	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)
	time.Sleep(100 * time.Millisecond)
	if got := countRepoRuns(repoKey); got != before {
		t.Errorf("disabled workflow triggered: runs %d → %d", before, got)
	}

	if code := disable("enable"); code != http.StatusNoContent {
		t.Fatalf("enable status = %d", code)
	}
	if wfFile := testServer.resolveWorkflowFile(repoKey, "ci.yml"); wfFile.State != "active" {
		t.Errorf("state after enable = %q", wfFile.State)
	}
}

func countRepoRuns(repoKey string) int {
	testServer.store.mu.RLock()
	defer testServer.store.mu.RUnlock()
	n := 0
	for _, w := range testServer.store.Workflows {
		if w.RepoFullName == repoKey {
			n++
		}
	}
	return n
}

func TestOrgRunnerEndpoints(t *testing.T) {
	// Seed an org + an agent.
	resp := ghPost(t, "/api/v3/admin/organizations", defaultToken,
		map[string]interface{}{"login": "runner-org", "admin": "admin"})
	resp.Body.Close()

	testServer.store.mu.Lock()
	agentID := testServer.store.NextAgent
	testServer.store.NextAgent++
	testServer.store.Agents[agentID] = &Agent{ID: agentID, Name: "org-agent", Status: "online",
		Labels: []Label{{Name: "self-hosted"}}}
	testServer.store.mu.Unlock()

	get := func(path string) (int, map[string]interface{}) {
		req, _ := http.NewRequest("GET", "http://"+testServer.addr+path, nil)
		req.Header.Set("Authorization", "Bearer "+defaultToken)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		return r.StatusCode, body
	}

	code, body := get("/api/v3/orgs/runner-org/actions/runners")
	if code != http.StatusOK {
		t.Fatalf("org runners list = %d", code)
	}
	if int(body["total_count"].(float64)) < 1 {
		t.Errorf("org runners total_count = %v", body["total_count"])
	}

	code, runner := get(fmt.Sprintf("/api/v3/orgs/runner-org/actions/runners/%d", agentID))
	if code != http.StatusOK || runner["name"] != "org-agent" {
		t.Errorf("org runner get = %d, name %v", code, runner["name"])
	}
	if _, ok := runner["busy"].(bool); !ok {
		t.Errorf("runner busy missing/false-typed: %v", runner["busy"])
	}

	code, _ = get("/api/v3/orgs/no-such-org/actions/runners")
	if code != http.StatusNotFound {
		t.Errorf("unknown org runners list = %d, want 404", code)
	}

	respTok := ghPost(t, "/api/v3/orgs/runner-org/actions/runners/registration-token", defaultToken, map[string]interface{}{})
	if respTok.StatusCode != http.StatusCreated {
		t.Errorf("org registration-token = %d, want 201", respTok.StatusCode)
	}
	respTok.Body.Close()
}
