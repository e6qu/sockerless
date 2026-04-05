package bleephub

import (
	"net/http"
	"sort"
)

func (s *Server) registerMgmtRoutes() {
	s.mux.HandleFunc("GET /internal/workflows", s.handleListWorkflows)
	s.mux.HandleFunc("GET /internal/workflows/{workflowId}", s.handleGetWorkflow)
	s.mux.HandleFunc("GET /internal/workflows/{workflowId}/logs", s.handleGetWorkflowLogs)
	s.mux.HandleFunc("GET /internal/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /internal/repos", s.handleListRepos)
}

// workflowView is the JSON representation of a workflow for the management API.
type workflowView struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	RunID        int                     `json:"runId"`
	Status       string                  `json:"status"`
	Result       string                  `json:"result"`
	CreatedAt    string                  `json:"createdAt"`
	EventName    string                  `json:"eventName,omitempty"`
	RepoFullName string                  `json:"repoFullName,omitempty"`
	Jobs         map[string]*WorkflowJob `json:"jobs"`
}

func workflowToView(wf *Workflow) workflowView {
	return workflowView{
		ID:           wf.ID,
		Name:         wf.Name,
		RunID:        wf.RunID,
		Status:       wf.Status,
		Result:       wf.Result,
		CreatedAt:    wf.CreatedAt.Format("2006-01-02T15:04:05Z"),
		EventName:    wf.EventName,
		RepoFullName: wf.RepoFullName,
		Jobs:         wf.Jobs,
	}
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	workflows := make([]workflowView, 0, len(s.store.Workflows))
	for _, wf := range s.store.Workflows {
		workflows = append(workflows, workflowToView(wf))
	}
	s.store.mu.RUnlock()

	// Sort by CreatedAt descending
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].CreatedAt > workflows[j].CreatedAt
	})

	writeJSON(w, http.StatusOK, workflows)
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("workflowId")

	s.store.mu.RLock()
	wf, ok := s.store.Workflows[id]
	if !ok {
		s.store.mu.RUnlock()
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	view := workflowToView(wf)
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetWorkflowLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("workflowId")

	s.store.mu.RLock()
	_, ok := s.store.Workflows[id]
	if !ok {
		s.store.mu.RUnlock()
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}

	// Collect log lines for all planIDs associated with this workflow's jobs
	logs := make(map[string][]string)
	for _, wf := range s.store.Workflows {
		if wf.ID != id {
			continue
		}
		for _, job := range wf.Jobs {
			if lines, exists := s.store.LogLines[job.JobID]; exists {
				logs[job.JobID] = lines
			}
		}
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, logs)
}

// sessionView is the JSON representation of a session for the management API.
type sessionView struct {
	SessionID       string `json:"sessionId"`
	OwnerName       string `json:"ownerName"`
	Agent           *Agent `json:"agent"`
	PendingMessages int    `json:"pendingMessages"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	sessions := make([]sessionView, 0, len(s.store.Sessions))
	for _, sess := range s.store.Sessions {
		pending := 0
		if sess.MsgCh != nil {
			pending = len(sess.MsgCh)
		}
		sessions = append(sessions, sessionView{
			SessionID:       sess.SessionID,
			OwnerName:       sess.OwnerName,
			Agent:           sess.Agent,
			PendingMessages: pending,
		})
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, sessions)
}

// repoView is the JSON representation of a repo for the management API.
type repoView struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
	Private       bool   `json:"private"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	repos := make([]repoView, 0, len(s.store.Repos))
	for _, repo := range s.store.Repos {
		repos = append(repos, repoView{
			ID:            repo.ID,
			Name:          repo.Name,
			FullName:      repo.FullName,
			Description:   repo.Description,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
			Private:       repo.Private,
			CreatedAt:     repo.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     repo.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, repos)
}
