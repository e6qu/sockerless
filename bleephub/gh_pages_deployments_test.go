package bleephub

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

var repoWriteRepoSeq int64

// createRepoWriteRepo creates a repo owned by admin through the API,
// optionally with an auto-init initial commit. Returns the repo name.
func createRepoWriteRepo(t *testing.T, autoInit bool) string {
	t.Helper()
	name := fmt.Sprintf("rw-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&repoWriteRepoSeq, 1))
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":      name,
		"auto_init": autoInit,
	})
	requireStatus(t, resp, 201)
	return name
}

func TestPagesDeployments_CreateStatusCancel(t *testing.T) {
	repo := createRepoWriteRepo(t, true)

	resp := ghGet(t, "/api/v3/repos/admin/"+repo, defaultToken)
	repoData := decodeJSONWithStatus(t, resp, 200)
	if repoData["has_pages"] != false {
		t.Fatalf("repo has_pages before Pages create = %v, want false", repoData["has_pages"])
	}

	// A deployment for a repo without a Pages site is a 404.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_url":        "https://example.invalid/artifact.zip",
		"pages_build_version": "abc123",
		"oidc_token":          "token",
	})
	requireStatus(t, resp, 404)

	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken, map[string]interface{}{
		"source": map[string]interface{}{"branch": "main", "path": "/"},
	})
	requireStatus(t, resp, 201)
	resp = ghGet(t, "/api/v3/repos/admin/"+repo, defaultToken)
	repoData = decodeJSONWithStatus(t, resp, 200)
	if repoData["has_pages"] != true {
		t.Fatalf("repo has_pages after Pages create = %v, want true", repoData["has_pages"])
	}

	// Required members are validated.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_url": "https://example.invalid/artifact.zip",
		"oidc_token":   "token",
	})
	requireStatus(t, resp, 422)
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_url":        "https://example.invalid/artifact.zip",
		"pages_build_version": "abc123",
	})
	requireStatus(t, resp, 422)
	// Either artifact_id or artifact_url is required.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"pages_build_version": "abc123",
		"oidc_token":          "token",
	})
	requireStatus(t, resp, 400)
	// An artifact_id the repository does not own is rejected.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_id":         999999,
		"pages_build_version": "abc123",
		"oidc_token":          "token",
	})
	requireStatus(t, resp, 400)

	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_url":        "https://example.invalid/artifact.zip",
		"pages_build_version": "abc123",
		"oidc_token":          "token",
	})
	data := decodeJSONWithStatus(t, resp, 200)
	id, ok := data["id"].(float64)
	if !ok {
		t.Fatalf("id = %v, want number", data["id"])
	}
	statusURL, _ := data["status_url"].(string)
	if statusURL == "" {
		t.Fatal("missing status_url")
	}
	if data["page_url"] == "" {
		t.Fatal("missing page_url")
	}

	idStr := strconv.Itoa(int(id))
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages/deployments/"+idStr, defaultToken)
	status := decodeJSONWithStatus(t, resp, 200)
	if status["status"] != "succeed" {
		t.Fatalf("status = %v, want succeed", status["status"])
	}

	// The publish flipped the Pages site to built.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken)
	site := decodeJSONWithStatus(t, resp, 200)
	if site["status"] != "built" {
		t.Fatalf("pages status = %v, want built", site["status"])
	}

	// A synchronously completed deployment is terminal — not cancellable.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments/"+idStr+"/cancel", defaultToken, nil)
	requireStatus(t, resp, 422)

	// Unknown deployment IDs are 404 for status and cancel.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages/deployments/424242", defaultToken)
	requireStatus(t, resp, 404)
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments/424242/cancel", defaultToken, nil)
	requireStatus(t, resp, 404)

	resp = ghDelete(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken)
	requireStatus(t, resp, 204)
	resp = ghGet(t, "/api/v3/repos/admin/"+repo, defaultToken)
	repoData = decodeJSONWithStatus(t, resp, 200)
	if repoData["has_pages"] != false {
		t.Fatalf("repo has_pages after Pages delete = %v, want false", repoData["has_pages"])
	}
}

func TestPagesHealthCheck(t *testing.T) {
	repo := createRepoWriteRepo(t, true)

	// No Pages site → 404.
	resp := ghGet(t, "/api/v3/repos/admin/"+repo+"/pages/health", defaultToken)
	requireStatus(t, resp, 404)

	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken, map[string]interface{}{
		"source": map[string]interface{}{"branch": "main"},
	})
	requireStatus(t, resp, 201)

	// No custom domain → 400.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages/health", defaultToken)
	requireStatus(t, resp, 400)

	cname := "localhost"
	resp = ghPut(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken, map[string]interface{}{"cname": cname})
	requireStatus(t, resp, 204)

	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages/health", defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	domain, _ := data["domain"].(map[string]interface{})
	if domain == nil {
		t.Fatalf("missing domain object: %v", data)
	}
	if domain["host"] != cname {
		t.Fatalf("domain.host = %v, want %s", domain["host"], cname)
	}
	if domain["dns_resolves"] != true {
		t.Fatalf("domain.dns_resolves = %v, want true (localhost resolves)", domain["dns_resolves"])
	}
	if domain["is_valid_domain"] != true {
		t.Fatalf("domain.is_valid_domain = %v, want true", domain["is_valid_domain"])
	}
	if _, has := data["alt_domain"]; !has {
		t.Fatal("missing alt_domain member")
	}
}
