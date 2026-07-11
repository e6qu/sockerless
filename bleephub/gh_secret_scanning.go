package bleephub

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHSecretScanningRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/secret-scanning/alerts", s.handleListSecretScanningAlerts)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/secret-scanning/alerts", s.handleBulkUpdateSecretScanningAlerts)
	s.route("GET /api/v3/repos/{owner}/{repo}/secret-scanning/alerts/{alert_number}", s.handleGetSecretScanningAlert)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/secret-scanning/alerts/{alert_number}", s.handleUpdateSecretScanningAlert)
	s.route("GET /api/v3/repos/{owner}/{repo}/secret-scanning/alerts/{alert_number}/locations", s.handleListSecretScanningAlertLocations)

	// Organization-level alerts and pattern configurations
	s.route("GET /api/v3/orgs/{org}/secret-scanning/alerts",
		s.requireOrgAdmin(scopeSecurityEvents, permRead, s.handleListSecretScanningOrgAlerts))
	s.route("GET /api/v3/orgs/{org}/secret-scanning/pattern-configurations",
		s.requireOrgAdmin(scopeSecurityEvents, permRead, s.handleListSecretScanningPatternConfigurations))
	s.route("PATCH /api/v3/orgs/{org}/secret-scanning/pattern-configurations",
		s.requireOrgAdmin(scopeSecurityEvents, permWrite, s.handleUpdateSecretScanningPatternConfigurations))

	// Push protection bypasses + scan history
	s.route("POST /api/v3/repos/{owner}/{repo}/secret-scanning/push-protection-bypasses", s.handleCreateSecretScanningPushProtectionBypass)
	s.route("GET /api/v3/repos/{owner}/{repo}/secret-scanning/scan-history", s.handleGetSecretScanningScanHistory)
}

func (s *Server) handleListSecretScanningAlerts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	state := r.URL.Query().Get("state")
	secretType := r.URL.Query().Get("secret_type")
	resolution := r.URL.Query().Get("resolution")
	sort := r.URL.Query().Get("sort")
	direction := r.URL.Query().Get("direction")

	alerts := s.store.ListSecretScanningAlerts(repo.FullName, state, secretType, resolution, sort, direction)
	page := paginateAndLink(w, r, alerts)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(page))
	for i, a := range page {
		out[i] = secretScanningAlertToJSON(a, baseURL, repo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetSecretScanningAlert(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	a := s.lookupSecretScanningAlert(w, r, repo)
	if a == nil {
		return
	}
	writeJSON(w, http.StatusOK, secretScanningAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) handleUpdateSecretScanningAlert(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	a := s.lookupSecretScanningAlert(w, r, repo)
	if a == nil {
		return
	}

	var req struct {
		State             string `json:"state"`
		Resolution        string `json:"resolution"`
		ResolutionComment string `json:"resolution_comment"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.State == "" && req.Resolution == "" {
		writeGHValidationError(w, "SecretScanningAlert", "state", "missing_field")
		return
	}
	if err := s.store.UpdateSecretScanningAlert(a, req.State, req.Resolution, req.ResolutionComment); err != nil {
		writeGHValidationError(w, "SecretScanningAlert", "state", "invalid")
		return
	}
	writeJSON(w, http.StatusOK, secretScanningAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) handleBulkUpdateSecretScanningAlerts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		State             string `json:"state"`
		Resolution        string `json:"resolution"`
		ResolutionComment string `json:"resolution_comment"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.State != "resolved" || !isValidResolution(req.Resolution) {
		writeGHValidationError(w, "SecretScanningAlert", "state", "invalid")
		return
	}

	state := r.URL.Query().Get("state")
	secretType := r.URL.Query().Get("secret_type")
	resolution := r.URL.Query().Get("resolution")

	updated, err := s.store.BulkUpdateSecretScanningAlerts(repo.FullName, state, secretType, resolution, req.Resolution, req.ResolutionComment)
	if err != nil {
		writeGHValidationError(w, "SecretScanningAlert", "state", "invalid")
		return
	}

	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(updated))
	for i, a := range updated {
		out[i] = secretScanningAlertToJSON(a, baseURL, repo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListSecretScanningAlertLocations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	a := s.lookupSecretScanningAlert(w, r, repo)
	if a == nil {
		return
	}
	out := make([]map[string]interface{}, len(a.Locations))
	for i, loc := range a.Locations {
		out[i] = secretScanningLocationToJSON(loc)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListSecretScanningOrgAlerts(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForSecretScanning(w, r)
	if !ok {
		return
	}

	alerts := s.store.ListSecretScanningAlertsByOrg(org.ID)
	page := paginateAndLink(w, r, alerts)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		repo := s.store.ReposByName[a.RepoKey]
		if repo == nil {
			continue
		}
		alertJSON := secretScanningAlertToJSON(a, baseURL, repo)
		alertJSON["repository"] = simpleRepoJSON(repo, s.store, baseURL)
		out = append(out, alertJSON)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListSecretScanningPatternConfigurations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.ListSecretScanningPatternConfigurations(r.PathValue("org")))
}

func validPushProtectionSetting(setting string, allowNotSet bool) bool {
	switch setting {
	case "enabled", "disabled":
		return true
	case "not-set":
		return allowNotSet
	}
	return false
}

func (s *Server) handleUpdateSecretScanningPatternConfigurations(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	var req struct {
		PatternConfigVersion    *string `json:"pattern_config_version"`
		ProviderPatternSettings []struct {
			TokenType             string `json:"token_type"`
			PushProtectionSetting string `json:"push_protection_setting"`
		} `json:"provider_pattern_settings"`
		CustomPatternSettings []struct {
			TokenType             string  `json:"token_type"`
			CustomPatternVersion  *string `json:"custom_pattern_version"`
			PushProtectionSetting string  `json:"push_protection_setting"`
		} `json:"custom_pattern_settings"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	provider := map[string]string{}
	for _, setting := range req.ProviderPatternSettings {
		if !isSecretScanningProviderPattern(setting.TokenType) {
			writeGHValidationError(w, "SecretScanningPatternConfiguration", "token_type", "invalid")
			return
		}
		if !validPushProtectionSetting(setting.PushProtectionSetting, true) {
			writeGHValidationError(w, "SecretScanningPatternConfiguration", "push_protection_setting", "invalid")
			return
		}
		provider[setting.TokenType] = setting.PushProtectionSetting
	}
	custom := map[string]string{}
	for _, setting := range req.CustomPatternSettings {
		if setting.TokenType == "" {
			writeGHValidationError(w, "SecretScanningPatternConfiguration", "token_type", "missing_field")
			return
		}
		if !validPushProtectionSetting(setting.PushProtectionSetting, false) {
			writeGHValidationError(w, "SecretScanningPatternConfiguration", "push_protection_setting", "invalid")
			return
		}
		custom[setting.TokenType] = setting.PushProtectionSetting
	}

	newVersion, ok := s.store.UpdateSecretScanningPatternConfig(orgLogin, req.PatternConfigVersion, provider, custom)
	if !ok {
		writeGHError(w, http.StatusConflict, "pattern_config_version does not match the current configuration version")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pattern_config_version": newVersion,
	})
}

func (s *Server) handleCreateSecretScanningPushProtectionBypass(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "User does not have enough permissions to perform this action.")
		return
	}

	var req struct {
		Reason        string `json:"reason"`
		PlaceholderID string `json:"placeholder_id"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.Reason {
	case "false_positive", "used_in_tests", "will_fix_later":
	default:
		writeGHValidationError(w, "SecretScanningPushProtectionBypass", "reason", "invalid")
		return
	}
	if req.PlaceholderID == "" {
		writeGHValidationError(w, "SecretScanningPushProtectionBypass", "placeholder_id", "missing_field")
		return
	}

	bypass := s.store.CreateSecretScanningPushProtectionBypass(repo.FullName, req.PlaceholderID, req.Reason)
	if bypass == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"reason":     bypass.Reason,
		"expire_at":  bypass.ExpireAt.UTC().Format(time.RFC3339),
		"token_type": bypass.TokenType,
	})
}

func writeSecretScanningPushProtectionBlocked(w http.ResponseWriter, ph *SecretScanningPushProtectionPlaceholder) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
		"message":        "Push cannot contain secrets.",
		"placeholder_id": ph.ID,
		"token_type":     ph.TokenType,
		"errors": []map[string]interface{}{
			{
				"resource":       "SecretScanningPushProtectionBypass",
				"field":          "placeholder_id",
				"code":           "custom",
				"placeholder_id": ph.ID,
				"token_type":     ph.TokenType,
			},
		},
	})
}

func secretScanningScanRecordsJSON(records []*SecretScanningScanRecord) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		out = append(out, map[string]interface{}{
			"type":         rec.Type,
			"status":       rec.Status,
			"started_at":   rec.StartedAt.UTC().Format(time.RFC3339),
			"completed_at": rec.CompletedAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func (s *Server) handleGetSecretScanningScanHistory(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	incremental, patternUpdate, backfill := s.store.SecretScanningScanHistory(repo)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"incremental_scans":              secretScanningScanRecordsJSON(incremental),
		"pattern_update_scans":           secretScanningScanRecordsJSON(patternUpdate),
		"backfill_scans":                 secretScanningScanRecordsJSON(backfill),
		"custom_pattern_backfill_scans":  []map[string]interface{}{},
		"generic_secrets_backfill_scans": []map[string]interface{}{},
	})
}

// func (s *Server) handleListSecretScanningUserAlerts(w http.ResponseWriter, r *http.Request) {
// 	user := ghUserFromContext(r.Context())
// 	alerts := s.store.ListSecretScanningAlertsByUser(user.ID)
// 	page := paginateAndLink(w, r, alerts)
// 	baseURL := s.baseURL(r)
// 	out := make([]map[string]interface{}, 0, len(page))
// 	for _, a := range page {
// 		repo := s.store.ReposByName[a.RepoKey]
// 		if repo == nil {
// 			continue
// 		}
// 		out = append(out, secretScanningAlertToJSON(a, baseURL, repo))
// 	}
// 	writeJSON(w, http.StatusOK, out)
// }

func (s *Server) lookupSecretScanningAlert(w http.ResponseWriter, r *http.Request, repo *Repo) *SecretScanningAlert {
	number, err := strconv.Atoi(r.PathValue("alert_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	a := s.store.GetSecretScanningAlert(repo.FullName, number)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return a
}

func secretScanningAlertToJSON(a *SecretScanningAlert, baseURL string, repo *Repo) map[string]interface{} {
	apiURL := fmt.Sprintf("%s/api/v3/repos/%s/secret-scanning/alerts/%d", baseURL, repo.FullName, a.Number)
	htmlURL := fmt.Sprintf("%s/%s/security/secret-scanning/%d", baseURL, repo.FullName, a.Number)
	locationsURL := fmt.Sprintf("%s/locations", apiURL)

	resolvedAt := interface{}(nil)
	if a.ResolvedAt != nil {
		resolvedAt = a.ResolvedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	return map[string]interface{}{
		"number":                   a.Number,
		"created_at":               a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":               a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"url":                      apiURL,
		"html_url":                 htmlURL,
		"locations_url":            locationsURL,
		"state":                    a.State,
		"resolution":               nullOrString(a.Resolution),
		"resolved_at":              resolvedAt,
		"secret_type":              a.SecretType,
		"secret_type_display_name": a.SecretTypeDisplayName,
	}
}

func secretScanningLocationToJSON(loc SecretScanningLocation) map[string]interface{} {
	return map[string]interface{}{
		"type": loc.Type,
		"details": map[string]interface{}{
			"path":         loc.Details.Path,
			"start_line":   loc.Details.StartLine,
			"end_line":     loc.Details.EndLine,
			"start_column": loc.Details.StartColumn,
			"end_column":   loc.Details.EndColumn,
			"blob_sha":     loc.Details.BlobSHA,
			"blob_url":     loc.Details.BlobURL,
			"commit_sha":   loc.Details.CommitSHA,
			"commit_url":   loc.Details.CommitURL,
			"html_url":     loc.Details.HTMLURL,
		},
	}
}

func nullOrString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// requireUserSelf ensures the authenticated user is the user named in the
// {username} path segment. It returns 404 for mismatches, matching real
// GitHub's behavior for user-scoped resources.
// func (s *Server) requireUserSelf(next http.HandlerFunc) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		user := ghUserFromContext(r.Context())
// 		target := s.store.LookupUserByLogin(r.PathValue("username"))
// 		if target == nil || user == nil || target.ID != user.ID {
// 			writeGHError(w, http.StatusNotFound, "Not Found")
// 			return
// 		}
// 		next(w, r)
// 	}
// }

// resolveOrgForSecretScanning resolves {org} for secret-scanning handlers.
func (s *Server) resolveOrgForSecretScanning(w http.ResponseWriter, r *http.Request) (*Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return org, true
}
