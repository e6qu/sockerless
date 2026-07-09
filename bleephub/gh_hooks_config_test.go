package bleephub

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRepoWebhookConfig_GetAndPatch(t *testing.T) {
	repo := createRepoWriteRepo(t, false)

	resp := ghPost(t, "/api/v3/repos/admin/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{
			"url":          "https://example.com/webhook",
			"content_type": "json",
			"secret":       "s3cret",
		},
		"events": []string{"push"},
	})
	hook := decodeJSONWithStatus(t, resp, 201)
	hookID := fmt.Sprintf("%d", int(hook["id"].(float64)))
	base := "/api/v3/repos/admin/" + repo + "/hooks/" + hookID

	resp = ghGet(t, base+"/config", defaultToken)
	config := decodeJSONWithStatus(t, resp, 200)
	if config["url"] != "https://example.com/webhook" {
		t.Fatalf("config url = %v", config["url"])
	}
	if config["content_type"] != "json" {
		t.Fatalf("config content_type = %v", config["content_type"])
	}
	if config["insecure_ssl"] != "0" {
		t.Fatalf("config insecure_ssl = %v, want \"0\"", config["insecure_ssl"])
	}
	// A configured secret is masked, never echoed back.
	if config["secret"] != "********" {
		t.Fatalf("config secret = %v, want masked", config["secret"])
	}

	resp = ghPatch(t, base+"/config", defaultToken, map[string]interface{}{
		"url":          "https://example.com/webhook2",
		"content_type": "form",
		"insecure_ssl": "1",
	})
	config = decodeJSONWithStatus(t, resp, 200)
	if config["url"] != "https://example.com/webhook2" || config["content_type"] != "form" || config["insecure_ssl"] != "1" {
		t.Fatalf("patched config = %v", config)
	}

	// The config change is a view of the same webhook: the hook object
	// reflects it.
	resp = ghGet(t, base, defaultToken)
	hook = decodeJSONWithStatus(t, resp, 200)
	hookConfig, _ := hook["config"].(map[string]interface{})
	if hookConfig["url"] != "https://example.com/webhook2" {
		t.Fatalf("hook config url = %v, want the PATCHed url", hookConfig["url"])
	}

	// Unknown hook → 404.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/hooks/424242/config", defaultToken)
	requireStatus(t, resp, 404)
	resp = ghPatch(t, "/api/v3/repos/admin/"+repo+"/hooks/424242/config", defaultToken, map[string]interface{}{"url": "x"})
	requireStatus(t, resp, 404)
}

func TestRepoWebhookTest_DeliversRealPushEvent(t *testing.T) {
	repo := createRepoWriteRepo(t, true)
	resp := ghPut(t, "/api/v3/repos/admin/"+repo+"/contents/webhook-test.txt", defaultToken, map[string]interface{}{
		"message": "seed webhook test head",
		"content": base64.StdEncoding.EncodeToString([]byte("webhook test\n")),
		"branch":  "main",
	})
	requireStatus(t, resp, 201)

	received := make(chan *http.Request, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := r.Clone(r.Context())
		req.Body = io.NopCloser(strings.NewReader(string(body)))
		select {
		case received <- req:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": receiver.URL, "content_type": "json", "secret": "hush"},
		"events": []string{"push"},
		"active": false,
	})
	hook := decodeJSONWithStatus(t, resp, 201)
	hookID := fmt.Sprintf("%d", int(hook["id"].(float64)))

	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/hooks/"+hookID+"/tests", defaultToken, nil)
	requireStatus(t, resp, 204)

	select {
	case req := <-received:
		if got := req.Header.Get("X-GitHub-Event"); got != "push" {
			t.Fatalf("X-GitHub-Event = %q, want push", got)
		}
		body, _ := io.ReadAll(req.Body)
		if strings.Contains(string(body), "0000000000000000000000000000000000000000") {
			t.Fatalf("test delivery payload contained all-zero SHA: %s", body)
		}
		if req.Header.Get("X-Hub-Signature-256") == "" {
			t.Fatal("test delivery missing X-Hub-Signature-256 for a secret-bearing hook")
		}
		if req.Header.Get("X-GitHub-Delivery") == "" {
			t.Fatal("test delivery missing X-GitHub-Delivery")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("hook test did not deliver a push event")
	}

	// Unknown hook → 404.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/hooks/424242/tests", defaultToken, nil)
	requireStatus(t, resp, 404)
}

func TestRepoWebhookTest_RejectsMissingDefaultBranchHead(t *testing.T) {
	repo := createRepoWriteRepo(t, false)

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("webhook test delivered despite missing default-branch head")
	}))
	defer receiver.Close()

	resp := ghPost(t, "/api/v3/repos/admin/"+repo+"/hooks", defaultToken, map[string]interface{}{
		"config": map[string]interface{}{"url": receiver.URL, "content_type": "json"},
		"events": []string{"push"},
		"active": false,
	})
	hook := decodeJSONWithStatus(t, resp, 201)
	hookID := fmt.Sprintf("%d", int(hook["id"].(float64)))

	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/hooks/"+hookID+"/tests", defaultToken, nil)
	requireStatus(t, resp, 422)
}
