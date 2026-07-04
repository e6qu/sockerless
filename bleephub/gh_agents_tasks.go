package bleephub

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GitHub Copilot coding agent tasks — the /agents/tasks and
// /agents/repos/{owner}/{repo}/tasks surfaces. A task is created against
// a repository with a prompt; each task carries the session spawned for
// it. bleephub stores the task/session entities and their state; the
// Copilot coding agent's execution engine is not part of bleephub, so a
// created task stays "queued" (nothing dequeues it), exactly what the
// store knows to be true.

// AgentTaskSession is one Copilot coding agent session within a task.
type AgentTaskSession struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Prompt    string    `json:"prompt"`
	HeadRef   string    `json:"head_ref"`
	BaseRef   string    `json:"base_ref"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentTask is a Copilot coding agent task.
type AgentTask struct {
	ID          string             `json:"id"`
	RepoID      int                `json:"repo_id"`
	OwnerID     int                `json:"owner_id"`
	CreatorID   int                `json:"creator_id"`
	CreatorType string             `json:"creator_type"` // user | organization
	Name        string             `json:"name"`
	Prompt      string             `json:"prompt"`
	Model       string             `json:"model"`
	CreatePR    bool               `json:"create_pull_request"`
	BaseRef     string             `json:"base_ref"`
	HeadRef     string             `json:"head_ref"`
	State       string             `json:"state"`
	Sessions    []AgentTaskSession `json:"sessions"`
	ArchivedAt  *time.Time         `json:"archived_at"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

func (s *Server) registerGHAgentsTasksRoutes() {
	s.route("GET /api/v3/agents/tasks", s.handleListAgentTasks)
	s.route("GET /api/v3/agents/tasks/{task_id}", s.handleGetAgentTask)
	s.route("GET /api/v3/agents/repos/{owner}/{repo}/tasks", s.handleListAgentTasksForRepo)
	s.route("POST /api/v3/agents/repos/{owner}/{repo}/tasks", s.handleCreateAgentTaskInRepo)
	s.route("GET /api/v3/agents/repos/{owner}/{repo}/tasks/{task_id}", s.handleGetAgentTaskInRepo)
}

// --- store methods ---

// CreateAgentTask stores a new Copilot coding agent task for a repository
// with its initial session.
func (st *Store) CreateAgentTask(repo *Repo, creator *User, prompt, model string, createPR bool, baseRef, headRef string) *AgentTask {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()

	// The task name is derived from the first line of the prompt.
	name := prompt
	if idx := strings.IndexByte(name, '\n'); idx >= 0 {
		name = name[:idx]
	}
	if len(name) > 80 {
		name = name[:80]
	}

	task := &AgentTask{
		ID:          uuid.New().String(),
		RepoID:      repo.ID,
		OwnerID:     repo.OwnerID,
		CreatorID:   creator.ID,
		CreatorType: "user",
		Name:        name,
		Prompt:      prompt,
		Model:       model,
		CreatePR:    createPR,
		BaseRef:     baseRef,
		HeadRef:     headRef,
		State:       "queued",
		Sessions: []AgentTaskSession{{
			ID:        uuid.New().String(),
			Name:      name,
			State:     "queued",
			Prompt:    prompt,
			HeadRef:   headRef,
			BaseRef:   baseRef,
			Model:     model,
			CreatedAt: now,
			UpdatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	st.AgentTasks[task.ID] = task
	if st.persist != nil {
		st.persist.MustPut("agent_tasks", task.ID, task)
	}
	return task
}

// GetAgentTask returns a task by ID, or nil.
func (st *Store) GetAgentTask(id string) *AgentTask {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.AgentTasks[id]
}

// agentTaskFilter carries the documented list-tasks query filters.
type agentTaskFilter struct {
	repoID     int   // 0 = any repository
	creatorID  int   // 0 = any creator
	creatorIDs []int // non-empty = restrict to these creators
	states     []string
	isArchived bool
	since      *time.Time
	sortField  string // "updated_at" (default) | "created_at"
	direction  string // "desc" (default) | "asc"
}

// ListAgentTasks returns the tasks matching the filter, sorted, plus the
// active/archived totals within the filter's repo/creator scope.
func (st *Store) ListAgentTasks(f agentTaskFilter) (tasks []*AgentTask, totalActive, totalArchived int) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	for _, t := range st.AgentTasks {
		if f.repoID != 0 && t.RepoID != f.repoID {
			continue
		}
		if f.creatorID != 0 && t.CreatorID != f.creatorID {
			continue
		}
		if len(f.creatorIDs) > 0 {
			found := false
			for _, id := range f.creatorIDs {
				if t.CreatorID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if t.ArchivedAt != nil {
			totalArchived++
		} else {
			totalActive++
		}
		if f.isArchived != (t.ArchivedAt != nil) {
			continue
		}
		if len(f.states) > 0 {
			found := false
			for _, s := range f.states {
				if t.State == s {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if f.since != nil && t.UpdatedAt.Before(*f.since) {
			continue
		}
		tasks = append(tasks, t)
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		var less bool
		if f.sortField == "created_at" {
			less = tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		} else {
			less = tasks[i].UpdatedAt.Before(tasks[j].UpdatedAt)
		}
		if f.direction == "asc" {
			return less
		}
		return !less
	})
	return tasks, totalActive, totalArchived
}

// --- JSON rendering ---

func (s *Server) agentTaskJSON(t *AgentTask, baseURL string) map[string]interface{} {
	repo := s.store.GetRepoByID(t.RepoID)
	fullName := ""
	if repo != nil {
		fullName = repo.FullName
	}

	var archivedAt interface{}
	if t.ArchivedAt != nil {
		archivedAt = t.ArchivedAt.UTC().Format(time.RFC3339)
	}

	return map[string]interface{}{
		"id":            t.ID,
		"url":           baseURL + "/api/v3/agents/repos/" + fullName + "/tasks/" + t.ID,
		"html_url":      baseURL + "/" + fullName + "/copilot/tasks/" + t.ID,
		"name":          t.Name,
		"creator":       map[string]interface{}{"id": t.CreatorID},
		"creator_type":  t.CreatorType,
		"owner":         map[string]interface{}{"id": t.OwnerID},
		"repository":    map[string]interface{}{"id": t.RepoID},
		"state":         t.State,
		"session_count": len(t.Sessions),
		"artifacts":     []map[string]interface{}{},
		"archived_at":   archivedAt,
		"created_at":    t.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":    t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func agentTaskSessionJSON(t *AgentTask, sess AgentTaskSession) map[string]interface{} {
	out := map[string]interface{}{
		"id":         sess.ID,
		"name":       sess.Name,
		"owner":      map[string]interface{}{"id": t.OwnerID},
		"user":       map[string]interface{}{"id": t.CreatorID},
		"repository": map[string]interface{}{"id": t.RepoID},
		"task_id":    t.ID,
		"state":      sess.State,
		"prompt":     sess.Prompt,
		"created_at": sess.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": sess.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if sess.HeadRef != "" {
		out["head_ref"] = sess.HeadRef
	}
	if sess.BaseRef != "" {
		out["base_ref"] = sess.BaseRef
	}
	if sess.Model != "" {
		out["model"] = sess.Model
	}
	return out
}

func (s *Server) agentTaskDetailJSON(t *AgentTask, baseURL string) map[string]interface{} {
	out := s.agentTaskJSON(t, baseURL)
	sessions := make([]map[string]interface{}, 0, len(t.Sessions))
	for _, sess := range t.Sessions {
		sessions = append(sessions, agentTaskSessionJSON(t, sess))
	}
	out["sessions"] = sessions
	return out
}

// --- handlers ---

// parseAgentTaskFilter reads the documented list query parameters.
func parseAgentTaskFilter(r *http.Request) agentTaskFilter {
	q := r.URL.Query()
	f := agentTaskFilter{
		sortField: q.Get("sort"),
		direction: q.Get("direction"),
	}
	if v := q.Get("state"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				f.states = append(f.states, s)
			}
		}
	}
	if q.Get("is_archived") == "true" {
		f.isArchived = true
	}
	if v := q.Get("since"); v != "" {
		if ts, err := time.Parse(time.RFC3339, v); err == nil {
			f.since = &ts
		}
	}
	for _, raw := range q["creator_id"] {
		if id, err := strconv.Atoi(raw); err == nil {
			f.creatorIDs = append(f.creatorIDs, id)
		}
	}
	return f
}

func (s *Server) writeAgentTaskList(w http.ResponseWriter, r *http.Request, f agentTaskFilter) {
	tasks, totalActive, totalArchived := s.store.ListAgentTasks(f)
	page := paginateAndLink(w, r, tasks)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, t := range page {
		out = append(out, s.agentTaskJSON(t, baseURL))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tasks":                out,
		"total_active_count":   totalActive,
		"total_archived_count": totalArchived,
	})
}

// handleListAgentTasks lists the authenticated user's tasks.
func (s *Server) handleListAgentTasks(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	f := parseAgentTaskFilter(r)
	f.creatorID = user.ID
	s.writeAgentTaskList(w, r, f)
}

func (s *Server) handleGetAgentTask(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	task := s.store.GetAgentTask(r.PathValue("task_id"))
	if task == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repo := s.store.GetRepoByID(task.RepoID)
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.agentTaskDetailJSON(task, s.baseURL(r)))
}

func (s *Server) handleListAgentTasksForRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	f := parseAgentTaskFilter(r)
	f.repoID = repo.ID
	s.writeAgentTaskList(w, r, f)
}

func (s *Server) handleCreateAgentTaskInRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have write access to Repository.")
		return
	}

	var req struct {
		Prompt            string `json:"prompt"`
		Model             string `json:"model"`
		CreatePullRequest bool   `json:"create_pull_request"`
		BaseRef           string `json:"base_ref"`
		HeadRef           string `json:"head_ref"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Prompt == "" {
		writeGHValidationError(w, "AgentTask", "prompt", "missing_field")
		return
	}

	task := s.store.CreateAgentTask(repo, user, req.Prompt, req.Model, req.CreatePullRequest, req.BaseRef, req.HeadRef)
	writeJSON(w, http.StatusCreated, s.agentTaskJSON(task, s.baseURL(r)))
}

func (s *Server) handleGetAgentTaskInRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	task := s.store.GetAgentTask(r.PathValue("task_id"))
	if task == nil || task.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.agentTaskDetailJSON(task, s.baseURL(r)))
}
