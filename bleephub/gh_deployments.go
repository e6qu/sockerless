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

// Deployment / DeploymentStatus / Environment carry real json names on
// their linkage + protection fields so persistence (which marshals the
// structs as-is) round-trips them. Client responses never marshal these
// structs — deploymentToJSON / deploymentStatusToJSON / environmentToJSON
// emit explicit maps. Deployment.Statuses stays json:"-": statuses persist
// in their own bucket and the loader relinks them via DeploymentID.
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
	CreatorID     int                    `json:"creator_id"`
	RepoID        int                    `json:"repo_id"`
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
	CreatorID      int       `json:"creator_id"`
	DeploymentID   int       `json:"deployment_id"`
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
	RepoID                 int                      `json:"repo_id"`
	WaitTimer              int                      `json:"wait_timer"`
	Reviewers              []map[string]interface{} `json:"reviewers"`
	DeploymentBranchPolicy *DeploymentBranchPolicy  `json:"deployment_branch_policy"`
	CreatedAt              time.Time                `json:"created_at"`
	UpdatedAt              time.Time                `json:"updated_at"`
	ProtectionRules        []map[string]interface{} `json:"protection_rules"`
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
	persist      *Persistence
}

func newDeploymentStore(p *Persistence) *DeploymentStore {
	return &DeploymentStore{
		deployments:  map[int]*Deployment{},
		byRepo:       map[int][]*Deployment{},
		statuses:     map[int]*DeploymentStatus{},
		environments: map[string]*Environment{},
		envsByRepo:   map[int][]*Environment{},
		nextDepID:    1,
		nextStatusID: 1,
		nextEnvID:    1,
		persist:      p,
	}
}

func (ds *DeploymentStore) CreateDeployment(repoID, creatorID int, ref, sha, task, env, description string, payload map[string]interface{}, productionEnv, transientEnv bool) *Deployment {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	id := ds.nextDepID
	ds.nextDepID++
	now := time.Now().UTC()
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
	if ds.persist != nil {
		ds.persist.MustPut("deployments", strconv.Itoa(id), d)
	}
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
	ds.deleteDeploymentLocked(d)
	return true
}

func (ds *DeploymentStore) deleteDeploymentLocked(d *Deployment) {
	delete(ds.deployments, d.ID)
	src := ds.byRepo[d.RepoID]
	for i, x := range src {
		if x.ID == d.ID {
			ds.byRepo[d.RepoID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	for id, status := range ds.statuses {
		if status.DeploymentID == d.ID {
			delete(ds.statuses, id)
			if ds.persist != nil {
				ds.persist.MustDelete("deployment_statuses", strconv.Itoa(id))
			}
		}
	}
	if ds.persist != nil {
		ds.persist.MustDelete("deployments", strconv.Itoa(d.ID))
	}
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
	now := time.Now().UTC()
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
	if ds.persist != nil {
		ds.persist.MustPut("deployment_statuses", strconv.Itoa(id), status)
		ds.persist.MustPut("deployments", strconv.Itoa(deploymentID), d)
	}
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
		existing.UpdatedAt = time.Now().UTC()
		return existing
	}
	id := ds.nextEnvID
	ds.nextEnvID++
	now := time.Now().UTC()
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
	if ds.persist != nil {
		ds.persist.MustPut("environments", key, env)
	}
	return env
}

// SetEnvironmentProtection updates an environment's reviewer/wait-timer
// protection config (the PUT environment body).
func (ds *DeploymentStore) SetEnvironmentProtection(repoID int, name string, waitTimer *int, reviewers []map[string]interface{}) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	key := fmt.Sprintf("%d:%s", repoID, name)
	env := ds.environments[key]
	if env == nil {
		return
	}
	if waitTimer != nil {
		env.WaitTimer = *waitTimer
	}
	if reviewers != nil {
		env.Reviewers = reviewers
	}
	env.UpdatedAt = time.Now().UTC()
	if ds.persist != nil {
		ds.persist.MustPut("environments", key, env)
	}
}

// SetEnvironmentBranchPolicyConfig sets an environment's deployment branch
// policy configuration. nil clears it (all branches may deploy) — the PUT
// environment body treats an absent/null field as a reset, matching real
// GitHub's full-replace semantics for this member.
func (ds *DeploymentStore) SetEnvironmentBranchPolicyConfig(repoID int, name string, policy *DeploymentBranchPolicy) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	key := fmt.Sprintf("%d:%s", repoID, name)
	env := ds.environments[key]
	if env == nil {
		return
	}
	env.DeploymentBranchPolicy = policy
	env.UpdatedAt = time.Now().UTC()
	if ds.persist != nil {
		ds.persist.MustPut("environments", key, env)
	}
}

func (ds *DeploymentStore) GetEnvironment(repoID int, name string) *Environment {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.environments[fmt.Sprintf("%d:%s", repoID, name)]
}

// GetEnvironmentByID returns an environment by its numeric id, or nil.
func (ds *DeploymentStore) GetEnvironmentByID(id int) *Environment {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	for _, env := range ds.environments {
		if env.ID == id {
			return env
		}
	}
	return nil
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
	if ds.persist != nil {
		ds.persist.MustDelete("environments", key)
	}
	return true
}

func (ds *DeploymentStore) DeleteRepo(repoID int) []int {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	for _, d := range append([]*Deployment(nil), ds.byRepo[repoID]...) {
		ds.deleteDeploymentLocked(d)
	}
	delete(ds.byRepo, repoID)

	envIDs := make([]int, 0, len(ds.envsByRepo[repoID]))
	for _, env := range append([]*Environment(nil), ds.envsByRepo[repoID]...) {
		envIDs = append(envIDs, env.ID)
		key := fmt.Sprintf("%d:%s", repoID, env.Name)
		delete(ds.environments, key)
		if ds.persist != nil {
			ds.persist.MustDelete("environments", key)
		}
	}
	delete(ds.envsByRepo, repoID)
	return envIDs
}

func (s *Server) registerGHDeploymentsRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/deployments",
		s.requirePerm(scopeDeployments, permWrite, s.handleCreateDeployment))
	s.route("GET /api/v3/repos/{owner}/{repo}/deployments",
		s.handleListDeployments)
	s.route("GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}",
		s.handleGetDeployment)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}",
		s.requirePerm(scopeDeployments, permWrite, s.handleDeleteDeployment))
	s.route("POST /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses",
		s.requirePerm(scopeDeployments, permWrite, s.handleCreateDeploymentStatus))
	s.route("GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses",
		s.handleListDeploymentStatuses)
	s.route("GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses/{status_id}",
		s.handleGetDeploymentStatus)

	s.route("GET /api/v3/repos/{owner}/{repo}/environments",
		s.handleListEnvironments)
	s.route("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}",
		s.handleGetEnvironment)
	s.route("PUT /api/v3/repos/{owner}/{repo}/environments/{env_name}",
		s.requirePerm(scopeAdministration, permWrite, s.handleUpsertEnvironment))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/environments/{env_name}",
		s.requirePerm(scopeAdministration, permWrite, s.handleDeleteEnvironment))
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
	s.recordAuditEvent("deployment.create", user.Login, "", map[string]interface{}{"repo": repo.FullName, "deployment_id": d.ID})
	writeJSON(w, http.StatusCreated, deploymentToJSON(d, s.store, s.baseURL(r), repo))
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
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
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
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
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
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
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
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
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	envs := s.store.Deployments.ListEnvironments(repo.ID)
	out := make([]map[string]interface{}, 0, len(envs))
	for _, e := range envs {
		out = append(out, environmentToJSON(e, s.store, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":  len(envs),
		"environments": out,
	})
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	env := s.store.Deployments.GetEnvironment(repo.ID, r.PathValue("env_name"))
	if env == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, environmentToJSON(env, s.store, s.baseURL(r), repo))
}

func (s *Server) handleUpsertEnvironment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var body struct {
		WaitTimer *int `json:"wait_timer"`
		Reviewers []struct {
			Type string `json:"type"`
			ID   int    `json:"id"`
		} `json:"reviewers"`
		DeploymentBranchPolicy *DeploymentBranchPolicy `json:"deployment_branch_policy"`
	}
	// An absent body is valid (environment with no protection config), but
	// malformed JSON is still a 400 like real GitHub.
	if !decodeJSONBodyOptional(w, r, &body) {
		return
	}

	env := s.store.Deployments.UpsertEnvironment(repo.ID, r.PathValue("env_name"))

	if body.WaitTimer != nil || body.Reviewers != nil {
		var reviewers []map[string]interface{}
		for _, rev := range body.Reviewers {
			revType := rev.Type
			if revType == "" {
				revType = "User"
			}
			reviewers = append(reviewers, map[string]interface{}{"type": revType, "id": rev.ID})
		}
		s.store.Deployments.SetEnvironmentProtection(repo.ID, env.Name, body.WaitTimer, reviewers)
	}
	s.store.Deployments.SetEnvironmentBranchPolicyConfig(repo.ID, env.Name, body.DeploymentBranchPolicy)
	writeJSON(w, http.StatusOK, environmentToJSON(env, s.store, s.baseURL(r), repo))
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	env := s.store.Deployments.GetEnvironment(repo.ID, r.PathValue("env_name"))
	if env == nil || !s.store.Deployments.DeleteEnvironment(repo.ID, env.Name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.PruneEnvironmentPolicies(env.ID)
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

func environmentToJSON(e *Environment, st *Store, baseURL string, repo *Repo) map[string]interface{} {
	if e == nil {
		return nil
	}
	var branchPolicy interface{}
	if e.DeploymentBranchPolicy != nil {
		branchPolicy = map[string]interface{}{
			"protected_branches":     e.DeploymentBranchPolicy.ProtectedBranches,
			"custom_branch_policies": e.DeploymentBranchPolicy.CustomBranchPolicies,
		}
	}
	out := map[string]interface{}{
		"id":                       e.ID,
		"node_id":                  e.NodeID,
		"name":                     e.Name,
		"url":                      fmt.Sprintf("%s/api/v3/repos/%s/environments/%s", baseURL, repo.FullName, e.Name),
		"html_url":                 fmt.Sprintf("%s/%s/deployments/activity_log?environments_filter=%s", baseURL, repo.FullName, e.Name),
		"created_at":               e.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":               e.UpdatedAt.UTC().Format(time.RFC3339),
		"deployment_branch_policy": branchPolicy,
	}
	rules := []map[string]interface{}{}
	if e.WaitTimer > 0 {
		rules = append(rules, map[string]interface{}{
			"id":         e.ID*10 + 1,
			"node_id":    fmt.Sprintf("GA_kwDO%08d", e.ID*10+1),
			"type":       "wait_timer",
			"wait_timer": e.WaitTimer,
		})
	}
	if len(e.Reviewers) > 0 {
		rules = append(rules, map[string]interface{}{
			"id":        e.ID*10 + 2,
			"node_id":   fmt.Sprintf("GA_kwDO%08d", e.ID*10+2),
			"type":      "required_reviewers",
			"reviewers": environmentReviewersJSON(e, st),
		})
	}
	if branchPolicy != nil {
		rules = append(rules, map[string]interface{}{
			"id":      e.ID*10 + 3,
			"node_id": fmt.Sprintf("GA_kwDO%08d", e.ID*10+3),
			"type":    "branch_policy",
		})
	}
	out["protection_rules"] = rules
	return out
}

// environmentReviewersJSON renders the configured reviewers with their
// resolved user objects, the shape protection rules and pending
// deployments share.
func environmentReviewersJSON(e *Environment, st *Store) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, rev := range e.Reviewers {
		revType, _ := rev["type"].(string)
		var id int
		switch v := rev["id"].(type) {
		case int:
			id = v
		case float64:
			id = int(v)
		}
		entry := map[string]interface{}{"type": revType}
		st.mu.RLock()
		if u := st.Users[id]; u != nil {
			entry["reviewer"] = userToJSON(u)
		}
		st.mu.RUnlock()
		out = append(out, entry)
	}
	return out
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
