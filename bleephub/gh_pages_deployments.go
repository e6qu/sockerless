package bleephub

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GitHub Pages deployments + the Pages health check.
// Endpoints:
//
//	POST /repos/{o}/{r}/pages/deployments
//	GET  /repos/{o}/{r}/pages/deployments/{pages_deployment_id}
//	POST /repos/{o}/{r}/pages/deployments/{pages_deployment_id}/cancel
//	GET  /repos/{o}/{r}/pages/health
//
// A deployment publishes an Actions artifact to the repo's Pages site. The
// publish is synchronous (there is no CDN tier to wait on), so a stored
// deployment is already terminal: "succeed" — the same value real GitHub
// reports once its pipeline finishes. Cancelling is therefore only possible
// for a non-terminal deployment, which cannot be observed in-process.
type PagesDeploymentRecord struct {
	ID           int       `json:"id"`
	RepoID       int       `json:"repo_id"`
	Status       string    `json:"status"`
	Environment  string    `json:"environment"`
	BuildVersion string    `json:"pages_build_version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Server) registerGHPagesDeploymentRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/pages/deployments",
		s.requirePerm(scopeAdministration, permWrite, s.handlePagesDeploymentCreate))
	s.route("GET /api/v3/repos/{owner}/{repo}/pages/deployments/{pages_deployment_id}", s.handlePagesDeploymentStatus)
	s.route("POST /api/v3/repos/{owner}/{repo}/pages/deployments/{pages_deployment_id}/cancel",
		s.requirePerm(scopeAdministration, permWrite, s.handlePagesDeploymentCancel))
	s.route("GET /api/v3/repos/{owner}/{repo}/pages/health", s.handlePagesHealthCheck)
}

// --- Store ---

// CreatePagesDeployment records a Pages deployment for a repository.
func (st *Store) CreatePagesDeployment(repoID int, environment, buildVersion, status string) *PagesDeploymentRecord {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().UTC()
	d := &PagesDeploymentRecord{
		ID:           st.NextPagesDeploymentID,
		RepoID:       repoID,
		Status:       status,
		Environment:  environment,
		BuildVersion: buildVersion,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.NextPagesDeploymentID++
	if st.PagesDeployments[repoID] == nil {
		st.PagesDeployments[repoID] = map[int]*PagesDeploymentRecord{}
	}
	st.PagesDeployments[repoID][d.ID] = d
	if st.persist != nil {
		st.persist.MustPut("pages_deployments", strconv.Itoa(repoID), st.PagesDeployments[repoID])
	}
	return d
}

// GetPagesDeployment returns a Pages deployment by repo and ID, or nil.
func (st *Store) GetPagesDeployment(repoID, id int) *PagesDeploymentRecord {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.PagesDeployments[repoID][id]
}

// SetPagesDeploymentStatus transitions a Pages deployment's status.
// Returns false if the deployment does not exist.
func (st *Store) SetPagesDeploymentStatus(repoID, id int, status string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	d := st.PagesDeployments[repoID][id]
	if d == nil {
		return false
	}
	d.Status = status
	d.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("pages_deployments", strconv.Itoa(repoID), st.PagesDeployments[repoID])
	}
	return true
}

// --- Handlers ---

func (s *Server) handlePagesDeploymentCreate(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	site := s.store.Misc.pagesByRepo[repo.ID]
	s.store.Misc.mu.RUnlock()
	if site == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		ArtifactID        *int64 `json:"artifact_id"`
		ArtifactURL       string `json:"artifact_url"`
		Environment       string `json:"environment"`
		PagesBuildVersion string `json:"pages_build_version"`
		OIDCToken         string `json:"oidc_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.PagesBuildVersion == "" {
		writeGHValidationError(w, "PageDeployment", "pages_build_version", "missing_field")
		return
	}
	if req.OIDCToken == "" {
		writeGHValidationError(w, "PageDeployment", "oidc_token", "missing_field")
		return
	}
	if req.ArtifactID == nil && req.ArtifactURL == "" {
		writeGHError(w, http.StatusBadRequest, "Either artifact_id or artifact_url is required.")
		return
	}
	if req.ArtifactID != nil {
		if !s.repoOwnsFinalizedArtifact(repo.FullName, *req.ArtifactID) {
			writeGHError(w, http.StatusBadRequest, "The artifact could not be found or does not belong to this repository.")
			return
		}
	}
	environment := coalesceStr(req.Environment, "github-pages")

	// The publish happens here, synchronously: the site's content becomes
	// the artifact and its status flips to built. The stored deployment is
	// therefore already terminal.
	s.store.Misc.mu.Lock()
	site.Status = "built"
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("pages_sites", strconv.Itoa(repo.ID), site)
	}
	s.store.Misc.mu.Unlock()

	d := s.store.CreatePagesDeployment(repo.ID, environment, req.PagesBuildVersion, "succeed")

	user := ghUserFromContext(r.Context())
	if user != nil {
		s.recordAuditEvent("pages.deployment", user.Login, "", map[string]interface{}{"repo": repo.FullName, "deployment_id": d.ID})
	}

	base := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         d.ID,
		"status_url": base + "/api/v3/repos/" + repo.FullName + "/pages/deployments/" + strconv.Itoa(d.ID),
		"page_url":   site.HTMLURL,
	})
}

func (s *Server) handlePagesDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("pages_deployment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.GetPagesDeployment(repo.ID, id)
	if d == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": d.Status})
}

func (s *Server) handlePagesDeploymentCancel(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("pages_deployment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.GetPagesDeployment(repo.ID, id)
	if d == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if pagesDeploymentTerminal(d.Status) {
		writeGHError(w, http.StatusUnprocessableEntity, "Deployment cannot be cancelled")
		return
	}
	s.store.SetPagesDeploymentStatus(repo.ID, id, "deployment_cancelled")
	w.WriteHeader(http.StatusNoContent)
}

func pagesDeploymentTerminal(status string) bool {
	switch status {
	case "succeed", "deployment_cancelled", "deployment_failed",
		"deployment_content_failed", "deployment_attempt_error", "deployment_lost":
		return true
	}
	return false
}

// repoOwnsFinalizedArtifact reports whether the repository owns a finalized
// Actions artifact with the given ID.
func (s *Server) repoOwnsFinalizedArtifact(repoFullName string, artifactID int64) bool {
	s.artifactStore.mu.Lock()
	defer s.artifactStore.mu.Unlock()
	for _, art := range s.artifactStore.artifacts {
		if art.ID == artifactID && art.RepoFullName == repoFullName && art.Finalized {
			return true
		}
	}
	return false
}

// --- Pages health check ---

func (s *Server) handlePagesHealthCheck(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Misc.mu.RLock()
	site := s.store.Misc.pagesByRepo[repo.ID]
	var cname string
	var httpsEnforced bool
	if site != nil {
		cname = site.CNAME
		httpsEnforced = site.HTTPSEnforced
	}
	s.store.Misc.mu.RUnlock()
	if site == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if cname == "" {
		writeGHError(w, http.StatusBadRequest, "There isn't a custom domain on this Pages site")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"domain":     pagesDomainHealthJSON(r.Context(), cname, httpsEnforced),
		"alt_domain": nil,
	})
}

// pagesDomainHealthJSON runs the real domain checks bleephub can perform
// locally: a DNS resolution and syntactic domain classification. Checks
// that would require probing GitHub's Pages edge (A-record targets,
// Fastly/Cloudflare classification, live HTTPS probes) are omitted rather
// than fabricated — every member is optional in the health-check schema.
func pagesDomainHealthJSON(ctx context.Context, host string, httpsEnforced bool) map[string]interface{} {
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	dnsResolves := err == nil && len(addrs) > 0

	isValidDomain := validPagesDomain(host)
	isApex := isValidDomain && strings.Count(host, ".") == 1
	isPagesDomain := strings.HasSuffix(host, ".github.io") || host == "github.io"

	var reason interface{}
	isValid := isValidDomain && dnsResolves
	if !isValidDomain {
		reason = "invalid domain"
	} else if !dnsResolves {
		reason = "domain does not resolve"
	}

	return map[string]interface{}{
		"host":            host,
		"uri":             "http://" + host + "/",
		"nameservers":     "default",
		"dns_resolves":    dnsResolves,
		"is_valid_domain": isValidDomain,
		"is_apex_domain":  isApex,
		"is_pages_domain": isPagesDomain,
		"is_valid":        isValid,
		"reason":          reason,
		"enforces_https":  httpsEnforced,
	}
}

// validPagesDomain applies hostname syntax rules (RFC 1123 labels).
func validPagesDomain(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i, c := range label {
			isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
			if !isAlnum && c != '-' {
				return false
			}
			if c == '-' && (i == 0 || i == len(label)-1) {
				return false
			}
		}
	}
	return true
}
