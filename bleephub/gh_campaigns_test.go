package bleephub

import (
	"testing"
	"time"
)

func TestOrgCampaigns_CRUD(t *testing.T) {
	org := createTestOrg(t)
	repoName, repoID := createOrgRepoForGovernance(t, org)

	alert := seedCodeScanningAlert(t, org, repoName, "campaign-rule", "error", "CodeQL")
	alertNumber := int(alert["number"].(float64))
	endsAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)

	// Create.
	resp := ghPost(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken, map[string]interface{}{
		"name":        "Q4 code scanning cleanup",
		"description": "Fix the open CodeQL alerts before the freeze",
		"ends_at":     endsAt,
		"code_scanning_alerts": []map[string]interface{}{
			{"repository_id": repoID, "alert_numbers": []int{alertNumber}},
		},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("create campaign: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["name"] != "Q4 code scanning cleanup" || created["state"] != "open" {
		t.Fatalf("created = %v", created)
	}
	if created["closed_at"] != nil {
		t.Fatalf("closed_at on open campaign = %v", created["closed_at"])
	}
	// The creator becomes the manager when none are named.
	managers := created["managers"].([]interface{})
	if len(managers) != 1 || managers[0].(map[string]interface{})["login"] != "admin" {
		t.Fatalf("managers = %v", managers)
	}
	stats := created["alert_stats"].(map[string]interface{})
	if stats["open_count"].(float64) != 1 || stats["closed_count"].(float64) != 0 {
		t.Fatalf("alert_stats = %v", stats)
	}
	number := itoa(int(created["number"].(float64)))

	// GET one.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/campaigns/"+number, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get campaign: %d", resp.StatusCode)
	}
	if got := decodeJSON(t, resp); got["description"] != "Fix the open CodeQL alerts before the freeze" {
		t.Fatalf("get = %v", got)
	}

	// List.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list campaigns: %d", resp.StatusCode)
	}
	if list := decodeJSONArray(t, resp); len(list) != 1 {
		t.Fatalf("list = %v", list)
	}

	// Dismissing the linked alert moves it to the closed count.
	patchAlert := ghPatch(t, "/api/v3/repos/"+org+"/"+repoName+"/code-scanning/alerts/"+itoa(alertNumber), defaultToken, map[string]interface{}{
		"state":            "dismissed",
		"dismissed_reason": "won't_fix",
	})
	patchAlert.Body.Close()
	if patchAlert.StatusCode != 200 {
		t.Fatalf("dismiss alert: %d", patchAlert.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/campaigns/"+number, defaultToken)
	stats = decodeJSON(t, resp)["alert_stats"].(map[string]interface{})
	if stats["open_count"].(float64) != 0 || stats["closed_count"].(float64) != 1 {
		t.Fatalf("alert_stats after dismissal = %v", stats)
	}

	// PATCH: close the campaign.
	resp = ghPatch(t, "/api/v3/orgs/"+org+"/campaigns/"+number, defaultToken, map[string]interface{}{
		"state": "closed",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("close campaign: %d", resp.StatusCode)
	}
	closed := decodeJSON(t, resp)
	if closed["state"] != "closed" || closed["closed_at"] == nil {
		t.Fatalf("closed = %v", closed)
	}

	// State filter.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/campaigns?state=open", defaultToken)
	if got := decodeJSONArray(t, resp); len(got) != 0 {
		t.Fatalf("open campaigns = %v", got)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/campaigns?state=closed", defaultToken)
	if got := decodeJSONArray(t, resp); len(got) != 1 {
		t.Fatalf("closed campaigns = %v", got)
	}

	// DELETE.
	resp = ghDelete(t, "/api/v3/orgs/"+org+"/campaigns/"+number, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete campaign: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/campaigns/"+number, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("get deleted campaign: %d", resp.StatusCode)
	}
}

func TestOrgCampaigns_Validation(t *testing.T) {
	org := createTestOrg(t)
	repoName, repoID := createOrgRepoForGovernance(t, org)
	alert := seedCodeScanningAlert(t, org, repoName, "v-rule", "warning", "CodeQL")
	alertNumber := int(alert["number"].(float64))
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	alerts := []map[string]interface{}{
		{"repository_id": repoID, "alert_numbers": []int{alertNumber}},
	}

	// ends_at in the past.
	resp := ghPost(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken, map[string]interface{}{
		"name":                 "past",
		"description":          "d",
		"ends_at":              time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"code_scanning_alerts": alerts,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("past ends_at: %d", resp.StatusCode)
	}

	// Missing alert linkage.
	resp = ghPost(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken, map[string]interface{}{
		"name": "no alerts", "description": "d", "ends_at": future,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing alerts: %d", resp.StatusCode)
	}

	// Unknown alert number.
	resp = ghPost(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken, map[string]interface{}{
		"name": "bad alert", "description": "d", "ends_at": future,
		"code_scanning_alerts": []map[string]interface{}{
			{"repository_id": repoID, "alert_numbers": []int{987654}},
		},
	})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown alert: %d", resp.StatusCode)
	}

	// A manager who is not an org member.
	outsider := createTestUser(t, "campaign-outsider-"+org)
	_ = outsider
	resp = ghPost(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken, map[string]interface{}{
		"name": "bad manager", "description": "d", "ends_at": future,
		"managers":             []string{"campaign-outsider-" + org},
		"code_scanning_alerts": alerts,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("outsider manager: %d", resp.StatusCode)
	}

	// A name longer than 50 characters.
	resp = ghPost(t, "/api/v3/orgs/"+org+"/campaigns", defaultToken, map[string]interface{}{
		"name":                 "0123456789012345678901234567890123456789012345678901",
		"description":          "d",
		"ends_at":              future,
		"code_scanning_alerts": alerts,
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("long name: %d", resp.StatusCode)
	}
}
