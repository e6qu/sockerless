package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// hookResp is the shape GitHub returns for a repository webhook.
type hookResp struct {
	ID            int                    `json:"id"`
	Type          string                 `json:"type"`
	Name          string                 `json:"name"`
	Active        bool                   `json:"active"`
	Events        []string               `json:"events"`
	Config        map[string]interface{} `json:"config"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
	URL           string                 `json:"url"`
	TestURL       string                 `json:"test_url"`
	PingURL       string                 `json:"ping_url"`
	DeliveriesURL string                 `json:"deliveries_url"`
	LastResponse  map[string]interface{} `json:"last_response"`
}

func decodeHook(t *testing.T, resp *http.Response) hookResp {
	t.Helper()
	defer resp.Body.Close()
	var h hookResp
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatalf("decode hook: %v", err)
	}
	return h
}

func decodeHookList(t *testing.T, resp *http.Response) []hookResp {
	t.Helper()
	defer resp.Body.Close()
	var list []hookResp
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode hook list: %v", err)
	}
	return list
}

// pollDeliveries polls the deliveries endpoint until at least minCount entries
// are present or 3 seconds pass. AddDelivery is called after the HTTP round-trip
// completes, so there is a brief window between the target receiving the request
// and the delivery appearing in the store.
func pollDeliveries(t *testing.T, path string, minCount int) []map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := ghGet(t, path, defaultToken)
		var list []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&list)
		resp.Body.Close()
		if len(list) >= minCount {
			return list
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pollDeliveries: wanted %d entries at %s within 3s", minCount, path)
	return nil
}

// assertHookShape verifies all fields GitHub's published schema requires.
func assertHookShape(t *testing.T, h hookResp, targetURL string) {
	t.Helper()
	if h.Type != "Repository" {
		t.Errorf("type = %q, want Repository", h.Type)
	}
	if h.Name != "web" {
		t.Errorf("name = %q, want web", h.Name)
	}
	if h.ID <= 0 {
		t.Errorf("id = %d, want > 0", h.ID)
	}
	if h.URL == "" {
		t.Error("url must be present (GitHub API self-link)")
	}
	if !strings.HasSuffix(h.URL, strconv.Itoa(h.ID)) {
		t.Errorf("url %q should end with the hook id %d", h.URL, h.ID)
	}
	if h.TestURL == "" {
		t.Error("test_url must be present")
	}
	if h.PingURL == "" {
		t.Error("ping_url must be present")
	}
	if h.DeliveriesURL == "" {
		t.Error("deliveries_url must be present")
	}
	if h.LastResponse == nil {
		t.Error("last_response must be present")
	}
	if _, ok := h.LastResponse["status"]; !ok {
		t.Error("last_response must contain 'status'")
	}
	cfg, ok := h.Config["url"]
	if !ok || cfg != targetURL {
		t.Errorf("config.url = %v, want %q", cfg, targetURL)
	}
	// content_type and insecure_ssl must always round-trip (Terraform reads them).
	if _, ok := h.Config["content_type"]; !ok {
		t.Error("config.content_type must be present")
	}
	if _, ok := h.Config["insecure_ssl"]; !ok {
		t.Error("config.insecure_ssl must be present")
	}
	if h.CreatedAt == "" {
		t.Error("created_at must be present")
	}
	if h.UpdatedAt == "" {
		t.Error("updated_at must be present")
	}
}

// TestHooks_CRUD exercises create → list → get → update → delete lifecycle
// and verifies the response shape matches GitHub's published schema.
func TestHooks_CRUD(t *testing.T) {
	// Spin up a trivial HTTP target that returns 200 for every POST.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	repo := "admin/hooks-crud"
	// Create repo so the hook has something to attach to.
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-crud",
	}).Body.Close()

	// CreateHook
	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url":          target.URL + "/hook",
			"content_type": "json",
		},
		"events": []string{"push", "pull_request"},
		"active": true,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create hook: got %d: %s", resp.StatusCode, body)
	}
	created := decodeHook(t, resp)
	assertHookShape(t, created, target.URL+"/hook")
	if !created.Active {
		t.Error("created hook should be active")
	}
	hookID := created.ID

	// ListHooks
	listResp := ghGet(t, "/api/v3/repos/"+repo+"/hooks", defaultToken)
	if listResp.StatusCode != http.StatusOK {
		listResp.Body.Close()
		t.Fatalf("list hooks: got %d", listResp.StatusCode)
	}
	hooks := decodeHookList(t, listResp)
	found := false
	for _, h := range hooks {
		if h.ID == hookID {
			assertHookShape(t, h, target.URL+"/hook")
			found = true
		}
	}
	if !found {
		t.Errorf("created hook id=%d not found in ListHooks", hookID)
	}

	// GetHook
	getResp := ghGet(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID), defaultToken)
	if getResp.StatusCode != http.StatusOK {
		getResp.Body.Close()
		t.Fatalf("get hook: got %d", getResp.StatusCode)
	}
	got := decodeHook(t, getResp)
	assertHookShape(t, got, target.URL+"/hook")
	if got.ID != hookID {
		t.Errorf("get hook id = %d, want %d", got.ID, hookID)
	}

	// UpdateHook — deactivate and add a new event.
	newTarget := target.URL + "/hook-updated"
	patchResp, _ := func() (*http.Response, error) {
		b, _ := json.Marshal(map[string]interface{}{
			"config": map[string]interface{}{"url": newTarget},
			"active": false,
			"events": []string{"push"},
		})
		req, _ := http.NewRequest("PATCH",
			testBaseURL+"/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID),
			strings.NewReader(string(b)))
		req.Header.Set("Authorization", "token "+defaultToken)
		req.Header.Set("Content-Type", "application/json")
		return http.DefaultClient.Do(req)
	}()
	if patchResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(patchResp.Body)
		patchResp.Body.Close()
		t.Fatalf("update hook: got %d: %s", patchResp.StatusCode, body)
	}
	updated := decodeHook(t, patchResp)
	assertHookShape(t, updated, newTarget)
	if updated.Active {
		t.Error("hook should be inactive after update")
	}
	if len(updated.Events) != 1 || updated.Events[0] != "push" {
		t.Errorf("updated events = %v, want [push]", updated.Events)
	}

	// DeleteHook
	delReq, _ := http.NewRequest("DELETE",
		testBaseURL+"/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID), nil)
	delReq.Header.Set("Authorization", "token "+defaultToken)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("delete hook: got %d, want 204", delResp.StatusCode)
	}

	// Verify gone
	goneResp := ghGet(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID), defaultToken)
	goneResp.Body.Close()
	if goneResp.StatusCode != http.StatusNotFound {
		t.Errorf("deleted hook: got %d, want 404", goneResp.StatusCode)
	}
}

// TestHooks_Ping verifies that POST /hooks/{id}/pings triggers a delivery
// and that delivery objects carry the fields GitHub's schema requires.
func TestHooks_Ping(t *testing.T) {
	received := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-GitHub-Event") != "" {
			received <- struct{}{}
		}
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	repo := "admin/hooks-ping"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-ping",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": target.URL + "/hook", "content_type": "json"},
		"events": []string{"push"},
		"active": true,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create hook: %d", resp.StatusCode)
	}

	// Fetch hook list to get the ID.
	hooks := decodeHookList(t, ghGet(t, "/api/v3/repos/"+repo+"/hooks", defaultToken))
	if len(hooks) == 0 {
		t.Fatal("no hooks in list")
	}
	hookID := hooks[len(hooks)-1].ID

	// Ping
	pingResp := ghPost(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/pings",
		defaultToken, nil)
	pingResp.Body.Close()
	if pingResp.StatusCode != http.StatusNoContent {
		t.Errorf("ping: got %d, want 204", pingResp.StatusCode)
	}

	// Wait for async delivery (up to 3 s).
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Error("ping webhook delivery not received within 3s")
		return
	}

	// Poll for the delivery to be stored (AddDelivery happens after the HTTP call returns).
	deliveries := pollDeliveries(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries", 1)

	if len(deliveries) == 0 {
		t.Fatal("no deliveries recorded after ping")
	}

	d := deliveries[0]
	// Required fields per GitHub's delivery-summary schema. The summary carries
	// throttled_at but NOT url (only the full GET delivery object has url).
	for _, field := range []string{"id", "guid", "delivered_at", "redelivery", "duration", "status", "status_code", "event", "action", "throttled_at"} {
		if _, ok := d[field]; !ok {
			t.Errorf("delivery missing required field %q", field)
		}
	}
	if _, ok := d["url"]; ok {
		t.Error("delivery summary must NOT contain 'url' (only the full delivery object does)")
	}
	if d["throttled_at"] != nil {
		t.Errorf("throttled_at = %v, want null for an un-throttled delivery", d["throttled_at"])
	}
	if d["event"] != "ping" {
		t.Errorf("event = %v, want ping", d["event"])
	}
	if d["status"] != "OK" {
		t.Errorf("status = %v, want OK", d["status"])
	}

	// GetDelivery — verify full delivery includes request + response.
	dlID := int(d["id"].(float64))
	fullResp := ghGet(t,
		"/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries/"+strconv.Itoa(dlID),
		defaultToken)
	if fullResp.StatusCode != http.StatusOK {
		fullResp.Body.Close()
		t.Fatalf("get delivery: %d", fullResp.StatusCode)
	}
	var full map[string]interface{}
	json.NewDecoder(fullResp.Body).Decode(&full)
	fullResp.Body.Close()

	if _, ok := full["request"]; !ok {
		t.Error("full delivery must include 'request'")
	}
	if _, ok := full["response"]; !ok {
		t.Error("full delivery must include 'response'")
	}
	// Unlike the summary, the full delivery object DOES carry url.
	if full["url"] != target.URL+"/hook" {
		t.Errorf("full delivery url = %v, want %s/hook", full["url"], target.URL)
	}
}

// TestHooks_Deliveries_Redeliver verifies the redeliver endpoint and that
// the redelivery flag is set to true.
func TestHooks_Deliveries_Redeliver(t *testing.T) {
	delivered := make(chan string, 10)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered <- r.Header.Get("X-GitHub-Event")
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	repo := "admin/hooks-redeliver"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-redeliver",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": target.URL + "/hook", "content_type": "json"},
		"events": []string{"push"},
		"active": true,
	})
	resp.Body.Close()

	listResp := ghGet(t, "/api/v3/repos/"+repo+"/hooks", defaultToken)
	hooks := decodeHookList(t, listResp)
	hookID := hooks[len(hooks)-1].ID

	// Trigger a ping to produce the initial delivery.
	ghPost(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/pings", defaultToken, nil).Body.Close()
	select {
	case <-delivered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial ping not received")
	}

	// Poll for the delivery to be persisted before querying.
	deliveries := pollDeliveries(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries", 1)
	dlID := int(deliveries[0]["id"].(float64))

	// Redeliver.
	redeliverResp := ghPost(t,
		"/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries/"+strconv.Itoa(dlID)+"/attempts",
		defaultToken, nil)
	if redeliverResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(redeliverResp.Body)
		redeliverResp.Body.Close()
		t.Fatalf("redeliver: got %d: %s", redeliverResp.StatusCode, body)
	}
	redeliverResp.Body.Close()

	// A second delivery should arrive.
	select {
	case <-delivered:
	case <-time.After(3 * time.Second):
		t.Fatal("redelivered webhook not received")
	}

	// Poll until a redelivery=true entry is persisted (the create-time
	// auto-ping and the explicit ping also produce deliveries, so a fixed
	// count isn't a reliable signal — wait for the redelivery flag itself).
	foundRedelivery := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !foundRedelivery {
		for _, d := range pollDeliveries(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries", 1) {
			if r, ok := d["redelivery"].(bool); ok && r {
				foundRedelivery = true
				break
			}
		}
		if !foundRedelivery {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !foundRedelivery {
		t.Error("no delivery with redelivery=true found after redeliver attempt")
	}
}

// TestHooks_NotFound verifies 404 responses for missing hooks and deliveries.
func TestHooks_NotFound(t *testing.T) {
	repo := "admin/hooks-404"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-404",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/"+repo+"/hooks/99999", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing hook: got %d, want 404", resp.StatusCode)
	}

	resp = ghGet(t, "/api/v3/repos/"+repo+"/hooks/99999/deliveries", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deliveries for missing hook: got %d, want 404", resp.StatusCode)
	}
}

// TestHooks_ValidationError verifies that creating a hook without config.url
// returns GitHub's validation error shape.
func TestHooks_ValidationError(t *testing.T) {
	repo := "admin/hooks-validation"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-validation",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{},
		"events": []string{"push"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("missing url: got %d, want 422", resp.StatusCode)
	}
}

// TestHooks_ConfigContentTypeRoundTrip verifies config.content_type and
// config.insecure_ssl default correctly and round-trip through create + update,
// the contract Terraform's github_repository_webhook relies on for idempotency.
func TestHooks_ConfigContentTypeRoundTrip(t *testing.T) {
	repo := "admin/hooks-config-roundtrip"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-config-roundtrip",
	}).Body.Close()

	// Create with no content_type/insecure_ssl → GitHub defaults (form / "0").
	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": "http://example.com/hook"},
		"events": []string{"push"},
		"active": true,
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: got %d", resp.StatusCode)
	}
	h := decodeHook(t, resp)
	if h.Config["content_type"] != "form" {
		t.Errorf("default content_type = %v, want form", h.Config["content_type"])
	}
	if h.Config["insecure_ssl"] != "0" {
		t.Errorf("default insecure_ssl = %v, want \"0\"", h.Config["insecure_ssl"])
	}

	// Create with explicit values → echoed back verbatim.
	resp2 := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url":          "http://example.com/hook2",
			"content_type": "json",
			"insecure_ssl": "1",
		},
		"events": []string{"push"},
	})
	if resp2.StatusCode != http.StatusCreated {
		resp2.Body.Close()
		t.Fatalf("create2: got %d", resp2.StatusCode)
	}
	h2 := decodeHook(t, resp2)
	if h2.Config["content_type"] != "json" {
		t.Errorf("content_type = %v, want json", h2.Config["content_type"])
	}
	if h2.Config["insecure_ssl"] != "1" {
		t.Errorf("insecure_ssl = %v, want \"1\"", h2.Config["insecure_ssl"])
	}

	// GET round-trips the stored values.
	getResp := ghGet(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(h2.ID), defaultToken)
	got := decodeHook(t, getResp)
	if got.Config["content_type"] != "json" || got.Config["insecure_ssl"] != "1" {
		t.Errorf("GET config = %v, want json/1", got.Config)
	}

	// PATCH back to form/0 and confirm.
	patchResp := ghPatch(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(h2.ID), defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url":          "http://example.com/hook2",
			"content_type": "form",
			"insecure_ssl": "0",
		},
	})
	patched := decodeHook(t, patchResp)
	if patched.Config["content_type"] != "form" || patched.Config["insecure_ssl"] != "0" {
		t.Errorf("patched config = %v, want form/0", patched.Config)
	}
}

// TestHooks_NameValidation verifies the optional `name` field must equal "web".
func TestHooks_NameValidation(t *testing.T) {
	repo := "admin/hooks-name-validation"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-name-validation",
	}).Body.Close()

	// name=web is accepted.
	ok := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"name":   "web",
		"config": map[string]interface{}{"url": "http://example.com/hook"},
		"events": []string{"push"},
	})
	if ok.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(ok.Body)
		ok.Body.Close()
		t.Fatalf("name=web: got %d: %s", ok.StatusCode, body)
	}
	ok.Body.Close()

	// A non-"web" name is rejected with 422.
	bad := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"name":   "slack",
		"config": map[string]interface{}{"url": "http://example.com/hook"},
		"events": []string{"push"},
	})
	bad.Body.Close()
	if bad.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("name=slack: got %d, want 422", bad.StatusCode)
	}
}

// TestHooks_LastResponseAfterDelivery verifies hook.last_response is "unused"
// before any delivery and reflects the delivery outcome afterwards.
func TestHooks_LastResponseAfterDelivery(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	repo := "admin/hooks-last-response"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-last-response",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": target.URL + "/hook"},
		"events": []string{"push"},
		"active": true,
	})
	created := decodeHook(t, resp)
	hookID := created.ID

	// Before any delivery → unused.
	if created.LastResponse["status"] != "unused" {
		t.Errorf("pre-delivery last_response.status = %v, want unused", created.LastResponse["status"])
	}

	// Ping triggers a successful delivery.
	ghPost(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/pings", defaultToken, nil).Body.Close()
	pollDeliveries(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries", 1)

	// Poll the hook until last_response reflects the OK delivery.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got := decodeHook(t, ghGet(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID), defaultToken))
		if got.LastResponse["status"] == "OK" {
			if code, _ := got.LastResponse["code"].(float64); int(code) != 200 {
				t.Errorf("last_response.code = %v, want 200", got.LastResponse["code"])
			}
			if got.LastResponse["message"] != "OK" {
				t.Errorf("last_response.message = %v, want OK", got.LastResponse["message"])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("last_response never reflected the OK delivery within 3s")
}

// TestHooks_RedeliverEmptyBody verifies the redeliver endpoint returns 202 with
// no synthetic {id,redelivery} JSON body (GitHub returns a minimal/empty body).
func TestHooks_RedeliverEmptyBody(t *testing.T) {
	delivered := make(chan struct{}, 4)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		delivered <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	repo := "admin/hooks-redeliver-body"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "hooks-redeliver-body",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": target.URL + "/hook"},
		"events": []string{"push"},
		"active": true,
	})
	hookID := decodeHook(t, resp).ID

	ghPost(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/pings", defaultToken, nil).Body.Close()
	<-delivered
	deliveries := pollDeliveries(t, "/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries", 1)
	dlID := int(deliveries[0]["id"].(float64))

	redeliver := ghPost(t,
		"/api/v3/repos/"+repo+"/hooks/"+strconv.Itoa(hookID)+"/deliveries/"+strconv.Itoa(dlID)+"/attempts",
		defaultToken, nil)
	if redeliver.StatusCode != http.StatusAccepted {
		redeliver.Body.Close()
		t.Fatalf("redeliver: got %d, want 202", redeliver.StatusCode)
	}
	body, _ := io.ReadAll(redeliver.Body)
	redeliver.Body.Close()
	if strings.TrimSpace(string(body)) != "" {
		t.Errorf("redeliver body = %q, want empty", body)
	}
}
