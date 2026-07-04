package bleephub

import (
	"net/http"
)

func (s *Server) registerGHEnterpriseActionsRoutes() {
	s.route("GET /api/v3/enterprises/{enterprise}/actions/cache/retention-limit", s.requireEnterpriseOwner(s.handleGetEnterpriseActionsCacheRetentionLimit))
	s.route("PUT /api/v3/enterprises/{enterprise}/actions/cache/retention-limit", s.requireEnterpriseOwner(s.handleSetEnterpriseActionsCacheRetentionLimit))
	s.route("GET /api/v3/enterprises/{enterprise}/actions/cache/storage-limit", s.requireEnterpriseOwner(s.handleGetEnterpriseActionsCacheStorageLimit))
	s.route("PUT /api/v3/enterprises/{enterprise}/actions/cache/storage-limit", s.requireEnterpriseOwner(s.handleSetEnterpriseActionsCacheStorageLimit))

	s.route("GET /api/v3/enterprises/{enterprise}/actions/oidc/customization/properties/repo", s.requireEnterpriseOwner(s.handleListEnterpriseOIDCCustomProperties))
	s.route("POST /api/v3/enterprises/{enterprise}/actions/oidc/customization/properties/repo", s.requireEnterpriseOwner(s.handleCreateEnterpriseOIDCCustomProperty))
	s.route("DELETE /api/v3/enterprises/{enterprise}/actions/oidc/customization/properties/repo/{custom_property_name}", s.requireEnterpriseOwner(s.handleDeleteEnterpriseOIDCCustomProperty))
}

// --- GitHub Actions cache limits ---

func (s *Server) handleGetEnterpriseActionsCacheRetentionLimit(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	days := s.store.EnterpriseSettings.ActionsCacheRetentionDays
	s.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"max_cache_retention_days": days,
	})
}

func (s *Server) handleSetEnterpriseActionsCacheRetentionLimit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxCacheRetentionDays *int `json:"max_cache_retention_days"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.MaxCacheRetentionDays == nil || *req.MaxCacheRetentionDays <= 0 {
		writeGHError(w, http.StatusBadRequest, "Invalid request. max_cache_retention_days must be a positive integer.")
		return
	}
	s.store.SetEnterpriseActionsCacheRetentionDays(*req.MaxCacheRetentionDays)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetEnterpriseActionsCacheStorageLimit(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	gb := s.store.EnterpriseSettings.ActionsCacheSizeGB
	s.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"max_cache_size_gb": gb,
	})
}

func (s *Server) handleSetEnterpriseActionsCacheStorageLimit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxCacheSizeGB *int `json:"max_cache_size_gb"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.MaxCacheSizeGB == nil || *req.MaxCacheSizeGB <= 0 {
		writeGHError(w, http.StatusBadRequest, "Invalid request. max_cache_size_gb must be a positive integer.")
		return
	}
	s.store.SetEnterpriseActionsCacheSizeGB(*req.MaxCacheSizeGB)
	w.WriteHeader(http.StatusNoContent)
}

// --- GitHub Actions OIDC custom property inclusions ---

func enterpriseOIDCCustomPropertyJSON(name string) map[string]interface{} {
	return map[string]interface{}{
		"custom_property_name": name,
		"inclusion_source":     "enterprise",
	}
}

func (s *Server) handleListEnterpriseOIDCCustomProperties(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	names := append([]string(nil), s.store.EnterpriseSettings.OIDCCustomProperties...)
	s.store.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		out = append(out, enterpriseOIDCCustomPropertyJSON(name))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateEnterpriseOIDCCustomProperty(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CustomPropertyName string `json:"custom_property_name"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.CustomPropertyName == "" {
		writeGHValidationError(w, "OIDCCustomPropertyInclusion", "custom_property_name", "missing_field")
		return
	}
	if !s.store.AddEnterpriseOIDCCustomProperty(req.CustomPropertyName) {
		writeGHValidationError(w, "OIDCCustomPropertyInclusion", "custom_property_name", "already_exists")
		return
	}
	writeJSON(w, http.StatusCreated, enterpriseOIDCCustomPropertyJSON(req.CustomPropertyName))
}

func (s *Server) handleDeleteEnterpriseOIDCCustomProperty(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("custom_property_name")
	if !s.store.RemoveEnterpriseOIDCCustomProperty(name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
