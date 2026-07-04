package bleephub

// GitHub Actions OIDC custom property inclusions for an organization
// (/orgs/{org}/actions/oidc/customization/properties/repo): the set of
// repository custom property names included as claims in the OIDC
// tokens Actions issues. Backed by the store and persisted.

import (
	"net/http"
	"sort"
	"strings"
)

func (s *Server) registerGHActionsOIDCPropertyRoutes() {
	s.route("GET /api/v3/orgs/{org}/actions/oidc/customization/properties/repo",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOIDCPropertyInclusions)))
	s.route("POST /api/v3/orgs/{org}/actions/oidc/customization/properties/repo",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleCreateOIDCPropertyInclusion)))
	s.route("DELETE /api/v3/orgs/{org}/actions/oidc/customization/properties/repo/{custom_property_name}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteOIDCPropertyInclusion)))
}

func oidcPropertyInclusionJSON(name string) map[string]any {
	return map[string]any{
		"custom_property_name": name,
		"inclusion_source":     "organization",
	}
}

func (s *Server) handleListOIDCPropertyInclusions(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	s.store.mu.RLock()
	names := append([]string(nil), s.store.OrgOIDCPropertyInclusions[org]...)
	s.store.mu.RUnlock()
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, oidcPropertyInclusionJSON(name))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateOIDCPropertyInclusion(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		CustomPropertyName string `json:"custom_property_name"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.CustomPropertyName == "" {
		writeGHValidationError(w, "OIDCCustomPropertyInclusion", "custom_property_name", "missing_field")
		return
	}
	s.store.mu.Lock()
	for _, name := range s.store.OrgOIDCPropertyInclusions[org] {
		if strings.EqualFold(name, req.CustomPropertyName) {
			s.store.mu.Unlock()
			writeGHValidationError(w, "OIDCCustomPropertyInclusion", "custom_property_name", "already_exists")
			return
		}
	}
	s.store.OrgOIDCPropertyInclusions[org] = append(s.store.OrgOIDCPropertyInclusions[org], req.CustomPropertyName)
	if s.store.persist != nil {
		s.store.persist.MustPut("org_oidc_property_inclusions", org, s.store.OrgOIDCPropertyInclusions[org])
	}
	s.store.mu.Unlock()
	writeJSON(w, http.StatusCreated, oidcPropertyInclusionJSON(req.CustomPropertyName))
}

func (s *Server) handleDeleteOIDCPropertyInclusion(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	name := r.PathValue("custom_property_name")
	s.store.mu.Lock()
	found := false
	kept := s.store.OrgOIDCPropertyInclusions[org][:0]
	for _, existing := range s.store.OrgOIDCPropertyInclusions[org] {
		if strings.EqualFold(existing, name) {
			found = true
			continue
		}
		kept = append(kept, existing)
	}
	if found {
		s.store.OrgOIDCPropertyInclusions[org] = kept
		if s.store.persist != nil {
			s.store.persist.MustPut("org_oidc_property_inclusions", org, kept)
		}
	}
	s.store.mu.Unlock()
	if !found {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
