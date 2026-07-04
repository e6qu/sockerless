package bleephub

import (
	"testing"
)

func TestUserProfileUpdate(t *testing.T) {
	user := createTestUser(t, "patch-profile-user")
	token := testServer.store.CreateToken(user.ID, "repo").Value

	resp := ghPatch(t, "/api/v3/user", token, map[string]interface{}{
		"name":             "Omar Jahandar",
		"bio":              "builds things",
		"company":          "Acme corporation",
		"location":         "Berlin, Germany",
		"blog":             "blog.example.com",
		"hireable":         true,
		"twitter_username": "therealomarj",
	})
	updated := decodeJSONWithStatus(t, resp, 200)
	if updated["name"] != "Omar Jahandar" || updated["bio"] != "builds things" {
		t.Fatalf("patched user: name=%v bio=%v", updated["name"], updated["bio"])
	}
	if updated["company"] != "Acme corporation" || updated["location"] != "Berlin, Germany" {
		t.Fatalf("patched user: company=%v location=%v", updated["company"], updated["location"])
	}
	if updated["blog"] != "blog.example.com" || updated["hireable"] != true {
		t.Fatalf("patched user: blog=%v hireable=%v", updated["blog"], updated["hireable"])
	}
	if updated["twitter_username"] != "therealomarj" {
		t.Fatalf("twitter_username = %v", updated["twitter_username"])
	}

	// The update round-trips through GET /user.
	resp = ghGet(t, "/api/v3/user", token)
	fetched := decodeJSONWithStatus(t, resp, 200)
	if fetched["company"] != "Acme corporation" || fetched["hireable"] != true {
		t.Fatalf("GET /user after patch: %v", fetched)
	}

	// A partial patch leaves other fields untouched.
	resp = ghPatch(t, "/api/v3/user", token, map[string]interface{}{
		"location": "Hamburg, Germany",
	})
	updated = decodeJSONWithStatus(t, resp, 200)
	if updated["location"] != "Hamburg, Germany" || updated["company"] != "Acme corporation" {
		t.Fatalf("partial patch: location=%v company=%v", updated["location"], updated["company"])
	}

	// Unauthenticated → 401.
	resp = ghPatch(t, "/api/v3/user", "", map[string]interface{}{"name": "x"})
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated status = %d, want 401", resp.StatusCode)
	}
}
