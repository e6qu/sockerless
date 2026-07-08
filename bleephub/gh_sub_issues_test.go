package bleephub

import (
	"fmt"
	"strconv"
	"testing"
)

// createIssueForTest creates an issue and returns (databaseID, number).
func createIssueForTest(t *testing.T, repo, title string) (int, int) {
	t.Helper()
	resp := ghPost(t, "/api/v3/repos/admin/"+repo+"/issues", defaultToken, map[string]interface{}{"title": title})
	data := decodeJSONWithStatus(t, resp, 201)
	return int(data["id"].(float64)), int(data["number"].(float64))
}

func subIssueNumbers(t *testing.T, repo string, parentNumber int) []int {
	t.Helper()
	resp := ghGet(t, fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d/sub_issues", repo, parentNumber), defaultToken)
	list := decodeJSONWithStatus2xxArray(t, resp, 200)
	out := make([]int, 0, len(list))
	for _, item := range list {
		out = append(out, int(item["number"].(float64)))
	}
	return out
}

func TestSubIssues_AddListReprioritizeRemove(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	parentID, parentNum := createIssueForTest(t, repo, "parent")
	aID, aNum := createIssueForTest(t, repo, "child a")
	bID, bNum := createIssueForTest(t, repo, "child b")
	cID, cNum := createIssueForTest(t, repo, "child c")
	parentPath := fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d", repo, parentNum)

	for _, childID := range []int{aID, bID, cID} {
		resp := ghPost(t, parentPath+"/sub_issues", defaultToken, map[string]interface{}{"sub_issue_id": childID})
		created := decodeJSONWithStatus(t, resp, 201)
		if int(created["number"].(float64)) != parentNum {
			t.Fatalf("add sub-issue returned issue #%v, want parent #%d", created["number"], parentNum)
		}
	}

	if got := subIssueNumbers(t, repo, parentNum); len(got) != 3 || got[0] != aNum || got[1] != bNum || got[2] != cNum {
		t.Fatalf("sub-issues = %v, want [%d %d %d]", got, aNum, bNum, cNum)
	}

	// An issue may not be its own sub-issue; a linked child may not be
	// re-added; a child of one parent may not join another without
	// replace_parent.
	resp := ghPost(t, parentPath+"/sub_issues", defaultToken, map[string]interface{}{"sub_issue_id": parentID})
	requireStatus(t, resp, 422)
	resp = ghPost(t, parentPath+"/sub_issues", defaultToken, map[string]interface{}{"sub_issue_id": aID})
	requireStatus(t, resp, 422)
	childPath := fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d", repo, aNum)
	resp = ghPost(t, childPath+"/sub_issues", defaultToken, map[string]interface{}{"sub_issue_id": parentID})
	requireStatus(t, resp, 422) // cycle

	// Reprioritize: move c before a → [c a b].
	resp = ghPatch(t, parentPath+"/sub_issues/priority", defaultToken, map[string]interface{}{
		"sub_issue_id": cID, "before_id": aID,
	})
	reordered := decodeJSONWithStatus(t, resp, 200)
	if int(reordered["number"].(float64)) != parentNum {
		t.Fatalf("priority PATCH returned issue #%v, want parent #%d", reordered["number"], parentNum)
	}
	if got := subIssueNumbers(t, repo, parentNum); got[0] != cNum || got[1] != aNum || got[2] != bNum {
		t.Fatalf("after before_id: %v, want [%d %d %d]", got, cNum, aNum, bNum)
	}

	// Move a after b → [c b a].
	resp = ghPatch(t, parentPath+"/sub_issues/priority", defaultToken, map[string]interface{}{
		"sub_issue_id": aID, "after_id": bID,
	})
	requireStatus(t, resp, 200)
	if got := subIssueNumbers(t, repo, parentNum); got[0] != cNum || got[1] != bNum || got[2] != aNum {
		t.Fatalf("after after_id: %v, want [%d %d %d]", got, cNum, bNum, aNum)
	}

	// Both positional arguments at once is invalid; so is a non-child.
	resp = ghPatch(t, parentPath+"/sub_issues/priority", defaultToken, map[string]interface{}{
		"sub_issue_id": aID, "after_id": bID, "before_id": cID,
	})
	requireStatus(t, resp, 422)
	otherID, _ := createIssueForTest(t, repo, "not a child")
	resp = ghPatch(t, parentPath+"/sub_issues/priority", defaultToken, map[string]interface{}{"sub_issue_id": otherID})
	requireStatus(t, resp, 422)

	// Remove a child: the response is the removed sub-issue.
	resp = ghDeleteWithBody(t, parentPath+"/sub_issue", defaultToken, map[string]interface{}{"sub_issue_id": bID})
	removed := decodeJSONWithStatus(t, resp, 200)
	if int(removed["number"].(float64)) != bNum {
		t.Fatalf("removed sub-issue #%v, want #%d", removed["number"], bNum)
	}
	if got := subIssueNumbers(t, repo, parentNum); len(got) != 2 {
		t.Fatalf("after removal: %v, want 2 entries", got)
	}
	resp = ghDeleteWithBody(t, parentPath+"/sub_issue", defaultToken, map[string]interface{}{"sub_issue_id": bID})
	requireStatus(t, resp, 422)
}

func TestSubIssues_ReplaceParentPersistsOldParent(t *testing.T) {
	var firstParentID, secondParentID, childID int
	st2 := reloadedStore(t, func(_ *Persistence, st *Store) {
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		repo := st.CreateRepo(admin, "subissue-replace-reload", "", false)
		firstParent := st.CreateIssue(repo.ID, admin.ID, "first parent", "", nil, nil, 0)
		secondParent := st.CreateIssue(repo.ID, admin.ID, "second parent", "", nil, nil, 0)
		child := st.CreateIssue(repo.ID, admin.ID, "child", "", nil, nil, 0)
		firstParentID, secondParentID, childID = firstParent.ID, secondParent.ID, child.ID
		if err := st.AddSubIssue(firstParentID, childID, false); err != nil {
			t.Fatalf("AddSubIssue first parent: %v", err)
		}
		if err := st.AddSubIssue(secondParentID, childID, true); err != nil {
			t.Fatalf("AddSubIssue replace parent: %v", err)
		}
	})

	if got := st2.ListSubIssues(firstParentID); len(got) != 0 {
		t.Fatalf("first parent sub-issues after reload = %v, want empty", got)
	}
	if got := st2.ListSubIssues(secondParentID); len(got) != 1 || got[0] != childID {
		t.Fatalf("second parent sub-issues after reload = %v, want [%d]", got, childID)
	}
}

func TestIssueDependencies_BlockedBy(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	blockedID, blockedNum := createIssueForTest(t, repo, "blocked issue")
	blockerID, blockerNum := createIssueForTest(t, repo, "blocking issue")
	blockedPath := fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d", repo, blockedNum)

	resp := ghGet(t, blockedPath+"/dependencies/blocked_by", defaultToken)
	if list := decodeJSONWithStatus2xxArray(t, resp, 200); len(list) != 0 {
		t.Fatalf("initial blocked_by = %v, want empty", list)
	}

	resp = ghPost(t, blockedPath+"/dependencies/blocked_by", defaultToken, map[string]interface{}{"issue_id": blockerID})
	created := decodeJSONWithStatus(t, resp, 201)
	if int(created["number"].(float64)) != blockerNum {
		t.Fatalf("blocked_by POST returned #%v, want the blocking issue #%d", created["number"], blockerNum)
	}

	resp = ghGet(t, blockedPath+"/dependencies/blocked_by", defaultToken)
	list := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(list) != 1 || int(list[0]["number"].(float64)) != blockerNum {
		t.Fatalf("blocked_by list = %v, want [#%d]", list, blockerNum)
	}

	// The link is bidirectional: the blocker sees the blocked issue on its
	// blocking side.
	if blocking := testServer.store.ListIssueBlocking(blockerID); len(blocking) != 1 || blocking[0] != blockedID {
		t.Fatalf("blocking view = %v, want [%d]", blocking, blockedID)
	}

	// Duplicates and self-links are rejected.
	resp = ghPost(t, blockedPath+"/dependencies/blocked_by", defaultToken, map[string]interface{}{"issue_id": blockerID})
	requireStatus(t, resp, 422)
	resp = ghPost(t, blockedPath+"/dependencies/blocked_by", defaultToken, map[string]interface{}{"issue_id": blockedID})
	requireStatus(t, resp, 422)
	resp = ghPost(t, blockedPath+"/dependencies/blocked_by", defaultToken, map[string]interface{}{"issue_id": 999999})
	requireStatus(t, resp, 404)

	// Removal returns the unlinked blocking issue and clears both sides.
	resp = ghDelete(t, blockedPath+"/dependencies/blocked_by/"+strconv.Itoa(blockerID), defaultToken)
	removed := decodeJSONWithStatus(t, resp, 200)
	if int(removed["number"].(float64)) != blockerNum {
		t.Fatalf("removed dependency #%v, want #%d", removed["number"], blockerNum)
	}
	resp = ghGet(t, blockedPath+"/dependencies/blocked_by", defaultToken)
	if list := decodeJSONWithStatus2xxArray(t, resp, 200); len(list) != 0 {
		t.Fatalf("blocked_by after removal = %v, want empty", list)
	}
	if blocking := testServer.store.ListIssueBlocking(blockerID); len(blocking) != 0 {
		t.Fatalf("blocking view after removal = %v, want empty", blocking)
	}
	resp = ghDelete(t, blockedPath+"/dependencies/blocked_by/"+strconv.Itoa(blockerID), defaultToken)
	requireStatus(t, resp, 404)
}
