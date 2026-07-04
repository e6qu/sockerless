package bleephub

import (
	"testing"
)

// TestNotificationThreadMarkDone exercises DELETE
// /notifications/threads/{thread_id} ("mark a thread as done"): the thread
// disappears from the user's notification list.
func TestNotificationThreadMarkDone(t *testing.T) {
	repoKey := createTestRepo(t)
	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/issues", defaultToken, map[string]interface{}{
		"title": "thread-done source issue",
	})
	decodeJSONWithStatus(t, resp, 201)

	resp = ghGet(t, "/api/v3/notifications?all=true", defaultToken)
	threads := decodeJSONArray(t, resp)
	var threadID string
	for _, th := range threads {
		subject, _ := th["subject"].(map[string]interface{})
		if subject != nil && subject["title"] == "thread-done source issue" {
			threadID, _ = th["id"].(string)
		}
	}
	if threadID == "" {
		t.Fatalf("no notification thread for the created issue; threads=%v", threads)
	}

	del := ghDelete(t, "/api/v3/notifications/threads/"+threadID, defaultToken)
	del.Body.Close()
	if del.StatusCode != 204 {
		t.Fatalf("mark-done status = %d, want 204", del.StatusCode)
	}

	// The dismissed thread no longer appears, even with all=true.
	resp = ghGet(t, "/api/v3/notifications?all=true", defaultToken)
	threads = decodeJSONArray(t, resp)
	for _, th := range threads {
		if th["id"] == threadID {
			t.Fatal("thread still listed after DELETE")
		}
	}

	// Unknown thread → 404; unauthenticated → 401.
	del = ghDelete(t, "/api/v3/notifications/threads/999999999", defaultToken)
	del.Body.Close()
	if del.StatusCode != 404 {
		t.Fatalf("unknown thread status = %d, want 404", del.StatusCode)
	}
	del = ghDelete(t, "/api/v3/notifications/threads/"+threadID, "")
	del.Body.Close()
	if del.StatusCode != 401 {
		t.Fatalf("unauthenticated status = %d, want 401", del.StatusCode)
	}
}
