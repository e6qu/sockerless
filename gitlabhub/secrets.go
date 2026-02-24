package gitlabhub

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (s *Server) registerSecretsRoutes() {
	s.mux.HandleFunc("POST /api/v4/projects/{id}/variables", s.handleCreateVariable)
	s.mux.HandleFunc("GET /api/v4/projects/{id}/variables", s.handleListVariables)
	s.mux.HandleFunc("DELETE /api/v4/projects/{id}/variables/{key}", s.handleDeleteVariable)
}

// handleCreateVariable handles POST /api/v4/projects/:id/variables.
func (s *Server) handleCreateVariable(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project ID", http.StatusBadRequest)
		return
	}

	project := s.store.GetProject(projectID)
	if project == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	var req struct {
		Key       string `json:"key"`
		Value     string `json:"value"`
		Protected bool   `json:"protected"`
		Masked    bool   `json:"masked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	s.store.mu.Lock()
	project.Variables[req.Key] = &Variable{
		Key:       req.Key,
		Value:     req.Value,
		Protected: req.Protected,
		Masked:    req.Masked,
	}
	s.store.mu.Unlock()

	s.logger.Info().
		Int("project_id", projectID).
		Str("key", req.Key).
		Bool("masked", req.Masked).
		Msg("variable created")

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"key":       req.Key,
		"value":     req.Value,
		"protected": req.Protected,
		"masked":    req.Masked,
	})
}

// handleListVariables handles GET /api/v4/projects/:id/variables.
func (s *Server) handleListVariables(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project ID", http.StatusBadRequest)
		return
	}

	project := s.store.GetProject(projectID)
	if project == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	s.store.mu.RLock()
	var vars []map[string]interface{}
	for _, v := range project.Variables {
		vars = append(vars, map[string]interface{}{
			"key":       v.Key,
			"value":     v.Value,
			"protected": v.Protected,
			"masked":    v.Masked,
		})
	}
	s.store.mu.RUnlock()

	if vars == nil {
		vars = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, vars)
}

// handleDeleteVariable handles DELETE /api/v4/projects/:id/variables/:key.
func (s *Server) handleDeleteVariable(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid project ID", http.StatusBadRequest)
		return
	}

	project := s.store.GetProject(projectID)
	if project == nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	key := r.PathValue("key")

	s.store.mu.Lock()
	_, exists := project.Variables[key]
	if exists {
		delete(project.Variables, key)
	}
	s.store.mu.Unlock()

	if !exists {
		http.Error(w, "variable not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
