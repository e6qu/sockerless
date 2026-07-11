package bleephub

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"
)

func headShaForTest(t *testing.T, repo string) string {
	t.Helper()
	return headShaForRepoPath(t, "admin/"+repo)
}

func headShaForRepoPath(t *testing.T, repoFullName string) string {
	t.Helper()
	resp := ghGet(t, "/api/v3/repos/"+repoFullName+"/commits", defaultToken)
	commits := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(commits) == 0 {
		t.Fatal("repo has no commits")
	}
	return commits[0]["sha"].(string)
}

func submitSnapshotForTest(t *testing.T, repo, ref, sha, correlator string, purls ...string) map[string]interface{} {
	t.Helper()
	return submitSnapshotForRepoPath(t, "admin/"+repo, "go.mod", ref, sha, correlator, purls...)
}

func submitSnapshotForRepoPath(t *testing.T, repoFullName, manifestPath, ref, sha, correlator string, purls ...string) map[string]interface{} {
	t.Helper()
	resolved := map[string]interface{}{}
	for _, purl := range purls {
		resolved[purl] = map[string]interface{}{"package_url": purl, "scope": "runtime"}
	}
	resp := ghPost(t, "/api/v3/repos/"+repoFullName+"/dependency-graph/snapshots", defaultToken, map[string]interface{}{
		"version": 0,
		"ref":     ref,
		"sha":     sha,
		"job":     map[string]interface{}{"id": "job-1", "correlator": correlator},
		"detector": map[string]interface{}{
			"name": "bleephub-test-detector", "version": "1.0.0", "url": "https://example.com/detector",
		},
		"scanned": time.Now().UTC().Format(time.RFC3339),
		"manifests": map[string]interface{}{
			manifestPath: map[string]interface{}{
				"name":     manifestPath,
				"file":     map[string]interface{}{"source_location": manifestPath},
				"resolved": resolved,
			},
		},
	})
	return decodeJSONWithStatus(t, resp, 201)
}

func TestDependencyGraphSnapshots_SubmitAndSBOM(t *testing.T) {
	repo := createRepoWriteRepo(t, true)
	sha := headShaForTest(t, repo)

	// An SBOM with no snapshots honestly describes only the repository.
	resp := ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/sbom", defaultToken)
	doc := decodeJSONWithStatus(t, resp, 200)
	sbom, _ := doc["sbom"].(map[string]interface{})
	if sbom == nil {
		t.Fatalf("missing sbom: %v", doc)
	}
	if packages, _ := sbom["packages"].([]interface{}); len(packages) != 1 {
		t.Fatalf("empty-repo SBOM packages = %v, want just the repo package", sbom["packages"])
	}

	// Default-branch snapshot → SUCCESS.
	result := submitSnapshotForTest(t, repo, "refs/heads/main", sha, "ci/build",
		"pkg:golang/github.com/example/dep@v1.2.3")
	if result["result"] != "SUCCESS" {
		t.Fatalf("default-branch snapshot result = %v, want SUCCESS", result)
	}
	if result["id"] == nil || result["created_at"] == nil || result["message"] == nil {
		t.Fatalf("snapshot response missing members: %v", result)
	}

	// Non-default ref → ACCEPTED.
	result = submitSnapshotForTest(t, repo, "refs/heads/feature", strings.Repeat("a", 40), "ci/build",
		"pkg:golang/github.com/example/dep@v1.2.3")
	if result["result"] != "ACCEPTED" {
		t.Fatalf("feature-branch snapshot result = %v, want ACCEPTED", result)
	}

	// A malformed snapshot is stored with an honest INVALID result.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/snapshots", defaultToken, map[string]interface{}{
		"version": 0,
		"ref":     "refs/heads/main",
		"sha":     "short",
		"job":     map[string]interface{}{"id": "job-2", "correlator": "ci/build"},
		"detector": map[string]interface{}{
			"name": "bleephub-test-detector", "version": "1.0.0", "url": "https://example.com/detector",
		},
		"scanned": time.Now().UTC().Format(time.RFC3339),
	})
	invalid := decodeJSONWithStatus(t, resp, 201)
	if invalid["result"] != "INVALID" {
		t.Fatalf("malformed snapshot result = %v, want INVALID", invalid)
	}

	// The SBOM now includes the recorded default-branch dependency as a real
	// SPDX package with a purl external reference.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/sbom", defaultToken)
	doc = decodeJSONWithStatus(t, resp, 200)
	sbom = doc["sbom"].(map[string]interface{})
	if sbom["spdxVersion"] != "SPDX-2.3" || sbom["dataLicense"] != "CC0-1.0" || sbom["SPDXID"] != "SPDXRef-DOCUMENT" {
		t.Fatalf("SBOM document members wrong: %v", sbom)
	}
	packages, _ := sbom["packages"].([]interface{})
	if len(packages) != 2 {
		t.Fatalf("SBOM packages = %d entries, want repo + 1 dependency", len(packages))
	}
	dep := packages[1].(map[string]interface{})
	refs, _ := dep["externalRefs"].([]interface{})
	if len(refs) != 1 || refs[0].(map[string]interface{})["referenceLocator"] != "pkg:golang/github.com/example/dep@v1.2.3" {
		t.Fatalf("dependency externalRefs = %v", dep["externalRefs"])
	}
	relationships, _ := sbom["relationships"].([]interface{})
	if len(relationships) != 2 {
		t.Fatalf("SBOM relationships = %v, want DESCRIBES + DEPENDS_ON", relationships)
	}
}

func TestDependencyGraphSBOMReport_GenerateAndFetch(t *testing.T) {
	repo := createRepoWriteRepo(t, true)

	resp := ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/sbom/generate-report", defaultToken)
	report := decodeJSONWithStatus(t, resp, 201)
	sbomURL, _ := report["sbom_url"].(string)
	if sbomURL == "" {
		t.Fatalf("missing sbom_url: %v", report)
	}

	// The fetch step is a real redirect to the SBOM download location.
	req, _ := http.NewRequest("GET", sbomURL, nil)
	req.Header.Set("Authorization", "token "+defaultToken)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	redirectResp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	redirectResp.Body.Close()
	if redirectResp.StatusCode != 302 {
		t.Fatalf("fetch-report status = %d, want 302", redirectResp.StatusCode)
	}
	location := redirectResp.Header.Get("Location")
	if location == "" {
		t.Fatal("fetch-report missing Location")
	}

	// Following the redirect yields the SPDX document.
	followResp := ghGet(t, strings.TrimPrefix(location, testBaseURL), defaultToken)
	doc := decodeJSONWithStatus(t, followResp, 200)
	if doc["sbom"] == nil {
		t.Fatalf("redirect target did not serve an SBOM: %v", doc)
	}

	// Unknown report UUID → 404.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/sbom/fetch-report/not-a-report", defaultToken)
	requireStatus(t, resp, 404)
}

func TestDependencyGraphCompare_RealDiff(t *testing.T) {
	repo := createRepoWriteRepo(t, true)
	baseSha := headShaForTest(t, repo)

	// A second commit on main gives the head revision.
	resp := ghPut(t, "/api/v3/repos/admin/"+repo+"/contents/notes.txt", defaultToken, map[string]interface{}{
		"message": "add notes",
		"content": base64.StdEncoding.EncodeToString([]byte("notes\n")),
	})
	requireStatus(t, resp, 201)
	headSha := headShaForTest(t, repo)
	if headSha == baseSha {
		t.Fatal("contents PUT did not advance main")
	}

	submitSnapshotForTest(t, repo, "refs/heads/main", baseSha, "ci/build",
		"pkg:npm/left-pad@1.0.0", "pkg:npm/lodash@4.17.21")
	submitSnapshotForTest(t, repo, "refs/heads/main", headSha, "ci/build",
		"pkg:npm/lodash@4.17.21", "pkg:npm/chalk@5.3.0")

	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/compare/"+baseSha+"..."+headSha, defaultToken)
	diff := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(diff) != 2 {
		t.Fatalf("diff = %v, want one added + one removed", diff)
	}
	byType := map[string]map[string]interface{}{}
	for _, entry := range diff {
		byType[entry["change_type"].(string)] = entry
	}
	added := byType["added"]
	removed := byType["removed"]
	if added == nil || added["name"] != "chalk" || added["version"] != "5.3.0" || added["ecosystem"] != "npm" {
		t.Fatalf("added entry = %v, want npm chalk 5.3.0", added)
	}
	if removed == nil || removed["name"] != "left-pad" {
		t.Fatalf("removed entry = %v, want npm left-pad", removed)
	}
	if added["manifest"] != "go.mod" || added["scope"] != "runtime" {
		t.Fatalf("added entry manifest/scope = %v/%v", added["manifest"], added["scope"])
	}
	if _, has := added["vulnerabilities"]; !has {
		t.Fatal("diff entry missing vulnerabilities member")
	}

	// Malformed basehead → 400; unknown revisions → 404.
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/compare/justonerev", defaultToken)
	requireStatus(t, resp, 400)
	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/dependency-graph/compare/nope...alsonope", defaultToken)
	requireStatus(t, resp, 404)
}
