package bleephub

import (
	"strconv"
	"testing"
)

func TestImmutableReleases_OrgSettingsAndRepoEnforcement(t *testing.T) {
	org := createTestOrg(t)
	repoName, _ := createOrgRepoForGovernance(t, org)
	repoPath := org + "/" + repoName
	orgSettings := "/api/v3/orgs/" + org + "/settings/immutable-releases"
	repoEndpoint := "/api/v3/repos/" + repoPath + "/immutable-releases"

	// The unconfigured default is "none".
	resp := ghGet(t, orgSettings, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("get org settings: %d", resp.StatusCode)
	}
	settings := decodeJSON(t, resp)
	if settings["enforced_repositories"] != "none" {
		t.Fatalf("default settings = %v", settings)
	}
	if _, ok := settings["selected_repositories_url"]; ok {
		t.Fatalf("selected_repositories_url present without selected policy: %v", settings)
	}

	// With no enforcement and no repo toggle the repo check is a 404.
	resp = ghGet(t, repoEndpoint, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("repo check while disabled: %d", resp.StatusCode)
	}

	// Enforce for all repositories.
	resp = ghPut(t, orgSettings, defaultToken, map[string]interface{}{
		"enforced_repositories": "all",
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("set org settings: %d", resp.StatusCode)
	}

	// The repo check reflects the owner enforcement.
	resp = ghGet(t, repoEndpoint, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("repo check under enforcement: %d", resp.StatusCode)
	}
	check := decodeJSON(t, resp)
	if check["enabled"] != true || check["enforced_by_owner"] != true {
		t.Fatalf("check = %v", check)
	}

	// The repo toggle cannot be disabled while the owner enforces it.
	resp = ghDelete(t, repoEndpoint, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("disable under enforcement: %d", resp.StatusCode)
	}

	// Drop the enforcement; the repo goes back to disabled.
	resp = ghPut(t, orgSettings, defaultToken, map[string]interface{}{
		"enforced_repositories": "none",
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("clear org settings: %d", resp.StatusCode)
	}
	resp = ghGet(t, repoEndpoint, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("repo check after clearing: %d", resp.StatusCode)
	}

	// Repo-level enable / check / disable round-trip.
	resp = ghPut(t, repoEndpoint, defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("enable repo immutable releases: %d", resp.StatusCode)
	}
	resp = ghGet(t, repoEndpoint, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("repo check after enable: %d", resp.StatusCode)
	}
	check = decodeJSON(t, resp)
	if check["enabled"] != true || check["enforced_by_owner"] != false {
		t.Fatalf("check after enable = %v", check)
	}
	resp = ghDelete(t, repoEndpoint, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("disable repo immutable releases: %d", resp.StatusCode)
	}
	resp = ghGet(t, repoEndpoint, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("repo check after disable: %d", resp.StatusCode)
	}
}

func TestImmutableReleases_SelectedRepositories(t *testing.T) {
	org := createTestOrg(t)
	repoAName, repoAID := createOrgRepoForGovernance(t, org)
	_, repoBID := createOrgRepoForGovernance(t, org)
	orgSettings := "/api/v3/orgs/" + org + "/settings/immutable-releases"
	reposEndpoint := orgSettings + "/repositories"

	// The selection endpoints require the selected policy.
	resp := ghPut(t, reposEndpoint, defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repoAID},
	})
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("set selection without selected policy: %d", resp.StatusCode)
	}

	// Switch to selected with an initial selection.
	resp = ghPut(t, orgSettings, defaultToken, map[string]interface{}{
		"enforced_repositories":   "selected",
		"selected_repository_ids": []int{repoAID},
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("set selected policy: %d", resp.StatusCode)
	}

	// The settings carry the selection URL.
	resp = ghGet(t, orgSettings, defaultToken)
	settings := decodeJSON(t, resp)
	if settings["enforced_repositories"] != "selected" || settings["selected_repositories_url"] == nil {
		t.Fatalf("settings = %v", settings)
	}

	// List the selection.
	resp = ghGet(t, reposEndpoint, defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list selection: %d", resp.StatusCode)
	}
	listed := decodeJSON(t, resp)
	if listed["total_count"].(float64) != 1 {
		t.Fatalf("selection = %v", listed)
	}
	repos := listed["repositories"].([]interface{})
	if int(repos[0].(map[string]interface{})["id"].(float64)) != repoAID {
		t.Fatalf("selection = %v", repos)
	}

	// The selected repository is enforced; its sibling is not.
	resp = ghGet(t, "/api/v3/repos/"+org+"/"+repoAName+"/immutable-releases", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("selected repo check: %d", resp.StatusCode)
	}
	if check := decodeJSON(t, resp); check["enforced_by_owner"] != true {
		t.Fatalf("selected repo check = %v", check)
	}

	// Add the sibling with the single-repository PUT.
	resp = ghPut(t, reposEndpoint+"/"+strconv.Itoa(repoBID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("add selected repo: %d", resp.StatusCode)
	}
	resp = ghGet(t, reposEndpoint, defaultToken)
	if listed = decodeJSON(t, resp); listed["total_count"].(float64) != 2 {
		t.Fatalf("selection after add = %v", listed)
	}

	// Remove one with the single-repository DELETE.
	resp = ghDelete(t, reposEndpoint+"/"+strconv.Itoa(repoAID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("remove selected repo: %d", resp.StatusCode)
	}
	resp = ghGet(t, reposEndpoint, defaultToken)
	listed = decodeJSON(t, resp)
	if listed["total_count"].(float64) != 1 {
		t.Fatalf("selection after remove = %v", listed)
	}
	repos = listed["repositories"].([]interface{})
	if int(repos[0].(map[string]interface{})["id"].(float64)) != repoBID {
		t.Fatalf("selection after remove = %v", repos)
	}

	// Replace the whole selection.
	resp = ghPut(t, reposEndpoint, defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repoAID, repoBID},
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("replace selection: %d", resp.StatusCode)
	}
	resp = ghGet(t, reposEndpoint, defaultToken)
	if listed = decodeJSON(t, resp); listed["total_count"].(float64) != 2 {
		t.Fatalf("replaced selection = %v", listed)
	}

	// A repository outside the org cannot be selected.
	resp = ghPut(t, reposEndpoint, defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{99999999},
	})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("foreign repo selection: %d", resp.StatusCode)
	}

	// An invalid policy value is rejected.
	resp = ghPut(t, orgSettings, defaultToken, map[string]interface{}{
		"enforced_repositories": "some",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("bad policy: %d", resp.StatusCode)
	}
}
