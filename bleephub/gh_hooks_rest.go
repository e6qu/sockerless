package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHHookRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/hooks", s.requirePerm("administration", permWrite, s.handleCreateHook))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks", s.requirePerm("administration", permRead, s.handleListHooks))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks/{id}", s.requirePerm("administration", permRead, s.handleGetHook))
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/hooks/{id}", s.requirePerm("administration", permWrite, s.handleUpdateHook))
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/hooks/{id}", s.requirePerm("administration", permWrite, s.handleDeleteHook))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries", s.requirePerm("administration", permRead, s.handleListHookDeliveries))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries/{delivery_id}", s.requirePerm("administration", permRead, s.handleGetHookDelivery))
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries/{delivery_id}/attempts", s.requirePerm("administration", permWrite, s.handleRedeliverHookDelivery))
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/hooks/{id}/pings", s.requirePerm("administration", permWrite, s.handlePingHook))
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
		Events []string  `json:"events"`
		Active *flexBool `json:"active"`
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
		active = bool(*req.Active)
	}

	hook := s.store.CreateHook(repoKey, req.Config.URL, req.Config.Secret, events, active)
	writeJSON(w, http.StatusCreated, hookToJSON(hook, r, r.PathValue("owner"), r.PathValue("repo")))
}

func (s *Server) handleListHooks(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hooks := s.store.ListHooks(repoKey)

	owner, repo := r.PathValue("owner"), r.PathValue("repo")
	result := make([]map[string]interface{}, 0, len(hooks))
	for _, h := range hooks {
		result = append(result, hookToJSON(h, r, owner, repo))
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

	writeJSON(w, http.StatusOK, hookToJSON(hook, r, r.PathValue("owner"), r.PathValue("repo")))
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
		Events []string  `json:"events"`
		Active *flexBool `json:"active"`
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
			h.Active = bool(*req.Active)
		}
	})

	if !found {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	hook := s.store.GetHook(repoKey, hookID)
	writeJSON(w, http.StatusOK, hookToJSON(hook, r, r.PathValue("owner"), r.PathValue("repo")))
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

// handleGetHookDelivery — GET /repos/{o}/{r}/hooks/{id}/deliveries/{delivery_id}.
// Real GitHub: returns the full delivery with request + response payloads.
func (s *Server) handleGetHookDelivery(w http.ResponseWriter, r *http.Request) {
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
	deliveryID, err := strconv.Atoi(r.PathValue("delivery_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	hook := s.store.GetHook(repoKey, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	for _, d := range s.store.ListDeliveries(hookID) {
		if d.ID == deliveryID {
			writeJSON(w, http.StatusOK, deliveryFullJSON(d))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// handleRedeliverHookDelivery — POST /repos/{o}/{r}/hooks/{id}/deliveries/{delivery_id}/attempts.
func (s *Server) handleRedeliverHookDelivery(w http.ResponseWriter, r *http.Request) {
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
	deliveryID, err := strconv.Atoi(r.PathValue("delivery_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	hook := s.store.GetHook(repoKey, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var original *WebhookDelivery
	for _, d := range s.store.ListDeliveries(hookID) {
		if d.ID == deliveryID {
			original = d
			break
		}
	}
	if original == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	payloadBytes := mustMarshal(original.Request.Payload)
	go func() {
		delivery := s.doDeliverAttempt(hook, original.Event, original.Action, original.GUID, payloadBytes, true)
		s.store.AddDelivery(delivery)
	}()
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": deliveryID, "redelivery": true})
}

// hookToJSON serialises a Webhook to GitHub's published hook object shape.
// r and owner/repo are needed to construct the self-referential API URLs.
func hookToJSON(h *Webhook, r *http.Request, owner, repo string) map[string]interface{} {
	base := "http://" + r.Host
	hookBase := base + "/api/v3/repos/" + owner + "/" + repo + "/hooks/" + strconv.Itoa(h.ID)
	return map[string]interface{}{
		"type":   "Repository",
		"id":     h.ID,
		"name":   "web",
		"active": h.Active,
		"events": h.Events,
		"config": map[string]interface{}{
			"url":          h.URL,
			"content_type": "json",
			"insecure_ssl": "0",
		},
		"updated_at":     h.UpdatedAt.UTC().Format(time.RFC3339),
		"created_at":     h.CreatedAt.UTC().Format(time.RFC3339),
		"url":            hookBase,
		"test_url":       hookBase + "/test",
		"ping_url":       hookBase + "/pings",
		"deliveries_url": hookBase + "/deliveries",
		"last_response": map[string]interface{}{
			"code":    nil,
			"status":  "unused",
			"message": nil,
		},
	}
}

// deliveryStatus returns the human-readable status string GitHub uses.
func deliveryStatus(statusCode int) string {
	if statusCode >= 200 && statusCode < 300 {
		return "OK"
	}
	if statusCode == 0 {
		return "failed to connect"
	}
	return strconv.Itoa(statusCode) + " " + http.StatusText(statusCode)
}

func deliveryToJSON(d *WebhookDelivery) map[string]interface{} {
	return map[string]interface{}{
		"id":              d.ID,
		"guid":            d.GUID,
		"delivered_at":    d.DeliveredAt.UTC().Format(time.RFC3339),
		"redelivery":      d.Redelivery,
		"duration":        d.Duration,
		"status":          deliveryStatus(d.StatusCode),
		"status_code":     d.StatusCode,
		"event":           d.Event,
		"action":          d.Action,
		"installation_id": d.InstallationID,
		"repository_id":   d.RepositoryID,
		"url":             d.TargetURL,
	}
}
