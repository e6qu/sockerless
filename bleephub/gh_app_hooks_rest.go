package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	s.route("GET /api/v3/app/hook/config", s.handleGetAppHookConfig)
	s.route("PATCH /api/v3/app/hook/config", s.handleUpdateAppHookConfig)
	s.route("GET /api/v3/app/hook/deliveries", s.handleListAppHookDeliveries)
	s.route("GET /api/v3/app/hook/deliveries/{delivery_id}", s.handleGetAppHookDelivery)
	s.route("POST /api/v3/app/hook/deliveries/{delivery_id}/attempts", s.handleRedeliverAppHookDelivery)
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
	if !decodeJSONBody(w, r, &req) {
		return
	}
	s.store.UpdateAppHookConfig(app.ID, func(a *App) {
		if req.URL != "" {
			a.WebhookURL = req.URL
		}
		if req.Secret != "" {
			a.WebhookSecret = req.Secret
		}
		if req.ContentType != "" {
			a.WebhookContentType = req.ContentType
		}
		if req.InsecureSSL != "" {
			a.WebhookInsecureSSL = req.InsecureSSL
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
}

func appHookConfigJSON(app *App) map[string]interface{} {
	contentType := app.WebhookContentType
	if contentType == "" {
		contentType = "form" // GitHub's documented default for app webhooks
	}
	insecureSSL := app.WebhookInsecureSSL
	if insecureSSL == "" {
		insecureSSL = "0"
	}
	// webhook-config carries exactly these four members — the hook's
	// active flag lives on the hook object, not its config.
	return map[string]interface{}{
		"url":          app.WebhookURL,
		"content_type": contentType,
		"insecure_ssl": insecureSSL,
		"secret":       "********", // real GH redacts; preserves contract
	}
}

func deliveryFullJSON(d *WebhookDelivery) map[string]interface{} {
	out := deliveryToJSON(d)
	// The full delivery object (unlike the list summary) carries the target url.
	out["url"] = d.TargetURL
	// request and response are required members of hook-delivery; GitHub
	// emits them with null members when nothing was captured. The HTTP
	// status lives only in the top-level status_code — the response
	// object carries exactly headers + payload.
	request := map[string]interface{}{"headers": nil, "payload": nil}
	if d.Request != nil {
		request["headers"] = d.Request.Headers
		request["payload"] = d.Request.Payload
	}
	out["request"] = request
	response := map[string]interface{}{"headers": nil, "payload": nil}
	if d.Response != nil {
		response["headers"] = d.Response.Headers
		response["payload"] = d.Response.Body
	}
	out["response"] = response
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
	list := append(st.AppHookDeliveries[appID], d)
	if len(list) > maxHookDeliveries {
		list = list[len(list)-maxHookDeliveries:]
	}
	st.AppHookDeliveries[appID] = list
	if st.persist != nil {
		st.persist.MustPut("app_hook_deliveries", strconv.Itoa(appID), list)
	}
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
