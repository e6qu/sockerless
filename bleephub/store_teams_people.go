package bleephub

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Store state for the organization people surfaces: organization
// invitations, user blocks, interaction limits, organization-role
// assignments, and outside collaborators.

// orgInvitationTTL is how long an organization invitation stays pending
// before it fails as expired — GitHub org invitations expire after 7 days.
const orgInvitationTTL = 7 * 24 * time.Hour

// OrgInvitation is a pending (or failed) invitation to join an
// organization, created via POST /orgs/{org}/invitations. The Role holds
// GitHub's invitation-role wire value (direct_member | admin |
// billing_manager), which is distinct from the membership role enum.
type OrgInvitation struct {
	ID           int        `json:"id"`
	NodeID       string     `json:"node_id"`
	OrgID        int        `json:"org_id"`
	UserID       int        `json:"user_id"` // resolved invitee; 0 when the email matches no account
	Login        string     `json:"login"`   // "" for email-only invitations
	Email        string     `json:"email"`
	Role         string     `json:"role"`
	InviterID    int        `json:"inviter_id"`
	TeamIDs      []int      `json:"team_ids"`
	Source       string     `json:"source"` // "member" for API-created invitations
	CreatedAt    time.Time  `json:"created_at"`
	FailedAt     *time.Time `json:"failed_at,omitempty"`
	FailedReason string     `json:"failed_reason,omitempty"`
}

// OrgInteractionLimit is an organization-wide interaction restriction
// (PUT /orgs/{org}/interaction-limits). The limit auto-expires at
// ExpiresAt, exactly like real GitHub's temporary restrictions.
type OrgInteractionLimit struct {
	Limit     string    `json:"limit"`
	ExpiresAt time.Time `json:"expires_at"`
}

// invitationMembershipRole maps an invitation role to the org-membership
// role the invitee holds once they accept. bleephub has no separate
// billing-manager account class, so a billing_manager invitation confers
// ordinary membership.
func invitationMembershipRole(role string) OrgRole {
	if role == "admin" {
		return OrgRoleAdmin
	}
	return OrgRoleMember
}

// CreateOrgInvitation creates an organization invitation and, when the
// invitee resolves to an account, the pending membership the invitee
// accepts through PATCH /user/memberships/orgs/{org}. Returns nil and a
// reason string when the invitation is invalid (already a member,
// already invited).
func (st *Store) CreateOrgInvitation(org *Org, inviter *User, invitee *User, email, role string, teamIDs []int) (*OrgInvitation, string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if invitee != nil {
		if m := st.Memberships[membershipKey(org.Login, invitee.ID)]; m != nil {
			if m.State == MembershipStateActive {
				return nil, "Invitee is already a member of this organization."
			}
			return nil, "A pending invitation already exists for this invitee."
		}
		for _, inv := range st.OrgInvitations {
			if inv.OrgID == org.ID && inv.FailedAt == nil && inv.UserID == invitee.ID {
				return nil, "A pending invitation already exists for this invitee."
			}
		}
	} else {
		for _, inv := range st.OrgInvitations {
			if inv.OrgID == org.ID && inv.FailedAt == nil && inv.Email != "" && strings.EqualFold(inv.Email, email) {
				return nil, "A pending invitation already exists for this invitee."
			}
		}
	}

	inv := &OrgInvitation{
		ID:        st.NextOrgInvitationID,
		NodeID:    fmt.Sprintf("OI_kgDO%08d", st.NextOrgInvitationID),
		OrgID:     org.ID,
		Email:     email,
		Role:      role,
		InviterID: inviter.ID,
		TeamIDs:   append([]int{}, teamIDs...),
		Source:    "member",
		CreatedAt: time.Now().UTC(),
	}
	st.NextOrgInvitationID++
	if invitee != nil {
		inv.UserID = invitee.ID
		inv.Login = invitee.Login
	}
	st.OrgInvitations[inv.ID] = inv

	if invitee != nil {
		key := membershipKey(org.Login, invitee.ID)
		m := &Membership{OrgID: org.ID, UserID: invitee.ID, Role: invitationMembershipRole(role), State: MembershipStatePending}
		st.Memberships[key] = m
		if st.persist != nil {
			st.persist.MustPut("memberships", key, m)
		}
	}
	if st.persist != nil {
		st.persist.MustPut("org_invitations", strconv.Itoa(inv.ID), inv)
	}
	return inv, ""
}

// reconcileOrgInvitationsLocked brings the org's invitations in line with
// the authoritative membership state: invitations whose membership turned
// active are consumed (the invitee joins the invited teams), invitations
// whose pending membership disappeared are cancelled, and invitations
// older than the 7-day TTL fail as expired (dropping the pending
// membership). Callers must hold st.mu for writing.
func (st *Store) reconcileOrgInvitationsLocked(org *Org, now time.Time) {
	for id, inv := range st.OrgInvitations {
		if inv.OrgID != org.ID || inv.FailedAt != nil {
			continue
		}
		if inv.UserID != 0 {
			m := st.Memberships[membershipKey(org.Login, inv.UserID)]
			switch {
			case m == nil:
				// The pending membership was removed out-of-band; the
				// invitation no longer has anything to accept.
				delete(st.OrgInvitations, id)
				if st.persist != nil {
					st.persist.MustDelete("org_invitations", strconv.Itoa(id))
				}
				continue
			case m.State == MembershipStateActive:
				st.consumeOrgInvitationLocked(inv)
				continue
			}
		}
		if now.Sub(inv.CreatedAt) > orgInvitationTTL {
			failedAt := inv.CreatedAt.Add(orgInvitationTTL)
			inv.FailedAt = &failedAt
			inv.FailedReason = "Invitation expired."
			if inv.UserID != 0 {
				key := membershipKey(org.Login, inv.UserID)
				if m := st.Memberships[key]; m != nil && m.State == MembershipStatePending {
					delete(st.Memberships, key)
					if st.persist != nil {
						st.persist.MustDelete("memberships", key)
					}
				}
			}
			if st.persist != nil {
				st.persist.MustPut("org_invitations", strconv.Itoa(inv.ID), inv)
			}
		}
	}
}

// consumeOrgInvitationLocked completes an accepted invitation: the
// invitee joins every team the invitation carried and the invitation
// itself is removed. Callers must hold st.mu for writing.
func (st *Store) consumeOrgInvitationLocked(inv *OrgInvitation) {
	for _, teamID := range inv.TeamIDs {
		team := st.Teams[teamID]
		if team == nil || team.OrgID != inv.OrgID {
			continue
		}
		if !slices.Contains(team.MemberIDs, inv.UserID) {
			team.MemberIDs = append(team.MemberIDs, inv.UserID)
			team.UpdatedAt = time.Now().UTC()
			if st.persist != nil {
				st.persist.MustPut("teams", strconv.Itoa(team.ID), team)
			}
		}
	}
	delete(st.OrgInvitations, inv.ID)
	if st.persist != nil {
		st.persist.MustDelete("org_invitations", strconv.Itoa(inv.ID))
	}
}

// consumeOrgInvitationsForUserLocked consumes every live invitation the
// user holds in the org — invoked when a membership turns active so the
// invited-team joins happen at acceptance time. Callers must hold st.mu
// for writing.
func (st *Store) consumeOrgInvitationsForUserLocked(orgLogin string, userID int) {
	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return
	}
	for _, inv := range st.OrgInvitations {
		if inv.OrgID == org.ID && inv.UserID == userID && inv.FailedAt == nil {
			st.consumeOrgInvitationLocked(inv)
		}
	}
}

// ListPendingOrgInvitations returns the org's live invitations sorted by
// ID, reconciling state first (expiry, out-of-band accepts/cancels).
func (st *Store) ListPendingOrgInvitations(orgLogin string) []*OrgInvitation {
	st.mu.Lock()
	defer st.mu.Unlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	st.reconcileOrgInvitationsLocked(org, time.Now().UTC())
	var out []*OrgInvitation
	for _, inv := range st.OrgInvitations {
		if inv.OrgID == org.ID && inv.FailedAt == nil {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListFailedOrgInvitations returns the org's failed (expired)
// invitations sorted by ID.
func (st *Store) ListFailedOrgInvitations(orgLogin string) []*OrgInvitation {
	st.mu.Lock()
	defer st.mu.Unlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	st.reconcileOrgInvitationsLocked(org, time.Now().UTC())
	var out []*OrgInvitation
	for _, inv := range st.OrgInvitations {
		if inv.OrgID == org.ID && inv.FailedAt != nil {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetOrgInvitation returns a live (non-failed) invitation by org and ID.
func (st *Store) GetOrgInvitation(orgLogin string, id int) *OrgInvitation {
	st.mu.Lock()
	defer st.mu.Unlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	st.reconcileOrgInvitationsLocked(org, time.Now().UTC())
	inv := st.OrgInvitations[id]
	if inv == nil || inv.OrgID != org.ID || inv.FailedAt != nil {
		return nil
	}
	return inv
}

// CancelOrgInvitation removes a live invitation and its pending
// membership. Returns false when no such invitation exists.
func (st *Store) CancelOrgInvitation(orgLogin string, id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return false
	}
	inv := st.OrgInvitations[id]
	if inv == nil || inv.OrgID != org.ID || inv.FailedAt != nil {
		return false
	}
	if inv.UserID != 0 {
		key := membershipKey(org.Login, inv.UserID)
		if m := st.Memberships[key]; m != nil && m.State == MembershipStatePending {
			delete(st.Memberships, key)
			if st.persist != nil {
				st.persist.MustDelete("memberships", key)
			}
		}
	}
	delete(st.OrgInvitations, id)
	if st.persist != nil {
		st.persist.MustDelete("org_invitations", strconv.Itoa(id))
	}
	return true
}

// ListPendingOrgInvitationsForTeam returns the org's live invitations
// that carry the given team, sorted by ID.
func (st *Store) ListPendingOrgInvitationsForTeam(orgLogin string, teamID int) []*OrgInvitation {
	pending := st.ListPendingOrgInvitations(orgLogin)
	var out []*OrgInvitation
	for _, inv := range pending {
		if slices.Contains(inv.TeamIDs, teamID) {
			out = append(out, inv)
		}
	}
	return out
}

// --- organization user blocks ---

// BlockUserForOrg records a block of the user by the organization.
// Idempotent.
func (st *Store) BlockUserForOrg(orgLogin string, userID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgBlocks[orgLogin] == nil {
		st.OrgBlocks[orgLogin] = map[int]time.Time{}
	}
	if _, ok := st.OrgBlocks[orgLogin][userID]; !ok {
		st.OrgBlocks[orgLogin][userID] = time.Now().UTC()
	}
	if st.persist != nil {
		st.persist.MustPut("org_blocks", orgLogin, st.OrgBlocks[orgLogin])
	}
}

// UnblockUserForOrg removes an organization's block of the user.
// Idempotent.
func (st *Store) UnblockUserForOrg(orgLogin string, userID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgBlocks[orgLogin] == nil {
		return
	}
	delete(st.OrgBlocks[orgLogin], userID)
	if st.persist != nil {
		st.persist.MustPut("org_blocks", orgLogin, st.OrgBlocks[orgLogin])
	}
}

// IsUserBlockedByOrg reports whether the organization blocks the user.
func (st *Store) IsUserBlockedByOrg(orgLogin string, userID int) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()
	_, ok := st.OrgBlocks[orgLogin][userID]
	return ok
}

// ListOrgBlockedUsers returns the users the organization blocks, sorted
// by user ID.
func (st *Store) ListOrgBlockedUsers(orgLogin string) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()

	ids := make([]int, 0, len(st.OrgBlocks[orgLogin]))
	for id := range st.OrgBlocks[orgLogin] {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]*User, 0, len(ids))
	for _, id := range ids {
		if u := st.Users[id]; u != nil {
			out = append(out, u)
		}
	}
	return out
}

// --- organization interaction limits ---

// GetOrgInteractionLimit returns the org's active interaction limit, or
// nil when none is set. An expired limit is removed on read — real
// GitHub interaction restrictions lapse automatically at their expiry.
func (st *Store) GetOrgInteractionLimit(orgLogin string) *OrgInteractionLimit {
	st.mu.Lock()
	defer st.mu.Unlock()

	lim := st.OrgInteractionLimits[orgLogin]
	if lim == nil {
		return nil
	}
	if time.Now().UTC().After(lim.ExpiresAt) {
		delete(st.OrgInteractionLimits, orgLogin)
		if st.persist != nil {
			st.persist.MustDelete("org_interaction_limits", orgLogin)
		}
		return nil
	}
	return lim
}

// SetOrgInteractionLimit stores the org's interaction limit.
func (st *Store) SetOrgInteractionLimit(orgLogin, limit string, expiresAt time.Time) *OrgInteractionLimit {
	st.mu.Lock()
	defer st.mu.Unlock()

	lim := &OrgInteractionLimit{Limit: limit, ExpiresAt: expiresAt}
	st.OrgInteractionLimits[orgLogin] = lim
	if st.persist != nil {
		st.persist.MustPut("org_interaction_limits", orgLogin, lim)
	}
	return lim
}

// DeleteOrgInteractionLimit removes the org's interaction limit.
// Idempotent.
func (st *Store) DeleteOrgInteractionLimit(orgLogin string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	delete(st.OrgInteractionLimits, orgLogin)
	if st.persist != nil {
		st.persist.MustDelete("org_interaction_limits", orgLogin)
	}
}

// --- organization role assignments ---

// AssignOrgRoleToTeam grants an organization role to a team. Idempotent.
func (st *Store) AssignOrgRoleToTeam(orgLogin string, roleID, teamID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgRoleTeamAssignments[orgLogin] == nil {
		st.OrgRoleTeamAssignments[orgLogin] = map[int][]int{}
	}
	ids := st.OrgRoleTeamAssignments[orgLogin][roleID]
	if !slices.Contains(ids, teamID) {
		st.OrgRoleTeamAssignments[orgLogin][roleID] = append(ids, teamID)
	}
	if st.persist != nil {
		st.persist.MustPut("org_role_team_assignments", orgLogin, st.OrgRoleTeamAssignments[orgLogin])
	}
}

// UnassignOrgRoleFromTeam revokes one organization role from a team.
// Idempotent.
func (st *Store) UnassignOrgRoleFromTeam(orgLogin string, roleID, teamID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgRoleTeamAssignments[orgLogin] == nil {
		return
	}
	st.OrgRoleTeamAssignments[orgLogin][roleID] = intSliceRemove(st.OrgRoleTeamAssignments[orgLogin][roleID], teamID)
	if st.persist != nil {
		st.persist.MustPut("org_role_team_assignments", orgLogin, st.OrgRoleTeamAssignments[orgLogin])
	}
}

// UnassignAllOrgRolesFromTeam revokes every organization role from a
// team. Idempotent.
func (st *Store) UnassignAllOrgRolesFromTeam(orgLogin string, teamID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgRoleTeamAssignments[orgLogin] == nil {
		return
	}
	for roleID, ids := range st.OrgRoleTeamAssignments[orgLogin] {
		st.OrgRoleTeamAssignments[orgLogin][roleID] = intSliceRemove(ids, teamID)
	}
	if st.persist != nil {
		st.persist.MustPut("org_role_team_assignments", orgLogin, st.OrgRoleTeamAssignments[orgLogin])
	}
}

// AssignOrgRoleToUser grants an organization role to a user directly.
// Idempotent.
func (st *Store) AssignOrgRoleToUser(orgLogin string, roleID, userID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgRoleUserAssignments[orgLogin] == nil {
		st.OrgRoleUserAssignments[orgLogin] = map[int][]int{}
	}
	ids := st.OrgRoleUserAssignments[orgLogin][roleID]
	if !slices.Contains(ids, userID) {
		st.OrgRoleUserAssignments[orgLogin][roleID] = append(ids, userID)
	}
	if st.persist != nil {
		st.persist.MustPut("org_role_user_assignments", orgLogin, st.OrgRoleUserAssignments[orgLogin])
	}
}

// UnassignOrgRoleFromUser revokes one organization role from a user.
// Idempotent.
func (st *Store) UnassignOrgRoleFromUser(orgLogin string, roleID, userID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgRoleUserAssignments[orgLogin] == nil {
		return
	}
	st.OrgRoleUserAssignments[orgLogin][roleID] = intSliceRemove(st.OrgRoleUserAssignments[orgLogin][roleID], userID)
	if st.persist != nil {
		st.persist.MustPut("org_role_user_assignments", orgLogin, st.OrgRoleUserAssignments[orgLogin])
	}
}

// UnassignAllOrgRolesFromUser revokes every organization role from a
// user. Idempotent.
func (st *Store) UnassignAllOrgRolesFromUser(orgLogin string, userID int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.OrgRoleUserAssignments[orgLogin] == nil {
		return
	}
	for roleID, ids := range st.OrgRoleUserAssignments[orgLogin] {
		st.OrgRoleUserAssignments[orgLogin][roleID] = intSliceRemove(ids, userID)
	}
	if st.persist != nil {
		st.persist.MustPut("org_role_user_assignments", orgLogin, st.OrgRoleUserAssignments[orgLogin])
	}
}

// ListTeamsWithOrgRole returns the org's existing teams holding the
// role, sorted by team ID. Assignments to since-deleted teams are
// skipped — team existence is the source of truth.
func (st *Store) ListTeamsWithOrgRole(orgLogin string, roleID int) []*Team {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	ids := append([]int{}, st.OrgRoleTeamAssignments[orgLogin][roleID]...)
	sort.Ints(ids)
	out := make([]*Team, 0, len(ids))
	for _, id := range ids {
		if team := st.Teams[id]; team != nil && team.OrgID == org.ID {
			out = append(out, team)
		}
	}
	return out
}

// ListUsersWithOrgRole returns the users holding the role, mapping each
// user ID to the GitHub assignment kind: "direct" (assigned to the
// user), "indirect" (via a team holding the role), or "mixed" (both).
// Users without an active org membership are skipped — membership is
// the source of truth.
func (st *Store) ListUsersWithOrgRole(orgLogin string, roleID int) map[int]string {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	activeMember := func(userID int) bool {
		m := st.Memberships[membershipKey(orgLogin, userID)]
		return m != nil && m.State == MembershipStateActive
	}

	out := map[int]string{}
	for _, userID := range st.OrgRoleUserAssignments[orgLogin][roleID] {
		if st.Users[userID] != nil && activeMember(userID) {
			out[userID] = "direct"
		}
	}
	for _, teamID := range st.OrgRoleTeamAssignments[orgLogin][roleID] {
		team := st.Teams[teamID]
		if team == nil || team.OrgID != org.ID {
			continue
		}
		for _, userID := range team.MemberIDs {
			if st.Users[userID] == nil || !activeMember(userID) {
				continue
			}
			if out[userID] == "direct" {
				out[userID] = "mixed"
			} else if out[userID] == "" {
				out[userID] = "indirect"
			}
		}
	}
	return out
}

// --- outside collaborators ---

// ListOutsideCollaborators returns users who collaborate on at least one
// of the organization's repositories without holding an active org
// membership, sorted by user ID. Derived from the repo-collaborator
// grants — the same state the repo collaborator endpoints serve.
func (st *Store) ListOutsideCollaborators(orgLogin string) []*User {
	st.mu.RLock()
	defer st.mu.RUnlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return nil
	}
	prefix := orgLogin + "/"
	seen := map[int]bool{}
	var out []*User
	for repoKey, collabs := range st.RepoCollaborators {
		if !strings.HasPrefix(repoKey, prefix) {
			continue
		}
		repo := st.ReposByName[repoKey]
		if repo == nil || repo.OwnerType != "Organization" {
			continue
		}
		for login := range collabs {
			u := st.UsersByLogin[login]
			if u == nil || seen[u.ID] {
				continue
			}
			if m := st.Memberships[membershipKey(orgLogin, u.ID)]; m != nil && m.State == MembershipStateActive {
				continue
			}
			seen[u.ID] = true
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GrantTeamRepoAccessAsCollaborator materializes a member's team-derived
// repository access as direct collaborator grants — the state a member
// keeps when converted to an outside collaborator ("they'll only have
// access to the repositories that their current team membership
// allows"). Existing direct grants are kept when stronger.
func (st *Store) GrantTeamRepoAccessAsCollaborator(orgLogin string, user *User) {
	st.mu.Lock()
	defer st.mu.Unlock()

	org := st.OrgsByLogin[orgLogin]
	if org == nil {
		return
	}
	levels := map[string]int{"pull": 1, "push": 2, "admin": 3}
	permName := func(p TeamPermission) string {
		switch p {
		case TeamPermissionPush:
			return "push"
		case TeamPermissionAdmin:
			return "admin"
		default:
			return "pull"
		}
	}
	changed := map[string]bool{}
	for _, team := range st.TeamsBySlug {
		if team.OrgID != org.ID || !slices.Contains(team.MemberIDs, user.ID) {
			continue
		}
		for _, repoKey := range team.RepoNames {
			if st.ReposByName[repoKey] == nil {
				continue
			}
			perm := team.Permission
			if team.RepoPermissions != nil {
				if override, ok := team.RepoPermissions[repoKey]; ok {
					perm = override
				}
			}
			grant := permName(perm)
			if st.RepoCollaborators[repoKey] == nil {
				st.RepoCollaborators[repoKey] = map[string]string{}
			}
			if existing := st.RepoCollaborators[repoKey][user.Login]; levels[existing] >= levels[grant] {
				continue
			}
			st.RepoCollaborators[repoKey][user.Login] = grant
			changed[repoKey] = true
		}
	}
	if st.persist != nil {
		for repoKey := range changed {
			st.persist.MustPut("repo_collaborators", repoKey, st.RepoCollaborators[repoKey])
		}
	}
}

// RemoveOutsideCollaborator strips the user's collaborator grants and
// pending repository invitations across every repository of the
// organization.
func (st *Store) RemoveOutsideCollaborator(orgLogin, login string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	prefix := orgLogin + "/"
	for repoKey, collabs := range st.RepoCollaborators {
		if !strings.HasPrefix(repoKey, prefix) {
			continue
		}
		if _, ok := collabs[login]; !ok {
			continue
		}
		delete(collabs, login)
		if st.persist != nil {
			st.persist.MustPut("repo_collaborators", repoKey, collabs)
		}
	}
	for repoKey, invs := range st.RepoInvitations {
		if !strings.HasPrefix(repoKey, prefix) {
			continue
		}
		removed := false
		for id, inv := range invs {
			if inv.Status == "pending" && strings.EqualFold(inv.InviteeLogin, login) {
				delete(invs, id)
				removed = true
			}
		}
		if removed && st.persist != nil {
			st.persist.MustPut("repo_invitations", repoKey, invs)
		}
	}
}
