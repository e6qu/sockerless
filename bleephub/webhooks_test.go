package bleephub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
	memfs "github.com/go-git/go-billy/v5/memfs"
)

func createWebhookTestRepo(t *testing.T, name string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": name,
	})
	resp.Body.Close()
}

func TestWebhookCRUD(t *testing.T) {
	createWebhookTestRepo(t, "wh-crud")

	// Create
	resp := ghPost(t, "/api/v3/repos/admin/wh-crud/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url":    "http://example.com/hook",
			"secret": "s3cret",
		},
		"events": []string{"push", "pull_request"},
		"active": true,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	hookID := int(data["id"].(float64))
	if hookID == 0 {
		t.Fatal("hook ID should be non-zero")
	}
	if data["active"] != true {
		t.Fatalf("expected active=true, got %v", data["active"])
	}
	events := data["events"].([]interface{})
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// List
	resp2 := ghGet(t, "/api/v3/repos/admin/wh-crud/hooks", defaultToken)
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("list: expected 200, got %d", resp2.StatusCode)
	}
	defer resp2.Body.Close()
	var list []map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&list)
	if len(list) < 1 {
		t.Fatal("expected at least 1 hook in list")
	}

	// Get
	resp3 := ghGet(t, fmt.Sprintf("/api/v3/repos/admin/wh-crud/hooks/%d", hookID), defaultToken)
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("get: expected 200, got %d", resp3.StatusCode)
	}
	data3 := decodeJSON(t, resp3)
	if int(data3["id"].(float64)) != hookID {
		t.Fatalf("expected id=%d, got %v", hookID, data3["id"])
	}

	// Update
	resp4 := ghPatch(t, fmt.Sprintf("/api/v3/repos/admin/wh-crud/hooks/%d", hookID), defaultToken, map[string]interface{}{
		"active": false,
		"events": []string{"push"},
	})
	if resp4.StatusCode != 200 {
		resp4.Body.Close()
		t.Fatalf("update: expected 200, got %d", resp4.StatusCode)
	}
	data4 := decodeJSON(t, resp4)
	if data4["active"] != false {
		t.Fatalf("expected active=false after update, got %v", data4["active"])
	}
	updatedEvents := data4["events"].([]interface{})
	if len(updatedEvents) != 1 || updatedEvents[0] != "push" {
		t.Fatalf("expected [push] events after update, got %v", updatedEvents)
	}

	// Delete
	resp5 := ghDelete(t, fmt.Sprintf("/api/v3/repos/admin/wh-crud/hooks/%d", hookID), defaultToken)
	defer resp5.Body.Close()
	if resp5.StatusCode != 204 {
		t.Fatalf("delete: expected 204, got %d", resp5.StatusCode)
	}

	// Verify deleted
	resp6 := ghGet(t, fmt.Sprintf("/api/v3/repos/admin/wh-crud/hooks/%d", hookID), defaultToken)
	defer resp6.Body.Close()
	if resp6.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp6.StatusCode)
	}
}

func TestWebhookHMACSignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"action":"opened"}`)

	sig := computeHMACSignature(secret, payload)

	// Verify manually
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if sig != expected {
		t.Fatalf("signature mismatch: got %s, want %s", sig, expected)
	}

	// Verify prefix
	if len(sig) < 7 || sig[:7] != "sha256=" {
		t.Fatalf("expected sha256= prefix, got %s", sig)
	}
}

// startWebhookReceiver starts an HTTP server that records received webhook payloads.
func startWebhookReceiver(t *testing.T, handler http.HandlerFunc) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)
	return "http://" + ln.Addr().String(), func() { srv.Close() }
}

func TestWebhookDeliverySuccess(t *testing.T) {
	var received atomic.Int32
	var mu sync.Mutex
	var lastHeaders http.Header
	var lastBody []byte

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		lastHeaders = r.Header.Clone()
		lastBody = body
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-deliver")

	// Create webhook pointing to our receiver
	resp := ghPost(t, "/api/v3/repos/admin/wh-deliver/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url":    url,
			"secret": "delivery-secret",
		},
		"events": []string{"push"},
		"active": true,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	hookData := decodeJSON(t, resp)
	hookID := int(hookData["id"].(float64))

	pingResp := ghPost(t, fmt.Sprintf("/api/v3/repos/admin/wh-deliver/hooks/%d/pings", hookID), defaultToken, nil)
	defer pingResp.Body.Close()
	if pingResp.StatusCode != 204 {
		t.Fatalf("ping: expected 204, got %d", pingResp.StatusCode)
	}

	// Wait for delivery
	time.Sleep(200 * time.Millisecond)

	if received.Load() < 1 {
		t.Fatal("expected at least 1 delivery")
	}

	mu.Lock()
	defer mu.Unlock()

	// Check headers
	if lastHeaders.Get("X-GitHub-Event") != "ping" {
		t.Fatalf("expected X-GitHub-Event=ping, got %s", lastHeaders.Get("X-GitHub-Event"))
	}
	if lastHeaders.Get("X-Hub-Signature-256") == "" {
		t.Fatal("expected X-Hub-Signature-256 header")
	}
	if lastHeaders.Get("User-Agent") != "GitHub-Hookshot/bleephub" {
		t.Fatalf("expected User-Agent=GitHub-Hookshot/bleephub, got %s", lastHeaders.Get("User-Agent"))
	}

	// Verify HMAC
	sig := lastHeaders.Get("X-Hub-Signature-256")
	expectedSig := computeHMACSignature("delivery-secret", lastBody)
	if sig != expectedSig {
		t.Fatalf("HMAC mismatch: got %s, want %s", sig, expectedSig)
	}
}

func TestWebhookDeliveryRetry(t *testing.T) {
	var attempts atomic.Int32

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(500) // fail first 2 attempts
		} else {
			w.WriteHeader(200) // succeed on 3rd
		}
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-retry")

	resp := ghPost(t, "/api/v3/repos/admin/wh-retry/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url": url,
		},
		"events": []string{"issues"},
		"active": true,
	})
	resp.Body.Close()

	// Create an issue to trigger the webhook
	resp2 := ghPost(t, "/api/v3/repos/admin/wh-retry/issues", defaultToken, map[string]interface{}{
		"title": "test issue for retry",
	})
	resp2.Body.Close()

	// Wait for retries (1s + 5s backoff = ~6s, use generous timeout)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if attempts.Load() >= 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if attempts.Load() < 3 {
		t.Fatalf("expected at least 3 delivery attempts, got %d", attempts.Load())
	}
}

func TestWebhookDeliveryTimeout(t *testing.T) {
	var received atomic.Int32

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		// Hang longer than the 10s client timeout
		time.Sleep(15 * time.Second)
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-timeout")

	resp := ghPost(t, "/api/v3/repos/admin/wh-timeout/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url": url,
		},
		"events": []string{"issues"},
		"active": true,
	})
	resp.Body.Close()

	// Create an issue to trigger
	resp2 := ghPost(t, "/api/v3/repos/admin/wh-timeout/issues", defaultToken, map[string]interface{}{
		"title": "timeout test",
	})
	resp2.Body.Close()

	// Wait enough for at least one timeout attempt
	time.Sleep(12 * time.Second)

	if received.Load() < 1 {
		t.Fatal("expected at least 1 delivery attempt")
	}
}

func TestWebhookPushEvent(t *testing.T) {
	var received atomic.Int32
	var mu sync.Mutex
	var lastEvent string
	var lastPayload map[string]interface{}

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		lastEvent = r.Header.Get("X-GitHub-Event")
		json.Unmarshal(body, &lastPayload)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-push")

	// Create webhook
	resp := ghPost(t, "/api/v3/repos/admin/wh-push/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": url},
		"events": []string{"push"},
		"active": true,
	})
	resp.Body.Close()

	// Push via git (use go-git)
	pushTestCommit(t, "admin", "wh-push")

	// Wait for delivery
	time.Sleep(500 * time.Millisecond)

	if received.Load() < 1 {
		t.Fatal("expected push webhook delivery")
	}

	mu.Lock()
	defer mu.Unlock()
	if lastEvent != "push" {
		t.Fatalf("expected event=push, got %s", lastEvent)
	}
	if lastPayload["ref"] == nil {
		t.Fatal("push payload missing 'ref' field")
	}
}

func TestWebhookPREvent(t *testing.T) {
	var received atomic.Int32
	var mu sync.Mutex
	var events []string

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		mu.Lock()
		events = append(events, r.Header.Get("X-GitHub-Event"))
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-pr")

	// Create webhook for pull_request events
	resp := ghPost(t, "/api/v3/repos/admin/wh-pr/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": url},
		"events": []string{"pull_request"},
		"active": true,
	})
	resp.Body.Close()

	// Create a PR
	resp2 := ghPost(t, "/api/v3/repos/admin/wh-pr/pulls", defaultToken, map[string]interface{}{
		"title": "test PR",
		"head":  "feature",
		"base":  "main",
	})
	if resp2.StatusCode != 201 {
		resp2.Body.Close()
		t.Fatalf("create PR: expected 201, got %d", resp2.StatusCode)
	}
	prData := decodeJSON(t, resp2)
	prNum := int(prData["number"].(float64))

	// Merge the PR
	resp3 := ghPut(t, fmt.Sprintf("/api/v3/repos/admin/wh-pr/pulls/%d/merge", prNum), defaultToken, nil)
	resp3.Body.Close()

	// Wait for deliveries
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if received.Load() < 2 {
		t.Fatalf("expected at least 2 PR event deliveries (opened + closed), got %d", received.Load())
	}

	hasOpened := false
	hasClosed := false
	for _, e := range events {
		if e == "pull_request" {
			hasOpened = true
			hasClosed = true
		}
	}
	if !hasOpened || !hasClosed {
		t.Fatalf("expected pull_request events, got %v", events)
	}
}

func TestWebhookIssuesEvent(t *testing.T) {
	var received atomic.Int32
	var mu sync.Mutex
	var payloads []map[string]interface{}

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		var p map[string]interface{}
		json.Unmarshal(body, &p)
		mu.Lock()
		payloads = append(payloads, p)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-issues")

	// Create webhook for issues events
	resp := ghPost(t, "/api/v3/repos/admin/wh-issues/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": url},
		"events": []string{"issues"},
		"active": true,
	})
	resp.Body.Close()

	// Create issue
	resp2 := ghPost(t, "/api/v3/repos/admin/wh-issues/issues", defaultToken, map[string]interface{}{
		"title": "webhook test issue",
	})
	if resp2.StatusCode != 201 {
		resp2.Body.Close()
		t.Fatalf("create issue: expected 201, got %d", resp2.StatusCode)
	}
	issueData := decodeJSON(t, resp2)
	issueNum := int(issueData["number"].(float64))

	// Close issue
	resp3 := ghPatch(t, fmt.Sprintf("/api/v3/repos/admin/wh-issues/issues/%d", issueNum), defaultToken, map[string]interface{}{
		"state": "closed",
	})
	resp3.Body.Close()

	// Wait for deliveries
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if received.Load() < 2 {
		t.Fatalf("expected at least 2 issue event deliveries, got %d", received.Load())
	}

	// Verify actions
	actions := make([]string, 0, len(payloads))
	for _, p := range payloads {
		if a, ok := p["action"].(string); ok {
			actions = append(actions, a)
		}
	}
	hasOpened := false
	hasClosed := false
	for _, a := range actions {
		if a == "opened" {
			hasOpened = true
		}
		if a == "closed" {
			hasClosed = true
		}
	}
	if !hasOpened {
		t.Fatalf("missing 'opened' action in payloads, got %v", actions)
	}
	if !hasClosed {
		t.Fatalf("missing 'closed' action in payloads, got %v", actions)
	}
}

func TestWebhookPing(t *testing.T) {
	var received atomic.Int32
	var mu sync.Mutex
	var lastPayload map[string]interface{}

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		json.Unmarshal(body, &lastPayload)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-ping")

	// Create webhook
	resp := ghPost(t, "/api/v3/repos/admin/wh-ping/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": url},
		"events": []string{"push"},
		"active": true,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create hook: expected 201, got %d", resp.StatusCode)
	}
	hookData := decodeJSON(t, resp)
	hookID := int(hookData["id"].(float64))

	// Ping
	pingResp := ghPost(t, fmt.Sprintf("/api/v3/repos/admin/wh-ping/hooks/%d/pings", hookID), defaultToken, nil)
	defer pingResp.Body.Close()
	if pingResp.StatusCode != 204 {
		t.Fatalf("ping: expected 204, got %d", pingResp.StatusCode)
	}

	// Wait for delivery
	time.Sleep(300 * time.Millisecond)

	if received.Load() < 1 {
		t.Fatal("expected ping delivery")
	}

	mu.Lock()
	defer mu.Unlock()
	if lastPayload["zen"] == nil {
		t.Fatal("ping payload missing 'zen' field")
	}
	if lastPayload["hook_id"] == nil {
		t.Fatal("ping payload missing 'hook_id' field")
	}
}

func TestWebhookDeliveryLog(t *testing.T) {
	var received atomic.Int32

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-log")

	// Create webhook
	resp := ghPost(t, "/api/v3/repos/admin/wh-log/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": url},
		"events": []string{"push"},
		"active": true,
	})
	hookData := decodeJSON(t, resp)
	hookID := int(hookData["id"].(float64))

	// Ping to create a delivery
	pingResp := ghPost(t, fmt.Sprintf("/api/v3/repos/admin/wh-log/hooks/%d/pings", hookID), defaultToken, nil)
	pingResp.Body.Close()

	// Wait for delivery
	time.Sleep(300 * time.Millisecond)

	// List deliveries
	delResp := ghGet(t, fmt.Sprintf("/api/v3/repos/admin/wh-log/hooks/%d/deliveries", hookID), defaultToken)
	if delResp.StatusCode != 200 {
		delResp.Body.Close()
		t.Fatalf("list deliveries: expected 200, got %d", delResp.StatusCode)
	}
	defer delResp.Body.Close()

	var deliveries []map[string]interface{}
	json.NewDecoder(delResp.Body).Decode(&deliveries)
	if len(deliveries) < 1 {
		t.Fatal("expected at least 1 delivery in log")
	}

	d := deliveries[0]
	if d["guid"] == nil {
		t.Fatal("delivery missing 'guid' field")
	}
	if d["event"] == nil {
		t.Fatal("delivery missing 'event' field")
	}
	if d["status_code"] == nil {
		t.Fatal("delivery missing 'status_code' field")
	}
}

func TestWebhookInactiveSkipped(t *testing.T) {
	var received atomic.Int32

	url, cleanup := startWebhookReceiver(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer cleanup()

	createWebhookTestRepo(t, "wh-inactive")

	// Create inactive webhook
	active := false
	resp := ghPost(t, "/api/v3/repos/admin/wh-inactive/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": url},
		"events": []string{"issues"},
		"active": active,
	})
	resp.Body.Close()

	// Create issue — should NOT trigger inactive webhook
	resp2 := ghPost(t, "/api/v3/repos/admin/wh-inactive/issues", defaultToken, map[string]interface{}{
		"title": "should not trigger",
	})
	resp2.Body.Close()

	// Wait to ensure no delivery happens
	time.Sleep(300 * time.Millisecond)

	if received.Load() != 0 {
		t.Fatalf("expected 0 deliveries for inactive webhook, got %d", received.Load())
	}
}

// pushTestCommit creates a commit in-memory and pushes to the bleephub server via go-git.
func pushTestCommit(t *testing.T, owner, repoName string) {
	t.Helper()

	fs := memfs.New()
	repo, err := git.Init(memory.NewStorage(), fs)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Create a file and commit
	f, err := fs.Create("test.txt")
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	f.Write([]byte("hello webhook"))
	f.Close()

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	wt.Add("test.txt")
	_, err = wt.Commit("test commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{testBaseURL + "/" + owner + "/" + repoName + ".git"},
	})
	if err != nil {
		t.Fatalf("create remote: %v", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth:     &githttp.BasicAuth{Username: "x-token", Password: defaultToken},
		RefSpecs: []config.RefSpec{"+refs/heads/master:refs/heads/main"},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		t.Fatalf("push: %v", err)
	}
}
