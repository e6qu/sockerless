package bleephub

import (
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestPersistenceReload_OwnerAndCountersAndState exercises the reload-path
// fixes:
//   - BUG-1605: Repo.Owner relinked from FullName; per-repo issue-number
//     counter recomputed so post-reload issues don't collide at 0/1.
//   - BUG-1595: workflow_files restored (incl. RepoFullName/YAML), not dropped.
//   - BUG-1608: NextRunID survives reload (no artifact-epoch collision).
//   - BUG-1611: issue lock state persisted.
//   - BUG-1612: user SSH keys + branch protection persisted.
func TestPersistenceReload_OwnerAndCountersAndState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	// --- session 1: create state, then close ---
	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	st1.SeedDefaultUser()
	user := st1.UsersByLogin["admin"]

	repo := st1.CreateRepo(user, "reload-repo", "", false)
	if repo == nil {
		t.Fatal("CreateRepo returned nil")
	}
	i1 := st1.CreateIssue(repo.ID, user.ID, "first", "body", nil, nil, 0)
	i2 := st1.CreateIssue(repo.ID, user.ID, "second", "body", nil, nil, 0)
	if i1.Number != 1 || i2.Number != 2 {
		t.Fatalf("pre-reload issue numbers = %d,%d want 1,2", i1.Number, i2.Number)
	}
	st1.SetIssueOrPRLock(repo.ID, i1.Number, true, "resolved")

	wfFile := st1.RegisterWorkflowFile(repo.FullName, ".github/workflows/ci.yml", "ci", "name: ci\non: push\njobs: {}", "submitted")

	// Reserve a couple run IDs so the counter is well past 1.
	_ = st1.ReserveRunID()
	lastRun := st1.ReserveRunID()

	// Misc.persist is wired by SetPersistence; write the two MiscStore
	// buckets the handlers persist (user_keys, branch_protection) the same
	// way handleCreateUserKey / handleBranchProtectionPut do.
	key := &UserKey{ID: st1.Misc.nextKeyID, Title: "laptop", Key: "ssh-ed25519 AAAA", Verified: true, UserID: user.ID}
	st1.Misc.userKeys[key.ID] = key
	st1.Misc.keysByUser[user.ID] = append(st1.Misc.keysByUser[user.ID], key)
	p1.MustPut("user_keys", "1", key)
	bp := &BranchProtection{}
	st1.Misc.branchProtection[bpKey(repo.ID, "main")] = bp
	p1.MustPut("branch_protection", bpKey(repo.ID, "main"), bp)

	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// --- session 2: reload, assert everything came back coherently ---
	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	defer p2.Close()

	got := st2.GetRepo(user.Login, "reload-repo")
	if got == nil {
		t.Fatal("repo did not persist")
	}
	// BUG-1605: owner relinked.
	if got.Owner == nil {
		t.Fatal("repo Owner is nil after reload (BUG-1605)")
	}
	if got.Owner.Login != user.Login {
		t.Errorf("repo Owner.Login = %q want %q", got.Owner.Login, user.Login)
	}
	// BUG-1605: next issue number resumes at 3, not 1.
	i3 := st2.CreateIssue(got.ID, user.ID, "third", "", nil, nil, 0)
	if i3.Number != 3 {
		t.Errorf("post-reload issue number = %d want 3 (counter must not restart)", i3.Number)
	}
	if st2.GetIssueByNumber(got.ID, 1) == nil || st2.GetIssueByNumber(got.ID, 2) == nil {
		t.Error("persisted issues #1/#2 not retrievable after reload")
	}
	// BUG-1611: lock state survived.
	if locked := st2.GetIssueByNumber(got.ID, 1); locked == nil || !locked.Locked {
		t.Error("issue lock state did not persist (BUG-1611)")
	}

	// BUG-1595: workflow file restored with usable RepoFullName + YAML.
	gotWF := st2.GetWorkflowFile(repo.FullName, wfFile.ID)
	if gotWF == nil {
		t.Fatal("workflow file did not persist (BUG-1595)")
	}
	if gotWF.RepoFullName != repo.FullName || gotWF.YAML == "" {
		t.Errorf("workflow file restored without RepoFullName/YAML: %+v", gotWF)
	}

	// BUG-1608: run-ID counter resumed (next reserved ID is strictly greater
	// than the last one handed out before the restart).
	nextRun := st2.ReserveRunID()
	if nextRun <= lastRun {
		t.Errorf("post-reload run ID = %d, want > %d (counter must not restart)", nextRun, lastRun)
	}

	// BUG-1612: SSH key + branch protection survived.
	if len(st2.Misc.keysByUser[user.ID]) == 0 {
		t.Error("user SSH key did not persist (BUG-1612)")
	}
	if _, ok := st2.Misc.branchProtection[bpKey(got.ID, "main")]; !ok {
		t.Error("branch protection did not persist (BUG-1612)")
	}
}

func TestPersistenceReload_OrganizationRepositoryOwnerIsValidated(t *testing.T) {
	st2 := reloadedStore(t, func(_ *Persistence, st *Store) {
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		org := st.CreateOrg(admin, "persist-owner-org", "Persist Owner", "")
		if org == nil {
			t.Fatal("CreateOrg returned nil")
		}
		if st.CreateOrgRepo(org, admin, "persist-owner-repo", "", false) == nil {
			t.Fatal("CreateOrgRepo returned nil")
		}
	})

	repo := st2.GetRepo("persist-owner-org", "persist-owner-repo")
	if repo == nil {
		t.Fatal("organization repository did not persist")
	}
	if repo.OwnerType != "Organization" {
		t.Fatalf("OwnerType = %q, want Organization", repo.OwnerType)
	}
	org := st2.Orgs[repo.OwnerID]
	if org == nil || org.Login != "persist-owner-org" {
		t.Fatalf("organization owner id=%d resolved to %#v", repo.OwnerID, org)
	}
	if repos := st2.ListReposForOrg("persist-owner-org", RepoListOptions{}); len(repos) != 1 || repos[0].FullName != repo.FullName {
		t.Fatalf("ListReposForOrg returned %#v", repos)
	}
}

func TestPersistenceReload_RepositoryMissingOwnerTypeFailsLoud(t *testing.T) {
	err := reloadWithMutatedPersistedRepo(t, func(raw map[string]interface{}) {
		delete(raw, "owner_type")
	})
	if err == nil || !strings.Contains(err.Error(), `invalid owner_type ""`) {
		t.Fatalf("reload error = %v, want invalid owner_type", err)
	}
}

func TestPersistenceReload_RepositoryMissingOwnerIDFailsLoud(t *testing.T) {
	err := reloadWithMutatedPersistedRepo(t, func(raw map[string]interface{}) {
		delete(raw, "owner_id")
	})
	if err == nil || !strings.Contains(err.Error(), "missing owner_id") {
		t.Fatalf("reload error = %v, want missing owner_id", err)
	}
}

func reloadWithMutatedPersistedRepo(t *testing.T, mutate func(map[string]interface{})) error {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	st1.SeedDefaultUser()
	admin := st1.UsersByLogin["admin"]
	repo := st1.CreateRepo(admin, "strict-owner-repo", "", false)
	if repo == nil {
		t.Fatal("CreateRepo returned nil")
	}
	rawRepos, err := p1.List("repos")
	if err != nil {
		t.Fatalf("list repos: %v", err)
	}
	raw, ok := rawRepos[strconv.Itoa(repo.ID)]
	if !ok {
		t.Fatalf("persisted repo %d not found", repo.ID)
	}
	var row map[string]interface{}
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatalf("unmarshal persisted repo: %v", err)
	}
	mutate(row)
	mutated, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal mutated repo: %v", err)
	}
	if _, err := p1.db.Exec(p1.dialect.putSQL, "repos", strconv.Itoa(repo.ID), mutated); err != nil {
		t.Fatalf("write mutated repo: %v", err)
	}
	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer p2.Close()
	return NewStore().SetPersistence(p2)
}

// reloadedStore runs build against a fresh persisted store, closes the
// database, and returns a second store loaded from the same database —
// the standard restart simulation for reload round-trip tests.
func reloadedStore(t *testing.T, build func(p *Persistence, st *Store)) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)

	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	build(p1, st1)
	if err := p1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("re-load SetPersistence: %v", err)
	}
	t.Cleanup(func() { _ = p2.Close() })
	return st2
}

// addTestUser inserts (and persists) a non-admin user the way the user
// management surface does.
func addTestUser(p *Persistence, st *Store, login string) *User {
	now := time.Now().UTC()
	u := &User{ID: st.NextUser, Login: login, Name: login, Type: "User", CreatedAt: now, UpdatedAt: now}
	st.Users[u.ID] = u
	st.UsersByLogin[u.Login] = u
	st.NextUser++
	p.MustPut("users", strconv.Itoa(u.ID), u)
	return u
}

func TestPersistenceReload_GistsCommentsStarsAndForks(t *testing.T) {
	var gistID, forkID string
	var commentID int

	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		alice := addTestUser(p, st, "alice")

		g, err := st.CreateGistE(admin, "persisted gist", true, map[string]*GistFile{
			"hello.txt": {Filename: "hello.txt", Content: "hello"},
		})
		if err != nil {
			t.Fatalf("CreateGistE: %v", err)
		}
		gistID = g.ID
		nextDescription := "updated gist"
		if _, ok, err := st.UpdateGistE(g.ID, &nextDescription, map[string]*GistFile{
			"second.txt": {Filename: "second.txt", Content: "second"},
		}, nil); err != nil || !ok {
			t.Fatalf("UpdateGistE ok=%v err=%v", ok, err)
		}
		if !st.StarGist(alice.ID, g.ID) {
			t.Fatal("StarGist returned false")
		}
		fork, ok, err := st.ForkGistE(alice, g.ID)
		if err != nil || !ok {
			t.Fatalf("ForkGistE ok=%v err=%v", ok, err)
		}
		forkID = fork.ID
		comment := st.CreateGistComment(g.ID, alice, "first")
		if comment == nil {
			t.Fatal("CreateGistComment returned nil")
		}
		commentID = comment.ID
		if _, ok := st.UpdateGistComment(comment.ID, "edited"); !ok {
			t.Fatal("UpdateGistComment returned false")
		}

		deleted, err := st.CreateGistE(admin, "deleted gist", true, map[string]*GistFile{
			"gone.txt": {Filename: "gone.txt", Content: "gone"},
		})
		if err != nil {
			t.Fatalf("CreateGistE deleted fixture: %v", err)
		}
		deletedComment := st.CreateGistComment(deleted.ID, admin, "gone")
		if deletedComment == nil {
			t.Fatal("CreateGistComment deleted fixture returned nil")
		}
		if !st.StarGist(alice.ID, deleted.ID) {
			t.Fatal("StarGist deleted fixture returned false")
		}
		if !st.DeleteGist(deleted.ID) {
			t.Fatal("DeleteGist returned false")
		}
	})

	g := st2.GetGist(gistID)
	if g == nil {
		t.Fatal("gist did not persist")
	}
	if g.Description != "updated gist" || g.Files["hello.txt"].Content != "hello" || g.Files["second.txt"].Content != "second" {
		t.Fatalf("gist content did not persist: %#v", g)
	}
	if len(g.History) != 2 {
		t.Fatalf("gist history length = %d, want 2", len(g.History))
	}
	if g.Comments != 1 {
		t.Fatalf("gist comments count = %d, want 1", g.Comments)
	}
	comment := st2.GetGistComment(commentID)
	if comment == nil || comment.Body != "edited" {
		t.Fatalf("gist comment did not persist: %#v", comment)
	}
	alice := st2.UsersByLogin["alice"]
	if alice == nil {
		t.Fatal("alice user did not persist")
	}
	if !st2.IsGistStarred(alice.ID, gistID) {
		t.Fatal("starred gist state did not persist")
	}
	forks := st2.ListGistForks(gistID)
	if len(forks) != 1 || forks[0].ID != forkID || forks[0].OwnerID != alice.ID {
		t.Fatalf("gist fork list did not persist: %#v", forks)
	}
	if deleted := st2.ListGistsForUser(st2.UsersByLogin["admin"].ID, time.Time{}); len(deleted) != 1 || deleted[0].ID != gistID {
		t.Fatalf("deleted gist or residue survived reload: %#v", deleted)
	}
	if starred := st2.ListStarredGists(alice.ID); len(starred) != 1 || starred[0].ID != gistID {
		t.Fatalf("deleted starred gist residue survived reload: %#v", starred)
	}
}

func TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren(t *testing.T) {
	var oldRepoID, oldIssueID, oldPRID, projectID int
	const orgLogin = "delete-cascade-org"

	st2 := reloadedStore(t, func(_ *Persistence, st *Store) {
		_, byteStore := newObjectByteStoreForTest(t)
		st.ObjectByteStore = byteStore
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		org := st.CreateOrg(admin, orgLogin, "Delete Cascade", "")
		repo := st.CreateOrgRepo(org, admin, "deleted-issue-children", "", false)
		oldRepoID = repo.ID
		parent := st.CreateIssue(repo.ID, admin.ID, "parent", "", nil, nil, 0)
		child := st.CreateIssue(repo.ID, admin.ID, "child", "", nil, nil, 0)
		blocker := st.CreateIssue(repo.ID, admin.ID, "blocker", "", nil, nil, 0)
		oldIssueID = parent.ID
		now := time.Now().UTC()
		issueComment := st.CreateComment(parent.ID, admin.ID, "stale issue comment")
		if issueComment == nil {
			t.Fatal("CreateComment returned nil")
		}
		if _, _, err := st.Reactions.AddReaction("issue", parent.ID, admin.ID, "+1"); err != nil {
			t.Fatalf("AddReaction issue: %v", err)
		}
		if _, _, err := st.Reactions.AddReaction("issue_comment", issueComment.ID, admin.ID, "heart"); err != nil {
			t.Fatalf("AddReaction issue comment: %v", err)
		}
		st.SetIssueFieldValues(parent.ID, map[int]interface{}{1: "High"})
		st.MarkNotificationsRead(admin.ID, now, repo.FullName)
		issueThreadID := notificationThreadID("Issue", parent.ID)
		st.MarkThreadRead(admin.ID, issueThreadID, now)
		st.MarkThreadDone(admin.ID, issueThreadID)
		st.SetThreadSubscription(admin.ID, issueThreadID, &ThreadSubscription{Subscribed: true, Reason: "manual", CreatedAt: now})
		if err := st.AddSubIssue(parent.ID, child.ID, false); err != nil {
			t.Fatalf("AddSubIssue: %v", err)
		}
		if !st.AddIssueBlockedBy(parent.ID, blocker.ID) {
			t.Fatal("AddIssueBlockedBy returned false")
		}
		project := st.ProjectsV2.CreateProject(org.ID, "Organization", "Repository cleanup", admin.ID)
		projectID = project.ID
		st.ProjectsV2.AddItem(project.ID, "Issue", parent.ID, admin.ID)
		st.RecordRepoActivity(repo.ID, "refs/heads/main", "0000000", "abcdef0", admin.ID, "push")
		st.RecordRepoClone(repo.ID, admin.Login)
		if !st.SetRepoSubscription(admin.ID, repo.ID, true) {
			t.Fatal("SetRepoSubscription returned false")
		}
		if _, err := st.CreateAttestation(repo.ID, []byte(`{"bundle":true}`), []string{"sha256:deadbeef"}, "https://slsa.dev/provenance/v1", admin.Login); err != nil {
			t.Fatalf("CreateAttestation: %v", err)
		}

		st.mu.Lock()
		inst := &Installation{
			ID:                  st.NextInstallationID,
			AppID:               1,
			AppSlug:             "cascade-app",
			TargetType:          "Organization",
			TargetID:            org.ID,
			TargetLogin:         org.Login,
			RepositorySelection: "selected",
			SelectedRepoIDs:     []int{repo.ID},
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		st.NextInstallationID++
		st.Installations[inst.ID] = inst
		token := &InstallationToken{
			Token:          "cascade-installation-token",
			ExpiresAt:      now.Add(time.Hour),
			RepositoryIDs:  []int{repo.ID},
			InstallationID: inst.ID,
			AppID:          inst.AppID,
		}
		st.InstallationTokens[token.Token] = token
		orgActions := st.getOrgActionsPermissionsLocked(org.Login)
		orgActions.SelectedRepositoryIDs = []int{repo.ID}
		orgActions.SelfHostedRunnersSelectedRepoIDs = []int{repo.ID}
		st.RunnerGroups[99] = &RunnerGroup{ID: 99, Name: "cascade", Visibility: "selected", SelectedRepoIDs: []int{repo.ID}, CreatedAt: now}
		st.OrgSecrets[org.Login] = map[string]*OrgSecret{
			"ORG_SECRET": {Secret: Secret{Name: "ORG_SECRET", Value: "secret", CreatedAt: now, UpdatedAt: now}, Visibility: "selected", SelectedRepoIDs: []int{repo.ID}},
		}
		st.OrgVariables[org.Login] = map[string]*ActionsVariable{
			"ORG_VAR": {Name: "ORG_VAR", Value: "value", Visibility: "selected", SelectedRepoIDs: []int{repo.ID}, CreatedAt: now, UpdatedAt: now},
		}
		st.AgentsOrgSecrets[org.Login] = map[string]*OrgSecret{
			"AGENT_SECRET": {Secret: Secret{Name: "AGENT_SECRET", Value: "secret", CreatedAt: now, UpdatedAt: now}, Visibility: "selected", SelectedRepoIDs: []int{repo.ID}},
		}
		st.AgentsOrgVariables[org.Login] = map[string]*ActionsVariable{
			"AGENT_VAR": {Name: "AGENT_VAR", Value: "value", Visibility: "selected", SelectedRepoIDs: []int{repo.ID}, CreatedAt: now, UpdatedAt: now},
		}
		st.DependabotRepositoryAccess[org.Login] = []int{repo.ID}
		st.DependabotOrgSecrets[org.Login] = map[string]*DependabotOrgSecret{
			"DEPENDABOT_SECRET": {DependabotSecret: DependabotSecret{Name: "DEPENDABOT_SECRET", Value: "secret", KeyID: "key", CreatedAt: now, UpdatedAt: now}, Visibility: "selected", SelectedRepoIDs: []int{repo.ID}},
		}
		codespaceScope := codespaceSecretScopeKey("org", org.Login)
		st.CodespaceSecrets[codespaceScope] = map[string]*CodespaceSecret{
			"CODESPACE_SECRET": {Name: "CODESPACE_SECRET", Key: "CODESPACE_SECRET", Visibility: "selected", SelectedRepoIDs: []int{repo.ID}, CreatedAt: now, UpdatedAt: now},
		}
		st.CopilotCodingAgentPerms[org.Login] = &CopilotCodingAgentPermissions{OrgLogin: org.Login, EnabledRepositories: "selected", SelectedRepositoryIDs: []int{repo.ID}}
		st.OrgPrivateRegistries[org.Login] = map[string]*PrivateRegistryConfiguration{
			"registry": {Name: "registry", Visibility: "selected", SelectedRepositoryIDs: []int{repo.ID}, CreatedAt: now, UpdatedAt: now},
		}
		st.OrgImmutableReleases[org.Login] = &OrgImmutableReleasesSettings{EnforcedRepositories: "selected", SelectedRepositoryIDs: []int{repo.ID}}
		st.CodeSecurityRepoAttachments[org.Login] = map[int]int{repo.ID: 321}
		st.EnterpriseCodeSecurityRepoConfigs[repo.ID] = 654
		if st.persist != nil {
			st.persist.MustPut("installations", strconv.Itoa(inst.ID), inst)
			st.persist.MustPut("installation_tokens", token.Token, token)
			st.persist.MustPut("org_actions_permissions", org.Login, orgActions)
			st.persist.MustPut("runner_groups", "99", st.RunnerGroups[99])
			st.persist.MustPut("org_secrets", org.Login, st.OrgSecrets[org.Login])
			st.persist.MustPut("org_variables", org.Login, st.OrgVariables[org.Login])
			st.persist.MustPut("agents_org_secrets", org.Login, st.AgentsOrgSecrets[org.Login])
			st.persist.MustPut("agents_org_variables", org.Login, st.AgentsOrgVariables[org.Login])
			st.persist.MustPut("dependabot_repo_access", org.Login, st.DependabotRepositoryAccess[org.Login])
			st.persist.MustPut("dependabot_org_secrets", org.Login, st.DependabotOrgSecrets[org.Login])
			st.persist.MustPut("codespace_secrets", codespaceScope, st.CodespaceSecrets[codespaceScope])
			st.persist.MustPut("copilot_coding_agent_permissions", org.Login, st.CopilotCodingAgentPerms[org.Login])
			st.persist.MustPut("org_private_registries", org.Login, st.OrgPrivateRegistries[org.Login])
			st.persist.MustPut("org_immutable_releases", org.Login, st.OrgImmutableReleases[org.Login])
			st.persist.MustPut("code_security_repo_attachments", org.Login, st.CodeSecurityRepoAttachments[org.Login])
			st.persist.MustPut("enterprise_code_security_attachments", strconv.Itoa(repo.ID), &EnterpriseCodeSecurityAttachment{RepoID: repo.ID, ConfigID: 654})
		}
		pr := &PullRequest{
			ID:          st.NextPR,
			NodeID:      "PR_kgDOdelete",
			Number:      repo.NextIssueNumber,
			RepoID:      repo.ID,
			Title:       "pull request",
			State:       "OPEN",
			HeadRefName: "feature",
			HeadRepoID:  repo.ID,
			BaseRefName: repo.DefaultBranch,
			AuthorID:    admin.ID,
			Mergeable:   "UNKNOWN",
			CreatedAt:   now,
			UpdatedAt:   now,
			AssigneeIDs: []int{},
			LabelIDs:    []int{},
		}
		st.NextPR++
		repo.NextIssueNumber++
		st.PullRequests[pr.ID] = pr
		st.indexPullLocked(pr)
		if st.persist != nil {
			st.persist.MustPut("pull_requests", strconv.Itoa(pr.ID), pr)
		}
		st.mu.Unlock()
		oldPRID = pr.ID
		prThreadID := notificationThreadID("PullRequest", pr.ID)
		st.MarkThreadRead(admin.ID, prThreadID, now)
		st.MarkThreadDone(admin.ID, prThreadID)
		st.SetThreadSubscription(admin.ID, prThreadID, &ThreadSubscription{Subscribed: true, Reason: "manual", CreatedAt: now})
		st.ProjectsV2.AddItem(projectID, "PullRequest", pr.ID, admin.ID)
		if _, _, err := st.Reactions.AddReaction("pull_request", pr.ID, admin.ID, "rocket"); err != nil {
			t.Fatalf("AddReaction pull request: %v", err)
		}
		prComment := st.CreateCommentFor("pull_request", pr.ID, admin.ID, "stale pull request comment")
		if prComment == nil {
			t.Fatal("CreateCommentFor pull request returned nil")
		}
		if _, _, err := st.Reactions.AddReaction("pull_request_comment", prComment.ID, admin.ID, "eyes"); err != nil {
			t.Fatalf("AddReaction pull request comment: %v", err)
		}
		if st.CreatePRReview(pr.ID, admin.ID, "APPROVED", "approved") == nil {
			t.Fatal("CreatePRReview returned nil")
		}
		reviewComment := st.PRReviewComments.CreateRootComment(pr.ID, admin.ID, "README.md", "review comment", "abc123", "RIGHT", 1, 0)
		if reviewComment == nil {
			t.Fatal("CreateRootComment returned nil")
		}
		if _, _, err := st.Reactions.AddReaction("pull_request_comment", reviewComment.ID, admin.ID, "hooray"); err != nil {
			t.Fatalf("AddReaction pull request review comment: %v", err)
		}
		if deleted, err := st.DeleteRepo(org.Login, repo.Name); err != nil {
			t.Fatalf("DeleteRepo: %v", err)
		} else if !deleted {
			t.Fatal("DeleteRepo returned false")
		}
	})

	if len(st2.Comments) != 0 {
		t.Fatalf("comments survived deleted repo reload: %#v", st2.Comments)
	}
	if len(st2.IssueEvents) != 0 {
		t.Fatalf("issue events survived deleted repo reload: %#v", st2.IssueEvents)
	}
	if got := st2.ListSubIssues(oldIssueID); len(got) != 0 {
		t.Fatalf("sub-issues survived deleted repo reload: %v", got)
	}
	if got := st2.ListIssueBlockedBy(oldIssueID); len(got) != 0 {
		t.Fatalf("issue dependencies survived deleted repo reload: %v", got)
	}
	if values := st2.IssueFieldValues[oldIssueID]; len(values) != 0 {
		t.Fatalf("issue field values survived deleted repo reload: %#v", values)
	}
	assertNotificationStateAbsent(t, st2, "admin", orgLogin+"/deleted-issue-children", notificationThreadID("Issue", oldIssueID), notificationThreadID("PullRequest", oldPRID))
	if got := st2.ProjectsV2.ListItemsForIssue(oldIssueID); len(got) != 0 {
		t.Fatalf("Projects v2 issue items survived deleted repo reload: %#v", got)
	}
	if got := st2.ProjectsV2.ListItemsForPR(oldPRID); len(got) != 0 {
		t.Fatalf("Projects v2 pull request items survived deleted repo reload: %#v", got)
	}
	if got := st2.ProjectsV2.ListItemsForProject(projectID); len(got) != 0 {
		t.Fatalf("Projects v2 project retained deleted repo items after reload: %#v", got)
	}
	if len(st2.PRReviews) != 0 || len(st2.PRReviewsByPR) != 0 {
		t.Fatalf("pull request reviews survived deleted repo reload: reviews=%#v byPR=%#v", st2.PRReviews, st2.PRReviewsByPR)
	}
	if got := st2.PRReviewComments.ListForPR(oldPRID); len(got) != 0 {
		t.Fatalf("pull request review comments survived deleted repo reload: %#v", got)
	}
	if got := reactionStoreCount(st2); got != 0 {
		t.Fatalf("reactions survived deleted issue and pull request parents after reload: %d", got)
	}
	if len(st2.Attestations) != 0 {
		t.Fatalf("attestations survived deleted repo reload: %#v", st2.Attestations)
	}
	if len(st2.RepoActivities) != 0 {
		t.Fatalf("repository activity survived deleted repo reload: %#v", st2.RepoActivities)
	}
	if len(st2.RepoCloneTraffic) != 0 {
		t.Fatalf("repository clone traffic survived deleted repo reload: %#v", st2.RepoCloneTraffic)
	}
	if len(st2.RepoSubscriptions) != 0 {
		t.Fatalf("repository subscriptions survived deleted repo reload: %#v", st2.RepoSubscriptions)
	}
	assertRepoIDAbsentFromCascadeLists(t, st2, orgLogin, oldRepoID)

	admin := st2.UsersByLogin["admin"]
	org := st2.GetOrg(orgLogin)
	if org == nil {
		t.Fatalf("organization %s did not reload", orgLogin)
	}
	recreated := st2.CreateOrgRepo(org, admin, "deleted-issue-children", "", false)
	if recreated.ID != oldRepoID {
		t.Fatalf("fixture did not reuse repository ID after reload: got %d want %d", recreated.ID, oldRepoID)
	}
	fresh := st2.CreateIssue(recreated.ID, admin.ID, "fresh", "", nil, nil, 0)
	if fresh.ID != oldIssueID {
		t.Fatalf("fixture did not reuse issue ID after reload: got %d want %d", fresh.ID, oldIssueID)
	}
	if got := st2.CountCommentsFor("issue", fresh.ID); got != 0 {
		t.Fatalf("fresh issue inherited stale comment count = %d", got)
	}
	if thread := st2.GetNotificationThread(admin, "http://example.test", notificationThreadID("Issue", fresh.ID)); thread == nil {
		t.Fatal("fresh issue did not create a notification thread")
	} else if !thread.Unread {
		t.Fatalf("fresh issue inherited stale read notification state: %#v", thread)
	}
	if got := st2.ProjectsV2.ListItemsForIssue(fresh.ID); len(got) != 0 {
		t.Fatalf("fresh issue inherited stale Projects v2 items: %#v", got)
	}
}

func assertNotificationStateAbsent(t *testing.T, st *Store, login, repoKey string, threadIDs ...string) {
	t.Helper()
	user := st.UsersByLogin[login]
	if user == nil {
		t.Fatalf("user %s did not reload", login)
	}
	state := st.NotificationsState[user.ID]
	if state == nil {
		return
	}
	if _, ok := state.RepoLastReadAt[repoKey]; ok {
		t.Fatalf("notification repo read state survived for %s: %#v", repoKey, state.RepoLastReadAt)
	}
	for _, threadID := range threadIDs {
		if _, ok := state.ReadThreadIDs[threadID]; ok {
			t.Fatalf("notification read state survived for %s: %#v", threadID, state.ReadThreadIDs)
		}
		if _, ok := state.DismissedThreadIDs[threadID]; ok {
			t.Fatalf("notification done state survived for %s: %#v", threadID, state.DismissedThreadIDs)
		}
		if _, ok := state.Subscriptions[threadID]; ok {
			t.Fatalf("notification subscription survived for %s: %#v", threadID, state.Subscriptions)
		}
	}
}

func reactionStoreCount(st *Store) int {
	st.Reactions.mu.RLock()
	defer st.Reactions.mu.RUnlock()
	return len(st.Reactions.byID)
}

func assertRepoIDAbsentFromCascadeLists(t *testing.T, st *Store, orgLogin string, repoID int) {
	t.Helper()
	assertNoRepoID := func(name string, ids []int) {
		t.Helper()
		for _, id := range ids {
			if id == repoID {
				t.Fatalf("%s still referenced deleted repository ID %d: %v", name, repoID, ids)
			}
		}
	}
	for _, inst := range st.Installations {
		assertNoRepoID("installation selected repositories", inst.SelectedRepoIDs)
	}
	for _, token := range st.InstallationTokens {
		assertNoRepoID("installation token repositories", token.RepositoryIDs)
	}
	if p := st.OrgActionsPermissions[orgLogin]; p != nil {
		assertNoRepoID("organization Actions selected repositories", p.SelectedRepositoryIDs)
		assertNoRepoID("organization self-hosted runner repositories", p.SelfHostedRunnersSelectedRepoIDs)
	}
	for _, group := range st.RunnerGroups {
		assertNoRepoID("runner group selected repositories", group.SelectedRepoIDs)
	}
	for _, sec := range st.OrgSecrets[orgLogin] {
		assertNoRepoID("organization secret selected repositories", sec.SelectedRepoIDs)
	}
	for _, v := range st.OrgVariables[orgLogin] {
		assertNoRepoID("organization variable selected repositories", v.SelectedRepoIDs)
	}
	for _, sec := range st.AgentsOrgSecrets[orgLogin] {
		assertNoRepoID("agent organization secret selected repositories", sec.SelectedRepoIDs)
	}
	for _, v := range st.AgentsOrgVariables[orgLogin] {
		assertNoRepoID("agent organization variable selected repositories", v.SelectedRepoIDs)
	}
	assertNoRepoID("Dependabot repository access", st.DependabotRepositoryAccess[orgLogin])
	for _, sec := range st.DependabotOrgSecrets[orgLogin] {
		assertNoRepoID("Dependabot organization secret selected repositories", sec.SelectedRepoIDs)
	}
	for _, sec := range st.CodespaceSecrets[codespaceSecretScopeKey("org", orgLogin)] {
		assertNoRepoID("Codespaces organization secret selected repositories", sec.SelectedRepoIDs)
	}
	if p := st.CopilotCodingAgentPerms[orgLogin]; p != nil {
		assertNoRepoID("Copilot coding agent selected repositories", p.SelectedRepositoryIDs)
	}
	for _, reg := range st.OrgPrivateRegistries[orgLogin] {
		assertNoRepoID("private registry selected repositories", reg.SelectedRepositoryIDs)
	}
	if settings := st.OrgImmutableReleases[orgLogin]; settings != nil {
		assertNoRepoID("immutable releases selected repositories", settings.SelectedRepositoryIDs)
	}
	if attachments := st.CodeSecurityRepoAttachments[orgLogin]; attachments != nil {
		if _, ok := attachments[repoID]; ok {
			t.Fatalf("code security attachments still referenced deleted repository ID %d: %v", repoID, attachments)
		}
	}
	if cfg, ok := st.EnterpriseCodeSecurityRepoConfigs[repoID]; ok {
		t.Fatalf("enterprise code security attachment survived for deleted repository ID %d: %d", repoID, cfg)
	}
}

// G1: app credentials + webhook config survive a restart — JWT auth (PEM),
// OAuth client-secret auth, and app webhook delivery config all depend on them.
func TestPersistenceReload_AppCredentialsAndWebhookConfig(t *testing.T) {
	var app *App
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		app = st.CreateApp(user.ID, "Cred App", "", map[string]string{"checks": "write"}, []string{"push"})
		st.UpdateAppHookConfig(app.ID, func(a *App) {
			a.WebhookURL = "http://sink.localhost/hook"
			a.WebhookEvents = []string{"push", "issues"}
			a.WebhookContentType = "json"
			a.WebhookInsecureSSL = "1"
			a.WebhookActive = false
		})
	})

	got := st2.GetApp(app.ID)
	if got == nil {
		t.Fatal("app did not persist")
	}
	if got.PEMPrivateKey == "" || got.PEMPrivateKey != app.PEMPrivateKey {
		t.Error("app PEM private key did not round-trip — JWT auth dead after restart")
	}
	if got.ClientSecret == "" || got.ClientSecret != app.ClientSecret {
		t.Error("app client secret did not round-trip")
	}
	if st2.VerifyAppClientSecret(app.ClientID, app.ClientSecret) == nil {
		t.Error("VerifyAppClientSecret fails after reload")
	}
	if got.WebhookSecret != app.WebhookSecret {
		t.Error("app webhook secret did not round-trip")
	}
	if got.WebhookURL != "http://sink.localhost/hook" {
		t.Errorf("app webhook URL = %q after reload", got.WebhookURL)
	}
	if len(got.WebhookEvents) != 2 || got.WebhookEvents[0] != "push" {
		t.Errorf("app webhook events = %v after reload", got.WebhookEvents)
	}
	if got.WebhookContentType != "json" || got.WebhookInsecureSSL != "1" || got.WebhookActive {
		t.Errorf("app webhook config drifted after reload: content_type=%q insecure_ssl=%q active=%v",
			got.WebhookContentType, got.WebhookInsecureSSL, got.WebhookActive)
	}
}

// G2: hook secret survives (deliveries keep signing identically) and RepoKey
// is rebuilt from the bucket key.
func TestPersistenceReload_HookSecretSignatureAndRepoKey(t *testing.T) {
	var hookID int
	const repoKey = "admin/hooked"
	payload := []byte(`{"action":"opened"}`)
	var wantSig string
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		if st.CreateRepo(user, "hooked", "", false) == nil {
			t.Fatal("CreateRepo returned nil")
		}
		hook := st.CreateHook(repoKey, "http://sink.localhost/h", "s3cret", "json", "0", []string{"push"}, true)
		hookID = hook.ID
		wantSig = computeHMACSignature(hook.Secret, payload)
	})

	got := st2.GetHook(repoKey, hookID)
	if got == nil {
		t.Fatal("hook did not persist")
	}
	if got.Secret != "s3cret" {
		t.Errorf("hook secret = %q after reload — X-Hub-Signature-256 would be missing/wrong", got.Secret)
	}
	if sig := computeHMACSignature(got.Secret, payload); sig != wantSig {
		t.Errorf("post-reload signature %q != pre-restart %q", sig, wantSig)
	}
	if got.RepoKey != repoKey {
		t.Errorf("hook RepoKey = %q after reload, want %q (backfilled from bucket key)", got.RepoKey, repoKey)
	}
}

// G3: secret VALUES reload, not just names.
func TestPersistenceReload_SecretValue(t *testing.T) {
	const repoKey = "admin/sealed"
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		if st.CreateRepo(user, "sealed", "", false) == nil {
			t.Fatal("CreateRepo returned nil")
		}
		// Mirror handlePutSecret's store mutation + write-through.
		now := time.Now().UTC()
		st.RepoSecrets[repoKey] = map[string]*Secret{
			"TOKEN": {Name: "TOKEN", Value: "hunter2", CreatedAt: now, UpdatedAt: now},
		}
		p.MustPut("repo_secrets", repoKey, st.RepoSecrets[repoKey])
	})

	sec := st2.RepoSecrets[repoKey]["TOKEN"]
	if sec == nil {
		t.Fatal("secret did not persist")
	}
	if sec.Value != "hunter2" {
		t.Errorf("secret value = %q after reload, want %q — workflow secret injection dead", sec.Value, "hunter2")
	}
}

// G4: releases re-index under their repo (not byRepo[0]) and keep their author.
func TestPersistenceReload_ReleasesByRepo(t *testing.T) {
	var repoID, relID, authorID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "released", "", false)
		repoID, authorID = repo.ID, user.ID
		rel := st.Releases.Create(repo.ID, user.ID, "v1.0.0", "main", "First", "notes", false, false)
		relID = rel.ID
	})

	list := st2.Releases.List(repoID)
	if len(list) != 1 || list[0].ID != relID {
		t.Fatalf("releases for repo %d after reload = %v, want exactly release %d", repoID, list, relID)
	}
	if got := st2.Releases.Get(relID); got.RepoID != repoID || got.AuthorID != authorID {
		t.Errorf("release linkage after reload: repo_id=%d author_id=%d, want %d/%d", got.RepoID, got.AuthorID, repoID, authorID)
	}
	if st2.Releases.GetByTag(repoID, "v1.0.0") == nil {
		t.Error("release-by-tag lookup dead after reload")
	}
	if len(st2.Releases.List(0)) != 0 {
		t.Error("releases leaked into byRepo[0] after reload")
	}
}

// G5: deployments keep creator/repo linkage, statuses relink to their
// deployment in creation order, and environments keep reviewers + wait
// timer (deployment-approval gating) under the bucket key.
func TestPersistenceReload_DeploymentsStatusesEnvironments(t *testing.T) {
	var repoID, depID, creatorID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "deployed", "", false)
		repoID, creatorID = repo.ID, user.ID
		st.Deployments.UpsertEnvironment(repo.ID, "production")
		wait := 30
		st.Deployments.SetEnvironmentProtection(repo.ID, "production", &wait,
			[]map[string]interface{}{{"type": "User", "id": user.ID}})
		d := st.Deployments.CreateDeployment(repo.ID, user.ID, "main", "abc123", "deploy", "production", "ship it", nil, true, false)
		depID = d.ID
		st.Deployments.AddStatus(d.ID, user.ID, "in_progress", "", "", "", "", "production", false)
		st.Deployments.AddStatus(d.ID, user.ID, "success", "", "", "", "", "production", false)
	})

	deps := st2.Deployments.ListDeployments(repoID)
	if len(deps) != 1 || deps[0].ID != depID {
		t.Fatalf("deployments for repo %d after reload = %d entries, want exactly deployment %d", repoID, len(deps), depID)
	}
	if deps[0].CreatorID != creatorID {
		t.Errorf("deployment creator_id = %d after reload, want %d", deps[0].CreatorID, creatorID)
	}
	statuses := st2.Deployments.ListStatuses(depID)
	if len(statuses) != 2 {
		t.Fatalf("deployment statuses after reload = %d, want 2 (relinked from own bucket)", len(statuses))
	}
	if statuses[0].ID > statuses[1].ID {
		t.Error("deployment statuses not in creation (ID) order after reload")
	}
	if statuses[1].State != "success" || statuses[1].DeploymentID != depID || statuses[1].CreatorID != creatorID {
		t.Errorf("status linkage after reload: %+v", statuses[1])
	}
	env := st2.Deployments.GetEnvironment(repoID, "production")
	if env == nil {
		t.Fatal("environment not retrievable by (repo, name) after reload")
	}
	if env.WaitTimer != 30 {
		t.Errorf("environment wait_timer = %d after reload, want 30 — approval gating broken", env.WaitTimer)
	}
	if len(env.Reviewers) != 1 {
		t.Fatalf("environment reviewers lost on reload — approval gating broken")
	}
	if envs := st2.Deployments.ListEnvironments(repoID); len(envs) != 1 {
		t.Errorf("environments for repo after reload = %d, want 1", len(envs))
	}
}

// G6: reactions re-index under their parent with their user.
func TestPersistenceReload_Reactions(t *testing.T) {
	var issueID, userID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "reacted", "", false)
		issue := st.CreateIssue(repo.ID, user.ID, "hot take", "", nil, nil, 0)
		issueID, userID = issue.ID, user.ID
		if _, _, err := st.Reactions.AddReaction("issue", issue.ID, user.ID, "+1"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := st.Reactions.AddReaction("issue", issue.ID, user.ID, "heart"); err != nil {
			t.Fatal(err)
		}
	})

	got := st2.Reactions.ListReactions("issue", issueID, "")
	if len(got) != 2 {
		t.Fatalf("reactions on issue after reload = %d, want 2", len(got))
	}
	for _, r := range got {
		if r.ParentType != "issue" || r.ParentID != issueID || r.UserID != userID {
			t.Errorf("reaction linkage after reload: %+v", r)
		}
	}
	if sum := st2.Reactions.SummarizeReactions("issue", issueID); sum["total_count"] != 2 {
		t.Errorf("reaction summary total_count = %v after reload, want 2", sum["total_count"])
	}
}

func TestPersistenceReload_ReactionParentDeletion(t *testing.T) {
	st2 := reloadedStore(t, func(_ *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "reaction-delete", "", false)
		issue := st.CreateIssue(repo.ID, user.ID, "cleanup", "", nil, nil, 0)
		comment := st.CreateComment(issue.ID, user.ID, "cleanup comment")
		if comment == nil {
			t.Fatal("CreateComment returned nil")
		}
		if _, _, err := st.Reactions.AddReaction("issue", issue.ID, user.ID, "+1"); err != nil {
			t.Fatalf("AddReaction issue: %v", err)
		}
		if _, _, err := st.Reactions.AddReaction("issue_comment", comment.ID, user.ID, "heart"); err != nil {
			t.Fatalf("AddReaction issue comment: %v", err)
		}
		st.Reactions.DeleteParent("issue", issue.ID)
		if !st.DeleteComment(comment.ID) {
			t.Fatal("DeleteComment returned false")
		}
	})

	if got := reactionStoreCount(st2); got != 0 {
		t.Fatalf("deleted reaction parents survived reload: %d", got)
	}
}

// G7: PR review comments re-index under their PR with author, thread and
// resolved state intact.
func TestPersistenceReload_PRReviewComments(t *testing.T) {
	var prID, rootID, replyID, authorID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "reviewed", "", false)
		seedStorePullRequestBranches(t, st, repo, "feature")
		pr := st.CreatePullRequest(repo.ID, user.ID, "fix", "", "feature", "main", false, nil, nil, 0)
		prID, authorID = pr.ID, user.ID
		root := st.PRReviewComments.CreateRootComment(pr.ID, user.ID, "main.go", "off-by-one?", "abc123", "RIGHT", 10, 0)
		rootID = root.ID
		reply := st.PRReviewComments.Reply(pr.ID, root.ID, user.ID, "confirmed")
		replyID = reply.ID
		if !st.PRReviewComments.ResolveThread(root.ID, true) {
			t.Fatal("ResolveThread failed")
		}
	})

	list := st2.PRReviewComments.ListForPR(prID)
	if len(list) != 2 {
		t.Fatalf("PR review comments for PR %d after reload = %d, want 2 (byPR[0] regression)", prID, len(list))
	}
	root := st2.PRReviewComments.Get(rootID)
	if root.PullRequestID != prID || root.AuthorID != authorID {
		t.Errorf("root comment linkage after reload: %+v", root)
	}
	if !root.Resolved {
		t.Error("thread resolved flag lost on reload")
	}
	reply := st2.PRReviewComments.Get(replyID)
	if reply.ThreadID != rootID {
		t.Errorf("reply thread id = %d after reload, want %d", reply.ThreadID, rootID)
	}
	threads := st2.PRReviewComments.ListThreads(prID)
	if len(threads) != 1 || len(threads[0].Comments) != 2 || !threads[0].IsResolved {
		t.Errorf("threads after reload = %+v, want one resolved thread with 2 comments", threads)
	}
}

// G8: check runs/suites keep their repo key so commit-scoped lookups match,
// and check-run annotations survive.
func TestPersistenceReload_CheckRunsAndSuites(t *testing.T) {
	const repoKey = "admin/checked"
	const sha = "deadbeefcafe"
	var runID, suiteID int64
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		if st.CreateRepo(user, "checked", "", false) == nil {
			t.Fatal("CreateRepo returned nil")
		}
		app := st.CreateApp(user.ID, "CI App", "", nil, nil)
		cr := st.CreateCheckRun(repoKey, sha, "build", app.ID, 0)
		runID, suiteID = cr.ID, cr.SuiteID
		st.UpdateCheckRun(cr.ID, func(c *CheckRun) {
			c.Status = "completed"
			c.Conclusion = "failure"
			c.Output = &CheckRunOutput{
				Title:            "1 failure",
				Summary:          "boom",
				AnnotationsCount: 1,
				Annotations: []*CheckAnnotation{
					{Path: "main.go", StartLine: 3, EndLine: 3, AnnotationLevel: "failure", Message: "nil deref"},
				},
			}
		})
		st.UpdateCheckSuite(cr.SuiteID, func(s *CheckSuite) {
			s.WorkflowRunID = 42
			s.WorkflowRunBackendID = "workflow-backend-42"
			s.WorkflowName = "ci"
			s.WorkflowFileID = 99
			s.WorkflowFilePath = ".github/workflows/ci.yml"
		})
	})

	runs := st2.ListCheckRunsForCommit(repoKey, sha, "", "", 0)
	if len(runs) != 1 || runs[0].ID != runID {
		t.Fatalf("check runs for commit after reload = %d, want exactly run %d (RepoKey lost?)", len(runs), runID)
	}
	if runs[0].Output == nil || len(runs[0].Output.Annotations) != 1 || runs[0].Output.Annotations[0].Message != "nil deref" {
		t.Errorf("check-run annotations did not round-trip: %+v", runs[0].Output)
	}
	suites := st2.ListCheckSuitesForCommit(repoKey, sha, 0)
	if len(suites) != 1 || suites[0].ID != suiteID {
		t.Fatalf("check suites for commit after reload = %d, want exactly suite %d (RepoKey lost?)", len(suites), suiteID)
	}
	if suites[0].WorkflowRunID != 42 || suites[0].WorkflowRunBackendID != "workflow-backend-42" ||
		suites[0].WorkflowName != "ci" || suites[0].WorkflowFileID != 99 ||
		suites[0].WorkflowFilePath != ".github/workflows/ci.yml" {
		t.Fatalf("check suite workflow metadata did not round-trip: %+v", suites[0])
	}
}

func TestPersistenceReload_WorkflowRunsAndAttempts(t *testing.T) {
	const repoKey = "admin/actions-persist"
	now := time.Now().UTC()
	var completedRunID, runningRunID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		if st.CreateRepo(user, "actions-persist", "", false) == nil {
			t.Fatal("CreateRepo returned nil")
		}
		completedRunID = st.ReserveRunID()
		completed := &Workflow{
			ID:           "completed-run",
			Name:         "ci",
			RunID:        completedRunID,
			RunNumber:    completedRunID,
			Status:       WorkflowStatusCompleted,
			Result:       ResultSuccess,
			CreatedAt:    now,
			EventName:    "push",
			Ref:          "refs/heads/main",
			Sha:          "1111111111111111111111111111111111111111",
			RepoFullName: repoKey,
			Jobs: map[string]*WorkflowJob{
				"build": {
					Key:         "build",
					JobID:       "job-completed",
					DisplayName: "build",
					Status:      JobStatusCompleted,
					Result:      ResultSuccess,
					StartedAt:   now,
					CompletedAt: now,
				},
			},
			WorkflowFileID:   1001,
			WorkflowFilePath: ".github/workflows/ci.yml",
		}
		attempt := *completed
		attempt.ID = "completed-run-attempt-1"
		attempt.Attempt = 1
		attempt.Result = ResultFailure
		runningRunID = st.ReserveRunID()
		running := &Workflow{
			ID:           "running-run",
			Name:         "deploy",
			RunID:        runningRunID,
			RunNumber:    runningRunID,
			Status:       WorkflowStatusRunning,
			CreatedAt:    now,
			EventName:    "workflow_dispatch",
			Ref:          "refs/heads/main",
			Sha:          "2222222222222222222222222222222222222222",
			RepoFullName: repoKey,
			Jobs: map[string]*WorkflowJob{
				"deploy": {
					Key:         "deploy",
					JobID:       "job-running",
					DisplayName: "deploy",
					Status:      JobStatusRunning,
					StartedAt:   now,
				},
			},
		}
		st.Workflows[completed.ID] = completed
		st.Workflows[running.ID] = running
		st.WorkflowAttempts[completedRunID] = []*Workflow{&attempt}
		st.persistWorkflowRecord(completed)
		st.persistWorkflowRecord(running)
		st.persistWorkflowAttemptsRecord(completedRunID)
	})

	completed := st2.Workflows["completed-run"]
	if completed == nil || completed.RunID != completedRunID || completed.RepoFullName != repoKey ||
		completed.Status != WorkflowStatusCompleted || completed.Result != ResultSuccess ||
		completed.WorkflowFilePath != ".github/workflows/ci.yml" {
		t.Fatalf("completed workflow after reload = %+v", completed)
	}
	if got := completed.Jobs["build"]; got == nil || got.Status != JobStatusCompleted || got.Result != ResultSuccess {
		t.Fatalf("completed job after reload = %+v", got)
	}
	attempts := st2.WorkflowAttempts[completedRunID]
	if len(attempts) != 1 || attempts[0].ID != "completed-run-attempt-1" || attempts[0].Result != ResultFailure {
		t.Fatalf("workflow attempts after reload = %+v", attempts)
	}
	running := st2.Workflows["running-run"]
	if running == nil || running.RunID != runningRunID || running.Status != WorkflowStatusCompleted ||
		running.Result != ResultCancelled || !running.CancelRequested {
		t.Fatalf("running workflow after reload = %+v, want completed/cancelled abandoned run", running)
	}
	if got := running.Jobs["deploy"]; got == nil || got.Status != JobStatusCompleted || got.Result != ResultCancelled ||
		got.CompletedAt.IsZero() {
		t.Fatalf("running job after reload = %+v, want completed/cancelled", got)
	}
}

// G9: installation selected-repo lists and token repo scoping survive reload,
// including the state left by add/remove mutations.
func TestPersistenceReload_InstallationSelectedRepos(t *testing.T) {
	var instID, r1ID, r2ID int
	var tokValue string
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		r1 := st.CreateRepo(user, "sel-one", "", false)
		r2 := st.CreateRepo(user, "sel-two", "", false)
		r1ID, r2ID = r1.ID, r2.ID
		app := st.CreateApp(user.ID, "Sel App", "", nil, nil)
		inst := st.CreateInstallation(app.ID, "User", user.ID, user.Login, nil, nil)
		instID = inst.ID
		st.SetInstallationRepositorySelection(inst.ID, "selected", []int{r1.ID})
		if added, ok := st.AddInstallationRepo(inst.ID, r2.ID); !added || !ok {
			t.Fatal("AddInstallationRepo failed")
		}
		if removed, ok := st.RemoveInstallationRepo(inst.ID, r1.ID); !removed || !ok {
			t.Fatal("RemoveInstallationRepo failed")
		}
		tok := st.CreateInstallationToken(inst.ID, app.ID, nil, []int{r2.ID})
		tokValue = tok.Token
	})

	inst := st2.GetInstallation(instID)
	if inst == nil {
		t.Fatal("installation did not persist")
	}
	if inst.RepositorySelection != "selected" {
		t.Errorf("repository_selection = %q after reload", inst.RepositorySelection)
	}
	if len(inst.SelectedRepoIDs) != 1 || inst.SelectedRepoIDs[0] != r2ID {
		t.Errorf("selected repos after reload = %v, want [%d] (add %d / remove %d must persist)",
			inst.SelectedRepoIDs, r2ID, r2ID, r1ID)
	}
	tok, _ := st2.LookupInstallationToken(tokValue)
	if tok == nil {
		t.Fatal("installation token did not persist")
	}
	if len(tok.RepositoryIDs) != 1 || tok.RepositoryIDs[0] != r2ID {
		t.Errorf("token repository scoping after reload = %v, want [%d]", tok.RepositoryIDs, r2ID)
	}
}

// G10 + G11: removed org members stay removed; team member/repo mutations
// survive a restart.
func TestPersistenceReload_OrgMembershipAndTeams(t *testing.T) {
	var devID, adminID, teamID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		adminID = admin.ID
		dev := addTestUser(p, st, "dev")
		devID = dev.ID
		org := st.CreateOrg(admin, "acme", "Acme", "")
		if org == nil {
			t.Fatal("CreateOrg returned nil")
		}
		if st.SetMembership("acme", dev.ID, OrgRoleMember, MembershipStateActive) == nil {
			t.Fatal("SetMembership failed")
		}
		team := st.CreateTeam("acme", "Platform", TeamOptions{Privacy: TeamPrivacyClosed, Permission: TeamPermissionPush})
		teamID = team.ID
		st.CreateOrgRepo(org, admin, "infra", "", false)
		st.CreateOrgRepo(org, admin, "app", "", false)
		st.SetTeamMembership("acme", "platform", admin.ID, TeamRoleMaintainer)
		st.SetTeamMembership("acme", "platform", dev.ID, TeamRoleMember)
		st.SetTeamRepoPermission("acme", "platform", "acme/infra", "")
		st.SetTeamRepoPermission("acme", "platform", "acme/app", "")
		st.RemoveTeamRepo("acme", "platform", "acme/infra")
		// Removing the org membership also strips the user from org teams.
		if !st.RemoveMembership("acme", dev.ID) {
			t.Fatal("RemoveMembership failed")
		}
	})

	if st2.GetMembership("acme", devID) != nil {
		t.Error("removed org membership resurrected after reload")
	}
	if st2.GetMembership("acme", adminID) == nil {
		t.Error("creator admin membership did not persist")
	}
	team := st2.GetTeam("acme", "platform")
	if team == nil || team.ID != teamID {
		t.Fatal("team did not persist")
	}
	if len(team.MemberIDs) != 1 || team.MemberIDs[0] != adminID {
		t.Errorf("team members after reload = %v, want [%d] (membership-removal strip must persist)", team.MemberIDs, adminID)
	}
	if len(team.RepoNames) != 1 || team.RepoNames[0] != "acme/app" {
		t.Errorf("team repos after reload = %v, want [acme/app]", team.RepoNames)
	}
}

// G12 (negative tests): rotated and grant-revoked user-to-server credentials
// must stay dead after a restart.
func TestPersistenceReload_RevokedAndRotatedTokensStayDead(t *testing.T) {
	var oldTok, oldRefresh, newTok, revokedTok, revokedRefresh string
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		appA := st.CreateOAuthApp(user.ID, "Rotator", "", "", "")
		appB := st.CreateOAuthApp(user.ID, "Revoked", "", "", "")

		tokA, rtA := st.CreateUserToServerToken(user.ID, 0, appA.ClientID, "repo", time.Hour, true)
		oldTok, oldRefresh = tokA.Token, rtA.Token
		rotated, _ := st.RotateUserToServerToken(rtA.Token)
		if rotated == nil {
			t.Fatal("RotateUserToServerToken failed")
		}
		newTok = rotated.Token

		tokB, rtB := st.CreateUserToServerToken(user.ID, 0, appB.ClientID, "repo", time.Hour, true)
		revokedTok, revokedRefresh = tokB.Token, rtB.Token
		if n := st.RevokeUserGrant(appB.ClientID, user.ID); n != 1 {
			t.Fatalf("RevokeUserGrant revoked %d tokens, want 1", n)
		}
	})

	if tok, _ := st2.LookupUserToServerToken(oldTok); tok != nil {
		t.Error("rotated-out user token resurrected after reload")
	}
	if got, _ := st2.RotateUserToServerToken(oldRefresh); got != nil {
		t.Error("rotated-out refresh token resurrected after reload")
	}
	if tok, _ := st2.LookupUserToServerToken(newTok); tok == nil {
		t.Error("rotated-in user token did not persist")
	}
	if tok, _ := st2.LookupUserToServerToken(revokedTok); tok != nil {
		t.Error("grant-revoked user token resurrected after reload")
	}
	if got, _ := st2.RotateUserToServerToken(revokedRefresh); got != nil {
		t.Error("grant-revoked refresh token resurrected after reload")
	}
}

// G13: app-level deliveries reload under the app ID (the bucket key), not
// the synthetic delivery HookID.
func TestPersistenceReload_AppHookDeliveries(t *testing.T) {
	var appID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		app := st.CreateApp(user.ID, "Delivery App", "", nil, nil)
		appID = app.ID
		st.AddAppDelivery(app.ID, &WebhookDelivery{
			HookID:      -app.ID, // app-level synthetic hook id
			AppID:       app.ID,
			TargetURL:   "http://sink.localhost/app",
			GUID:        "guid-1",
			Event:       "installation",
			Action:      "created",
			StatusCode:  200,
			DeliveredAt: time.Now().UTC(),
		})
	})

	deliveries := st2.ListAppDeliveries(appID)
	if len(deliveries) != 1 {
		t.Fatalf("ListAppDeliveries after reload = %d, want 1 (filed under HookID instead of app ID?)", len(deliveries))
	}
	if deliveries[0].GUID != "guid-1" || deliveries[0].Event != "installation" {
		t.Errorf("app delivery content after reload: %+v", deliveries[0])
	}
}

// H2: audit log order is newest-first (ID descending) after reload, not
// map-iteration order.
func TestPersistenceReload_AuditLogOrdering(t *testing.T) {
	const n = 8
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		// Mirror recordAuditEvent's mutation + write-through.
		for i := 0; i < n; i++ {
			st.Misc.nextAuditID++
			entry := &AuditEntry{
				ID:        st.Misc.nextAuditID,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Action:    "repo.create",
				Actor:     "admin",
				Version:   "1.1",
			}
			st.Misc.auditLog = append([]*AuditEntry{entry}, st.Misc.auditLog...)
			p.MustPut("audit_log", strconv.FormatInt(entry.ID, 10), entry)
		}
	})

	if len(st2.Misc.auditLog) != n {
		t.Fatalf("audit log after reload = %d entries, want %d", len(st2.Misc.auditLog), n)
	}
	for i, e := range st2.Misc.auditLog {
		if want := int64(n - i); e.ID != want {
			t.Fatalf("audit log entry %d has ID %d, want %d (must be newest-first)", i, e.ID, want)
		}
	}
	if st2.Misc.nextAuditID != n {
		t.Errorf("nextAuditID = %d after reload, want %d", st2.Misc.nextAuditID, n)
	}
}

func TestPersistenceReload_PagesBuildIDSequence(t *testing.T) {
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "pages", "", false)
		if repo == nil {
			t.Fatal("CreateRepo returned nil")
		}
		st.Misc.pagesBuilds[repo.FullName] = []*PagesBuild{
			{ID: 41, URL: "http://127.0.0.1/api/v3/repos/admin/pages/pages/builds/41", Status: "built"},
			{ID: 9, URL: "http://127.0.0.1/api/v3/repos/admin/pages/pages/builds/9", Status: "built"},
		}
		p.MustPut("pages_builds", repo.FullName, st.Misc.pagesBuilds[repo.FullName])
		st.Misc.nextAuditID = 7
		p.MustPut("audit_log", "7", &AuditEntry{ID: 7, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Action: "repo.create", Actor: "admin", Version: "1.1"})
	})

	if st2.Misc.nextPagesBuildID != 42 {
		t.Fatalf("nextPagesBuildID = %d, want 42", st2.Misc.nextPagesBuildID)
	}
	if st2.Misc.nextAuditID != 7 {
		t.Fatalf("nextAuditID = %d, want 7", st2.Misc.nextAuditID)
	}
}

// H4: deleting a repo purges everything keyed to it, so a restart plus a
// recreated same-name repo inherits nothing.
func TestPersistenceReload_DeleteRepoLeavesNoResidue(t *testing.T) {
	const repoName = "doomed"
	const repoKey = "admin/" + repoName
	const controllerKey = "admin/variant-controller"
	var oldRepoID int
	var codespaceWorkspace string
	var objectFS *s3FS
	var codeQLDBPath string
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		var objectStore actionsByteStore
		objectFS, objectStore = newObjectByteStoreForTest(t)
		st.ObjectByteStore = objectStore
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, repoName, "", false)
		controller := st.CreateRepo(user, "variant-controller", "", false)
		oldRepoID = repo.ID
		now := time.Now().UTC()

		org := st.CreateOrg(user, "delete-cascade-org", "Delete Cascade Org", "")
		team := st.CreateTeam(org.Login, "Reviewers", TeamOptions{Permission: TeamPermissionPull})
		if team == nil {
			t.Fatal("CreateTeam returned nil")
		}
		team.RepoNames = append(team.RepoNames, repoKey)
		team.RepoPermissions = map[string]TeamPermission{repoKey: TeamPermissionPush}
		p.MustPut("teams", strconv.Itoa(team.ID), team)
		st.CreateArtifactStorageRecord(&ArtifactStorageRecord{OrgID: org.ID, Name: "build", Digest: "sha256:" + strings.Repeat("a", 64), Status: "active", GitHubRepository: repoKey})
		st.UpsertArtifactDeploymentRecord(&ArtifactDeploymentRecord{OrgID: org.ID, Name: "deploy", Digest: "sha256:" + strings.Repeat("b", 64), Status: "deployed", LogicalEnvironment: "prod", PhysicalEnvironment: "us", Cluster: "cluster", DeploymentName: "web", GitHubRepository: repoKey})
		importPercent := 100
		st.PutRepoImport(&RepoImport{RepoID: repo.ID, VCS: "git", VCSURL: "https://example.invalid/repo.git", Status: "complete", ImportPercent: &importPercent, CreatedAt: now})
		st.AddDependencySnapshot(&DependencySnapshot{
			RepoID:   repo.ID,
			Version:  1,
			Ref:      "refs/heads/main",
			Sha:      strings.Repeat("1", 40),
			Job:      SnapshotJob{ID: "job", Correlator: "job"},
			Detector: SnapshotDetector{Name: "detector", Version: "1", URL: "https://example.invalid/detector"},
			Scanned:  now.Format(time.RFC3339),
			Result:   "SUCCESS",
		})
		st.AddSBOMExport(repo.ID)
		st.SetEnterpriseDependabotRepoAccess([]int{repo.ID})

		hook := st.CreateHook(repoKey, "http://sink.localhost/h", "sec", "json", "0", []string{"push"}, true)
		st.AddDelivery(&WebhookDelivery{HookID: hook.ID, Event: "push", DeliveredAt: time.Now().UTC()})
		st.RepoSecrets[repoKey] = map[string]*Secret{"TOKEN": {Name: "TOKEN", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("repo_secrets", repoKey, st.RepoSecrets[repoKey])
		st.RepoVariables[repoKey] = map[string]*ActionsVariable{"VAR": {Name: "VAR", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("repo_variables", repoKey, st.RepoVariables[repoKey])
		st.SetCodeScanningDefaultSetup(&CodeScanningDefaultSetup{RepoKey: repoKey, State: "configured", QuerySuite: "default", Languages: []string{"go"}})
		st.SetCodeQualitySetup(&CodeQualitySetup{RepoFullName: repoKey, State: "configured", Languages: []string{"go"}, UpdatedAt: &now})
		st.SetRepoCustomPropertyValues(repoKey, []customPropertyValuePayload{{PropertyName: "team", Value: "platform"}})
		st.SetRepoImmutableReleases(repoKey, true)
		st.AgentsRepoSecrets[repoKey] = map[string]*Secret{"AGENT": {Name: "AGENT", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("agents_repo_secrets", repoKey, st.AgentsRepoSecrets[repoKey])
		st.AgentsRepoVariables[repoKey] = map[string]*ActionsVariable{"AGENT_VAR": {Name: "AGENT_VAR", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("agents_repo_variables", repoKey, st.AgentsRepoVariables[repoKey])
		st.CreateAgentTask(repo, user, "fix stale repository state", "claude-sonnet-4.6", false, "", "")
		db, err := st.UpsertCodeQLDatabase(repoKey, "go", "db.zip", "application/zip", "reload-sha", []byte("db"), user.ID)
		if err != nil {
			t.Fatalf("UpsertCodeQLDatabase: %v", err)
		}
		codeQLDBPath = db.StoragePath
		if got := string(readS3TestFile(t, objectFS, db.StoragePath)); got != "db" {
			t.Fatalf("CodeQL database object bytes = %q, want db", got)
		}
		va, err := st.CreateCodeQLVariantAnalysis(controller.FullName, user.ID, "go", []byte("pack"), []string{repoKey})
		if err != nil {
			t.Fatalf("CreateCodeQLVariantAnalysis: %v", err)
		}
		if got := string(readS3TestFile(t, objectFS, va.StoragePath)); got != "pack" {
			t.Fatalf("CodeQL variant-analysis query-pack object bytes = %q, want pack", got)
		}
		if _, created := st.CreatePackage("Repository", repoKey, "container", "image", "private"); !created {
			t.Fatal("CreatePackage did not create")
		}
		codespaceWorkspace = t.TempDir()
		if err := os.WriteFile(codespaceWorkspace+"/workspace.txt", []byte("workspace"), 0o644); err != nil {
			t.Fatalf("seed codespace workspace: %v", err)
		}
		codespace := &Codespace{
			ID:             st.NextCodespaceID,
			Name:           "delete-cascade-codespace",
			OwnerLogin:     user.Login,
			RepoKey:        repoKey,
			GitRef:         repo.DefaultBranch,
			State:          "Shutdown",
			WorkspaceMount: codespaceWorkspace,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastUsedAt:     now,
		}
		st.NextCodespaceID++
		st.Codespaces[codespace.ID] = codespace
		st.CodespacesByName[codespace.Name] = codespace
		p.MustPut("codespaces", strconv.Itoa(codespace.ID), codespace)
		st.SetCheckSuitePreferences(repoKey, []*CheckSuitePref{{AppID: 1, Setting: true}})
		st.Misc.branchProtection[bpKey(repo.ID, "main")] = &BranchProtection{}
		p.MustPut("branch_protection", bpKey(repo.ID, "main"), st.Misc.branchProtection[bpKey(repo.ID, "main")])
		st.Misc.pagesBuilds[repoKey] = []*PagesBuild{{ID: 1, Status: "built"}}
		p.MustPut("pages_builds", repoKey, st.Misc.pagesBuilds[repoKey])
		dep := st.Deployments.CreateDeployment(repo.ID, user.ID, "main", "abc123", "deploy", "production", "", nil, true, false)
		st.Deployments.AddStatus(dep.ID, user.ID, "success", "", "", "", "", "production", false)
		env := st.Deployments.UpsertEnvironment(repo.ID, "production")
		st.Deployments.SetEnvironmentBranchPolicyConfig(repo.ID, "production", &DeploymentBranchPolicy{CustomBranchPolicies: true})
		st.CreateEnvBranchPolicy(env.ID, "main", "branch")
		st.CreateEnvProtectionRule(env.ID, 1)
		st.CreatePagesDeployment(repo.ID, "github-pages", "pages-build", "succeed", int64(len("pages artifact")), "sha256:"+strings.Repeat("b", 64), "pages/sites/test/artifact")
		label := st.CreateLabel(repo.ID, "stale-label", "stale", "ededed")
		if label == nil {
			t.Fatal("CreateLabel returned nil")
		}
		if st.CreateMilestone(repo.ID, user.ID, "stale milestone", "", "open", nil) == nil {
			t.Fatal("CreateMilestone returned nil")
		}
		st.CreateIssue(repo.ID, user.ID, "stale", "", nil, nil, 0)
		seedStorePullRequestBranches(t, st, repo, "f")
		st.CreatePullRequest(repo.ID, user.ID, "stale pr", "", "f", "main", false, nil, nil, 0)
		release := st.Releases.Create(repo.ID, user.ID, "v0.0.1", "main", "", "", false, false)
		if release == nil {
			t.Fatal("Create release returned nil")
		}
		if _, _, err := st.Reactions.AddReaction("release", release.ID, user.ID, "+1"); err != nil {
			t.Fatalf("AddReaction release: %v", err)
		}
		commitComment := st.CommitComments.Create(repo.ID, "abc123", user.ID, "stale commit comment", "README.md", nil, nil)
		if commitComment == nil {
			t.Fatal("Create commit comment returned nil")
		}
		if _, _, err := st.Reactions.AddReaction("commit_comment", commitComment.ID, user.ID, "eyes"); err != nil {
			t.Fatalf("AddReaction commit comment: %v", err)
		}

		if deleted, err := st.DeleteRepo("admin", repoName); err != nil {
			t.Fatalf("DeleteRepo: %v", err)
		} else if !deleted {
			t.Fatal("DeleteRepo failed")
		}
		if _, err := objectFS.Open(codeQLDBPath); err == nil {
			t.Fatalf("CodeQL database object %s survived repository deletion", codeQLDBPath)
		}
	})

	if st2.GetRepo("admin", repoName) != nil {
		t.Fatal("deleted repo resurrected after reload")
	}
	if len(st2.Hooks[repoKey]) != 0 {
		t.Error("hooks survived repo deletion")
	}
	if len(st2.HookDeliveries) != 0 {
		t.Error("hook deliveries survived repo deletion")
	}
	if len(st2.RepoSecrets[repoKey]) != 0 {
		t.Error("repo secrets survived repo deletion")
	}
	assertNoRepoKeyResidue(t, st2, repoKey)
	if len(st2.CheckSuitePrefs[repoKey]) != 0 {
		t.Error("check suite prefs survived repo deletion")
	}
	if _, ok := st2.Misc.branchProtection[bpKey(oldRepoID, "main")]; ok {
		t.Error("branch protection survived repo deletion")
	}
	if len(st2.Misc.pagesBuilds[repoKey]) != 0 {
		t.Error("pages builds survived repo deletion")
	}
	if len(st2.Deployments.ListDeployments(oldRepoID)) != 0 {
		t.Error("deployments survived repo deletion")
	}
	if len(st2.Deployments.statuses) != 0 {
		t.Error("deployment statuses survived repo deletion")
	}
	if len(st2.Deployments.ListEnvironments(oldRepoID)) != 0 {
		t.Error("environments survived repo deletion")
	}
	if len(st2.EnvBranchPolicies) != 0 {
		t.Error("environment branch policies survived repo deletion")
	}
	if len(st2.EnvProtectionRules) != 0 {
		t.Error("environment protection rules survived repo deletion")
	}
	if len(st2.PagesDeployments[oldRepoID]) != 0 {
		t.Error("Pages deployments survived repo deletion")
	}
	if got := st2.ListCodespacesByRepo(repoKey); len(got) != 0 {
		t.Fatalf("codespaces survived repo deletion: %#v", got)
	}
	if _, err := os.Stat(codespaceWorkspace); !os.IsNotExist(err) {
		t.Fatalf("codespace workspace survived repo deletion: %v", err)
	}
	for _, task := range st2.AgentTasks {
		if task.RepoID == oldRepoID {
			t.Fatalf("agent task survived repo deletion: %#v", task)
		}
	}
	if va := st2.GetCodeQLVariantAnalysis(controllerKey, 1); va == nil {
		t.Fatal("surviving CodeQL variant analysis controller was deleted")
	} else {
		for _, task := range va.ScannedRepositories {
			if task.RepoID == oldRepoID || task.FullName == repoKey {
				t.Fatalf("CodeQL variant analysis target survived repo deletion: %#v", va.ScannedRepositories)
			}
		}
		if slices.Contains(va.NoCodeQLDBRepos, oldRepoID) {
			t.Fatalf("CodeQL variant analysis missing-database target survived repo deletion: %#v", va.NoCodeQLDBRepos)
		}
	}
	if st2.GetRepoImport(oldRepoID) != nil {
		t.Error("repository import survived repo deletion")
	}
	if len(st2.ListDependencySnapshots(oldRepoID)) != 0 {
		t.Error("dependency snapshots survived repo deletion")
	}
	for _, exp := range st2.SBOMExports {
		if exp.RepoID == oldRepoID {
			t.Error("SBOM export survived repo deletion")
		}
	}
	if slices.Contains(st2.EnterpriseSettings.DependabotAccessibleRepoIDs, oldRepoID) {
		t.Error("enterprise Dependabot repository access survived repo deletion")
	}
	for _, team := range st2.Teams {
		if slices.Contains(team.RepoNames, repoKey) {
			t.Fatal("team repository grant survived repo deletion")
		}
		if _, ok := team.RepoPermissions[repoKey]; ok {
			t.Fatal("team repository permission override survived repo deletion")
		}
	}
	for _, rec := range st2.ArtifactStorageRecords {
		if rec.GitHubRepository == repoKey {
			t.Fatal("artifact storage metadata survived repo deletion")
		}
	}
	for _, rec := range st2.ArtifactDeploymentRecords {
		if rec.GitHubRepository == repoKey {
			t.Fatal("artifact deployment metadata survived repo deletion")
		}
	}
	for _, i := range st2.Issues {
		if i.RepoID == oldRepoID {
			t.Error("issue survived repo deletion")
		}
	}
	for _, pr := range st2.PullRequests {
		if pr.RepoID == oldRepoID {
			t.Error("pull request survived repo deletion")
		}
	}
	if len(st2.ListLabels(oldRepoID)) != 0 {
		t.Error("labels survived repo deletion")
	}
	if len(st2.ListMilestones(oldRepoID, "all")) != 0 {
		t.Error("milestones survived repo deletion")
	}
	if len(st2.Releases.List(oldRepoID)) != 0 {
		t.Error("releases survived repo deletion")
	}
	if got := reactionStoreCount(st2); got != 0 {
		t.Fatalf("release or commit comment reactions survived repo deletion: %d", got)
	}

	// A recreated same-name repo starts clean.
	user := st2.UsersByLogin["admin"]
	fresh := st2.CreateRepo(user, repoName, "", false)
	if fresh == nil {
		t.Fatal("recreate after delete failed")
	}
	if got := st2.CreateIssue(fresh.ID, user.ID, "fresh", "", nil, nil, 0); got.Number != 1 {
		t.Errorf("fresh repo issue number = %d, want 1", got.Number)
	}
	if len(st2.ListHooks(repoKey)) != 0 {
		t.Error("recreated repo inherited hooks")
	}
}

func TestPersistenceReload_DeleteDeploymentPurgesStatuses(t *testing.T) {
	var depID int
	st2 := reloadedStore(t, func(_ *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "deployment-delete", "", false)
		dep := st.Deployments.CreateDeployment(repo.ID, user.ID, "main", "abc123", "deploy", "production", "", nil, true, false)
		depID = dep.ID
		st.Deployments.AddStatus(dep.ID, user.ID, "queued", "", "", "", "", "production", false)
		st.Deployments.AddStatus(dep.ID, user.ID, "success", "", "", "", "", "production", false)
		if !st.Deployments.DeleteDeployment(dep.ID) {
			t.Fatal("DeleteDeployment failed")
		}
	})

	if got := st2.Deployments.GetDeployment(depID); got != nil {
		t.Fatalf("deployment survived deletion after reload: %+v", got)
	}
	if len(st2.Deployments.statuses) != 0 {
		t.Fatalf("deployment statuses survived deletion after reload: %+v", st2.Deployments.statuses)
	}
}

func TestPersistenceReload_RenameRepoMovesRepoScopedMetadata(t *testing.T) {
	const oldKey = "admin/rename-source"
	const newKey = "admin/rename-target"
	now := time.Now().UTC()

	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		_, objectStore := newObjectByteStoreForTest(t)
		st.ObjectByteStore = objectStore
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "rename-source", "", false)
		if repo == nil {
			t.Fatal("CreateRepo returned nil")
		}
		org := st.CreateOrg(user, "rename-cascade-org", "Rename Cascade Org", "")
		team := st.CreateTeam(org.Login, "Reviewers", TeamOptions{Permission: TeamPermissionPull})
		if team == nil {
			t.Fatal("CreateTeam returned nil")
		}
		team.RepoNames = append(team.RepoNames, oldKey)
		team.RepoPermissions = map[string]TeamPermission{oldKey: TeamPermissionAdmin}
		p.MustPut("teams", strconv.Itoa(team.ID), team)
		st.CreateArtifactStorageRecord(&ArtifactStorageRecord{OrgID: org.ID, Name: "build", Digest: "sha256:" + strings.Repeat("c", 64), Status: "active", GitHubRepository: oldKey})
		st.UpsertArtifactDeploymentRecord(&ArtifactDeploymentRecord{OrgID: org.ID, Name: "deploy", Digest: "sha256:" + strings.Repeat("d", 64), Status: "deployed", LogicalEnvironment: "prod", PhysicalEnvironment: "us", Cluster: "cluster", DeploymentName: "web", GitHubRepository: oldKey})

		st.RepoSecrets[oldKey] = map[string]*Secret{"TOKEN": {Name: "TOKEN", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("repo_secrets", oldKey, st.RepoSecrets[oldKey])
		st.RepoVariables[oldKey] = map[string]*ActionsVariable{"VAR": {Name: "VAR", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("repo_variables", oldKey, st.RepoVariables[oldKey])
		envKey := envScopeKey(oldKey, "prod")
		st.EnvSecrets[envKey] = map[string]*Secret{"ENV_TOKEN": {Name: "ENV_TOKEN", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("env_secrets", envKey, st.EnvSecrets[envKey])
		st.EnvVariables[envKey] = map[string]*ActionsVariable{"ENV_VAR": {Name: "ENV_VAR", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("env_variables", envKey, st.EnvVariables[envKey])

		st.SetCheckSuitePreferences(oldKey, []*CheckSuitePref{{AppID: 1, Setting: true}})
		suite := st.CreateCheckSuite(oldKey, "main", "0123456789abcdef", 1)
		st.CreateCheckRun(oldKey, "0123456789abcdef", "build", 1, suite.ID)
		st.CommitStatuses.Create(oldKey, "0123456789abcdef", user.ID, "success", "", "ok", "ci")
		st.CommitComments.Create(repo.ID, "0123456789abcdef", user.ID, "body", "", nil, nil)
		st.RegisterWorkflowFile(oldKey, ".github/workflows/ci.yml", "ci", "name: ci\non: push\njobs: {}", "submitted")
		st.MarkNotificationsRead(user.ID, now, oldKey)

		st.SetCodeScanningDefaultSetup(&CodeScanningDefaultSetup{RepoKey: oldKey, State: "configured", QuerySuite: "default", Languages: []string{"go"}})
		alert := st.CreateCodeScanningAlert(oldKey, "rule", "error", "desc", "CodeQL", "open", []CodeScanningAlertInstance{{Path: "main.go", StartLine: 1}})
		st.CreateCodeScanningAutofix(alert)
		upload := &SARIFUpload{ID: "sarif-rename", RepoKey: oldKey, Status: "complete", CreatedAt: now}
		st.SARIFUploads[upload.ID] = upload
		p.MustPut("sarif_uploads", upload.ID, upload)

		st.SetCodeQualitySetup(&CodeQualitySetup{RepoFullName: oldKey, State: "configured", Languages: []string{"go"}, UpdatedAt: &now})
		st.SetRepoCustomPropertyValues(oldKey, []customPropertyValuePayload{{PropertyName: "team", Value: "platform"}})
		st.SetRepoImmutableReleases(oldKey, true)
		st.AgentsRepoSecrets[oldKey] = map[string]*Secret{"AGENT": {Name: "AGENT", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("agents_repo_secrets", oldKey, st.AgentsRepoSecrets[oldKey])
		st.AgentsRepoVariables[oldKey] = map[string]*ActionsVariable{"AGENT_VAR": {Name: "AGENT_VAR", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("agents_repo_variables", oldKey, st.AgentsRepoVariables[oldKey])
		st.CreateSecretScanningPushProtectionPlaceholder(oldKey, "token")
		bypass := &SecretScanningPushProtectionBypass{PlaceholderID: "ph", RepoKey: oldKey, Reason: "false_positive", TokenType: "token", CreatedAt: now, ExpireAt: now.Add(time.Hour)}
		st.SecretScanningPushBypasses[oldKey] = []*SecretScanningPushProtectionBypass{bypass}
		p.MustPut("secret_scanning_push_bypasses", oldKey, st.SecretScanningPushBypasses[oldKey])

		if _, err := st.UpsertCodeQLDatabase(oldKey, "go", "db.zip", "application/zip", "0123456789abcdef", []byte("db"), user.ID); err != nil {
			t.Fatalf("UpsertCodeQLDatabase: %v", err)
		}
		if _, err := st.CreateCodeQLVariantAnalysis(oldKey, user.ID, "go", []byte("pack"), []string{oldKey}); err != nil {
			t.Fatalf("CreateCodeQLVariantAnalysis: %v", err)
		}
		st.CreateRuleset(repo, &Ruleset{Name: "protect"})
		st.CreateProjectClassic(repo, user.ID, "board", "", "open")
		st.Codespaces[1] = &Codespace{ID: 1, Name: "cs", OwnerLogin: user.Login, RepoKey: oldKey, State: "Available", CreatedAt: now, UpdatedAt: now}
		st.CodespacesByName["cs"] = st.Codespaces[1]
		p.MustPut("codespaces", "1", st.Codespaces[1])
		if _, created := st.CreatePackage("Repository", oldKey, "container", "image", "private"); !created {
			t.Fatal("CreatePackage did not create")
		}

		if !st.RenameRepo("admin", "rename-source", "rename-target") {
			t.Fatal("RenameRepo failed")
		}
	})

	if st2.GetRepo("admin", "rename-target") == nil {
		t.Fatal("renamed repo row did not survive reload")
	}
	if st2.GetRepo("admin", "rename-source") != nil {
		t.Fatal("old repo name survived reload")
	}
	assertNoRepoKeyResidue(t, st2, oldKey)
	assertRepoKeyMoved(t, st2, newKey)
}

func TestPersistenceReload_TransferRepoMovesRepoScopedMetadata(t *testing.T) {
	const oldKey = "admin/transfer-source"
	const newKey = "bob/transfer-source"
	now := time.Now().UTC()

	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		addTestUser(p, st, "bob")
		repo := st.CreateRepo(user, "transfer-source", "", false)
		if repo == nil {
			t.Fatal("CreateRepo returned nil")
		}
		st.RepoSecrets[oldKey] = map[string]*Secret{"TOKEN": {Name: "TOKEN", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("repo_secrets", oldKey, st.RepoSecrets[oldKey])
		if !st.TransferRepo("admin", "transfer-source", "bob") {
			t.Fatal("TransferRepo failed")
		}
	})

	if st2.GetRepo("bob", "transfer-source") == nil {
		t.Fatal("transferred repo row did not survive reload")
	}
	if st2.GetRepo("admin", "transfer-source") != nil {
		t.Fatal("old transferred repo owner survived reload")
	}
	if st2.RepoSecrets[newKey]["TOKEN"] == nil {
		t.Fatalf("repo secrets did not move to %s", newKey)
	}
	if len(st2.RepoSecrets[oldKey]) != 0 {
		t.Fatalf("repo secrets residue survived for %s", oldKey)
	}
}

func assertRepoKeyMoved(t *testing.T, st *Store, repoKey string) {
	t.Helper()
	if st.RepoSecrets[repoKey]["TOKEN"] == nil || st.RepoVariables[repoKey]["VAR"] == nil {
		t.Fatalf("actions secrets/variables did not move to %s", repoKey)
	}
	if st.EnvSecrets[envScopeKey(repoKey, "prod")]["ENV_TOKEN"] == nil || st.EnvVariables[envScopeKey(repoKey, "prod")]["ENV_VAR"] == nil {
		t.Fatalf("environment secrets/variables did not move to %s", repoKey)
	}
	if len(st.CheckSuitePrefs[repoKey]) != 1 || len(st.CommitStatuses.List(repoKey, "0123456789abcdef")) != 1 {
		t.Fatalf("check/status state did not move to %s", repoKey)
	}
	if st.GetCodeScanningDefaultSetup(repoKey) == nil || st.GetCodeScanningAutofix(repoKey, 1) == nil || st.GetSARIFUpload(repoKey, "sarif-rename") == nil {
		t.Fatalf("code scanning state did not move to %s", repoKey)
	}
	if st.GetCodeQualitySetup(repoKey).State != "configured" || len(st.RepoCustomPropertyValues[repoKey]) == 0 || !st.RepoImmutableReleases[repoKey] {
		t.Fatalf("repo governance state did not move to %s", repoKey)
	}
	if st.AgentsRepoSecrets[repoKey]["AGENT"] == nil || st.AgentsRepoVariables[repoKey]["AGENT_VAR"] == nil {
		t.Fatalf("agent secrets/variables did not move to %s", repoKey)
	}
	if st.CodeQLDatabasesByRepo[repoKey]["go"] == nil || st.GetCodeQLVariantAnalysis(repoKey, 1) == nil {
		t.Fatalf("CodeQL state did not move to %s", repoKey)
	}
	if len(st.ListPackages(repoKey)) != 1 {
		t.Fatalf("repository-owned package did not move to %s", repoKey)
	}
	foundTeamGrant := false
	for _, team := range st.Teams {
		if slices.Contains(team.RepoNames, repoKey) && team.RepoPermissions[repoKey] == TeamPermissionAdmin {
			foundTeamGrant = true
			break
		}
	}
	if !foundTeamGrant {
		t.Fatalf("team repository grant did not move to %s", repoKey)
	}
	foundArtifactStorage := false
	for _, rec := range st.ArtifactStorageRecords {
		if rec.GitHubRepository == repoKey {
			foundArtifactStorage = true
			break
		}
	}
	if !foundArtifactStorage {
		t.Fatalf("artifact storage metadata did not move to %s", repoKey)
	}
	foundArtifactDeployment := false
	for _, rec := range st.ArtifactDeploymentRecords {
		if rec.GitHubRepository == repoKey {
			foundArtifactDeployment = true
			break
		}
	}
	if !foundArtifactDeployment {
		t.Fatalf("artifact deployment metadata did not move to %s", repoKey)
	}
	for _, state := range st.NotificationsState {
		if at, ok := state.RepoLastReadAt[repoKey]; ok && at.IsZero() {
			t.Fatalf("notification repo read marker moved to %s with zero timestamp", repoKey)
		} else if ok {
			return
		}
	}
	t.Fatalf("notification repo read marker did not move to %s", repoKey)
}

func assertNoRepoKeyResidue(t *testing.T, st *Store, repoKey string) {
	t.Helper()
	if len(st.RepoSecrets[repoKey]) != 0 || len(st.RepoVariables[repoKey]) != 0 || len(st.CheckSuitePrefs[repoKey]) != 0 {
		t.Fatalf("basic repo-key residue survived for %s", repoKey)
	}
	if len(st.EnvSecrets[envScopeKey(repoKey, "prod")]) != 0 || len(st.EnvVariables[envScopeKey(repoKey, "prod")]) != 0 {
		t.Fatalf("environment repo-key residue survived for %s", repoKey)
	}
	if st.GetCodeScanningDefaultSetup(repoKey) != nil || st.GetSARIFUpload(repoKey, "sarif-rename") != nil {
		t.Fatalf("code scanning residue survived for %s", repoKey)
	}
	if setup := st.CodeQualitySetups[repoKey]; setup != nil {
		t.Fatalf("code quality residue survived for %s", repoKey)
	}
	if len(st.RepoCustomPropertyValues[repoKey]) != 0 || st.RepoImmutableReleases[repoKey] {
		t.Fatalf("repo governance residue survived for %s", repoKey)
	}
	if len(st.AgentsRepoSecrets[repoKey]) != 0 || len(st.AgentsRepoVariables[repoKey]) != 0 {
		t.Fatalf("agent residue survived for %s", repoKey)
	}
	if len(st.CodeQLDatabasesByRepo[repoKey]) != 0 || len(st.ListPackages(repoKey)) != 0 {
		t.Fatalf("CodeQL/package residue survived for %s", repoKey)
	}
	for _, team := range st.Teams {
		if slices.Contains(team.RepoNames, repoKey) {
			t.Fatalf("team repository grant residue survived for %s", repoKey)
		}
		if _, ok := team.RepoPermissions[repoKey]; ok {
			t.Fatalf("team repository permission residue survived for %s", repoKey)
		}
	}
	for _, rec := range st.ArtifactStorageRecords {
		if rec.GitHubRepository == repoKey {
			t.Fatalf("artifact storage metadata residue survived for %s", repoKey)
		}
	}
	for _, rec := range st.ArtifactDeploymentRecords {
		if rec.GitHubRepository == repoKey {
			t.Fatalf("artifact deployment metadata residue survived for %s", repoKey)
		}
	}
	for _, state := range st.NotificationsState {
		if _, ok := state.RepoLastReadAt[repoKey]; ok {
			t.Fatalf("notification repo read marker residue survived for %s", repoKey)
		}
	}
}

// H5: ProjectV2 single-select option IDs keep advancing after a reload
// instead of restarting (and colliding) at seed 1.
func TestPersistenceReload_ProjectV2OptionSeed(t *testing.T) {
	var projID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		proj := st.ProjectsV2.CreateProject(user.ID, "User", "Roadmap", user.ID)
		projID = proj.ID
		st.ProjectsV2.CreateField(proj.ID, "Status", ProjectV2FieldSingleSelect,
			[]*ProjectV2SingleSelectOption{{Name: "Todo"}, {Name: "Done"}}, nil)
	})

	f2 := st2.ProjectsV2.CreateField(projID, "Priority", ProjectV2FieldSingleSelect,
		[]*ProjectV2SingleSelectOption{{Name: "High"}}, nil)
	if f2 == nil {
		t.Fatal("CreateField after reload failed")
	}
	seen := map[string]bool{}
	for _, f := range st2.ProjectsV2.fieldsByProj[projID] {
		for _, opt := range f.Options {
			if seen[opt.ID] {
				t.Fatalf("option ID %q collides after reload — nextOptionSeed not restored", opt.ID)
			}
			seen[opt.ID] = true
		}
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct option IDs, got %d", len(seen))
	}
}

// Org-surface reload round-trip: membership pending state + public flag,
// team hierarchy/roles/notification setting, org profile fields, and
// org-level webhooks must all survive a restart.
func TestPersistenceReload_OrgProfileMembershipFlagsAndOrgHooks(t *testing.T) {
	var devID, parentID, childID, hookID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		admin := st.UsersByLogin["admin"]
		dev := addTestUser(p, st, "orgdev")
		devID = dev.ID

		if st.CreateOrg(admin, "persist-org", "Persist", "") == nil {
			t.Fatal("CreateOrg returned nil")
		}
		canCreate := false
		if !st.UpdateOrg("persist-org", func(o *Org) {
			o.Company = "ACME"
			o.BillingEmail = "bill@example.test"
			o.DefaultRepositoryPermission = "write"
			o.MembersCanCreateRepositories = &canCreate
		}) {
			t.Fatal("UpdateOrg failed")
		}

		// Pending membership + a publicized admin membership.
		if st.SetMembership("persist-org", dev.ID, OrgRoleMember, MembershipStatePending) == nil {
			t.Fatal("SetMembership failed")
		}
		if !st.SetMembershipPublic("persist-org", admin.ID, true) {
			t.Fatal("SetMembershipPublic failed")
		}

		parent := st.CreateTeam("persist-org", "Core", TeamOptions{
			Permission:          TeamPermissionPush,
			NotificationSetting: TeamNotificationsDisabled,
		})
		parentID = parent.ID
		child := st.CreateTeam("persist-org", "Core Infra", TeamOptions{ParentID: parent.ID})
		childID = child.ID
		st.SetTeamMembership("persist-org", "core", admin.ID, TeamRoleMaintainer)

		hook := st.CreateOrgHook("persist-org", "https://hooks.example.test/x", "s3cret", "json", "0", []string{"push", "organization"}, true)
		hookID = hook.ID
	})

	org := st2.GetOrg("persist-org")
	if org == nil {
		t.Fatal("org did not persist")
	}
	if org.Company != "ACME" || org.BillingEmail != "bill@example.test" || org.DefaultRepositoryPermission != "write" {
		t.Errorf("org profile fields after reload = %+v", org)
	}
	if org.MembersCanCreateRepositories == nil || *org.MembersCanCreateRepositories {
		t.Error("members_can_create_repositories=false did not persist")
	}

	dev := st2.GetMembership("persist-org", devID)
	if dev == nil || dev.State != MembershipStatePending {
		t.Errorf("pending membership after reload = %+v, want pending", dev)
	}
	adminM := st2.GetMembership("persist-org", st2.UsersByLogin["admin"].ID)
	if adminM == nil || !adminM.Public {
		t.Error("public membership flag did not persist")
	}

	parent := st2.GetTeamByID(parentID)
	if parent == nil || parent.NotificationSetting != TeamNotificationsDisabled {
		t.Errorf("team notification setting after reload = %+v", parent)
	}
	if role, ok := parent.roleOf(st2.UsersByLogin["admin"].ID); !ok || role != TeamRoleMaintainer {
		t.Errorf("maintainer role after reload = %v/%v, want maintainer/true", role, ok)
	}
	child := st2.GetTeamByID(childID)
	if child == nil || child.ParentID != parentID {
		t.Errorf("team parent after reload = %+v, want ParentID=%d", child, parentID)
	}

	hook := st2.GetOrgHook("persist-org", hookID)
	if hook == nil {
		t.Fatal("org hook did not persist")
	}
	if hook.OrgLogin != "persist-org" {
		t.Errorf("org hook OrgLogin not backfilled from bucket key: %q", hook.OrgLogin)
	}
	if hook.Secret != "s3cret" || hook.URL != "https://hooks.example.test/x" {
		t.Errorf("org hook config after reload = %+v", hook)
	}
	if st2.NextHookID <= hookID {
		t.Errorf("NextHookID after reload = %d, want > %d", st2.NextHookID, hookID)
	}
}

// Codespaces and codespace secrets survive a restart; the reloaded
// codespace reports the real Docker container state.
func TestPersistenceReload_CodespacesAndSecrets(t *testing.T) {
	var (
		cs        *Codespace
		userScope string
		orgScope  = codespaceSecretScopeKey("org", "cs-reload-org")
		repoID    int
	)
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		userScope = codespaceSecretScopeKey("user", user.Login)
		repo := st.CreateRepo(user, "cs-reload-repo", "", false)
		if repo == nil {
			t.Fatal("CreateRepo returned nil")
		}
		repoID = repo.ID
		stor := st.GitStorages[repo.FullName]
		if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "init", map[string]string{
			".devcontainer/devcontainer.json": `{"image":"` + codespaceTestImage + `"}`,
		}, repoSignature("admin", "admin@bleephub.local")); err != nil {
			t.Fatalf("init repo files: %v", err)
		}
		var err error
		cs, err = st.CreateCodespace(user.Login, repo.FullName, "", "basicLinux32", "Reload Codespace")
		if err != nil {
			t.Fatalf("CreateCodespace: %v", err)
		}
		st.CreateCodespaceSecret(userScope, "RELOAD_TOKEN", "v1", "", nil)
		st.CreateCodespaceSecret(orgScope, "ORG_TOKEN", "v2", "selected", []int{repo.ID})
	})
	t.Cleanup(func() { cleanupCodespaceContainer(t, cs.Name) })

	got := st2.GetCodespace(cs.ID)
	if got == nil {
		t.Fatal("codespace did not persist")
	}
	if got.Name != cs.Name || got.OwnerLogin != "admin" || got.RepoKey != "admin/cs-reload-repo" {
		t.Errorf("codespace identity did not round-trip: %+v", got)
	}
	if got.MachineName != "basicLinux32" || got.DisplayName != "Reload Codespace" || got.ContainerID != cs.ContainerID {
		t.Errorf("codespace metadata did not round-trip: %+v", got)
	}
	if st2.GetCodespaceByName(cs.Name) != got {
		t.Error("CodespacesByName index not rebuilt on reload")
	}
	if st2.NextCodespaceID <= cs.ID {
		t.Errorf("NextCodespaceID after reload = %d, want > %d (counter must not restart)", st2.NextCodespaceID, cs.ID)
	}

	// The backing container survived the restart, so the codespace
	// reports its real Docker state.
	if state := st2.RefreshCodespaceState(cs.ID); state != "Available" {
		t.Errorf("reloaded codespace state = %q, want Available (container still running)", state)
	}
	// Once the container is gone the codespace honestly reports the
	// container-lost state instead of the stale persisted one.
	ctx, cancel := contextWithTimeout(30 * time.Second)
	err := dockerRemoveContainer(ctx, got.ContainerID)
	cancel()
	if err != nil {
		t.Fatalf("remove container: %v", err)
	}
	if state := st2.RefreshCodespaceState(cs.ID); state != "Unavailable" {
		t.Errorf("state after container removal = %q, want Unavailable", state)
	}

	if st2.GetCodespaceSecret(userScope, "RELOAD_TOKEN") == nil {
		t.Fatal("user codespace secret did not persist")
	}
	orgSec := st2.GetCodespaceSecret(orgScope, "ORG_TOKEN")
	if orgSec == nil {
		t.Fatal("org codespace secret did not persist")
	}
	if orgSec.Visibility != "selected" || len(orgSec.SelectedRepoIDs) != 1 || orgSec.SelectedRepoIDs[0] != repoID {
		t.Errorf("org secret visibility/selected repos did not round-trip: %+v", orgSec)
	}
}

// Code scanning default setup configuration survives a restart.
func TestPersistenceReload_CodeScanningDefaultSetup(t *testing.T) {
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "csds-reload", "", false)
		if repo == nil {
			t.Fatal("CreateRepo returned nil")
		}
		st.SetCodeScanningDefaultSetup(&CodeScanningDefaultSetup{
			RepoKey:    repo.FullName,
			State:      "configured",
			QuerySuite: "extended",
			Languages:  []string{"go"},
		})
	})
	got := st2.GetCodeScanningDefaultSetup("admin/csds-reload")
	if got == nil {
		t.Fatal("code scanning default setup did not persist")
	}
	if got.State != "configured" || got.QuerySuite != "extended" || len(got.Languages) != 1 || got.Languages[0] != "go" {
		t.Errorf("default setup did not round-trip: %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("default setup UpdatedAt did not persist")
	}
}
