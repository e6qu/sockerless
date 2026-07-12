package bleephub

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
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
	buildVersion := "abc123"

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
	// An artifact_url must be readable before the deployment can succeed.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_url":        "://not-a-url",
		"pages_build_version": "bad-url",
		"oidc_token":          "token",
	})
	requireStatus(t, resp, 502)
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken)
	site := decodeJSONWithStatus(t, resp, 200)
	if site["status"] != "building" {
		t.Fatalf("pages status after rejected artifact_url = %v, want building", site["status"])
	}

	artifactBytes := []byte("pages artifact archive bytes")
	_, byteStore := newObjectByteStoreForTest(t)
	originalArtifactStore := testServer.artifactStore
	testServer.artifactStore = NewArtifactStoreWithByteStore("", byteStore)
	t.Cleanup(func() {
		testServer.artifactStore = originalArtifactStore
	})
	if err := byteStore.Put(context.Background(), artifactDataKey(4242), artifactBytes); err != nil {
		t.Fatalf("put object-backed artifact: %v", err)
	}
	testServer.artifactStore.mu.Lock()
	testServer.artifactStore.artifacts[4242] = &Artifact{
		ID:           4242,
		Name:         "pages-object-artifact",
		Size:         int64(len(artifactBytes)),
		Finalized:    true,
		RepoFullName: "admin/" + repo,
		CreatedAt:    time.Now(),
	}
	testServer.artifactStore.nextID = 4243
	testServer.artifactStore.mu.Unlock()
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_url":        testBaseURL + "/_apis/v1/artifacts/4242/download",
		"pages_build_version": buildVersion,
		"oidc_token":          "token",
	})
	data := decodeJSONWithStatus(t, resp, 200)
	id, ok := data["id"].(string)
	if !ok || id != buildVersion {
		t.Fatalf("id = %v, want public pages build version %q", data["id"], buildVersion)
	}
	statusURL, _ := data["status_url"].(string)
	if statusURL == "" {
		t.Fatal("missing status_url")
	}
	parsedStatusURL, err := url.Parse(statusURL)
	if err != nil {
		t.Fatalf("status_url did not parse: %v", err)
	}
	wantStatusPath := "/api/v3/repos/admin/" + repo + "/pages/deployments/" + buildVersion + "/status"
	if parsedStatusURL.Path != wantStatusPath {
		t.Fatalf("status_url path = %q, want %q", parsedStatusURL.Path, wantStatusPath)
	}
	if data["page_url"] == "" {
		t.Fatal("missing page_url")
	}

	resp = ghGet(t, parsedStatusURL.Path, defaultToken)
	status := decodeJSONWithStatus(t, resp, 200)
	if status["status"] != "succeed" {
		t.Fatalf("status = %v, want succeed", status["status"])
	}
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages/deployments/"+buildVersion, defaultToken)
	status = decodeJSONWithStatus(t, resp, 200)
	if status["status"] != "succeed" {
		t.Fatalf("status by build version = %v, want succeed", status["status"])
	}

	// The publish flipped the Pages site to built.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken)
	site = decodeJSONWithStatus(t, resp, 200)
	if site["status"] != "built" {
		t.Fatalf("pages status = %v, want built", site["status"])
	}
	deployment := testServer.store.GetPagesDeploymentByIdentifier(int(repoData["id"].(float64)), buildVersion)
	if deployment == nil {
		t.Fatal("missing stored Pages deployment")
	}
	if deployment.ArtifactSize != int64(len(artifactBytes)) {
		t.Fatalf("deployment artifact size = %d, want %d", deployment.ArtifactSize, len(artifactBytes))
	}
	wantArtifactSHA := fmt.Sprintf("sha256:%x", sha256.Sum256(artifactBytes))
	if deployment.ArtifactSHA != wantArtifactSHA {
		t.Fatalf("deployment artifact SHA = %q, want %q", deployment.ArtifactSHA, wantArtifactSHA)
	}

	objectBuildVersion := buildVersion + "-object"
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_id":         4242,
		"pages_build_version": objectBuildVersion,
		"oidc_token":          "token",
	})
	requireStatus(t, resp, 200)
	objectDeployment := testServer.store.GetPagesDeploymentByIdentifier(int(repoData["id"].(float64)), objectBuildVersion)
	if objectDeployment == nil {
		t.Fatal("missing object-backed Pages deployment")
	}
	if objectDeployment.ArtifactSize != int64(len(artifactBytes)) {
		t.Fatalf("object-backed deployment artifact size = %d, want %d", objectDeployment.ArtifactSize, len(artifactBytes))
	}
	wantObjectSHA := fmt.Sprintf("sha256:%x", sha256.Sum256(artifactBytes))
	if objectDeployment.ArtifactSHA != wantObjectSHA {
		t.Fatalf("object-backed deployment artifact SHA = %q, want %q", objectDeployment.ArtifactSHA, wantObjectSHA)
	}

	// A synchronously completed deployment is terminal — not cancellable.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments/"+buildVersion+"/cancel", defaultToken, nil)
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
