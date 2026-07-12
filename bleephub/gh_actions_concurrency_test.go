package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// submitConcurrencyWorkflow submits a one-job workflow with a
// concurrency group through the real workflow-submission surface and
// returns the engine workflow id.
func submitConcurrencyWorkflow(t *testing.T, name, group, repo string) string {
	t.Helper()
	yaml := fmt.Sprintf("name: %s\nconcurrency: %s\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n", name, group)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		t.Fatalf("expected owner/repo, got %q", repo)
	}
	repoRow := testServer.store.GetRepoByFullName(repo)
	if repoRow == nil {
		t.Fatalf("repository %s missing", repo)
	}
	stor := testServer.store.GetGitStorage(parts[0], parts[1])
	if stor == nil {
		t.Fatalf("git storage for %s missing", repo)
	}
	if resolveBranchSha(stor, repoRow.DefaultBranch) == "" {
		admin := testServer.store.Users[1]
		if _, err := initRepoWithFiles(stor, repoRow.DefaultBranch, "seed workflow", map[string]string{
			".github/workflows/" + name + ".yml": yaml,
		}, repoSignature(admin.Login, "bleephub@local")); err != nil {
			t.Fatalf("seed workflow git state: %v", err)
		}
	}
	body, _ := json.Marshal(map[string]string{"workflow": yaml, "repo": repo, "image": "alpine:latest"})
	resp, err := authedPost("/internal/exec/workflow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	data := decodeJSONWithStatus(t, resp, 200)
	id, _ := data["workflowId"].(string)
	if id == "" {
		t.Fatalf("no workflowId in submit response: %v", data)
	}
	return id
}

func cancelWorkflowByID(t *testing.T, id string) {
	t.Helper()
	resp, err := authedPost("/internal/exec/workflows/"+id+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// runIDByName resolves a run's GitHub-shape id from the repo run list.
func runIDByName(t *testing.T, repo, name string) int {
	t.Helper()
	data := decodeJSONWithStatus(t, ghGet(t, "/api/v3/repos/"+repo+"/actions/runs", defaultToken), 200)
	runs, _ := data["workflow_runs"].([]interface{})
	for _, r := range runs {
		run, _ := r.(map[string]interface{})
		if run["name"] == name {
			return int(run["id"].(float64))
		}
	}
	t.Fatalf("run %q not found in %v", name, data)
	return 0
}

func TestConcurrencyGroups_RepoAndRunEndpoints(t *testing.T) {
	org := seedTestOrg(t, "cg-org")
	repo := seedOrgRepo(t, org, "cg-repo", false).FullName
	group := "cg-test-group"
	wf1 := submitConcurrencyWorkflow(t, "cg-holder", group, repo)
	wf2 := submitConcurrencyWorkflow(t, "cg-waiter", group, repo)
	defer cancelWorkflowByID(t, wf2)
	defer cancelWorkflowByID(t, wf1)

	holderID := runIDByName(t, repo, "cg-holder")
	waiterID := runIDByName(t, repo, "cg-waiter")

	// Repo-level active group list: one group, lease acquired.
	data := decodeJSONWithStatus(t, ghGet(t, "/api/v3/repos/"+repo+"/actions/concurrency_groups", defaultToken), 200)
	groups, _ := data["concurrency_groups"].([]interface{})
	if int(data["total_count"].(float64)) != 1 || len(groups) != 1 {
		t.Fatalf("concurrency_groups = %v, want exactly the one active group", data)
	}
	entry, _ := groups[0].(map[string]interface{})
	if entry["group_name"] != group || entry["last_acquired_at"] == nil {
		t.Fatalf("group entry = %v, want %q with a lease timestamp", entry, group)
	}
	if !strings.Contains(entry["group_url"].(string), "/actions/concurrency_groups/") {
		t.Fatalf("group_url = %v", entry["group_url"])
	}

	// Group by name: holder in_progress, waiter pending.
	groupPath := "/api/v3/repos/" + repo + "/actions/concurrency_groups/" + url.PathEscape(group)
	data = decodeJSONWithStatus(t, ghGet(t, groupPath, defaultToken), 200)
	members, _ := data["group_members"].([]interface{})
	if int(data["total_count"].(float64)) != 2 || len(members) != 2 {
		t.Fatalf("group members = %v, want holder + waiter", data)
	}
	statusByRun := map[int]string{}
	for _, m := range members {
		member, _ := m.(map[string]interface{})
		statusByRun[int(member["run_id"].(float64))] = member["status"].(string)
	}
	if statusByRun[holderID] != "in_progress" || statusByRun[waiterID] != "pending" {
		t.Fatalf("member statuses = %v, want holder in_progress / waiter pending", statusByRun)
	}

	// ahead_of_run: only the holder is ahead of the waiter.
	data = decodeJSONWithStatus(t, ghGet(t, fmt.Sprintf("%s?ahead_of_run=%d", groupPath, waiterID), defaultToken), 200)
	members, _ = data["group_members"].([]interface{})
	if len(members) != 1 || int(members[0].(map[string]interface{})["run_id"].(float64)) != holderID {
		t.Fatalf("ahead_of_run members = %v, want only the holder", members)
	}
	// A run that isn't a member rejects.
	resp := ghGet(t, groupPath+"?ahead_of_run=999999", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("ahead_of_run non-member = %d, want 422", resp.StatusCode)
	}

	// Run-level view: the waiter queues at position 1 with a position_url.
	data = decodeJSONWithStatus(t, ghGet(t,
		fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/concurrency_groups", repo, waiterID), defaultToken), 200)
	groups, _ = data["concurrency_groups"].([]interface{})
	if int(data["total_count"].(float64)) != 1 || len(groups) != 1 {
		t.Fatalf("run concurrency_groups = %v", data)
	}
	entry, _ = groups[0].(map[string]interface{})
	members, _ = entry["group_members"].([]interface{})
	if len(members) != 1 {
		t.Fatalf("run group members = %v, want the run's own entry", entry)
	}
	member, _ := members[0].(map[string]interface{})
	if member["position"].(float64) != 1 || member["status"] != "pending" {
		t.Fatalf("waiter member = %v, want position 1 / pending", member)
	}
	if !strings.Contains(member["position_url"].(string), "ahead_of_run=") {
		t.Fatalf("position_url = %v", member["position_url"])
	}

	// The holder reports position 0 / in_progress.
	data = decodeJSONWithStatus(t, ghGet(t,
		fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/concurrency_groups", repo, holderID), defaultToken), 200)
	groups, _ = data["concurrency_groups"].([]interface{})
	entry, _ = groups[0].(map[string]interface{})
	members, _ = entry["group_members"].([]interface{})
	member, _ = members[0].(map[string]interface{})
	if member["position"].(float64) != 0 || member["status"] != "in_progress" {
		t.Fatalf("holder member = %v, want position 0 / in_progress", member)
	}

	// Unknown group 404s.
	resp = ghGet(t, "/api/v3/repos/"+repo+"/actions/concurrency_groups/no-such-group", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown group = %d, want 404", resp.StatusCode)
	}
}

func TestConcurrencyGroups_CompletedRunReleasesLease(t *testing.T) {
	org := seedTestOrg(t, "cg-org")
	repo := seedOrgRepo(t, org, "cg-repo-done", false).FullName
	wfID := submitConcurrencyWorkflow(t, "cg-done", "cg-done-group", repo)
	runID := runIDByName(t, repo, "cg-done")
	cancelWorkflowByID(t, wfID)

	// Cancelled (completed) run: its group is no longer active
	// repo-wide, and the run-level view lists the group with no members.
	data := decodeJSONWithStatus(t, ghGet(t, "/api/v3/repos/"+repo+"/actions/concurrency_groups", defaultToken), 200)
	if int(data["total_count"].(float64)) != 0 {
		t.Fatalf("active groups after completion = %v, want none", data)
	}
	data = decodeJSONWithStatus(t, ghGet(t,
		fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/concurrency_groups", repo, runID), defaultToken), 200)
	groups, _ := data["concurrency_groups"].([]interface{})
	if int(data["total_count"].(float64)) != 1 || len(groups) != 1 {
		t.Fatalf("run groups after completion = %v, want the configured group", data)
	}
	members, _ := groups[0].(map[string]interface{})["group_members"].([]interface{})
	if len(members) != 0 {
		t.Fatalf("members after completion = %v, want empty (lease released)", members)
	}

	// A run with no concurrency configuration reports zero groups.
	body, _ := json.Marshal(map[string]string{
		"workflow": "name: cg-none\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo hi\n",
		"repo":     repo,
		"image":    "alpine:latest",
	})
	resp, err := authedPost("/internal/exec/workflow", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	sub := decodeJSONWithStatus(t, resp, 200)
	defer cancelWorkflowByID(t, sub["workflowId"].(string))
	plainID := runIDByName(t, repo, "cg-none")
	data = decodeJSONWithStatus(t, ghGet(t,
		fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/concurrency_groups", repo, plainID), defaultToken), 200)
	if int(data["total_count"].(float64)) != 0 {
		t.Fatalf("no-concurrency run groups = %v, want 0", data)
	}
}
