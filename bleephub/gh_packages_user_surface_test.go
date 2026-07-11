package bleephub

import (
	"net/http"
	"strconv"
	"testing"
)

func TestPackagesAuthUser_CRUDAndVersionRestore(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	_, versionID := seedPackageVersion(t, "user", admin.Login, "npm", "authuser-pkg", "1.0.0")

	// package_type is a required query parameter on the list endpoint.
	resp := ghGet(t, "/api/v3/user/packages", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("list without package_type: %d, want 400", resp.StatusCode)
	}

	list := decodeJSONArray(t, ghGet(t, "/api/v3/user/packages?package_type=npm", defaultToken))
	found := false
	for _, p := range list {
		if p["name"] == "authuser-pkg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("authuser-pkg not in npm list: %v", list)
	}
	// A different package_type filters it out.
	list = decodeJSONArray(t, ghGet(t, "/api/v3/user/packages?package_type=maven", defaultToken))
	for _, p := range list {
		if p["name"] == "authuser-pkg" {
			t.Fatalf("authuser-pkg leaked into maven list: %v", list)
		}
	}

	pkg := decodeJSON(t, ghGet(t, "/api/v3/user/packages/npm/authuser-pkg", defaultToken))
	if pkg["name"] != "authuser-pkg" || pkg["package_type"] != "npm" {
		t.Fatalf("get package = %v", pkg)
	}

	versions := decodeJSONArray(t, ghGet(t, "/api/v3/user/packages/npm/authuser-pkg/versions", defaultToken))
	if len(versions) != 1 || versions[0]["name"] != "1.0.0" {
		t.Fatalf("versions = %v", versions)
	}
	version := decodeJSON(t, ghGet(t, "/api/v3/user/packages/npm/authuser-pkg/versions/"+strconv.Itoa(versionID), defaultToken))
	if int(version["id"].(float64)) != versionID {
		t.Fatalf("get version = %v", version)
	}

	// Delete + restore the version.
	resp = ghDelete(t, "/api/v3/user/packages/npm/authuser-pkg/versions/"+strconv.Itoa(versionID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete version: %d", resp.StatusCode)
	}
	if v := decodeJSONArray(t, ghGet(t, "/api/v3/user/packages/npm/authuser-pkg/versions", defaultToken)); len(v) != 0 {
		t.Fatalf("versions after delete = %v", v)
	}
	resp = ghPost(t, "/api/v3/user/packages/npm/authuser-pkg/versions/"+strconv.Itoa(versionID)+"/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("restore version: %d", resp.StatusCode)
	}
	if v := decodeJSONArray(t, ghGet(t, "/api/v3/user/packages/npm/authuser-pkg/versions", defaultToken)); len(v) != 1 {
		t.Fatalf("versions after restore = %v", v)
	}
}

func TestPackagesAuthUser_DeleteAndRestorePackage(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	seedPackageVersion(t, "user", admin.Login, "npm", "restorable-pkg", "2.0.0")

	resp := ghDelete(t, "/api/v3/user/packages/npm/restorable-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete package: %d", resp.StatusCode)
	}

	// Deleted packages disappear from gets and lists...
	resp = ghGet(t, "/api/v3/user/packages/npm/restorable-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted package: %d, want 404", resp.StatusCode)
	}

	// ...but restore brings the package back with its versions intact.
	resp = ghPost(t, "/api/v3/user/packages/npm/restorable-pkg/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("restore package: %d", resp.StatusCode)
	}
	pkg := decodeJSON(t, ghGet(t, "/api/v3/user/packages/npm/restorable-pkg", defaultToken))
	if pkg["name"] != "restorable-pkg" {
		t.Fatalf("restored package = %v", pkg)
	}
	versions := decodeJSONArray(t, ghGet(t, "/api/v3/user/packages/npm/restorable-pkg/versions", defaultToken))
	if len(versions) != 1 || versions[0]["name"] != "2.0.0" {
		t.Fatalf("restored versions = %v", versions)
	}

	// Restoring a package that was never deleted is a 404.
	resp = ghPost(t, "/api/v3/user/packages/npm/restorable-pkg/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("restore live package: %d, want 404", resp.StatusCode)
	}
}

func TestPackagesRestore_UserAndOrgScoped(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	seedPackageVersion(t, "user", admin.Login, "maven", "userscope-pkg", "1.0.0")

	resp := ghDelete(t, "/api/v3/users/admin/packages/maven/userscope-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete user-scoped package: %d", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/users/admin/packages/maven/userscope-pkg/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("restore user-scoped package: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/users/admin/packages/maven/userscope-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get restored user-scoped package: %d", resp.StatusCode)
	}

	org := testServer.store.CreateOrg(admin, "pkg-restore-user-org", "Pkg Restore User Org", "")
	seedPackageVersion(t, "org", org.Login, "npm", "orgscope-pkg", "1.0.0")
	resp = ghDelete(t, "/api/v3/orgs/"+org.Login+"/packages/npm/orgscope-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete org-scoped package: %d", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/packages/npm/orgscope-pkg/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("restore org-scoped package: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/packages/npm/orgscope-pkg", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get restored org-scoped package: %d", resp.StatusCode)
	}

	// Restoring a package that never existed is a 404.
	resp = ghPost(t, "/api/v3/orgs/"+org.Login+"/packages/npm/never-existed/restore", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("restore unknown package: %d, want 404", resp.StatusCode)
	}
}

func TestPackagesDockerConflicts_ComputedFromStore(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]

	// No conflicts: empty array.
	list := decodeJSONArray(t, ghGet(t, "/api/v3/user/docker/conflicts", defaultToken))
	if len(list) != 0 {
		t.Fatalf("conflicts before seeding = %v", list)
	}

	// A Docker-registry package whose name collides with a Container
	// registry package cannot migrate — that is the conflict set.
	seedPackageVersion(t, "user", admin.Login, "docker", "conflicted-image", "1.0.0")
	publishContainerPackageVersion(t, admin.Login, "conflicted-image", "1.0.0")
	seedPackageVersion(t, "user", admin.Login, "docker", "unconflicted-image", "1.0.0")

	for _, path := range []string{"/api/v3/user/docker/conflicts", "/api/v3/users/admin/docker/conflicts"} {
		list = decodeJSONArray(t, ghGet(t, path, defaultToken))
		if len(list) != 1 || list[0]["name"] != "conflicted-image" || list[0]["package_type"] != "docker" {
			t.Fatalf("%s: conflicts = %v", path, list)
		}
	}
}
