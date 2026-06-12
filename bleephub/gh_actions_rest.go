package bleephub

// GitHub-shape REST surface for workflow runs / jobs / runners that
// matches what the github-runner-dispatcher and unmodified `gh` CLI
// poll today. Bleephub's internal `/internal/workflows` surface
// already tracks every active Workflow + WorkflowJob in the in-memory
// store; this file exposes that state via the public GitHub paths so
// bleephub can stand in for real GitHub end-to-end.
//
// scope: actions/runs (with status filter),.../runs/{id},
// .../runs/{id}/jobs, .../jobs/{id}, .../jobs/{id}/logs, run cancel +
// rerun + delete, runners list + delete. Workflows REST + dispatch
// land in.

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHActionsRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs", s.handleListWorkflowRuns)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}", s.handleGetWorkflowRun)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/jobs", s.handleListWorkflowRunJobs)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}",
		s.requirePerm(scopeActions, permWrite, s.handleDeleteWorkflowRun))
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/cancel",
		s.requirePerm(scopeActions, permWrite, s.handleCancelWorkflowRun))
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/rerun",
		s.requirePerm(scopeActions, permWrite, s.handleRerunWorkflowRun))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}", s.handleGetRunAttempt)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}/jobs", s.handleListRunAttemptJobs)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/jobs/{job_id}", s.handleGetWorkflowJob)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/jobs/{job_id}/logs", s.handleGetWorkflowJobLogs)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runners", s.handleListRunners)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}", s.handleGetRunner)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}",
		s.requirePerm(scopeAdministration, permWrite, s.handleDeleteRunner))

	// Org-scoped runner surface: bleephub's pool is global, so the org
	// scope serves the same agents (404 only for unknown orgs).
	s.route("GET /api/v3/orgs/{org}/actions/runners", s.handleListRunners)
	s.route("GET /api/v3/orgs/{org}/actions/runners/{runner_id}", s.handleGetRunner)
	s.route("DELETE /api/v3/orgs/{org}/actions/runners/{runner_id}",
		s.requirePerm(scopeAdministration, permWrite, s.handleDeleteRunner))
	s.route("POST /api/v3/orgs/{org}/actions/runners/registration-token",
		s.requirePerm(scopeAdministration, permWrite, s.handleOrgRegistrationToken))
}

// handleOrgRegistrationToken mirrors the repo-scoped registration token
// at org scope (single global pool).
func (s *Server) handleOrgRegistrationToken(w http.ResponseWriter, r *http.Request) {
	if s.store.GetOrg(r.PathValue("org")) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.handleRegistrationToken(w, r)
}

// repoFullName returns "owner/repo" for the request's path params,
// matching the format Workflow.RepoFullName uses (set at submit time).
func repoFullName(r *http.Request) string {
	return r.PathValue("owner") + "/" + r.PathValue("repo")
}

// sortRunsNewestFirst orders runs the way real GitHub lists them
// (most recent run first). Run lists are collected from a map, so
// without an explicit sort the page boundaries shift between requests.
func sortRunsNewestFirst(runs []*Workflow) {
	sort.Slice(runs, func(i, j int) bool { return runs[i].RunID > runs[j].RunID })
}

// stableJobID maps a WorkflowJob's UUID to a stable int64 GitHub-shape
// `id`. GitHub uses int64 IDs for jobs everywhere; bleephub uses UUIDs
// internally. FNV-1a 64-bit gives a stable, collision-resistant int
// per UUID without modifying the existing WorkflowJob struct.
func stableJobID(uuid string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(uuid))
	// Mask sign bit so the result is a positive int64 the way GitHub
	// IDs are.
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// runStatus maps internal Workflow.Status → GitHub's three statuses
// (`queued`, `in_progress`, `completed`). Bleephub uses
// "running"/"completed"/"pending_concurrency" internally.
func runStatus(internal string) string {
	switch internal {
	case "completed":
		return "completed"
	case "running":
		return "in_progress"
	case "pending_concurrency":
		return "queued"
	case "waiting":
		return "waiting"
	default:
		return "queued"
	}
}

// runConclusion maps internal Workflow.Result → GitHub's nullable
// conclusion field (`success`, `failure`, `cancelled`, `skipped`,
// `timed_out`, etc.). Returned as nil for in-flight runs.
func runConclusion(status, result string) any {
	if status != "completed" {
		return nil
	}
	if result == "" {
		return "success"
	}
	return result
}

// jobStatus maps internal WorkflowJob.Status → GitHub status.
func jobStatus(internal string) string {
	switch internal {
	case "queued":
		return "queued"
	case "running":
		return "in_progress"
	case "completed", "skipped":
		return "completed"
	default:
		return "queued"
	}
}

func jobConclusion(status, result string) any {
	if status != "completed" {
		return nil
	}
	if result == "" {
		return "success"
	}
	return result
}

// workflowRunJSON converts a Workflow to GitHub's `WorkflowRun` shape.
// Fields cover what `gh run list` + `gh run view` + the
// runner-dispatcher's poll handler read; per-job + step detail comes
// from the .../jobs endpoints.

// runRepoJSON resolves the repository object embedded in workflow-run
// responses (required `repository` / `head_repository` members). Runs
// reference repos by full name; a run whose repo is gone renders null.
func (s *Server) runRepoJSON(fullName, baseURL string) map[string]interface{} {
	s.store.mu.RLock()
	repo := s.store.ReposByName[fullName]
	s.store.mu.RUnlock()
	if repo == nil {
		return nil
	}
	return repoToJSON(repo, s.store, baseURL)
}

func workflowRunJSON(wf *Workflow, baseURL, repoName string, repoJSON map[string]interface{}) map[string]any {
	repoPath := repoName
	if wf.RepoFullName != "" {
		repoPath = wf.RepoFullName
	}
	apiBase := fmt.Sprintf("%s/api/v3/repos/%s", baseURL, repoPath)
	htmlBase := fmt.Sprintf("%s/%s", baseURL, repoPath)
	status := runStatus(string(wf.Status))
	// workflow_id / workflow_url / path reference the originating workflow
	// FILE, which is stable across every run produced from it — never the
	// per-run RunID. Use the values resolved at submit/dispatch time; fall
	// back to a deterministic derivation for runs created without a backing
	// file (e.g. directly seeded in tests).
	fileID := wf.WorkflowFileID
	filePath := wf.WorkflowFilePath
	if filePath == "" {
		filePath = ".github/workflows/" + wf.Name + ".yml"
	}
	if fileID == 0 {
		fileID = stableWorkflowFileID(wf.RepoFullName, filePath)
	}
	created := wf.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	return map[string]any{
		"repository":           repoJSON,
		"head_repository":      repoJSON,
		"id":                   int64(wf.RunID),
		"name":                 wf.Name,
		"node_id":              "WFR_" + wf.ID,
		"head_branch":          headBranchOf(wf),
		"head_sha":             wf.Sha,
		"path":                 filePath,
		"display_title":        wf.Name,
		"run_number":           wf.RunNumber,
		"event":                eventOf(wf),
		"status":               status,
		"conclusion":           runConclusion(status, string(wf.Result)),
		"workflow_id":          fileID,
		"check_suite_id":       int64(wf.RunID),
		"check_suite_node_id":  "CS_" + wf.ID,
		"url":                  fmt.Sprintf("%s/actions/runs/%d", apiBase, wf.RunID),
		"html_url":             fmt.Sprintf("%s/actions/runs/%d", htmlBase, wf.RunID),
		"pull_requests":        []any{},
		"created_at":           created,
		"updated_at":           created,
		"actor":                runActorJSON(wf),
		"run_attempt":          wf.AttemptNumber(),
		"referenced_workflows": []any{},
		"run_started_at":       created,
		"triggering_actor":     runActorJSON(wf),
		"jobs_url":             fmt.Sprintf("%s/actions/runs/%d/jobs", apiBase, wf.RunID),
		"logs_url":             fmt.Sprintf("%s/actions/runs/%d/logs", apiBase, wf.RunID),
		"check_suite_url":      fmt.Sprintf("%s/check-suites/%d", apiBase, wf.RunID),
		"artifacts_url":        fmt.Sprintf("%s/actions/runs/%d/artifacts", apiBase, wf.RunID),
		"cancel_url":           fmt.Sprintf("%s/actions/runs/%d/cancel", apiBase, wf.RunID),
		"rerun_url":            fmt.Sprintf("%s/actions/runs/%d/rerun", apiBase, wf.RunID),
		"workflow_url":         fmt.Sprintf("%s/actions/workflows/%d", apiBase, fileID),
		"head_commit": map[string]any{
			"id":        wf.Sha,
			"tree_id":   wf.Sha,
			"message":   wf.Name,
			"timestamp": created,
			"author":    map[string]any{"name": "bleephub", "email": "actions@bleephub"},
			"committer": map[string]any{"name": "bleephub", "email": "actions@bleephub"},
		},
	}
}

// runActorJSON resolves the run's actor from the triggering event's
// sender payload (already user-shaped); nil when the run has no
// originating user (directly seeded runs).
func runActorJSON(wf *Workflow) any {
	if wf.EventPayload == nil {
		return nil
	}
	if sender, ok := wf.EventPayload["sender"].(map[string]interface{}); ok && sender != nil {
		return sender
	}
	return nil
}

func headBranchOf(wf *Workflow) string {
	if wf.Ref == "" {
		return "main"
	}
	return strings.TrimPrefix(wf.Ref, "refs/heads/")
}

func eventOf(wf *Workflow) string {
	if wf.EventName == "" {
		return "workflow_dispatch"
	}
	return wf.EventName
}

// workflowJobJSON converts a WorkflowJob to GitHub's `Job` shape. Step
// detail comes from the timeline records the runner reported for the
// job's plan.
func (s *Server) workflowJobJSON(wf *Workflow, wfJob *WorkflowJob, baseURL, repoName string) map[string]any {
	repoPath := repoName
	if wf.RepoFullName != "" {
		repoPath = wf.RepoFullName
	}
	apiBase := fmt.Sprintf("%s/api/v3/repos/%s", baseURL, repoPath)
	htmlBase := fmt.Sprintf("%s/%s", baseURL, repoPath)
	status := jobStatus(string(wfJob.Status))
	id := stableJobID(wfJob.JobID)
	startedAt := wfJob.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	// created_at is the queue time; bleephub records StartedAt at queue
	// (dispatchReadyJobs sets it when the job is marked queued), so it
	// doubles as the created timestamp.
	var completedAt any
	if status == "completed" {
		t := wfJob.CompletedAt
		if t.IsZero() {
			t = wfJob.StartedAt
		}
		completedAt = t.UTC().Format("2006-01-02T15:04:05Z")
	}
	return map[string]any{
		"id":                id,
		"run_id":            int64(wf.RunID),
		"workflow_name":     wf.Name,
		"head_branch":       headBranchOf(wf),
		"run_url":           fmt.Sprintf("%s/actions/runs/%d", apiBase, wf.RunID),
		"run_attempt":       1,
		"node_id":           "JOB_" + wfJob.JobID,
		"head_sha":          wf.Sha,
		"url":               fmt.Sprintf("%s/actions/jobs/%d", apiBase, id),
		"html_url":          fmt.Sprintf("%s/actions/runs/%d/job/%d", htmlBase, wf.RunID, id),
		"status":            status,
		"conclusion":        jobConclusion(status, string(wfJob.Result)),
		"created_at":        startedAt,
		"started_at":        startedAt,
		"completed_at":      completedAt,
		"name":              wfJob.DisplayName,
		"steps":             s.jobStepsJSON(wfJob),
		"check_run_url":     fmt.Sprintf("%s/check-runs/%d", apiBase, id),
		"labels":            labelsForJob(wfJob),
		"runner_id":         nil,
		"runner_name":       nil,
		"runner_group_id":   nil,
		"runner_group_name": nil,
	}
}

// jobStepsJSON renders the GitHub-shape `steps` array from the timeline
// records the runner uploaded for the job's plan (Type "Task", in Order).
// A job whose runner hasn't reported records yet has no step truth to
// serve, so the array is empty — step state is never fabricated.
func (s *Server) jobStepsJSON(wfJob *WorkflowJob) []map[string]any {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	tasks := s.taskRecordsForJobLocked(wfJob.JobID)
	steps := make([]map[string]any, 0, len(tasks))
	for i, rec := range tasks {
		steps = append(steps, map[string]any{
			"name":         rec.Name,
			"status":       stepStatus(rec.State),
			"conclusion":   stepConclusion(rec.State, rec.Result),
			"number":       i + 1,
			"started_at":   stepTimestamp(rec.StartTime),
			"completed_at": stepTimestamp(rec.FinishTime),
		})
	}
	return steps
}

// taskRecordsForJobLocked returns the job's "Task" (step) timeline records
// sorted by Order. Caller must hold store.mu.
func (s *Server) taskRecordsForJobLocked(jobUUID string) []*TimelineRecord {
	job := s.store.Jobs[jobUUID]
	if job == nil {
		return nil
	}
	var tasks []*TimelineRecord
	for _, rec := range s.store.TimelineRecords[job.PlanID] {
		if rec.Type == "Task" {
			tasks = append(tasks, rec)
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Order < tasks[j].Order })
	return tasks
}

// stepStatus maps the runner's timeline record state (pending |
// inProgress | completed) to GitHub's step status enum (queued |
// in_progress | completed).
func stepStatus(state string) string {
	switch state {
	case "inProgress":
		return "in_progress"
	case "completed":
		return "completed"
	default:
		return "queued"
	}
}

// stepConclusion maps the runner's timeline record result to GitHub's
// step conclusion; null until the step completes.
func stepConclusion(state, result string) any {
	if state != "completed" {
		return nil
	}
	switch result {
	case "succeeded", "succeededWithIssues":
		return "success"
	case "failed":
		return "failure"
	case "canceled", "abandoned":
		return "cancelled"
	case "skipped":
		return "skipped"
	default:
		return nil
	}
}

// stepTimestamp normalizes the runner's ISO-8601 timestamps (which carry
// fractional seconds) to GitHub's second-resolution RFC3339; null when
// the runner hasn't reported the time. A value that doesn't parse is
// passed through verbatim rather than dropped.
func stepTimestamp(ts string) any {
	if ts == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format("2006-01-02T15:04:05Z")
	}
	return ts
}

func labelsForJob(wfJob *WorkflowJob) []string {
	// JobDef.RunsOn is `interface{}` because YAML allows either a
	// scalar ("ubuntu-latest") or a sequence (["self-hosted", "linux"]).
	// Normalize both into the GitHub-shape `labels` array.
	if wfJob.Def == nil || wfJob.Def.RunsOn == nil {
		return []string{"ubuntu-latest"}
	}
	switch v := wfJob.Def.RunsOn.(type) {
	case string:
		return []string{v}
	case []string:
		if len(v) > 0 {
			return v
		}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{"ubuntu-latest"}
}

// runnerJSON converts a registered Agent to GitHub's `Runner` shape
// (`/repos/{o}/{r}/actions/runners`). GitHub's Runner.id is int64;
// bleephub Agent.ID is int — direct cast is safe.
func runnerJSON(a *Agent, busy bool) map[string]any {
	labels := make([]map[string]any, 0, len(a.Labels))
	for _, l := range a.Labels {
		labelType := "custom"
		if l.Type == "system" {
			labelType = "read-only"
		}
		labels = append(labels, map[string]any{
			"id":   l.ID,
			"name": l.Name,
			"type": labelType,
		})
	}
	return map[string]any{
		"id":              int64(a.ID),
		"runner_group_id": 1,
		"name":            a.Name,
		"os":              osFromDescription(a.OSDescription),
		"status":          agentStatusForRunner(a.Status),
		"busy":            busy,
		"ephemeral":       false,
		"version":         versionForRunner(a),
		"labels":          labels,
	}
}

// busyAgentIDsLocked returns the agents with an assigned, unfinished
// job (same predicate the broker uses to keep jobs away from busy
// runners). Callers hold the store lock.
func (s *Server) busyAgentIDsLocked() map[int]bool {
	busy := map[int]bool{}
	for _, j := range s.store.Jobs {
		if j.AgentID != 0 && j.Status != "completed" {
			busy[j.AgentID] = true
		}
	}
	return busy
}

// versionForRunner reports the agent's reported version, or nil when the
// agent never advertised one (GitHub renders absent versions as null).
func versionForRunner(a *Agent) any {
	if a.Version == "" {
		return nil
	}
	return a.Version
}

func osFromDescription(desc string) string {
	d := strings.ToLower(desc)
	switch {
	case strings.Contains(d, "linux"):
		return "linux"
	case strings.Contains(d, "windows"):
		return "windows"
	case strings.Contains(d, "darwin"), strings.Contains(d, "macos"):
		return "macos"
	default:
		return "linux"
	}
}

func agentStatusForRunner(internal string) string {
	if internal == "online" {
		return "online"
	}
	return "offline"
}

// findWorkflowByRunID looks up a workflow in the store by RunID.
// Returns nil if not present. Bleephub keys workflows by UUID
// internally; the GitHub-facing run_id is the int RunID.
func (s *Server) findWorkflowByRunID(runID int) *Workflow {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, wf := range s.store.Workflows {
		if wf.RunID == runID {
			return wf
		}
	}
	return nil
}

// findJobByStableID resolves the stable int64 GitHub-shape job ID
// back to (workflow, job). Returns (nil, nil) if no job in any
// workflow hashes to this ID.
func (s *Server) findJobByStableID(jobID int64) (*Workflow, *WorkflowJob) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, wf := range s.store.Workflows {
		for _, j := range wf.Jobs {
			if stableJobID(j.JobID) == jobID {
				return wf, j
			}
		}
	}
	return nil, nil
}

// handleListWorkflowRuns — GET /api/v3/repos/{owner}/{repo}/actions/runs
// Filters: ?status= (queued/in_progress/completed), ?branch=, ?event=,
// ?per_page=, ?page=. Returns `{total_count, workflow_runs:[...]}`
// matching the real GitHub paginated-list shape.
func (s *Server) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	statusFilter := r.URL.Query().Get("status")
	branchFilter := r.URL.Query().Get("branch")
	eventFilter := r.URL.Query().Get("event")

	s.store.mu.RLock()
	matching := []*Workflow{}
	for _, wf := range s.store.Workflows {
		if wf.RepoFullName != "" && wf.RepoFullName != repo {
			continue
		}
		if statusFilter != "" && runStatus(string(wf.Status)) != statusFilter {
			continue
		}
		if branchFilter != "" && headBranchOf(wf) != branchFilter {
			continue
		}
		if eventFilter != "" && eventOf(wf) != eventFilter {
			continue
		}
		matching = append(matching, wf)
	}
	s.store.mu.RUnlock()

	sortRunsNewestFirst(matching)
	page := paginateAndLink(w, r, matching)
	base := s.baseURL(r)
	runRepoJSON := s.runRepoJSON(repo, base)
	runs := make([]map[string]any, 0, len(page))
	for _, wf := range page {
		runs = append(runs, workflowRunJSON(wf, base, repo, runRepoJSON))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":   len(matching),
		"workflow_runs": runs,
	})
}

// handleGetWorkflowRun — GET .../actions/runs/{run_id}
func (s *Server) handleGetWorkflowRun(w http.ResponseWriter, r *http.Request) {
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
	base := s.baseURL(r)
	writeJSON(w, http.StatusOK, workflowRunJSON(wf, base, repoFullName(r), s.runRepoJSON(repoFullName(r), base)))
}

// handleListWorkflowRunJobs — GET .../actions/runs/{run_id}/jobs
// Real GitHub supports ?filter=latest|all (default latest, returns the
// most recent attempt's jobs). Bleephub doesn't track attempts so the
// filter is accepted but ignored.
func (s *Server) handleListWorkflowRunJobs(w http.ResponseWriter, r *http.Request) {
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
	s.store.mu.RLock()
	allJobs := make([]*WorkflowJob, 0, len(wf.Jobs))
	for _, j := range wf.Jobs {
		// Synthetic reusable-workflow gate/collector nodes are engine
		// bookkeeping; real GitHub lists only the called jobs.
		if j.Hidden {
			continue
		}
		allJobs = append(allJobs, j)
	}
	s.store.mu.RUnlock()

	page := paginateAndLink(w, r, allJobs)
	base := s.baseURL(r)
	repo := repoFullName(r)
	jobs := make([]map[string]any, 0, len(page))
	for _, j := range page {
		jobs = append(jobs, s.workflowJobJSON(wf, j, base, repo))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count": len(allJobs),
		"jobs":        jobs,
	})
}

// handleGetWorkflowJob — GET .../actions/jobs/{job_id}
func (s *Server) handleGetWorkflowJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(r.PathValue("job_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid job_id")
		return
	}
	wf, j := s.findJobByStableID(jobID)
	if j == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.workflowJobJSON(wf, j, s.baseURL(r), repoFullName(r)))
}

// handleGetWorkflowJobLogs — GET .../actions/jobs/{job_id}/logs
// Real GitHub returns text/plain logs (sometimes 302 to a pre-signed
// URL). Bleephub serves the complete log the runner uploaded when the
// job's timeline records reference log files, falling back to the live
// console capture in `store.LogLines`.
func (s *Server) handleGetWorkflowJobLogs(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(r.PathValue("job_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid job_id")
		return
	}
	_, j := s.findJobByStableID(jobID)
	if j == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	content := s.jobLogContentLocked(j.JobID)
	s.store.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// jobLogContentLocked assembles the job's complete log: the runner-
// uploaded log files referenced by the job's Task records, concatenated
// in step Order; when none were uploaded, the captured console lines.
// Caller must hold store.mu.
func (s *Server) jobLogContentLocked(jobUUID string) []byte {
	var buf bytes.Buffer
	for _, rec := range s.taskRecordsForJobLocked(jobUUID) {
		if rec.Log == nil {
			continue
		}
		content := s.store.LogFiles[rec.Log.ID]
		if len(content) == 0 {
			continue
		}
		buf.Write(content)
		if content[len(content)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	if buf.Len() > 0 {
		return buf.Bytes()
	}
	for _, line := range s.store.LogLines[jobUUID] {
		buf.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

// handleCancelWorkflowRun — POST .../actions/runs/{run_id}/cancel
// Real GitHub returns 202 Accepted with empty body when accepted.
func (s *Server) handleCancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
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
	s.cancelWorkflow(wf)
	w.WriteHeader(http.StatusAccepted)
}

// handleRerunWorkflowRun — POST .../actions/runs/{run_id}/rerun.
// Real GitHub: 201 Created. Bleephub re-submits the run by looking up
// the matching WorkflowFile (by name + repo) and replaying its cached
// YAML through submitWorkflow with the original event metadata.
// Returns 422 if no cached YAML exists (-or-later WorkflowFile
// not registered for this run) — caller should re-submit via
// /api/v3/bleephub/workflow or push the YAML to git.
func (s *Server) handleRerunWorkflowRun(w http.ResponseWriter, r *http.Request) {
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
	repo := wf.RepoFullName
	if repo == "" {
		repo = repoFullName(r)
	}
	s.store.DiscoverWorkflowFilesFromGit(repo)
	var match *WorkflowFile
	for _, f := range s.store.ListWorkflowFiles(repo) {
		if f.Name == wf.Name && f.YAML != "" {
			match = f
			break
		}
	}
	if match == nil {
		writeGHError(w, http.StatusUnprocessableEntity,
			"no cached workflow YAML for this run (push the workflow file to git or POST /api/v3/bleephub/workflow first)")
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
	def.Env["__defaultImage"] = "alpine:latest"
	if err := s.rerunWorkflowAsNewAttempt(r, wf, def, serverURL, nil); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "rerun submit: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// rerunWorkflowAsNewAttempt archives the current run as a prior attempt
// and re-submits the parsed definition under the SAME run id with
// run_attempt+1 (real GitHub never mints a new run id for a re-run).
// carryOver pre-completes the listed job keys with the previous
// attempt's results (rerun-failed-jobs).
func (s *Server) rerunWorkflowAsNewAttempt(r *http.Request, old *Workflow, def *WorkflowDef, serverURL string, carryOver map[string]*WorkflowJob) error {
	// Archive + remove the old attempt first; restore on submit failure.
	s.store.mu.Lock()
	s.store.WorkflowAttempts[old.RunID] = append(s.store.WorkflowAttempts[old.RunID], old)
	delete(s.store.Workflows, old.ID)
	s.store.mu.Unlock()
	if old.cancelTimeout != nil {
		old.cancelTimeout()
	}

	meta := WorkflowEventMeta{
		EventName:     eventOf(old),
		Ref:           old.Ref,
		Sha:           old.Sha,
		Repo:          old.RepoFullName,
		Inputs:        old.Inputs,
		TypedInputs:   old.TypedInputs,
		Payload:       old.EventPayload,
		ReuseRunID:    old.RunID,
		Attempt:       old.AttemptNumber() + 1,
		CarryOverJobs: carryOver,
	}
	if _, err := s.submitWorkflow(r.Context(), serverURL, def, "alpine:latest", &meta); err != nil {
		// Put the old attempt back so the run doesn't vanish.
		s.store.mu.Lock()
		attempts := s.store.WorkflowAttempts[old.RunID]
		if n := len(attempts); n > 0 && attempts[n-1] == old {
			s.store.WorkflowAttempts[old.RunID] = attempts[:n-1]
		}
		s.store.Workflows[old.ID] = old
		s.store.mu.Unlock()
		return err
	}
	return nil
}

// findRunAttempt resolves a run's specific attempt: the live run when
// attempt matches its number, else the archived attempt.
func (s *Server) findRunAttempt(runID, attempt int) *Workflow {
	current := s.findWorkflowByRunID(runID)
	if current != nil && current.AttemptNumber() == attempt {
		return current
	}
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, archived := range s.store.WorkflowAttempts[runID] {
		if archived.AttemptNumber() == attempt {
			return archived
		}
	}
	return nil
}

// handleGetRunAttempt — GET .../actions/runs/{run_id}/attempts/{attempt_number}
func (s *Server) handleGetRunAttempt(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	attempt, err := strconv.Atoi(r.PathValue("attempt_number"))
	if err != nil || attempt < 1 {
		writeGHError(w, http.StatusBadRequest, "invalid attempt_number")
		return
	}
	wf := s.findRunAttempt(runID, attempt)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	base := s.baseURL(r)
	repo := repoFullName(r)
	writeJSON(w, http.StatusOK, workflowRunJSON(wf, base, repo, s.runRepoJSON(repo, base)))
}

// handleListRunAttemptJobs — GET .../runs/{run_id}/attempts/{attempt_number}/jobs
func (s *Server) handleListRunAttemptJobs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	attempt, err := strconv.Atoi(r.PathValue("attempt_number"))
	if err != nil || attempt < 1 {
		writeGHError(w, http.StatusBadRequest, "invalid attempt_number")
		return
	}
	wf := s.findRunAttempt(runID, attempt)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	allJobs := make([]*WorkflowJob, 0, len(wf.Jobs))
	for _, j := range wf.Jobs {
		if j.Hidden {
			continue
		}
		allJobs = append(allJobs, j)
	}
	s.store.mu.RUnlock()
	page := paginateAndLink(w, r, allJobs)
	base := s.baseURL(r)
	repo := repoFullName(r)
	jobs := make([]map[string]any, 0, len(page))
	for _, j := range page {
		jobs = append(jobs, s.workflowJobJSON(wf, j, base, repo))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count": len(allJobs),
		"jobs":        jobs,
	})
}

// handleDeleteWorkflowRun — DELETE .../actions/runs/{run_id}
// Real GitHub returns 204 No Content. Bleephub deletes the workflow
// entry from the in-memory store.
func (s *Server) handleDeleteWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	s.store.mu.Lock()
	var foundKey string
	for k, wf := range s.store.Workflows {
		if wf.RunID == runID {
			foundKey = k
			break
		}
	}
	if foundKey == "" {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	delete(s.store.Workflows, foundKey)
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// handleListRunners — GET .../actions/runners
// Returns every registered agent. Real GitHub scopes runners to the
// repo (or org); bleephub's agents are global today, so all are
// returned regardless of repo path. The path scoping is preserved for
// future per-repo runner pools.
func (s *Server) handleListRunners(w http.ResponseWriter, r *http.Request) {
	// Org-scoped listings 404 for unknown orgs; the pool itself is
	// global (bleephub has a single runner pool).
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	all := make([]*Agent, 0, len(s.store.Agents))
	for _, a := range s.store.Agents {
		all = append(all, a)
	}
	busy := s.busyAgentIDsLocked()
	s.store.mu.RUnlock()

	page := paginateAndLink(w, r, all)
	runners := make([]map[string]any, 0, len(page))
	for _, a := range page {
		runners = append(runners, runnerJSON(a, busy[a.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count": len(all),
		"runners":     runners,
	})
}

// handleGetRunner — GET .../actions/runners/{runner_id} (repo + org scope).
func (s *Server) handleGetRunner(w http.ResponseWriter, r *http.Request) {
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	a := s.store.Agents[id]
	busy := s.busyAgentIDsLocked()
	s.store.mu.RUnlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, runnerJSON(a, busy[a.ID]))
}

// handleDeleteRunner — DELETE .../actions/runners/{runner_id}
// Real GitHub returns 204 No Content. Symmetric with the existing
// agent-CRUD path on `_apis/v1/Agent/{poolId}/{agentId}`.
func (s *Server) handleDeleteRunner(w http.ResponseWriter, r *http.Request) {
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	runnerID, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid runner_id")
		return
	}
	s.store.mu.Lock()
	if _, ok := s.store.Agents[runnerID]; !ok {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	delete(s.store.Agents, runnerID)
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
