package bleephub

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// storeCensus is a per-resource-class cardinality + counter snapshot used to
// compare a live store against one reloaded from the same persistence.
type storeCensus struct {
	repos, issues, prs, comments, labels, milestones int
	orgs, teams, memberships, reactions              int
	repoKeys                                         map[string]int // fullName → repo ID
	issueIDs                                         map[int]bool
	orgLogins                                        map[string]bool

	nextRepo, nextIssue, nextLabel, nextMilestone int
	nextComment, nextPR, nextOrg, nextTeam        int
	nextReaction                                  int
}

// snapshotStoreCensus reads the store under its locks and records the
// cardinality of every resource class the persistence storm creates plus the
// monotonic Next* allocators.
func snapshotStoreCensus(st *Store) storeCensus {
	st.mu.RLock()
	c := storeCensus{
		repos:         len(st.Repos),
		issues:        len(st.Issues),
		prs:           len(st.PullRequests),
		comments:      len(st.Comments),
		labels:        len(st.Labels),
		milestones:    len(st.Milestones),
		orgs:          len(st.Orgs),
		teams:         len(st.Teams),
		memberships:   len(st.Memberships),
		repoKeys:      make(map[string]int, len(st.ReposByName)),
		issueIDs:      make(map[int]bool, len(st.Issues)),
		orgLogins:     make(map[string]bool, len(st.Orgs)),
		nextRepo:      st.NextRepo,
		nextIssue:     st.NextIssue,
		nextLabel:     st.NextLabel,
		nextMilestone: st.NextMilestone,
		nextComment:   st.NextComment,
		nextPR:        st.NextPR,
		nextOrg:       st.NextOrg,
		nextTeam:      st.NextTeam,
	}
	for name, r := range st.ReposByName {
		c.repoKeys[name] = r.ID
	}
	for id := range st.Issues {
		c.issueIDs[id] = true
	}
	for login := range st.OrgsByLogin {
		c.orgLogins[login] = true
	}
	st.mu.RUnlock()

	st.Reactions.mu.RLock()
	c.reactions = len(st.Reactions.byID)
	c.nextReaction = st.Reactions.nextID
	st.Reactions.mu.RUnlock()
	return c
}

// assertEqual fails t if any resource-class cardinality, identity set, or
// Next* counter differs between the live and reloaded censuses.
func (live storeCensus) assertEqual(t *testing.T, got storeCensus) {
	t.Helper()
	eq := func(name string, a, b int) {
		if a != b {
			t.Errorf("%s: live=%d reloaded=%d (mismatch)", name, a, b)
		}
	}
	eq("repos", live.repos, got.repos)
	eq("issues", live.issues, got.issues)
	eq("pull_requests", live.prs, got.prs)
	eq("comments", live.comments, got.comments)
	eq("labels", live.labels, got.labels)
	eq("milestones", live.milestones, got.milestones)
	eq("orgs", live.orgs, got.orgs)
	eq("teams", live.teams, got.teams)
	eq("memberships", live.memberships, got.memberships)
	eq("reactions", live.reactions, got.reactions)

	// Counters must never regress across reload (a restart handing out an
	// already-used ID collides with prior state).
	eq("NextRepo", live.nextRepo, got.nextRepo)
	eq("NextIssue", live.nextIssue, got.nextIssue)
	eq("NextLabel", live.nextLabel, got.nextLabel)
	eq("NextMilestone", live.nextMilestone, got.nextMilestone)
	eq("NextComment", live.nextComment, got.nextComment)
	eq("NextPR", live.nextPR, got.nextPR)
	eq("NextOrg", live.nextOrg, got.nextOrg)
	eq("NextTeam", live.nextTeam, got.nextTeam)
	eq("Reactions.nextID", live.nextReaction, got.nextReaction)

	for name, id := range live.repoKeys {
		if gid, ok := got.repoKeys[name]; !ok {
			t.Errorf("repo %q missing after reload", name)
		} else if gid != id {
			t.Errorf("repo %q id live=%d reloaded=%d", name, id, gid)
		}
	}
	for id := range live.issueIDs {
		if !got.issueIDs[id] {
			t.Errorf("issue %d missing after reload", id)
		}
	}
	for login := range live.orgLogins {
		if !got.orgLogins[login] {
			t.Errorf("org %q missing after reload", login)
		}
	}
}

// TestStressPersistenceReloadConsistency runs a concurrent create-storm against
// a persistence-backed store, then reconstructs a SECOND store from the same
// database and asserts the reloaded state matches the live state for every
// resource class exercised. It catches two bug families at once: persist-path
// races (a write-through lost or corrupted under concurrency — every store
// method write-throughs under its own lock while p.mu serializes the DB write)
// and reload-fidelity gaps (a resource or counter that does not survive the
// round-trip). The whole storm runs under `-race`.
func TestStressPersistenceReloadConsistency(t *testing.T) {
	dir := t.TempDir()
	gitDir := t.TempDir()
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", dir)
	t.Setenv("BLEEPHUB_GIT_DIR", gitDir)

	p1, err := NewPersistence()
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	st1 := NewStore()
	if err := st1.SetPersistence(p1); err != nil {
		t.Fatalf("SetPersistence: %v", err)
	}
	st1.SeedDefaultUser()
	admin := st1.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin missing")
	}

	nWorkers := stressWorkers(16)
	perWorker := 6
	if d := stressDuration(0); d > 0 {
		perWorker = int(d/time.Second) + 6
	}

	var errMu sync.Mutex
	var errs []error
	fail := func(err error) {
		errMu.Lock()
		errs = append(errs, err)
		errMu.Unlock()
	}

	var wg sync.WaitGroup
	for w := 0; w < nWorkers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for k := 0; k < perWorker; k++ {
				tag := fmt.Sprintf("w%d-%d", id, k)
				repo := st1.CreateRepo(admin, "prepo-"+tag, "desc "+tag, false)
				if repo == nil {
					fail(fmt.Errorf("CreateRepo %s returned nil", tag))
					continue
				}
				if st1.CreateLabel(repo.ID, "lbl-"+tag, "d", "ededed") == nil {
					fail(fmt.Errorf("CreateLabel %s nil", tag))
				}
				if st1.CreateMilestone(repo.ID, admin.ID, "ms-"+tag, "d", "open", nil) == nil {
					fail(fmt.Errorf("CreateMilestone %s nil", tag))
				}
				for j := 0; j < 3; j++ {
					issue := st1.CreateIssue(repo.ID, admin.ID, fmt.Sprintf("issue %s-%d", tag, j), "body", nil, nil, 0)
					if issue == nil {
						fail(fmt.Errorf("CreateIssue %s-%d nil", tag, j))
						continue
					}
					if st1.CreateComment(issue.ID, admin.ID, "comment") == nil {
						fail(fmt.Errorf("CreateComment on issue %s-%d nil", tag, j))
					}
					if _, _, err := st1.Reactions.AddReaction("issue", issue.ID, admin.ID, "+1"); err != nil {
						fail(fmt.Errorf("AddReaction %s-%d: %w", tag, j, err))
					}
				}
				seedStorePullRequestBranches(t, st1, repo, "feature-"+tag)
				if st1.CreatePullRequest(repo.ID, admin.ID, "pr "+tag, "b", "feature-"+tag, "main", false, nil, nil, 0) == nil {
					fail(fmt.Errorf("CreatePullRequest %s nil", tag))
				}
				org := st1.CreateOrg(admin, "porg-"+tag, "Org "+tag, "")
				if org == nil {
					fail(fmt.Errorf("CreateOrg %s nil", tag))
					continue
				}
				if st1.CreateTeam(org.Login, "team-"+tag, TeamOptions{}) == nil {
					fail(fmt.Errorf("CreateTeam %s nil", tag))
				}
			}
		}(w)
	}
	wg.Wait()

	for _, e := range errs {
		t.Error(e)
	}
	if t.Failed() {
		t.Fatal("create-storm reported failures")
	}

	live := snapshotStoreCensus(st1)
	// Guard against a vacuously-passing comparison: the storm must have
	// actually produced state across every class before we trust the reload.
	if live.repos == 0 || live.issues == 0 || live.comments == 0 ||
		live.reactions == 0 || live.prs == 0 || live.orgs == 0 || live.teams == 0 {
		t.Fatalf("create-storm produced empty census: %+v", live)
	}

	if err := p1.Close(); err != nil {
		t.Fatalf("close p1: %v", err)
	}

	p2, err := NewPersistence()
	if err != nil {
		t.Fatalf("reopen persistence: %v", err)
	}
	st2 := NewStore()
	if err := st2.SetPersistence(p2); err != nil {
		t.Fatalf("reload SetPersistence: %v", err)
	}
	defer p2.Close()

	reloaded := snapshotStoreCensus(st2)
	live.assertEqual(t, reloaded)
}
