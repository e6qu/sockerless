package bleephub

import (
	"testing"
)

func TestAgentTasks_CreateAndReadBack(t *testing.T) {
	repo := seedTestRepo(t, "agent-tasks-repo", false)
	repoBase := "/api/v3/agents/repos/" + repo.FullName + "/tasks"

	// Create.
	resp := ghPost(t, repoBase, defaultToken, map[string]interface{}{
		"prompt":              "Fix the login button on the homepage\n\nIt renders off-screen on mobile.",
		"model":               "claude-sonnet-4.6",
		"create_pull_request": true,
		"base_ref":            "main",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create task status %d, want 201", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	taskID, _ := created["id"].(string)
	if taskID == "" {
		t.Fatal("created task has no id")
	}
	if created["state"] != "queued" {
		t.Fatalf("state = %v, want queued", created["state"])
	}
	if created["name"] != "Fix the login button on the homepage" {
		t.Fatalf("name = %v, want first prompt line", created["name"])
	}
	if created["session_count"] != float64(1) {
		t.Fatalf("session_count = %v, want 1", created["session_count"])
	}
	if created["creator_type"] != "user" {
		t.Fatalf("creator_type = %v, want user", created["creator_type"])
	}
	repoRef := created["repository"].(map[string]interface{})
	if repoRef["id"] != float64(repo.ID) {
		t.Fatalf("repository.id = %v, want %d", repoRef["id"], repo.ID)
	}
	admin := testServer.store.LookupUserByLogin("admin")
	creator := created["creator"].(map[string]interface{})
	if creator["id"] != float64(admin.ID) {
		t.Fatalf("creator.id = %v, want %d", creator["id"], admin.ID)
	}

	// Get by global id — includes the sessions.
	resp = ghGet(t, "/api/v3/agents/tasks/"+taskID, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get task status %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	sessions, _ := got["sessions"].([]interface{})
	if len(sessions) != 1 {
		t.Fatalf("sessions = %v, want 1 session", got["sessions"])
	}
	sess := sessions[0].(map[string]interface{})
	if sess["task_id"] != taskID {
		t.Fatalf("session task_id = %v, want %s", sess["task_id"], taskID)
	}
	if sess["state"] != "queued" {
		t.Fatalf("session state = %v, want queued", sess["state"])
	}
	if sess["model"] != "claude-sonnet-4.6" {
		t.Fatalf("session model = %v, want claude-sonnet-4.6", sess["model"])
	}

	// Get by repo + id.
	resp = ghGet(t, repoBase+"/"+taskID, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get repo task status %d, want 200", resp.StatusCode)
	}
	got = decodeJSON(t, resp)
	if got["id"] != taskID {
		t.Fatalf("repo task id = %v, want %s", got["id"], taskID)
	}

	// List for repo.
	resp = ghGet(t, repoBase, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list repo tasks status %d, want 200", resp.StatusCode)
	}
	list := decodeJSON(t, resp)
	tasks, _ := list["tasks"].([]interface{})
	if len(tasks) != 1 {
		t.Fatalf("repo tasks = %d, want 1", len(tasks))
	}
	if list["total_active_count"] != float64(1) {
		t.Fatalf("total_active_count = %v, want 1", list["total_active_count"])
	}
	if list["total_archived_count"] != float64(0) {
		t.Fatalf("total_archived_count = %v, want 0", list["total_archived_count"])
	}

	// List for the authenticated user contains the task too.
	resp = ghGet(t, "/api/v3/agents/tasks", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list user tasks status %d, want 200", resp.StatusCode)
	}
	list = decodeJSON(t, resp)
	found := false
	for _, item := range list["tasks"].([]interface{}) {
		if item.(map[string]interface{})["id"] == taskID {
			found = true
		}
	}
	if !found {
		t.Fatal("created task missing from the authenticated user's task list")
	}
}

func TestAgentTasks_Filters(t *testing.T) {
	repo := seedTestRepo(t, "agent-tasks-filter", false)
	repoBase := "/api/v3/agents/repos/" + repo.FullName + "/tasks"

	resp := ghPost(t, repoBase, defaultToken, map[string]interface{}{"prompt": "task one"})
	mustStatus(t, resp, 201, "create task one")
	resp = ghPost(t, repoBase, defaultToken, map[string]interface{}{"prompt": "task two"})
	mustStatus(t, resp, 201, "create task two")

	// Every stored task is queued; a completed-only filter matches none.
	resp = ghGet(t, repoBase+"?state=completed", defaultToken)
	list := decodeJSON(t, resp)
	if n := len(list["tasks"].([]interface{})); n != 0 {
		t.Fatalf("completed filter matched %d tasks, want 0", n)
	}

	// The queued filter matches both.
	resp = ghGet(t, repoBase+"?state=queued,failed", defaultToken)
	list = decodeJSON(t, resp)
	if n := len(list["tasks"].([]interface{})); n != 2 {
		t.Fatalf("queued filter matched %d tasks, want 2", n)
	}

	// Archived-only view is empty (nothing archives tasks).
	resp = ghGet(t, repoBase+"?is_archived=true", defaultToken)
	list = decodeJSON(t, resp)
	if n := len(list["tasks"].([]interface{})); n != 0 {
		t.Fatalf("archived filter matched %d tasks, want 0", n)
	}

	// created_at ascending puts task one first.
	resp = ghGet(t, repoBase+"?sort=created_at&direction=asc", defaultToken)
	list = decodeJSON(t, resp)
	tasks := list["tasks"].([]interface{})
	if len(tasks) != 2 {
		t.Fatalf("sorted list = %d tasks, want 2", len(tasks))
	}
	if tasks[0].(map[string]interface{})["name"] != "task one" {
		t.Fatalf("first sorted task = %v, want task one", tasks[0].(map[string]interface{})["name"])
	}
}

func TestAgentTasks_ValidationAndErrors(t *testing.T) {
	repo := seedTestRepo(t, "agent-tasks-errors", false)
	repoBase := "/api/v3/agents/repos/" + repo.FullName + "/tasks"

	// Missing prompt.
	mustStatus(t, ghPost(t, repoBase, defaultToken, map[string]interface{}{}), 422, "create without prompt")

	// Unauthenticated.
	mustStatus(t, ghGet(t, "/api/v3/agents/tasks", ""), 401, "list without auth")
	mustStatus(t, ghPost(t, repoBase, "", map[string]interface{}{"prompt": "p"}), 401, "create without auth")

	// Unknown repository.
	mustStatus(t, ghGet(t, "/api/v3/agents/repos/admin/no-such-repo/tasks", defaultToken), 404, "list unknown repo")

	// Unknown task.
	mustStatus(t, ghGet(t, "/api/v3/agents/tasks/00000000-0000-0000-0000-000000000000", defaultToken), 404, "get unknown task")
	mustStatus(t, ghGet(t, repoBase+"/00000000-0000-0000-0000-000000000000", defaultToken), 404, "get unknown repo task")

	// A task from another repository is a 404 on this repo's surface.
	other := seedTestRepo(t, "agent-tasks-other", false)
	resp := ghPost(t, "/api/v3/agents/repos/"+other.FullName+"/tasks", defaultToken, map[string]interface{}{"prompt": "other"})
	if resp.StatusCode != 201 {
		t.Fatalf("create other-repo task status %d, want 201", resp.StatusCode)
	}
	otherTask := decodeJSON(t, resp)
	mustStatus(t, ghGet(t, repoBase+"/"+otherTask["id"].(string), defaultToken), 404, "cross-repo task lookup")
}
