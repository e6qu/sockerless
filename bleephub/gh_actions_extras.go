package bleephub

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Actions extras gh CLI / Octokit hit.
//   POST /repos/{o}/{r}/dispatches                          repository_dispatch
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/logs           run-level logs zip
//   POST /repos/{o}/{r}/actions/runs/{run_id}/rerun-failed-jobs
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/timing         per-job timing summary
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/artifacts      artifact list
//   GET  /repos/{o}/{r}/actions/artifacts                    repo-wide artifact list
//   GET  /repos/{o}/{r}/actions/runs/{run_id}/approvals      env-pending-approvals stub

func (s *Server) registerGHActionsExtrasRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/dispatches",
		s.requirePerm("contents", permWrite, s.handleRepositoryDispatch))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/logs",
		s.handleRunLogs)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs",
		s.requirePerm("actions", permWrite, s.handleRerunFailedJobs))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/timing",
		s.handleRunTiming)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/artifacts",
		s.handleRunArtifacts)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/artifacts",
		s.handleRepoArtifacts)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/approvals",
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

// handleRunArtifacts + handleRepoArtifacts — bleephub stores artifacts in
// ArtifactStore but doesn't index by run_id today. Returns empty list with
// real-GH shape; future work to wire run linkage.
func (s *Server) handleRunArtifacts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": 0,
		"artifacts":   []interface{}{},
	})
}

func (s *Server) handleRepoArtifacts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": 0,
		"artifacts":   []interface{}{},
	})
}

// handleRunApprovals — env-pending-approvals stub. Empty until Environments
// land.
func (s *Server) handleRunApprovals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []interface{}{})
}

// findWorkflowByRunID lives in gh_actions_rest.go alongside the rest of
// the workflow-run helpers; reused here.
