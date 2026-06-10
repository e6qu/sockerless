package sdktests

import (
	"net/http"
	"testing"
)

// TestAppsGetBySlug provisions a GitHub App via the /internal/apps sim-control
// endpoint (go-github has no App-create method — apps are created out of band
// via the manifest flow on real GitHub), then reads it back through the typed
// Apps.Get(slug), which maps to GET /api/v3/apps/{app_slug}.
func TestAppsGetBySlug(t *testing.T) {
	var created struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
		PEM  string `json:"pem"`
	}
	if code := internalPost(t, "/internal/apps", map[string]interface{}{
		"name":        "SDK Test App",
		"description": "an app for sdk-tests",
		"permissions": map[string]string{"contents": "read", "checks": "write"},
		"events":      []string{"push"},
	}, &created); code != http.StatusCreated {
		t.Fatalf("internal create app status = %d, want 201", code)
	}
	if created.Slug == "" {
		t.Fatal("internal create app returned empty slug")
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
	if app.GetName() != "SDK Test App" {
		t.Errorf("app name = %q, want SDK Test App", app.GetName())
	}
}
