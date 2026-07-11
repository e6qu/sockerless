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

// Live-server shape test for the packages surface. Exercises the JSON
// endpoints through the shared TestMain server so the OpenAPI response-shape
// validator observes them.
func TestLivePackages_UserOrgRepo(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "live-pkg-org", "Live Pkg Org", "")
	repo := testServer.store.CreateRepo(admin, "live-pkg-repo", "live pkg repo", false)

	seed := func(ownerType, owner, pkgType, pkgName, version string) int {
		body, _ := json.Marshal(map[string]any{
			"version": version,
			"metadata": map[string]any{
				"package_type": pkgType,
				"container":    map[string]any{"tags": []string{"latest"}},
			},
			"files": []map[string]any{
				{
					"name":           "live.tgz",
					"content_type":   "application/gzip",
					"content_base64": base64.StdEncoding.EncodeToString([]byte("live bytes")),
				},
			},
		})
		resp, err := authedPost("/internal/packages/"+ownerType+"/"+owner+"/"+pkgType+"/"+pkgName+"/versions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("seed %s/%s: %v", ownerType, pkgName, err)
		}
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("seed %s/%s: %d %s", ownerType, pkgName, resp.StatusCode, b)
		}
		var v map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			t.Fatalf("decode seeded version: %v", err)
		}
		resp.Body.Close()
		return int(v["id"].(float64))
	}

	_, userVersionID := publishContainerPackageVersion(t, admin.Login, "live-user-pkg", "1.0.0")
	orgVersionID := seed("org", org.Login, "npm", "live-org-pkg", "1.0.0")
	repoVersionID := seed("repository", repo.FullName, "docker", "live-repo-pkg", "1.0.0")

	cases := []struct {
		path string
	}{
		{"/api/v3/users/" + admin.Login + "/packages?package_type=container"},
		{"/api/v3/users/" + admin.Login + "/packages/container/live-user-pkg"},
		{"/api/v3/users/" + admin.Login + "/packages/container/live-user-pkg/versions"},
		{"/api/v3/users/" + admin.Login + "/packages/container/live-user-pkg/versions/" + strconv.Itoa(userVersionID)},
		{"/api/v3/users/" + admin.Login + "/packages/container/live-user-pkg/versions/" + strconv.Itoa(userVersionID) + "/files"},
		{"/api/v3/orgs/" + org.Login + "/packages?package_type=npm"},
		{"/api/v3/orgs/" + org.Login + "/packages/npm/live-org-pkg"},
		{"/api/v3/orgs/" + org.Login + "/packages/npm/live-org-pkg/versions"},
		{"/api/v3/orgs/" + org.Login + "/packages/npm/live-org-pkg/versions/" + strconv.Itoa(orgVersionID)},
		{"/api/v3/orgs/" + org.Login + "/packages/npm/live-org-pkg/versions/" + strconv.Itoa(orgVersionID) + "/files"},
		{"/api/v3/repos/" + repo.FullName + "/packages"},
		{"/api/v3/repos/" + repo.FullName + "/packages/docker/live-repo-pkg"},
		{"/api/v3/repos/" + repo.FullName + "/packages/docker/live-repo-pkg/versions"},
		{"/api/v3/repos/" + repo.FullName + "/packages/docker/live-repo-pkg/versions/" + strconv.Itoa(repoVersionID)},
		{"/api/v3/repos/" + repo.FullName + "/packages/docker/live-repo-pkg/versions/" + strconv.Itoa(repoVersionID) + "/files"},
	}

	for _, c := range cases {
		resp := authedGet(t, c.path)
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("%s: %d %s", c.path, resp.StatusCode, b)
		}
		// Drain body so the observer can validate the shape.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
