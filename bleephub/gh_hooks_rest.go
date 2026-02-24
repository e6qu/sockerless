package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHHookRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/hooks", s.handleCreateHook)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks", s.handleListHooks)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks/{id}", s.handleGetHook)
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/hooks/{id}", s.handleUpdateHook)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/hooks/{id}", s.handleDeleteHook)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries", s.handleListHookDeliveries)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/hooks/{id}/pings", s.handlePingHook)
}

func (s *Server) handleCreateHook(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")

	var req struct {
		Config struct {
			URL    string `json:"url"`
			Secret string `json:"secret"`
		} `json:"config"`
		Events []string `json:"events"`
		Active *bool    `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	if req.Config.URL == "" {
		writeGHValidationError(w, "Hook", "url", "missing_field")
		return
	}

	events := req.Events
	if len(events) == 0 {
		events = []string{"push"}
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	hook := s.store.CreateHook(repoKey, req.Config.URL, req.Config.Secret, events, active)
	writeJSON(w, http.StatusCreated, hookToJSON(hook))
}

func (s *Server) handleListHooks(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hooks := s.store.ListHooks(repoKey)

	result := make([]map[string]interface{}, 0, len(hooks))
	for _, h := range hooks {
		result = append(result, hookToJSON(h))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetHook(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	hook := s.store.GetHook(repoKey, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, hookToJSON(hook))
}

func (s *Server) handleUpdateHook(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Config *struct {
			URL    string `json:"url"`
			Secret string `json:"secret"`
		} `json:"config"`
		Events []string `json:"events"`
		Active *bool    `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	found := s.store.UpdateHook(repoKey, hookID, func(h *Webhook) {
		if req.Config != nil {
			if req.Config.URL != "" {
				h.URL = req.Config.URL
			}
			if req.Config.Secret != "" {
				h.Secret = req.Config.Secret
			}
		}
		if req.Events != nil {
			h.Events = req.Events
		}
		if req.Active != nil {
			h.Active = *req.Active
		}
	})

	if !found {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	hook := s.store.GetHook(repoKey, hookID)
	writeJSON(w, http.StatusOK, hookToJSON(hook))
}

func (s *Server) handleDeleteHook(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !s.store.DeleteHook(repoKey, hookID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListHookDeliveries(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	hook := s.store.GetHook(repoKey, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	deliveries := s.store.ListDeliveries(hookID)
	result := make([]map[string]interface{}, 0, len(deliveries))
	for _, d := range deliveries {
		result = append(result, deliveryToJSON(d))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePingHook(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	repoKey := owner + "/" + repoName
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	hook := s.store.GetHook(repoKey, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	repo := s.store.GetRepo(owner, repoName)
	payload := buildPingPayload(repo, hook)

	go s.deliverWebhook(hook, "ping", "", mustMarshal(payload))

	w.WriteHeader(http.StatusNoContent)
}

func mustMarshal(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("mustMarshal: " + err.Error())
	}
	return b
}

func hookToJSON(h *Webhook) map[string]interface{} {
	return map[string]interface{}{
		"id":     h.ID,
		"type":   "Repository",
		"name":   "web",
		"active": h.Active,
		"events": h.Events,
		"config": map[string]interface{}{
			"url":          h.URL,
			"content_type": "json",
			"insecure_ssl": "0",
		},
		"created_at": h.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": h.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func deliveryToJSON(d *WebhookDelivery) map[string]interface{} {
	return map[string]interface{}{
		"id":           d.ID,
		"guid":         d.GUID,
		"event":        d.Event,
		"action":       d.Action,
		"status_code":  d.StatusCode,
		"duration":     d.Duration,
		"redelivery":   d.Redelivery,
		"delivered_at": d.DeliveredAt.UTC().Format(time.RFC3339),
	}
}
