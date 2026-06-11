package bleephub

import "strings"

// canAdminOrg checks if a user is an active admin of the given organization.
// A pending (invited) admin has not accepted yet and holds no rights.
func canAdminOrg(st *Store, user *User, org *Org) bool {
	m := st.GetMembership(org.Login, user.ID)
	return m != nil && m.Role == OrgRoleAdmin && m.State == MembershipStateActive
}

// canReadRepo checks if a user can read a repository.
// Public repos are readable by all. Private repos require ownership or org membership.
func canReadRepo(st *Store, user *User, repo *Repo) bool {
	if !repo.Private {
		return true
	}
	if user == nil {
		return false
	}
	// Owner can always read
	if repo.Owner != nil && repo.Owner.ID == user.ID {
		return true
	}
	// Check org membership for org-owned repos
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return false
	}
	orgLogin := parts[0]
	org := st.GetOrg(orgLogin)
	if org == nil {
		return false
	}
	m := st.GetMembership(orgLogin, user.ID)
	if m != nil && m.State == "active" {
		return true
	}
	// Check team access
	return hasTeamAccess(st, orgLogin, user.ID, repo.FullName, "pull")
}

// canAdminRepo checks if a user has admin rights to a repository.
func canAdminRepo(st *Store, user *User, repo *Repo) bool {
	if user == nil {
		return false
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
	if org == nil {
		return false
	}
	return canAdminOrg(st, user, org)
}

// hasTeamAccess checks if a user has at least the given permission level
// on a repo via team membership.
func hasTeamAccess(st *Store, orgLogin string, userID int, repoFullName string, minPermission TeamPermission) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

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

// canPushRepo checks if a user can push to a repository.
// Push requires ownership or org team-level "push" permission.
func canPushRepo(st *Store, user *User, repo *Repo) bool {
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
	org := st.GetOrg(orgLogin)
	if org == nil {
		return false
	}
	m := st.GetMembership(orgLogin, user.ID)
	if m != nil && m.State == "active" {
		return true
	}
	return hasTeamAccess(st, orgLogin, user.ID, repo.FullName, "push")
}

// permissionAtLeast returns true if perm is at least minPerm.
// Permission hierarchy: pull < push < admin.
func permissionAtLeast(perm, minPerm TeamPermission) bool {
	levels := map[TeamPermission]int{TeamPermissionPull: 1, TeamPermissionPush: 2, TeamPermissionAdmin: 3}
	return levels[perm] >= levels[minPerm]
}
