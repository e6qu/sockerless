package bleephub

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestWorkflowRunsListNewestFirst guards BUG-1607: the run-list endpoints
// paginated an unsorted map iteration, so order was unstable across requests
// and never GitHub's newest-first. The response must be sorted by run ID
// descending and stable across repeated calls.
func TestWorkflowRunsListNewestFirst(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	for i := 0; i < 6; i++ {
		seedRun(t, s, "octo/repo", "completed", "success")
	}

	var firstOrder []float64
	for attempt := 0; attempt < 3; attempt++ {
		w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runs")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		var resp struct {
			WorkflowRuns []map[string]any `json:"workflow_runs"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		ids := make([]float64, len(resp.WorkflowRuns))
		for i, r := range resp.WorkflowRuns {
			ids[i] = r["id"].(float64)
		}
		// Newest-first: strictly descending.
		for i := 1; i < len(ids); i++ {
			if ids[i] >= ids[i-1] {
				t.Errorf("runs not sorted newest-first: %v", ids)
				break
			}
		}
		if attempt == 0 {
			firstOrder = ids
		} else {
			for i := range ids {
				if ids[i] != firstOrder[i] {
					t.Errorf("run order unstable across requests: %v vs %v", firstOrder, ids)
					break
				}
			}
		}
	}
}

// TestReleaseListNewestFirst guards BUG-1615: releases were returned in
// insertion (oldest-first) order. Real GitHub lists newest-first.
func TestReleaseListNewestFirst(t *testing.T) {
	s := newTestServer()
	rs := s.store.Releases
	r1 := rs.Create(1, 1, "v1.0.0", "main", "v1", "", false, false)
	r2 := rs.Create(1, 1, "v1.1.0", "main", "v1.1", "", false, false)
	r3 := rs.Create(1, 1, "v2.0.0", "main", "v2", "", false, false)

	list := rs.List(1)
	if len(list) != 3 {
		t.Fatalf("list len = %d, want 3", len(list))
	}
	if list[0].ID != r3.ID || list[1].ID != r2.ID || list[2].ID != r1.ID {
		t.Errorf("releases not newest-first: got %d,%d,%d want %d,%d,%d",
			list[0].ID, list[1].ID, list[2].ID, r3.ID, r2.ID, r1.ID)
	}
}
