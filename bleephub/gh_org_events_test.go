package bleephub

import (
	"net/http"
	"testing"
)

func TestOrgEvents_FeedFromRecordedActivity(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "events-org", "Events Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	repo := testServer.store.CreateOrgRepo(org, admin, "events-repo", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}

	// Honest empty feed before any activity.
	resp := ghGet(t, "/api/v3/orgs/events-org/events", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list org events (empty): %d", resp.StatusCode)
	}
	if events := decodeJSONArray(t, resp); len(events) != 0 {
		t.Fatalf("expected empty event feed, got %v", events)
	}

	// Real activity: an issue and a comment on it.
	resp = ghPost(t, "/api/v3/repos/events-org/events-repo/issues", defaultToken, map[string]interface{}{
		"title": "events feed issue",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create issue: %d", resp.StatusCode)
	}
	issue := decodeJSON(t, resp)
	issueNumber := int(issue["number"].(float64))

	resp = ghPost(t, "/api/v3/repos/events-org/events-repo/issues/1/comments", defaultToken, map[string]interface{}{
		"body": "commenting for the feed",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create comment: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = ghGet(t, "/api/v3/orgs/events-org/events", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list org events: %d", resp.StatusCode)
	}
	events := decodeJSONArray(t, resp)
	if len(events) != 2 {
		t.Fatalf("expected 2 events (issue opened + comment), got %v", events)
	}

	byType := map[string]map[string]interface{}{}
	for _, ev := range events {
		typ, _ := ev["type"].(string)
		byType[typ] = ev
		if ev["public"] != true {
			t.Fatalf("event not public: %v", ev)
		}
		if id, _ := ev["id"].(string); id == "" {
			t.Fatalf("event missing id: %v", ev)
		}
		if ts, _ := ev["created_at"].(string); ts == "" {
			t.Fatalf("event missing created_at: %v", ev)
		}
		actor, _ := ev["actor"].(map[string]interface{})
		if actor == nil || actor["login"] != "admin" {
			t.Fatalf("event actor wrong: %v", ev)
		}
		repoJSON, _ := ev["repo"].(map[string]interface{})
		if repoJSON == nil || repoJSON["name"] != "events-org/events-repo" {
			t.Fatalf("event repo wrong: %v", ev)
		}
		orgJSON, _ := ev["org"].(map[string]interface{})
		if orgJSON == nil || orgJSON["login"] != "events-org" {
			t.Fatalf("event org wrong: %v", ev)
		}
	}
	issuesEvent := byType["IssuesEvent"]
	if issuesEvent == nil {
		t.Fatalf("IssuesEvent missing: %v", events)
	}
	payload, _ := issuesEvent["payload"].(map[string]interface{})
	if payload["action"] != "opened" {
		t.Fatalf("IssuesEvent payload action = %v", payload["action"])
	}
	payloadIssue, _ := payload["issue"].(map[string]interface{})
	if payloadIssue == nil || payloadIssue["title"] != "events feed issue" {
		t.Fatalf("IssuesEvent payload issue wrong: %v", payload)
	}
	commentEvent := byType["IssueCommentEvent"]
	if commentEvent == nil {
		t.Fatalf("IssueCommentEvent missing: %v", events)
	}
	commentPayload, _ := commentEvent["payload"].(map[string]interface{})
	comment, _ := commentPayload["comment"].(map[string]interface{})
	if commentPayload["action"] != "created" || comment == nil || comment["body"] != "commenting for the feed" {
		t.Fatalf("IssueCommentEvent payload wrong: %v", commentPayload)
	}

	// The feed is stable: identical IDs and timestamps across requests.
	resp = ghGet(t, "/api/v3/orgs/events-org/events", defaultToken)
	again := decodeJSONArray(t, resp)
	if len(again) != 2 || again[0]["id"] != events[0]["id"] || again[0]["created_at"] != events[0]["created_at"] {
		t.Fatalf("event feed not stable across requests: %v vs %v", again, events)
	}

	// Closing the issue adds a closed event with the recorded closure time.
	resp = ghPatch(t, "/api/v3/repos/events-org/events-repo/issues/1", defaultToken, map[string]interface{}{"state": "closed"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("close issue %d: %d", issueNumber, resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, "/api/v3/orgs/events-org/events", defaultToken)
	closedFeed := decodeJSONArray(t, resp)
	if len(closedFeed) != 3 {
		t.Fatalf("expected 3 events after closing the issue, got %d", len(closedFeed))
	}
	var sawClosed bool
	for _, ev := range closedFeed {
		if ev["type"] == "IssuesEvent" {
			if p, _ := ev["payload"].(map[string]interface{}); p != nil && p["action"] == "closed" {
				sawClosed = true
			}
		}
	}
	if !sawClosed {
		t.Fatalf("closed IssuesEvent missing: %v", closedFeed)
	}

	// Unknown org.
	resp = ghGet(t, "/api/v3/orgs/no-such-events-org/events", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown org events: %d, want 404", resp.StatusCode)
	}
}
