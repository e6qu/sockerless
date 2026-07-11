package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func seedPackageVersion(t *testing.T, ownerType, owner, pkgType, pkgName, version string) (int, int) {
	t.Helper()
	if pkgType == "container" {
		t.Fatalf("container packages must be published through the GitHub Container Registry-compatible /v2/ data plane, not /internal/packages")
	}
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

func publishContainerPackageVersion(t *testing.T, owner, pkgName, version string) (int, int) {
	t.Helper()
	name := owner + "/" + pkgName
	layerBytes := []byte("package layer " + owner + "/" + pkgName + ":" + version)
	layerDigest := uploadRegistryBlob(t, name, layerBytes)
	manifest := mustRegistryJSON(map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"layers": []map[string]interface{}{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar",
				"digest":    layerDigest,
				"size":      len(layerBytes),
			},
		},
	})
	resp := registryRequest(t, http.MethodPut, "/v2/"+name+"/manifests/"+version, bytes.NewReader(manifest))
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("publish container package %s:%s: %d %s", name, version, resp.StatusCode, body)
	}
	created := decodeJSON(t, resp)
	pkgID := 0
	if pkg := testServer.store.GetPackage(owner, "container", pkgName); pkg != nil {
		pkgID = pkg.ID
	}
	versionID := int(created["id"].(float64))
	return pkgID, versionID
}

func TestPackageFixturesDoNotSeedContainerPackagesThroughInternalRoute(t *testing.T) {
	for _, path := range []string{"gh_packages_test.go", "gh_packages_live_test.go", "gh_packages_user_surface_test.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, needle := range []string{
			strings.Join([]string{`seedPackageVersion`, `(t, "user", admin.Login, "container"`}, ""),
			strings.Join([]string{`seedPackageVersion`, `(t, "org", org.Login, "container"`}, ""),
			strings.Join([]string{`seed`, `("user", admin.Login, "container"`}, ""),
		} {
			if strings.Contains(string(source), needle) {
				t.Fatalf("%s still seeds a container package through internal package setup; publish it through /v2/", path)
			}
		}
	}
}

func TestPackages_UserCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	pkgID, versionID := publishContainerPackageVersion(t, admin.Login, "user-pkg", "1.0.0")

	// List user packages (package_type is a required query parameter)
	resp := ghGet(t, "/api/v3/users/"+admin.Login+"/packages?package_type=container", defaultToken)
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
	if len(files) < 2 {
		t.Fatalf("expected registry manifest and layer files, got %d", len(files))
	}
	fileID := int(files[0]["id"].(float64))
	hasManifest := false
	hasLayer := false
	for _, file := range files {
		name, _ := file["name"].(string)
		switch {
		case name == "manifest.json":
			hasManifest = true
		case strings.HasPrefix(name, "blobs/sha256/"):
			hasLayer = true
		}
	}
	if !hasManifest || !hasLayer {
		t.Fatalf("expected registry manifest and layer files, got %v", files)
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
	if len(data) == 0 {
		t.Fatal("expected package file bytes, got empty body")
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

	resp := ghGet(t, "/api/v3/orgs/"+org.Login+"/packages?package_type=npm", defaultToken)
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
	resp, _ := authedPost("/internal/packages/user/"+admin.Login+"/npm/bad-pkg/versions", "application/json", bytes.NewReader([]byte(`{}`)))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422 missing version, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Container packages publish through the GitHub Container Registry-compatible
	// registry data plane, not the internal metadata upload route.
	resp, _ = authedPost("/internal/packages/user/"+admin.Login+"/container/bad-pkg/versions", "application/json", bytes.NewReader([]byte(`{"version":"1.0.0"}`)))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422 container package internal upload, got %d %s", resp.StatusCode, b)
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
	resp, _ = authedPost("/internal/packages/user/no-such-user/npm/bad-pkg/versions", "application/json", bytes.NewReader([]byte(`{"version":"1.0.0"}`)))
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 404 missing owner, got %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestPackages_OrgPackageRestore(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "pkg-restore-org", "Pkg Restore Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	_, versionID := seedPackageVersion(t, "org", org.Login, "npm", "restorable-pkg", "1.0.0")

	// Delete the whole package: gone from every read surface.
	resp := ghDelete(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete package: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted package: %d, want 404", resp.StatusCode)
	}

	// Restore brings it back with its versions intact.
	resp = ghPost(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("restore package: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get restored package: %d", resp.StatusCode)
	}
	restored := decodeJSON(t, resp)
	if restored["version_count"] != float64(1) {
		t.Fatalf("restored package version_count = %v, want 1", restored["version_count"])
	}
	resp = ghGet(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg/versions/"+strconv.Itoa(versionID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get restored package version: %d", resp.StatusCode)
	}

	// A live package cannot be restored again; unknown names are not found.
	resp = ghPost(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("restore live package: %d, want 404", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/pkg-restore-org/packages/npm/never-existed/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("restore unknown package: %d, want 404", resp.StatusCode)
	}

	// Reusing the namespace forfeits restore: deleting and republishing the
	// same name purges the old package for good.
	resp = ghDelete(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("re-delete package: %d, want 204", resp.StatusCode)
	}
	seedPackageVersion(t, "org", org.Login, "npm", "restorable-pkg", "2.0.0")
	resp = ghGet(t, "/api/v3/orgs/pkg-restore-org/packages/npm/restorable-pkg", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get republished package: %d", resp.StatusCode)
	}
	republished := decodeJSON(t, resp)
	if republished["version_count"] != float64(1) {
		t.Fatalf("republished package version_count = %v, want 1 (old versions purged)", republished["version_count"])
	}
}

func TestPackages_OrgDockerConflicts(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "docker-conflicts-org", "Docker Conflicts Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}

	// No packages → honestly empty.
	resp := ghGet(t, "/api/v3/orgs/docker-conflicts-org/docker/conflicts", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("docker conflicts (empty): %d", resp.StatusCode)
	}
	if conflicts := decodeJSONArray(t, resp); len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}

	// A docker package alone does not conflict.
	seedPackageVersion(t, "org", org.Login, "docker", "shared-image", "1.0.0")
	resp = ghGet(t, "/api/v3/orgs/docker-conflicts-org/docker/conflicts", defaultToken)
	if conflicts := decodeJSONArray(t, resp); len(conflicts) != 0 {
		t.Fatalf("expected no conflicts with docker package alone, got %v", conflicts)
	}

	// A container package with the same name makes the docker package a
	// migration conflict.
	publishContainerPackageVersion(t, org.Login, "shared-image", "1.0.0")
	resp = ghGet(t, "/api/v3/orgs/docker-conflicts-org/docker/conflicts", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("docker conflicts: %d", resp.StatusCode)
	}
	conflicts := decodeJSONArray(t, resp)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %v", conflicts)
	}
	if conflicts[0]["name"] != "shared-image" || conflicts[0]["package_type"] != "docker" {
		t.Fatalf("conflict row wrong: %v", conflicts[0])
	}

	// Requires authentication.
	resp = ghGet(t, "/api/v3/orgs/docker-conflicts-org/docker/conflicts", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated docker conflicts: %d, want 401", resp.StatusCode)
	}
}
