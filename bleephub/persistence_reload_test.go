package bleephub

import (
	"strconv"
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

// G7: PR review comments re-index under their PR with author, thread and
// resolved state intact.
func TestPersistenceReload_PRReviewComments(t *testing.T) {
	var prID, rootID, replyID, authorID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, "reviewed", "", false)
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
}

// G9: installation selected-repo lists and token repo scoping survive,
// including the add/remove mutations that previously never persisted.
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
		st.AddTeamMember("acme", "platform", admin.ID, TeamRoleMaintainer)
		st.AddTeamMember("acme", "platform", dev.ID, TeamRoleMember)
		st.AddTeamRepo("acme", "platform", "acme/infra")
		st.AddTeamRepo("acme", "platform", "acme/app")
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

// H4: deleting a repo purges everything keyed to it, so a restart plus a
// recreated same-name repo inherits nothing.
func TestPersistenceReload_DeleteRepoLeavesNoResidue(t *testing.T) {
	const repoName = "doomed"
	const repoKey = "admin/" + repoName
	var oldRepoID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		repo := st.CreateRepo(user, repoName, "", false)
		oldRepoID = repo.ID

		hook := st.CreateHook(repoKey, "http://sink.localhost/h", "sec", "json", "0", []string{"push"}, true)
		st.AddDelivery(&WebhookDelivery{HookID: hook.ID, Event: "push", DeliveredAt: time.Now().UTC()})
		now := time.Now().UTC()
		st.RepoSecrets[repoKey] = map[string]*Secret{"TOKEN": {Name: "TOKEN", Value: "v", CreatedAt: now, UpdatedAt: now}}
		p.MustPut("repo_secrets", repoKey, st.RepoSecrets[repoKey])
		st.SetCheckSuitePreferences(repoKey, []*CheckSuitePref{{AppID: 1, Setting: true}})
		st.Misc.branchProtection[bpKey(repo.ID, "main")] = &BranchProtection{}
		p.MustPut("branch_protection", bpKey(repo.ID, "main"), st.Misc.branchProtection[bpKey(repo.ID, "main")])
		st.Misc.pagesBuilds[repoKey] = []*PagesBuild{{ID: 1, Status: "built"}}
		p.MustPut("pages_builds", repoKey, st.Misc.pagesBuilds[repoKey])
		st.CreateIssue(repo.ID, user.ID, "stale", "", nil, nil, 0)
		st.CreatePullRequest(repo.ID, user.ID, "stale pr", "", "f", "main", false, nil, nil, 0)
		st.Releases.Create(repo.ID, user.ID, "v0.0.1", "main", "", "", false, false)

		if !st.DeleteRepo("admin", repoName) {
			t.Fatal("DeleteRepo failed")
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
	if len(st2.CheckSuitePrefs[repoKey]) != 0 {
		t.Error("check suite prefs survived repo deletion")
	}
	if _, ok := st2.Misc.branchProtection[bpKey(oldRepoID, "main")]; ok {
		t.Error("branch protection survived repo deletion")
	}
	if len(st2.Misc.pagesBuilds[repoKey]) != 0 {
		t.Error("pages builds survived repo deletion")
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
	if len(st2.Releases.List(oldRepoID)) != 0 {
		t.Error("releases survived repo deletion")
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

// H5: ProjectV2 single-select option IDs keep advancing after a reload
// instead of restarting (and colliding) at seed 1.
func TestPersistenceReload_ProjectV2OptionSeed(t *testing.T) {
	var projID int
	st2 := reloadedStore(t, func(p *Persistence, st *Store) {
		st.SeedDefaultUser()
		user := st.UsersByLogin["admin"]
		proj := st.ProjectsV2.CreateProject(user.ID, "User", "Roadmap")
		projID = proj.ID
		st.ProjectsV2.CreateField(proj.ID, "Status", ProjectV2FieldSingleSelect, []string{"Todo", "Done"})
	})

	f2 := st2.ProjectsV2.CreateField(projID, "Priority", ProjectV2FieldSingleSelect, []string{"High"})
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
		st.AddTeamMember("persist-org", "core", admin.ID, TeamRoleMaintainer)

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
