package bleephub

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestAPIInsights_StatsFromObservedTraffic drives real REST traffic with a
// dedicated member's classic personal access token and asserts every API
// insights aggregation reports it.
func TestAPIInsights_StatsFromObservedTraffic(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "insights-org", "Insights Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	member := createTestUser(t, "insights-member")
	testServer.store.SetMembership(org.Login, member.ID, OrgRoleMember, MembershipStateActive)
	memberToken := "ghp_insights_member_token"
	testServer.store.Tokens[memberToken] = &Token{Value: memberToken, UserID: member.ID}

	minTS := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)

	// Real observed traffic: three authenticated requests by the member.
	for _, path := range []string{"/api/v3/user", "/api/v3/user", "/api/v3/users/insights-member"} {
		resp := ghGet(t, path, memberToken)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("member request %s: %d", path, resp.StatusCode)
		}
	}

	window := "min_timestamp=" + url.QueryEscape(minTS)
	actorPath := fmt.Sprintf("classic_pat/%d", member.ID)

	// Route stats by actor.
	resp := ghGet(t, "/api/v3/orgs/insights-org/insights/api/route-stats/"+actorPath+"?"+window, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("route-stats: %d", resp.StatusCode)
	}
	routeRows := decodeJSONArray(t, resp)
	var userRoute map[string]interface{}
	for _, row := range routeRows {
		if row["api_route"] == "/user" && row["http_method"] == "GET" {
			userRoute = row
		}
	}
	if userRoute == nil {
		t.Fatalf("GET /user route missing from route-stats: %v", routeRows)
	}
	if userRoute["total_request_count"] != float64(2) {
		t.Fatalf("GET /user total_request_count = %v, want 2", userRoute["total_request_count"])
	}
	if userRoute["rate_limited_request_count"] != float64(0) || userRoute["last_rate_limited_timestamp"] != nil {
		t.Fatalf("rate-limit fields wrong: %v", userRoute)
	}
	if ts, _ := userRoute["last_request_timestamp"].(string); ts == "" {
		t.Fatalf("last_request_timestamp missing: %v", userRoute)
	}

	// Subject stats.
	resp = ghGet(t, "/api/v3/orgs/insights-org/insights/api/subject-stats?"+window+"&subject_name_substring=insights-member", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("subject-stats: %d", resp.StatusCode)
	}
	subjectRows := decodeJSONArray(t, resp)
	if len(subjectRows) != 1 {
		t.Fatalf("expected exactly the member subject, got %v", subjectRows)
	}
	if subjectRows[0]["subject_type"] != "user" || subjectRows[0]["subject_id"] != float64(member.ID) || subjectRows[0]["total_request_count"] != float64(3) {
		t.Fatalf("member subject row wrong: %v", subjectRows[0])
	}

	// Summary stats (whole org includes the admin's own insights queries, so
	// only the lower bound is fixed).
	resp = ghGet(t, "/api/v3/orgs/insights-org/insights/api/summary-stats?"+window, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("summary-stats: %d", resp.StatusCode)
	}
	summary := decodeJSON(t, resp)
	if total, _ := summary["total_request_count"].(float64); total < 3 {
		t.Fatalf("summary total_request_count = %v, want >= 3", summary["total_request_count"])
	}

	// Summary stats scoped to the member (by user and by actor): exactly the
	// three requests made with the member's token.
	for _, path := range []string{
		fmt.Sprintf("/api/v3/orgs/insights-org/insights/api/summary-stats/users/%d?%s", member.ID, window),
		"/api/v3/orgs/insights-org/insights/api/summary-stats/" + actorPath + "?" + window,
	} {
		resp = ghGet(t, path, defaultToken)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
		scoped := decodeJSON(t, resp)
		if scoped["total_request_count"] != float64(3) || scoped["rate_limited_request_count"] != float64(0) {
			t.Fatalf("%s = %v, want 3 total / 0 rate limited", path, scoped)
		}
	}

	// Time stats: a 1h increment covers the whole window in one bucket.
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/insights-org/insights/api/time-stats/users/%d?%s&timestamp_increment=1h", member.ID, window), defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("time-stats by user: %d", resp.StatusCode)
	}
	buckets := decodeJSONArray(t, resp)
	var bucketTotal float64
	for _, b := range buckets {
		if ts, _ := b["timestamp"].(string); ts == "" {
			t.Fatalf("time-stats bucket missing timestamp: %v", b)
		}
		total, _ := b["total_request_count"].(float64)
		bucketTotal += total
	}
	if bucketTotal != 3 {
		t.Fatalf("time-stats by user total = %v, want 3", bucketTotal)
	}

	resp = ghGet(t, "/api/v3/orgs/insights-org/insights/api/time-stats?"+window+"&timestamp_increment=1h", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("time-stats: %d", resp.StatusCode)
	}
	decodeJSONArray(t, resp)

	resp = ghGet(t, "/api/v3/orgs/insights-org/insights/api/time-stats/"+actorPath+"?"+window+"&timestamp_increment=1h", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("time-stats by actor: %d", resp.StatusCode)
	}
	actorBuckets := decodeJSONArray(t, resp)
	bucketTotal = 0
	for _, b := range actorBuckets {
		total, _ := b["total_request_count"].(float64)
		bucketTotal += total
	}
	if bucketTotal != 3 {
		t.Fatalf("time-stats by actor total = %v, want 3", bucketTotal)
	}

	// User stats: the member's traffic groups under one classic PAT actor.
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/insights-org/insights/api/user-stats/%d?%s", member.ID, window), defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("user-stats: %d", resp.StatusCode)
	}
	actorRows := decodeJSONArray(t, resp)
	if len(actorRows) != 1 {
		t.Fatalf("expected one actor row, got %v", actorRows)
	}
	if actorRows[0]["actor_type"] != "classic_pat" || actorRows[0]["actor_id"] != float64(member.ID) || actorRows[0]["actor_name"] != "insights-member" {
		t.Fatalf("actor row wrong: %v", actorRows[0])
	}
	if actorRows[0]["total_request_count"] != float64(3) {
		t.Fatalf("actor total = %v, want 3", actorRows[0]["total_request_count"])
	}
}

func TestAPIInsights_ParameterValidation(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	if testServer.store.CreateOrg(admin, "insights-val-org", "Insights Val Org", "") == nil {
		t.Fatal("create org failed")
	}

	// min_timestamp is required.
	resp := ghGet(t, "/api/v3/orgs/insights-val-org/insights/api/summary-stats", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing min_timestamp: %d, want 422", resp.StatusCode)
	}

	// timestamp_increment is required for time stats.
	minTS := url.QueryEscape(time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
	resp = ghGet(t, "/api/v3/orgs/insights-val-org/insights/api/time-stats?min_timestamp="+minTS, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing timestamp_increment: %d, want 422", resp.StatusCode)
	}

	// Unknown actor type is not found.
	resp = ghGet(t, "/api/v3/orgs/insights-val-org/insights/api/route-stats/space_probe/1?min_timestamp="+minTS, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown actor type: %d, want 404", resp.StatusCode)
	}

	// Non-admin caller is forbidden.
	outsider := createTestUser(t, "insights-outsider")
	testServer.store.Tokens["ghp_insights_outsider"] = &Token{Value: "ghp_insights_outsider", UserID: outsider.ID}
	resp = ghGet(t, "/api/v3/orgs/insights-val-org/insights/api/summary-stats?min_timestamp="+minTS, "ghp_insights_outsider")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin summary-stats: %d, want 403", resp.StatusCode)
	}
}
