package bleephub

import (
	"fmt"
	"sync"
	"testing"
)

// TestStressCounterIntegrity hammers the store's monotonic ID allocators and
// its idempotent operations from many goroutines at once and asserts the
// invariants that concurrency must never break: every allocated ID is unique
// (no two callers get the same run/repo/issue ID), an idempotent reaction POST
// yields exactly one reaction with a stable ID, and a membership upsert
// converges on a single row. A lost update or a non-atomic read-modify-write
// on a Next* counter shows up here as a duplicate ID.
func TestStressCounterIntegrity(t *testing.T) {
	s := newTestServer()
	st := s.store
	admin := st.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin missing")
	}

	const workers = 32
	const perWorker = 40

	// --- run-ID allocator: every ReserveRunID must be unique ---
	t.Run("ReserveRunID uniqueness", func(t *testing.T) {
		var mu sync.Mutex
		seen := make(map[int]bool, workers*perWorker)
		dups := 0
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				local := make([]int, 0, perWorker)
				for i := 0; i < perWorker; i++ {
					local = append(local, st.ReserveRunID())
				}
				mu.Lock()
				for _, id := range local {
					if seen[id] {
						dups++
					}
					seen[id] = true
				}
				mu.Unlock()
			}()
		}
		wg.Wait()
		if dups != 0 {
			t.Errorf("ReserveRunID handed out %d duplicate IDs across %d allocations", dups, workers*perWorker)
		}
		if len(seen) != workers*perWorker {
			t.Errorf("distinct run IDs = %d, want %d", len(seen), workers*perWorker)
		}
	})

	// --- repo + issue ID allocators: unique object IDs under concurrent create ---
	t.Run("repo and issue ID uniqueness", func(t *testing.T) {
		var mu sync.Mutex
		repoIDs := make(map[int]bool)
		issueIDs := make(map[int]bool)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := 0; i < 4; i++ {
					repo := st.CreateRepo(admin, fmt.Sprintf("cnt-%d-%d", id, i), "", false)
					if repo == nil {
						continue
					}
					var localIssues []int
					for j := 0; j < 3; j++ {
						is := st.CreateIssue(repo.ID, admin.ID, "t", "b", nil, nil, 0)
						if is != nil {
							localIssues = append(localIssues, is.ID)
						}
					}
					mu.Lock()
					if repoIDs[repo.ID] {
						t.Errorf("duplicate repo ID %d", repo.ID)
					}
					repoIDs[repo.ID] = true
					for _, iid := range localIssues {
						if issueIDs[iid] {
							t.Errorf("duplicate issue ID %d", iid)
						}
						issueIDs[iid] = true
					}
					mu.Unlock()
				}
			}(w)
		}
		wg.Wait()
	})

	// --- reaction idempotency: same (user, content) repeatedly → one reaction ---
	t.Run("reaction idempotency", func(t *testing.T) {
		repo := st.CreateRepo(admin, "reaction-idem", "", false)
		if repo == nil {
			t.Fatal("CreateRepo nil")
		}
		issue := st.CreateIssue(repo.ID, admin.ID, "react", "b", nil, nil, 0)
		if issue == nil {
			t.Fatal("CreateIssue nil")
		}
		var mu sync.Mutex
		ids := make(map[int]bool)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					r, _, err := st.Reactions.AddReaction("issue", issue.ID, admin.ID, "+1")
					if err != nil {
						t.Errorf("AddReaction: %v", err)
						return
					}
					mu.Lock()
					ids[r.ID] = true
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if len(ids) != 1 {
			t.Errorf("idempotent reaction produced %d distinct IDs, want 1", len(ids))
		}
		if got := st.Reactions.ListReactions("issue", issue.ID, ""); len(got) != 1 {
			t.Errorf("issue has %d reactions, want exactly 1 (idempotency broken)", len(got))
		}
	})

	// --- membership upsert: concurrent SetMembership converges on one row ---
	t.Run("membership upsert", func(t *testing.T) {
		org := st.CreateOrg(admin, "cnt-org", "Org", "")
		if org == nil {
			t.Fatal("CreateOrg nil")
		}
		member := &User{ID: st.NextUser, Login: "cnt-member", Type: "User"}
		st.mu.Lock()
		st.Users[member.ID] = member
		st.UsersByLogin[member.Login] = member
		st.NextUser++
		st.mu.Unlock()

		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					role := OrgRoleMember
					if (n+i)%2 == 0 {
						role = OrgRoleAdmin
					}
					if st.SetMembership(org.Login, member.ID, role, MembershipStateActive) == nil {
						t.Error("SetMembership returned nil under contention")
						return
					}
				}
			}(w)
		}
		wg.Wait()

		// Exactly one membership row for (org, member) — upsert must not
		// create duplicates under contention.
		st.mu.RLock()
		count := len(st.Memberships)
		m := st.Memberships[membershipKey(org.Login, member.ID)]
		st.mu.RUnlock()
		if m == nil {
			t.Fatal("membership missing after upsert storm")
		}
		// admin (org creator) + the single member row.
		if count != 2 {
			t.Errorf("membership rows = %d, want 2 (creator + one upserted member)", count)
		}
	})
}
