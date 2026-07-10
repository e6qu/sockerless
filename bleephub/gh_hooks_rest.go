package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHHookRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/hooks", s.requirePerm(scopeAdministration, permWrite, s.handleCreateHook))
	s.route("GET /api/v3/repos/{owner}/{repo}/hooks", s.requirePerm(scopeAdministration, permRead, s.handleListHooks))
	s.route("GET /api/v3/repos/{owner}/{repo}/hooks/{id}", s.requirePerm(scopeAdministration, permRead, s.handleGetHook))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/hooks/{id}", s.requirePerm(scopeAdministration, permWrite, s.handleUpdateHook))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/hooks/{id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteHook))
	s.route("GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries", s.requirePerm(scopeAdministration, permRead, s.handleListHookDeliveries))
	s.route("GET /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries/{delivery_id}", s.requirePerm(scopeAdministration, permRead, s.handleGetHookDelivery))
	s.route("POST /api/v3/repos/{owner}/{repo}/hooks/{id}/deliveries/{delivery_id}/attempts", s.requirePerm(scopeAdministration, permWrite, s.handleRedeliverHookDelivery))
	s.route("POST /api/v3/repos/{owner}/{repo}/hooks/{id}/pings", s.requirePerm(scopeAdministration, permWrite, s.handlePingHook))
	s.route("GET /api/v3/repos/{owner}/{repo}/hooks/{id}/config", s.requirePerm(scopeAdministration, permRead, s.handleGetHookConfig))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/hooks/{id}/config", s.requirePerm(scopeAdministration, permWrite, s.handleUpdateHookConfig))
	s.route("POST /api/v3/repos/{owner}/{repo}/hooks/{id}/tests", s.requirePerm(scopeAdministration, permWrite, s.handleTestHook))
}

func (s *Server) handleCreateHook(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")

	var req struct {
		Name   string `json:"name"`
		Config struct {
			URL         string      `json:"url"`
			Secret      string      `json:"secret"`
			ContentType string      `json:"content_type"`
			InsecureSSL interface{} `json:"insecure_ssl"`
		} `json:"config"`
		Events []string  `json:"events"`
		Active *flexBool `json:"active"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	// GitHub only supports the "web" hook type via the REST API.
	if req.Name != "" && req.Name != "web" {
		writeGHValidationError(w, "Hook", "name", "invalid")
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

	hook := s.store.CreateHook(repoKey, req.Config.URL, req.Config.Secret,
		req.Config.ContentType, normalizeInsecureSSL(req.Config.InsecureSSL), events, active)
	s.recordAuditEvent("hook.create", user.Login, "", map[string]interface{}{"repo": repoKey, "hook_id": hook.ID})

	// Real GitHub fires a `ping` event automatically when an active hook
	// is created (so the consumer can verify the endpoint). Inactive hooks
	// receive no deliveries.
	if hook.Active {
		repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
		go s.deliverWebhook(hook, "ping", "", mustMarshal(buildPingPayload(repo, hook)))
	}

	writeJSON(w, http.StatusCreated, hookToJSON(hook, s.store.HookLastResp(hook), r, r.PathValue("owner"), r.PathValue("repo")))
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
		result = append(result, hookToJSON(h, s.store.HookLastResp(h), r, owner, repo))
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

	writeJSON(w, http.StatusOK, hookToJSON(hook, s.store.HookLastResp(hook), r, r.PathValue("owner"), r.PathValue("repo")))
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
		Name   string `json:"name"`
		Config *struct {
			URL         string      `json:"url"`
			Secret      string      `json:"secret"`
			ContentType string      `json:"content_type"`
			InsecureSSL interface{} `json:"insecure_ssl"`
		} `json:"config"`
		Events []string  `json:"events"`
		Active *flexBool `json:"active"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Name != "" && req.Name != "web" {
		writeGHValidationError(w, "Hook", "name", "invalid")
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
			if req.Config.ContentType != "" {
				h.ContentType = req.Config.ContentType
			}
			if ssl := normalizeInsecureSSL(req.Config.InsecureSSL); ssl != "" {
				h.InsecureSSL = ssl
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
	writeJSON(w, http.StatusOK, hookToJSON(hook, s.store.HookLastResp(hook), r, r.PathValue("owner"), r.PathValue("repo")))
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

	s.recordAuditEvent("hook.destroy", user.Login, "", map[string]interface{}{"repo": repoKey, "hook_id": hookID})
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

	deliveries := paginateAndLink(w, r, s.store.ListDeliveries(hookID))
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

// hookConfigJSON renders a webhook's config sub-object in the published
// webhook-config shape. The secret is never echoed back in cleartext — real
// GitHub masks a configured secret as "********".
func hookConfigJSON(h *Webhook) map[string]interface{} {
	config := map[string]interface{}{
		"url":          h.URL,
		"content_type": coalesceStr(h.ContentType, "form"),
		"insecure_ssl": coalesceStr(h.InsecureSSL, "0"),
	}
	if h.Secret != "" {
		config["secret"] = "********"
	}
	return config
}

// handleGetHookConfig — GET /repos/{o}/{r}/hooks/{id}/config.
func (s *Server) handleGetHookConfig(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, hookConfigJSON(hook))
}

// handleUpdateHookConfig — PATCH /repos/{o}/{r}/hooks/{id}/config. Updates
// the config sub-view of the webhook: present members replace the stored
// value, absent members are left unchanged.
func (s *Server) handleUpdateHookConfig(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		URL         *string     `json:"url"`
		ContentType *string     `json:"content_type"`
		Secret      *string     `json:"secret"`
		InsecureSSL interface{} `json:"insecure_ssl"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	found := s.store.UpdateHook(repoKey, hookID, func(h *Webhook) {
		if req.URL != nil {
			h.URL = *req.URL
		}
		if req.ContentType != nil {
			h.ContentType = *req.ContentType
		}
		if req.Secret != nil {
			h.Secret = *req.Secret
		}
		if ssl := normalizeInsecureSSL(req.InsecureSSL); ssl != "" {
			h.InsecureSSL = ssl
		}
	})
	if !found {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, hookConfigJSON(s.store.GetHook(repoKey, hookID)))
}

// handleTestHook — POST /repos/{o}/{r}/hooks/{id}/tests. Triggers the hook
// with a push event for the repository's latest push (the default branch
// head). When the hook is not subscribed to push events no delivery is
// generated, but the response is still 204 — matching real GitHub.
func (s *Server) handleTestHook(w http.ResponseWriter, r *http.Request) {
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
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if hookMatchesEvent(hook, "push") {
		sender := ghUserFromContext(r.Context())
		branch := repo.DefaultBranch
		headSha := resolveBranchSha(s.store.GetGitStorage(owner, repoName), branch)
		if headSha == "" {
			writeGHError(w, http.StatusUnprocessableEntity, "No default branch commit found")
			return
		}
		payload := buildPushPayload(s.store, repo, sender, "refs/heads/"+branch, headSha, headSha)
		go s.deliverWebhook(hook, "push", "", mustMarshal(payload))
	}
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
		if hook.RepoKey != "" {
			s.store.SetHookLastResponse(hook.RepoKey, hook.ID, deliveryLastResponse(delivery))
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

// hookToJSON serialises a Webhook to GitHub's published hook object shape.
// r and owner/repo are needed to construct the self-referential API URLs.
func hookToJSON(h *Webhook, lastResp *HookLastResponse, r *http.Request, owner, repo string) map[string]interface{} {
	base := "http://" + r.Host
	hookBase := base + "/api/v3/repos/" + owner + "/" + repo + "/hooks/" + strconv.Itoa(h.ID)

	contentType := h.ContentType
	if contentType == "" {
		contentType = "form"
	}
	insecureSSL := h.InsecureSSL
	if insecureSSL == "" {
		insecureSSL = "0"
	}

	return map[string]interface{}{
		"type":   "Repository",
		"id":     h.ID,
		"name":   "web",
		"active": h.Active,
		"events": h.Events,
		"config": map[string]interface{}{
			"url":          h.URL,
			"content_type": contentType,
			"insecure_ssl": insecureSSL,
		},
		"updated_at":     h.UpdatedAt.UTC().Format(time.RFC3339),
		"created_at":     h.CreatedAt.UTC().Format(time.RFC3339),
		"url":            hookBase,
		"test_url":       hookBase + "/test",
		"ping_url":       hookBase + "/pings",
		"deliveries_url": hookBase + "/deliveries",
		"last_response":  hookLastResponseJSON(lastResp),
	}
}

// hookLastResponseJSON renders the hook's last_response field. Before any
// delivery has occurred GitHub returns {code:null,status:"unused",message:null}.
func hookLastResponseJSON(lr *HookLastResponse) map[string]interface{} {
	if lr == nil {
		return map[string]interface{}{
			"code":    nil,
			"status":  "unused",
			"message": nil,
		}
	}
	return map[string]interface{}{
		"code":    lr.Code,
		"status":  lr.Status,
		"message": lr.Message,
	}
}

// normalizeInsecureSSL coerces GitHub's insecure_ssl config value (which clients
// send as either a string "0"/"1" or a JSON number 0/1) to the canonical
// string form. Returns "" when unset so callers can preserve the stored value.
func normalizeInsecureSSL(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == 1 {
			return "1"
		}
		return "0"
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return ""
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
	out := map[string]interface{}{
		"id":              d.ID,
		"guid":            d.GUID,
		"delivered_at":    d.DeliveredAt.UTC().Format(time.RFC3339),
		"redelivery":      d.Redelivery,
		"duration":        d.Duration,
		"status":          deliveryStatus(d.StatusCode),
		"status_code":     d.StatusCode,
		"event":           d.Event,
		"action":          nullableString(d.Action),
		"installation_id": nullableInt(d.InstallationID),
		"repository_id":   nullableInt(d.RepositoryID),
		"throttled_at":    nil,
	}
	if d.ThrottledAt != nil {
		out["throttled_at"] = d.ThrottledAt.UTC().Format(time.RFC3339)
	}
	return out
}

// nullableInt renders 0 as JSON null (GitHub emits unset nullable ids as null).
func nullableInt(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

// nullableString renders "" as JSON null.
func nullableString(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}
