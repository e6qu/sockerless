package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// validCustomPropertyName reports whether name is an acceptable custom-property
// name. GitHub rejects names with surrounding or embedded whitespace and
// control characters (a name is a URL path segment on the values/schema
// endpoints); mirror that so an invalid name is a 422, not a silently stored
// definition.
func validCustomPropertyName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// GitHub organization custom properties: typed key/value definitions an
// organization declares once (the schema) and assigns per repository (the
// values). terraform-provider-github drives this surface, so the shapes
// follow the OpenAPI description exactly.

// CustomProperty is an organization custom property definition.
type CustomProperty struct {
	PropertyName          string      `json:"property_name"`
	ValueType             string      `json:"value_type"`
	Required              bool        `json:"required"`
	DefaultValue          interface{} `json:"default_value"`
	Description           *string     `json:"description"`
	AllowedValues         []string    `json:"allowed_values"`
	ValuesEditableBy      string      `json:"values_editable_by"`
	RequireExplicitValues bool        `json:"require_explicit_values"`
}

func (s *Server) registerGHCustomPropertyRoutes() {
	s.route("GET /api/v3/orgs/{org}/properties/schema",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgCustomProperties)))
	s.route("PATCH /api/v3/orgs/{org}/properties/schema",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleBatchUpsertOrgCustomProperties)))
	s.route("GET /api/v3/orgs/{org}/properties/schema/{custom_property_name}",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgCustomProperty)))
	s.route("PUT /api/v3/orgs/{org}/properties/schema/{custom_property_name}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleUpsertOrgCustomProperty)))
	s.route("DELETE /api/v3/orgs/{org}/properties/schema/{custom_property_name}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteOrgCustomProperty)))
	s.route("GET /api/v3/orgs/{org}/properties/values",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOrgRepoCustomPropertyValues)))
	s.route("PATCH /api/v3/orgs/{org}/properties/values",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleBatchSetOrgRepoCustomPropertyValues)))
	s.route("GET /api/v3/repos/{owner}/{repo}/properties/values", s.handleGetRepoCustomPropertyValues)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/properties/values",
		s.requirePerm(scopeAdministration, permWrite, s.handleSetRepoCustomPropertyValues))
}

var customPropertyValueTypes = map[string]bool{
	"string": true, "single_select": true, "multi_select": true, "true_false": true, "url": true,
}

// customPropertyPayload is the wire shape of a definition write (the PUT
// payload; the batch PATCH items add property_name).
type customPropertyPayload struct {
	PropertyName          string      `json:"property_name"`
	ValueType             string      `json:"value_type"`
	Required              bool        `json:"required"`
	DefaultValue          interface{} `json:"default_value"`
	Description           *string     `json:"description"`
	AllowedValues         []string    `json:"allowed_values"`
	ValuesEditableBy      *string     `json:"values_editable_by"`
	RequireExplicitValues bool        `json:"require_explicit_values"`
}

// toCustomProperty validates the payload and materializes the definition.
// Missing optional values fall back to their documented defaults.
func (p *customPropertyPayload) toCustomProperty(w http.ResponseWriter, name string) *CustomProperty {
	if name == "" {
		writeGHValidationError(w, "CustomProperty", "property_name", "missing_field")
		return nil
	}
	if !validCustomPropertyName(name) {
		writeGHValidationError(w, "CustomProperty", "property_name", "invalid")
		return nil
	}
	if !customPropertyValueTypes[p.ValueType] {
		writeGHValidationError(w, "CustomProperty", "value_type", "invalid")
		return nil
	}
	isSelect := p.ValueType == "single_select" || p.ValueType == "multi_select"
	if !isSelect && len(p.AllowedValues) > 0 {
		writeGHValidationError(w, "CustomProperty", "allowed_values", "invalid")
		return nil
	}
	if isSelect && len(p.AllowedValues) > 200 {
		writeGHValidationError(w, "CustomProperty", "allowed_values", "invalid")
		return nil
	}
	if p.Required && p.DefaultValue == nil {
		writeGHValidationError(w, "CustomProperty", "default_value", "missing_field")
		return nil
	}
	if p.DefaultValue != nil {
		if err := validateCustomPropertyValue(&CustomProperty{ValueType: p.ValueType, AllowedValues: p.AllowedValues}, p.DefaultValue); err != nil {
			writeGHValidationError(w, "CustomProperty", "default_value", "invalid")
			return nil
		}
	}
	editableBy := "org_actors"
	if p.ValuesEditableBy != nil {
		if *p.ValuesEditableBy != "org_actors" && *p.ValuesEditableBy != "org_and_repo_actors" {
			writeGHValidationError(w, "CustomProperty", "values_editable_by", "invalid")
			return nil
		}
		editableBy = *p.ValuesEditableBy
	}
	return &CustomProperty{
		PropertyName:          name,
		ValueType:             p.ValueType,
		Required:              p.Required,
		DefaultValue:          p.DefaultValue,
		Description:           p.Description,
		AllowedValues:         p.AllowedValues,
		ValuesEditableBy:      editableBy,
		RequireExplicitValues: p.RequireExplicitValues,
	}
}

func (s *Server) handleGetOrgCustomProperties(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	props := s.store.ListCustomProperties(org)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(props))
	for _, p := range props {
		out = append(out, customPropertyJSON(p, org, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBatchUpsertOrgCustomProperties(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		Properties []customPropertyPayload `json:"properties"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Properties) == 0 {
		writeGHValidationError(w, "CustomProperty", "properties", "missing_field")
		return
	}
	defs := make([]*CustomProperty, 0, len(req.Properties))
	for i := range req.Properties {
		def := req.Properties[i].toCustomProperty(w, req.Properties[i].PropertyName)
		if def == nil {
			return
		}
		defs = append(defs, def)
	}
	for _, def := range defs {
		s.store.UpsertCustomProperty(org, def)
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(defs))
	for _, def := range defs {
		out = append(out, customPropertyJSON(def, org, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetOrgCustomProperty(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	name := r.PathValue("custom_property_name")
	p := s.store.GetCustomProperty(org, name)
	if p == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, customPropertyJSON(p, org, s.baseURL(r)))
}

func (s *Server) handleUpsertOrgCustomProperty(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	name := r.PathValue("custom_property_name")
	var req customPropertyPayload
	if !decodeJSONBody(w, r, &req) {
		return
	}
	def := req.toCustomProperty(w, name)
	if def == nil {
		return
	}
	s.store.UpsertCustomProperty(org, def)
	writeJSON(w, http.StatusOK, customPropertyJSON(def, org, s.baseURL(r)))
}

func (s *Server) handleDeleteOrgCustomProperty(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	name := r.PathValue("custom_property_name")
	if !s.store.DeleteCustomProperty(org, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgRepoCustomPropertyValues(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	query := r.URL.Query().Get("repository_query")
	repos := s.store.ListOrgReposForProperties(org, query)
	entries := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		entries = append(entries, map[string]interface{}{
			"repository_id":        repo.ID,
			"repository_name":      repo.Name,
			"repository_full_name": repo.FullName,
			"properties":           s.store.EffectiveRepoCustomPropertyValues(org, repo.FullName),
		})
	}
	entries = paginateAndLink(w, r, entries)
	writeJSON(w, http.StatusOK, entries)
}

type customPropertyValuePayload struct {
	PropertyName string      `json:"property_name"`
	Value        interface{} `json:"value"`
}

func (s *Server) handleBatchSetOrgRepoCustomPropertyValues(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		RepositoryNames []string                     `json:"repository_names"`
		Properties      []customPropertyValuePayload `json:"properties"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.RepositoryNames) == 0 || len(req.RepositoryNames) > 30 {
		writeGHValidationError(w, "CustomPropertyValues", "repository_names", "invalid")
		return
	}
	if req.Properties == nil {
		writeGHValidationError(w, "CustomPropertyValues", "properties", "missing_field")
		return
	}
	repoKeys := make([]string, 0, len(req.RepositoryNames))
	for _, name := range req.RepositoryNames {
		repo := s.store.GetRepo(org, name)
		if repo == nil {
			writeGHValidationError(w, "CustomPropertyValues", "repository_names", "invalid")
			return
		}
		repoKeys = append(repoKeys, repo.FullName)
	}
	if !s.applyCustomPropertyValues(w, org, repoKeys, req.Properties) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetRepoCustomPropertyValues(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.store.EffectiveRepoCustomPropertyValues(owner, repo.FullName))
}

func (s *Server) handleSetRepoCustomPropertyValues(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}
	var req struct {
		Properties []customPropertyValuePayload `json:"properties"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Properties == nil {
		writeGHValidationError(w, "CustomPropertyValues", "properties", "missing_field")
		return
	}
	if !s.applyCustomPropertyValues(w, owner, []string{repo.FullName}, req.Properties) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyCustomPropertyValues validates every value against the org schema and
// applies the batch to each repo. A null value unsets the property.
func (s *Server) applyCustomPropertyValues(w http.ResponseWriter, org string, repoKeys []string, values []customPropertyValuePayload) bool {
	for _, v := range values {
		def := s.store.GetCustomProperty(org, v.PropertyName)
		if def == nil {
			writeGHValidationError(w, "CustomPropertyValues", "property_name", "invalid")
			return false
		}
		if v.Value == nil {
			continue
		}
		if err := validateCustomPropertyValue(def, v.Value); err != nil {
			writeGHValidationError(w, "CustomPropertyValues", def.PropertyName, "invalid")
			return false
		}
	}
	for _, repoKey := range repoKeys {
		s.store.SetRepoCustomPropertyValues(repoKey, values)
	}
	return true
}

// validateCustomPropertyValue checks a non-null value against the property's
// value type (and allowed values for the select types).
func validateCustomPropertyValue(def *CustomProperty, value interface{}) error {
	allowed := func(str string) bool {
		for _, v := range def.AllowedValues {
			if v == str {
				return true
			}
		}
		return false
	}
	switch def.ValueType {
	case "string", "url":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("property %q expects a string value", def.PropertyName)
		}
	case "true_false":
		str, ok := value.(string)
		if !ok || (str != "true" && str != "false") {
			return fmt.Errorf("property %q expects \"true\" or \"false\"", def.PropertyName)
		}
	case "single_select":
		str, ok := value.(string)
		if !ok || !allowed(str) {
			return fmt.Errorf("property %q expects one of its allowed values", def.PropertyName)
		}
	case "multi_select":
		switch v := value.(type) {
		case string:
			// A bare string is accepted as a one-element selection.
			if !allowed(v) {
				return fmt.Errorf("property %q expects a subset of its allowed values", def.PropertyName)
			}
		case []interface{}:
			for _, item := range v {
				str, ok := item.(string)
				if !ok || !allowed(str) {
					return fmt.Errorf("property %q expects a subset of its allowed values", def.PropertyName)
				}
			}
		case []string:
			for _, item := range v {
				if !allowed(item) {
					return fmt.Errorf("property %q expects a subset of its allowed values", def.PropertyName)
				}
			}
		default:
			return fmt.Errorf("property %q expects an array of allowed values", def.PropertyName)
		}
	default:
		return fmt.Errorf("property %q has unsupported value type %q", def.PropertyName, def.ValueType)
	}
	return nil
}

func customPropertyJSON(p *CustomProperty, org, baseURL string) map[string]interface{} {
	var desc interface{}
	if p.Description != nil {
		desc = *p.Description
	}
	out := map[string]interface{}{
		"property_name":           p.PropertyName,
		"url":                     baseURL + "/api/v3/orgs/" + org + "/properties/schema/" + p.PropertyName,
		"source_type":             "organization",
		"value_type":              p.ValueType,
		"required":                p.Required,
		"default_value":           p.DefaultValue,
		"description":             desc,
		"values_editable_by":      p.ValuesEditableBy,
		"require_explicit_values": p.RequireExplicitValues,
	}
	if p.ValueType == "single_select" || p.ValueType == "multi_select" {
		av := p.AllowedValues
		if av == nil {
			av = []string{}
		}
		out["allowed_values"] = av
	} else {
		out["allowed_values"] = nil
	}
	return out
}

// --- store ---

// ListCustomProperties returns the org's property definitions sorted by name.
func (st *Store) ListCustomProperties(orgLogin string) []*CustomProperty {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgCustomProperties[orgLogin]
	out := make([]*CustomProperty, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PropertyName < out[j].PropertyName })
	return out
}

// GetCustomProperty returns a property definition by name, or nil.
func (st *Store) GetCustomProperty(orgLogin, name string) *CustomProperty {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgCustomProperties[orgLogin][name]
}

// UpsertCustomProperty creates or replaces a property definition.
func (st *Store) UpsertCustomProperty(orgLogin string, def *CustomProperty) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgCustomProperties[orgLogin] == nil {
		st.OrgCustomProperties[orgLogin] = map[string]*CustomProperty{}
	}
	st.OrgCustomProperties[orgLogin][def.PropertyName] = def
	if st.persist != nil {
		st.persist.MustPut("org_custom_properties", orgLogin, st.OrgCustomProperties[orgLogin])
	}
}

// DeleteCustomProperty removes a property definition and every repo value
// assigned under it. Returns true when the definition existed.
func (st *Store) DeleteCustomProperty(orgLogin, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgCustomProperties[orgLogin][name] == nil {
		return false
	}
	delete(st.OrgCustomProperties[orgLogin], name)
	prefix := orgLogin + "/"
	for repoKey, values := range st.RepoCustomPropertyValues {
		if !strings.HasPrefix(repoKey, prefix) {
			continue
		}
		if _, ok := values[name]; ok {
			delete(values, name)
			if st.persist != nil {
				st.persist.MustPut("repo_custom_property_values", repoKey, values)
			}
		}
	}
	if st.persist != nil {
		st.persist.MustPut("org_custom_properties", orgLogin, st.OrgCustomProperties[orgLogin])
	}
	return true
}

// SetRepoCustomPropertyValues applies a validated batch of values to one
// repo; null values unset.
func (st *Store) SetRepoCustomPropertyValues(repoKey string, values []customPropertyValuePayload) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.RepoCustomPropertyValues[repoKey] == nil {
		st.RepoCustomPropertyValues[repoKey] = map[string]interface{}{}
	}
	for _, v := range values {
		if v.Value == nil {
			delete(st.RepoCustomPropertyValues[repoKey], v.PropertyName)
		} else {
			st.RepoCustomPropertyValues[repoKey][v.PropertyName] = v.Value
		}
	}
	if st.persist != nil {
		st.persist.MustPut("repo_custom_property_values", repoKey, st.RepoCustomPropertyValues[repoKey])
	}
}

// EffectiveRepoCustomPropertyValues renders the repo's property values in the
// custom-property-value shape: the explicitly set value, else the property's
// default. Properties with no effective value are omitted, matching real
// GitHub.
func (st *Store) EffectiveRepoCustomPropertyValues(orgLogin, repoKey string) []map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()
	defs := st.OrgCustomProperties[orgLogin]
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	set := st.RepoCustomPropertyValues[repoKey]
	out := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		value, ok := set[name]
		if !ok {
			value = defs[name].DefaultValue
		}
		if value == nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"property_name": name,
			"value":         value,
		})
	}
	return out
}

// ListOrgReposForProperties returns the org's repositories, optionally
// filtered by a repository_query keyword matched against the repo name
// (the `repo:owner/name` qualifier is honored as an exact match).
func (st *Store) ListOrgReposForProperties(orgLogin, query string) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	prefix := orgLogin + "/"
	query = strings.TrimSpace(query)
	out := []*Repo{}
	for key, repo := range st.ReposByName {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if query != "" {
			if full, ok := strings.CutPrefix(query, "repo:"); ok {
				if !strings.EqualFold(repo.FullName, full) {
					continue
				}
			} else if !strings.Contains(strings.ToLower(repo.Name), strings.ToLower(query)) {
				continue
			}
		}
		out = append(out, repo)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
