package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// installation / installation_repositories webhook events — emission +
// X-GitHub-Hook-* headers + X-Hub-Signature SHA1 + SHA256 +
// installation:{id} payload, matching the GitHub-spec wire shape.

func TestInstallationCreatedFiresAppWebhook(t *testing.T) {
	// Sink captures the incoming webhook.
	type capture struct {
		event       string
		hookID      string
		targetType  string
		targetID    string
		sig256      string
		sigSHA1     string
		bodyHasInst bool
		appID       float64
	}
	got := make(chan capture, 1)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := bytesReadAll(r)
		_ = json.Unmarshal(raw, &body)
		hasInst := body["installation"] != nil
		// app_id is a field of the installation object (real GitHub), not a
		// top-level key on the installation event payload.
		var appID float64
		if inst, ok := body["installation"].(map[string]any); ok {
			appID, _ = inst["app_id"].(float64)
		}
		got <- capture{
			event:       r.Header.Get("X-GitHub-Event"),
			hookID:      r.Header.Get("X-GitHub-Hook-ID"),
			targetType:  r.Header.Get("X-GitHub-Hook-Installation-Target-Type"),
			targetID:    r.Header.Get("X-GitHub-Hook-Installation-Target-ID"),
			sig256:      r.Header.Get("X-Hub-Signature-256"),
			sigSHA1:     r.Header.Get("X-Hub-Signature"),
			bodyHasInst: hasInst,
			appID:       appID,
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppHookRoutes()

	user := s.store.UsersByLogin["admin"]
	app := s.store.CreateApp(user.ID, "Event Fire App", "", nil, nil)
	// Configure app webhook.
	s.store.UpdateAppHookConfig(app.ID, func(a *App) {
		a.WebhookURL = sink.URL
		a.WebhookActive = true
		a.WebhookSecret = "shh"
	})

	form := url.Values{"target_login": {user.Login}, "repository_selection": {"all"}}
	req := httptest.NewRequest("POST", "/apps/"+app.Slug+"/installations/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.internalAuthMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create installation status = %d body = %s", w.Code, w.Body.String())
	}

	select {
	case c := <-got:
		if c.event != "installation" {
			t.Errorf("X-GitHub-Event = %q, want installation", c.event)
		}
		if c.hookID == "" {
			t.Error("X-GitHub-Hook-ID empty")
		}
		if c.targetType != "integration" {
			t.Errorf("Target-Type = %q, want integration", c.targetType)
		}
		if c.targetID == "" {
			t.Error("Target-ID empty")
		}
		if !strings.HasPrefix(c.sig256, "sha256=") {
			t.Errorf("sig256 = %q, want sha256=...", c.sig256)
		}
		if !strings.HasPrefix(c.sigSHA1, "sha1=") {
			t.Errorf("sigSHA1 = %q, want sha1=...", c.sigSHA1)
		}
		if !c.bodyHasInst {
			t.Error("payload missing installation block")
		}
		if c.appID != float64(app.ID) {
			t.Errorf("installation.app_id = %v, want %d", c.appID, app.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for webhook delivery")
	}
}

// helper to read body in test without pulling in io
func bytesReadAll(r *http.Request) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}
