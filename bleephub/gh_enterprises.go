package bleephub

import (
	"net/http"
	"os"
	"strings"
)

// bleephub plays the role of a single GitHub Enterprise Server instance, so
// exactly one enterprise exists. Its slug is configuration — the GHES
// installation names its enterprise account at setup — via
// BLEEPHUB_ENTERPRISE_SLUG, defaulting to "bleephub". Every authenticated
// user of the instance is an enterprise member; site administrators are the
// enterprise owners (on GHES the two roles coincide).

const defaultEnterpriseSlug = "bleephub"

// enterpriseSlug returns the configured enterprise slug.
func (s *Server) enterpriseSlug() string {
	if v := os.Getenv("BLEEPHUB_ENTERPRISE_SLUG"); v != "" {
		return v
	}
	return defaultEnterpriseSlug
}

// resolveEnterprise 404s (like real GitHub for an unknown enterprise) unless
// the {enterprise} path parameter names the configured enterprise.
func (s *Server) resolveEnterprise(w http.ResponseWriter, r *http.Request) bool {
	if r.PathValue("enterprise") != s.enterpriseSlug() {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return false
	}
	return true
}

// requireEnterpriseMember gates an enterprise endpoint on an authenticated
// user of the instance (every user belongs to the single enterprise) and on
// the {enterprise} path parameter naming the configured enterprise.
func (s *Server) requireEnterpriseMember(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ghUserFromContext(r.Context()) == nil {
			writeGHError(w, http.StatusUnauthorized, "Requires authentication")
			return
		}
		if !s.resolveEnterprise(w, r) {
			return
		}
		next(w, r)
	}
}

// requireEnterpriseOwner additionally requires the caller to be an
// enterprise owner — on GitHub Enterprise Server that is a site
// administrator.
func (s *Server) requireEnterpriseOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := ghUserFromContext(r.Context())
		if user == nil {
			writeGHError(w, http.StatusUnauthorized, "Requires authentication")
			return
		}
		if !s.resolveEnterprise(w, r) {
			return
		}
		if !user.SiteAdmin {
			writeGHError(w, http.StatusForbidden, "Must be an enterprise owner.")
			return
		}
		next(w, r)
	}
}

// registerGHEnterpriseRoutes mounts the enterprise-scoped REST surface:
// enterprise teams (+ memberships and organization assignments), code
// security configurations, Dependabot alerts / repository access, GitHub
// Actions cache limits, Actions OIDC custom property inclusions, and the
// Copilot coding agent policy + usage metrics reports.
// splitCommaList splits a comma-separated query filter into its trimmed,
// non-empty entries.
func splitCommaList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) registerGHEnterpriseRoutes() {
	s.registerGHEnterpriseTeamRoutes()
	s.registerGHEnterpriseCodeSecurityRoutes()
	s.registerGHEnterpriseActionsRoutes()
	s.registerGHEnterpriseCopilotRoutes()
	s.registerGHEnterpriseDependabotRoutes()
}
