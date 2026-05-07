package bleephub

import (
	"net/http"
	"sort"
)

func (s *Server) registerMgmtRoutes() {
	s.mux.HandleFunc("GET /internal/workflows", s.handleListWorkflows)
	s.mux.HandleFunc("GET /internal/workflows/{workflowId}", s.handleGetWorkflow)
	s.mux.HandleFunc("GET /internal/workflows/{workflowId}/logs", s.handleGetWorkflowLogs)
	s.mux.HandleFunc("GET /internal/workflow_files", s.handleListWorkflowFilesInternal)
	s.mux.HandleFunc("GET /internal/apps", s.handleListAppsInternal)
	s.mux.HandleFunc("GET /internal/installations", s.handleListInstallationsInternal)
	s.mux.HandleFunc("GET /internal/oauth/state", s.handleOAuthStateInternal)
	s.mux.HandleFunc("GET /internal/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /internal/repos", s.handleListRepos)
}

// appView / installationView / oauthState — operator-facing admin
// shapes for the bleephub UI Apps Manager + OAuth Debug pages. Real
// GitHub doesn't expose these aggregations (they're internal sim
// state, not part of the public REST surface).

type appView struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     int    `json:"ownerId"`
	CreatedAt   string `json:"createdAt"`
}

type installationViewMgmt struct {
	ID                  int    `json:"id"`
	AppID               int    `json:"appId"`
	AppSlug             string `json:"appSlug"`
	TargetType          string `json:"targetType"`
	TargetLogin         string `json:"targetLogin"`
	RepositorySelection string `json:"repositorySelection"`
	CreatedAt           string `json:"createdAt"`
}

type oauthStateView struct {
	DeviceCodes []deviceCodeView `json:"deviceCodes"`
	AuthCodes   []authCodeView   `json:"authCodes"`
}

type deviceCodeView struct {
	Code      string `json:"code"`
	UserCode  string `json:"userCode"`
	Scopes    string `json:"scopes"`
	UserID    int    `json:"userId"`
	ExpiresAt string `json:"expiresAt"`
}

type authCodeView struct {
	Code        string `json:"code"`
	ClientID    string `json:"clientId"`
	RedirectURI string `json:"redirectUri"`
	Scopes      string `json:"scopes"`
	State       string `json:"state"`
	UserID      int    `json:"userId"`
	CreatedAt   string `json:"createdAt"`
	ExpiresAt   string `json:"expiresAt"`
}

func (s *Server) handleListAppsInternal(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	apps := make([]appView, 0, len(s.store.Apps))
	for _, app := range s.store.Apps {
		apps = append(apps, appView{
			ID:          app.ID,
			Slug:        app.Slug,
			Name:        app.Name,
			Description: app.Description,
			OwnerID:     app.OwnerID,
			CreatedAt:   app.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) handleListInstallationsInternal(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	installs := make([]installationViewMgmt, 0, len(s.store.Installations))
	for _, inst := range s.store.Installations {
		installs = append(installs, installationViewMgmt{
			ID:                  inst.ID,
			AppID:               inst.AppID,
			AppSlug:             inst.AppSlug,
			TargetType:          inst.TargetType,
			TargetLogin:         inst.TargetLogin,
			RepositorySelection: inst.RepositorySelection,
			CreatedAt:           inst.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()
	sort.Slice(installs, func(i, j int) bool { return installs[i].ID < installs[j].ID })
	writeJSON(w, http.StatusOK, installs)
}

func (s *Server) handleOAuthStateInternal(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	dcs := make([]deviceCodeView, 0, len(s.store.DeviceCodes))
	for _, dc := range s.store.DeviceCodes {
		dcs = append(dcs, deviceCodeView{
			Code:      dc.Code,
			UserCode:  dc.UserCode,
			Scopes:    dc.Scopes,
			UserID:    dc.UserID,
			ExpiresAt: dc.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	acs := make([]authCodeView, 0, len(s.store.AuthCodes))
	for _, ac := range s.store.AuthCodes {
		acs = append(acs, authCodeView{
			Code:        ac.Code,
			ClientID:    ac.ClientID,
			RedirectURI: ac.RedirectURI,
			Scopes:      ac.Scopes,
			State:       ac.State,
			UserID:      ac.UserID,
			CreatedAt:   ac.CreatedAt.Format("2006-01-02T15:04:05Z"),
			ExpiresAt:   ac.ExpiresAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, oauthStateView{DeviceCodes: dcs, AuthCodes: acs})
}

// workflowFileView is the operator-facing aggregate shape of every
// registered WorkflowFile across every repo. The bleephub UI's
// Workflows tab reads this; the per-repo GitHub-shape endpoints
// (`/api/v3/repos/{o}/{r}/actions/workflows`) are for the gh CLI +
// runner-dispatcher.
type workflowFileView struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	State        string `json:"state"`
	RepoFullName string `json:"repoFullName"`
	Source       string `json:"source"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (s *Server) handleListWorkflowFilesInternal(w http.ResponseWriter, r *http.Request) {
	// Discover from every repo's git storage so files newly pushed
	// since last poll show up. Cheap — the discovery is idempotent.
	s.store.mu.RLock()
	repoNames := make([]string, 0, len(s.store.Repos))
	for _, repo := range s.store.Repos {
		repoNames = append(repoNames, repo.FullName)
	}
	s.store.mu.RUnlock()
	for _, name := range repoNames {
		s.store.DiscoverWorkflowFilesFromGit(name)
	}

	s.store.mu.RLock()
	files := make([]workflowFileView, 0, len(s.store.WorkflowFiles))
	for _, wf := range s.store.WorkflowFiles {
		files = append(files, workflowFileView{
			ID:           wf.ID,
			Name:         wf.Name,
			Path:         wf.Path,
			State:        wf.State,
			RepoFullName: wf.RepoFullName,
			Source:       wf.Source,
			CreatedAt:    wf.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    wf.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	s.store.mu.RUnlock()

	sort.Slice(files, func(i, j int) bool {
		if files[i].RepoFullName != files[j].RepoFullName {
			return files[i].RepoFullName < files[j].RepoFullName
		}
		return files[i].Path < files[j].Path
	})

	writeJSON(w, http.StatusOK, files)
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
