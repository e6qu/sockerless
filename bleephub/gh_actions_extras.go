package bleephub

import (
	"archive/zip"
	"bytes"
	"context"
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
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/pending_deployments",
		s.handleGetPendingDeployments)
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/pending_deployments",
		s.requirePerm(scopeActions, permWrite, s.handleReviewPendingDeployments))
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
	payload := repositoryDispatchPayload(repo, user, req.EventType, req.ClientPayload)
	s.emitWebhookEvent(repo.FullName, "repository_dispatch", req.EventType, attachInstallationBlock(payload, nil))
	// The custom event_type is the activity type: `on.repository_dispatch.
	// types` filters against it on real GitHub.
	s.triggerWorkflowsForEvent(repo.FullName, "repository_dispatch", req.EventType, "refs/heads/"+repo.DefaultBranch, payload)
	w.WriteHeader(http.StatusNoContent)
}

// repositoryDispatchPayload builds the repository_dispatch webhook event
// body. GitHub includes a top-level `branch` (the repo's default branch)
// alongside action / client_payload / repository / sender.
func repositoryDispatchPayload(repo *Repo, user *User, eventType string, clientPayload map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"action":         eventType,
		"event_type":     eventType,
		"branch":         repo.DefaultBranch,
		"client_payload": clientPayload,
		"repository":     repoPayload(repo),
		"sender":         senderPayload(user),
	}
}

// handleRunLogs — returns the run's log archive shaped like real GitHub's:
// per job a top-level "0_<jobname>.txt" (full job log) plus a
// "<jobname>/" folder with "<number>_<step name>.txt" per step that has
// uploaded log content. Real GH redirects to a signed-URL download;
// bleephub returns the zip directly with Content-Type: application/zip
// (curl + gh both handle the body).
func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writeRunLogsZip(r.Context(), w, wf, runID)
}

// writeRunLogsZip renders the run's log archive (real GitHub's layout:
// per job a top-level "0_<jobname>.txt" full job log plus a
// "<jobname>/" folder with per-step files) and writes it as the
// response. Shared by the run-level and attempt-level log endpoints.
func (s *Server) writeRunLogsZip(ctx context.Context, w http.ResponseWriter, wf *Workflow, runID int) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	s.store.mu.RLock()
	jobKeys := make([]string, 0, len(wf.Jobs))
	for jobKey := range wf.Jobs {
		jobKeys = append(jobKeys, jobKey)
	}
	sort.Strings(jobKeys)
	type runLogJob struct {
		name       string
		refs       []jobLogRef
		memoryLogs map[int][]byte
	}
	jobs := make([]runLogJob, 0, len(jobKeys))
	for _, jobKey := range jobKeys {
		job := wf.Jobs[jobKey]
		jobName := job.DisplayName
		if jobName == "" {
			jobName = jobKey
		}
		refs := s.jobLogRefsLocked(job.JobID)
		jobs = append(jobs, runLogJob{
			name:       zipSafeName(jobName),
			refs:       refs,
			memoryLogs: s.memoryLogFilesForDownloadLocked(refs),
		})
	}
	s.store.mu.RUnlock()

	wroteAny := false
	for _, job := range jobs {
		var full bytes.Buffer
		type stepEntry struct {
			name    string
			content []byte
		}
		steps := make([]stepEntry, 0, len(job.refs))
		for _, ref := range job.refs {
			content, ok, err := s.logFileContent(ctx, ref.ID, job.memoryLogs[ref.ID])
			if err != nil {
				_ = zw.Close()
				writeGHError(w, http.StatusInternalServerError, "log byte-store read: "+err.Error())
				return
			}
			if !ok {
				continue
			}
			full.Write(content)
			if content[len(content)-1] != '\n' {
				full.WriteByte('\n')
			}
			steps = append(steps, stepEntry{name: ref.Name, content: content})
		}
		if full.Len() == 0 {
			continue
		}
		if f, err := zw.Create(fmt.Sprintf("0_%s.txt", job.name)); err == nil {
			_, _ = f.Write(full.Bytes())
			wroteAny = true
		}
		for i, step := range steps {
			f, err := zw.Create(fmt.Sprintf("%s/%d_%s.txt", job.name, i+1, zipSafeName(step.name)))
			if err != nil {
				_ = zw.Close()
				writeGHError(w, http.StatusInternalServerError, "create log archive entry: "+err.Error())
				return
			}
			_, _ = f.Write(step.content)
			wroteAny = true
		}
	}
	if !wroteAny {
		_ = zw.Close()
		writeGHError(w, http.StatusNotFound, "Logs not found")
		return
	}
	if err := zw.Close(); err != nil {
		writeGHError(w, http.StatusInternalServerError, "finish log archive: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="logs_%d.zip"`, runID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// zipSafeName keeps job/step names usable as zip entry path segments —
// '/' in a name would otherwise change the archive layout.
func zipSafeName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

// handleRerunFailedJobs — POST .../runs/{run_id}/rerun-failed-jobs.
// Real behavior: the new attempt re-runs ONLY the failed/cancelled
// jobs; jobs that succeeded (or were skipped) in the previous attempt
// carry their results over as already-completed.
func (s *Server) handleRerunFailedJobs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repo := wf.RepoFullName
	if repo == "" {
		repo = repoFullName(r)
	}
	match, err := s.cachedWorkflowFileForRun(repo, wf)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	def, perr := ParseWorkflow([]byte(match.YAML))
	if perr != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "parse cached YAML: "+perr.Error())
		return
	}
	def = expandMatrixJobs(def)
	if def.Env == nil {
		def.Env = map[string]string{}
	}
	serverURL := s.baseURL(r)
	def.Env["__serverURL"] = serverURL
	def.Env["__defaultImage"] = ""

	carryOver := map[string]*WorkflowJob{}
	s.store.mu.RLock()
	for key, j := range wf.Jobs {
		if j.Result == ResultSuccess || j.Result == ResultSkipped {
			carryOver[key] = j
		}
	}
	s.store.mu.RUnlock()

	if err := s.rerunWorkflowAsNewAttempt(r, wf, match, def, serverURL, carryOver); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "rerun submit: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleRunTiming returns the per-job billing-style timing summary.
func (s *Server) handleRunTiming(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
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
	// GitHub reports a per-job breakdown under billable.{OS}.job_runs (an
	// array of {job_id, duration_ms}), not just a count. job_id is the
	// stable GitHub-shape int64 ID workflowJobJSON exposes.
	jobRuns := make([]map[string]interface{}, 0, len(wf.Jobs))
	for _, j := range wf.Jobs {
		jobMs := int64(0)
		if !j.StartedAt.IsZero() {
			end := j.CompletedAt
			if end.IsZero() {
				end = time.Now()
			}
			jobMs = end.Sub(j.StartedAt).Milliseconds()
		}
		jobRuns = append(jobRuns, map[string]interface{}{
			"job_id":      stableJobID(j.JobID),
			"duration_ms": jobMs,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"billable": map[string]interface{}{
			"UBUNTU": map[string]interface{}{
				"total_ms": durationMs,
				"jobs":     len(wf.Jobs),
				"job_runs": jobRuns,
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
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
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
	deleted, err := s.artifactStore.deleteArtifact(r.Context(), art.ID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "artifact byte-store delete: "+err.Error())
		return
	}
	if !deleted {
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

// handleRunApprovals lists the deployment reviews submitted for a run
// (the review history; pending reviews live on /pending_deployments).
func (s *Server) handleRunApprovals(w http.ResponseWriter, r *http.Request) {
	repo, wf := s.lookupRunFromPath(r)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	base := s.baseURL(r)
	out := []map[string]interface{}{}
	s.store.mu.RLock()
	approvals := append([]*EnvApproval(nil), wf.EnvApprovals...)
	s.store.mu.RUnlock()
	for _, a := range approvals {
		envs := []map[string]interface{}{}
		for _, id := range a.EnvIDs {
			// The approvals schema nests the slim environment shape —
			// no protection_rules / deployment_branch_policy members.
			if env := s.store.Deployments.GetEnvironmentByID(id); env != nil {
				envs = append(envs, map[string]interface{}{
					"id":         env.ID,
					"node_id":    env.NodeID,
					"name":       env.Name,
					"url":        fmt.Sprintf("%s/api/v3/repos/%s/environments/%s", base, repo.FullName, env.Name),
					"html_url":   fmt.Sprintf("%s/%s/deployments/activity_log?environments_filter=%s", base, repo.FullName, env.Name),
					"created_at": env.CreatedAt.UTC().Format(time.RFC3339),
					"updated_at": env.UpdatedAt.UTC().Format(time.RFC3339),
				})
			}
		}
		var user map[string]interface{}
		s.store.mu.RLock()
		if u := s.store.Users[a.UserID]; u != nil {
			user = userToJSON(u)
		}
		s.store.mu.RUnlock()
		out = append(out, map[string]interface{}{
			"environments": envs,
			"state":        a.State,
			"comment":      a.Comment,
			"user":         user,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPendingDeployments lists the reviewer-protected environments a
// run is currently waiting on.
func (s *Server) handleGetPendingDeployments(w http.ResponseWriter, r *http.Request) {
	repo, wf := s.lookupRunFromPath(r)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	base := s.baseURL(r)
	out := []map[string]interface{}{}
	s.store.mu.RLock()
	pending := append([]*PendingDeployment(nil), wf.PendingDeployments...)
	s.store.mu.RUnlock()
	for _, p := range pending {
		env := s.store.Deployments.GetEnvironmentByID(p.EnvID)
		if env == nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"environment": map[string]interface{}{
				"id":       env.ID,
				"node_id":  env.NodeID,
				"name":     env.Name,
				"url":      fmt.Sprintf("%s/api/v3/repos/%s/environments/%s", base, repo.FullName, env.Name),
				"html_url": fmt.Sprintf("%s/%s/deployments/activity_log?environments_filter=%s", base, repo.FullName, env.Name),
			},
			"wait_timer":               env.WaitTimer,
			"wait_timer_started_at":    p.WaitTimerStartedAt.UTC().Format(time.RFC3339),
			"current_user_can_approve": true,
			"reviewers":                environmentReviewersJSON(env, s.store),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleReviewPendingDeployments approves or rejects a run's pending
// deployments. Approval releases the waiting jobs (and creates the
// deployments, which the response returns, matching real GitHub);
// rejection fails them.
func (s *Server) handleReviewPendingDeployments(w http.ResponseWriter, r *http.Request) {
	repo, wf := s.lookupRunFromPath(r)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var body struct {
		EnvironmentIDs []int  `json:"environment_ids"`
		State          string `json:"state"`
		Comment        string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Invalid request body")
		return
	}
	if body.State != "approved" && body.State != "rejected" {
		writeGHError(w, http.StatusUnprocessableEntity, "state must be approved or rejected")
		return
	}
	if len(body.EnvironmentIDs) == 0 {
		writeGHError(w, http.StatusUnprocessableEntity, "environment_ids is required")
		return
	}
	s.store.mu.RLock()
	pendingByID := map[int]bool{}
	for _, p := range wf.PendingDeployments {
		pendingByID[p.EnvID] = true
	}
	s.store.mu.RUnlock()
	for _, id := range body.EnvironmentIDs {
		if !pendingByID[id] {
			writeGHError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("environment %d has no pending deployment for this run", id))
			return
		}
	}

	reviewer := ghUserFromContext(r.Context())
	names := s.applyDeploymentReview(r.Context(), wf, body.EnvironmentIDs, body.State, body.Comment, reviewer)

	deployments := []map[string]interface{}{}
	if body.State == "approved" {
		creatorID := 0
		if reviewer != nil {
			creatorID = reviewer.ID
		}
		base := s.baseURL(r)
		for _, name := range names {
			d := s.store.Deployments.CreateDeployment(repo.ID, creatorID, wf.Ref, wf.Sha, "deploy", name, "", nil, false, false)
			deployments = append(deployments, deploymentToJSON(d, s.store, base, repo))
		}
	}
	writeJSON(w, http.StatusOK, deployments)
}

// lookupRunFromPath resolves the {owner}/{repo} + {run_id} path params to
// the repo and workflow run.
func (s *Server) lookupRunFromPath(r *http.Request) (*Repo, *Workflow) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		return nil, nil
	}
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		return repo, nil
	}
	wf := s.findWorkflowByRunID(runID)
	if wf == nil || !workflowBelongsToRepo(wf, repo.FullName) {
		return repo, nil
	}
	return repo, wf
}

// findWorkflowByRunID lives in gh_actions_rest.go alongside the rest of
// the workflow-run helpers; reused here.
