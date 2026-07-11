package bleephub

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Artifact attestation REST API: uploading Sigstore bundles to a
// repository and listing/deleting them by subject digest at the
// repository, organization, and user scope.

func (s *Server) registerGHAttestationsRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/attestations", s.handleRepoCreateAttestation)
	s.route("GET /api/v3/repos/{owner}/{repo}/attestations/{subject_digest}", s.handleRepoListAttestations)

	s.route("GET /api/v3/orgs/{org}/attestations/repositories", s.handleOrgListAttestationRepositories)
	s.route("GET /api/v3/orgs/{org}/attestations/{subject_digest}", s.handleOrgListAttestations)
	s.route("POST /api/v3/orgs/{org}/attestations/bulk-list", s.handleOrgListAttestationsBulk)
	s.route("POST /api/v3/orgs/{org}/attestations/delete-request", s.handleOrgDeleteAttestationsBulk)
	s.route("DELETE /api/v3/orgs/{org}/attestations/{attestation_id}", s.handleOrgDeleteAttestationByID)
	s.route("DELETE /api/v3/orgs/{org}/attestations/digest/{subject_digest}", s.handleOrgDeleteAttestationsByDigest)

	s.route("GET /api/v3/users/{username}/attestations/{subject_digest}", s.handleUserListAttestations)
	s.route("POST /api/v3/users/{username}/attestations/bulk-list", s.handleUserListAttestationsBulk)
	s.route("POST /api/v3/users/{username}/attestations/delete-request", s.handleUserDeleteAttestationsBulk)
	s.route("DELETE /api/v3/users/{username}/attestations/{attestation_id}", s.handleUserDeleteAttestationByID)
	s.route("DELETE /api/v3/users/{username}/attestations/digest/{subject_digest}", s.handleUserDeleteAttestationsByDigest)
}

// ---------------------------------------------------------------------------
// Repository scope

func (s *Server) handleRepoCreateAttestation(w http.ResponseWriter, r *http.Request) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have write access to the repository.")
		return
	}
	var req struct {
		Bundle json.RawMessage `json:"bundle"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Bundle) == 0 || string(req.Bundle) == "null" {
		writeGHValidationError(w, "Attestation", "bundle", "missing_field")
		return
	}
	subjects, predicateType, err := parseSigstoreBundleSubjects(req.Bundle)
	if err != nil {
		writeGHValidationError(w, "Attestation", "bundle", "invalid")
		return
	}
	a, err := s.store.CreateAttestation(repo.ID, req.Bundle, subjects, predicateType, user.Login)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": a.ID})
}

func (s *Server) handleRepoListAttestations(w http.ResponseWriter, r *http.Request) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	user := ghUserFromContext(r.Context())
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	attestations := s.store.ListAttestations(
		map[int]bool{repo.ID: true},
		r.PathValue("subject_digest"),
		r.URL.Query().Get("predicate_type"),
	)
	s.writeAttestationList(w, r, attestations)
}

// ---------------------------------------------------------------------------
// Owner (org/user) scope plumbing

// attestationRepoScope returns the IDs of the owner's repositories the
// requesting user can read — the set an attestation list may draw from.
func (s *Server) attestationRepoScope(r *http.Request, ownerLogin string) map[int]bool {
	user := ghUserFromContext(r.Context())
	ids := s.store.RepoIDsOwnedBy(ownerLogin)
	for id := range ids {
		repo := s.store.GetRepoByID(id)
		if repo == nil || !canReadRepo(s.store, user, repo) {
			delete(ids, id)
		}
	}
	return ids
}

// requireOrgAttestationAdmin resolves {org} and enforces org-admin
// rights for attestation deletion. Writes 404/401/403 on failure.
func (s *Server) requireOrgAttestationAdmin(w http.ResponseWriter, r *http.Request) (*Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil, false
	}
	if !user.SiteAdmin && !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization admin.")
		return nil, false
	}
	return org, true
}

// requireUserAttestationOwner resolves {username} and enforces that the
// caller is that user (or a site admin) for attestation deletion.
func (s *Server) requireUserAttestationOwner(w http.ResponseWriter, r *http.Request) (*User, bool) {
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil, false
	}
	if user.ID != target.ID && !user.SiteAdmin {
		writeGHError(w, http.StatusForbidden, "Must be the addressed user.")
		return nil, false
	}
	return target, true
}

// writeAttestationList paginates and renders the shared
// {"attestations": [{bundle, repository_id}]} list shape.
func (s *Server) writeAttestationList(w http.ResponseWriter, r *http.Request, attestations []*Attestation) {
	page, pi := cursorPaginate(r, attestations, func(a *Attestation) int { return a.ID })
	setCursorLinkHeader(w, r, pi)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		bundle, err := s.store.ReadAttestationBundle(r.Context(), a)
		if err != nil {
			writeGHError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, map[string]interface{}{
			"bundle":        bundle,
			"repository_id": a.RepoID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"attestations": out})
}

func (s *Server) serveOwnerListAttestations(w http.ResponseWriter, r *http.Request, ownerLogin string) {
	attestations := s.store.ListAttestations(
		s.attestationRepoScope(r, ownerLogin),
		r.PathValue("subject_digest"),
		r.URL.Query().Get("predicate_type"),
	)
	s.writeAttestationList(w, r, attestations)
}

func (s *Server) serveOwnerListAttestationsBulk(w http.ResponseWriter, r *http.Request, ownerLogin string) {
	var req struct {
		SubjectDigests []string `json:"subject_digests"`
		PredicateType  string   `json:"predicate_type"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.SubjectDigests) == 0 || len(req.SubjectDigests) > 1024 {
		writeGHValidationError(w, "Attestation", "subject_digests", "invalid")
		return
	}
	scope := s.attestationRepoScope(r, ownerLogin)

	requested := make([]string, 0, len(req.SubjectDigests))
	for _, d := range req.SubjectDigests {
		requested = append(requested, strings.ToLower(d))
	}
	matched := make([]*Attestation, 0)
	seen := map[int]bool{}
	for _, digest := range requested {
		for _, a := range s.store.ListAttestations(scope, digest, req.PredicateType) {
			if !seen[a.ID] {
				seen[a.ID] = true
				matched = append(matched, a)
			}
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })

	page, pi := cursorPaginate(r, matched, func(a *Attestation) int { return a.ID })
	setCursorLinkHeader(w, r, pi)

	byDigest := map[string]interface{}{}
	for _, digest := range requested {
		entries := make([]map[string]interface{}, 0)
		for _, a := range page {
			if a.hasSubjectDigest(digest) {
				bundle, err := s.store.ReadAttestationBundle(r.Context(), a)
				if err != nil {
					writeGHError(w, http.StatusInternalServerError, err.Error())
					return
				}
				entries = append(entries, map[string]interface{}{
					"bundle":        bundle,
					"repository_id": a.RepoID,
				})
			}
		}
		if len(entries) == 0 {
			anyMatch := false
			for _, a := range matched {
				if a.hasSubjectDigest(digest) {
					anyMatch = true
					break
				}
			}
			if !anyMatch {
				byDigest[digest] = nil
				continue
			}
		}
		byDigest[digest] = entries
	}

	pageInfo := map[string]interface{}{
		"has_next":     pi.HasNext,
		"has_previous": pi.HasPrev,
	}
	if pi.HasNext {
		pageInfo["next"] = pi.Next
	}
	if pi.HasPrev {
		pageInfo["previous"] = pi.Prev
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"attestations_subject_digests": byDigest,
		"page_info":                    pageInfo,
	})
}

// serveOwnerDeleteAttestationsBulk handles the delete-request body: a
// list of subject digests or a list of attestation IDs, but not both.
func (s *Server) serveOwnerDeleteAttestationsBulk(w http.ResponseWriter, r *http.Request, ownerLogin string) {
	var req struct {
		SubjectDigests []string `json:"subject_digests"`
		AttestationIDs []int    `json:"attestation_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if (len(req.SubjectDigests) == 0) == (len(req.AttestationIDs) == 0) {
		writeGHValidationError(w, "Attestation", "subject_digests", "invalid")
		return
	}
	scope := s.store.RepoIDsOwnedBy(ownerLogin)

	var doomed []int
	if len(req.SubjectDigests) > 0 {
		for _, digest := range req.SubjectDigests {
			for _, a := range s.store.ListAttestations(scope, strings.ToLower(digest), "") {
				doomed = append(doomed, a.ID)
			}
		}
	} else {
		for _, id := range req.AttestationIDs {
			if a := s.store.GetAttestation(id); a != nil && scope[a.RepoID] {
				doomed = append(doomed, id)
			}
		}
	}
	deleted := false
	for _, id := range doomed {
		ok, err := s.store.DeleteAttestation(id)
		if err != nil {
			writeGHError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ok {
			deleted = true
		}
	}
	if !deleted {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) serveOwnerDeleteAttestationByID(w http.ResponseWriter, r *http.Request, ownerLogin string) {
	id, err := strconv.Atoi(r.PathValue("attestation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	a := s.store.GetAttestation(id)
	if a == nil || !s.store.RepoIDsOwnedBy(ownerLogin)[a.RepoID] {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if _, err := s.store.DeleteAttestation(id); err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveOwnerDeleteAttestationsByDigest(w http.ResponseWriter, r *http.Request, ownerLogin string) {
	scope := s.store.RepoIDsOwnedBy(ownerLogin)
	matched := s.store.ListAttestations(scope, r.PathValue("subject_digest"), "")
	if len(matched) == 0 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	for _, a := range matched {
		if _, err := s.store.DeleteAttestation(a.ID); err != nil {
			writeGHError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Organization scope

func (s *Server) handleOrgListAttestations(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.serveOwnerListAttestations(w, r, org.Login)
}

// handleOrgListAttestationRepositories lists the org's repositories
// with at least one attestation, ascending by repository ID.
func (s *Server) handleOrgListAttestationRepositories(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	attestations := s.store.ListAttestations(
		s.attestationRepoScope(r, org.Login), "", r.URL.Query().Get("predicate_type"))
	repoIDs := map[int]bool{}
	for _, a := range attestations {
		repoIDs[a.RepoID] = true
	}
	ids := make([]int, 0, len(repoIDs))
	for id := range repoIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	page, pi := cursorPaginate(r, ids, func(id int) int { return id })
	setCursorLinkHeader(w, r, pi)
	out := make([]map[string]interface{}, 0, len(page))
	for _, id := range page {
		if repo := s.store.GetRepoByID(id); repo != nil {
			out = append(out, map[string]interface{}{"id": repo.ID, "name": repo.Name})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOrgListAttestationsBulk(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.serveOwnerListAttestationsBulk(w, r, org.Login)
}

func (s *Server) handleOrgDeleteAttestationsBulk(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgAttestationAdmin(w, r)
	if !ok {
		return
	}
	s.serveOwnerDeleteAttestationsBulk(w, r, org.Login)
}

func (s *Server) handleOrgDeleteAttestationByID(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgAttestationAdmin(w, r)
	if !ok {
		return
	}
	s.serveOwnerDeleteAttestationByID(w, r, org.Login)
}

func (s *Server) handleOrgDeleteAttestationsByDigest(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgAttestationAdmin(w, r)
	if !ok {
		return
	}
	s.serveOwnerDeleteAttestationsByDigest(w, r, org.Login)
}

// ---------------------------------------------------------------------------
// User scope

func (s *Server) handleUserListAttestations(w http.ResponseWriter, r *http.Request) {
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.serveOwnerListAttestations(w, r, target.Login)
}

func (s *Server) handleUserListAttestationsBulk(w http.ResponseWriter, r *http.Request) {
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.serveOwnerListAttestationsBulk(w, r, target.Login)
}

func (s *Server) handleUserDeleteAttestationsBulk(w http.ResponseWriter, r *http.Request) {
	target, ok := s.requireUserAttestationOwner(w, r)
	if !ok {
		return
	}
	s.serveOwnerDeleteAttestationsBulk(w, r, target.Login)
}

func (s *Server) handleUserDeleteAttestationByID(w http.ResponseWriter, r *http.Request) {
	target, ok := s.requireUserAttestationOwner(w, r)
	if !ok {
		return
	}
	s.serveOwnerDeleteAttestationByID(w, r, target.Login)
}

func (s *Server) handleUserDeleteAttestationsByDigest(w http.ResponseWriter, r *http.Request) {
	target, ok := s.requireUserAttestationOwner(w, r)
	if !ok {
		return
	}
	s.serveOwnerDeleteAttestationsByDigest(w, r, target.Login)
}
