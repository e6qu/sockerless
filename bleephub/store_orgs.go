package bleephub

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OrgRole is a user's role in an organization. The values are GitHub's
// wire enum for org-membership role.
type OrgRole string

const (
	OrgRoleAdmin  OrgRole = "admin"
	OrgRoleMember OrgRole = "member"
)

// MembershipState is the lifecycle state of an org membership: "pending"
// while an invitation awaits acceptance, "active" once accepted.
type MembershipState string

const (
	MembershipStateActive  MembershipState = "active"
	MembershipStatePending MembershipState = "pending"
)

// TeamPrivacy is GitHub's team visibility enum.
type TeamPrivacy string

const (
	TeamPrivacyClosed TeamPrivacy = "closed"
	TeamPrivacySecret TeamPrivacy = "secret"
)

// TeamPermission is the default repository permission a team confers.
type TeamPermission string

const (
	TeamPermissionPull  TeamPermission = "pull"
	TeamPermissionPush  TeamPermission = "push"
	TeamPermissionAdmin TeamPermission = "admin"
)

// TeamRole is a user's role within a team.
type TeamRole string

const (
	TeamRoleMember     TeamRole = "member"
	TeamRoleMaintainer TeamRole = "maintainer"
)

// TeamNotificationSetting is GitHub's team notification enum.
type TeamNotificationSetting string

const (
	TeamNotificationsEnabled  TeamNotificationSetting = "notifications_enabled"
	TeamNotificationsDisabled TeamNotificationSetting = "notifications_disabled"
)

// Org represents a GitHub organization account.
//
// MembersCanCreateRepositories is a pointer because GitHub's default is
// true: a nil value (including rows persisted before the field existed)
// means "default", not false.
type Org struct {
	ID                           int       `json:"id"`
	NodeID                       string    `json:"node_id"`
	Login                        string    `json:"login"`
	Name                         string    `json:"name"`
	Description                  string    `json:"description"`
	Email                        string    `json:"email"`
	AvatarURL                    string    `json:"avatar_url"`
	Type                         string    `json:"type"`
	Company                      string    `json:"company"`
	Blog                         string    `json:"blog"`
	Location                     string    `json:"location"`
	TwitterUsername              string    `json:"twitter_username"`
	BillingEmail                 string    `json:"billing_email"`
	DefaultRepositoryPermission  string    `json:"default_repository_permission"` // "" = GitHub default "read"
	MembersCanCreateRepositories *bool     `json:"members_can_create_repositories"`
	WebCommitSignoffRequired     bool      `json:"web_commit_signoff_required"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

// Membership represents a user's membership in an organization.
type Membership struct {
	OrgID  int             `json:"org_id"`
	UserID int             `json:"user_id"`
	Role   OrgRole         `json:"role"`
	State  MembershipState `json:"state"`
	Public bool            `json:"public"` // publicized via PUT /orgs/{org}/public_members/{username}
}

// Team represents a team within an organization.
type Team struct {
	ID                  int                       `json:"id"`
	NodeID              string                    `json:"node_id"`
	OrgID               int                       `json:"org_id"`
	Name                string                    `json:"name"`
	Slug                string                    `json:"slug"`
	Description         string                    `json:"description"`
	Privacy             TeamPrivacy               `json:"privacy"`
	Permission          TeamPermission            `json:"permission"`
	NotificationSetting TeamNotificationSetting   `json:"notification_setting"`
	ParentID            int                       `json:"parent_id"` // 0 = no parent team
	MemberIDs           []int                     `json:"member_ids"`
	MaintainerIDs       []int                     `json:"maintainer_ids"`   // subset of MemberIDs with the maintainer role
	RepoNames           []string                  `json:"repo_names"`       // "owner/name" entries
	RepoPermissions     map[string]TeamPermission `json:"repo_permissions"` // per-repo override; nil/missing entry uses Permission
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
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

	now := time.Now().UTC()
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
	m := &Membership{
		OrgID:  org.ID,
		UserID: creator.ID,
		Role:   OrgRoleAdmin,
		State:  MembershipStateActive,
	}
	st.Memberships[key] = m

	if st.persist != nil {
		st.persist.MustPut("orgs", strconv.Itoa(org.ID), org)
		st.persist.MustPut("memberships", key, m)
	}

	return org
}

// GetOrg returns an organization by login, or nil if not found.
func (st *Store) GetOrg(login string) *Org {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgsByLogin[login]
}

// GetOrgByID returns an organization by its numeric ID, or nil.
func (st *Store) GetOrgByID(id int) *Org {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Orgs[id]
}

// ListTeamsByUser returns every team across all orgs that the given user is a member of.
func (st *Store) ListTeamsByUser(userID int) []*Team {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var teams []*Team
	for _, t := range st.Teams {
		for _, mid := range t.MemberIDs {
			if mid == userID {
				teams = append(teams, t)
				break
			}
		}
	}
	return teams
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
	org.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("orgs", strconv.Itoa(org.ID), org)
	}
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
			if st.persist != nil {
				st.persist.MustDelete("memberships", k)
			}
		}
	}

	// Remove all teams for this org
	for k, t := range st.TeamsBySlug {
		if t.OrgID == org.ID {
			delete(st.Teams, t.ID)
			delete(st.TeamsBySlug, k)
			if st.persist != nil {
				st.persist.MustDelete("teams", strconv.Itoa(t.ID))
			}
		}
	}

	delete(st.Orgs, org.ID)
	delete(st.OrgsByLogin, login)
	if st.persist != nil {
		st.persist.MustDelete("orgs", strconv.Itoa(org.ID))
	}
	return true
}

// ListOrgsByUser returns all organizations the user belongs to, in
// ascending id order like real GitHub. The memberships map iterates in
// random order; the /user/orgs handlers paginate over this list, and
// offset pagination over an unstable order would skip or duplicate orgs
// across pages.
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
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].ID < orgs[j].ID })
	return orgs
}

// SetMembership upserts a user's membership in an organization with the
// given role and state. An existing membership keeps its Public flag.
// Returns the stored membership, or nil if the org doesn't exist.
func (st *Store) SetMembership(orgLogin string, userID int, role OrgRole, state MembershipState) *Membership {
	st.mu.Lock()
	defer st.mu.Unlock()

	org, ok := st.OrgsByLogin[orgLogin]
	if !ok {
		return nil
	}

	key := membershipKey(orgLogin, userID)
	m := st.Memberships[key]
	if m == nil {
		m = &Membership{OrgID: org.ID, UserID: userID}
		st.Memberships[key] = m
	}
	m.Role = role
	m.State = state
	if st.persist != nil {
		st.persist.MustPut("memberships", key, m)
	}
	// An activated membership completes any organization invitation the
	// user held: the invitee joins the invited teams and the invitation
	// row is consumed.
	if state == MembershipStateActive {
		st.consumeOrgInvitationsForUserLocked(orgLogin, userID)
	}
	return m
}

// SetMembershipPublic flips the membership's public-member flag. Returns
// false when no active membership exists.
func (st *Store) SetMembershipPublic(orgLogin string, userID int, public bool) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := membershipKey(orgLogin, userID)
	m := st.Memberships[key]
	if m == nil || m.State != MembershipStateActive {
		return false
	}
	m.Public = public
	if st.persist != nil {
		st.persist.MustPut("memberships", key, m)
	}
	return true
}

// ListPublicOrgMembers returns active members who publicized their membership.
func (st *Store) ListPublicOrgMembers(orgLogin string) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	var users []*User
	for _, m := range st.Memberships {
		if m.OrgID == org.ID && m.State == MembershipStateActive && m.Public {
			if u, ok := st.Users[m.UserID]; ok {
				users = append(users, u)
			}
		}
	}
	return users
}

// ListMembershipsByUser returns the user's memberships across all orgs,
// optionally filtered by state ("" = all).
func (st *Store) ListMembershipsByUser(userID int, state MembershipState) []*Membership {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []*Membership
	for _, m := range st.Memberships {
		if m.UserID != userID {
			continue
		}
		if state != "" && m.State != state {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ListOrgsAll returns every organization with ID greater than `since`,
// ordered by ID ascending — the GET /organizations contract.
func (st *Store) ListOrgsAll(since int) []*Org {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var orgs []*Org
	for _, o := range st.Orgs {
		if o.ID > since {
			orgs = append(orgs, o)
		}
	}
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].ID < orgs[j].ID })
	return orgs
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
	if st.persist != nil {
		st.persist.MustDelete("memberships", key)
	}

	// Also remove from all teams in this org; re-persist every team whose
	// member list changed so the removal sticks across restarts.
	org := st.OrgsByLogin[orgLogin]
	if org != nil {
		for _, t := range st.TeamsBySlug {
			if t.OrgID == org.ID {
				for i, mid := range t.MemberIDs {
					if mid == userID {
						t.MemberIDs = append(t.MemberIDs[:i], t.MemberIDs[i+1:]...)
						if st.persist != nil {
							st.persist.MustPut("teams", strconv.Itoa(t.ID), t)
						}
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

// TeamOptions carries the optional attributes of team creation.
type TeamOptions struct {
	Description         string
	Privacy             TeamPrivacy
	Permission          TeamPermission
	NotificationSetting TeamNotificationSetting
	ParentID            int
}

// CreateTeam creates a team within an organization. A non-zero ParentID
// must reference an existing team in the same org.
func (st *Store) CreateTeam(orgLogin, name string, opts TeamOptions) *Team {
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

	if opts.Privacy == "" {
		opts.Privacy = TeamPrivacyClosed
	}
	if opts.Permission == "" {
		opts.Permission = TeamPermissionPull
	}
	if opts.NotificationSetting == "" {
		opts.NotificationSetting = TeamNotificationsEnabled
	}
	if opts.ParentID != 0 {
		parent := st.Teams[opts.ParentID]
		if parent == nil || parent.OrgID != org.ID {
			return nil
		}
	}

	now := time.Now().UTC()
	team := &Team{
		ID:                  st.NextTeam,
		NodeID:              fmt.Sprintf("T_kgDO%08d", st.NextTeam),
		OrgID:               org.ID,
		Name:                name,
		Slug:                slug,
		Description:         opts.Description,
		Privacy:             opts.Privacy,
		Permission:          opts.Permission,
		NotificationSetting: opts.NotificationSetting,
		ParentID:            opts.ParentID,
		MemberIDs:           []int{},
		MaintainerIDs:       []int{},
		RepoNames:           []string{},
		RepoPermissions:     map[string]TeamPermission{},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	st.NextTeam++

	st.Teams[team.ID] = team
	st.TeamsBySlug[key] = team
	if st.persist != nil {
		st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
	}
	return team
}

// GetTeam returns a team by org login and slug, or nil.
func (st *Store) GetTeam(orgLogin, slug string) *Team {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
}

// GetTeamByID returns a team by its numeric ID, or nil.
func (st *Store) GetTeamByID(id int) *Team {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Teams[id]
}

// UpdateTeam applies a mutation function to a team. When the mutation
// changes the slug (team rename), the slug index is re-keyed so the old
// slug stops resolving and the new one does.
func (st *Store) UpdateTeam(orgLogin, slug string, fn func(*Team)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := teamSlugKey(orgLogin, slug)
	team, ok := st.TeamsBySlug[key]
	if !ok {
		return false
	}
	fn(team)
	if team.Slug != slug {
		delete(st.TeamsBySlug, key)
		st.TeamsBySlug[teamSlugKey(orgLogin, team.Slug)] = team
	}
	team.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
	}
	return true
}

// TeamParentWouldCycle reports whether re-parenting team `teamID` under
// `parentID` would create a cycle in the team hierarchy.
func (st *Store) TeamParentWouldCycle(teamID, parentID int) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

	for id := parentID; id != 0; {
		if id == teamID {
			return true
		}
		parent := st.Teams[id]
		if parent == nil {
			return false
		}
		id = parent.ParentID
	}
	return false
}

// ListChildTeams returns the teams whose parent is the given team.
func (st *Store) ListChildTeams(orgLogin string, parentID int) []*Team {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	var out []*Team
	for _, t := range st.TeamsBySlug {
		if t.OrgID == org.ID && t.ParentID == parentID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	if st.persist != nil {
		st.persist.MustDelete("teams", strconv.Itoa(team.ID))
	}

	// Children of a deleted team move up to the deleted team's parent
	// (real GitHub re-parents rather than orphaning).
	for _, t := range st.Teams {
		if t.ParentID == team.ID {
			t.ParentID = team.ParentID
			if st.persist != nil {
				st.persist.MustPut("teams", strconv.Itoa(t.ID), t)
			}
		}
	}
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

// ListTeamMembers returns the users who are members of a team.
func (st *Store) ListTeamMembers(orgLogin, slug string) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return nil
	}
	members := make([]*User, 0, len(team.MemberIDs))
	for _, uid := range team.MemberIDs {
		if u, ok := st.Users[uid]; ok {
			members = append(members, u)
		}
	}
	return members
}

// GetTeamMembership returns a user's role in a team and whether they are a
// member at all. The role is empty when the user is not a member.
func (st *Store) GetTeamMembership(orgLogin, slug string, userID int) (TeamRole, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return "", false
	}
	return team.roleOf(userID)
}

// SetTeamMembership upserts a user's team membership with the given role.
func (st *Store) SetTeamMembership(orgLogin, slug string, userID int, role TeamRole) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}

	if !slices.Contains(team.MemberIDs, userID) {
		team.MemberIDs = append(team.MemberIDs, userID)
	}
	switch role {
	case TeamRoleMaintainer:
		if !slices.Contains(team.MaintainerIDs, userID) {
			team.MaintainerIDs = append(team.MaintainerIDs, userID)
		}
	default:
		team.MaintainerIDs = intSliceRemove(team.MaintainerIDs, userID)
	}
	team.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
	}
	return true
}

// RemoveTeamMembership removes a user from a team.
func (st *Store) RemoveTeamMembership(orgLogin, slug string, userID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}
	if !slices.Contains(team.MemberIDs, userID) {
		return false
	}
	team.MemberIDs = intSliceRemove(team.MemberIDs, userID)
	team.MaintainerIDs = intSliceRemove(team.MaintainerIDs, userID)
	team.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
	}
	return true
}

// ListTeamRepos returns the repositories linked to a team.
func (st *Store) ListTeamRepos(orgLogin, slug string) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return nil
	}
	repos := make([]*Repo, 0, len(team.RepoNames))
	for _, fullName := range team.RepoNames {
		owner, name, ok := strings.Cut(fullName, "/")
		if !ok {
			continue
		}
		if repo := st.ReposByName[owner+"/"+name]; repo != nil {
			repos = append(repos, repo)
		}
	}
	return repos
}

// GetTeamRepoPermission returns the effective permission a team confers on a
// repository. The second value is false when the repository is not linked to
// the team. A nil/missing per-repo override falls back to the team's default
// Permission.
func (st *Store) GetTeamRepoPermission(orgLogin, slug, fullName string) (TeamPermission, bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return "", false
	}
	if !slices.Contains(team.RepoNames, fullName) {
		return "", false
	}
	if team.RepoPermissions != nil {
		if perm, ok := team.RepoPermissions[fullName]; ok {
			return perm, true
		}
	}
	return team.Permission, true
}

// SetTeamRepoPermission links a repository to a team and records an explicit
// permission override. An empty permission uses the team's default.
func (st *Store) SetTeamRepoPermission(orgLogin, slug, fullName string, perm TeamPermission) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	team := st.TeamsBySlug[teamSlugKey(orgLogin, slug)]
	if team == nil {
		return false
	}
	if st.ReposByName[fullName] == nil {
		return false
	}

	found := false
	for _, rn := range team.RepoNames {
		if rn == fullName {
			found = true
			break
		}
	}
	if !found {
		team.RepoNames = append(team.RepoNames, fullName)
	}
	if perm != "" {
		if team.RepoPermissions == nil {
			team.RepoPermissions = map[string]TeamPermission{}
		}
		team.RepoPermissions[fullName] = perm
	}
	team.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
	}
	return true
}

// roleOf returns the user's role in the team, and whether they're a member.
func (t *Team) roleOf(userID int) (TeamRole, bool) {
	if !slices.Contains(t.MemberIDs, userID) {
		return "", false
	}
	if slices.Contains(t.MaintainerIDs, userID) {
		return TeamRoleMaintainer, true
	}
	return TeamRoleMember, true
}

func intSliceRemove(s []int, v int) []int {
	if i := slices.Index(s, v); i >= 0 {
		return slices.Delete(s, i, i+1)
	}
	return s
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
	if team.RepoPermissions == nil {
		team.RepoPermissions = map[string]TeamPermission{}
	}
	team.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
	}
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
			if team.RepoPermissions != nil {
				delete(team.RepoPermissions, repoFullName)
			}
			team.UpdatedAt = time.Now().UTC()
			if st.persist != nil {
				st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
			}
			return true
		}
	}
	return false
}

// CreateOrgRepo creates a repository owned by an organization.
func (st *Store) CreateOrgRepo(org *Org, creator *User, name, description string, private bool) *Repo {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createRepoLocked(org.Login+"/"+name, name, description, private, org.ID, "Organization", nil)
}
