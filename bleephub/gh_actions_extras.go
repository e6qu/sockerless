package bleephub

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Actions extras gh CLI / Octokit hit.
//   POST /repos/{o}/{r}/dispatches                          repository_dispatch
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/logs           run-level logs zip
//   POST /repos/{o}/{r}/actions/runs/{run_id}/rerun-failed-jobs
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/timing         per-job timing summary
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/artifacts      artifact list
//   GET  /repos/{o}/{r}/actions/artifacts                    repo-wide artifact list
//   GET  /repos/{o}/{r}/actions/artifacts/{artifact_id}       artifact metadata
//   DELETE /repos/{o}/{r}/actions/artifacts/{artifact_id}     delete artifact
//   GET  /repos/{o}/{r}/actions/artifacts/{artifact_id}/zip   artifact download redirect
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/approvals      env-pending approvals

func (s *Server) registerGHActionsExtrasRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/dispatches",
		s.requirePerm(scopeContents, permWrite, s.handleRepositoryDispatch))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/logs",
		s.handleRunLogs)
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs",
		s.requirePerm(scopeActions, permWrite, s.handleRerunFailedJobs))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/timing",
		s.handleRunTiming)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/artifacts",
		s.handleRunArtifacts)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/artifacts",
		s.handleRepoArtifacts)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/artifacts/{artifact_id}",
		s.handleGetArtifact)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/artifacts/{artifact_id}",
		s.requirePerm(scopeActions, permWrite, s.handleDeleteArtifact))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/artifacts/{artifact_id}/{archive_format}",
		s.handleDownloadArtifactArchive)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/approvals",
		s.handleRunApprovals)
}

// handleRepositoryDispatch — POST /repos/{o}/{r}/dispatches.
// gh / curl GitOps tools send this to fire a workflow listening on
// `on: repository_dispatch`. Real GH 204s. Bleephub also emits a
// `repository_dispatch` webhook event so downstream automation runs.
func (s *Server) handleRepositoryDispatch(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	var req struct {
		EventType     string                 `json:"event_type"`
		ClientPayload map[string]interface{} `json:"client_payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventType == "" {
		writeGHValidationError(w, "RepositoryDispatch", "event_type", "missing_field")
		return
	}
	payload := map[string]interface{}{
		"action":         req.EventType,
		"event_type":     req.EventType,
		"client_payload": req.ClientPayload,
		"repository":     repoPayload(repo),
		"sender":         senderPayload(user),
	}
	s.emitWebhookEvent(repo.FullName, "repository_dispatch", req.EventType, attachInstallationBlock(payload, nil))
	// Trigger any workflow_dispatch-style triggers; for now, real GH also
	// invokes `repository_dispatch` workflows — wire by event name.
	go s.triggerWorkflowsForEvent(repo.FullName, "repository_dispatch", "refs/heads/"+repo.DefaultBranch)
	w.WriteHeader(http.StatusNoContent)
}

// handleRunLogs — returns a zip with one txt file per job. Real GH redirects
// to a signed-URL download; for bleephub we return the zip directly with
// Content-Type: application/zip (curl + gh both handle the response body).
func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findWorkflowByRunID(runID)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for jobKey, job := range wf.Jobs {
		lines := s.store.LogLines[job.JobID]
		f, err := zw.Create(fmt.Sprintf("%s_%s.txt", jobKey, job.JobID))
		if err != nil {
			continue
		}
		for _, line := range lines {
			_, _ = f.Write([]byte(line + "\n"))
		}
	}
	_ = zw.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="logs_%d.zip"`, runID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// handleRerunFailedJobs reuses the rerun path; bleephub doesn't distinguish
// failed-only re-run from full re-run today. Real GH does — record the
// shape but no behaviour difference until we model per-attempt state.
func (s *Server) handleRerunFailedJobs(w http.ResponseWriter, r *http.Request) {
	s.handleRerunWorkflowRun(w, r)
}

// handleRunTiming returns the per-job billing-style timing summary.
func (s *Server) handleRunTiming(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findWorkflowByRunID(runID)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// Without real timing instrumentation, bleephub reports the wall-clock
	// time from workflow creation to now (or completion if known).
	durationMs := int64(0)
	if !wf.CreatedAt.IsZero() {
		durationMs = time.Since(wf.CreatedAt).Milliseconds()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"billable": map[string]interface{}{
			"UBUNTU": map[string]interface{}{
				"total_ms": durationMs,
				"jobs":     len(wf.Jobs),
			},
		},
		"run_duration_ms": durationMs,
	})
}

func (s *Server) handleRunArtifacts(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	wf := s.findWorkflowByRunID(runID)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	matching := s.filterArtifacts(r, func(art *Artifact) bool {
		return s.artifactBelongsToRun(art, wf)
	})
	s.writeArtifactList(w, r, matching)
}

func (s *Server) handleRepoArtifacts(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	matching := s.filterArtifacts(r, func(art *Artifact) bool {
		return s.artifactBelongsToRepo(art, repo)
	})
	s.writeArtifactList(w, r, matching)
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	art, ok := s.getRepoArtifact(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.artifactJSON(art, r))
}

func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	art, ok := s.getRepoArtifact(w, r)
	if !ok {
		return
	}
	if !s.artifactStore.deleteArtifact(art.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDownloadArtifactArchive(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("archive_format") != "zip" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	art, ok := s.getRepoArtifact(w, r)
	if !ok {
		return
	}
	http.Redirect(w, r, fmt.Sprintf("%s/_apis/v1/artifacts/%d/download", s.baseURL(r), art.ID), http.StatusFound)
}

func (s *Server) getRepoArtifact(w http.ResponseWriter, r *http.Request) (*Artifact, bool) {
	artifactID, err := strconv.ParseInt(r.PathValue("artifact_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid artifact_id")
		return nil, false
	}
	art, ok := s.artifactStore.artifactByID(artifactID)
	if !ok || !s.artifactBelongsToRepo(art, repoFullName(r)) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return art, true
}

func (s *Server) filterArtifacts(r *http.Request, keep func(*Artifact) bool) []*Artifact {
	nameFilter := r.URL.Query().Get("name")
	artifacts := s.artifactStore.finalizedArtifacts()
	matching := make([]*Artifact, 0, len(artifacts))
	for _, art := range artifacts {
		if nameFilter != "" && art.Name != nameFilter {
			continue
		}
		if keep(art) {
			matching = append(matching, art)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool {
		if matching[i].CreatedAt.Equal(matching[j].CreatedAt) {
			return matching[i].ID > matching[j].ID
		}
		return matching[i].CreatedAt.After(matching[j].CreatedAt)
	})
	return matching
}

func (s *Server) writeArtifactList(w http.ResponseWriter, r *http.Request, matching []*Artifact) {
	page := paginateAndLink(w, r, matching)
	artifacts := make([]map[string]any, 0, len(page))
	for _, art := range page {
		artifacts = append(artifacts, s.artifactJSON(art, r))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count": len(matching),
		"artifacts":   artifacts,
	})
}

func (s *Server) artifactJSON(art *Artifact, r *http.Request) map[string]any {
	repo := art.RepoFullName
	if repo == "" {
		if wf := s.workflowForArtifact(art); wf != nil {
			repo = wf.RepoFullName
		}
	}
	if repo == "" {
		repo = repoFullName(r)
	}
	base := s.baseURL(r)
	apiBase := fmt.Sprintf("%s/api/v3/repos/%s", base, repo)
	created := art.CreatedAt.UTC()
	if created.IsZero() {
		created = time.Unix(0, 0).UTC()
	}
	hash := sha256.Sum256(art.Data)
	runID := art.GitHubRunID
	headBranch := ""
	headSHA := ""
	if wf := s.workflowForArtifact(art); wf != nil {
		runID = wf.RunID
		headBranch = headBranchOf(wf)
		headSHA = wf.Sha
	}
	return map[string]any{
		"id":                   art.ID,
		"node_id":              base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("08:Artifact%d", art.ID))),
		"name":                 art.Name,
		"size_in_bytes":        art.Size,
		"url":                  fmt.Sprintf("%s/actions/artifacts/%d", apiBase, art.ID),
		"archive_download_url": fmt.Sprintf("%s/actions/artifacts/%d/zip", apiBase, art.ID),
		"expired":              false,
		"created_at":           created.Format("2006-01-02T15:04:05Z"),
		"expires_at":           created.Add(90 * 24 * time.Hour).Format("2006-01-02T15:04:05Z"),
		"updated_at":           created.Format("2006-01-02T15:04:05Z"),
		"digest":               fmt.Sprintf("sha256:%x", hash),
		"workflow_run": map[string]any{
			"id":                 int64(runID),
			"repository_id":      int64(s.repoIDByFullName(repo)),
			"head_repository_id": int64(s.repoIDByFullName(repo)),
			"head_branch":        headBranch,
			"head_sha":           headSHA,
		},
	}
}

func (s *Server) artifactBelongsToRun(art *Artifact, wf *Workflow) bool {
	if wf == nil {
		return false
	}
	if art.GitHubRunID == wf.RunID || art.RunID == wf.ID || art.WorkflowRunBackendID == wf.ID {
		return true
	}
	runID := strconv.Itoa(wf.RunID)
	return art.RunID == runID || art.WorkflowRunBackendID == runID
}

func (s *Server) artifactBelongsToRepo(art *Artifact, repo string) bool {
	if strings.EqualFold(art.RepoFullName, repo) {
		return true
	}
	wf := s.workflowForArtifact(art)
	return wf != nil && strings.EqualFold(wf.RepoFullName, repo)
}

func (s *Server) workflowForArtifact(art *Artifact) *Workflow {
	if art == nil {
		return nil
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, wf := range s.store.Workflows {
		if s.artifactBelongsToRun(art, wf) {
			return wf
		}
	}
	return nil
}

func (s *Server) findWorkflowByBackendID(backendID string) *Workflow {
	if backendID == "" {
		return nil
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, wf := range s.store.Workflows {
		if backendID == wf.ID || backendID == strconv.Itoa(wf.RunID) {
			return wf
		}
	}
	return nil
}

func (s *Server) repoIDByFullName(fullName string) int {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	if repo := s.store.ReposByName[fullName]; repo != nil {
		return repo.ID
	}
	lowerFullName := strings.ToLower(fullName)
	for name, repo := range s.store.ReposByName {
		if strings.ToLower(name) == lowerFullName {
			return repo.ID
		}
	}
	return 0
}

// handleRunApprovals reports environment approvals once Environments are modeled.
func (s *Server) handleRunApprovals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

// findWorkflowByRunID lives in gh_actions_rest.go alongside the rest of
// the workflow-run helpers; reused here.
