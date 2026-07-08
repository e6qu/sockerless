package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitHub organization issue types: named, colored classifications (Bug, Epic,
// Task, ...) an organization defines once and assigns to issues in any of its
// repositories.

// IssueType is an organization-level issue type definition.
type IssueType struct {
	ID          int       `json:"id"`
	NodeID      string    `json:"node_id"`
	OrgLogin    string    `json:"org_login"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Color       *string   `json:"color"`
	IsEnabled   bool      `json:"is_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Server) registerGHIssueTypeRoutes() {
	s.route("GET /api/v3/orgs/{org}/issue-types",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOrgIssueTypes)))
	s.route("POST /api/v3/orgs/{org}/issue-types",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleCreateOrgIssueType)))
	s.route("PUT /api/v3/orgs/{org}/issue-types/{issue_type_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleUpdateOrgIssueType)))
	s.route("DELETE /api/v3/orgs/{org}/issue-types/{issue_type_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteOrgIssueType)))
}

// writeGHValidationErrorSimple writes GitHub's validation-error-simple shape
// (a bare string per error) that the issue-types / issue-fields endpoints
// document as their 422 response.
func writeGHValidationErrorSimple(w http.ResponseWriter, errs ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":           "Validation Failed",
		"documentation_url": "https://docs.github.com/rest",
		"errors":            errs,
	})
}

var issueTypeColors = map[string]bool{
	"gray": true, "blue": true, "green": true, "yellow": true,
	"orange": true, "red": true, "pink": true, "purple": true,
}

func (s *Server) handleListOrgIssueTypes(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	types := s.store.ListIssueTypes(org)
	out := make([]map[string]interface{}, 0, len(types))
	for _, it := range types {
		out = append(out, issueTypeJSON(it))
	}
	writeJSON(w, http.StatusOK, out)
}

type issueTypeRequest struct {
	Name        *string `json:"name"`
	IsEnabled   *bool   `json:"is_enabled"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
}

func (req *issueTypeRequest) validate(w http.ResponseWriter) bool {
	if req.Name == nil || *req.Name == "" {
		writeGHValidationErrorSimple(w, "name is required")
		return false
	}
	if req.IsEnabled == nil {
		writeGHValidationErrorSimple(w, "is_enabled is required")
		return false
	}
	if req.Color != nil && !issueTypeColors[*req.Color] {
		writeGHValidationErrorSimple(w, fmt.Sprintf("color %q is not a supported issue type color", *req.Color))
		return false
	}
	return true
}

func (s *Server) handleCreateOrgIssueType(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req issueTypeRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !req.validate(w) {
		return
	}
	it := s.store.CreateIssueType(org, *req.Name, req.Description, req.Color, *req.IsEnabled)
	writeJSON(w, http.StatusOK, issueTypeJSON(it))
}

func (s *Server) handleUpdateOrgIssueType(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	id, err := strconv.Atoi(r.PathValue("issue_type_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req issueTypeRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !req.validate(w) {
		return
	}
	it := s.store.UpdateIssueType(org, id, *req.Name, req.Description, req.Color, *req.IsEnabled)
	if it == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, issueTypeJSON(it))
}

func (s *Server) handleDeleteOrgIssueType(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	id, err := strconv.Atoi(r.PathValue("issue_type_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteIssueType(org, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func issueTypeJSON(it *IssueType) map[string]interface{} {
	var desc interface{}
	if it.Description != nil {
		desc = *it.Description
	}
	var color interface{}
	if it.Color != nil {
		color = *it.Color
	}
	return map[string]interface{}{
		"id":          it.ID,
		"node_id":     it.NodeID,
		"name":        it.Name,
		"description": desc,
		"color":       color,
		"is_enabled":  it.IsEnabled,
		"created_at":  it.CreatedAt.Format(time.RFC3339),
		"updated_at":  it.UpdatedAt.Format(time.RFC3339),
	}
}

// --- store ---

func orgLoginForIssueTypeRepo(repo *Repo) string {
	if repo == nil || repo.OwnerType != "Organization" {
		return ""
	}
	owner, _, ok := strings.Cut(repo.FullName, "/")
	if !ok {
		return ""
	}
	return owner
}

// ListIssueTypes returns the org's issue types sorted by ID.
func (st *Store) ListIssueTypes(orgLogin string) []*IssueType {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgIssueTypes[orgLogin]
	out := make([]*IssueType, 0, len(m))
	for _, it := range m {
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetAssignableIssueTypeForRepo returns an enabled issue type owned by the
// repository's organization. User-owned repositories do not have issue types.
func (st *Store) GetAssignableIssueTypeForRepo(repo *Repo, id int) *IssueType {
	if id <= 0 {
		return nil
	}
	orgLogin := orgLoginForIssueTypeRepo(repo)
	if orgLogin == "" {
		return nil
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	it := st.OrgIssueTypes[orgLogin][id]
	if it == nil || !it.IsEnabled {
		return nil
	}
	return it
}

// issueTypeForIssueLocked resolves the issue's assigned type while st.mu is
// held. It returns nil when the repo no longer resolves, the repo is not owned
// by an organization, or the assigned definition was removed.
func (st *Store) issueTypeForIssueLocked(issue *Issue) *IssueType {
	if issue == nil || issue.IssueTypeID == 0 {
		return nil
	}
	repo := st.Repos[issue.RepoID]
	orgLogin := orgLoginForIssueTypeRepo(repo)
	if orgLogin == "" {
		return nil
	}
	return st.OrgIssueTypes[orgLogin][issue.IssueTypeID]
}

func findIssueTypeByNodeID(st *Store, nodeID string) *IssueType {
	if nodeID == "" {
		return nil
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, types := range st.OrgIssueTypes {
		for _, it := range types {
			if it.NodeID == nodeID {
				return it
			}
		}
	}
	return nil
}

// CreateIssueType creates a new organization issue type.
func (st *Store) CreateIssueType(orgLogin, name string, description, color *string, isEnabled bool) *IssueType {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().UTC()
	it := &IssueType{
		ID:          st.NextIssueTypeID,
		NodeID:      fmt.Sprintf("IT_kwDO%08d", st.NextIssueTypeID),
		OrgLogin:    orgLogin,
		Name:        name,
		Description: description,
		Color:       color,
		IsEnabled:   isEnabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextIssueTypeID++
	if st.OrgIssueTypes[orgLogin] == nil {
		st.OrgIssueTypes[orgLogin] = map[int]*IssueType{}
	}
	st.OrgIssueTypes[orgLogin][it.ID] = it
	if st.persist != nil {
		st.persist.MustPut("org_issue_types", orgLogin, st.OrgIssueTypes[orgLogin])
	}
	return it
}

// UpdateIssueType replaces the mutable fields of an issue type.
// Returns nil when the issue type does not exist in the org.
func (st *Store) UpdateIssueType(orgLogin string, id int, name string, description, color *string, isEnabled bool) *IssueType {
	st.mu.Lock()
	defer st.mu.Unlock()
	it := st.OrgIssueTypes[orgLogin][id]
	if it == nil {
		return nil
	}
	it.Name = name
	it.Description = description
	it.Color = color
	it.IsEnabled = isEnabled
	it.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("org_issue_types", orgLogin, st.OrgIssueTypes[orgLogin])
	}
	return it
}

// DeleteIssueType removes an issue type. Returns true when it existed.
func (st *Store) DeleteIssueType(orgLogin string, id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgIssueTypes[orgLogin][id] == nil {
		return false
	}
	delete(st.OrgIssueTypes[orgLogin], id)
	if st.persist != nil {
		st.persist.MustPut("org_issue_types", orgLogin, st.OrgIssueTypes[orgLogin])
	}
	return true
}
