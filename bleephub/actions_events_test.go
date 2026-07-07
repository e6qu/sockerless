package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// eventRecorder captures webhook deliveries (event + action) for
// assertion; bleephub delivers asynchronously.
type eventRecorder struct {
	mu     sync.Mutex
	events []string // "event/action"
}

func (er *eventRecorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		er.mu.Lock()
		er.events = append(er.events, r.Header.Get("X-GitHub-Event")+"/"+body.Action)
		er.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func (er *eventRecorder) has(want string) bool {
	er.mu.Lock()
	defer er.mu.Unlock()
	for _, e := range er.events {
		if e == want {
			return true
		}
	}
	return false
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestActionsChecksLifecycle(t *testing.T) {
	repoKey := "checksowner/checks-repo"
	cancelRepoRunsCleanup(t, repoKey)
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", `name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)

	rec := &eventRecorder{}
	receiver := httptest.NewServer(rec.handler())
	defer receiver.Close()
	testServer.store.CreateHook(repoKey, receiver.URL, "", "json", "0", []string{"*"}, true)

	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	var wf *Workflow
	waitUntil(t, "workflow", func() bool {
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

	// The drain creates one check run per job, queued, under a suite.
	var checkRun *CheckRun
	waitUntil(t, "check run", func() bool {
		runs := testServer.store.ListCheckRunsForCommit(repoKey, wf.Sha, "", "", 0)
		if len(runs) != 1 {
			return false
		}
		checkRun = runs[0]
		return true
	})
	if checkRun.Name != "build" {
		t.Errorf("check run name = %q, want build", checkRun.Name)
	}
	if checkRun.AppID != githubActionsAppID {
		t.Errorf("check run app = %d, want %d", checkRun.AppID, githubActionsAppID)
	}
	suites := testServer.store.ListCheckSuitesForCommit(repoKey, wf.Sha, githubActionsAppID)
	if len(suites) != 1 {
		t.Fatalf("suites = %d, want 1", len(suites))
	}

	waitUntil(t, "workflow_run requested", func() bool { return rec.has("workflow_run/requested") })
	waitUntil(t, "workflow_job queued", func() bool { return rec.has("workflow_job/queued") })
	waitUntil(t, "check_run created", func() bool { return rec.has("check_run/created") })
	waitUntil(t, "check_suite requested", func() bool { return rec.has("check_suite/requested") })

	// Runner pickup: renew the request → in_progress.
	testServer.store.mu.RLock()
	job := testServer.store.Jobs[wf.Jobs["build"].JobID]
	testServer.store.mu.RUnlock()
	if job == nil {
		t.Fatal("engine job missing")
	}
	req, _ := http.NewRequest("PATCH",
		fmt.Sprintf("http://%s/_apis/v1/AgentRequest/1/%d", testServer.addr, job.RequestID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	waitUntil(t, "check run in_progress", func() bool {
		return testServer.store.GetCheckRun(checkRun.ID).Status == "in_progress"
	})
	waitUntil(t, "workflow_run in_progress", func() bool { return rec.has("workflow_run/in_progress") })
	waitUntil(t, "workflow_job in_progress", func() bool { return rec.has("workflow_job/in_progress") })

	// Completion: check run success, suite completed, completed events.
	testServer.onJobCompleted(context.Background(), wf.Jobs["build"].JobID, "Succeeded")
	waitUntil(t, "check run success", func() bool {
		cr := testServer.store.GetCheckRun(checkRun.ID)
		return cr.Status == "completed" && cr.Conclusion == "success"
	})
	waitUntil(t, "suite completed", func() bool {
		s := testServer.store.GetCheckSuite(suites[0].ID)
		return s.Status == "completed" && s.Conclusion == "success"
	})
	waitUntil(t, "workflow_run completed", func() bool { return rec.has("workflow_run/completed") })
	waitUntil(t, "workflow_job completed", func() bool { return rec.has("workflow_job/completed") })
	waitUntil(t, "check_run completed", func() bool { return rec.has("check_run/completed") })
	waitUntil(t, "check_suite completed", func() bool { return rec.has("check_suite/completed") })
}

func TestActionsSkippedJobCheckRun(t *testing.T) {
	repoKey := "checkskip/skip-repo"
	cancelRepoRunsCleanup(t, repoKey)
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", `name: skip-ci
on: [push]
jobs:
  build:
    if: github.ref == 'refs/heads/never'
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)
	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	var wf *Workflow
	waitUntil(t, "workflow", func() bool {
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
	waitUntil(t, "skipped check run", func() bool {
		runs := testServer.store.ListCheckRunsForCommit(repoKey, wf.Sha, "", "", 0)
		return len(runs) == 1 && runs[0].Status == "completed" && runs[0].Conclusion == "skipped"
	})
}

func TestMergeGatingByRequiredChecks(t *testing.T) {
	owner := "gateowner"
	repoName := "gate-repo"
	repoKey := owner + "/" + repoName
	// Head branch content so the PR head sha resolves.
	commitFilesToStorage(t, testServer, repoKey, map[string]string{"README.md": "hi"})
	repo := testServer.store.GetRepo(owner, repoName)
	user := testServer.store.UsersByLogin[owner]

	// The default branch the commit landed on serves as the PR head.
	stor := testServer.store.GetGitStorage(owner, repoName)
	headBranch := "main"
	if resolveBranchSha(stor, "main") == "" {
		headBranch = "master"
	}
	seedStorePullRequestBranches(t, testServer.store, repo, headBranch, "base")
	headSha := resolveBranchSha(stor, headBranch)
	if headSha == "" {
		t.Fatal("head branch sha did not resolve")
	}

	pr := testServer.store.CreatePullRequest(repo.ID, user.ID, "gate", "", headBranch, "base", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("PR not created")
	}
	testServer.store.UpdatePullRequest(pr.ID, func(p *PullRequest) { p.Mergeable = "MERGEABLE" })

	// Protect the base branch with a required status check.
	testServer.store.mu.Lock()
	testServer.store.Misc.branchProtection[bpKey(repo.ID, "base")] = &BranchProtection{
		RequiredStatusChecks: &BPStatusChecks{
			Strict:   false,
			Contexts: []string{"ci-job"},
		},
	}
	testServer.store.mu.Unlock()

	// No check runs yet → blocked + merge rejected.
	out := pullRequestToJSON(testServer.store.GetPullRequest(pr.ID), testServer.store, "http://x", repoKey)
	testServer.applyChecksToMergeability(out, repo, testServer.store.GetPullRequest(pr.ID))
	if out["mergeable_state"] != "blocked" {
		t.Errorf("mergeable_state = %v, want blocked", out["mergeable_state"])
	}

	resp := ghPut(t, fmt.Sprintf("/api/v3/repos/%s/pulls/%d/merge", repoKey, pr.Number), defaultToken, map[string]interface{}{})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("merge with missing required check: status %d, want 405", resp.StatusCode)
	}
	resp.Body.Close()

	// A pending check run with the required name still blocks.
	cr := testServer.store.CreateCheckRun(repoKey, headSha, "ci-job", githubActionsAppID, 0)
	resp = ghPut(t, fmt.Sprintf("/api/v3/repos/%s/pulls/%d/merge", repoKey, pr.Number), defaultToken, map[string]interface{}{})
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("merge with pending required check: status %d, want 405", resp.StatusCode)
	}
	resp.Body.Close()

	// Green required check → merge allowed.
	now := time.Now().UTC()
	testServer.store.UpdateCheckRun(cr.ID, func(c *CheckRun) {
		c.Status = "completed"
		c.Conclusion = "success"
		c.CompletedAt = &now
	})
	out = pullRequestToJSON(testServer.store.GetPullRequest(pr.ID), testServer.store, "http://x", repoKey)
	testServer.applyChecksToMergeability(out, repo, testServer.store.GetPullRequest(pr.ID))
	if out["mergeable_state"] != "clean" {
		t.Errorf("mergeable_state after green check = %v, want clean", out["mergeable_state"])
	}
	resp = ghPut(t, fmt.Sprintf("/api/v3/repos/%s/pulls/%d/merge", repoKey, pr.Number), defaultToken, map[string]interface{}{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merge with green required check: status %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUnstableMergeableStateOnFailingNonRequired(t *testing.T) {
	owner := "unstableowner"
	repoName := "unstable-repo"
	repoKey := owner + "/" + repoName
	commitFilesToStorage(t, testServer, repoKey, map[string]string{"README.md": "hi"})
	repo := testServer.store.GetRepo(owner, repoName)
	user := testServer.store.UsersByLogin[owner]
	stor := testServer.store.GetGitStorage(owner, repoName)
	headBranch := "main"
	if resolveBranchSha(stor, "main") == "" {
		headBranch = "master"
	}
	seedStorePullRequestBranches(t, testServer.store, repo, headBranch, "base")
	headSha := resolveBranchSha(stor, headBranch)
	if headSha == "" {
		t.Fatal("head branch sha did not resolve")
	}
	pr := testServer.store.CreatePullRequest(repo.ID, user.ID, "u", "", headBranch, "base", false, nil, nil, 0)
	testServer.store.UpdatePullRequest(pr.ID, func(p *PullRequest) { p.Mergeable = "MERGEABLE" })

	cr := testServer.store.CreateCheckRun(repoKey, headSha, "lint", githubActionsAppID, 0)
	now := time.Now().UTC()
	testServer.store.UpdateCheckRun(cr.ID, func(c *CheckRun) {
		c.Status = "completed"
		c.Conclusion = "failure"
		c.CompletedAt = &now
	})

	out := pullRequestToJSON(testServer.store.GetPullRequest(pr.ID), testServer.store, "http://x", repoKey)
	testServer.applyChecksToMergeability(out, repo, testServer.store.GetPullRequest(pr.ID))
	if out["mergeable_state"] != "unstable" {
		t.Errorf("mergeable_state = %v, want unstable (failing non-required check)", out["mergeable_state"])
	}
}
