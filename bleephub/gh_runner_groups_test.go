package bleephub

import (
	"fmt"
	"io"
	"testing"
)

func registerRunnerForLabels(t *testing.T) int {
	t.Helper()
	body := map[string]interface{}{
		"name": "label-runner",
		"labels": []map[string]string{
			{"name": "self-hosted", "type": "system"},
			{"name": "linux", "type": "system"},
		},
	}
	resp := ghPost(t, "/_apis/v1/Agent/1", defaultToken, body)
	agent := decodeJSONWithStatus(t, resp, 200)
	return int(agent["id"].(float64))
}

func TestRunnerLabels_Repo_ListSetDelete(t *testing.T) {
	agentID := registerRunnerForLabels(t)
	repo := createTestRepo(t)

	// List labels: should include the two system labels.
	listResp := ghGet(t, fmt.Sprintf("/api/v3/repos/%s/actions/runners/%d/labels", repo, agentID), defaultToken)
	listData := decodeJSONWithStatus(t, listResp, 200)
	if listData["total_count"].(float64) != 2 {
		t.Fatalf("initial labels count = %v, want 2", listData["total_count"])
	}

	// Set custom labels.
	setResp := ghPut(t, fmt.Sprintf("/api/v3/repos/%s/actions/runners/%d/labels", repo, agentID), defaultToken, map[string]interface{}{
		"labels": []string{"gpu", "arm64"},
	})
	setData := decodeJSONWithStatus(t, setResp, 200)
	if setData["total_count"].(float64) != 4 {
		t.Fatalf("labels after set = %v, want 4", setData["total_count"])
	}

	// Delete all custom labels; system labels remain.
	delResp := ghDelete(t, fmt.Sprintf("/api/v3/repos/%s/actions/runners/%d/labels", repo, agentID), defaultToken)
	delData := decodeJSONWithStatus(t, delResp, 200)
	if delData["total_count"].(float64) != 2 {
		t.Fatalf("labels after delete = %v, want 2", delData["total_count"])
	}
	labels, _ := delData["labels"].([]interface{})
	for _, l := range labels {
		lm := l.(map[string]interface{})
		if lm["type"] != "read-only" {
			t.Errorf("expected only read-only labels after delete, got %v", lm)
		}
	}
}

func TestRunnerLabels_Org_ListSet(t *testing.T) {
	agentID := registerRunnerForLabels(t)
	org := createTestOrg(t)

	listResp := ghGet(t, fmt.Sprintf("/api/v3/orgs/%s/actions/runners/%d/labels", org, agentID), defaultToken)
	listData := decodeJSONWithStatus(t, listResp, 200)
	if listData["total_count"].(float64) != 2 {
		t.Fatalf("initial org labels count = %v, want 2", listData["total_count"])
	}

	setResp := ghPut(t, fmt.Sprintf("/api/v3/orgs/%s/actions/runners/%d/labels", org, agentID), defaultToken, map[string]interface{}{
		"labels": []string{"builder"},
	})
	setData := decodeJSONWithStatus(t, setResp, 200)
	if setData["total_count"].(float64) != 3 {
		t.Fatalf("org labels after set = %v, want 3", setData["total_count"])
	}
}

func TestRunnerLabels_Org_UnknownOrg(t *testing.T) {
	agentID := registerRunnerForLabels(t)
	listResp := ghGet(t, fmt.Sprintf("/api/v3/orgs/no-such-org-999/actions/runners/%d/labels", agentID), defaultToken)
	if listResp.StatusCode != 404 {
		body, _ := io.ReadAll(listResp.Body)
		listResp.Body.Close()
		t.Fatalf("expected 404 for unknown org, got %d: %s", listResp.StatusCode, body)
	}
	listResp.Body.Close()
}

func TestRunnerLabels_Repo_UnknownRunner(t *testing.T) {
	repo := createTestRepo(t)
	listResp := ghGet(t, fmt.Sprintf("/api/v3/repos/%s/actions/runners/999999/labels", repo), defaultToken)
	if listResp.StatusCode != 404 {
		body, _ := io.ReadAll(listResp.Body)
		listResp.Body.Close()
		t.Fatalf("expected 404 for unknown runner, got %d: %s", listResp.StatusCode, body)
	}
	listResp.Body.Close()
}
