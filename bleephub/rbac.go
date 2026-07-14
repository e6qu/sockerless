package bleephub

import "strings"

// canAdminOrg checks if a user is an active admin of the given organization.
// A pending (invited) admin has not accepted yet and holds no rights.
func canAdminOrg(st *Store, user *User, org *Org) bool {
	m := st.GetMembership(org.Login, user.ID)
	return m != nil && m.Role == OrgRoleAdmin && m.State == MembershipStateActive
}

// isActiveOrgMember reports whether user holds an active membership in the
// org. Org teams (their names, members, and repo grants) are visible only to
// org members on real GitHub — a non-member authenticated caller gets 404, the
// same as an unknown org, so the org's internal structure never leaks.
func isActiveOrgMember(st *Store, user *User, orgLogin string) bool {
	if user == nil {
		return false
	}
	m := st.GetMembership(orgLogin, user.ID)
	return m != nil && m.State == MembershipStateActive
}

// canReadRepo checks if a user can read a repository.
// Public repos are readable by all. Private repos require ownership, org
// membership, team access, or collaborator pull access.
// Must not be called with st.mu held; it takes the read lock itself.
func canReadRepo(st *Store, user *User, repo *Repo) bool {
	if !repo.Private {
		return true
	}
	if user != nil && user.SiteAdmin {
		return true
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	return canPullRepoLocked(st, user, repo)
}

// canReadRepoLocked is canReadRepo for callers that already hold st.mu
// (read or write); it never acquires the lock itself.
func canReadRepoLocked(st *Store, user *User, repo *Repo) bool {
	if !repo.Private {
		return true
	}
	return canPullRepoLocked(st, user, repo)
}

// canAdminRepo checks if a user has admin rights to a repository.
func canAdminRepo(st *Store, user *User, repo *Repo) bool {
	if user == nil {
		return false
	}
	if user.SiteAdmin {
		return true
	}
	// Owner can always admin
	if repo.Owner != nil && repo.Owner.ID == user.ID {
		return true
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return false
	}
	orgLogin := parts[0]
	org := st.GetOrg(orgLogin)
	if org != nil && canAdminOrg(st, user, org) {
		return true
	}
	return repoCollaboratorPermissionAtLeast(st, repo.FullName, user.Login, "admin")
}

// canPushRepo checks if a user can push to a repository.
// Push requires ownership, org membership, team push permission, or
// collaborator push/admin access.
func canPushRepo(st *Store, user *User, repo *Repo) bool {
	if user == nil {
		return false
	}
	if user.SiteAdmin {
		return true
	}
	if repo.Owner != nil && repo.Owner.ID == user.ID {
		return true
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return false
	}
	orgLogin := parts[0]
	org := st.GetOrg(orgLogin)
	if org != nil {
		m := st.GetMembership(orgLogin, user.ID)
		if m != nil && m.State == "active" {
			return true
		}
		if hasTeamAccess(st, orgLogin, user.ID, repo.FullName, "push") {
			return true
		}
	}
	return repoCollaboratorPermissionAtLeast(st, repo.FullName, user.Login, "push")
}

// canPullRepoLocked checks if a user can pull (read) from a repository.
// Callers already hold st.mu (read or write); it never acquires the lock
// itself.
func canPullRepoLocked(st *Store, user *User, repo *Repo) bool {
	if !repo.Private {
		return true
	}
	if user == nil {
		return false
	}
	if repo.Owner != nil && repo.Owner.ID == user.ID {
		return true
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return false
	}
	orgLogin := parts[0]
	org := st.OrgsByLogin[orgLogin]
	if org != nil {
		m := st.Memberships[membershipKey(orgLogin, user.ID)]
		if m != nil && m.State == "active" {
			return true
		}
		if hasTeamAccessLocked(st, orgLogin, user.ID, repo.FullName, "pull") {
			return true
		}
	}
	return repoCollaboratorPermissionAtLeastLocked(st, repo.FullName, user.Login, "pull")
}

// repoCollaboratorPermissionAtLeast reports whether login has at least the
// requested permission level on the repo via direct collaboration.
// Must not be called with st.mu held; it takes the read lock itself.
func repoCollaboratorPermissionAtLeast(st *Store, repoFullName, login, minPerm string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return repoCollaboratorPermissionAtLeastLocked(st, repoFullName, login, minPerm)
}

// repoCollaboratorPermissionAtLeastLocked is repoCollaboratorPermissionAtLeast
// for callers that already hold st.mu; it never acquires the lock itself.
func repoCollaboratorPermissionAtLeastLocked(st *Store, repoFullName, login, minPerm string) bool {
	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 {
		return false
	}
	perm := ""
	if collabs := st.RepoCollaborators[repoFullName]; collabs != nil {
		perm = collabs[login]
	}
	if perm == "" {
		return false
	}
	levels := map[string]int{"pull": 1, "push": 2, "admin": 3}
	return levels[perm] >= levels[minPerm]
}

// hasTeamAccess checks if a user has at least the given permission level
// on a repo via team membership.
// Must not be called with st.mu held; it takes the read lock itself.
func hasTeamAccess(st *Store, orgLogin string, userID int, repoFullName string, minPermission TeamPermission) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return hasTeamAccessLocked(st, orgLogin, userID, repoFullName, minPermission)
}

// hasTeamAccessLocked is hasTeamAccess for callers that already hold st.mu;
// it never acquires the lock itself.
func hasTeamAccessLocked(st *Store, orgLogin string, userID int, repoFullName string, minPermission TeamPermission) bool {
	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return false
	}

	for _, team := range st.TeamsBySlug {
		if team.OrgID != org.ID {
			continue
		}
		if !permissionAtLeast(team.Permission, minPermission) {
			continue
		}
		// Check if repo is in team's repo list
		repoFound := false
		for _, rn := range team.RepoNames {
			if rn == repoFullName {
				repoFound = true
				break
			}
		}
		if !repoFound {
			continue
		}
		// Check if user is a team member
		for _, mid := range team.MemberIDs {
			if mid == userID {
				return true
			}
		}
	}
	return false
}

// permissionAtLeast returns true if perm is at least minPerm.
// Permission hierarchy: pull < push < admin.
func permissionAtLeast(perm, minPerm TeamPermission) bool {
	levels := map[TeamPermission]int{TeamPermissionPull: 1, TeamPermissionPush: 2, TeamPermissionAdmin: 3}
	return levels[perm] >= levels[minPerm]
}
