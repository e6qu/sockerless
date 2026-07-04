package bleephub

import (
	"net/http"
	"strconv"
	"time"
)

// Organization-level webhooks (`/orgs/{org}/hooks`). Org hooks receive the
// org's own events (organization membership changes) plus every repo event
// on org-owned repositories — emitWebhookEvent fans repo events out to the
// owner org's hooks. Stored separately from repo hooks (Store.OrgHooks,
// keyed by org login) but sharing the hook ID sequence and the deliveries
// table, so delivery introspection works identically.

func (s *Server) registerGHOrgHookRoutes() {
	s.route("POST /api/v3/orgs/{org}/hooks", s.handleCreateOrgHook)
	s.route("GET /api/v3/orgs/{org}/hooks", s.handleListOrgHooks)
	s.route("GET /api/v3/orgs/{org}/hooks/{id}", s.handleGetOrgHook)
	s.route("PATCH /api/v3/orgs/{org}/hooks/{id}", s.handleUpdateOrgHook)
	s.route("DELETE /api/v3/orgs/{org}/hooks/{id}", s.handleDeleteOrgHook)
	s.route("GET /api/v3/orgs/{org}/hooks/{hook_id}/config", s.handleGetOrgHookConfig)
	s.route("PATCH /api/v3/orgs/{org}/hooks/{hook_id}/config", s.handleUpdateOrgHookConfig)
	s.route("GET /api/v3/orgs/{org}/hooks/{id}/deliveries", s.handleListOrgHookDeliveries)
	s.route("GET /api/v3/orgs/{org}/hooks/{id}/deliveries/{delivery_id}", s.handleGetOrgHookDelivery)
	s.route("POST /api/v3/orgs/{org}/hooks/{id}/deliveries/{delivery_id}/attempts", s.handleRedeliverOrgHookDelivery)
	s.route("POST /api/v3/orgs/{org}/hooks/{id}/pings", s.handlePingOrgHook)
}

// orgHookGate resolves the org and enforces the org-admin requirement
// (the sim analogue of real GitHub's admin:org_hook scope). Returns nil
// after writing the error response when the gate fails.
func (s *Server) orgHookGate(w http.ResponseWriter, r *http.Request) *Org {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return org
}

func (s *Server) handleCreateOrgHook(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
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
	// Unlike repo hooks, real GitHub REQUIRES name=web for org hooks.
	if req.Name != "web" {
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
	hook := s.store.CreateOrgHook(org.Login, req.Config.URL, req.Config.Secret,
		req.Config.ContentType, normalizeInsecureSSL(req.Config.InsecureSSL), events, active)
	s.recordAuditEvent("hook.create", ghUserFromContext(r.Context()).Login, org.Login, map[string]interface{}{"hook_id": hook.ID})

	// Real GitHub fires a `ping` event automatically on active-hook creation.
	if hook.Active {
		go s.deliverWebhook(hook, "ping", "", mustMarshal(s.orgPingPayload(org, hook, r)))
	}

	writeJSON(w, http.StatusCreated, orgHookToJSON(hook, org, s.baseURL(r)))
}

func (s *Server) handleListOrgHooks(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hooks := s.store.ListOrgHooks(org.Login)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(hooks))
	for _, h := range hooks {
		result = append(result, orgHookToJSON(h, org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// orgHookFromRequest resolves the {id} path value to a stored org hook,
// writing 404 and returning nil when it doesn't resolve.
func (s *Server) orgHookFromRequest(w http.ResponseWriter, r *http.Request, org *Org) *Webhook {
	hookID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	hook := s.store.GetOrgHook(org.Login, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return hook
}

func (s *Server) handleGetOrgHook(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	writeJSON(w, http.StatusOK, orgHookToJSON(hook, org, s.baseURL(r)))
}

func (s *Server) handleUpdateOrgHook(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	var req struct {
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
	s.store.UpdateOrgHook(org.Login, hook.ID, func(h *Webhook) {
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
	writeJSON(w, http.StatusOK, orgHookToJSON(s.store.GetOrgHook(org.Login, hook.ID), org, s.baseURL(r)))
}

// orgHookFromConfigRequest resolves {org} + {hook_id} for the webhook
// config sub-resource routes.
func (s *Server) orgHookFromConfigRequest(w http.ResponseWriter, r *http.Request) (*Org, *Webhook) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return nil, nil
	}
	hookID, err := strconv.Atoi(r.PathValue("hook_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	hook := s.store.GetOrgHook(org.Login, hookID)
	if hook == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return org, hook
}

// orgHookConfigJSON renders the webhook-config shape. The secret is masked,
// as on real GitHub, since the raw value is never surfaced after creation.
func orgHookConfigJSON(h *Webhook) map[string]interface{} {
	contentType := h.ContentType
	if contentType == "" {
		contentType = "form"
	}
	insecureSSL := h.InsecureSSL
	if insecureSSL == "" {
		insecureSSL = "0"
	}
	out := map[string]interface{}{
		"url":          h.URL,
		"content_type": contentType,
		"insecure_ssl": insecureSSL,
	}
	if h.Secret != "" {
		out["secret"] = "********"
	}
	return out
}

func (s *Server) handleGetOrgHookConfig(w http.ResponseWriter, r *http.Request) {
	_, hook := s.orgHookFromConfigRequest(w, r)
	if hook == nil {
		return
	}
	writeJSON(w, http.StatusOK, orgHookConfigJSON(hook))
}

func (s *Server) handleUpdateOrgHookConfig(w http.ResponseWriter, r *http.Request) {
	org, hook := s.orgHookFromConfigRequest(w, r)
	if hook == nil {
		return
	}
	var req struct {
		URL         string      `json:"url"`
		ContentType string      `json:"content_type"`
		Secret      string      `json:"secret"`
		InsecureSSL interface{} `json:"insecure_ssl"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	s.store.UpdateOrgHook(org.Login, hook.ID, func(h *Webhook) {
		if req.URL != "" {
			h.URL = req.URL
		}
		if req.ContentType != "" {
			h.ContentType = req.ContentType
		}
		if req.Secret != "" {
			h.Secret = req.Secret
		}
		if ssl := normalizeInsecureSSL(req.InsecureSSL); ssl != "" {
			h.InsecureSSL = ssl
		}
	})
	writeJSON(w, http.StatusOK, orgHookConfigJSON(s.store.GetOrgHook(org.Login, hook.ID)))
}

func (s *Server) handleDeleteOrgHook(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	s.store.DeleteOrgHook(org.Login, hook.ID)
	s.recordAuditEvent("hook.destroy", ghUserFromContext(r.Context()).Login, org.Login, map[string]interface{}{"hook_id": hook.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgHookDeliveries(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	deliveries := s.store.ListDeliveries(hook.ID)
	page := paginateAndLink(w, r, deliveries)
	result := make([]map[string]interface{}, 0, len(page))
	for _, d := range page {
		result = append(result, deliveryToJSON(d))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetOrgHookDelivery(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	deliveryID, err := strconv.Atoi(r.PathValue("delivery_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	for _, d := range s.store.ListDeliveries(hook.ID) {
		if d.ID == deliveryID {
			writeJSON(w, http.StatusOK, deliveryFullJSON(d))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleRedeliverOrgHookDelivery(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	deliveryID, err := strconv.Atoi(r.PathValue("delivery_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var original *WebhookDelivery
	for _, d := range s.store.ListDeliveries(hook.ID) {
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
		s.store.SetOrgHookLastResponse(hook.OrgLogin, hook.ID, deliveryLastResponse(delivery))
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handlePingOrgHook(w http.ResponseWriter, r *http.Request) {
	org := s.orgHookGate(w, r)
	if org == nil {
		return
	}
	hook := s.orgHookFromRequest(w, r, org)
	if hook == nil {
		return
	}
	go s.deliverWebhook(hook, "ping", "", mustMarshal(s.orgPingPayload(org, hook, r)))
	w.WriteHeader(http.StatusNoContent)
}

// orgPingPayload builds the `ping` event payload for an org hook.
func (s *Server) orgPingPayload(org *Org, hook *Webhook, r *http.Request) map[string]interface{} {
	return map[string]interface{}{
		"zen":          "Keep it logically awesome.",
		"hook_id":      hook.ID,
		"hook":         orgHookToJSON(hook, org, s.baseURL(r)),
		"organization": orgWebhookPayload(org),
	}
}

// orgHookToJSON serialises an org webhook to GitHub's org-hook shape.
func orgHookToJSON(h *Webhook, org *Org, baseURL string) map[string]interface{} {
	hookBase := baseURL + "/api/v3/orgs/" + org.Login + "/hooks/" + strconv.Itoa(h.ID)
	contentType := h.ContentType
	if contentType == "" {
		contentType = "form"
	}
	insecureSSL := h.InsecureSSL
	if insecureSSL == "" {
		insecureSSL = "0"
	}
	return map[string]interface{}{
		"id":     h.ID,
		"type":   "Organization",
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
		"ping_url":       hookBase + "/pings",
		"deliveries_url": hookBase + "/deliveries",
	}
}

// emitOrgWebhookEvent delivers an org-level event to the org's hooks.
func (s *Server) emitOrgWebhookEvent(orgLogin, eventType, action string, payload interface{}) {
	payloadBytes := mustMarshal(payload)
	for _, hook := range s.store.ListOrgHooks(orgLogin) {
		if !hook.Active || !hookMatchesEvent(hook, eventType) {
			continue
		}
		go s.deliverWebhook(hook, eventType, action, payloadBytes)
	}
}

// emitOrgMembershipEvent fires the `organization` event for membership
// changes (member_invited | member_added | member_removed). bleephub models
// invitations as pending memberships, so the member_invited payload's
// invitation object is derived from the membership (its id is a stable
// derivation — there is no separate invitation entity to expose).
func (s *Server) emitOrgMembershipEvent(org *Org, action string, m *Membership, target, sender *User) {
	payload := map[string]interface{}{
		"action":       action,
		"organization": orgWebhookPayload(org),
	}
	if sender != nil {
		payload["sender"] = userToJSON(sender)
	}
	if action == "member_invited" {
		invitationRole := "direct_member"
		if m.Role == OrgRoleAdmin {
			invitationRole = "admin"
		}
		payload["user"] = userToJSON(target)
		payload["invitation"] = map[string]interface{}{
			"id":         authorizationID(org.Login + "/" + target.Login),
			"login":      target.Login,
			"email":      nil,
			"role":       invitationRole,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		}
	} else {
		payload["membership"] = map[string]interface{}{
			"url":              "/api/v3/orgs/" + org.Login + "/memberships/" + target.Login,
			"organization_url": "/api/v3/orgs/" + org.Login,
			"state":            m.State,
			"role":             m.Role,
			"user":             userToJSON(target),
		}
	}
	s.emitOrgWebhookEvent(org.Login, "organization", action, payload)
}

// Store operations — the org-hook mirror of the repo-hook CRUD.

// CreateOrgHook creates a webhook on an organization.
func (st *Store) CreateOrgHook(orgLogin, url, secret, contentType, insecureSSL string, events []string, active bool) *Webhook {
	st.mu.Lock()
	defer st.mu.Unlock()

	if contentType == "" {
		contentType = "form"
	}
	if insecureSSL == "" {
		insecureSSL = "0"
	}
	now := time.Now()
	hook := &Webhook{
		ID:          st.NextHookID,
		URL:         url,
		Secret:      secret,
		ContentType: contentType,
		InsecureSSL: insecureSSL,
		Events:      events,
		Active:      active,
		OrgLogin:    orgLogin,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.NextHookID++
	st.OrgHooks[orgLogin] = append(st.OrgHooks[orgLogin], hook)
	if st.persist != nil {
		st.persist.MustPut("org_hooks", orgLogin, st.OrgHooks[orgLogin])
	}
	return hook
}

// GetOrgHook returns an org webhook by org login and hook ID, or nil.
func (st *Store) GetOrgHook(orgLogin string, hookID int) *Webhook {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, h := range st.OrgHooks[orgLogin] {
		if h.ID == hookID {
			return h
		}
	}
	return nil
}

// ListOrgHooks returns all webhooks on an organization.
func (st *Store) ListOrgHooks(orgLogin string) []*Webhook {
	st.mu.RLock()
	defer st.mu.RUnlock()
	hooks := st.OrgHooks[orgLogin]
	out := make([]*Webhook, len(hooks))
	copy(out, hooks)
	return out
}

// UpdateOrgHook updates an org webhook in place. Returns false if not found.
func (st *Store) UpdateOrgHook(orgLogin string, hookID int, fn func(h *Webhook)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, h := range st.OrgHooks[orgLogin] {
		if h.ID == hookID {
			fn(h)
			h.UpdatedAt = time.Now()
			if st.persist != nil {
				st.persist.MustPut("org_hooks", orgLogin, st.OrgHooks[orgLogin])
			}
			return true
		}
	}
	return false
}

// DeleteOrgHook removes an org webhook. Returns false if not found.
func (st *Store) DeleteOrgHook(orgLogin string, hookID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	hooks := st.OrgHooks[orgLogin]
	for i, h := range hooks {
		if h.ID == hookID {
			st.OrgHooks[orgLogin] = append(hooks[:i], hooks[i+1:]...)
			if st.persist != nil {
				if len(st.OrgHooks[orgLogin]) > 0 {
					st.persist.MustPut("org_hooks", orgLogin, st.OrgHooks[orgLogin])
				} else {
					st.persist.MustDelete("org_hooks", orgLogin)
				}
			}
			return true
		}
	}
	return false
}

// SetOrgHookLastResponse records the outcome of an org hook's most recent delivery.
func (st *Store) SetOrgHookLastResponse(orgLogin string, hookID int, lr *HookLastResponse) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, h := range st.OrgHooks[orgLogin] {
		if h.ID == hookID {
			h.LastResponse = lr
			if st.persist != nil {
				st.persist.MustPut("org_hooks", orgLogin, st.OrgHooks[orgLogin])
			}
			return
		}
	}
}
