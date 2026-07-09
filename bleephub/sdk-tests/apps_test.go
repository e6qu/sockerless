package sdktests

import (
	"net/http"
	"testing"
)

// TestAppsGetBySlug provisions a GitHub App through the manifest flow, then
// reads it back through the typed Apps.Get(slug), which maps to GET
// /api/v3/apps/{app_slug}.
func TestAppsGetBySlug(t *testing.T) {
	var created struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
		PEM  string `json:"pem"`
	}
	if code := createGitHubAppViaManifest(t, "Software Development Kit Test App",
		map[string]string{"contents": "read", "checks": "write"}, &created); code != http.StatusCreated {
		t.Fatalf("GitHub App manifest conversion status = %d, want 201", code)
	}
	if created.Slug == "" {
		t.Fatal("GitHub App manifest conversion returned empty slug")
	}

	app, _, err := client.Apps.Get(ctx(), created.Slug)
	if err != nil {
		t.Fatalf("Apps.Get(%q): %v", created.Slug, err)
	}
	if app.GetID() != created.ID {
		t.Errorf("app ID = %d, want %d", app.GetID(), created.ID)
	}
	if app.GetSlug() != created.Slug {
		t.Errorf("app slug = %q, want %q", app.GetSlug(), created.Slug)
	}
	if app.GetName() != "Software Development Kit Test App" {
		t.Errorf("app name = %q, want Software Development Kit Test App", app.GetName())
	}
}
