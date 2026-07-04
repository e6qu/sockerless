package bleephub

import (
	"fmt"
	"testing"
	"time"
)

func TestOrgActionsPermissions_ArtifactAndLogRetention(t *testing.T) {
	org := createTestOrg(t)
	path := "/api/v3/orgs/" + org + "/actions/permissions/artifact-and-log-retention"

	data := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["days"].(float64) != 90 || data["maximum_allowed_days"].(float64) != 400 {
		t.Fatalf("defaults = %v, want days 90 / maximum_allowed_days 400", data)
	}

	resp := ghPut(t, path, defaultToken, map[string]int{"days": 30})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("put = %d, want 204", resp.StatusCode)
	}
	data = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["days"].(float64) != 30 {
		t.Fatalf("days after put = %v, want 30", data["days"])
	}

	resp = ghPut(t, path, defaultToken, map[string]int{"days": 500})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("out-of-range put = %d, want 422", resp.StatusCode)
	}
}

func TestOrgActionsPermissions_ForkPRContributorApproval(t *testing.T) {
	org := createTestOrg(t)
	path := "/api/v3/orgs/" + org + "/actions/permissions/fork-pr-contributor-approval"

	data := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["approval_policy"] != "first_time_contributors" {
		t.Fatalf("default approval_policy = %v", data["approval_policy"])
	}

	resp := ghPut(t, path, defaultToken, map[string]string{"approval_policy": "all_external_contributors"})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("put = %d, want 204", resp.StatusCode)
	}
	data = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["approval_policy"] != "all_external_contributors" {
		t.Fatalf("approval_policy after put = %v", data["approval_policy"])
	}

	resp = ghPut(t, path, defaultToken, map[string]string{"approval_policy": "nobody"})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid policy put = %d, want 422", resp.StatusCode)
	}
}

func TestOrgActionsPermissions_ForkPRWorkflowsPrivateRepos(t *testing.T) {
	org := createTestOrg(t)
	path := "/api/v3/orgs/" + org + "/actions/permissions/fork-pr-workflows-private-repos"

	data := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	for _, field := range []string{
		"run_workflows_from_fork_pull_requests", "send_write_tokens_to_workflows",
		"send_secrets_and_variables", "require_approval_for_fork_pr_workflows",
	} {
		if data[field] != false {
			t.Fatalf("default %s = %v, want false", field, data[field])
		}
	}

	resp := ghPut(t, path, defaultToken, map[string]bool{
		"run_workflows_from_fork_pull_requests":  true,
		"require_approval_for_fork_pr_workflows": true,
	})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("put = %d, want 204", resp.StatusCode)
	}
	data = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["run_workflows_from_fork_pull_requests"] != true ||
		data["require_approval_for_fork_pr_workflows"] != true ||
		data["send_secrets_and_variables"] != false {
		t.Fatalf("settings after put = %v", data)
	}

	// The one required member must be present.
	resp = ghPut(t, path, defaultToken, map[string]bool{"send_secrets_and_variables": true})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing required member put = %d, want 422", resp.StatusCode)
	}
}

func TestOrgActionsPermissions_SelfHostedRunners(t *testing.T) {
	org := createTestOrg(t)
	repoKey := createTestRepo(t)
	repoData := decodeJSONWithStatus(t, ghGet(t, "/api/v3/repos/"+repoKey, defaultToken), 200)
	repoID := int(repoData["id"].(float64))

	path := "/api/v3/orgs/" + org + "/actions/permissions/self-hosted-runners"
	data := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["enabled_repositories"] != "all" {
		t.Fatalf("default enabled_repositories = %v", data["enabled_repositories"])
	}

	resp := ghPut(t, path, defaultToken, map[string]string{"enabled_repositories": "selected"})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("put = %d, want 204", resp.StatusCode)
	}
	data = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if data["enabled_repositories"] != "selected" || data["selected_repositories_url"] == nil {
		t.Fatalf("settings after put = %v", data)
	}

	resp = ghPut(t, path, defaultToken, map[string]string{"enabled_repositories": "sometimes"})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid enum put = %d, want 422", resp.StatusCode)
	}

	// Selected repository management.
	resp = ghPut(t, path+"/repositories", defaultToken, map[string][]int{"selected_repository_ids": {}})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("set repos = %d, want 204", resp.StatusCode)
	}
	resp = ghPut(t, fmt.Sprintf("%s/repositories/%d", path, repoID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("add repo = %d, want 204", resp.StatusCode)
	}
	resp = ghPut(t, path+"/repositories/999999", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("add unknown repo = %d, want 404", resp.StatusCode)
	}
	list := decodeJSONWithStatus(t, ghGet(t, path+"/repositories", defaultToken), 200)
	repos, _ := list["repositories"].([]interface{})
	if int(list["total_count"].(float64)) != 1 || len(repos) != 1 {
		t.Fatalf("selected repos = %v, want the added repo", list)
	}
	resp = ghDelete(t, fmt.Sprintf("%s/repositories/%d", path, repoID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("remove repo = %d, want 204", resp.StatusCode)
	}
	list = decodeJSONWithStatus(t, ghGet(t, path+"/repositories", defaultToken), 200)
	if int(list["total_count"].(float64)) != 0 {
		t.Fatalf("selected repos after remove = %v, want none", list)
	}
}

func TestOrganizationsCachePolicyLimits(t *testing.T) {
	org := createTestOrg(t)
	retention := "/api/v3/organizations/" + org + "/actions/cache/retention-limit"
	storage := "/api/v3/organizations/" + org + "/actions/cache/storage-limit"

	data := decodeJSONWithStatus(t, ghGet(t, retention, defaultToken), 200)
	if data["max_cache_retention_days"].(float64) != 90 {
		t.Fatalf("default max_cache_retention_days = %v, want 90", data)
	}
	resp := ghPut(t, retention, defaultToken, map[string]int{"max_cache_retention_days": 14})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("put retention = %d, want 204", resp.StatusCode)
	}
	data = decodeJSONWithStatus(t, ghGet(t, retention, defaultToken), 200)
	if data["max_cache_retention_days"].(float64) != 14 {
		t.Fatalf("max_cache_retention_days after put = %v, want 14", data)
	}
	resp = ghPut(t, retention, defaultToken, map[string]int{"max_cache_retention_days": 0})
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("invalid retention put = %d, want 400", resp.StatusCode)
	}

	data = decodeJSONWithStatus(t, ghGet(t, storage, defaultToken), 200)
	if data["max_cache_size_gb"].(float64) != 10 {
		t.Fatalf("default max_cache_size_gb = %v, want 10", data)
	}
	resp = ghPut(t, storage, defaultToken, map[string]int{"max_cache_size_gb": 25})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("put storage = %d, want 204", resp.StatusCode)
	}
	data = decodeJSONWithStatus(t, ghGet(t, storage, defaultToken), 200)
	if data["max_cache_size_gb"].(float64) != 25 {
		t.Fatalf("max_cache_size_gb after put = %v, want 25", data)
	}
}

func TestOrgCacheUsage_ComputedFromRealCacheStore(t *testing.T) {
	org := createTestOrg(t)
	usagePath := "/api/v3/orgs/" + org + "/actions/cache/usage"
	byRepoPath := "/api/v3/orgs/" + org + "/actions/cache/usage-by-repository"

	// Zero when the org has no caches.
	data := decodeJSONWithStatus(t, ghGet(t, usagePath, defaultToken), 200)
	if data["total_active_caches_count"].(float64) != 0 ||
		data["total_active_caches_size_in_bytes"].(float64) != 0 {
		t.Fatalf("empty-org usage = %v, want zeros", data)
	}

	// Store two finalized cache entries for one org repo and one for
	// another, plus one for a foreign owner that must not count.
	as := testServer.artifactStore
	seedCache := func(repo, key string, size int64) {
		as.mu.Lock()
		id := as.nextCacheID
		as.nextCacheID++
		as.caches[id] = &CacheEntry{
			ID: id, Repo: repo, Key: key, Version: "v1",
			Size: size, Finalized: true, CreatedAt: time.Now(),
		}
		as.cacheIndex[cacheLookupKey(repo, key, "v1")] = id
		as.mu.Unlock()
	}
	seedCache(org+"/repo-a", "npm-cache", 1000)
	seedCache(org+"/repo-a", "go-cache", 500)
	seedCache(org+"/repo-b", "pip-cache", 250)
	seedCache("someone-else/repo", "other", 9999)

	data = decodeJSONWithStatus(t, ghGet(t, usagePath, defaultToken), 200)
	if data["total_active_caches_count"].(float64) != 3 ||
		data["total_active_caches_size_in_bytes"].(float64) != 1750 {
		t.Fatalf("org usage = %v, want 3 caches / 1750 bytes", data)
	}

	byRepo := decodeJSONWithStatus(t, ghGet(t, byRepoPath, defaultToken), 200)
	usages, _ := byRepo["repository_cache_usages"].([]interface{})
	if int(byRepo["total_count"].(float64)) != 2 || len(usages) != 2 {
		t.Fatalf("usage-by-repository = %v, want 2 repos", byRepo)
	}
	first, _ := usages[0].(map[string]interface{})
	if first["full_name"] != org+"/repo-a" ||
		first["active_caches_count"].(float64) != 2 ||
		first["active_caches_size_in_bytes"].(float64) != 1500 {
		t.Fatalf("repo-a usage = %v", first)
	}
}

func TestRunnerLabels_AddAndRemoveSingle(t *testing.T) {
	org := createTestOrg(t)

	// Register a runner with a system label through the real agent
	// registration path.
	testServer.store.mu.Lock()
	agent := &Agent{
		ID:     testServer.store.NextAgent,
		Name:   "labels-test-runner",
		Status: "online",
		Labels: []Label{{ID: 1, Name: "self-hosted", Type: "system"}},
	}
	testServer.store.NextAgent++
	testServer.store.Agents[agent.ID] = agent
	testServer.store.mu.Unlock()

	orgPath := fmt.Sprintf("/api/v3/orgs/%s/actions/runners/%d/labels", org, agent.ID)
	repoPath := fmt.Sprintf("/api/v3/repos/octo/repo/actions/runners/%d/labels", agent.ID)

	// POST adds labels (org scope).
	data := decodeJSONWithStatus(t, ghPost(t, orgPath, defaultToken, map[string][]string{
		"labels": {"gpu", "x64"},
	}), 200)
	if int(data["total_count"].(float64)) != 3 {
		t.Fatalf("labels after add = %v, want 3 (system + 2 custom)", data)
	}

	// Empty labels array rejects.
	resp := ghPost(t, orgPath, defaultToken, map[string][]string{"labels": {}})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("empty labels add = %d, want 422", resp.StatusCode)
	}

	// DELETE one custom label (repo scope) returns the remaining set.
	data = decodeJSONWithStatus(t, ghDelete(t, repoPath+"/gpu", defaultToken), 200)
	labels, _ := data["labels"].([]interface{})
	if int(data["total_count"].(float64)) != 2 {
		t.Fatalf("labels after remove = %v, want 2", data)
	}
	for _, l := range labels {
		if l.(map[string]interface{})["name"] == "gpu" {
			t.Fatalf("gpu label still present: %v", labels)
		}
	}

	// Removing an absent label 404s; removing a read-only label 422s.
	resp = ghDelete(t, repoPath+"/gpu", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("remove absent label = %d, want 404", resp.StatusCode)
	}
	resp = ghDelete(t, orgPath+"/self-hosted", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("remove read-only label = %d, want 422", resp.StatusCode)
	}

	// Unknown runner 404s.
	resp = ghPost(t, fmt.Sprintf("/api/v3/orgs/%s/actions/runners/999999/labels", org),
		defaultToken, map[string][]string{"labels": {"x"}})
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("add labels unknown runner = %d, want 404", resp.StatusCode)
	}

	// Cleanup so runner-list tests elsewhere see the shared pool unchanged.
	testServer.store.mu.Lock()
	delete(testServer.store.Agents, agent.ID)
	testServer.store.mu.Unlock()
}
