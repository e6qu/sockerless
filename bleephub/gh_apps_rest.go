package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHAppsRoutes() {
	// GitHub App API endpoints
	s.mux.HandleFunc("POST /api/v3/app-manifests/{code}/conversions", s.handleManifestConversion)
	s.mux.HandleFunc("GET /api/v3/app", s.handleGetAuthenticatedApp)
	s.mux.HandleFunc("GET /api/v3/app/installations", s.handleListAppInstallations)
	s.mux.HandleFunc("GET /api/v3/app/installations/{id}", s.handleGetAppInstallation)
	s.mux.HandleFunc("POST /api/v3/app/installations/{id}/access_tokens", s.handleCreateInstallationToken)
	s.mux.HandleFunc("DELETE /api/v3/app/installations/{id}", s.handleDeleteAppInstallation)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/installation", s.handleGetRepoInstallation)

	// Management endpoints for testing
	s.mux.HandleFunc("POST /api/v3/bleephub/apps", s.handleCreateApp)
	s.mux.HandleFunc("POST /api/v3/bleephub/apps/{app_id}/installations", s.handleCreateInstallationMgmt)
}

func (s *Server) handleManifestConversion(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	appID, ok := s.store.ConsumeManifestCode(code)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Manifest code not found or already used")
		return
	}
	app := s.store.GetApp(appID)
	if app == nil {
		writeGHError(w, http.StatusNotFound, "App not found")
		return
	}
	writeJSON(w, http.StatusCreated, appToJSON(app, true))
}

func (s *Server) handleGetAuthenticatedApp(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	writeJSON(w, http.StatusOK, appToJSON(app, false))
}

func (s *Server) handleListAppInstallations(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	installations := s.store.ListAppInstallations(app.ID)
	result := make([]map[string]interface{}, 0, len(installations))
	for _, inst := range installations {
		result = append(result, installationToJSON(inst))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetAppInstallation(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}
	writeJSON(w, http.StatusOK, installationToJSON(inst))
}

func (s *Server) handleCreateInstallationToken(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}

	// Optional permissions override from request body
	perms := inst.Permissions
	var body struct {
		Permissions map[string]string `json:"permissions"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Permissions != nil {
			perms = body.Permissions
		}
	}

	token := s.store.CreateInstallationToken(inst.ID, app.ID, perms)
	writeJSON(w, http.StatusCreated, installationTokenToJSON(token))
}

func (s *Server) handleDeleteAppInstallation(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}
	s.store.DeleteInstallation(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetRepoInstallation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	owner := r.PathValue("owner")
	inst := s.store.GetRepoInstallation(owner)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, installationToJSON(inst))
}

// Management endpoints

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Permissions map[string]string `json:"permissions"`
		Events      []string          `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "App", "name", "missing_field")
		return
	}

	app := s.store.CreateApp(user.ID, req.Name, req.Description, req.Permissions, req.Events)
	writeJSON(w, http.StatusCreated, appToJSON(app, true))
}

func (s *Server) handleCreateInstallationMgmt(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	appID, err := strconv.Atoi(r.PathValue("app_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid app ID")
		return
	}
	app := s.store.GetApp(appID)
	if app == nil {
		writeGHError(w, http.StatusNotFound, "App not found")
		return
	}

	var req struct {
		TargetType  string            `json:"target_type"`
		TargetID    int               `json:"target_id"`
		TargetLogin string            `json:"target_login"`
		Permissions map[string]string `json:"permissions"`
		Events      []string          `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.TargetType == "" {
		req.TargetType = "User"
	}

	inst := s.store.CreateInstallation(appID, req.TargetType, req.TargetID, req.TargetLogin, req.Permissions, req.Events)
	writeJSON(w, http.StatusCreated, installationToJSON(inst))
}

// JSON serializers

func appToJSON(app *App, includePEM bool) map[string]interface{} {
	result := map[string]interface{}{
		"id":            app.ID,
		"node_id":       app.NodeID,
		"slug":          app.Slug,
		"name":          app.Name,
		"client_id":     app.ClientID,
		"description":   app.Description,
		"external_url":  app.ExternalURL,
		"permissions":   app.Permissions,
		"events":        app.Events,
		"created_at":    app.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":    app.UpdatedAt.UTC().Format(time.RFC3339),
		"owner": map[string]interface{}{
			"login": "admin",
			"id":    app.OwnerID,
			"type":  "User",
		},
	}
	if includePEM {
		result["pem"] = app.PEMPrivateKey
	}
	return result
}

func installationToJSON(inst *Installation) map[string]interface{} {
	return map[string]interface{}{
		"id":     inst.ID,
		"app_id": inst.AppID,
		"app_slug":             inst.AppSlug,
		"target_type":          inst.TargetType,
		"target_id":            inst.TargetID,
		"permissions":          inst.Permissions,
		"events":               inst.Events,
		"repository_selection": inst.RepositorySelection,
		"created_at":           inst.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":           inst.UpdatedAt.UTC().Format(time.RFC3339),
		"account": map[string]interface{}{
			"login": inst.TargetLogin,
			"id":    inst.TargetID,
			"type":  inst.TargetType,
		},
	}
}

func installationTokenToJSON(token *InstallationToken) map[string]interface{} {
	return map[string]interface{}{
		"token":       token.Token,
		"expires_at":  token.ExpiresAt.UTC().Format(time.RFC3339),
		"permissions": token.Permissions,
	}
}
