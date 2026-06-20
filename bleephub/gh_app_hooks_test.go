package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// App-level webhook config + deliveries — GET/PATCH /app/hook/config + the
// per-app /app/hook/deliveries listing surface, matching GitHub's
// installation-vs-app distinction.

func TestAppHookConfig_GetPatch(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppHookRoutes()
	app := s.store.CreateApp(1, "Hook Cfg App", "", nil, nil)

	jwt, err := signAppJWT(app.PEMPrivateKey, app.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	doReq := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var bodyR *bytes.Reader
		if body != nil {
			bodyR = bytes.NewReader(body)
		}
		var req *http.Request
		if bodyR != nil {
			req = httptest.NewRequest(method, path, bodyR)
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	// GET initial config — secret is rendered as **** (redacted).
	w := doReq("GET", "/api/v3/app/hook/config", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["secret"] != "********" {
		t.Errorf("expected redacted secret, got %v", got["secret"])
	}

	// PATCH webhook URL.
	body, _ := json.Marshal(map[string]string{"url": "https://example.test/webhook", "secret": "new-secret"})
	w = doReq("PATCH", "/api/v3/app/hook/config", body)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d body = %s", w.Code, w.Body.String())
	}
	if updated := s.store.GetApp(app.ID); updated.WebhookURL != "https://example.test/webhook" || updated.WebhookSecret != "new-secret" {
		t.Errorf("PATCH did not persist; url=%q secret=%q", updated.WebhookURL, updated.WebhookSecret)
	}
}

func TestAppHookDeliveries_ListGetRedeliver(t *testing.T) {
	// Spin up a sink to receive the redelivery. The handler runs on the
	// httptest server's goroutine while the test body polls below, so the
	// capture must be synchronized.
	var gotMu sync.Mutex
	var got []byte
	gotLen := func() int { gotMu.Lock(); defer gotMu.Unlock(); return len(got) }
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotMu.Lock()
		got = buf
		gotMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppHookRoutes()
	app := s.store.CreateApp(1, "Deliveries App", "", nil, nil)
	s.store.UpdateAppHookConfig(app.ID, func(a *App) {
		a.WebhookURL = sink.URL
		a.WebhookActive = true
	})

	// Record an original delivery as if it had fired earlier.
	original := &WebhookDelivery{
		HookID:      -app.ID,
		AppID:       app.ID,
		Event:       "installation",
		Action:      "created",
		GUID:        "test-guid",
		StatusCode:  200,
		Duration:    0.01,
		Request:     &DeliveryRequest{Headers: map[string]string{"X-GitHub-Event": "installation"}, Payload: json.RawMessage(`{"action":"created"}`)},
		Response:    &DeliveryResponse{StatusCode: 200, Body: "ok"},
		DeliveredAt: time.Now(),
	}
	s.store.AddAppDelivery(app.ID, original)

	jwt, err := signAppJWT(app.PEMPrivateKey, app.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	doReq := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+jwt)
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	// LIST
	w := doReq("GET", "/api/v3/app/hook/deliveries")
	if w.Code != http.StatusOK {
		t.Fatalf("LIST status = %d body = %s", w.Code, w.Body.String())
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(list))
	}
	// Summary carries throttled_at but NOT url.
	if _, ok := list[0]["throttled_at"]; !ok {
		t.Error("delivery summary missing throttled_at")
	}
	if _, ok := list[0]["url"]; ok {
		t.Error("delivery summary must NOT contain url")
	}

	// GET single delivery — full request/response payload + url visible.
	w = doReq("GET", fmt.Sprintf("/api/v3/app/hook/deliveries/%d", original.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d body = %s", w.Code, w.Body.String())
	}
	var single map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &single)
	if single["request"] == nil || single["response"] == nil {
		t.Error("expected full request/response in single-delivery view")
	}
	if _, ok := single["url"]; !ok {
		t.Error("full delivery object must include url")
	}

	// REDELIVER — 202 with no synthetic JSON body (GitHub returns empty body).
	w = doReq("POST", fmt.Sprintf("/api/v3/app/hook/deliveries/%d/attempts", original.ID))
	if w.Code != http.StatusAccepted {
		t.Fatalf("REDELIVER status = %d body = %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Errorf("REDELIVER body = %q, want empty", w.Body.String())
	}
	// Sink fires async — quick poll.
	deadline := time.Now().Add(2 * time.Second)
	for gotLen() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	// Store now has 2 deliveries (the original + the redelivery).
	deliveries := s.store.ListAppDeliveries(app.ID)
	if len(deliveries) != 2 {
		t.Errorf("expected 2 deliveries after redeliver, got %d", len(deliveries))
	}
	foundRedelivery := false
	for _, d := range deliveries {
		if d.Redelivery {
			foundRedelivery = true
		}
	}
	if !foundRedelivery {
		t.Error("expected one delivery marked Redelivery=true")
	}
}
