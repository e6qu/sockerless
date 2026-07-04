package bleephub

import (
	"testing"
)

// publishTestAdvisory creates a repository security advisory through the real
// REST flow (create draft, then publish) and returns its GHSA ID.
func publishTestAdvisory(t *testing.T, repoKey, summary, severity string) string {
	t.Helper()
	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/security-advisories", defaultToken, map[string]interface{}{
		"summary":                  summary,
		"description":              "global advisory test description",
		"severity":                 severity,
		"cwe_ids":                  []string{"CWE-79"},
		"vulnerable_version_range": "< 1.2.3",
	})
	adv := decodeJSONWithStatus(t, resp, 201)
	ghsaID, _ := adv["ghsa_id"].(string)
	if ghsaID == "" {
		t.Fatalf("no ghsa_id in create response: %v", adv)
	}
	patch := ghPatch(t, "/api/v3/repos/"+repoKey+"/security-advisories/"+ghsaID, defaultToken, map[string]interface{}{
		"state": "published",
	})
	decodeJSONWithStatus(t, patch, 200)
	return ghsaID
}

func TestGlobalSecurityAdvisories_ListAndGet(t *testing.T) {
	repoKey := createTestRepo(t)
	ghsaID := publishTestAdvisory(t, repoKey, "global listing target", "high")

	resp := ghGet(t, "/api/v3/advisories", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	list := decodeJSONArray(t, resp)
	var found map[string]interface{}
	for _, a := range list {
		if a["ghsa_id"] == ghsaID {
			found = a
		}
	}
	if found == nil {
		t.Fatalf("published advisory %s not in global list", ghsaID)
	}
	if found["type"] != "reviewed" || found["severity"] != "high" {
		t.Fatalf("advisory shape: type=%v severity=%v", found["type"], found["severity"])
	}
	if found["url"] != testBaseURL+"/api/v3/advisories/"+ghsaID {
		t.Fatalf("url = %v", found["url"])
	}
	if found["published_at"] == nil || found["github_reviewed_at"] == nil {
		t.Fatal("published_at/github_reviewed_at missing")
	}

	resp = ghGet(t, "/api/v3/advisories/"+ghsaID, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get status = %d", resp.StatusCode)
	}
	single := decodeJSON(t, resp)
	if single["ghsa_id"] != ghsaID {
		t.Fatalf("get returned %v", single["ghsa_id"])
	}

	resp = ghGet(t, "/api/v3/advisories/GHSA-0000-0000-0000", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown advisory status = %d, want 404", resp.StatusCode)
	}
}

func TestGlobalSecurityAdvisories_DraftExcludedAndFilters(t *testing.T) {
	repoKey := createTestRepo(t)

	// A draft advisory must never surface in the global database view.
	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/security-advisories", defaultToken, map[string]interface{}{
		"summary":  "draft stays hidden",
		"severity": "low",
	})
	draft := decodeJSONWithStatus(t, resp, 201)
	draftID, _ := draft["ghsa_id"].(string)

	published := publishTestAdvisory(t, repoKey, "filterable advisory", "critical")

	resp = ghGet(t, "/api/v3/advisories", defaultToken)
	list := decodeJSONArray(t, resp)
	for _, a := range list {
		if a["ghsa_id"] == draftID {
			t.Fatal("draft advisory leaked into the global list")
		}
	}

	// severity filter
	resp = ghGet(t, "/api/v3/advisories?severity=critical", defaultToken)
	list = decodeJSONArray(t, resp)
	foundPublished := false
	for _, a := range list {
		if a["severity"] != "critical" {
			t.Fatalf("severity filter leaked %v", a["severity"])
		}
		if a["ghsa_id"] == published {
			foundPublished = true
		}
	}
	if !foundPublished {
		t.Fatal("critical advisory missing from severity-filtered list")
	}

	// ghsa_id filter
	resp = ghGet(t, "/api/v3/advisories?ghsa_id="+published, defaultToken)
	list = decodeJSONArray(t, resp)
	if len(list) != 1 || list[0]["ghsa_id"] != published {
		t.Fatalf("ghsa_id filter returned %d rows", len(list))
	}

	// The global database contains only reviewed advisories, so the
	// malware/unreviewed types match nothing.
	resp = ghGet(t, "/api/v3/advisories?type=malware", defaultToken)
	list = decodeJSONArray(t, resp)
	if len(list) != 0 {
		t.Fatalf("type=malware returned %d rows, want 0", len(list))
	}

	// cwes filter
	resp = ghGet(t, "/api/v3/advisories?cwes=CWE-79&ghsa_id="+published, defaultToken)
	list = decodeJSONArray(t, resp)
	if len(list) != 1 {
		t.Fatalf("cwes filter returned %d rows, want 1", len(list))
	}
}
