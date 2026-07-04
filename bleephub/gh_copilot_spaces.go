package bleephub

// GitHub Copilot Spaces REST surface, in both its organization-scoped
// (/orgs/{org}/copilot-spaces…) and user-scoped
// (/users/{username}/copilot-spaces…) flavors: space CRUD, collaborator
// management, and attached resources.

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHCopilotSpacesRoutes() {
	for _, prefix := range []string{"/api/v3/orgs/{org}/copilot-spaces", "/api/v3/users/{username}/copilot-spaces"} {
		s.route("GET "+prefix, s.handleListCopilotSpaces)
		s.route("POST "+prefix, s.handleCreateCopilotSpace)
		s.route("GET "+prefix+"/{space_number}", s.handleGetCopilotSpace)
		s.route("PUT "+prefix+"/{space_number}", s.handleUpdateCopilotSpace)
		s.route("DELETE "+prefix+"/{space_number}", s.handleDeleteCopilotSpace)
		s.route("GET "+prefix+"/{space_number}/collaborators", s.handleListCopilotSpaceCollaborators)
		s.route("POST "+prefix+"/{space_number}/collaborators", s.handleAddCopilotSpaceCollaborator)
		s.route("PUT "+prefix+"/{space_number}/collaborators/{actor_type}/{actor_identifier}", s.handleUpdateCopilotSpaceCollaborator)
		s.route("DELETE "+prefix+"/{space_number}/collaborators/{actor_type}/{actor_identifier}", s.handleRemoveCopilotSpaceCollaborator)
		s.route("GET "+prefix+"/{space_number}/resources", s.handleListCopilotSpaceResources)
		s.route("POST "+prefix+"/{space_number}/resources", s.handleCreateCopilotSpaceResource)
		s.route("GET "+prefix+"/{space_number}/resources/{space_resource_id}", s.handleGetCopilotSpaceResource)
		s.route("PUT "+prefix+"/{space_number}/resources/{space_resource_id}", s.handleUpdateCopilotSpaceResource)
		s.route("DELETE "+prefix+"/{space_number}/resources/{space_resource_id}", s.handleDeleteCopilotSpaceResource)
	}
}

var copilotSpaceRoleRank = map[string]int{"reader": 1, "writer": 2, "admin": 3}

// copilotSpaceOwner resolves the owner coordinates from the request
// path: {org} for the organization flavor, {username} for the user
// flavor. Writes a 404 and returns ok=false when the owner account does
// not exist.
func (s *Server) copilotSpaceOwner(w http.ResponseWriter, r *http.Request) (ownerType, ownerLogin string, ok bool) {
	if org := r.PathValue("org"); org != "" {
		if s.store.GetOrg(org) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return "", "", false
		}
		return "Organization", org, true
	}
	username := r.PathValue("username")
	if s.store.LookupUserByLogin(username) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return "", "", false
	}
	return "User", username, true
}

// copilotSpaceRole computes the caller's effective role on a space:
// space creators, user-space owners, and organization owners are
// admins; collaborator grants (directly or via team membership) and the
// space's base role fill in the rest. An empty role means the space is
// invisible to the caller.
func (s *Server) copilotSpaceRole(user *User, space *CopilotSpace) string {
	if user == nil {
		return ""
	}
	if space.OwnerType == "User" && strings.EqualFold(space.OwnerLogin, user.Login) {
		return "admin"
	}
	if user.ID == space.CreatorID {
		return "admin"
	}
	role := ""
	upgrade := func(candidate string) {
		if copilotSpaceRoleRank[candidate] > copilotSpaceRoleRank[role] {
			role = candidate
		}
	}
	if space.OwnerType == "Organization" {
		if org := s.store.GetOrg(space.OwnerLogin); org != nil {
			if canAdminOrg(s.store, user, org) {
				return "admin"
			}
			if isActiveOrgMember(s.store, user, space.OwnerLogin) && space.BaseRole != "" && space.BaseRole != "no_access" {
				upgrade(space.BaseRole)
			}
		}
	} else if space.BaseRole == "reader" {
		upgrade("reader")
	}
	for _, c := range space.Collaborators {
		switch c.ActorType {
		case "User":
			if c.UserID == user.ID {
				upgrade(c.Role)
			}
		case "Team":
			if team := s.store.GetTeamByID(c.TeamID); team != nil {
				if _, ok := s.store.GetTeamMembership(space.OwnerLogin, team.Slug, user.ID); ok {
					upgrade(c.Role)
				}
			}
		}
	}
	return role
}

// lookupCopilotSpace resolves the owner and {space_number} to a space
// visible to the caller, writing 401/404 on failure. minRole gates the
// operation: a caller who can see the space but lacks the role gets 403.
func (s *Server) lookupCopilotSpace(w http.ResponseWriter, r *http.Request, minRole string) (*CopilotSpace, *User) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return nil, nil
	}
	ownerType, ownerLogin, ok := s.copilotSpaceOwner(w, r)
	if !ok {
		return nil, nil
	}
	number, err := strconv.Atoi(r.PathValue("space_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	space := s.store.GetCopilotSpace(ownerType, ownerLogin, number)
	if space == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	role := s.copilotSpaceRole(user, space)
	if role == "" {
		// Spaces the caller has no access to stay hidden, like private repos.
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	if copilotSpaceRoleRank[role] < copilotSpaceRoleRank[minRole] {
		writeGHError(w, http.StatusForbidden,
			fmt.Sprintf("Must have %s access to this Copilot Space.", minRole))
		return nil, nil
	}
	return space, user
}

func (s *Server) copilotSpaceJSON(space *CopilotSpace, baseURL string) map[string]interface{} {
	var owner interface{}
	var apiURL string
	if space.OwnerType == "Organization" {
		if org := s.store.GetOrg(space.OwnerLogin); org != nil {
			owner = orgSimpleJSON(org, baseURL)
		}
		apiURL = fmt.Sprintf("%s/api/v3/orgs/%s/copilot-spaces/%d", baseURL, space.OwnerLogin, space.Number)
	} else {
		if u := s.store.LookupUserByLogin(space.OwnerLogin); u != nil {
			owner = userToJSON(u)
		}
		apiURL = fmt.Sprintf("%s/api/v3/users/%s/copilot-spaces/%d", baseURL, space.OwnerLogin, space.Number)
	}
	var creator interface{}
	if u := s.store.GetUserByID(space.CreatorID); u != nil {
		creator = userToJSON(u)
	}
	var description interface{}
	if space.Description != "" {
		description = space.Description
	}
	var instructions interface{}
	if space.GeneralInstructions != "" {
		instructions = space.GeneralInstructions
	}
	resources := make([]map[string]interface{}, 0, len(space.Resources))
	for _, res := range space.Resources {
		resources = append(resources, copilotSpaceResourceJSON(res))
	}
	return map[string]interface{}{
		"id":                   space.ID,
		"number":               space.Number,
		"name":                 space.Name,
		"description":          description,
		"general_instructions": instructions,
		"base_role":            space.BaseRole,
		"owner":                owner,
		"creator":              creator,
		"created_at":           space.CreatedAt.Format(time.RFC3339),
		"updated_at":           space.UpdatedAt.Format(time.RFC3339),
		"html_url":             fmt.Sprintf("%s/copilot/spaces/%s/%d", baseURL, space.OwnerLogin, space.Number),
		"api_url":              apiURL,
		"resources_attributes": resources,
	}
}

func copilotSpaceResourceJSON(res *CopilotSpaceResource) map[string]interface{} {
	return map[string]interface{}{
		"id":            res.ID,
		"resource_type": res.ResourceType,
		// Chat attachments (uploaded files, media) are not part of the
		// REST create surface, so no resource carries an attachment.
		"copilot_chat_attachment_id": nil,
		"metadata":                   res.Metadata,
		"created_at":                 res.CreatedAt.Format(time.RFC3339),
		"updated_at":                 res.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *Server) copilotSpaceCollaboratorJSON(c *CopilotSpaceCollaborator, space *CopilotSpace, baseURL string) map[string]interface{} {
	if c.ActorType == "User" {
		u := s.store.GetUserByID(c.UserID)
		if u == nil {
			return nil
		}
		out := userToJSON(u)
		out["actor_type"] = "User"
		out["role"] = c.Role
		return out
	}
	team := s.store.GetTeamByID(c.TeamID)
	org := s.store.GetOrg(space.OwnerLogin)
	if team == nil || org == nil {
		return nil
	}
	api := baseURL + "/api/v3/orgs/" + org.Login + "/teams/" + team.Slug
	var description interface{}
	if team.Description != "" {
		description = team.Description
	}
	var parent interface{}
	if team.ParentID != 0 {
		if p := s.store.GetTeamByID(team.ParentID); p != nil {
			parent = teamRefJSON(p, org, baseURL)
		}
	}
	return map[string]interface{}{
		"actor_type":           "Team",
		"role":                 c.Role,
		"id":                   team.ID,
		"node_id":              team.NodeID,
		"name":                 team.Name,
		"slug":                 team.Slug,
		"type":                 "Team",
		"description":          description,
		"privacy":              string(team.Privacy),
		"notification_setting": string(team.NotificationSetting),
		"url":                  api,
		"html_url":             baseURL + "/orgs/" + org.Login + "/teams/" + team.Slug,
		"members_url":          api + "/members{/member}",
		"repositories_url":     api + "/repos",
		"organization_id":      org.ID,
		"parent":               parent,
	}
}

// validCopilotSpaceBaseRole checks the base role against the flavor's
// enum: organization spaces allow reader/writer/admin/no_access, user
// spaces only reader/no_access.
func validCopilotSpaceBaseRole(ownerType, role string) bool {
	switch role {
	case "reader", "no_access":
		return true
	case "writer", "admin":
		return ownerType == "Organization"
	default:
		return false
	}
}

func (s *Server) handleListCopilotSpaces(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	ownerType, ownerLogin, ok := s.copilotSpaceOwner(w, r)
	if !ok {
		return
	}
	var visible []*CopilotSpace
	for _, space := range s.store.ListCopilotSpaces(ownerType, ownerLogin) {
		if s.copilotSpaceRole(user, space) != "" {
			visible = append(visible, space)
		}
	}

	// Cursor pagination over space numbers, matching the endpoint's
	// per_page/before/after parameters.
	q := r.URL.Query()
	perPage := 30
	if v := q.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = min(n, 100)
		}
	}
	if v := q.Get("after"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			kept := visible[:0]
			for _, sp := range visible {
				if sp.Number > n {
					kept = append(kept, sp)
				}
			}
			visible = kept
		}
	}
	if v := q.Get("before"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			kept := visible[:0]
			for _, sp := range visible {
				if sp.Number < n {
					kept = append(kept, sp)
				}
			}
			visible = kept
			// A backward cursor pages from the element just before it.
			if len(visible) > perPage {
				visible = visible[len(visible)-perPage:]
			}
		}
	}
	hasMore := len(visible) > perPage
	if hasMore {
		visible = visible[:perPage]
	}
	if hasMore && len(visible) > 0 {
		next := *r.URL
		nq := next.Query()
		nq.Set("after", strconv.Itoa(visible[len(visible)-1].Number))
		next.RawQuery = nq.Encode()
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next.String()))
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(visible))
	for _, space := range visible {
		out = append(out, s.copilotSpaceJSON(space, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"spaces": out})
}

func (s *Server) handleCreateCopilotSpace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	ownerType, ownerLogin, ok := s.copilotSpaceOwner(w, r)
	if !ok {
		return
	}
	if ownerType == "Organization" {
		// Organizations hide their internal structure from non-members.
		if !isActiveOrgMember(s.store, user, ownerLogin) {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	} else if !strings.EqualFold(ownerLogin, user.Login) {
		writeGHError(w, http.StatusForbidden, "Copilot Spaces can only be created for the authenticated user.")
		return
	}
	var req struct {
		Name                string  `json:"name"`
		Description         string  `json:"description"`
		GeneralInstructions string  `json:"general_instructions"`
		BaseRole            *string `json:"base_role"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeGHValidationError(w, "CopilotSpace", "name", "missing_field")
		return
	}
	if len(req.GeneralInstructions) > 4000 {
		writeGHValidationError(w, "CopilotSpace", "general_instructions", "invalid")
		return
	}
	baseRole := "no_access"
	if req.BaseRole != nil {
		if !validCopilotSpaceBaseRole(ownerType, *req.BaseRole) {
			writeGHValidationError(w, "CopilotSpace", "base_role", "invalid")
			return
		}
		baseRole = *req.BaseRole
	}
	space := s.store.CreateCopilotSpace(ownerType, ownerLogin, user.ID, req.Name, req.Description, req.GeneralInstructions, baseRole)
	writeJSON(w, http.StatusCreated, s.copilotSpaceJSON(space, s.baseURL(r)))
}

func (s *Server) handleGetCopilotSpace(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "reader")
	if space == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.copilotSpaceJSON(space, s.baseURL(r)))
}

func (s *Server) handleUpdateCopilotSpace(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "admin")
	if space == nil {
		return
	}
	var req struct {
		Name                *string `json:"name"`
		Description         *string `json:"description"`
		GeneralInstructions *string `json:"general_instructions"`
		BaseRole            *string `json:"base_role"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		writeGHValidationError(w, "CopilotSpace", "name", "missing_field")
		return
	}
	if req.GeneralInstructions != nil && len(*req.GeneralInstructions) > 4000 {
		writeGHValidationError(w, "CopilotSpace", "general_instructions", "invalid")
		return
	}
	if req.BaseRole != nil && !validCopilotSpaceBaseRole(space.OwnerType, *req.BaseRole) {
		writeGHValidationError(w, "CopilotSpace", "base_role", "invalid")
		return
	}
	if req.Name != nil {
		space.Name = *req.Name
	}
	if req.Description != nil {
		space.Description = *req.Description
	}
	if req.GeneralInstructions != nil {
		space.GeneralInstructions = *req.GeneralInstructions
	}
	if req.BaseRole != nil {
		space.BaseRole = *req.BaseRole
	}
	s.store.SaveCopilotSpace(space)
	writeJSON(w, http.StatusOK, s.copilotSpaceJSON(space, s.baseURL(r)))
}

func (s *Server) handleDeleteCopilotSpace(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "admin")
	if space == nil {
		return
	}
	s.store.DeleteCopilotSpace(space.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListCopilotSpaceCollaborators(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "reader")
	if space == nil {
		return
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(space.Collaborators))
	for _, c := range space.Collaborators {
		if j := s.copilotSpaceCollaboratorJSON(c, space, base); j != nil {
			out = append(out, j)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"collaborators": out})
}

// resolveCopilotSpaceActor resolves an actor_type + actor_identifier
// pair (username / team slug, or a numeric ID) against the store. Teams
// must belong to the organization that owns the space.
func (s *Server) resolveCopilotSpaceActor(space *CopilotSpace, actorType, identifier string) (userID, teamID int, ok bool) {
	switch actorType {
	case "User":
		if u := s.store.LookupUserByLogin(identifier); u != nil {
			return u.ID, 0, true
		}
		if id, err := strconv.Atoi(identifier); err == nil {
			if u := s.store.GetUserByID(id); u != nil {
				return u.ID, 0, true
			}
		}
	case "Team":
		if space.OwnerType != "Organization" {
			return 0, 0, false
		}
		org := s.store.GetOrg(space.OwnerLogin)
		if org == nil {
			return 0, 0, false
		}
		if t := s.store.GetTeam(org.Login, identifier); t != nil {
			return 0, t.ID, true
		}
		if id, err := strconv.Atoi(identifier); err == nil {
			if t := s.store.GetTeamByID(id); t != nil && t.OrgID == org.ID {
				return 0, t.ID, true
			}
		}
	}
	return 0, 0, false
}

func copilotSpaceCollaboratorIndex(space *CopilotSpace, userID, teamID int) int {
	for i, c := range space.Collaborators {
		if c.UserID == userID && c.TeamID == teamID {
			return i
		}
	}
	return -1
}

func (s *Server) handleAddCopilotSpaceCollaborator(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "admin")
	if space == nil {
		return
	}
	var req struct {
		ActorType       string `json:"actor_type"`
		ActorIdentifier string `json:"actor_identifier"`
		Role            string `json:"role"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ActorType != "User" && req.ActorType != "Team" {
		writeGHValidationError(w, "CopilotSpaceCollaborator", "actor_type", "invalid")
		return
	}
	if req.Role != "reader" && req.Role != "writer" && req.Role != "admin" {
		writeGHValidationError(w, "CopilotSpaceCollaborator", "role", "invalid")
		return
	}
	userID, teamID, ok := s.resolveCopilotSpaceActor(space, req.ActorType, req.ActorIdentifier)
	if !ok {
		writeGHValidationError(w, "CopilotSpaceCollaborator", "actor_identifier", "invalid")
		return
	}
	if req.ActorType == "User" && space.OwnerType == "Organization" {
		if u := s.store.GetUserByID(userID); u == nil || !isActiveOrgMember(s.store, u, space.OwnerLogin) {
			writeGHValidationError(w, "CopilotSpaceCollaborator", "actor_identifier", "invalid")
			return
		}
	}
	collab := &CopilotSpaceCollaborator{ActorType: req.ActorType, UserID: userID, TeamID: teamID, Role: req.Role}
	if i := copilotSpaceCollaboratorIndex(space, userID, teamID); i >= 0 {
		space.Collaborators[i] = collab
	} else {
		space.Collaborators = append(space.Collaborators, collab)
	}
	s.store.SaveCopilotSpace(space)
	writeJSON(w, http.StatusCreated, s.copilotSpaceCollaboratorJSON(collab, space, s.baseURL(r)))
}

// copilotSpaceActorFromPath resolves the {actor_type}/{actor_identifier}
// path segments, writing a 404 when the actor does not resolve.
func (s *Server) copilotSpaceActorFromPath(w http.ResponseWriter, r *http.Request, space *CopilotSpace) (userID, teamID int, ok bool) {
	actorType := r.PathValue("actor_type")
	if actorType != "User" && actorType != "Team" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return 0, 0, false
	}
	userID, teamID, ok = s.resolveCopilotSpaceActor(space, actorType, r.PathValue("actor_identifier"))
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return 0, 0, false
	}
	return userID, teamID, true
}

func (s *Server) handleUpdateCopilotSpaceCollaborator(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "admin")
	if space == nil {
		return
	}
	userID, teamID, ok := s.copilotSpaceActorFromPath(w, r, space)
	if !ok {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.Role {
	case "no_access":
		if i := copilotSpaceCollaboratorIndex(space, userID, teamID); i >= 0 {
			space.Collaborators = append(space.Collaborators[:i], space.Collaborators[i+1:]...)
			s.store.SaveCopilotSpace(space)
		}
		w.WriteHeader(http.StatusNoContent)
	case "reader", "writer", "admin":
		collab := &CopilotSpaceCollaborator{ActorType: r.PathValue("actor_type"), UserID: userID, TeamID: teamID, Role: req.Role}
		if i := copilotSpaceCollaboratorIndex(space, userID, teamID); i >= 0 {
			space.Collaborators[i] = collab
		} else {
			space.Collaborators = append(space.Collaborators, collab)
		}
		s.store.SaveCopilotSpace(space)
		writeJSON(w, http.StatusOK, s.copilotSpaceCollaboratorJSON(collab, space, s.baseURL(r)))
	default:
		writeGHValidationError(w, "CopilotSpaceCollaborator", "role", "invalid")
	}
}

func (s *Server) handleRemoveCopilotSpaceCollaborator(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "admin")
	if space == nil {
		return
	}
	userID, teamID, ok := s.copilotSpaceActorFromPath(w, r, space)
	if !ok {
		return
	}
	i := copilotSpaceCollaboratorIndex(space, userID, teamID)
	if i < 0 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	space.Collaborators = append(space.Collaborators[:i], space.Collaborators[i+1:]...)
	s.store.SaveCopilotSpace(space)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListCopilotSpaceResources(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "reader")
	if space == nil {
		return
	}
	out := make([]map[string]interface{}, 0, len(space.Resources))
	for _, res := range space.Resources {
		out = append(out, copilotSpaceResourceJSON(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"resources": out})
}

// validateCopilotSpaceResource checks the metadata a resource type
// requires: repository-backed types must reference an existing
// repository (plus a file path, or an issue / pull request number where
// applicable), and free text must carry the text itself. Returns a
// non-empty field name on failure.
func (s *Server) validateCopilotSpaceResource(resourceType string, metadata map[string]interface{}) string {
	requireRepo := func() string {
		id, ok := metadata["repository_id"].(float64)
		if !ok {
			return "metadata.repository_id"
		}
		s.store.mu.RLock()
		repo := s.store.Repos[int(id)]
		s.store.mu.RUnlock()
		if repo == nil {
			return "metadata.repository_id"
		}
		return ""
	}
	switch resourceType {
	case "repository":
		return requireRepo()
	case "github_file":
		if f := requireRepo(); f != "" {
			return f
		}
		if v, ok := metadata["file_path"].(string); !ok || v == "" {
			return "metadata.file_path"
		}
	case "github_issue", "github_pull_request":
		if f := requireRepo(); f != "" {
			return f
		}
		if _, ok := metadata["number"].(float64); !ok {
			return "metadata.number"
		}
	case "free_text":
		if v, ok := metadata["text"].(string); !ok || v == "" {
			return "metadata.text"
		}
	}
	return ""
}

func (s *Server) handleCreateCopilotSpaceResource(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "writer")
	if space == nil {
		return
	}
	var req struct {
		ResourceType string                 `json:"resource_type"`
		Metadata     map[string]interface{} `json:"metadata"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.ResourceType {
	case "repository", "github_file", "free_text", "github_issue", "github_pull_request":
	default:
		writeGHValidationError(w, "CopilotSpaceResource", "resource_type", "invalid")
		return
	}
	if req.Metadata == nil {
		writeGHValidationError(w, "CopilotSpaceResource", "metadata", "missing_field")
		return
	}
	if field := s.validateCopilotSpaceResource(req.ResourceType, req.Metadata); field != "" {
		writeGHValidationError(w, "CopilotSpaceResource", field, "invalid")
		return
	}
	// Attaching an identical resource again returns the existing one.
	for _, res := range space.Resources {
		if res.ResourceType == req.ResourceType && reflect.DeepEqual(res.Metadata, req.Metadata) {
			writeJSON(w, http.StatusOK, copilotSpaceResourceJSON(res))
			return
		}
	}
	now := time.Now().UTC()
	res := &CopilotSpaceResource{
		ID:           space.NextResourceID,
		ResourceType: req.ResourceType,
		Metadata:     req.Metadata,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	space.NextResourceID++
	space.Resources = append(space.Resources, res)
	s.store.SaveCopilotSpace(space)
	writeJSON(w, http.StatusCreated, copilotSpaceResourceJSON(res))
}

// copilotSpaceResourceFromPath resolves {space_resource_id}, writing a
// 404 when the resource does not exist on the space.
func copilotSpaceResourceFromPath(w http.ResponseWriter, r *http.Request, space *CopilotSpace) *CopilotSpaceResource {
	id, err := strconv.Atoi(r.PathValue("space_resource_id"))
	if err == nil {
		for _, res := range space.Resources {
			if res.ID == id {
				return res
			}
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
	return nil
}

func (s *Server) handleGetCopilotSpaceResource(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "reader")
	if space == nil {
		return
	}
	res := copilotSpaceResourceFromPath(w, r, space)
	if res == nil {
		return
	}
	writeJSON(w, http.StatusOK, copilotSpaceResourceJSON(res))
}

func (s *Server) handleUpdateCopilotSpaceResource(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "writer")
	if space == nil {
		return
	}
	res := copilotSpaceResourceFromPath(w, r, space)
	if res == nil {
		return
	}
	var req struct {
		Metadata map[string]interface{} `json:"metadata"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Metadata != nil {
		if field := s.validateCopilotSpaceResource(res.ResourceType, req.Metadata); field != "" {
			writeGHValidationError(w, "CopilotSpaceResource", field, "invalid")
			return
		}
		res.Metadata = req.Metadata
	}
	res.UpdatedAt = time.Now().UTC()
	s.store.SaveCopilotSpace(space)
	writeJSON(w, http.StatusOK, copilotSpaceResourceJSON(res))
}

func (s *Server) handleDeleteCopilotSpaceResource(w http.ResponseWriter, r *http.Request) {
	space, _ := s.lookupCopilotSpace(w, r, "writer")
	if space == nil {
		return
	}
	res := copilotSpaceResourceFromPath(w, r, space)
	if res == nil {
		return
	}
	for i, existing := range space.Resources {
		if existing.ID == res.ID {
			space.Resources = append(space.Resources[:i], space.Resources[i+1:]...)
			break
		}
	}
	s.store.SaveCopilotSpace(space)
	w.WriteHeader(http.StatusNoContent)
}
