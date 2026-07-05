package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// FuzzReactionCreate fuzzes the reaction content body and the parent id in the
// path for POST .../issues/{number}/reactions. Invariants:
//   - a valid content on a real parent returns 200/201 and echoes that content;
//   - an invalid/empty content is rejected 422;
//   - an unknown parent (issue number that does not exist) is 404;
//   - never a 5xx/panic, and a created reaction never carries a content the
//     caller did not send (no cross-content leak).
func FuzzReactionCreate(f *testing.F) {
	s := fuzzRoutedServer(f)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "react-fuzz", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "reactable", "body", nil, nil, 0)
	if issue == nil {
		f.Fatal("CreateIssue returned nil")
	}
	realNumber := issue.Number

	f.Add("+1", realNumber)
	f.Add("heart", realNumber)
	f.Add("rocket", realNumber)
	f.Add("thumbsup", realNumber)
	f.Add("", realNumber)
	f.Add("LAUGH", realNumber)
	f.Add("+1", 999999)
	f.Add("heart", 0)
	f.Add("heart", -1)
	f.Add("💩", realNumber)
	f.Add("eyes", 2147483647)

	f.Fuzz(func(t *testing.T, content string, number int) {
		body, _ := json.Marshal(map[string]interface{}{"content": content})
		path := fmt.Sprintf("/api/v3/repos/admin/react-fuzz/issues/%d/reactions", number)
		w := fuzzServe(s, http.MethodPost, path, body)

		if w.Code >= 500 {
			t.Fatalf("reaction POST content=%q number=%d -> %d: %s", content, number, w.Code, w.Body.String())
		}

		valid := validReactionContent[content]
		exists := number == realNumber

		switch {
		case exists && valid:
			if w.Code != http.StatusCreated && w.Code != http.StatusOK {
				t.Fatalf("valid reaction %q on real issue -> %d, want 200/201: %s", content, w.Code, w.Body.String())
			}
			var res map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("reaction body not JSON: %v", err)
			}
			if res["content"] != content {
				t.Fatalf("reaction echoed content %v, want %q (cross-content leak)", res["content"], content)
			}
		case exists && !valid:
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("invalid content %q -> %d, want 422: %s", content, w.Code, w.Body.String())
			}
		case !exists:
			// Unknown parent resolves to 404. The empty-content body is
			// rejected 422 before the parent is resolved, so accept that too.
			emptyContent422 := content == "" && w.Code == http.StatusUnprocessableEntity
			if w.Code != http.StatusNotFound && !emptyContent422 {
				t.Fatalf("unknown parent number=%d content=%q -> %d, want 404: %s", number, content, w.Code, w.Body.String())
			}
		}
	})
}
