package bleephub

import (
	"testing"
	"time"
)

func TestRepoVulnerabilityAlerts_CheckToggle(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	path := "/api/v3/repos/admin/" + repo + "/vulnerability-alerts"

	resp := ghGet(t, path, defaultToken)
	requireStatus(t, resp, 404)

	resp = ghPut(t, path, defaultToken, nil)
	requireStatus(t, resp, 204)
	resp = ghGet(t, path, defaultToken)
	requireStatus(t, resp, 204)

	resp = ghDelete(t, path, defaultToken)
	requireStatus(t, resp, 204)
	resp = ghGet(t, path, defaultToken)
	requireStatus(t, resp, 404)
}

func TestRepoAutomatedSecurityFixes_Check(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	path := "/api/v3/repos/admin/" + repo + "/automated-security-fixes"

	resp := ghGet(t, path, defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != false || data["paused"] != false {
		t.Fatalf("initial state = %v, want enabled false paused false", data)
	}

	resp = ghPut(t, path, defaultToken, nil)
	requireStatus(t, resp, 204)
	resp = ghGet(t, path, defaultToken)
	data = decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != true {
		t.Fatalf("enabled = %v after PUT, want true", data["enabled"])
	}
}

func TestRepoPrivateVulnerabilityReporting_Check(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	path := "/api/v3/repos/admin/" + repo + "/private-vulnerability-reporting"

	resp := ghGet(t, path, defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != false {
		t.Fatalf("initial enabled = %v, want false", data["enabled"])
	}

	resp = ghPut(t, path, defaultToken, nil)
	requireStatus(t, resp, 204)
	resp = ghGet(t, path, defaultToken)
	data = decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != true {
		t.Fatalf("enabled = %v after PUT, want true", data["enabled"])
	}
}

func TestRepoInteractionLimits_RoundTrip(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	path := "/api/v3/repos/admin/" + repo + "/interaction-limits"

	// No restriction in effect → empty object.
	resp := ghGet(t, path, defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	if len(data) != 0 {
		t.Fatalf("initial interaction limits = %v, want {}", data)
	}

	resp = ghPut(t, path, defaultToken, map[string]interface{}{
		"limit":  "collaborators_only",
		"expiry": "one_week",
	})
	set := decodeJSONWithStatus(t, resp, 200)
	if set["limit"] != "collaborators_only" || set["origin"] != "repository" {
		t.Fatalf("set response = %v", set)
	}
	expiresAt, err := time.Parse(time.RFC3339, set["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at unparsable: %v", err)
	}
	want := time.Now().Add(7 * 24 * time.Hour)
	if diff := expiresAt.Sub(want); diff < -time.Minute || diff > time.Minute {
		t.Fatalf("expires_at = %v, want ~one week out", expiresAt)
	}

	resp = ghGet(t, path, defaultToken)
	got := decodeJSONWithStatus(t, resp, 200)
	if got["limit"] != "collaborators_only" || got["origin"] != "repository" {
		t.Fatalf("read-back = %v", got)
	}

	// Invalid enum values are validation failures.
	resp = ghPut(t, path, defaultToken, map[string]interface{}{"limit": "everyone"})
	requireStatus(t, resp, 422)
	resp = ghPut(t, path, defaultToken, map[string]interface{}{"limit": "existing_users", "expiry": "forever"})
	requireStatus(t, resp, 422)

	resp = ghDelete(t, path, defaultToken)
	requireStatus(t, resp, 204)
	resp = ghGet(t, path, defaultToken)
	data = decodeJSONWithStatus(t, resp, 200)
	if len(data) != 0 {
		t.Fatalf("after DELETE = %v, want {}", data)
	}
}
