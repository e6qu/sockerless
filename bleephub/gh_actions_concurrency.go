package bleephub

// GitHub Actions concurrency-group REST surface
// (/repos/{o}/{r}/actions/concurrency_groups and
// /repos/{o}/{r}/actions/runs/{run_id}/concurrency_groups): reflects the
// real concurrency state the workflow engine tracks — a run with a
// `concurrency:` group either holds the group's lease (status
// in_progress, position 0) or queues behind the holder (status pending,
// position 1+, ordered by submission time). Bleephub implements
// workflow-level concurrency; job-level leases don't exist.

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

func (s *Server) registerGHActionsConcurrencyRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/concurrency_groups", s.handleListConcurrencyGroups)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/concurrency_groups/{concurrency_group_name}", s.handleGetConcurrencyGroup)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/concurrency_groups", s.handleRunConcurrencyGroups)
}

// concurrencyGroupMember is one run holding or waiting for a group's
// lease. Position 0 = holder (in_progress); 1+ = queued (pending).
type concurrencyGroupMember struct {
	wf       *Workflow
	position int
}

func (m concurrencyGroupMember) status() string {
	if m.position == 0 {
		return "in_progress"
	}
	return "pending"
}

// concurrencyGroupsForRepo collects the repo's active concurrency
// groups: group name → ordered members (holder first, then pending runs
// by submission time).
func (s *Server) concurrencyGroupsForRepo(repo string) map[string][]concurrencyGroupMember {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	holders := map[string][]*Workflow{}
	pending := map[string][]*Workflow{}
	for _, wf := range s.store.Workflows {
		if wf.ConcurrencyGroup == "" || wf.RepoFullName != repo {
			continue
		}
		switch wf.Status {
		case WorkflowStatusRunning, WorkflowStatusWaiting:
			holders[wf.ConcurrencyGroup] = append(holders[wf.ConcurrencyGroup], wf)
		case WorkflowStatusPendingConcurrency:
			pending[wf.ConcurrencyGroup] = append(pending[wf.ConcurrencyGroup], wf)
		}
	}
	out := map[string][]concurrencyGroupMember{}
	for group, runs := range holders {
		sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })
		for _, wf := range runs {
			out[group] = append(out[group], concurrencyGroupMember{wf: wf, position: len(out[group])})
		}
	}
	for group, runs := range pending {
		sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.Before(runs[j].CreatedAt) })
		for _, wf := range runs {
			out[group] = append(out[group], concurrencyGroupMember{wf: wf, position: len(out[group])})
		}
	}
	// A group whose only members would start at position >= 1 (no holder)
	// still lists them as pending from position 1, matching the queue
	// semantics: position 0 is reserved for the lease holder.
	for group, members := range out {
		if len(members) > 0 && members[0].wf.Status == WorkflowStatusPendingConcurrency {
			for i := range members {
				members[i].position = i + 1
			}
			out[group] = members
		}
	}
	return out
}

func concurrencyGroupURL(baseURL, repo, group string) string {
	return fmt.Sprintf("%s/api/v3/repos/%s/actions/concurrency_groups/%s",
		baseURL, repo, url.PathEscape(group))
}

func runMemberJSON(m concurrencyGroupMember, baseURL, repo string) map[string]any {
	apiBase := fmt.Sprintf("%s/api/v3/repos/%s", baseURL, repo)
	htmlBase := fmt.Sprintf("%s/%s", baseURL, repo)
	return map[string]any{
		"run_id":       int64(m.wf.RunID),
		"run_name":     m.wf.Name,
		"run_url":      fmt.Sprintf("%s/actions/runs/%d", apiBase, m.wf.RunID),
		"run_html_url": fmt.Sprintf("%s/actions/runs/%d", htmlBase, m.wf.RunID),
		"status":       m.status(),
	}
}

// handleListConcurrencyGroups — GET .../actions/concurrency_groups.
// Lists the repo's active groups with the time the current holder
// acquired the lease (null while the group has only queued members).
func (s *Server) handleListConcurrencyGroups(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRepoReadable(w, r) {
		return
	}
	repo := repoFullName(r)
	groups := s.concurrencyGroupsForRepo(repo)
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	perPage := 30
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
			if perPage > 100 {
				perPage = 100
			}
		}
	}
	start := 0
	if after := r.URL.Query().Get("after"); after != "" {
		start = decodeCursor(after) + 1
		if start < 0 {
			start = 0
		}
		if start > len(names) {
			start = len(names)
		}
	}
	end := start + perPage
	if end > len(names) {
		end = len(names)
	}
	page := names[start:end]
	if end < len(names) {
		q := r.URL.Query()
		q.Set("after", encodeCursor(end-1))
		q.Set("per_page", strconv.Itoa(perPage))
		w.Header().Set("Link", fmt.Sprintf(`<%s?%s>; rel="next"`, r.URL.Path, q.Encode()))
	}

	base := s.baseURL(r)
	out := make([]map[string]any, 0, len(page))
	for _, name := range page {
		var lastAcquired any
		s.store.mu.RLock()
		for _, m := range groups[name] {
			if m.position == 0 && !m.wf.ConcurrencyAcquiredAt.IsZero() {
				lastAcquired = m.wf.ConcurrencyAcquiredAt.UTC().Format(time.RFC3339)
			}
		}
		s.store.mu.RUnlock()
		out = append(out, map[string]any{
			"group_name":       name,
			"group_url":        concurrencyGroupURL(base, repo, name),
			"last_acquired_at": lastAcquired,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":        len(names),
		"concurrency_groups": out,
	})
}

// handleGetConcurrencyGroup — GET .../actions/concurrency_groups/{name}.
// Returns the group's live members; 404 when the group has no active
// items. ?ahead_of_run= narrows to the members ahead of that run in the
// queue (422 when the run isn't a member).
func (s *Server) handleGetConcurrencyGroup(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRepoReadable(w, r) {
		return
	}
	repo := repoFullName(r)
	name := r.PathValue("concurrency_group_name")
	groups := s.concurrencyGroupsForRepo(repo)
	members, ok := groups[name]
	if !ok || len(members) == 0 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if aheadOf := r.URL.Query().Get("ahead_of_run"); aheadOf != "" {
		runID, err := strconv.Atoi(aheadOf)
		if err != nil {
			writeGHError(w, http.StatusUnprocessableEntity, "ahead_of_run must be a workflow run id")
			return
		}
		cut := -1
		for _, m := range members {
			if m.wf.RunID == runID {
				cut = m.position
				break
			}
		}
		if cut < 0 {
			writeGHError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("run %d is not a member of concurrency group %q", runID, name))
			return
		}
		ahead := members[:0:0]
		for _, m := range members {
			if m.position < cut {
				ahead = append(ahead, m)
			}
		}
		members = ahead
	} else if aheadOfJob := r.URL.Query().Get("ahead_of_job"); aheadOfJob != "" {
		// Bleephub implements workflow-level concurrency only; no job
		// holds a group lease, so no member can match a job id.
		writeGHError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("job %s is not a member of concurrency group %q", aheadOfJob, name))
		return
	}

	base := s.baseURL(r)
	out := make([]map[string]any, 0, len(members))
	s.store.mu.RLock()
	for _, m := range members {
		out = append(out, runMemberJSON(m, base, repo))
	}
	s.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"group_name":    name,
		"group_url":     concurrencyGroupURL(base, repo, name),
		"total_count":   len(out),
		"group_members": out,
	})
}

// handleRunConcurrencyGroups — GET .../runs/{run_id}/concurrency_groups.
// Lists the groups the run participates in by configuration; the run's
// member entry carries its live queue position (0 = holding the lease),
// and a completed run renders its group with no members (lease
// released).
func (s *Server) handleRunConcurrencyGroups(w http.ResponseWriter, r *http.Request) {
	if !s.enforceRepoReadable(w, r) {
		return
	}
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
	repo := repoFullName(r)
	base := s.baseURL(r)

	s.store.mu.RLock()
	group := wf.ConcurrencyGroup
	wfRepo := wf.RepoFullName
	s.store.mu.RUnlock()
	if wfRepo == "" {
		wfRepo = repo
	}

	groupsOut := []map[string]any{}
	if group != "" {
		groupURL := concurrencyGroupURL(base, wfRepo, group)
		memberOut := []map[string]any{}
		for _, m := range s.concurrencyGroupsForRepo(wfRepo)[group] {
			if m.wf.RunID != wf.RunID {
				continue
			}
			s.store.mu.RLock()
			entry := runMemberJSON(m, base, wfRepo)
			s.store.mu.RUnlock()
			entry["position"] = m.position
			entry["position_url"] = groupURL + "?ahead_of_run=" + strconv.Itoa(wf.RunID)
			memberOut = append(memberOut, entry)
		}
		groupsOut = append(groupsOut, map[string]any{
			"group_name":    group,
			"group_url":     groupURL,
			"group_members": memberOut,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":        len(groupsOut),
		"concurrency_groups": groupsOut,
	})
}
