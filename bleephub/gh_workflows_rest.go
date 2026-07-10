package bleephub

// workflow-file REST surface (`/api/v3/repos/{o}/{r}/actions/workflows`).
// The run-level state lives at `actions/runs`; this file covers the YAML
// files themselves so `gh workflow list` + `gh workflow run` + the
// GitHub UI's workflow-dispatch form work against bleephub.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHWorkflowsRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/workflows", s.handleListGHWorkflows)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}", s.handleGetGHWorkflow)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs", s.handleListWorkflowFileRuns)
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches",
		s.requirePerm(scopeActions, permWrite, s.handleDispatchWorkflow))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/enable",
		s.requirePerm(scopeActions, permWrite, s.handleSetWorkflowState("active")))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/disable",
		s.requirePerm(scopeActions, permWrite, s.handleSetWorkflowState("disabled_manually")))
}

// handleSetWorkflowState backs PUT .../workflows/{id}/{enable,disable}:
// flips the workflow FILE's state (persisted) and 204s. Disabled
// workflows neither trigger (webhooks.go workflowFileDisabled) nor
// dispatch (403 below).
func (s *Server) handleSetWorkflowState(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo := repoFullName(r)
		s.store.DiscoverWorkflowFilesFromGit(repo)
		wf := s.resolveWorkflowFile(repo, r.PathValue("workflow_id"))
		if wf == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		s.store.mu.Lock()
		wf.State = state
		wf.UpdatedAt = time.Now().UTC()
		if s.store.persist != nil {
			s.store.persist.MustPut("workflow_files", strconv.FormatInt(wf.ID, 10), wf)
		}
		s.store.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}
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
		if statusFilter != "" && runStatus(run) != statusFilter {
			continue
		}
		if branchFilter != "" && headBranchOf(run) != branchFilter {
			continue
		}
		matching = append(matching, run)
	}
	s.store.mu.RUnlock()

	sortRunsNewestFirst(matching)
	page := paginateAndLink(w, r, matching)
	base := s.baseURL(r)
	runRepoJSON := s.runRepoJSON(repo, base)
	runs := make([]map[string]any, 0, len(page))
	for _, run := range page {
		runs = append(runs, workflowRunJSON(run, base, repo, runRepoJSON))
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
	if strings.HasPrefix(wf.State, "disabled") {
		// Real GitHub: 403 "Workflow does not have 'workflow_dispatch'
		// trigger" variant for disabled workflows is "Workflow is disabled".
		writeGHError(w, http.StatusForbidden, "Workflow is disabled")
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
		defaultBranch := "main"
		if repoObj := s.store.ReposByName[repo]; repoObj != nil && repoObj.DefaultBranch != "" {
			defaultBranch = repoObj.DefaultBranch
		}
		req.Ref = defaultBranch
	}
	repoParts := splitRepoKeyParts(repo)
	stor := s.store.GetGitStorage(repoParts[0], repoParts[1])
	if stor == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository git storage is not available")
		return
	}
	resolvedRef, sha := resolveGitHubRefInput(stor, req.Ref)
	if sha == "0000000000000000000000000000000000000000" {
		writeGHError(w, http.StatusUnprocessableEntity, "No ref found for: "+req.Ref)
		return
	}
	req.Ref = resolvedRef

	// Validate against the workflow's declared workflow_dispatch inputs:
	// unknown inputs reject, required inputs must arrive, declared
	// defaults apply, choice options and boolean values are enforced —
	// matching real GitHub's 422s.
	on, err := ParseWorkflowOn([]byte(wf.YAML))
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "parse workflow on: "+err.Error())
		return
	}
	dispatchDef, hasDispatch := on["workflow_dispatch"]
	if !hasDispatch {
		writeGHError(w, http.StatusUnprocessableEntity, "Workflow does not have 'workflow_dispatch' trigger")
		return
	}
	inputs, typedInputs, errMsg := resolveDispatchInputs(dispatchDef, req.Inputs)
	if errMsg != "" {
		writeGHError(w, http.StatusUnprocessableEntity, errMsg)
		return
	}
	req.Inputs = inputs

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
	def.Env["__defaultImage"] = ""

	// The workflow_dispatch event payload carries the string-typed
	// inputs (github.event.inputs), the ref, and the workflow path.
	eventInputs := make(map[string]interface{}, len(req.Inputs))
	for k, v := range req.Inputs {
		eventInputs[k] = v
	}
	payload := map[string]interface{}{
		"inputs":   eventInputs,
		"ref":      req.Ref,
		"workflow": wf.Path,
	}
	if user := ghUserFromContext(r.Context()); user != nil {
		payload["sender"] = senderPayload(user)
	}
	if repoObj := s.store.ReposByName[repo]; repoObj != nil {
		payload["repository"] = repoPayload(repoObj)
	}

	meta := WorkflowEventMeta{
		EventName:   "workflow_dispatch",
		Ref:         req.Ref,
		Sha:         sha,
		Repo:        repo,
		Inputs:      req.Inputs,
		TypedInputs: typedInputs,
		Payload:     payload,
	}
	if _, err := s.submitWorkflow(r.Context(), serverURL, def, "", &meta); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "submit: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveDispatchInputs validates caller inputs against the workflow's
// workflow_dispatch declarations and applies defaults. It returns the
// string form (github.event.inputs), the typed form (the `inputs`
// expression context, where boolean/number inputs carry real types),
// and a GitHub-cased wire error message ("" when valid).
func resolveDispatchInputs(td *TriggerDef, given map[string]string) (map[string]string, map[string]interface{}, string) {
	inputs := make(map[string]string, len(given))
	var declared map[string]*WorkflowInputDef
	if td != nil {
		declared = td.Inputs
	}
	for name, val := range given {
		if _, ok := declared[name]; !ok {
			return nil, nil, fmt.Sprintf("Unexpected inputs provided: [%q]", name)
		}
		inputs[name] = val
	}
	typed := make(map[string]interface{}, len(declared))
	for name, def := range declared {
		val, gotten := inputs[name]
		if !gotten {
			if def.Default != nil {
				val = exprToString(normalizeYAMLValue(def.Default))
				inputs[name] = val
			} else if def.Required {
				return nil, nil, fmt.Sprintf("Required input %q not provided", name)
			} else {
				if def.Type == "boolean" {
					// Undefaulted booleans are false on real GitHub.
					val = "false"
					inputs[name] = val
				} else {
					typed[name] = ""
					continue
				}
			}
		}
		switch def.Type {
		case "boolean":
			switch val {
			case "true":
				typed[name] = true
			case "false":
				typed[name] = false
			default:
				return nil, nil, fmt.Sprintf("Input %q must be 'true' or 'false'", name)
			}
		case "number":
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return nil, nil, fmt.Sprintf("Input %q must be a number", name)
			}
			typed[name] = f
		case "choice":
			ok := false
			for _, opt := range def.Options {
				if exprToString(normalizeYAMLValue(opt)) == val {
					ok = true
					break
				}
			}
			if !ok {
				return nil, nil, fmt.Sprintf("Input %q does not match any of the allowed options", name)
			}
			typed[name] = val
		default:
			typed[name] = val
		}
	}
	return inputs, typed, ""
}
