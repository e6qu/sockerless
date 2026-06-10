package bleephub

import (
	"testing"
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
	bp := BranchProtection{"required_status_checks": nil}
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
