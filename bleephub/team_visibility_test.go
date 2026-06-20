package bleephub

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestTeamVisibilityNonMember asserts that an org's teams (and their members /
// repo grants) are hidden from a non-member authenticated caller — real GitHub
// returns 404 for these to anyone outside the org, never exposing the team
// structure. A member sees them.
func TestTeamVisibilityNonMember(t *testing.T) {
	s := newTestServer()
	s.registerGHTeamRoutes()
	s.registerGHMemberRoutes()

	admin := s.store.LookupUserByLogin("admin")
	if s.store.CreateOrg(admin, "team-org", "Team Org", "") == nil {
		t.Fatal("CreateOrg nil")
	}
	team := s.store.CreateTeam("team-org", "Secret Squad", TeamOptions{})
	if team == nil {
		t.Fatal("CreateTeam nil")
	}

	outsider := seedTestUser(s, "team-outsider")
	outTok := s.store.CreateToken(outsider.ID, "read:org")

	gated := []string{
		"/api/v3/orgs/team-org/teams",
		"/api/v3/orgs/team-org/teams/secret-squad",
		"/api/v3/orgs/team-org/teams/secret-squad/members",
		"/api/v3/orgs/team-org/teams/secret-squad/repos",
	}
	for _, p := range gated {
		w := tokenRequest(s, "GET", p, outTok.Value)
		if w.Code != http.StatusNotFound {
			t.Errorf("outsider GET %s = %d, want 404 (body=%s)", p, w.Code, w.Body.String())
		}
		// Member (admin) is allowed.
		wa := tokenRequest(s, "GET", p, AdminToken())
		if wa.Code == http.StatusNotFound {
			t.Errorf("member GET %s = 404, want visible", p)
		}
	}

	// The team list, for a member, includes the team.
	w := tokenRequest(s, "GET", "/api/v3/orgs/team-org/teams", AdminToken())
	var teams []map[string]any
	json.Unmarshal(w.Body.Bytes(), &teams)
	if len(teams) == 0 {
		t.Error("member team list empty, want the seeded team")
	}
}
