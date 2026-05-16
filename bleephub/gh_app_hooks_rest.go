package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// app-level webhook config + deliveries.
// Distinct from the per-repo webhooks (`/repos/{o}/{r}/hooks`) shipped earlier.
// A GitHub App owns exactly one webhook URL; events targeted at the app
// (installation, installation_repositories, github_app_authorization, plus
// repo events when the app is installed there) deliver to this single URL.
//
// JWT-authenticated. App deliveries are stored separately from per-repo
// deliveries (Store.AppHookDeliveries, keyed by app ID).

func (s *Server) registerGHAppHookRoutes() {
	s.mux.HandleFunc("GET /api/v3/app/hook/config", s.handleGetAppHookConfig)
	s.mux.HandleFunc("PATCH /api/v3/app/hook/config", s.handleUpdateAppHookConfig)
	s.mux.HandleFunc("GET /api/v3/app/hook/deliveries", s.handleListAppHookDeliveries)
	s.mux.HandleFunc("GET /api/v3/app/hook/deliveries/{delivery_id}", s.handleGetAppHookDelivery)
	s.mux.HandleFunc("POST /api/v3/app/hook/deliveries/{delivery_id}/attempts", s.handleRedeliverAppHookDelivery)
}

func (s *Server) handleGetAppHookConfig(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	writeJSON(w, http.StatusOK, appHookConfigJSON(app))
}

func (s *Server) handleUpdateAppHookConfig(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	var req struct {
		URL         string `json:"url"`
		Secret      string `json:"secret"`
		ContentType string `json:"content_type"`
		InsecureSSL string `json:"insecure_ssl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	s.store.UpdateAppHookConfig(app.ID, func(a *App) {
		if req.URL != "" {
			a.WebhookURL = req.URL
		}
		if req.Secret != "" {
			a.WebhookSecret = req.Secret
		}
	})
	app = s.store.GetApp(app.ID)
	writeJSON(w, http.StatusOK, appHookConfigJSON(app))
}

func (s *Server) handleListAppHookDeliveries(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	deliveries := s.store.ListAppDeliveries(app.ID)
	page := paginateAndLink(w, r, deliveries)
	out := make([]map[string]interface{}, 0, len(page))
	for _, d := range page {
		out = append(out, deliveryToJSON(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetAppHookDelivery(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("delivery_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.GetAppDelivery(app.ID, id)
	if d == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, deliveryFullJSON(d))
}

func (s *Server) handleRedeliverAppHookDelivery(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("delivery_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	d := s.store.GetAppDelivery(app.ID, id)
	if d == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if app.WebhookURL == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "App has no webhook URL configured")
		return
	}
	go s.redeliverAppWebhook(app, d)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "redelivery": true})
}

func appHookConfigJSON(app *App) map[string]interface{} {
	active := app.WebhookActive
	return map[string]interface{}{
		"url":          app.WebhookURL,
		"content_type": "json",
		"insecure_ssl": "0",
		"secret":       "********", // real GH redacts; preserves contract
		"active":       active,
	}
}

func deliveryFullJSON(d *WebhookDelivery) map[string]interface{} {
	out := deliveryToJSON(d)
	out["installation_id"] = d.InstallationID
	out["repository_id"] = d.RepositoryID
	if d.Request != nil {
		out["request"] = map[string]interface{}{
			"headers": d.Request.Headers,
			"payload": d.Request.Payload,
		}
	}
	if d.Response != nil {
		out["response"] = map[string]interface{}{
			"status_code": d.Response.StatusCode,
			"headers":     d.Response.Headers,
			"payload":     d.Response.Body,
		}
	}
	return out
}

// AddAppDelivery records an app-level webhook delivery on the App's queue.
func (st *Store) AddAppDelivery(appID int, d *WebhookDelivery) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.AppHookDeliveries == nil {
		st.AppHookDeliveries = make(map[int][]*WebhookDelivery)
	}
	d.ID = st.NextDeliveryID
	st.NextDeliveryID++
	st.AppHookDeliveries[appID] = append(st.AppHookDeliveries[appID], d)
}

// ListAppDeliveries returns app-level deliveries newest-first.
func (st *Store) ListAppDeliveries(appID int) []*WebhookDelivery {
	st.mu.RLock()
	defer st.mu.RUnlock()
	src := st.AppHookDeliveries[appID]
	out := make([]*WebhookDelivery, len(src))
	copy(out, src)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// GetAppDelivery returns a single app-level delivery.
func (st *Store) GetAppDelivery(appID, deliveryID int) *WebhookDelivery {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, d := range st.AppHookDeliveries[appID] {
		if d.ID == deliveryID {
			return d
		}
	}
	return nil
}

// redeliverAppWebhook re-runs the delivery against the App's current webhook URL.
func (s *Server) redeliverAppWebhook(app *App, original *WebhookDelivery) {
	if app.WebhookURL == "" {
		return
	}
	payloadBytes, _ := json.Marshal(original.Request.Payload)
	hook := &Webhook{
		ID:     -app.ID, // pseudo-hook id for app deliveries
		URL:    app.WebhookURL,
		Secret: app.WebhookSecret,
		Events: app.WebhookEvents,
		Active: app.WebhookActive,
	}
	delivery := s.doDeliverAttempt(hook, original.Event, original.Action, original.GUID, payloadBytes, true)
	delivery.HookID = -app.ID
	delivery.AppID = app.ID
	delivery.InstallationID = original.InstallationID
	s.store.AddAppDelivery(app.ID, delivery)
}

// Quiet linter: ensure UpdateAt timestamp helper exists for store callers
// outside this file. (No-op for now; placeholder for future broadcaster.)
var _ = time.Now
