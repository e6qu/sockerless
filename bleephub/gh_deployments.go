package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Deployments + Deployment Statuses + Environments.
// Endpoints:
//   POST   /repos/{o}/{r}/deployments
//   GET    /repos/{o}/{r}/deployments
//   GET    /repos/{o}/{r}/deployments/{id}
//   DELETE /repos/{o}/{r}/deployments/{id}
//   POST   /repos/{o}/{r}/deployments/{id}/statuses
//   GET    /repos/{o}/{r}/deployments/{id}/statuses
//   GET    /repos/{o}/{r}/deployments/{id}/statuses/{status_id}
//   GET    /repos/{o}/{r}/environments
//   GET    /repos/{o}/{r}/environments/{env_name}
//   PUT    /repos/{o}/{r}/environments/{env_name}
//   DELETE /repos/{o}/{r}/environments/{env_name}
//
// gh CLI has no top-level deploy command; this surface is used heavily by
// octokit / probot / GitOps controllers reacting to `deployment` and
// `deployment_status` webhook events.

type Deployment struct {
	ID            int                    `json:"id"`
	NodeID        string                 `json:"node_id"`
	URL           string                 `json:"url"`
	Sha           string                 `json:"sha"`
	Ref           string                 `json:"ref"`
	Task          string                 `json:"task"`
	Payload       map[string]interface{} `json:"payload"`
	OriginalEnv   string                 `json:"original_environment"`
	Environment   string                 `json:"environment"`
	Description   string                 `json:"description"`
	CreatorID     int                    `json:"-"`
	RepoID        int                    `json:"-"`
	AutoMerge     bool                   `json:"auto_merge"`
	ProductionEnv bool                   `json:"production_environment"`
	TransientEnv  bool                   `json:"transient_environment"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Statuses      []*DeploymentStatus    `json:"-"`
}

type DeploymentStatus struct {
	ID             int       `json:"id"`
	NodeID         string    `json:"node_id"`
	State          string    `json:"state"` // error | failure | inactive | in_progress | queued | pending | success
	CreatorID      int       `json:"-"`
	DeploymentID   int       `json:"-"`
	Description    string    `json:"description"`
	Environment    string    `json:"environment"`
	TargetURL      string    `json:"target_url"`
	LogURL         string    `json:"log_url"`
	EnvironmentURL string    `json:"environment_url"`
	AutoInactive   bool      `json:"auto_inactive"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Environment represents a deployment environment configured on a repo.
type Environment struct {
	ID                     int                      `json:"id"`
	NodeID                 string                   `json:"node_id"`
	Name                   string                   `json:"name"`
	URL                    string                   `json:"url"`
	HTMLURL                string                   `json:"html_url"`
	RepoID                 int                      `json:"-"`
	WaitTimer              int                      `json:"-"`
	Reviewers              []map[string]interface{} `json:"-"`
	DeploymentBranchPolicy *DeploymentBranchPolicy  `json:"-"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
	ProtectionRules        []map[string]interface{} `json:"-"`
}

type DeploymentBranchPolicy struct {
	ProtectedBranches    bool `json:"protected_branches"`
	CustomBranchPolicies bool `json:"custom_branch_policies"`
}

// DeploymentStore wraps deployment + status + environment CRUD with a mutex.
type DeploymentStore struct {
	mu           sync.RWMutex
	deployments  map[int]*Deployment
	byRepo       map[int][]*Deployment
	statuses     map[int]*DeploymentStatus
	environments map[string]*Environment // key: "repoID:name"
	envsByRepo   map[int][]*Environment
	nextDepID    int
	nextStatusID int
	nextEnvID    int
}

func newDeploymentStore() *DeploymentStore {
	return &DeploymentStore{
		deployments:  map[int]*Deployment{},
		byRepo:       map[int][]*Deployment{},
		statuses:     map[int]*DeploymentStatus{},
		environments: map[string]*Environment{},
		envsByRepo:   map[int][]*Environment{},
		nextDepID:    1,
		nextStatusID: 1,
		nextEnvID:    1,
	}
}

func (ds *DeploymentStore) CreateDeployment(repoID, creatorID int, ref, sha, task, env, description string, payload map[string]interface{}, productionEnv, transientEnv bool) *Deployment {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	id := ds.nextDepID
	ds.nextDepID++
	now := time.Now()
	d := &Deployment{
		ID:            id,
		NodeID:        fmt.Sprintf("DE_kgDO%08d", id),
		Sha:           sha,
		Ref:           ref,
		Task:          coalesceStr(task, "deploy"),
		Payload:       payload,
		OriginalEnv:   env,
		Environment:   env,
		Description:   description,
		CreatorID:     creatorID,
		RepoID:        repoID,
		ProductionEnv: productionEnv,
		TransientEnv:  transientEnv,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	ds.deployments[id] = d
	ds.byRepo[repoID] = append(ds.byRepo[repoID], d)
	return d
}

func (ds *DeploymentStore) GetDeployment(id int) *Deployment {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.deployments[id]
}

func (ds *DeploymentStore) ListDeployments(repoID int) []*Deployment {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	out := make([]*Deployment, len(ds.byRepo[repoID]))
	copy(out, ds.byRepo[repoID])
	return out
}

func (ds *DeploymentStore) DeleteDeployment(id int) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	d := ds.deployments[id]
	if d == nil {
		return false
	}
	delete(ds.deployments, id)
	src := ds.byRepo[d.RepoID]
	for i, x := range src {
		if x.ID == id {
			ds.byRepo[d.RepoID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	return true
}

func (ds *DeploymentStore) AddStatus(deploymentID, creatorID int, state, description, targetURL, logURL, envURL, env string, autoInactive bool) *DeploymentStatus {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	d := ds.deployments[deploymentID]
	if d == nil {
		return nil
	}
	id := ds.nextStatusID
	ds.nextStatusID++
	now := time.Now()
	status := &DeploymentStatus{
		ID:             id,
		NodeID:         fmt.Sprintf("DS_kgDO%08d", id),
		State:          state,
		CreatorID:      creatorID,
		DeploymentID:   deploymentID,
		Description:    description,
		Environment:    env,
		TargetURL:      targetURL,
		LogURL:         logURL,
		EnvironmentURL: envURL,
		AutoInactive:   autoInactive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	ds.statuses[id] = status
	d.Statuses = append(d.Statuses, status)
	d.UpdatedAt = now
	return status
}

func (ds *DeploymentStore) ListStatuses(deploymentID int) []*DeploymentStatus {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	d := ds.deployments[deploymentID]
	if d == nil {
		return nil
	}
	out := make([]*DeploymentStatus, len(d.Statuses))
	copy(out, d.Statuses)
	return out
}

func (ds *DeploymentStore) GetStatus(id int) *DeploymentStatus {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.statuses[id]
}

func (ds *DeploymentStore) UpsertEnvironment(repoID int, name string) *Environment {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	key := fmt.Sprintf("%d:%s", repoID, name)
	if existing := ds.environments[key]; existing != nil {
		existing.UpdatedAt = time.Now()
		return existing
	}
	id := ds.nextEnvID
	ds.nextEnvID++
	now := time.Now()
	env := &Environment{
		ID:        id,
		NodeID:    fmt.Sprintf("EN_kgDO%08d", id),
		Name:      name,
		RepoID:    repoID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	ds.environments[key] = env
	ds.envsByRepo[repoID] = append(ds.envsByRepo[repoID], env)
	return env
}

func (ds *DeploymentStore) GetEnvironment(repoID int, name string) *Environment {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.environments[fmt.Sprintf("%d:%s", repoID, name)]
}

func (ds *DeploymentStore) ListEnvironments(repoID int) []*Environment {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	out := make([]*Environment, len(ds.envsByRepo[repoID]))
	copy(out, ds.envsByRepo[repoID])
	return out
}

func (ds *DeploymentStore) DeleteEnvironment(repoID int, name string) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	key := fmt.Sprintf("%d:%s", repoID, name)
	env := ds.environments[key]
	if env == nil {
		return false
	}
	delete(ds.environments, key)
	src := ds.envsByRepo[repoID]
	for i, x := range src {
		if x.ID == env.ID {
			ds.envsByRepo[repoID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	return true
}

func (s *Server) registerGHDeploymentsRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/deployments",
		s.requirePerm("deployments", permWrite, s.handleCreateDeployment))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/deployments",
		s.handleListDeployments)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}",
		s.handleGetDeployment)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}",
		s.requirePerm("deployments", permWrite, s.handleDeleteDeployment))
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses",
		s.requirePerm("deployments", permWrite, s.handleCreateDeploymentStatus))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses",
		s.handleListDeploymentStatuses)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses/{status_id}",
		s.handleGetDeploymentStatus)

	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/environments",
		s.handleListEnvironments)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}",
		s.handleGetEnvironment)
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/environments/{env_name}",
		s.requirePerm("administration", permWrite, s.handleUpsertEnvironment))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/environments/{env_name}",
		s.requirePerm("administration", permWrite, s.handleDeleteEnvironment))
}

func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Ref                   string                 `json:"ref"`
		Task                  string                 `json:"task"`
		AutoMerge             flexBool               `json:"auto_merge"`
		RequiredContexts      []string               `json:"required_contexts"`
		Payload               map[string]interface{} `json:"payload"`
		Environment           string                 `json:"environment"`
		Description           string                 `json:"description"`
		TransientEnvironment  flexBool               `json:"transient_environment"`
		ProductionEnvironment flexBool               `json:"production_environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Ref == "" {
		writeGHValidationError(w, "Deployment", "ref", "missing_field")
		return
	}
	env := req.Environment
	if env == "" {
		env = "production"
	}
	s.store.Deployments.UpsertEnvironment(repo.ID, env)
	d := s.store.Deployments.CreateDeployment(repo.ID, user.ID, req.Ref, req.Ref, req.Task, env, req.Description, req.Payload, bool(req.ProductionEnvironment), bool(req.TransientEnvironment))
	s.emitWebhookEvent(repo.FullName, "deployment", "created", buildDeploymentEventPayload(repo, d, user, "created"))
	writeJSON(w, http.StatusCreated, deploymentToJSON(d, s.store, s.baseURL(r), repo))
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	deployments := s.store.Deployments.ListDeployments(repo.ID)
	page := paginateAndLink(w, r, deployments)
	out := make([]map[string]interface{}, 0, len(page))
	for _, d := range page {
		out = append(out, deploymentToJSON(d, s.store, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("deployment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.Deployments.GetDeployment(id)
	if d == nil || d.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, deploymentToJSON(d, s.store, s.baseURL(r), repo))
}

func (s *Server) handleDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("deployment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.Deployments.GetDeployment(id)
	if d == nil || d.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Deployments.DeleteDeployment(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("deployment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.Deployments.GetDeployment(id)
	if d == nil || d.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		State          string   `json:"state"`
		LogURL         string   `json:"log_url"`
		Description    string   `json:"description"`
		Environment    string   `json:"environment"`
		EnvironmentURL string   `json:"environment_url"`
		AutoInactive   flexBool `json:"auto_inactive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.State == "" {
		writeGHValidationError(w, "DeploymentStatus", "state", "missing_field")
		return
	}
	env := req.Environment
	if env == "" {
		env = d.Environment
	}
	status := s.store.Deployments.AddStatus(id, user.ID, req.State, req.Description, "", req.LogURL, req.EnvironmentURL, env, bool(req.AutoInactive))
	s.emitWebhookEvent(repo.FullName, "deployment_status", req.State, buildDeploymentStatusEventPayload(repo, d, status, user))
	writeJSON(w, http.StatusCreated, deploymentStatusToJSON(status, s.store, s.baseURL(r), repo))
}

func (s *Server) handleListDeploymentStatuses(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("deployment_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.Deployments.GetDeployment(id)
	if d == nil || d.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	statuses := s.store.Deployments.ListStatuses(id)
	page := paginateAndLink(w, r, statuses)
	out := make([]map[string]interface{}, 0, len(page))
	for _, st := range page {
		out = append(out, deploymentStatusToJSON(st, s.store, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetDeploymentStatus(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	statusID, err := strconv.Atoi(r.PathValue("status_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	status := s.store.Deployments.GetStatus(statusID)
	if status == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, deploymentStatusToJSON(status, s.store, s.baseURL(r), repo))
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	envs := s.store.Deployments.ListEnvironments(repo.ID)
	out := make([]map[string]interface{}, 0, len(envs))
	for _, e := range envs {
		out = append(out, environmentToJSON(e, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":  len(envs),
		"environments": out,
	})
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	env := s.store.Deployments.GetEnvironment(repo.ID, r.PathValue("env_name"))
	if env == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, environmentToJSON(env, s.baseURL(r), repo))
}

func (s *Server) handleUpsertEnvironment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	env := s.store.Deployments.UpsertEnvironment(repo.ID, r.PathValue("env_name"))
	writeJSON(w, http.StatusOK, environmentToJSON(env, s.baseURL(r), repo))
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.Deployments.DeleteEnvironment(repo.ID, r.PathValue("env_name")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func deploymentToJSON(d *Deployment, st *Store, baseURL string, repo *Repo) map[string]interface{} {
	if d == nil {
		return nil
	}
	var creator map[string]interface{}
	st.mu.RLock()
	if u := st.Users[d.CreatorID]; u != nil {
		creator = userToJSON(u)
	}
	st.mu.RUnlock()
	return map[string]interface{}{
		"id":                     d.ID,
		"node_id":                d.NodeID,
		"sha":                    d.Sha,
		"ref":                    d.Ref,
		"task":                   d.Task,
		"payload":                d.Payload,
		"original_environment":   d.OriginalEnv,
		"environment":            d.Environment,
		"description":            d.Description,
		"creator":                creator,
		"created_at":             d.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":             d.UpdatedAt.UTC().Format(time.RFC3339),
		"statuses_url":           fmt.Sprintf("%s/api/v3/repos/%s/deployments/%d/statuses", baseURL, repo.FullName, d.ID),
		"repository_url":         fmt.Sprintf("%s/api/v3/repos/%s", baseURL, repo.FullName),
		"url":                    fmt.Sprintf("%s/api/v3/repos/%s/deployments/%d", baseURL, repo.FullName, d.ID),
		"transient_environment":  d.TransientEnv,
		"production_environment": d.ProductionEnv,
	}
}

func deploymentStatusToJSON(st *DeploymentStatus, store *Store, baseURL string, repo *Repo) map[string]interface{} {
	if st == nil {
		return nil
	}
	var creator map[string]interface{}
	store.mu.RLock()
	if u := store.Users[st.CreatorID]; u != nil {
		creator = userToJSON(u)
	}
	store.mu.RUnlock()
	return map[string]interface{}{
		"id":              st.ID,
		"node_id":         st.NodeID,
		"state":           st.State,
		"creator":         creator,
		"description":     st.Description,
		"environment":     st.Environment,
		"target_url":      st.TargetURL,
		"log_url":         st.LogURL,
		"environment_url": st.EnvironmentURL,
		"auto_inactive":   st.AutoInactive,
		"created_at":      st.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":      st.UpdatedAt.UTC().Format(time.RFC3339),
		"url":             fmt.Sprintf("%s/api/v3/repos/%s/deployments/%d/statuses/%d", baseURL, repo.FullName, st.DeploymentID, st.ID),
		"deployment_url":  fmt.Sprintf("%s/api/v3/repos/%s/deployments/%d", baseURL, repo.FullName, st.DeploymentID),
		"repository_url":  fmt.Sprintf("%s/api/v3/repos/%s", baseURL, repo.FullName),
	}
}

func environmentToJSON(e *Environment, baseURL string, repo *Repo) map[string]interface{} {
	if e == nil {
		return nil
	}
	return map[string]interface{}{
		"id":         e.ID,
		"node_id":    e.NodeID,
		"name":       e.Name,
		"url":        fmt.Sprintf("%s/api/v3/repos/%s/environments/%s", baseURL, repo.FullName, e.Name),
		"html_url":   fmt.Sprintf("%s/%s/deployments/activity_log?environments_filter=%s", baseURL, repo.FullName, e.Name),
		"created_at": e.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": e.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func buildDeploymentEventPayload(repo *Repo, d *Deployment, sender *User, action string) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"action": action,
		"deployment": map[string]interface{}{
			"id":          d.ID,
			"sha":         d.Sha,
			"ref":         d.Ref,
			"task":        d.Task,
			"environment": d.Environment,
		},
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}, nil)
}

func buildDeploymentStatusEventPayload(repo *Repo, d *Deployment, status *DeploymentStatus, sender *User) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"action": status.State,
		"deployment_status": map[string]interface{}{
			"id":          status.ID,
			"state":       status.State,
			"description": status.Description,
			"environment": status.Environment,
		},
		"deployment": map[string]interface{}{
			"id":          d.ID,
			"environment": d.Environment,
		},
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}, nil)
}
