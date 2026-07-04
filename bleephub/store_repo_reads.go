package bleephub

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// RepoActivity is one recorded ref update served by the repository activity
// and events APIs. Records are written on every git receive-pack ref command,
// so the surfaces expose real pushes — empty when nothing was ever pushed.
type RepoActivity struct {
	ID           int       `json:"id"`
	RepoID       int       `json:"repo_id"`
	Ref          string    `json:"ref"`    // full reference name (refs/heads/…)
	Before       string    `json:"before"` // SHA before the update (all-zero for creations)
	After        string    `json:"after"`  // SHA after the update (all-zero for deletions)
	ActorID      int       `json:"actor_id"`
	ActivityType string    `json:"activity_type"` // push, force_push, branch_creation, branch_deletion
	Timestamp    time.Time `json:"timestamp"`
}

// RepoTrafficBucket accumulates one repository's clone traffic for one UTC
// day. Actors holds the distinct cloner identities (login, or remote host for
// anonymous clones) so unique counts are counted, never estimated.
type RepoTrafficBucket struct {
	RepoID int             `json:"repo_id"`
	Day    string          `json:"day"` // YYYY-MM-DD, UTC
	Count  int             `json:"count"`
	Actors map[string]bool `json:"actors"`
}

func repoTrafficKey(repoID int, day string) string {
	return strconv.Itoa(repoID) + ":" + day
}

// RecordRepoActivity appends a ref-update record for a repository.
func (st *Store) RecordRepoActivity(repoID int, ref, before, after string, actorID int, activityType string) *RepoActivity {
	st.mu.Lock()
	defer st.mu.Unlock()
	a := &RepoActivity{
		ID:           st.NextRepoActivity,
		RepoID:       repoID,
		Ref:          ref,
		Before:       before,
		After:        after,
		ActorID:      actorID,
		ActivityType: activityType,
		Timestamp:    time.Now().UTC(),
	}
	st.NextRepoActivity++
	st.RepoActivities[a.ID] = a
	if st.persist != nil {
		st.persist.MustPut("repo_activity", strconv.Itoa(a.ID), a)
	}
	return a
}

// ListRepoActivity returns a repository's recorded ref updates, oldest first.
func (st *Store) ListRepoActivity(repoID int) []*RepoActivity {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*RepoActivity{}
	for _, a := range st.RepoActivities {
		if a.RepoID == repoID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// RecordRepoClone counts one clone of a repository by the given actor in
// today's UTC day bucket.
func (st *Store) RecordRepoClone(repoID int, actor string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	day := time.Now().UTC().Format("2006-01-02")
	key := repoTrafficKey(repoID, day)
	b := st.RepoCloneTraffic[key]
	if b == nil {
		b = &RepoTrafficBucket{RepoID: repoID, Day: day, Actors: map[string]bool{}}
		st.RepoCloneTraffic[key] = b
	}
	b.Count++
	b.Actors[actor] = true
	if st.persist != nil {
		st.persist.MustPut("repo_traffic_clones", key, b)
	}
}

// ListRepoCloneTraffic returns a repository's clone buckets on or after the
// given day, oldest first.
func (st *Store) ListRepoCloneTraffic(repoID int, since time.Time) []*RepoTrafficBucket {
	st.mu.RLock()
	defer st.mu.RUnlock()
	sinceDay := since.UTC().Format("2006-01-02")
	out := []*RepoTrafficBucket{}
	for _, b := range st.RepoCloneTraffic {
		if b.RepoID == repoID && b.Day >= sinceDay {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out
}

// ListRepoSubscribers returns the users holding a watch subscription on the
// repository, ordered by user ID.
func (st *Store) ListRepoSubscribers(repoID int) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*User{}
	for _, sub := range st.RepoSubscriptions {
		if sub == nil || sub.RepoID != repoID || !sub.Subscribed {
			continue
		}
		if u := st.Users[sub.UserID]; u != nil {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListPublicRepos returns public repositories with an ID greater than since,
// ordered by ID ascending — the contract of GET /repositories.
func (st *Store) ListPublicRepos(since int) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*Repo{}
	for _, repo := range st.Repos {
		if repo.Private || repo.ID <= since {
			continue
		}
		out = append(out, repo)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListTeamsForRepo returns the org teams granted access to the repository,
// ordered by team ID.
func (st *Store) ListTeamsForRepo(fullName string) []*Team {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := []*Team{}
	for _, team := range st.TeamsBySlug {
		for _, rn := range team.RepoNames {
			if rn == fullName {
				out = append(out, team)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LookupUserByEmail returns the user whose email matches case-insensitively,
// or nil. Git commit signatures carry emails, not logins; this is how commit
// authors resolve to real accounts.
func (st *Store) LookupUserByEmail(email string) *User {
	if email == "" {
		return nil
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, u := range st.Users {
		if strings.EqualFold(u.Email, email) {
			return u
		}
	}
	return nil
}

// ResolveUserBySignature maps a git commit signature to a real account: by
// email first (GitHub's rule), then by the signature name matching a login or
// a profile display name (the two names bleephub's own generated commits
// carry). Returns nil when no account matches.
func (st *Store) ResolveUserBySignature(name, email string) *User {
	if u := st.LookupUserByEmail(email); u != nil {
		return u
	}
	if name == "" {
		return nil
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, u := range st.Users {
		if strings.EqualFold(u.Login, name) {
			return u
		}
	}
	for _, u := range st.Users {
		if u.Name != "" && strings.EqualFold(u.Name, name) {
			return u
		}
	}
	return nil
}

// ListAssignableUsers returns the users GitHub considers assignable for a
// repository's issues: the owning user, direct collaborators, and — for
// org-owned repositories — active organization members. Ordered by login.
func (st *Store) ListAssignableUsers(repo *Repo) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	seen := map[int]bool{}
	out := []*User{}
	add := func(u *User) {
		if u != nil && !seen[u.ID] {
			seen[u.ID] = true
			out = append(out, u)
		}
	}
	add(repo.Owner)
	owner, name, ok := splitRepoFullName(repo.FullName)
	if ok {
		for login := range st.RepoCollaborators[owner+"/"+name] {
			add(st.UsersByLogin[login])
		}
		if org := st.OrgsByLogin[owner]; org != nil {
			for _, m := range st.Memberships {
				if m.OrgID == org.ID && m.State == MembershipStateActive {
					add(st.Users[m.UserID])
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Login < out[j].Login })
	return out
}
