package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHIssueRoutes() {
	// Labels
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/labels", s.handleCreateLabel)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/labels", s.handleListLabels)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/labels/{name}", s.handleGetLabel)
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/labels/{name}", s.handleUpdateLabel)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/labels/{name}", s.handleDeleteLabel)

	// Milestones
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/milestones", s.handleCreateMilestone)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/milestones", s.handleListMilestones)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/milestones/{number}", s.handleGetMilestone)
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/milestones/{number}", s.handleUpdateMilestone)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/milestones/{number}", s.handleDeleteMilestone)

	// Issues
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/issues", s.handleCreateIssue)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/issues", s.handleListIssues)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/issues/{number}", s.handleGetIssue)
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/issues/{number}", s.handleUpdateIssue)

	// Issue comments
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/issues/{number}/comments", s.handleCreateIssueComment)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/issues/{number}/comments", s.handleListIssueComments)

	// Issue label management
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/issues/{number}/labels", s.handleAddIssueLabels)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/labels/{name}", s.handleRemoveIssueLabel)
}

// --- Label handlers ---

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "Label", "name", "missing_field")
		return
	}

	label := s.store.CreateLabel(repo.ID, req.Name, req.Description, req.Color)
	if label == nil {
		writeGHValidationError(w, "Label", "name", "already_exists")
		return
	}

	writeJSON(w, http.StatusCreated, issueLabelToJSON(label, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	labels := s.store.ListLabels(repo.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(labels))
	for _, l := range labels {
		result = append(result, issueLabelToJSON(l, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetLabel(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	labelName := r.PathValue("name")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, issueLabelToJSON(label, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	labelName := r.PathValue("name")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.UpdateLabel(label.ID, func(l *IssueLabel) {
		if v, ok := req["new_name"].(string); ok {
			l.Name = v
		}
		if v, ok := req["color"].(string); ok {
			l.Color = v
		}
		if v, ok := req["description"].(string); ok {
			l.Description = v
		}
	})

	updated := s.store.GetLabel(label.ID)
	writeJSON(w, http.StatusOK, issueLabelToJSON(updated, s.baseURL(r), repo.FullName))
}

func (s *Server) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	labelName := r.PathValue("name")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	label := s.store.GetLabelByName(repo.ID, labelName)
	if label == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.DeleteLabel(label.ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- Milestone handlers ---

func (s *Server) handleCreateMilestone(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		DueOn       string `json:"due_on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Title == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	var dueOn *time.Time
	if req.DueOn != "" {
		t, err := time.Parse(time.RFC3339, req.DueOn)
		if err == nil {
			dueOn = &t
		}
	}

	ms := s.store.CreateMilestone(repo.ID, req.Title, req.Description, req.State, dueOn)
	if ms == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	writeJSON(w, http.StatusCreated, milestoneToJSON(ms, s.baseURL(r), repo.FullName))
}

func (s *Server) handleListMilestones(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}

	milestones := s.store.ListMilestones(repo.ID, state)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(milestones))
	for _, ms := range milestones {
		result = append(result, milestoneToJSON(ms, base, repo.FullName))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetMilestone(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, milestoneToJSON(ms, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateMilestone(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.UpdateMilestone(ms.ID, func(m *Milestone) {
		if v, ok := req["title"].(string); ok {
			m.Title = v
		}
		if v, ok := req["description"].(string); ok {
			m.Description = v
		}
		if v, ok := req["state"].(string); ok {
			m.State = v
		}
	})

	updated := s.store.GetMilestone(ms.ID)
	writeJSON(w, http.StatusOK, milestoneToJSON(updated, s.baseURL(r), repo.FullName))
}

func (s *Server) handleDeleteMilestone(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	numStr := r.PathValue("number")
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ms := s.store.GetMilestoneByNumber(repo.ID, num)
	if ms == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.DeleteMilestone(ms.ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- JSON converters ---

func issueLabelToJSON(l *IssueLabel, baseURL, repoFullName string) map[string]interface{} {
	return map[string]interface{}{
		"id":          l.ID,
		"node_id":     l.NodeID,
		"url":         baseURL + "/api/v3/repos/" + repoFullName + "/labels/" + l.Name,
		"name":        l.Name,
		"description": l.Description,
		"color":       l.Color,
		"default":     l.Default,
	}
}

func milestoneToJSON(ms *Milestone, baseURL, repoFullName string) map[string]interface{} {
	var dueOn interface{}
	if ms.DueOn != nil {
		dueOn = ms.DueOn.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"id":           ms.ID,
		"node_id":      ms.NodeID,
		"url":          baseURL + "/api/v3/repos/" + repoFullName + "/milestones/" + strconv.Itoa(ms.Number),
		"html_url":     baseURL + "/" + repoFullName + "/milestone/" + strconv.Itoa(ms.Number),
		"number":       ms.Number,
		"title":        ms.Title,
		"description":  ms.Description,
		"state":        ms.State,
		"due_on":       dueOn,
		"created_at":   ms.CreatedAt.Format(time.RFC3339),
		"updated_at":   ms.UpdatedAt.Format(time.RFC3339),
	}
}
