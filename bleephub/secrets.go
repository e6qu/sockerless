package bleephub

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Secret represents a repository secret for GitHub Actions.
type Secret struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Value     string    `json:"-"` // never exposed via GET
}

func (s *Server) registerSecretsRoutes() {
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/secrets", s.handleListSecrets)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/actions/secrets/{name}", s.handleGetSecret)
	s.mux.HandleFunc("PUT /api/v3/repos/{owner}/{repo}/actions/secrets/{name}", s.handlePutSecret)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/actions/secrets/{name}", s.handleDeleteSecret)
}

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")

	s.store.mu.RLock()
	secrets := s.store.RepoSecrets[repoKey]
	s.store.mu.RUnlock()

	list := make([]map[string]interface{}, 0, len(secrets))
	for _, sec := range secrets {
		list = append(list, map[string]interface{}{
			"name":       sec.Name,
			"created_at": sec.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at": sec.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	name := r.PathValue("name")

	s.store.mu.RLock()
	secrets := s.store.RepoSecrets[repoKey]
	sec := secrets[strings.ToUpper(name)]
	s.store.mu.RUnlock()

	if sec == nil {
		http.Error(w, "secret not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":       sec.Name,
		"created_at": sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": sec.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handlePutSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	name := strings.ToUpper(r.PathValue("name"))

	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	now := time.Now()

	s.store.mu.Lock()
	if s.store.RepoSecrets == nil {
		s.store.RepoSecrets = make(map[string]map[string]*Secret)
	}
	if s.store.RepoSecrets[repoKey] == nil {
		s.store.RepoSecrets[repoKey] = make(map[string]*Secret)
	}

	existing := s.store.RepoSecrets[repoKey][name]
	if existing != nil {
		existing.Value = body.Value
		existing.UpdatedAt = now
		s.store.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.store.RepoSecrets[repoKey][name] = &Secret{
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Value:     body.Value,
	}
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	name := strings.ToUpper(r.PathValue("name"))

	s.store.mu.Lock()
	if secrets, ok := s.store.RepoSecrets[repoKey]; ok {
		delete(secrets, name)
	}
	s.store.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}
