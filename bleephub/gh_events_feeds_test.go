package bleephub

import (
	"strconv"
	"testing"
)

func TestPublicEventsFeed(t *testing.T) {
	repoKey := createTestRepo(t)

	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/issues", defaultToken, map[string]interface{}{
		"title": "public event source issue",
		"body":  "event feed body",
	})
	issue := decodeJSONWithStatus(t, resp, 201)
	issueNumber := int(issue["number"].(float64))

	resp = ghPost(t, "/api/v3/repos/"+repoKey+"/issues/"+strconv.Itoa(issueNumber)+"/comments", defaultToken, map[string]interface{}{
		"body": "event feed comment",
	})
	decodeJSONWithStatus(t, resp, 201)

	resp = ghGet(t, "/api/v3/events", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("events status = %d", resp.StatusCode)
	}
	events := decodeJSONArray(t, resp)

	var issuesEvent, commentEvent map[string]interface{}
	for _, e := range events {
		repo, _ := e["repo"].(map[string]interface{})
		if repo == nil || repo["name"] != repoKey {
			continue
		}
		switch e["type"] {
		case "IssuesEvent":
			issuesEvent = e
		case "IssueCommentEvent":
			commentEvent = e
		}
	}
	if issuesEvent == nil {
		t.Fatal("no IssuesEvent for the created issue in GET /events")
	}
	payload, _ := issuesEvent["payload"].(map[string]interface{})
	if payload["action"] != "opened" {
		t.Fatalf("IssuesEvent action = %v", payload["action"])
	}
	embedded, _ := payload["issue"].(map[string]interface{})
	if embedded == nil || embedded["title"] != "public event source issue" {
		t.Fatalf("IssuesEvent payload issue = %v", payload["issue"])
	}
	actor, _ := issuesEvent["actor"].(map[string]interface{})
	if actor == nil || actor["login"] != "admin" {
		t.Fatalf("event actor = %v", issuesEvent["actor"])
	}
	if issuesEvent["public"] != true || issuesEvent["id"] == nil || issuesEvent["created_at"] == nil {
		t.Fatalf("event envelope: %v", issuesEvent)
	}

	if commentEvent == nil {
		t.Fatal("no IssueCommentEvent for the created comment in GET /events")
	}
	cp, _ := commentEvent["payload"].(map[string]interface{})
	comment, _ := cp["comment"].(map[string]interface{})
	if comment == nil || comment["body"] != "event feed comment" {
		t.Fatalf("IssueCommentEvent payload = %v", commentEvent["payload"])
	}
}

// TestPublicEventsExcludePrivateRepos verifies the global feed never leaks
// activity from private repositories.
func TestPublicEventsExcludePrivateRepos(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "private-events-repo",
		"private": true,
	})
	decodeJSONWithStatus(t, resp, 201)
	resp = ghPost(t, "/api/v3/repos/admin/private-events-repo/issues", defaultToken, map[string]interface{}{
		"title": "private issue must not surface",
	})
	decodeJSONWithStatus(t, resp, 201)

	resp = ghGet(t, "/api/v3/events", defaultToken)
	events := decodeJSONArray(t, resp)
	for _, e := range events {
		repo, _ := e["repo"].(map[string]interface{})
		if repo != nil && repo["name"] == "admin/private-events-repo" {
			t.Fatal("private repository event leaked into GET /events")
		}
	}
}

func TestFeedsCatalog(t *testing.T) {
	resp := ghGet(t, "/api/v3/feeds", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("feeds status = %d", resp.StatusCode)
	}
	feeds := decodeJSON(t, resp)
	if feeds["timeline_url"] != testBaseURL+"/timeline" {
		t.Fatalf("timeline_url = %v", feeds["timeline_url"])
	}
	if feeds["user_url"] != testBaseURL+"/{user}" {
		t.Fatalf("user_url = %v", feeds["user_url"])
	}
	if feeds["current_user_public_url"] != testBaseURL+"/admin" {
		t.Fatalf("current_user_public_url = %v", feeds["current_user_public_url"])
	}
	links, _ := feeds["_links"].(map[string]interface{})
	if links == nil {
		t.Fatal("missing _links")
	}
	timeline, _ := links["timeline"].(map[string]interface{})
	if timeline == nil || timeline["type"] != "application/atom+xml" || timeline["href"] != testBaseURL+"/timeline" {
		t.Fatalf("_links.timeline = %v", links["timeline"])
	}
	if links["user"] == nil {
		t.Fatal("missing _links.user")
	}
}
