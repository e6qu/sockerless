package bleephub

import (
	"fmt"
	"strings"
	"time"
)

// Org represents a GitHub organization account.
type Org struct {
	ID          int       `json:"id"`
	NodeID      string    `json:"node_id"`
	Login       string    `json:"login"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Email       string    `json:"email"`
	AvatarURL   string    `json:"avatar_url"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Membership represents a user's membership in an organization.
type Membership struct {
	OrgID  int    `json:"org_id"`
	UserID int    `json:"user_id"`
	Role   string `json:"role"`  // "admin", "member"
	State  string `json:"state"` // "active", "pending"
}

// Team represents a team within an organization.
type Team struct {
	ID          int       `json:"id"`
	NodeID      string    `json:"node_id"`
	OrgID       int       `json:"org_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Privacy     string    `json:"privacy"`    // "closed", "secret"
	Permission  string    `json:"permission"` // "pull", "push", "admin"
	MemberIDs   []int     `json:"member_ids"`
	RepoNames   []string  `json:"repo_names"` // "owner/name" entries
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// membershipKey returns the map key for org/user membership lookups.
func membershipKey(orgLogin string, userID int) string {
	return fmt.Sprintf("%s/%d", orgLogin, userID)
}

// teamSlugKey returns the map key for org/team slug lookups.
func teamSlugKey(orgLogin, slug string) string {
	return orgLogin + "/" + slug
}

// slugify converts a team name to a URL-safe slug.
func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// CreateOrg creates an organization and adds the creator as an admin member.
func (st *Store) CreateOrg(creator *User, login, name, description string) *Org {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, exists := st.OrgsByLogin[login]; exists {
		return nil
	}

	now := time.Now()
	org := &Org{
		ID:          st.NextOrg,
		NodeID:      fmt.Sprintf("O_kgDO%08d", st.NextOrg),
		Login:       login,
		Name:        name,
		Description: description,
		Type:        "Organization",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextOrg++

	st.Orgs[org.ID] = org
	st.OrgsByLogin[login] = org

	// Add creator as admin
	key := membershipKey(login, creator.ID)
	st.Memberships[key] = &Membership{
		OrgID:  org.ID,
		UserID: creator.ID,
		Role:   "admin",
		State:  "active",
	}

	return org
}

// GetOrg returns an organization by login, or nil if not found.
func (st *Store) GetOrg(login string) *Org {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgsByLogin[login]
}

// UpdateOrg applies a mutation function to an organization.
func (st *Store) UpdateOrg(login string, fn func(*Org)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	org, ok := st.OrgsByLogin[login]
	if !ok {
		return false
	}
	fn(org)
	org.UpdatedAt = time.Now()
	return true
}

// DeleteOrg removes an organization and all associated memberships and teams.
func (st *Store) DeleteOrg(login string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	org, ok := st.OrgsByLogin[login]
	if !ok {
		return false
	}

	// Remove all memberships for this org
	for k, m := range st.Memberships {
		if m.OrgID == org.ID {
			delete(st.Memberships, k)
		}
	}

	// Remove all teams for this org
	for k, t := range st.TeamsBySlug {
		if t.OrgID == org.ID {
			delete(st.Teams, t.ID)
			delete(st.TeamsBySlug, k)
		}
	}

	delete(st.Orgs, org.ID)
	delete(st.OrgsByLogin, login)
	return true
}

// ListOrgsByUser returns all organizations the user belongs to.
func (st *Store) ListOrgsByUser(userID int) []*Org {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var orgs []*Org
	for _, m := range st.Memberships {
		if m.UserID == userID && m.State == "active" {
			if org, ok := st.Orgs[m.OrgID]; ok {
				orgs = append(orgs, org)
			}
		}
	}
	return orgs
}

// SetMembership upserts a user's membership in an organization.
func (st *Store) SetMembership(orgLogin string, userID int, role string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	org, ok := st.OrgsByLogin[orgLogin]
	if !ok {
		return false
	}

	key := membershipKey(orgLogin, userID)
	st.Memberships[key] = &Membership{
		OrgID:  org.ID,
		UserID: userID,
		Role:   role,
		State:  "active",
	}
	return true
}

// GetMembership returns a user's membership in an organization, or nil.
func (st *Store) GetMembership(orgLogin string, userID int) *Membership {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Memberships[membershipKey(orgLogin, userID)]
}

// RemoveMembership removes a user's membership from an organization.
func (st *Store) RemoveMembership(orgLogin string, userID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := membershipKey(orgLogin, userID)
	if _, ok := st.Memberships[key]; !ok {
		return false
	}
	delete(st.Memberships, key)

	// Also remove from all teams in this org
	org := st.OrgsByLogin[orgLogin]
	if org != nil {
		for _, t := range st.TeamsBySlug {
			if t.OrgID == org.ID {
				for i, mid := range t.MemberIDs {
					if mid == userID {
						t.MemberIDs = append(t.MemberIDs[:i], t.MemberIDs[i+1:]...)
						break
					}
				}
			}
		}
	}

	return true
}

// ListOrgMembers returns all users who are active members of an organization.
func (st *Store) ListOrgMembers(orgLogin string) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}

	var users []*User
	for _, m := range st.Memberships {
		if m.OrgID == org.ID && m.State == "active" {
			if u, ok := st.Users[m.UserID]; ok {
				users = append(users, u)
			}
		}
	}
	return users
}

// CreateTeam creates a team within an organization.
func (st *Store) CreateTeam(orgLogin, name, description, privacy, permission string) *Team {
	st.mu.Lock()
	defer st.mu.Unlock()

	org, ok := st.OrgsByLogin[orgLogin]
	if !ok {
		return nil
	}

	slug := slugify(name)
	key := teamSlugKey(orgLogin, slug)
	if _, exists := st.TeamsBySlug[key]; exists {
		return nil
	}

	if privacy == "" {
		privacy = "closed"
	}
	if permission == "" {
		permission = "pull"
	}

	now := time.Now()
	team := &Team{
		ID:          st.NextTeam,
		NodeID:      fmt.Sprintf("T_kgDO%08d", st.NextTeam),
		OrgID:       org.ID,
		Name:        name,
		Slug:        slug,
		Description: description,
		Privacy:     privacy,
		Permission:  permission,
		MemberIDs:   []int{},
		RepoNames:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextTeam++

	st.Teams[team.ID] = team
	st.TeamsBySlug[key] = team
	return team
}

// GetTeam returns a team by org login and slug, or nil.
func (st *Store) GetTeam(orgLogin, slug string) *Team {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
}

// UpdateTeam applies a mutation function to a team.
func (st *Store) UpdateTeam(orgLogin, slug string, fn func(*Team)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := teamSlugKey(orgLogin, slug)
	team, ok := st.TeamsBySlug[key]
	if !ok {
		return false
	}
	fn(team)
	team.UpdatedAt = time.Now()
	return true
}

// DeleteTeam removes a team from an organization.
func (st *Store) DeleteTeam(orgLogin, slug string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := teamSlugKey(orgLogin, slug)
	team, ok := st.TeamsBySlug[key]
	if !ok {
		return false
	}

	delete(st.Teams, team.ID)
	delete(st.TeamsBySlug, key)
	return true
}

// ListTeams returns all teams in an organization.
func (st *Store) ListTeams(orgLogin string) []*Team {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}

	var teams []*Team
	for _, t := range st.TeamsBySlug {
		if t.OrgID == org.ID {
			teams = append(teams, t)
		}
	}
	return teams
}

// AddTeamMember adds a user to a team.
func (st *Store) AddTeamMember(orgLogin, slug string, userID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}

	for _, mid := range team.MemberIDs {
		if mid == userID {
			return true // already a member
		}
	}

	team.MemberIDs = append(team.MemberIDs, userID)
	team.UpdatedAt = time.Now()
	return true
}

// RemoveTeamMember removes a user from a team.
func (st *Store) RemoveTeamMember(orgLogin, slug string, userID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}

	for i, mid := range team.MemberIDs {
		if mid == userID {
			team.MemberIDs = append(team.MemberIDs[:i], team.MemberIDs[i+1:]...)
			team.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// AddTeamRepo adds a repository to a team's access list.
func (st *Store) AddTeamRepo(orgLogin, slug, repoFullName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}

	for _, rn := range team.RepoNames {
		if rn == repoFullName {
			return true // already added
		}
	}

	team.RepoNames = append(team.RepoNames, repoFullName)
	team.UpdatedAt = time.Now()
	return true
}

// RemoveTeamRepo removes a repository from a team's access list.
func (st *Store) RemoveTeamRepo(orgLogin, slug, repoFullName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}

	for i, rn := range team.RepoNames {
		if rn == repoFullName {
			team.RepoNames = append(team.RepoNames[:i], team.RepoNames[i+1:]...)
			team.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// CreateOrgRepo creates a repository owned by an organization.
func (st *Store) CreateOrgRepo(org *Org, creator *User, name, description string, private bool) *Repo {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := org.Login + "/" + name
	if _, exists := st.ReposByName[fullName]; exists {
		return nil
	}

	now := time.Now()
	visibility := "public"
	if private {
		visibility = "private"
	}

	repo := &Repo{
		ID:                  st.NextRepo,
		NodeID:              fmt.Sprintf("R_kgDO%08d", st.NextRepo),
		Name:                name,
		FullName:            fullName,
		Description:         description,
		DefaultBranch:       "main",
		Visibility:          visibility,
		Owner:               creator, // will also set OwnerType
		Private:             private,
		Topics:              []string{},
		NextIssueNumber:     1,
		NextMilestoneNumber: 1,
		CreatedAt:           now,
		UpdatedAt:           now,
		PushedAt:            now,
	}
	st.NextRepo++

	st.Repos[repo.ID] = repo
	st.ReposByName[fullName] = repo

	return repo
}
