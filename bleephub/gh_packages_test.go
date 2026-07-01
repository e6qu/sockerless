package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
)

func seedPackageVersion(t *testing.T, ownerType, owner, pkgType, pkgName, version string) (int, int) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"version":     version,
		"description": "test version",
		"metadata": map[string]any{
			"package_type": pkgType,
			"container":    map[string]any{"tags": []string{"latest"}},
		},
		"files": []map[string]any{
			{
				"name":           "package.tgz",
				"content_type":   "application/gzip",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("hello package")),
			},
		},
	})
	resp, err := authedPost("/internal/packages/"+ownerType+"/"+owner+"/"+pkgType+"/"+pkgName+"/versions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("seed package version: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("seed package version: %d %s", resp.StatusCode, b)
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode seeded version: %v", err)
	}
	resp.Body.Close()
	pkgID := 0
	if u := testServer.store.GetPackage(owner, pkgType, pkgName); u != nil {
		pkgID = u.ID
	}
	return pkgID, int(v["id"].(float64))
}

func TestPackages_UserCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	pkgID, versionID := seedPackageVersion(t, "user", admin.Login, "container", "user-pkg", "1.0.0")

	// List user packages
	resp := ghGet(t, "/api/v3/users/"+admin.Login+"/packages", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list user packages: %d %s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, p := range list {
		if p["name"] == "user-pkg" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("seeded user-pkg not in list: %v", list)
	}

	// Get package
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get user package: %d %s", resp.StatusCode, b)
	}
	pkg := decodeJSON(t, resp)
	if pkg["id"] == nil {
		t.Fatal("missing package id")
	}
	if pkg["owner"].(map[string]any)["login"] != admin.Login {
		t.Fatalf("expected owner login %s, got %v", admin.Login, pkg["owner"])
	}

	// List versions
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list versions: %d %s", resp.StatusCode, b)
	}
	var versions []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		t.Fatalf("decode versions: %v", err)
	}
	resp.Body.Close()
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0]["name"] != "1.0.0" {
		t.Fatalf("expected version 1.0.0, got %v", versions[0]["name"])
	}

	// Get version
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions/"+strconv.Itoa(versionID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get version: %d %s", resp.StatusCode, b)
	}
	version := decodeJSON(t, resp)
	if version["name"] != "1.0.0" {
		t.Fatalf("expected version name 1.0.0, got %v", version["name"])
	}

	// List files
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions/"+strconv.Itoa(versionID)+"/files", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list files: %d %s", resp.StatusCode, b)
	}
	var files []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		t.Fatalf("decode files: %v", err)
	}
	resp.Body.Close()
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	fileID := int(files[0]["id"].(float64))
	if files[0]["name"] != "package.tgz" {
		t.Fatalf("expected package.tgz, got %v", files[0]["name"])
	}

	// Download file
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions/"+strconv.Itoa(versionID)+"/files/"+strconv.Itoa(fileID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("download file: %d %s", resp.StatusCode, b)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "hello package" {
		t.Fatalf("expected file body hello package, got %q", string(data))
	}

	// Delete version
	resp = ghDelete(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions/"+strconv.Itoa(versionID), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete version: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// List versions after delete excludes deleted
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions", defaultToken)
	versions = nil
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		t.Fatalf("decode versions after delete: %v", err)
	}
	resp.Body.Close()
	if len(versions) != 0 {
		t.Fatalf("expected 0 versions after delete, got %d", len(versions))
	}

	// Restore version
	resp = ghPost(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions/"+strconv.Itoa(versionID)+"/restore", defaultToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("restore version: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg/versions", defaultToken)
	versions = nil
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		t.Fatalf("decode versions after restore: %v", err)
	}
	resp.Body.Close()
	if len(versions) != 1 {
		t.Fatalf("expected 1 version after restore, got %d", len(versions))
	}

	// Delete package
	resp = ghDelete(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete package: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Get package after delete returns 404
	resp = ghGet(t, "/api/v3/users/"+admin.Login+"/packages/container/user-pkg", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after package delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	_ = pkgID
}

func TestPackages_OrgCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "pkg-org", "Pkg Org", "")
	pkgID, versionID := seedPackageVersion(t, "org", org.Login, "npm", "org-pkg", "2.0.0")

	resp := ghGet(t, "/api/v3/orgs/"+org.Login+"/packages", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org packages: %d %s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode org packages: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, p := range list {
		if p["name"] == "org-pkg" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("seeded org-pkg not in list: %v", list)
	}

	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/packages/npm/org-pkg", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get org package: %d %s", resp.StatusCode, b)
	}
	pkg := decodeJSON(t, resp)
	if pkg["owner"].(map[string]any)["login"] != org.Login {
		t.Fatalf("expected org owner, got %v", pkg["owner"])
	}

	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/packages/npm/org-pkg/versions/"+strconv.Itoa(versionID), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete org version: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/packages/npm/org-pkg/versions/"+strconv.Itoa(versionID)+"/restore", defaultToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("restore org version: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/packages/npm/org-pkg", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete org package: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	_ = pkgID
}

func TestPackages_RepoCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "pkg-repo", "pkg repo", false)
	pkgID, versionID := seedPackageVersion(t, "repository", repo.FullName, "docker", "repo-pkg", "3.0.0")

	resp := ghGet(t, "/api/v3/repos/"+repo.FullName+"/packages", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list repo packages: %d %s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode repo packages: %v", err)
	}
	resp.Body.Close()
	found := false
	for _, p := range list {
		if p["name"] == "repo-pkg" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("seeded repo-pkg not in list: %v", list)
	}

	resp = ghGet(t, "/api/v3/repos/"+repo.FullName+"/packages/docker/repo-pkg", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get repo package: %d %s", resp.StatusCode, b)
	}
	pkg := decodeJSON(t, resp)
	if pkg["repository"] == nil {
		t.Fatal("expected repository block")
	}

	resp = ghDelete(t, "/api/v3/repos/"+repo.FullName+"/packages/docker/repo-pkg/versions/"+strconv.Itoa(versionID), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete repo version: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = ghDelete(t, "/api/v3/repos/"+repo.FullName+"/packages/docker/repo-pkg", defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete repo package: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	_ = pkgID
}

func TestPackages_404s(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	_ = admin

	// Missing user
	resp := ghGet(t, "/api/v3/users/nonexistent-user-xyz/packages", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing user, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing package
	resp = ghGet(t, "/api/v3/users/admin/packages/container/does-not-exist", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing package, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing version
	resp = ghGet(t, "/api/v3/users/admin/packages/container/does-not-exist/versions/999999", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing version, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing org
	resp = ghGet(t, "/api/v3/orgs/nonexistent-org-xyz/packages", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing org, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing repo
	resp = ghGet(t, "/api/v3/repos/nonexistent/repo/packages", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing repo, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Invalid package type
	resp = ghGet(t, "/api/v3/users/admin/packages/invalid/foo", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid package type, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPackages_RequiresAuth(t *testing.T) {
	resp := ghGet(t, "/api/v3/users/admin/packages", "")
	if resp.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 401 without token, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestPackages_InternalUploadValidation(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]

	// Missing version
	resp, _ := authedPost("/internal/packages/user/"+admin.Login+"/container/bad-pkg/versions", "application/json", bytes.NewReader([]byte(`{}`)))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422 missing version, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Invalid package type
	resp, _ = authedPost("/internal/packages/user/"+admin.Login+"/invalid/bad-pkg/versions", "application/json", bytes.NewReader([]byte(`{"version":"1.0.0"}`)))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422 invalid package type, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Missing owner
	resp, _ = authedPost("/internal/packages/user/no-such-user/container/bad-pkg/versions", "application/json", bytes.NewReader([]byte(`{"version":"1.0.0"}`)))
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 404 missing owner, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}
