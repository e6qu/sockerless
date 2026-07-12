package bleephub

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var repoWriteRepoSeq int64

func pagesActionsArtifact(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var tarBytes bytes.Buffer
	tw := tar.NewWriter(&tarBytes)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write Pages TAR header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write Pages TAR content: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close Pages TAR: %v", err)
	}
	var artifact bytes.Buffer
	zw := zip.NewWriter(&artifact)
	w, err := zw.Create("artifact.tar")
	if err != nil {
		t.Fatalf("create Pages artifact.tar entry: %v", err)
	}
	if _, err := w.Write(tarBytes.Bytes()); err != nil {
		t.Fatalf("write Pages artifact.tar entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close Pages Actions artifact: %v", err)
	}
	return artifact.Bytes()
}

func mintPagesOIDCToken(t *testing.T, repo, sha, ref, environment string) string {
	return mintPagesOIDCTokenForAudience(t, repo, sha, ref, environment, "")
}

func mintPagesOIDCTokenForAudience(t *testing.T, repo, sha, ref, environment, audience string) string {
	t.Helper()
	q := url.Values{
		"repo":          {repo},
		"ref":           {ref},
		"sha":           {sha},
		"run_id":        {"42"},
		"run_number":    {"7"},
		"run_attempt":   {"1"},
		"workflow":      {"Pages"},
		"workflow_file": {"pages.yml"},
		"event_name":    {"push"},
		"environment":   {environment},
	}
	if audience != "" {
		q.Set("audience", audience)
	}
	resp := ghGet(t, "/token?"+q.Encode(), defaultToken)
	data := decodeJSONWithStatus(t, resp, http.StatusOK)
	token, ok := data["value"].(string)
	if !ok || token == "" {
		t.Fatalf("OpenID Connect token response = %v", data)
	}
	return token
}

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
	buildVersion := "0123456789abcdef0123456789abcdef01234567"

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
		"oidc_token":          mintPagesOIDCToken(t, "admin/"+repo, "bad-url", "refs/heads/main", "github-pages"),
	})
	requireStatus(t, resp, 502)
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken)
	site := decodeJSONWithStatus(t, resp, 200)
	if site["status"] != "building" {
		t.Fatalf("pages status after rejected artifact_url = %v, want building", site["status"])
	}
	oidcToken := mintPagesOIDCToken(t, "admin/"+repo, buildVersion, "refs/heads/main", "github-pages")
	tokenParts := strings.Split(oidcToken, ".")
	tamperedPrefix := "A"
	if strings.HasPrefix(tokenParts[2], tamperedPrefix) {
		tamperedPrefix = "B"
	}
	tokenParts[2] = tamperedPrefix + tokenParts[2][1:]
	tamperedToken := strings.Join(tokenParts, ".")
	for name, token := range map[string]string{
		"not a JWT":         "token",
		"altered signature": tamperedToken,
		"wrong ref":         mintPagesOIDCToken(t, "admin/"+repo, buildVersion, "refs/heads/other", "github-pages"),
		"wrong environment": mintPagesOIDCToken(t, "admin/"+repo, buildVersion, "refs/heads/main", "production"),
		"wrong SHA":         mintPagesOIDCToken(t, "admin/"+repo, "ffffffffffffffffffffffffffffffffffffffff", "refs/heads/main", "github-pages"),
		"wrong audience":    mintPagesOIDCTokenForAudience(t, "admin/"+repo, buildVersion, "refs/heads/main", "github-pages", "https://example.invalid/pages"),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			resp := ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
				"artifact_url":        testBaseURL + "/_apis/v1/artifacts/999999/download",
				"pages_build_version": buildVersion,
				"oidc_token":          token,
			})
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}

	artifactBytes := pagesActionsArtifact(t, map[string]string{
		"index.html":      "<h1>Published by Bleephub Pages</h1>",
		"docs/index.html": "<p>Documentation</p>",
		"about.html":      "<p>About</p>",
		"assets/site.css": "body { color: navy; }",
		"404.html":        "<h1>Custom missing page</h1>",
	})
	rootContent, rootName, err := readPagesArtifactFile(artifactBytes, "")
	if err != nil || rootName != "index.html" || string(rootContent) != "<h1>Published by Bleephub Pages</h1>" {
		t.Fatalf("read generated Pages root = (%q, %q, %v)", rootName, rootContent, err)
	}
	_, byteStore := newObjectByteStoreForTest(t)
	originalArtifactStore := testServer.artifactStore
	originalObjectByteStore := testServer.store.ObjectByteStore
	testServer.artifactStore = NewArtifactStoreWithByteStore("", byteStore)
	testServer.store.ObjectByteStore = byteStore
	t.Cleanup(func() {
		testServer.artifactStore = originalArtifactStore
		testServer.store.ObjectByteStore = originalObjectByteStore
	})
	invalidArtifact := []byte("not an archive")
	if err := byteStore.Put(context.Background(), artifactDataKey(4241), invalidArtifact); err != nil {
		t.Fatalf("put invalid Pages artifact: %v", err)
	}
	testServer.artifactStore.mu.Lock()
	testServer.artifactStore.artifacts[4241] = &Artifact{ID: 4241, Name: "invalid-pages", Size: int64(len(invalidArtifact)), Finalized: true, RepoFullName: "admin/" + repo, CreatedAt: time.Now()}
	testServer.artifactStore.mu.Unlock()
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_id":         4241,
		"pages_build_version": "invalid-archive",
		"oidc_token":          mintPagesOIDCToken(t, "admin/"+repo, "invalid-archive", "refs/heads/main", "github-pages"),
	})
	requireStatus(t, resp, 422)
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/pages", defaultToken)
	site = decodeJSONWithStatus(t, resp, 200)
	if site["status"] != "building" {
		t.Fatalf("pages status after invalid archive = %v, want building", site["status"])
	}
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
		"oidc_token":          oidcToken,
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
	wantPageURL := testBaseURL + "/pages/admin/" + repo + "/"
	if data["page_url"] != wantPageURL {
		t.Fatalf("page_url = %v, want %q", data["page_url"], wantPageURL)
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
	if deployment.ArtifactKey == "" {
		t.Fatal("deployment did not retain Pages artifact object key")
	}
	if got := readS3TestFile(t, byteStore.(*s3ActionsByteStore).fs, deployment.ArtifactKey); !bytes.Equal(got, artifactBytes) {
		t.Fatal("published Pages artifact bytes differ from deployment artifact")
	}
	for requestPath, want := range map[string]struct {
		status int
		body   string
	}{
		"":                {200, "<h1>Published by Bleephub Pages</h1>"},
		"docs/":           {200, "<p>Documentation</p>"},
		"about":           {200, "<p>About</p>"},
		"assets/site.css": {200, "body { color: navy; }"},
		"missing":         {404, "<h1>Custom missing page</h1>"},
	} {
		pageResp := ghGet(t, "/pages/admin/"+repo+"/"+requestPath, "")
		body, err := io.ReadAll(pageResp.Body)
		pageResp.Body.Close()
		if err != nil {
			t.Fatalf("read published Pages path %q: %v", requestPath, err)
		}
		if pageResp.StatusCode != want.status || string(body) != want.body {
			t.Fatalf("published Pages path %q = (%d, %q), want (%d, %q)", requestPath, pageResp.StatusCode, body, want.status, want.body)
		}
	}
	headResp := ghDo(t, http.MethodHead, "/pages/admin/"+repo+"/", "", nil)
	if headResp.StatusCode != http.StatusOK {
		t.Fatalf("Pages HEAD status = %d, want 200", headResp.StatusCode)
	}
	headBody, err := io.ReadAll(headResp.Body)
	headResp.Body.Close()
	if err != nil || len(headBody) != 0 {
		t.Fatalf("Pages HEAD body = %q, err = %v; want empty", headBody, err)
	}

	objectBuildVersion := "1123456789abcdef0123456789abcdef01234567"
	replacementBytes := pagesActionsArtifact(t, map[string]string{
		"index.html": "<h1>Replacement Pages deployment</h1>",
		"404.html":   "<h1>Replacement missing page</h1>",
	})
	if err := byteStore.Put(context.Background(), artifactDataKey(4242), replacementBytes); err != nil {
		t.Fatalf("replace object-backed artifact: %v", err)
	}
	testServer.artifactStore.mu.Lock()
	testServer.artifactStore.artifacts[4242].Size = int64(len(replacementBytes))
	testServer.artifactStore.mu.Unlock()
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/pages/deployments", defaultToken, map[string]interface{}{
		"artifact_id":         4242,
		"pages_build_version": objectBuildVersion,
		"oidc_token":          mintPagesOIDCToken(t, "admin/"+repo, objectBuildVersion, "refs/heads/main", "github-pages"),
	})
	requireStatus(t, resp, 200)
	objectDeployment := testServer.store.GetPagesDeploymentByIdentifier(int(repoData["id"].(float64)), objectBuildVersion)
	if objectDeployment == nil {
		t.Fatal("missing object-backed Pages deployment")
	}
	if objectDeployment.ArtifactSize != int64(len(replacementBytes)) {
		t.Fatalf("object-backed deployment artifact size = %d, want %d", objectDeployment.ArtifactSize, len(replacementBytes))
	}
	wantObjectSHA := fmt.Sprintf("sha256:%x", sha256.Sum256(replacementBytes))
	if objectDeployment.ArtifactSHA != wantObjectSHA {
		t.Fatalf("object-backed deployment artifact SHA = %q, want %q", objectDeployment.ArtifactSHA, wantObjectSHA)
	}
	if _, err := byteStore.Get(context.Background(), deployment.ArtifactKey); err == nil {
		t.Fatalf("superseded Pages object %q survived replacement", deployment.ArtifactKey)
	}
	replacementResp := ghGet(t, "/pages/admin/"+repo+"/", "")
	replacementBody, err := io.ReadAll(replacementResp.Body)
	replacementResp.Body.Close()
	if err != nil || replacementResp.StatusCode != http.StatusOK || string(replacementBody) != "<h1>Replacement Pages deployment</h1>" {
		t.Fatalf("replacement Pages root = (%d, %q, %v)", replacementResp.StatusCode, replacementBody, err)
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
	if _, err := byteStore.Get(context.Background(), objectDeployment.ArtifactKey); err == nil {
		t.Fatalf("published Pages object %q survived Pages deletion", objectDeployment.ArtifactKey)
	}
	resp = ghGet(t, "/api/v3/repos/admin/"+repo, defaultToken)
	repoData = decodeJSONWithStatus(t, resp, 200)
	if repoData["has_pages"] != false {
		t.Fatalf("repo has_pages after Pages delete = %v, want false", repoData["has_pages"])
	}
}

func TestPagesArtifactValidationRejectsUnsafeAndEmptyArchives(t *testing.T) {
	tests := []struct {
		name   string
		header *tar.Header
	}{
		{name: "symbolic link", header: &tar.Header{Name: "link", Linkname: "index.html", Typeflag: tar.TypeSymlink}},
		{name: "hard link", header: &tar.Header{Name: "link", Linkname: "index.html", Typeflag: tar.TypeLink}},
		{name: "path traversal", header: &tar.Header{Name: "../index.html", Mode: 0o644, Typeflag: tar.TypeReg}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var archive bytes.Buffer
			tw := tar.NewWriter(&archive)
			if err := tw.WriteHeader(tt.header); err != nil {
				t.Fatalf("write unsafe TAR header: %v", err)
			}
			if err := tw.Close(); err != nil {
				t.Fatalf("close unsafe TAR: %v", err)
			}
			if err := validatePagesArtifact(archive.Bytes()); err == nil {
				t.Fatal("unsafe Pages artifact passed validation")
			}
		})
	}
	var empty bytes.Buffer
	tw := tar.NewWriter(&empty)
	if err := tw.Close(); err != nil {
		t.Fatalf("close empty TAR: %v", err)
	}
	if err := validatePagesArtifact(empty.Bytes()); err == nil {
		t.Fatal("empty Pages artifact passed validation")
	}
}

func TestPagesPermissionIsDistinctFromAdministration(t *testing.T) {
	if !hasPerm(map[string]string{"pages": "write"}, scopePages, permWrite) {
		t.Fatal("pages:write did not authorize a Pages write")
	}
	if !hasPerm(map[string]string{"pages": "read"}, scopePages, permRead) {
		t.Fatal("pages:read did not authorize a Pages read")
	}
	if hasPerm(map[string]string{"pages": "read"}, scopePages, permWrite) {
		t.Fatal("pages:read authorized a Pages write")
	}
	if hasPerm(map[string]string{"administration": "write"}, scopePages, permRead) {
		t.Fatal("administration:write authorized a Pages read")
	}
	if !classicScopeCovers("repo", scopePages, permWrite) {
		t.Fatal("classic repo scope did not authorize a Pages write")
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
