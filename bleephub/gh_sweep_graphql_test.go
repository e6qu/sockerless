package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Tests in this file replay the EXACT GraphQL shapes gh CLI (v2.96) sends —
// copied from the gh source (api/query_builder.go, api/queries_repo.go,
// pkg/cmd/pr/shared/{lister,finder}.go, pkg/cmd/pr/status/http.go,
// pkg/cmd/issue/list/http.go, pkg/cmd/release/list/http.go,
// pkg/cmd/release/shared/fetch.go, api/queries_pr_review.go) — so schema
// drift against the real client fails here before it fails in the harness.

// gqlDo posts a GraphQL request as the admin user and returns the full
// response envelope (data + errors).
func gqlDo(t *testing.T, query string, variables map[string]interface{}) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	resp := ghPost(t, "/api/graphql", defaultToken, body)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("graphql status = %d", resp.StatusCode)
	}
	return decodeJSON(t, resp)
}

// gqlData asserts an error-free response and returns data.
func gqlData(t *testing.T, query string, variables map[string]interface{}) map[string]interface{} {
	t.Helper()
	env := gqlDo(t, query, variables)
	if errs, ok := env["errors"]; ok && errs != nil {
		t.Fatalf("graphql errors: %v", errs)
	}
	d, _ := env["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("no data in response: %v", env)
	}
	return d
}

func TestRepoGraphQLURLUsesConfiguredExternalURL(t *testing.T) {
	t.Setenv("BLEEPHUB_EXTERNAL_URL", "https://bleephub.example.test/")
	repo := &Repo{FullName: "octo/example", Name: "example"}
	if got := repoToGraphQL(testServer.store, repo)["url"]; got != "https://bleephub.example.test/octo/example" {
		t.Fatalf("repository GraphQL url = %v", got)
	}
}

// sweepRepo creates a fresh repo via REST and returns (owner, name).
func sweepRepo(t *testing.T, name string) (string, string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": name, "auto_init": true})
	data := decodeJSON(t, resp)
	owner, _ := data["owner"].(map[string]interface{})
	login, _ := owner["login"].(string)
	repoName, _ := data["name"].(string)
	if login == "" || repoName == "" {
		t.Fatalf("repo create failed: %v", data)
	}
	repo := testServer.store.GetRepo(login, repoName)
	if repo == nil {
		t.Fatalf("repo %s/%s not found after create", login, repoName)
	}
	seedPullRequestBranches(t, testServer, repo, "feature")
	return login, repoName
}

// sweepPR creates a PR via REST and returns its number and database id.
func sweepPR(t *testing.T, owner, name, title string) (int, int) {
	t.Helper()
	resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/pulls", defaultToken, map[string]interface{}{
		"title": title,
		"head":  "feature",
		"base":  "main",
		"body":  "sweep pr body",
	})
	data := decodeJSON(t, resp)
	num, ok := data["number"].(float64)
	if !ok {
		t.Fatalf("pr create failed: %v", data)
	}
	id, _ := data["id"].(float64)
	return int(num), int(id)
}

// --- Finding 1: gh's GitHubRepo query (repo clone, pr create) ---

func TestRepoGraphQL_GitHubRepoQuery(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-githubrepo")

	// Verbatim from api/queries_repo.go GitHubRepo.
	query := `
	fragment repo on Repository {
		id
		name
		owner { login }
		hasIssuesEnabled
		description
		hasWikiEnabled
		viewerPermission
		defaultBranchRef {
			name
		}
	}

	query RepositoryInfo($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			...repo
			parent {
				...repo
			}
			mergeCommitAllowed
			rebaseMergeAllowed
			squashMergeAllowed
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	repo, _ := d["repository"].(map[string]interface{})
	if repo == nil {
		t.Fatalf("repository null: %v", d)
	}
	if v, ok := repo["hasWikiEnabled"].(bool); !ok || v {
		t.Errorf("hasWikiEnabled = %v, want false for default repo setting", repo["hasWikiEnabled"])
	}
	if repo["parent"] != nil {
		t.Errorf("parent = %v, want null for non-fork repo", repo["parent"])
	}
}

func TestRepoMetadataPushedAtFollowsGitHistory(t *testing.T) {
	name := "sweep-empty-pushed-at"
	createResp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": name})
	created := decodeJSON(t, createResp)
	ownerData, _ := created["owner"].(map[string]interface{})
	owner, _ := ownerData["login"].(string)
	if owner == "" || created["name"] != name {
		t.Fatalf("repo create failed: %v", created)
	}
	if created["pushed_at"] != nil {
		t.Fatalf("REST pushed_at for empty repository = %v, want null", created["pushed_at"])
	}

	query := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pushedAt,isEmpty}}`
	before := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	beforeRepo, _ := before["repository"].(map[string]interface{})
	if beforeRepo == nil {
		t.Fatalf("repository query before commit = %v", before)
	}
	if beforeRepo["pushedAt"] != nil {
		t.Fatalf("GraphQL pushedAt for empty repository = %v, want null", beforeRepo["pushedAt"])
	}
	if beforeRepo["isEmpty"] != true {
		t.Fatalf("GraphQL isEmpty before commit = %v, want true", beforeRepo["isEmpty"])
	}

	putResp := ghPut(t, "/api/v3/repos/"+owner+"/"+name+"/contents/README.md", defaultToken, map[string]interface{}{
		"message": "initial commit",
		"content": base64.StdEncoding.EncodeToString([]byte("# " + name + "\n")),
	})
	if putResp.StatusCode != http.StatusCreated {
		t.Fatalf("contents create status = %d", putResp.StatusCode)
	}

	repoResp := ghGet(t, "/api/v3/repos/"+owner+"/"+name, defaultToken)
	afterREST := decodeJSON(t, repoResp)
	if afterREST["pushed_at"] == nil {
		t.Fatalf("REST pushed_at after real commit = nil, want timestamp")
	}
	after := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	afterRepo, _ := after["repository"].(map[string]interface{})
	if afterRepo == nil {
		t.Fatalf("repository query after commit = %v", after)
	}
	if afterRepo["pushedAt"] == nil {
		t.Fatalf("GraphQL pushedAt after real commit = nil, want timestamp")
	}
	if afterRepo["isEmpty"] != false {
		t.Fatalf("GraphQL isEmpty after commit = %v, want false", afterRepo["isEmpty"])
	}
}

func TestRepoGraphQLArchivedAtFollowsArchiveState(t *testing.T) {
	owner, name := sweepRepo(t, "archive-state")
	query := `query($owner:String!,$name:String!){
		repository(owner:$owner,name:$name){isArchived,archivedAt}
	}`

	before := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	beforeRepo, _ := before["repository"].(map[string]interface{})
	if beforeRepo == nil {
		t.Fatalf("repository null before archive: %v", before)
	}
	if beforeRepo["isArchived"] != false || beforeRepo["archivedAt"] != nil {
		t.Fatalf("repository before archive = %v, want unarchived with null archivedAt", beforeRepo)
	}

	patchResp := ghPatch(t, "/api/v3/repos/"+owner+"/"+name, defaultToken, map[string]interface{}{"archived": true})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("archive patch status = %d", patchResp.StatusCode)
	}
	archived := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	archivedRepo, _ := archived["repository"].(map[string]interface{})
	if archivedRepo == nil {
		t.Fatalf("repository null after archive: %v", archived)
	}
	archivedAt, _ := archivedRepo["archivedAt"].(string)
	if archivedRepo["isArchived"] != true || archivedAt == "" {
		t.Fatalf("repository after archive = %v, want archived with archivedAt", archivedRepo)
	}
	if _, err := time.Parse(time.RFC3339, archivedAt); err != nil {
		t.Fatalf("archivedAt %q is not RFC3339: %v", archivedAt, err)
	}

	unarchiveResp := ghPatch(t, "/api/v3/repos/"+owner+"/"+name, defaultToken, map[string]interface{}{"archived": false})
	if unarchiveResp.StatusCode != http.StatusOK {
		t.Fatalf("unarchive patch status = %d", unarchiveResp.StatusCode)
	}
	after := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	afterRepo, _ := after["repository"].(map[string]interface{})
	if afterRepo == nil || afterRepo["isArchived"] != false || afterRepo["archivedAt"] != nil {
		t.Fatalf("repository after unarchive = %v, want unarchived with null archivedAt", afterRepo)
	}
}

// TestRepoGraphQL_RepositoryOwnerOrg verifies that repositoryOwner(login:)
// returns real organization data (not a synthetic partial User-shaped payload)
// when the login resolves to an organization.
func TestRepoGraphQL_RepositoryOwnerOrg(t *testing.T) {
	orgLogin := "sweep-owner-org"
	org := testServer.store.CreateOrg(testServer.store.UsersByLogin["admin"], orgLogin, "Sweep Owner Org", "")
	if org == nil {
		t.Fatal("failed to create org")
	}
	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Orgs, org.ID)
		delete(testServer.store.OrgsByLogin, orgLogin)
		delete(testServer.store.Memberships, membershipKey(orgLogin, testServer.store.UsersByLogin["admin"].ID))
		testServer.store.mu.Unlock()
	}()

	query := `query Owner($login: String!) {
		repositoryOwner(login: $login) {
			id
			login
			name
			url
			createdAt
			updatedAt
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"login": orgLogin})
	owner, _ := d["repositoryOwner"].(map[string]interface{})
	if owner == nil {
		t.Fatalf("repositoryOwner null: %v", d)
	}
	if owner["login"] != orgLogin {
		t.Errorf("login = %v, want %q", owner["login"], orgLogin)
	}
	if owner["name"] != "Sweep Owner Org" {
		t.Errorf("name = %v, want %q", owner["name"], "Sweep Owner Org")
	}
	if owner["url"] != "/"+orgLogin {
		t.Errorf("url = %v, want /%s", owner["url"], orgLogin)
	}
	if owner["createdAt"] == nil || owner["updatedAt"] == nil {
		t.Errorf("createdAt/updatedAt missing: %v", owner)
	}
}

// --- Finding 1: the static --json field set gh repo view exposes ---

func TestRepoGraphQL_ViewJSONStaticFields(t *testing.T) {
	repoResp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":             "sweep-repoview",
		"license_template": "mit",
		"has_discussions":  false,
	})
	repoData := decodeJSON(t, repoResp)
	ownerData, _ := repoData["owner"].(map[string]interface{})
	owner, _ := ownerData["login"].(string)
	name, _ := repoData["name"].(string)
	if owner == "" || name == "" {
		t.Fatalf("repo create failed: %v", repoData)
	}
	if repoData["has_discussions"] != false {
		t.Fatalf("created repo has_discussions = %v, want false", repoData["has_discussions"])
	}
	disabledQuery := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){hasDiscussionsEnabled}}`
	disabledData := gqlData(t, disabledQuery, map[string]interface{}{"owner": owner, "name": name})
	disabledRepo, _ := disabledData["repository"].(map[string]interface{})
	if disabledRepo == nil || disabledRepo["hasDiscussionsEnabled"] != false {
		t.Fatalf("GraphQL hasDiscussionsEnabled before patch = %v, want false", disabledRepo)
	}
	patchResp := ghPatch(t, "/api/v3/repos/"+owner+"/"+name, defaultToken, map[string]interface{}{
		"homepage":               "https://example.test/sweep-repoview",
		"has_projects":           true,
		"has_wiki":               true,
		"has_discussions":        true,
		"allow_squash_merge":     false,
		"allow_merge_commit":     false,
		"allow_rebase_merge":     true,
		"delete_branch_on_merge": true,
		"is_template":            true,
	})
	patched := decodeJSON(t, patchResp)
	if patched["has_discussions"] != true {
		t.Fatalf("patched repo has_discussions = %v, want true", patched["has_discussions"])
	}

	// Seed a published release so latestRelease resolves to real store data.
	relResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/releases", defaultToken, map[string]interface{}{
		"tag_name": "v1.0.0",
		"name":     "first",
	})
	relData := decodeJSON(t, relResp)
	if relData["id"] == nil {
		t.Fatalf("release create failed: %v", relData)
	}
	subResp := ghPut(t, "/api/v3/repos/"+owner+"/"+name+"/subscription", defaultToken, map[string]interface{}{
		"subscribed": true,
	})
	if subResp.StatusCode != http.StatusOK {
		t.Fatalf("subscription create failed: %d", subResp.StatusCode)
	}

	// Field selections verbatim from api/query_builder.go RepositoryGraphQL.
	query := `query($owner:String!,$name:String!){
		repository(owner:$owner,name:$name){
			latestRelease{publishedAt,tagName,name,url}
			templateRepository{id,name,owner{id,login}}
			homepageUrl
			hasProjectsEnabled
			hasDiscussionsEnabled
			forkCount
			watchers{totalCount}
			licenseInfo{key,name,nickname,spdxId}
			primaryLanguage{name}
			languages(first:100){edges{size,node{name}}}
			repositoryTopics(first:100){nodes{topic{name}}}
			mergeCommitAllowed
			rebaseMergeAllowed
			squashMergeAllowed
			deleteBranchOnMerge
			isTemplate
			isEmpty
			archivedAt
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name})
	repo, _ := d["repository"].(map[string]interface{})
	if repo == nil {
		t.Fatalf("repository null: %v", d)
	}

	latest, _ := repo["latestRelease"].(map[string]interface{})
	if latest == nil || latest["tagName"] != "v1.0.0" {
		t.Errorf("latestRelease = %v, want tagName v1.0.0", repo["latestRelease"])
	}
	for _, nullField := range []string{"templateRepository", "primaryLanguage", "archivedAt"} {
		if repo[nullField] != nil {
			t.Errorf("%s = %v, want null", nullField, repo[nullField])
		}
	}
	license, _ := repo["licenseInfo"].(map[string]interface{})
	if license == nil {
		t.Fatalf("licenseInfo = nil, want MIT license metadata")
	}
	if license["key"] != "mit" || license["name"] != "MIT License" || license["spdxId"] != "MIT" {
		t.Errorf("licenseInfo = %v, want MIT license metadata", license)
	}
	if repo["homepageUrl"] != "https://example.test/sweep-repoview" {
		t.Errorf("homepageUrl = %v, want patched homepage", repo["homepageUrl"])
	}
	for _, trueField := range []string{"hasProjectsEnabled", "hasDiscussionsEnabled", "deleteBranchOnMerge", "isTemplate"} {
		if v, ok := repo[trueField].(bool); !ok || !v {
			t.Errorf("%s = %v, want true", trueField, repo[trueField])
		}
	}
	for field, want := range map[string]bool{
		"mergeCommitAllowed": false,
		"rebaseMergeAllowed": true,
		"squashMergeAllowed": false,
	} {
		if got, ok := repo[field].(bool); !ok || got != want {
			t.Errorf("%s = %v, want %v", field, repo[field], want)
		}
	}
	if fc, _ := repo["forkCount"].(float64); fc != 0 {
		t.Errorf("forkCount = %v, want 0", repo["forkCount"])
	}
	watchers, _ := repo["watchers"].(map[string]interface{})
	if watchers == nil || watchers["totalCount"].(float64) != 1 {
		t.Errorf("watchers = %v, want totalCount 1", repo["watchers"])
	}
	langs, _ := repo["languages"].(map[string]interface{})
	if langs == nil {
		t.Fatalf("languages null")
	}
	if edges, ok := langs["edges"].([]interface{}); !ok || len(edges) != 0 {
		t.Errorf("languages.edges = %v, want []", langs["edges"])
	}
	// The public license_template path creates the initial license commit.
	if v, ok := repo["isEmpty"].(bool); !ok || v {
		t.Errorf("isEmpty = %v, want false for a licensed repo with an initial commit", repo["isEmpty"])
	}
}

func TestRepoGraphQL_ForkParentAndCount(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-fork-graphql")

	testServer.store.mu.Lock()
	forker := &User{ID: testServer.store.NextUser, Login: "graphql-forker", Type: "User", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	testServer.store.NextUser++
	testServer.store.Users[forker.ID] = forker
	testServer.store.UsersByLogin[forker.Login] = forker
	tok := &Token{Value: "graphql-forker-token", UserID: forker.ID, Scopes: "repo", CreatedAt: time.Now()}
	testServer.store.Tokens[tok.Value] = tok
	testServer.store.mu.Unlock()

	resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/forks", tok.Value, map[string]interface{}{"name": "sweep-fork-child"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("fork create status = %d, want 202", resp.StatusCode)
	}

	query := `query($owner:String!,$name:String!,$forkOwner:String!,$forkName:String!){
		source: repository(owner:$owner,name:$name){
			nameWithOwner
			forkCount
		}
		fork: repository(owner:$forkOwner,name:$forkName){
			nameWithOwner
			parent{nameWithOwner databaseId}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{
		"owner":     owner,
		"name":      name,
		"forkOwner": forker.Login,
		"forkName":  "sweep-fork-child",
	})
	source, _ := d["source"].(map[string]interface{})
	if source == nil || source["forkCount"].(float64) != 1 {
		t.Fatalf("source = %v, want forkCount 1", d["source"])
	}
	fork, _ := d["fork"].(map[string]interface{})
	parent, _ := fork["parent"].(map[string]interface{})
	if parent == nil || parent["nameWithOwner"] != owner+"/"+name {
		t.Fatalf("fork parent = %v, want %s/%s", fork["parent"], owner, name)
	}
}

// --- Finding 2: gh pr list (lister query with literal enum orderBy) ---

func TestPRGraphQL_ListQueryShape(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-prlist")
	prNum, _ := sweepPR(t, owner, name, "list me")

	// Verbatim from pkg/cmd/pr/shared/lister.go with gh pr list's default
	// fields expanded per api/query_builder.go.
	query := `fragment pr on PullRequest{number,title,state,url,headRefName,headRepositoryOwner{id,login,...on User{name}},isCrossRepository,isDraft,createdAt}
		query PullRequestList(
			$owner: String!,
			$repo: String!,
			$limit: Int!,
			$endCursor: String,
			$baseBranch: String,
			$headBranch: String,
			$state: [PullRequestState!] = OPEN
		) {
			repository(owner: $owner, name: $repo) {
				pullRequests(
					states: $state,
					baseRefName: $baseBranch,
					headRefName: $headBranch,
					first: $limit,
					after: $endCursor,
					orderBy: {field: CREATED_AT, direction: DESC}
				) {
					totalCount
					nodes {
						...pr
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}`
	d := gqlData(t, query, map[string]interface{}{
		"owner": owner,
		"repo":  name,
		"limit": 30,
		"state": []interface{}{"OPEN"},
	})
	repo, _ := d["repository"].(map[string]interface{})
	prs, _ := repo["pullRequests"].(map[string]interface{})
	if prs == nil {
		t.Fatalf("pullRequests null: %v", d)
	}
	nodes, _ := prs["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("nodes = %v, want 1 PR", prs["nodes"])
	}
	node := nodes[0].(map[string]interface{})
	if int(node["number"].(float64)) != prNum {
		t.Errorf("number = %v, want %d", node["number"], prNum)
	}
	if v, ok := node["isCrossRepository"].(bool); !ok || v {
		t.Errorf("isCrossRepository = %v, want false", node["isCrossRepository"])
	}
	hro, _ := node["headRepositoryOwner"].(map[string]interface{})
	if hro == nil || hro["login"] != owner {
		t.Errorf("headRepositoryOwner = %v, want login %q", node["headRepositoryOwner"], owner)
	}
}

// --- Finding 3: gh pr view's default field set, incl. the commits-aliased
// statusCheckRollup backed by the real checks and commit-status stores ---

func TestPRGraphQL_ViewDefaultFields(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-prview")
	prNum, prID := sweepPR(t, owner, name, "view me")
	createTestUser(t, "sweep-reviewer")
	reqResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum)+"/requested_reviewers", defaultToken,
		map[string]interface{}{"reviewers": []string{"sweep-reviewer"}})
	if reqResp.StatusCode != http.StatusCreated {
		reqResp.Body.Close()
		t.Fatalf("review request status = %d", reqResp.StatusCode)
	}
	reqResp.Body.Close()

	// Submit a review so reviews{} carries data.
	revResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum)+"/reviews", defaultToken,
		map[string]interface{}{"body": "ship it", "event": "APPROVE"})
	if revResp.StatusCode != 200 {
		revResp.Body.Close()
		t.Fatalf("review create status = %d", revResp.StatusCode)
	}
	revResp.Body.Close()

	// Record a completed GitHub Actions workflow job against the PR's head sha
	// so the checks store and GraphQL statusCheckRollup are fed by the same
	// Actions event path as real runs.
	headSHA := pullRequestHeadSHA(testServer.store.GetPullRequest(prID), testServer.store)
	if headSHA == "" {
		t.Fatal("PR head sha did not resolve")
	}
	repoKey := owner + "/" + name
	now := time.Now().UTC()
	runID := testServer.store.ReserveRunID()
	wf := &Workflow{
		ID:           "gql-rollup-" + repoKey,
		Name:         "ci",
		RunID:        runID,
		RunNumber:    runID,
		Status:       WorkflowStatusCompleted,
		Result:       ResultSuccess,
		CreatedAt:    now,
		EventName:    "pull_request",
		Ref:          "refs/heads/main",
		Sha:          headSHA,
		RepoFullName: repoKey,
		Jobs: map[string]*WorkflowJob{
			"build": {
				Key:         "build",
				JobID:       "gql-rollup-job-" + repoKey,
				DisplayName: "build",
				Status:      JobStatusCompleted,
				Result:      ResultSuccess,
				StartedAt:   now,
				CompletedAt: now,
			},
		},
	}
	testServer.store.mu.Lock()
	testServer.store.Workflows[wf.ID] = wf
	testServer.store.mu.Unlock()
	testServer.onActionsRunRequested(wf)
	statusResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/statuses/"+headSHA, defaultToken,
		map[string]interface{}{
			"state":       "failure",
			"target_url":  "https://ci.example.test/unit",
			"description": "unit suite failed",
			"context":     "ci/unit",
		})
	if statusResp.StatusCode != http.StatusCreated {
		statusResp.Body.Close()
		t.Fatalf("commit status create status = %d", statusResp.StatusCode)
	}
	statusResp.Body.Close()

	// Selections assembled exactly as api.PullRequestGraphQL renders gh pr
	// view's defaultFields (projectCards excluded: GHES >= 3.17 drops it).
	query := `fragment pr on PullRequest{
		url,number,title,state,body,
		author{login,...on User{id,name}},
		autoMergeRequest {authorEmail,commitBody,commitHeadline,mergeMethod,enabledAt,enabledBy{login,...on User{id,name}}},
		isDraft,maintainerCanModify,mergeable,additions,deletions,commits{totalCount},
		baseRefName,headRefName,headRepositoryOwner{id,login,...on User{name}},headRepository{id,name,nameWithOwner},isCrossRepository,
		reviewRequests(first: 100) {nodes {requestedReviewer {__typename,...on User{login,name},...on Bot{login},...on Team{organization{login}name,slug}}}},
		reviews(first: 100) {nodes {id,author{login},authorAssociation,submittedAt,body,state,commit{oid},reactionGroups{content,users{totalCount}}}pageInfo{hasNextPage,endCursor}totalCount},
		assignees(first:100){nodes{id,login,name,databaseId},totalCount},
		labels(first:100){nodes{id,name,description,color},totalCount},
		projectItems(first:100){nodes{id, project{id,title}, status:fieldValueByName(name: "Status") { ... on ProjectV2ItemFieldSingleSelectValue{optionId,name}}},totalCount},
		milestone{number,title,description,dueOn},
		comments(first: 100) {nodes {id,author{login,...on User{id,name}},authorAssociation,body,createdAt,includesCreatedEdit,isMinimized,minimizedReason,reactionGroups{content,users{totalCount}},url,viewerDidAuthor},pageInfo{hasNextPage,endCursor},totalCount},
		reactionGroups{content,users{totalCount}},
		createdAt,
		statusCheckRollup: commits(last: 1) {nodes {commit {statusCheckRollup {state, contexts(first:100) {checkRunCount,checkRunCountsByState{state,count},statusContextCount,statusContextCountsByState{state,count},nodes {__typename ...on StatusContext {context,state,targetUrl,createdAt,description}, ...on CheckRun {name,checkSuite{workflowRun{workflow{name}}},status,conclusion,startedAt,completedAt,detailsUrl}},pageInfo{hasNextPage,endCursor}}}}}}
	}
	query PullRequestByNumber($owner: String!, $repo: String!, $pr_number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pr_number) {...pr}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "repo": name, "pr_number": prNum})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	if pr == nil {
		t.Fatalf("pullRequest null: %v", d)
	}

	if pr["autoMergeRequest"] != nil {
		t.Errorf("autoMergeRequest = %v, want null", pr["autoMergeRequest"])
	}
	if v, ok := pr["maintainerCanModify"].(bool); !ok || v {
		t.Errorf("maintainerCanModify = %v, want false", pr["maintainerCanModify"])
	}
	headRepo, _ := pr["headRepository"].(map[string]interface{})
	if headRepo == nil || headRepo["nameWithOwner"] != repoKey {
		t.Errorf("headRepository = %v, want nameWithOwner %q", pr["headRepository"], repoKey)
	}
	reviewRequests, _ := pr["reviewRequests"].(map[string]interface{})
	requestNodes, _ := reviewRequests["nodes"].([]interface{})
	if len(requestNodes) != 1 {
		t.Fatalf("reviewRequests.nodes = %v, want 1", reviewRequests["nodes"])
	}
	requestedReviewer, _ := requestNodes[0].(map[string]interface{})["requestedReviewer"].(map[string]interface{})
	if requestedReviewer == nil || requestedReviewer["__typename"] != "User" || requestedReviewer["login"] != "sweep-reviewer" {
		t.Fatalf("requestedReviewer = %v, want User sweep-reviewer", requestedReviewer)
	}

	reviews, _ := pr["reviews"].(map[string]interface{})
	revNodes, _ := reviews["nodes"].([]interface{})
	if len(revNodes) != 1 {
		t.Fatalf("reviews.nodes = %v, want 1", reviews["nodes"])
	}
	review := revNodes[0].(map[string]interface{})
	if review["submittedAt"] == nil {
		t.Errorf("review.submittedAt = nil, want timestamp")
	}
	commit, _ := review["commit"].(map[string]interface{})
	if commit == nil || commit["oid"] != headSHA {
		t.Errorf("review.commit = %v, want oid %q", review["commit"], headSHA)
	}
	if review["reactionGroups"] == nil {
		t.Errorf("review.reactionGroups = nil, want reaction group list")
	}

	rollupCommits, _ := pr["statusCheckRollup"].(map[string]interface{})
	rcNodes, _ := rollupCommits["nodes"].([]interface{})
	if len(rcNodes) != 1 {
		t.Fatalf("statusCheckRollup commits nodes = %v, want 1", pr["statusCheckRollup"])
	}
	rollup, _ := rcNodes[0].(map[string]interface{})["commit"].(map[string]interface{})["statusCheckRollup"].(map[string]interface{})
	if rollup == nil {
		t.Fatalf("commit.statusCheckRollup null despite recorded status data")
	}
	if rollup["state"] != "FAILURE" {
		t.Errorf("statusCheckRollup.state = %v, want FAILURE from commit status", rollup["state"])
	}
	ctxNodes, _ := rollup["contexts"].(map[string]interface{})["nodes"].([]interface{})
	if len(ctxNodes) != 2 {
		t.Fatalf("contexts.nodes = %v, want StatusContext + CheckRun", rollup["contexts"])
	}
	contexts := rollup["contexts"].(map[string]interface{})
	if contexts["checkRunCount"] != float64(1) || contexts["statusContextCount"] != float64(1) {
		t.Fatalf("rollup counts = checkRun %v statusContext %v, want 1/1", contexts["checkRunCount"], contexts["statusContextCount"])
	}
	if got := countForState(contexts["checkRunCountsByState"], "SUCCESS"); got != 1 {
		t.Fatalf("checkRunCountsByState SUCCESS = %d, want 1: %v", got, contexts["checkRunCountsByState"])
	}
	if got := countForState(contexts["statusContextCountsByState"], "FAILURE"); got != 1 {
		t.Fatalf("statusContextCountsByState FAILURE = %d, want 1: %v", got, contexts["statusContextCountsByState"])
	}
	statusNode := ctxNodes[0].(map[string]interface{})
	if statusNode["__typename"] != "StatusContext" || statusNode["context"] != "ci/unit" ||
		statusNode["state"] != "FAILURE" || statusNode["targetUrl"] != "https://ci.example.test/unit" ||
		statusNode["description"] != "unit suite failed" {
		t.Errorf("status context node = %v, want ci/unit FAILURE", statusNode)
	}
	checkNode := ctxNodes[1].(map[string]interface{})
	if checkNode["__typename"] != "CheckRun" || checkNode["name"] != "build" ||
		checkNode["status"] != "COMPLETED" || checkNode["conclusion"] != "SUCCESS" {
		t.Errorf("check run node = %v, want CheckRun build COMPLETED SUCCESS", checkNode)
	}
	checkSuite, _ := checkNode["checkSuite"].(map[string]interface{})
	workflowRun, _ := checkSuite["workflowRun"].(map[string]interface{})
	workflow, _ := workflowRun["workflow"].(map[string]interface{})
	if workflow == nil || workflow["name"] != "ci" {
		t.Fatalf("checkSuite.workflowRun.workflow = %v, want ci", workflow)
	}
}

func countForState(raw interface{}, state string) int {
	nodes, _ := raw.([]interface{})
	for _, node := range nodes {
		m, _ := node.(map[string]interface{})
		if m["state"] == state {
			switch count := m["count"].(type) {
			case float64:
				return int(count)
			case int:
				return count
			}
		}
	}
	return 0
}

// --- Finding 4: gh pr merge's finder fields + the merge mutation shape ---

func TestPRGraphQL_MergePath(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-prmerge")
	prNum, _ := sweepPR(t, owner, name, "merge me")

	// Finder fields from pkg/cmd/pr/merge/merge.go (isInMergeQueue /
	// isMergeQueueEnabled removed by gh's introspection — bleephub doesn't
	// declare them), rendered per api.PullRequestGraphQL: lastCommit is the
	// commits(last:1) pseudo-field.
	query := `fragment pr on PullRequest{id,number,state,title,commits(last: 1){nodes{commit{oid}}},mergeStateStatus,headRepositoryOwner{id,login,...on User{name}},headRefName,baseRefName,headRefOid}
	query PullRequestByNumber($owner: String!, $repo: String!, $pr_number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $pr_number) {...pr}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "repo": name, "pr_number": prNum})
	pr, _ := d["repository"].(map[string]interface{})["pullRequest"].(map[string]interface{})
	if pr == nil {
		t.Fatalf("pullRequest null: %v", d)
	}
	if pr["mergeStateStatus"] != "CLEAN" {
		t.Errorf("mergeStateStatus = %v, want CLEAN (stored mergeability MERGEABLE)", pr["mergeStateStatus"])
	}
	commits, _ := pr["commits"].(map[string]interface{})
	lcNodes, _ := commits["nodes"].([]interface{})
	if len(lcNodes) != 1 || lcNodes[0].(map[string]interface{})["commit"].(map[string]interface{})["oid"] == "" {
		t.Fatalf("lastCommit nodes = %v, want one commit with oid", pr["commits"])
	}
	prNodeID, _ := pr["id"].(string)

	// Mutation shape from pkg/cmd/pr/merge/http.go (shurcooL-generated):
	// selects only clientMutationId.
	mergeMutation := `mutation PullRequestMerge($input:MergePullRequestInput!){mergePullRequest(input:$input){clientMutationId}}`
	md := gqlData(t, mergeMutation, map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": prNodeID,
			"mergeMethod":   "MERGE",
		},
	})
	if _, ok := md["mergePullRequest"]; !ok {
		t.Fatalf("mergePullRequest payload missing: %v", md)
	}

	// The merge is real: REST reports merged.
	getResp := ghGet(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum), defaultToken)
	prJSON := decodeJSON(t, getResp)
	if merged, _ := prJSON["merged"].(bool); !merged {
		t.Errorf("REST merged = %v after mergePullRequest, want true", prJSON["merged"])
	}
}

// --- Finding 5: gh pr review → addPullRequestReview mutation ---

func TestPRGraphQL_AddPullRequestReview(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-prreview")
	prNum, _ := sweepPR(t, owner, name, "review me")

	// gh pr review resolves the PR with Fields: ["id","number"] first.
	d := gqlData(t, `query($owner:String!,$repo:String!,$n:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$n){id,number}}}`,
		map[string]interface{}{"owner": owner, "repo": name, "n": prNum})
	prNodeID, _ := d["repository"].(map[string]interface{})["pullRequest"].(map[string]interface{})["id"].(string)
	if prNodeID == "" {
		t.Fatalf("no PR node id: %v", d)
	}

	// Mutation shape from api/queries_pr_review.go AddReview
	// (shurcooL-generated, selects clientMutationId).
	mutation := `mutation PullRequestReviewAdd($input:AddPullRequestReviewInput!){addPullRequestReview(input:$input){clientMutationId}}`
	gqlData(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": prNodeID,
			"event":         "REQUEST_CHANGES",
			"body":          "needs work",
		},
	})

	// The review landed in the same store REST serves.
	listResp := ghGet(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum)+"/reviews", defaultToken)
	reviews := decodeJSONArray(t, listResp)
	if len(reviews) != 1 {
		t.Fatalf("REST reviews = %v, want 1", reviews)
	}
	if reviews[0]["state"] != "CHANGES_REQUESTED" || reviews[0]["body"] != "needs work" {
		t.Errorf("review = %v, want CHANGES_REQUESTED 'needs work'", reviews[0])
	}

	// reviewDecision derives from the new review.
	d2 := gqlData(t, `query($owner:String!,$repo:String!,$n:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$n){reviewDecision,latestReviews(first: 100){nodes{author{login},authorAssociation,submittedAt,body,state}}}}}`,
		map[string]interface{}{"owner": owner, "repo": name, "n": prNum})
	pr2, _ := d2["repository"].(map[string]interface{})["pullRequest"].(map[string]interface{})
	if pr2["reviewDecision"] != "CHANGES_REQUESTED" {
		t.Errorf("reviewDecision = %v, want CHANGES_REQUESTED", pr2["reviewDecision"])
	}
	latest, _ := pr2["latestReviews"].(map[string]interface{})
	if nodes, _ := latest["nodes"].([]interface{}); len(nodes) != 1 {
		t.Errorf("latestReviews.nodes = %v, want 1", latest["nodes"])
	}
}

func TestPRGraphQL_ResolveReviewThread(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-prthread")
	prNum, _ := sweepPR(t, owner, name, "thread me")

	root := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum)+"/comments", defaultToken, map[string]interface{}{
		"body":      "please adjust this line",
		"path":      "main.go",
		"line":      3,
		"side":      "RIGHT",
		"commit_id": "abc123",
	}), http.StatusCreated)
	rootID := int(root["id"].(float64))
	decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/pulls/"+itoa(prNum)+"/comments", defaultToken, map[string]interface{}{
		"body":        "addressed",
		"in_reply_to": rootID,
	}), http.StatusCreated)

	query := `query($owner:String!,$repo:String!,$n:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$n){reviewThreads(first:10){nodes{id,isResolved,isOutdated,path,line,comments{totalCount,nodes{body,path,line,state,author{login}}}}}}}}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "repo": name, "n": prNum})
	threads := d["repository"].(map[string]interface{})["pullRequest"].(map[string]interface{})["reviewThreads"].(map[string]interface{})
	nodes, _ := threads["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("reviewThreads.nodes = %v, want one thread", threads["nodes"])
	}
	thread := nodes[0].(map[string]interface{})
	threadID, _ := thread["id"].(string)
	if threadID == "" || thread["isResolved"] != false || thread["path"] != "main.go" {
		t.Fatalf("thread before resolve = %v", thread)
	}
	comments := thread["comments"].(map[string]interface{})
	if comments["totalCount"] != float64(2) {
		t.Fatalf("thread comments = %v, want 2", comments)
	}

	resolve := `mutation($input:ResolveReviewThreadInput!){resolveReviewThread(input:$input){clientMutationId,thread{id,isResolved,comments{totalCount}}}}`
	rd := gqlData(t, resolve, map[string]interface{}{
		"input": map[string]interface{}{
			"threadId":         threadID,
			"clientMutationId": "resolve-1",
		},
	})
	resolvedThread := rd["resolveReviewThread"].(map[string]interface{})["thread"].(map[string]interface{})
	if resolvedThread["id"] != threadID || resolvedThread["isResolved"] != true {
		t.Fatalf("resolved thread = %v, want same id resolved", resolvedThread)
	}
	if rd["resolveReviewThread"].(map[string]interface{})["clientMutationId"] != "resolve-1" {
		t.Fatalf("resolve clientMutationId missing: %v", rd)
	}

	unresolve := `mutation($input:UnresolveReviewThreadInput!){unresolveReviewThread(input:$input){thread{id,isResolved}}}`
	ud := gqlData(t, unresolve, map[string]interface{}{
		"input": map[string]interface{}{
			"threadId": threadID,
		},
	})
	unresolvedThread := ud["unresolveReviewThread"].(map[string]interface{})["thread"].(map[string]interface{})
	if unresolvedThread["id"] != threadID || unresolvedThread["isResolved"] != false {
		t.Fatalf("unresolved thread = %v, want same id unresolved", unresolvedThread)
	}
}

// --- Finding 6: gh release list → Repository.releases connection ---

func TestRepoGraphQL_ReleasesConnection(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-releases")

	for _, rel := range []map[string]interface{}{
		{"tag_name": "v0.9.0", "name": "old"},
		{"tag_name": "v1.0.0", "name": "stable"},
		{"tag_name": "v1.1.0-rc1", "name": "rc", "prerelease": true},
		{"tag_name": "v2.0.0", "name": "draft", "draft": true},
	} {
		resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/releases", defaultToken, rel)
		if decodeJSON(t, resp)["id"] == nil {
			t.Fatalf("release create failed for %v", rel)
		}
	}

	resp := ghPut(t, "/api/v3/repos/"+owner+"/"+name+"/immutable-releases", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("enable immutable releases: %d", resp.StatusCode)
	}

	query := `query RepositoryReleaseList($direction:OrderDirection!$endCursor:String$name:String!$owner:String!$perPage:Int!){repository(owner: $owner, name: $name){releases(first: $perPage, orderBy: {field: CREATED_AT, direction: $direction}, after: $endCursor){nodes{name,tagName,isDraft,immutable,isLatest,isPrerelease,createdAt,publishedAt},pageInfo{hasNextPage,endCursor}}}}`
	d := gqlData(t, query, map[string]interface{}{
		"owner":     owner,
		"name":      name,
		"perPage":   30,
		"endCursor": nil,
		"direction": "DESC",
	})
	releases, _ := d["repository"].(map[string]interface{})["releases"].(map[string]interface{})
	nodes, _ := releases["nodes"].([]interface{})
	if len(nodes) != 4 {
		t.Fatalf("releases.nodes = %d entries, want 4", len(nodes))
	}
	byTag := map[string]map[string]interface{}{}
	for _, n := range nodes {
		m := n.(map[string]interface{})
		byTag[m["tagName"].(string)] = m
	}
	if v, _ := byTag["v1.0.0"]["isLatest"].(bool); !v {
		t.Errorf("v1.0.0 isLatest = %v, want true (newest published non-prerelease)", byTag["v1.0.0"]["isLatest"])
	}
	if v, _ := byTag["v2.0.0"]["isDraft"].(bool); !v {
		t.Errorf("v2.0.0 isDraft = %v, want true", byTag["v2.0.0"]["isDraft"])
	}
	if v, _ := byTag["v1.1.0-rc1"]["isPrerelease"].(bool); !v {
		t.Errorf("v1.1.0-rc1 isPrerelease = %v, want true", byTag["v1.1.0-rc1"]["isPrerelease"])
	}
	if v, _ := byTag["v1.0.0"]["immutable"].(bool); !v {
		t.Errorf("v1.0.0 immutable = %v, want true from repo immutable-release state", byTag["v1.0.0"]["immutable"])
	}
	if byTag["v2.0.0"]["publishedAt"] != nil {
		t.Errorf("draft publishedAt = %v, want null", byTag["v2.0.0"]["publishedAt"])
	}

	intro := gqlData(t, `query Release_fields{Release: __type(name: "Release"){fields{name}}}`, nil)
	fields, _ := intro["Release"].(map[string]interface{})["fields"].([]interface{})
	foundImmutable := false
	for _, f := range fields {
		if f.(map[string]interface{})["name"] == "immutable" {
			foundImmutable = true
			break
		}
	}
	if !foundImmutable {
		t.Fatalf("Release must declare immutable so official clients can use the immutable-aware query")
	}
}

// --- Finding 7: gh release view/download/delete draft lookup ---

func TestRepoGraphQL_ReleaseByTag(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-reltag")

	resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/releases", defaultToken, map[string]interface{}{
		"tag_name": "v9.9.9",
		"name":     "pending draft",
		"draft":    true,
	})
	created := decodeJSON(t, resp)
	wantID, _ := created["id"].(float64)
	if wantID == 0 {
		t.Fatalf("draft release create failed: %v", created)
	}

	// Verbatim shurcooL rendering of fetchDraftRelease's query.
	query := `query RepositoryReleaseByTag($name:String!$owner:String!$tagName:String!){repository(owner: $owner, name: $name){release(tagName: $tagName){databaseId,isDraft}}}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name, "tagName": "v9.9.9"})
	rel, _ := d["repository"].(map[string]interface{})["release"].(map[string]interface{})
	if rel == nil {
		t.Fatalf("release null for existing draft tag: %v", d)
	}
	if got, _ := rel["databaseId"].(float64); got != wantID {
		t.Errorf("databaseId = %v, want %v", rel["databaseId"], wantID)
	}
	if v, _ := rel["isDraft"].(bool); !v {
		t.Errorf("isDraft = %v, want true", rel["isDraft"])
	}

	// Missing tag: plain null, no error (gh keys on the null).
	d2 := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name, "tagName": "v0.0.0-missing"})
	if rel2 := d2["repository"].(map[string]interface{})["release"]; rel2 != nil {
		t.Errorf("release = %v for missing tag, want null", rel2)
	}
}

// --- Finding 8: IssueComment.viewerDidAuthor ---

func TestIssueGraphQL_CommentViewerDidAuthor(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-issuecomment")
	resp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/issues", defaultToken, map[string]interface{}{
		"title": "comment probe",
	})
	issue := decodeJSON(t, resp)
	num := int(issue["number"].(float64))
	cResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/issues/"+itoa(num)+"/comments", defaultToken,
		map[string]interface{}{"body": "mine"})
	cResp.Body.Close()

	// The shared comments fragment gh issue view sends (api/query_builder.go
	// issueComments) selects viewerDidAuthor.
	query := `query($owner:String!,$name:String!,$n:Int!){repository(owner:$owner,name:$name){issue(number:$n){
		comments(first: 100) {nodes {id,author{login,...on User{id,name}},authorAssociation,body,createdAt,includesCreatedEdit,isMinimized,minimizedReason,reactionGroups{content,users{totalCount}},url,viewerDidAuthor},pageInfo{hasNextPage,endCursor},totalCount}
	}}}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name, "n": num})
	comments, _ := d["repository"].(map[string]interface{})["issue"].(map[string]interface{})["comments"].(map[string]interface{})
	nodes, _ := comments["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("comments.nodes = %v, want 1", comments["nodes"])
	}
	if v, _ := nodes[0].(map[string]interface{})["viewerDidAuthor"].(bool); !v {
		t.Errorf("viewerDidAuthor = %v, want true (admin authored, admin viewing)", nodes[0])
	}
}

// --- Finding 9: Query.search — the gh pr status query shape ---

func TestSearchGraphQL_PRStatusShape(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-prstatus")
	sweepPR(t, owner, name, "status pr")

	// Structure from pkg/cmd/pr/status/http.go pullRequestStatus +
	// pullRequestFragment(false, false).
	query := `
	fragment pr on PullRequest{number,title,state,url,isDraft,isCrossRepository,headRefName,headRepositoryOwner{id,login,...on User{name}},mergeStateStatus,baseRef{branchProtectionRule{requiresStrictStatusChecks}},autoMergeRequest {authorEmail,commitBody,commitHeadline,mergeMethod,enabledAt,enabledBy{login,...on User{id,name}}},statusCheckRollup: commits(last: 1) {nodes {commit {statusCheckRollup {contexts(first:100) {nodes {__typename ...on StatusContext {context,state,targetUrl,createdAt,description}, ...on CheckRun {name,checkSuite{workflowRun{workflow{name}}},status,conclusion,startedAt,completedAt,detailsUrl}},pageInfo{hasNextPage,endCursor}}}}}}}
	fragment prWithReviews on PullRequest{...pr,reviewDecision,latestReviews(first: 100) {nodes {author{login},authorAssociation,submittedAt,body,state}}}
	query PullRequestStatus($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
		repository(owner: $owner, name: $repo) {
			defaultBranchRef {
				name
			}
			pullRequests(headRefName: $headRefName, first: $per_page, orderBy: { field: CREATED_AT, direction: DESC }) {
				totalCount
				edges {
					node {
						...prWithReviews
					}
				}
			}
		}
		viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $per_page) {
			totalCount: issueCount
			edges {
				node {
				...prWithReviews
				}
			}
		}
		reviewRequested: search(query: $reviewerQuery, type: ISSUE, first: $per_page) {
			totalCount: issueCount
			edges {
				node {
				...pr
				}
			}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{
		"owner":         owner,
		"repo":          name,
		"headRefName":   "feature",
		"viewerQuery":   fmt.Sprintf("repo:%s/%s state:open is:pr author:admin", owner, name),
		"reviewerQuery": fmt.Sprintf("repo:%s/%s state:open review-requested:admin", owner, name),
	})

	branchPRs, _ := d["repository"].(map[string]interface{})["pullRequests"].(map[string]interface{})
	if tc, _ := branchPRs["totalCount"].(float64); tc != 1 {
		t.Errorf("repository.pullRequests.totalCount = %v, want 1", branchPRs["totalCount"])
	}

	viewerCreated, _ := d["viewerCreated"].(map[string]interface{})
	if tc, _ := viewerCreated["totalCount"].(float64); tc != 1 {
		t.Fatalf("viewerCreated.totalCount = %v, want 1 (author:admin matches)", viewerCreated["totalCount"])
	}
	edges, _ := viewerCreated["edges"].([]interface{})
	node, _ := edges[0].(map[string]interface{})["node"].(map[string]interface{})
	if node["title"] != "status pr" || node["mergeStateStatus"] != "CLEAN" {
		t.Errorf("search node = %v, want the PR with CLEAN mergeStateStatus", node)
	}

	// review-requested: matches nothing — bleephub stores no review
	// requests, so zero results is the true answer.
	reviewRequested, _ := d["reviewRequested"].(map[string]interface{})
	if tc, _ := reviewRequested["totalCount"].(float64); tc != 0 {
		t.Errorf("reviewRequested.totalCount = %v, want 0", reviewRequested["totalCount"])
	}
}

// --- Finding 9 + 11: gh issue list --label goes through Query.search ---

func TestSearchGraphQL_IssueLabelSearch(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-issuesearch")

	lResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/labels", defaultToken,
		map[string]interface{}{"name": "bug", "color": "d73a4a"})
	lResp.Body.Close()
	iResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/issues", defaultToken,
		map[string]interface{}{"title": "labeled", "labels": []interface{}{"bug"}})
	labeled := decodeJSON(t, iResp)
	if labeled["number"] == nil {
		t.Fatalf("labeled issue create failed: %v", labeled)
	}
	iResp2 := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/issues", defaultToken,
		map[string]interface{}{"title": "unlabeled"})
	iResp2.Body.Close()

	// Verbatim from pkg/cmd/issue/list/http.go searchIssues (note: the page
	// size rides the `last` argument), fragment per gh issue list defaults.
	query := `fragment issue on Issue {number,title,url,state,labels(first:100){nodes{id,name,description,color},totalCount},updatedAt,stateReason}
		query IssueSearch($repo: String!, $owner: String!, $type: SearchType!, $limit: Int, $after: String, $query: String!) {
			repository(name: $repo, owner: $owner) {
				hasIssuesEnabled
			}
			search(type: $type, last: $limit, after: $after, query: $query) {
				issueCount
				nodes { ...issue }
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}`
	// The advanced-syntax string gh builds for `gh issue list --label bug`
	// groups the repo qualifier in parentheses.
	d := gqlData(t, query, map[string]interface{}{
		"repo":  name,
		"owner": owner,
		"type":  "ISSUE",
		"limit": 30,
		"query": fmt.Sprintf("label:bug (repo:%s/%s) state:open type:issue", owner, name),
	})
	search, _ := d["search"].(map[string]interface{})
	if ic, _ := search["issueCount"].(float64); ic != 1 {
		t.Fatalf("issueCount = %v, want 1 (only the labeled issue)", search["issueCount"])
	}
	nodes, _ := search["nodes"].([]interface{})
	if len(nodes) != 1 || nodes[0].(map[string]interface{})["title"] != "labeled" {
		t.Errorf("search nodes = %v, want just 'labeled'", search["nodes"])
	}

	// SearchType must NOT carry ISSUE_ADVANCED — gh introspects the enum and
	// would opt into the advanced type when present.
	intro := gqlData(t, `query SearchType_enumValues{SearchType: __type(name: "SearchType"){enumValues(includeDeprecated: true){name}}}`, nil)
	vals, _ := intro["SearchType"].(map[string]interface{})["enumValues"].([]interface{})
	for _, v := range vals {
		if v.(map[string]interface{})["name"] == "ISSUE_ADVANCED" {
			t.Fatalf("SearchType declares ISSUE_ADVANCED — gh would opt into the advanced backend")
		}
	}
}

// --- gh label list / gh issue create --label: RepositoryLabelList sends
// orderBy as literal enums (caught live by the docker harness) ---

func TestRepoGraphQL_LabelListOrderByEnums(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-labellist")
	lResp := ghPost(t, "/api/v3/repos/"+owner+"/"+name+"/labels", defaultToken,
		map[string]interface{}{"name": "enhancement", "color": "a2eeef"})
	lResp.Body.Close()

	// Verbatim shurcooL rendering of api.RepoLabels' query.
	query := `query RepositoryLabelList($endCursor:String$name:String!$owner:String!){repository(owner: $owner, name: $name){labels(first: 100, orderBy: {field: NAME, direction: ASC}, after: $endCursor){nodes{id,name},pageInfo{hasNextPage,endCursor}}}}`
	d := gqlData(t, query, map[string]interface{}{"owner": owner, "name": name, "endCursor": nil})
	labels, _ := d["repository"].(map[string]interface{})["labels"].(map[string]interface{})
	nodes, _ := labels["nodes"].([]interface{})
	if len(nodes) != 1 || nodes[0].(map[string]interface{})["name"] != "enhancement" {
		t.Fatalf("labels.nodes = %v, want [enhancement]", labels["nodes"])
	}
}

// --- gh issue create serializes projectIds (null) in CreateIssueInput —
// the input type must declare it or coercion rejects the whole mutation
// (caught live by the docker harness) ---

func TestIssueGraphQL_CreateWithNullProjectIDs(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-issuecreate")
	repoNodeID := gqlData(t, `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){id}}`,
		map[string]interface{}{"owner": owner, "name": name})["repository"].(map[string]interface{})["id"].(string)

	// Verbatim from api/queries_issue.go IssueCreate; variables mirror what
	// gh sends for `gh issue create --label` (null assigneeIds/projectIds).
	mutation := `
	mutation IssueCreate($input: CreateIssueInput!) {
		createIssue(input: $input) {
			issue {
				id
				url
			}
		}
	}`
	d := gqlData(t, mutation, map[string]interface{}{
		"input": map[string]interface{}{
			"repositoryId": repoNodeID,
			"title":        "created with null projectIds",
			"body":         "probe",
			"assigneeIds":  nil,
			"labelIds":     []interface{}{},
			"projectIds":   nil,
		},
	})
	issue, _ := d["createIssue"].(map[string]interface{})["issue"].(map[string]interface{})
	if issue == nil || issue["id"] == "" {
		t.Fatalf("createIssue returned %v, want an issue", d)
	}
}

// --- Finding 10: NOT_FOUND error fidelity ---

func TestGraphQL_NotFoundErrors(t *testing.T) {
	owner, name := sweepRepo(t, "sweep-notfound")

	assertNotFound := func(t *testing.T, env map[string]interface{}, wantMsgPart string) {
		t.Helper()
		errs, _ := env["errors"].([]interface{})
		if len(errs) == 0 {
			t.Fatalf("no errors[] in response: %v", env)
		}
		e0, _ := errs[0].(map[string]interface{})
		if e0["type"] != "NOT_FOUND" {
			t.Errorf("errors[0].type = %v, want NOT_FOUND", e0["type"])
		}
		msg, _ := e0["message"].(string)
		if !strings.Contains(msg, wantMsgPart) {
			t.Errorf("message = %q, want it to contain %q", msg, wantMsgPart)
		}
	}

	env := gqlDo(t, `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){id}}`,
		map[string]interface{}{"owner": owner, "name": "definitely-missing"})
	assertNotFound(t, env, "Could not resolve to a Repository with the name")
	if data, _ := env["data"].(map[string]interface{}); data["repository"] != nil {
		t.Errorf("data.repository = %v, want null", data["repository"])
	}

	env = gqlDo(t, `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pullRequest(number:9999){id}}}`,
		map[string]interface{}{"owner": owner, "name": name})
	assertNotFound(t, env, "Could not resolve to a PullRequest with the number of 9999")

	env = gqlDo(t, `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){issue(number:9999){id}}}`,
		map[string]interface{}{"owner": owner, "name": name})
	assertNotFound(t, env, "Could not resolve to an Issue with the number of 9999")

	env = gqlDo(t, `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){issueOrPullRequest(number:9999){...on Issue{id}...on PullRequest{id}}}}`,
		map[string]interface{}{"owner": owner, "name": name})
	assertNotFound(t, env, "Could not resolve to an issue or pull request with the number of 9999")
}

// --- Finding 11: GET /api/v3/meta (GHES shape) ---

func TestMetaEndpoint(t *testing.T) {
	resp := ghGet(t, "/api/v3/meta", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("GET /meta status = %d, want 200", resp.StatusCode)
	}
	meta := decodeJSON(t, resp)
	if meta["installed_version"] != "3.21.0" {
		t.Errorf("installed_version = %v, want 3.21.0", meta["installed_version"])
	}
	if v, ok := meta["verifiable_password_authentication"].(bool); !ok || v {
		t.Errorf("verifiable_password_authentication = %v, want false", meta["verifiable_password_authentication"])
	}
}

// --- Finding 12: push-triggered workflows carry event metadata into
// submitWorkflow, so workflow_id resolves ---

func TestWebhookTrigger_WorkflowFileResolvable(t *testing.T) {
	repoKey := "sweeppushowner/sweep-push-repo"
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/ci.yml", sampleWorkflowYAML)

	testServer.triggerWorkflowsForEvent(repoKey, "push", "", "refs/heads/main", nil)

	// The workflow was submitted synchronously above; find it.
	var wf *Workflow
	testServer.store.mu.RLock()
	for _, w := range testServer.store.Workflows {
		if w.RepoFullName == repoKey {
			wf = w
			break
		}
	}
	testServer.store.mu.RUnlock()
	if wf == nil {
		t.Fatalf("no workflow stored for %s after push trigger", repoKey)
	}
	if wf.EventName != "push" || wf.Ref != "refs/heads/main" {
		t.Errorf("workflow event/ref = %q/%q, want push/refs/heads/main", wf.EventName, wf.Ref)
	}
	if wf.Sha == "" || wf.Sha == strings.Repeat("0", 40) {
		t.Errorf("workflow sha = %q, want the real commit sha", wf.Sha)
	}
	if wf.WorkflowFileID == 0 || wf.WorkflowFilePath != ".github/workflows/ci.yml" {
		t.Fatalf("workflow file = id %d path %q, want resolved .github/workflows/ci.yml", wf.WorkflowFileID, wf.WorkflowFilePath)
	}

	// The run's workflow_id must resolve: GET the run, then GET its workflow.
	// workflow_id is an int64 beyond float64's exact range — decode with
	// json.Number (the same trap jq 1.6 hits; the harness uses gh --jq).
	runResp := ghGet(t, "/api/v3/repos/"+repoKey+"/actions/runs/"+itoa(wf.RunID), defaultToken)
	if runResp.StatusCode != 200 {
		runResp.Body.Close()
		t.Fatalf("GET run status = %d", runResp.StatusCode)
	}
	defer runResp.Body.Close()
	var run map[string]interface{}
	dec := json.NewDecoder(runResp.Body)
	dec.UseNumber()
	if err := dec.Decode(&run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	wfIDNum, _ := run["workflow_id"].(json.Number)
	wfID, err := wfIDNum.Int64()
	if err != nil || wfID != wf.WorkflowFileID {
		t.Errorf("run workflow_id = %v, want %d", run["workflow_id"], wf.WorkflowFileID)
	}
	wfResp := ghGet(t, fmt.Sprintf("/api/v3/repos/%s/actions/workflows/%d", repoKey, wfID), defaultToken)
	if wfResp.StatusCode != 200 {
		wfResp.Body.Close()
		t.Fatalf("GET /actions/workflows/{workflow_id} status = %d, want 200 (gh run view resolves this)", wfResp.StatusCode)
	}
	wfJSON := decodeJSON(t, wfResp)
	if wfJSON["path"] != ".github/workflows/ci.yml" {
		t.Errorf("workflow path = %v, want .github/workflows/ci.yml", wfJSON["path"])
	}
}
