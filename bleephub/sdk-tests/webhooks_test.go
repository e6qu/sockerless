package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestWebhooks covers CreateHook / ListHooks / GetHook / EditHook / PingHook /
// DeleteHook and specifically asserts content_type and insecure_ssl round-trip
// through the typed HookConfig (a known fidelity-sensitive area).
func TestWebhooks(t *testing.T) {
	name := uniqueName("hooks")
	createRepo(t, name)

	created, _, err := client.Repositories.CreateHook(ctx(), "admin", name, &github.Hook{
		Events: []string{"push", "pull_request"},
		Active: github.Ptr(true),
		Config: &github.HookConfig{
			URL:         github.Ptr("https://example.com/webhook"),
			ContentType: github.Ptr("json"),
			InsecureSSL: github.Ptr("1"),
		},
	})
	if err != nil {
		t.Fatalf("CreateHook: %v", err)
	}
	if created.GetID() == 0 {
		t.Error("hook ID is zero")
	}
	if created.GetConfig().GetContentType() != "json" {
		t.Errorf("created content_type = %q, want json", created.GetConfig().GetContentType())
	}
	if created.GetConfig().GetInsecureSSL() != "1" {
		t.Errorf("created insecure_ssl = %q, want 1", created.GetConfig().GetInsecureSSL())
	}
	if created.GetConfig().GetURL() != "https://example.com/webhook" {
		t.Errorf("created config url = %q", created.GetConfig().GetURL())
	}
	id := created.GetID()

	// Get — round-trip content_type / insecure_ssl.
	got, _, err := client.Repositories.GetHook(ctx(), "admin", name, id)
	if err != nil {
		t.Fatalf("GetHook: %v", err)
	}
	if got.GetConfig().GetContentType() != "json" {
		t.Errorf("GetHook content_type = %q, want json", got.GetConfig().GetContentType())
	}
	if got.GetConfig().GetInsecureSSL() != "1" {
		t.Errorf("GetHook insecure_ssl = %q, want 1", got.GetConfig().GetInsecureSSL())
	}

	// List
	hooks, _, err := client.Repositories.ListHooks(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListHooks: %v", err)
	}
	if len(hooks) == 0 {
		t.Error("ListHooks returned none")
	}

	// Edit: flip content_type to form and insecure_ssl to 0.
	edited, _, err := client.Repositories.EditHook(ctx(), "admin", name, id, &github.Hook{
		Config: &github.HookConfig{
			URL:         github.Ptr("https://example.com/webhook"),
			ContentType: github.Ptr("form"),
			InsecureSSL: github.Ptr("0"),
		},
	})
	if err != nil {
		t.Fatalf("EditHook: %v", err)
	}
	if edited.GetConfig().GetContentType() != "form" {
		t.Errorf("edited content_type = %q, want form", edited.GetConfig().GetContentType())
	}
	if edited.GetConfig().GetInsecureSSL() != "0" {
		t.Errorf("edited insecure_ssl = %q, want 0", edited.GetConfig().GetInsecureSSL())
	}

	// Ping
	if _, err := client.Repositories.PingHook(ctx(), "admin", name, id); err != nil {
		t.Fatalf("PingHook: %v", err)
	}

	// Delete
	if _, err := client.Repositories.DeleteHook(ctx(), "admin", name, id); err != nil {
		t.Fatalf("DeleteHook: %v", err)
	}
	if _, _, err := client.Repositories.GetHook(ctx(), "admin", name, id); err == nil {
		t.Error("GetHook after delete succeeded, want 404")
	}
}
