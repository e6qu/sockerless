package bleephub

// Phase 131 — workflow-file REST surface (`/api/v3/repos/{o}/{r}/actions/workflows`).
// Phase 130 covered run-level state (`actions/runs`); this file covers
// the YAML files themselves so `gh workflow list` + `gh workflow run`
// + the GitHub UI's workflow-dispatch form work against bleephub.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) registerGHWorkflowsRoutes() {
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/workflows", s.handleListGHWorkflows)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}", s.handleGetGHWorkflow)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs", s.handleListWorkflowFileRuns)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches", s.handleDispatchWorkflow)
}

// workflowFileJSON converts a WorkflowFile to GitHub's `Workflow`
// shape. `gh workflow list` reads name/path/state/url/html_url; the
// UI dispatch form additionally surfaces created_at/updated_at.
func workflowFileJSON(wf *WorkflowFile, baseURL, repoName string) map[string]any {
	repoPath := repoName
	if wf.RepoFullName != "" {
		repoPath = wf.RepoFullName
	}
	apiBase := fmt.Sprintf("%s/api/v3/repos/%s", baseURL, repoPath)
	htmlBase := fmt.Sprintf("%s/%s", baseURL, repoPath)
	badge := fmt.Sprintf("%s/actions/workflows/%s/badge.svg", htmlBase, lastPathSegment(wf.Path))
	return map[string]any{
		"id":         wf.ID,
		"node_id":    wf.NodeID,
		"name":       wf.Name,
		"path":       wf.Path,
		"state":      wf.State,
		"created_at": wf.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"updated_at": wf.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"url":        fmt.Sprintf("%s/actions/workflows/%d", apiBase, wf.ID),
		"html_url":   fmt.Sprintf("%s/actions/workflows/%s", htmlBase, lastPathSegment(wf.Path)),
		"badge_url":  badge,
	}
}

func lastPathSegment(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// handleListGHWorkflows — GET /api/v3/repos/{o}/{r}/actions/workflows.
// Discovers from git storage on every call (cheap; the discovery
// re-registers entries idempotently so push-time updates are visible
// immediately) THEN lists every WorkflowFile registered for the repo
// (includes both "discovered" and "submitted" sources).
func (s *Server) handleListGHWorkflows(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	s.store.DiscoverWorkflowFilesFromGit(repo)
	files := s.store.ListWorkflowFiles(repo)
	page := paginateAndLink(w, r, files)
	base := s.baseURL(r)
	workflows := make([]map[string]any, 0, len(page))
	for _, f := range page {
		workflows = append(workflows, workflowFileJSON(f, base, repo))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count": len(files),
		"workflows":   workflows,
	})
}

// handleGetGHWorkflow — GET .../actions/workflows/{workflow_id}.
// `workflow_id` may be either the numeric ID or the file path
// (`ci.yml`) per real GitHub. Resolution order: numeric → exact
// path → basename match.
func (s *Server) handleGetGHWorkflow(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	s.store.DiscoverWorkflowFilesFromGit(repo)
	wf := s.resolveWorkflowFile(repo, r.PathValue("workflow_id"))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, workflowFileJSON(wf, s.baseURL(r), repo))
}

// resolveWorkflowFile accepts the GitHub-shape `workflow_id` path
// param (numeric ID or filename) and returns the matching WorkflowFile
// or nil. Numeric ID is the canonical form; filename is the
// developer-ergonomic shortcut `gh workflow run` uses.
func (s *Server) resolveWorkflowFile(repoFullName, idOrPath string) *WorkflowFile {
	if id, err := strconv.ParseInt(idOrPath, 10, 64); err == nil {
		if wf := s.store.GetWorkflowFile(repoFullName, id); wf != nil {
			return wf
		}
	}
	for _, wf := range s.store.ListWorkflowFiles(repoFullName) {
		if wf.Path == idOrPath {
			return wf
		}
		if lastPathSegment(wf.Path) == idOrPath {
			return wf
		}
	}
	return nil
}

// handleListWorkflowFileRuns — GET .../actions/workflows/{id}/runs.
// Filters the existing run-level Workflows by repo + workflow name
// (matching the WorkflowFile's name). Reuses workflowRunJSON from
// gh_actions_rest.go so the response shape matches the run-list
// endpoint's exactly.
func (s *Server) handleListWorkflowFileRuns(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	s.store.DiscoverWorkflowFilesFromGit(repo)
	wf := s.resolveWorkflowFile(repo, r.PathValue("workflow_id"))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	statusFilter := r.URL.Query().Get("status")
	branchFilter := r.URL.Query().Get("branch")

	s.store.mu.RLock()
	matching := []*Workflow{}
	for _, run := range s.store.Workflows {
		if run.RepoFullName != "" && run.RepoFullName != repo {
			continue
		}
		if run.Name != wf.Name {
			continue
		}
		if statusFilter != "" && runStatus(run.Status) != statusFilter {
			continue
		}
		if branchFilter != "" && headBranchOf(run) != branchFilter {
			continue
		}
		matching = append(matching, run)
	}
	s.store.mu.RUnlock()

	page := paginateAndLink(w, r, matching)
	base := s.baseURL(r)
	runs := make([]map[string]any, 0, len(page))
	for _, run := range page {
		runs = append(runs, workflowRunJSON(run, base, repo))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":   len(matching),
		"workflow_runs": runs,
	})
}

// handleDispatchWorkflow — POST .../actions/workflows/{id}/dispatches.
// Real GitHub returns 204 No Content on accept. Body shape:
//
//	{ "ref": "main", "inputs": { "name": "value" } }
//
// Bleephub re-submits the cached YAML through submitWorkflow with the
// caller's ref + inputs. If the workflow file's YAML wasn't cached
// (discovered file with empty body, etc.), respond 422 with a clear
// message instead of submitting an empty workflow.
func (s *Server) handleDispatchWorkflow(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	s.store.DiscoverWorkflowFilesFromGit(repo)
	wf := s.resolveWorkflowFile(repo, r.PathValue("workflow_id"))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if wf.YAML == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "workflow YAML body not cached for this file (re-push to git or re-submit via /api/v3/bleephub/workflow)")
		return
	}

	var req struct {
		Ref    string            `json:"ref"`
		Inputs map[string]string `json:"inputs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Ref == "" {
		req.Ref = "refs/heads/main"
	}

	def, err := ParseWorkflow([]byte(wf.YAML))
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "parse workflow YAML: "+err.Error())
		return
	}
	def = expandMatrixJobs(def)
	if def.Env == nil {
		def.Env = map[string]string{}
	}
	serverURL := s.baseURL(r)
	def.Env["__serverURL"] = serverURL
	def.Env["__defaultImage"] = "alpine:latest"

	meta := WorkflowEventMeta{
		EventName: "workflow_dispatch",
		Ref:       req.Ref,
		Sha:       "0000000000000000000000000000000000000000",
		Repo:      repo,
		Inputs:    req.Inputs,
	}
	if _, err := s.submitWorkflow(r.Context(), serverURL, def, "alpine:latest", &meta); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "submit: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
