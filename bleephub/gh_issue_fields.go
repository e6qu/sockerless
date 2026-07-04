package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitHub organization issue fields: custom attributes (text, number, date,
// single select, multi select) defined at the organization level and assigned
// per-issue via the issue-field-values endpoints.

// IssueField is an organization-level issue field definition.
type IssueField struct {
	ID          int                 `json:"id"`
	NodeID      string              `json:"node_id"`
	OrgLogin    string              `json:"org_login"`
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	DataType    string              `json:"data_type"`
	Visibility  string              `json:"visibility"`
	Options     []*IssueFieldOption `json:"options,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// IssueFieldOption is one selectable option of a single/multi select field.
type IssueFieldOption struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Color       string    `json:"color"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Server) registerGHIssueFieldRoutes() {
	s.route("GET /api/v3/orgs/{org}/issue-fields",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOrgIssueFields)))
	s.route("POST /api/v3/orgs/{org}/issue-fields",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleCreateOrgIssueField)))
	s.route("PATCH /api/v3/orgs/{org}/issue-fields/{issue_field_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleUpdateOrgIssueField)))
	s.route("DELETE /api/v3/orgs/{org}/issue-fields/{issue_field_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteOrgIssueField)))

	// Per-issue field values. The GET route dispatches through the shared
	// two-segment issue GET handler (gh_labels_rest.go).
	s.route("POST /api/v3/repos/{owner}/{repo}/issues/{number}/issue-field-values",
		s.requirePerm(scopeIssues, permWrite, s.handleAddIssueFieldValues))
	s.route("PUT /api/v3/repos/{owner}/{repo}/issues/{number}/issue-field-values",
		s.requirePerm(scopeIssues, permWrite, s.handleSetIssueFieldValues))
}

var issueFieldDataTypes = map[string]bool{
	"text": true, "date": true, "single_select": true, "multi_select": true, "number": true,
}

func issueFieldIsSelect(dataType string) bool {
	return dataType == "single_select" || dataType == "multi_select"
}

func (s *Server) handleListOrgIssueFields(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	fields := s.store.ListIssueFields(org)
	out := make([]map[string]interface{}, 0, len(fields))
	for _, f := range fields {
		out = append(out, issueFieldJSON(f))
	}
	writeJSON(w, http.StatusOK, out)
}

type issueFieldOptionRequest struct {
	ID          *int    `json:"id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Priority    *int    `json:"priority"`
}

func (s *Server) handleCreateOrgIssueField(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		Name        *string                   `json:"name"`
		Description *string                   `json:"description"`
		DataType    *string                   `json:"data_type"`
		Visibility  *string                   `json:"visibility"`
		Options     []issueFieldOptionRequest `json:"options"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == nil || *req.Name == "" {
		writeGHValidationErrorSimple(w, "name is required")
		return
	}
	if req.DataType == nil || !issueFieldDataTypes[*req.DataType] {
		writeGHValidationErrorSimple(w, "data_type must be one of text, date, single_select, multi_select, number")
		return
	}
	visibility := "organization_members_only"
	if req.Visibility != nil {
		if *req.Visibility != "organization_members_only" && *req.Visibility != "all" {
			writeGHValidationErrorSimple(w, "visibility must be organization_members_only or all")
			return
		}
		visibility = *req.Visibility
	}
	if issueFieldIsSelect(*req.DataType) && len(req.Options) == 0 {
		writeGHValidationErrorSimple(w, "options are required for single_select and multi_select fields")
		return
	}
	if !issueFieldIsSelect(*req.DataType) && len(req.Options) > 0 {
		writeGHValidationErrorSimple(w, "options are only supported for single_select and multi_select fields")
		return
	}
	for _, opt := range req.Options {
		if opt.Name == nil || *opt.Name == "" {
			writeGHValidationErrorSimple(w, "option name is required")
			return
		}
		if opt.Color == nil || !issueTypeColors[*opt.Color] {
			writeGHValidationErrorSimple(w, "option color is required and must be a supported color")
			return
		}
	}
	f := s.store.CreateIssueField(org, *req.Name, req.Description, *req.DataType, visibility, req.Options)
	writeJSON(w, http.StatusOK, issueFieldJSON(f))
}

func (s *Server) handleUpdateOrgIssueField(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	id, err := strconv.Atoi(r.PathValue("issue_field_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Name        *string                   `json:"name"`
		Description *string                   `json:"description"`
		Visibility  *string                   `json:"visibility"`
		Options     []issueFieldOptionRequest `json:"options"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Visibility != nil && *req.Visibility != "organization_members_only" && *req.Visibility != "all" {
		writeGHValidationErrorSimple(w, "visibility must be organization_members_only or all")
		return
	}
	existing := s.store.GetIssueField(org, id)
	if existing == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if req.Options != nil && !issueFieldIsSelect(existing.DataType) {
		writeGHValidationErrorSimple(w, "options are only supported for single_select and multi_select fields")
		return
	}
	for _, opt := range req.Options {
		if opt.Name == nil || *opt.Name == "" {
			writeGHValidationErrorSimple(w, "option name is required")
			return
		}
		if opt.Color == nil || !issueTypeColors[*opt.Color] {
			writeGHValidationErrorSimple(w, "option color is required and must be a supported color")
			return
		}
		if opt.Priority == nil {
			writeGHValidationErrorSimple(w, "option priority is required")
			return
		}
	}
	f := s.store.UpdateIssueField(org, id, req.Name, req.Description, req.Visibility, req.Options)
	if f == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, issueFieldJSON(f))
}

func (s *Server) handleDeleteOrgIssueField(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	id, err := strconv.Atoi(r.PathValue("issue_field_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteIssueField(org, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func issueFieldJSON(f *IssueField) map[string]interface{} {
	var desc interface{}
	if f.Description != nil {
		desc = *f.Description
	}
	out := map[string]interface{}{
		"id":          f.ID,
		"node_id":     f.NodeID,
		"name":        f.Name,
		"description": desc,
		"data_type":   f.DataType,
		"visibility":  f.Visibility,
		"created_at":  f.CreatedAt.Format(time.RFC3339),
		"updated_at":  f.UpdatedAt.Format(time.RFC3339),
	}
	if issueFieldIsSelect(f.DataType) {
		opts := make([]map[string]interface{}, 0, len(f.Options))
		for _, opt := range f.Options {
			var optDesc interface{}
			if opt.Description != nil {
				optDesc = *opt.Description
			}
			opts = append(opts, map[string]interface{}{
				"id":          opt.ID,
				"name":        opt.Name,
				"description": optDesc,
				"color":       opt.Color,
				"priority":    opt.Priority,
				"created_at":  opt.CreatedAt.Format(time.RFC3339),
				"updated_at":  opt.UpdatedAt.Format(time.RFC3339),
			})
		}
		out["options"] = opts
	}
	return out
}

// --- per-issue field values ---

// resolveIssueForFieldValues resolves the repo + issue for the
// issue-field-values endpoints, writing the error response on failure.
func (s *Server) resolveIssueForFieldValues(w http.ResponseWriter, r *http.Request) (*Repo, *Issue, bool) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	if !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	issue := s.store.GetIssueByNumber(repo.ID, number)
	if issue == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	return repo, issue, true
}

// issueFieldsOrg returns the org login owning the repo, or "" for a
// user-owned repo (which has no organization issue fields).
func issueFieldsOrg(st *Store, repo *Repo) string {
	orgLogin, _, _ := strings.Cut(repo.FullName, "/")
	if st.GetOrg(orgLogin) == nil {
		return ""
	}
	return orgLogin
}

func (s *Server) handleListIssueFieldValues(w http.ResponseWriter, r *http.Request) {
	repo, issue, ok := s.resolveIssueForFieldValues(w, r)
	if !ok {
		return
	}
	org := issueFieldsOrg(s.store, repo)
	values := s.store.ListIssueFieldValues(org, issue.ID)
	values = paginateAndLink(w, r, values)
	writeJSON(w, http.StatusOK, values)
}

type issueFieldValueRequest struct {
	FieldID *int        `json:"field_id"`
	Value   interface{} `json:"value"`
}

func (s *Server) handleAddIssueFieldValues(w http.ResponseWriter, r *http.Request) {
	s.applyIssueFieldValues(w, r, false)
}

func (s *Server) handleSetIssueFieldValues(w http.ResponseWriter, r *http.Request) {
	s.applyIssueFieldValues(w, r, true)
}

// applyIssueFieldValues implements both POST (merge; an empty array clears)
// and PUT (replace) for issue field values.
func (s *Server) applyIssueFieldValues(w http.ResponseWriter, r *http.Request, replace bool) {
	repo, issue, ok := s.resolveIssueForFieldValues(w, r)
	if !ok {
		return
	}
	if !canPushRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to the repository.")
		return
	}
	var req struct {
		IssueFieldValues []issueFieldValueRequest `json:"issue_field_values"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	org := issueFieldsOrg(s.store, repo)

	updates := map[int]interface{}{}
	for _, v := range req.IssueFieldValues {
		if v.FieldID == nil {
			writeGHValidationError(w, "IssueFieldValue", "field_id", "missing_field")
			return
		}
		field := s.store.GetIssueField(org, *v.FieldID)
		if field == nil {
			writeGHValidationError(w, "IssueFieldValue", "field_id", "invalid")
			return
		}
		normalized, err := normalizeIssueFieldValue(field, v.Value)
		if err != nil {
			writeGHValidationErrorSimple(w, err.Error())
			return
		}
		updates[field.ID] = normalized
	}
	// A POST with an empty array clears all existing field values, exactly
	// like a PUT replacing them with nothing.
	if replace || len(req.IssueFieldValues) == 0 {
		s.store.SetIssueFieldValues(issue.ID, updates)
	} else {
		s.store.AddIssueFieldValues(issue.ID, updates)
	}
	writeJSON(w, http.StatusOK, s.store.ListIssueFieldValues(org, issue.ID))
}

// normalizeIssueFieldValue validates a raw JSON value against the field's
// data type and returns the canonical stored representation.
func normalizeIssueFieldValue(field *IssueField, value interface{}) (interface{}, error) {
	switch field.DataType {
	case "text":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %q expects a string value", field.Name)
		}
		return str, nil
	case "number":
		num, ok := value.(float64)
		if !ok {
			return nil, fmt.Errorf("field %q expects a numeric value", field.Name)
		}
		return num, nil
	case "date":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %q expects an ISO 8601 date string", field.Name)
		}
		if _, err := time.Parse("2006-01-02", str); err != nil {
			if _, err := time.Parse(time.RFC3339, str); err != nil {
				return nil, fmt.Errorf("field %q expects an ISO 8601 date string", field.Name)
			}
		}
		return str, nil
	case "single_select":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %q expects an option name", field.Name)
		}
		for _, opt := range field.Options {
			if opt.Name == str {
				return str, nil
			}
		}
		return nil, fmt.Errorf("%q is not an option of field %q", str, field.Name)
	case "multi_select":
		raw, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("field %q expects an array of option names", field.Name)
		}
		names := make([]string, 0, len(raw))
		for _, item := range raw {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("field %q expects an array of option names", field.Name)
			}
			found := false
			for _, opt := range field.Options {
				if opt.Name == str {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%q is not an option of field %q", str, field.Name)
			}
			names = append(names, str)
		}
		return names, nil
	}
	return nil, fmt.Errorf("field %q has unsupported data type %q", field.Name, field.DataType)
}

// --- store ---

// ListIssueFields returns the org's issue fields sorted by ID.
func (st *Store) ListIssueFields(orgLogin string) []*IssueField {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgIssueFields[orgLogin]
	out := make([]*IssueField, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetIssueField returns an issue field by org and ID, or nil.
func (st *Store) GetIssueField(orgLogin string, id int) *IssueField {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgIssueFields[orgLogin][id]
}

// buildIssueFieldOptionsLocked materializes option rows from a request,
// preserving CreatedAt for options carrying an existing option's ID.
func (st *Store) buildIssueFieldOptionsLocked(existing []*IssueFieldOption, reqs []issueFieldOptionRequest) []*IssueFieldOption {
	now := time.Now().UTC()
	byID := map[int]*IssueFieldOption{}
	for _, opt := range existing {
		byID[opt.ID] = opt
	}
	out := make([]*IssueFieldOption, 0, len(reqs))
	for i, req := range reqs {
		priority := i + 1
		if req.Priority != nil {
			priority = *req.Priority
		}
		if req.ID != nil {
			if prev, ok := byID[*req.ID]; ok {
				prev.Name = *req.Name
				prev.Description = req.Description
				prev.Color = *req.Color
				prev.Priority = priority
				prev.UpdatedAt = now
				out = append(out, prev)
				continue
			}
		}
		out = append(out, &IssueFieldOption{
			ID:          st.NextIssueFieldOptionID,
			Name:        *req.Name,
			Description: req.Description,
			Color:       *req.Color,
			Priority:    priority,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		st.NextIssueFieldOptionID++
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

// CreateIssueField creates a new organization issue field.
func (st *Store) CreateIssueField(orgLogin, name string, description *string, dataType, visibility string, options []issueFieldOptionRequest) *IssueField {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().UTC()
	f := &IssueField{
		ID:          st.NextIssueFieldID,
		NodeID:      fmt.Sprintf("IF_kwDO%08d", st.NextIssueFieldID),
		OrgLogin:    orgLogin,
		Name:        name,
		Description: description,
		DataType:    dataType,
		Visibility:  visibility,
		Options:     st.buildIssueFieldOptionsLocked(nil, options),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextIssueFieldID++
	if st.OrgIssueFields[orgLogin] == nil {
		st.OrgIssueFields[orgLogin] = map[int]*IssueField{}
	}
	st.OrgIssueFields[orgLogin][f.ID] = f
	if st.persist != nil {
		st.persist.MustPut("org_issue_fields", orgLogin, st.OrgIssueFields[orgLogin])
	}
	return f
}

// UpdateIssueField applies the provided fields; a non-nil options slice
// replaces the entire option set. Returns nil when the field is unknown.
func (st *Store) UpdateIssueField(orgLogin string, id int, name, description, visibility *string, options []issueFieldOptionRequest) *IssueField {
	st.mu.Lock()
	defer st.mu.Unlock()
	f := st.OrgIssueFields[orgLogin][id]
	if f == nil {
		return nil
	}
	if name != nil {
		f.Name = *name
	}
	if description != nil {
		f.Description = description
	}
	if visibility != nil {
		f.Visibility = *visibility
	}
	if options != nil {
		f.Options = st.buildIssueFieldOptionsLocked(f.Options, options)
	}
	f.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("org_issue_fields", orgLogin, st.OrgIssueFields[orgLogin])
	}
	return f
}

// DeleteIssueField removes an issue field and any per-issue values that
// reference it. Returns true when the field existed.
func (st *Store) DeleteIssueField(orgLogin string, id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgIssueFields[orgLogin][id] == nil {
		return false
	}
	delete(st.OrgIssueFields[orgLogin], id)
	for issueID, values := range st.IssueFieldValues {
		if _, ok := values[id]; ok {
			delete(values, id)
			if st.persist != nil {
				st.persist.MustPut("issue_field_values", strconv.Itoa(issueID), values)
			}
		}
	}
	if st.persist != nil {
		st.persist.MustPut("org_issue_fields", orgLogin, st.OrgIssueFields[orgLogin])
	}
	return true
}

// SetIssueFieldValues replaces all field values on an issue.
func (st *Store) SetIssueFieldValues(issueID int, values map[int]interface{}) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.IssueFieldValues[issueID] = values
	if st.persist != nil {
		st.persist.MustPut("issue_field_values", strconv.Itoa(issueID), values)
	}
}

// AddIssueFieldValues merges field values into an issue's existing set.
func (st *Store) AddIssueFieldValues(issueID int, values map[int]interface{}) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.IssueFieldValues[issueID] == nil {
		st.IssueFieldValues[issueID] = map[int]interface{}{}
	}
	for id, v := range values {
		st.IssueFieldValues[issueID][id] = v
	}
	if st.persist != nil {
		st.persist.MustPut("issue_field_values", strconv.Itoa(issueID), st.IssueFieldValues[issueID])
	}
}

// ListIssueFieldValues renders an issue's field values in the REST
// issue-field-value shape, sorted by field ID. Values whose field definition
// no longer exists are skipped.
func (st *Store) ListIssueFieldValues(orgLogin string, issueID int) []map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()
	values := st.IssueFieldValues[issueID]
	fieldIDs := make([]int, 0, len(values))
	for id := range values {
		fieldIDs = append(fieldIDs, id)
	}
	sort.Ints(fieldIDs)
	out := make([]map[string]interface{}, 0, len(fieldIDs))
	for _, id := range fieldIDs {
		field := st.OrgIssueFields[orgLogin][id]
		if field == nil {
			continue
		}
		out = append(out, issueFieldValueJSON(field, issueID, values[id]))
	}
	return out
}

// issueFieldValueJSON renders one issue-field-value. For single_select the
// value is the option name and single_select_option carries the option
// details; for multi_select the option details ride multi_select_options and
// value is null (the schema's value member does not admit arrays).
func issueFieldValueJSON(field *IssueField, issueID int, value interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"issue_field_id": field.ID,
		"node_id":        fmt.Sprintf("IFV_kwDO%08d%08d", issueID, field.ID),
		"data_type":      field.DataType,
		"value":          value,
	}
	optionJSON := func(name string) map[string]interface{} {
		for _, opt := range field.Options {
			if opt.Name == name {
				color := opt.Color
				if color == "" {
					color = "gray"
				}
				return map[string]interface{}{"id": opt.ID, "name": opt.Name, "color": color}
			}
		}
		return nil
	}
	switch field.DataType {
	case "single_select":
		if name, ok := value.(string); ok {
			if opt := optionJSON(name); opt != nil {
				out["single_select_option"] = opt
			}
		}
	case "multi_select":
		names := toStringSlice(value)
		opts := make([]map[string]interface{}, 0, len(names))
		for _, name := range names {
			if opt := optionJSON(name); opt != nil {
				opts = append(opts, opt)
			}
		}
		out["multi_select_options"] = opts
		out["value"] = nil
	}
	return out
}

// toStringSlice coerces a stored multi-value ( []string in memory,
// []interface{} after a persistence reload) into []string.
func toStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}
