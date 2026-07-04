package bleephub

import (
	"net/http"
	"testing"
	"time"
)

func TestEnterpriseCopilotCodingAgentPolicy(t *testing.T) {
	policy := enterpriseAPI + "/copilot/policies/coding_agent"
	orgs := policy + "/organizations"

	// Invalid policy state → 400.
	resp := ghPut(t, policy, defaultToken, map[string]interface{}{"policy_state": "sideways"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid policy: got %d, want 400", resp.StatusCode)
	}

	// Editing the organization selection before the policy is
	// enabled_for_selected_orgs → 400.
	resp = ghPut(t, policy, defaultToken, map[string]interface{}{"policy_state": "enabled_for_all_orgs"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set policy all orgs: got %d, want 204", resp.StatusCode)
	}
	resp = ghPost(t, orgs, defaultToken, map[string]interface{}{"organizations": []string{"ent-copilot-org"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("add orgs under all-orgs policy: got %d, want 400", resp.StatusCode)
	}

	// With enabled_for_selected_orgs the selection can be edited.
	createEnterpriseTestOrg(t, "ent-copilot-org")
	resp = ghPut(t, policy, defaultToken, map[string]interface{}{"policy_state": "enabled_for_selected_orgs"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set policy selected orgs: got %d, want 204", resp.StatusCode)
	}
	resp = ghPost(t, orgs, defaultToken, map[string]interface{}{"organizations": []string{"ent-copilot-org"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("add orgs: got %d, want 204", resp.StatusCode)
	}
	testServer.store.mu.RLock()
	enabled := append([]string(nil), testServer.store.EnterpriseSettings.CopilotCodingAgentOrgs...)
	testServer.store.mu.RUnlock()
	if len(enabled) != 1 || enabled[0] != "ent-copilot-org" {
		t.Fatalf("enabled orgs after add = %v, want [ent-copilot-org]", enabled)
	}

	// Remove via DELETE with a body.
	resp = ghDeleteWithBody(t, orgs, defaultToken, map[string]interface{}{"organizations": []string{"ent-copilot-org"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove orgs: got %d, want 204", resp.StatusCode)
	}
	testServer.store.mu.RLock()
	remaining := len(testServer.store.EnterpriseSettings.CopilotCodingAgentOrgs)
	testServer.store.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("enabled orgs after remove = %d, want 0", remaining)
	}

	// Non-owner → 403.
	memberTok := createEnterpriseTestUser(t, "ent-copilot-member")
	resp = ghPut(t, policy, memberTok, map[string]interface{}{"policy_state": "enabled_for_all_orgs"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner set policy: got %d, want 403", resp.StatusCode)
	}
}

func TestEnterpriseCopilotMetricsReports(t *testing.T) {
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	for _, path := range []string{
		"/copilot/metrics/reports/enterprise-1-day",
		"/copilot/metrics/reports/users-1-day",
		"/copilot/metrics/reports/user-teams-1-day",
	} {
		// day is a required parameter.
		resp := ghGet(t, enterpriseAPI+path, defaultToken)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s without day: got %d, want 422", path, resp.StatusCode)
		}
		resp = ghGet(t, enterpriseAPI+path+"?day=13-10-2025", defaultToken)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s malformed day: got %d, want 422", path, resp.StatusCode)
		}

		// A report can only exist for days that have already happened.
		future := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
		resp = ghGet(t, enterpriseAPI+path+"?day="+future, defaultToken)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s future day: got %d, want 404", path, resp.StatusCode)
		}

		// bleephub records no Copilot usage activity, so the report for any
		// past day is honestly empty in the documented shape.
		resp = ghGet(t, enterpriseAPI+path+"?day="+yesterday, defaultToken)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("%s: got %d, want 200", path, resp.StatusCode)
		}
		report := decodeJSON(t, resp)
		if report["report_day"] != yesterday {
			t.Fatalf("%s report_day = %v, want %s", path, report["report_day"], yesterday)
		}
		links, ok := report["download_links"].([]interface{})
		if !ok {
			t.Fatalf("%s download_links missing or not an array: %v", path, report["download_links"])
		}
		if len(links) != 0 {
			t.Fatalf("%s download_links = %v, want empty (no recorded Copilot activity)", path, links)
		}
	}

	for _, path := range []string{
		"/copilot/metrics/reports/enterprise-28-day/latest",
		"/copilot/metrics/reports/users-28-day/latest",
	} {
		resp := ghGet(t, enterpriseAPI+path, defaultToken)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("%s: got %d, want 200", path, resp.StatusCode)
		}
		report := decodeJSON(t, resp)
		startStr, _ := report["report_start_day"].(string)
		endStr, _ := report["report_end_day"].(string)
		start, err1 := time.Parse("2006-01-02", startStr)
		end, err2 := time.Parse("2006-01-02", endStr)
		if err1 != nil || err2 != nil {
			t.Fatalf("%s report period not dates: %v..%v", path, startStr, endStr)
		}
		// The latest report covers 28 full days ending yesterday.
		if got := end.Sub(start); got != 27*24*time.Hour {
			t.Fatalf("%s report period = %v..%v (span %v), want a 28-day window", path, startStr, endStr, got)
		}
		if endStr != yesterday {
			t.Fatalf("%s report_end_day = %v, want %s", path, endStr, yesterday)
		}
		if links, ok := report["download_links"].([]interface{}); !ok || len(links) != 0 {
			t.Fatalf("%s download_links = %v, want empty array", path, report["download_links"])
		}
	}
}
