package bleephub

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// GitHub organization private registries: Dependabot registry credentials
// configured at the organization level. The secret value is sealed against
// the org public key and stored opaque, exactly like organization secrets.

// PrivateRegistryConfiguration is an org private registry configuration.
type PrivateRegistryConfiguration struct {
	Name                     string    `json:"name"`
	RegistryType             string    `json:"registry_type"`
	AuthType                 string    `json:"auth_type"`
	URL                      string    `json:"url"`
	Username                 *string   `json:"username"`
	ReplacesBase             bool      `json:"replaces_base"`
	Visibility               string    `json:"visibility"`
	SelectedRepositoryIDs    []int     `json:"selected_repository_ids"`
	EncryptedValue           string    `json:"encrypted_value"` // opaque sealed box; never emitted
	KeyID                    string    `json:"key_id"`
	TenantID                 string    `json:"tenant_id"`
	ClientID                 string    `json:"client_id"`
	AWSRegion                string    `json:"aws_region"`
	AccountID                string    `json:"account_id"`
	RoleName                 string    `json:"role_name"`
	Domain                   string    `json:"domain"`
	DomainOwner              string    `json:"domain_owner"`
	JfrogOIDCProviderName    string    `json:"jfrog_oidc_provider_name"`
	Audience                 string    `json:"audience"`
	IdentityMappingName      string    `json:"identity_mapping_name"`
	Namespace                string    `json:"namespace"`
	ServiceSlug              string    `json:"service_slug"`
	APIHost                  string    `json:"api_host"`
	WorkloadIdentityProvider string    `json:"workload_identity_provider"`
	ServiceAccount           string    `json:"service_account"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (s *Server) registerGHPrivateRegistryRoutes() {
	s.route("GET /api/v3/orgs/{org}/private-registries",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOrgPrivateRegistries)))
	s.route("POST /api/v3/orgs/{org}/private-registries",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleCreateOrgPrivateRegistry)))
	s.route("GET /api/v3/orgs/{org}/private-registries/public-key",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgPrivateRegistriesPublicKey)))
	s.route("GET /api/v3/orgs/{org}/private-registries/{secret_name}",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgPrivateRegistry)))
	s.route("PATCH /api/v3/orgs/{org}/private-registries/{secret_name}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleUpdateOrgPrivateRegistry)))
	s.route("DELETE /api/v3/orgs/{org}/private-registries/{secret_name}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteOrgPrivateRegistry)))
}

var privateRegistryTypes = map[string]bool{
	"maven_repository": true, "nuget_feed": true, "goproxy_server": true,
	"npm_registry": true, "rubygems_server": true, "cargo_registry": true,
	"composer_repository": true, "docker_registry": true, "git_source": true,
	"helm_registry": true, "hex_organization": true, "hex_repository": true,
	"pub_repository": true, "python_index": true, "terraform_registry": true,
}

var privateRegistryAuthTypes = map[string]bool{
	"token": true, "username_password": true, "oidc_azure": true,
	"oidc_aws": true, "oidc_jfrog": true, "oidc_cloudsmith": true, "oidc_gcp": true,
}

func privateRegistryAuthIsOIDC(authType string) bool {
	return strings.HasPrefix(authType, "oidc_")
}

func (s *Server) handleListOrgPrivateRegistries(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	regs := s.store.ListPrivateRegistries(org)
	total := len(regs)
	regs = paginateAndLink(w, r, regs)
	out := make([]map[string]interface{}, 0, len(regs))
	for _, reg := range regs {
		out = append(out, privateRegistryJSON(reg, false))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":    total,
		"configurations": out,
	})
}

type privateRegistryRequest struct {
	RegistryType             *string `json:"registry_type"`
	URL                      *string `json:"url"`
	Username                 *string `json:"username"`
	ReplacesBase             *bool   `json:"replaces_base"`
	EncryptedValue           *string `json:"encrypted_value"`
	KeyID                    *string `json:"key_id"`
	Visibility               *string `json:"visibility"`
	SelectedRepositoryIDs    []int   `json:"selected_repository_ids"`
	AuthType                 *string `json:"auth_type"`
	TenantID                 *string `json:"tenant_id"`
	ClientID                 *string `json:"client_id"`
	AWSRegion                *string `json:"aws_region"`
	AccountID                *string `json:"account_id"`
	RoleName                 *string `json:"role_name"`
	Domain                   *string `json:"domain"`
	DomainOwner              *string `json:"domain_owner"`
	JfrogOIDCProviderName    *string `json:"jfrog_oidc_provider_name"`
	Audience                 *string `json:"audience"`
	IdentityMappingName      *string `json:"identity_mapping_name"`
	Namespace                *string `json:"namespace"`
	ServiceSlug              *string `json:"service_slug"`
	APIHost                  *string `json:"api_host"`
	WorkloadIdentityProvider *string `json:"workload_identity_provider"`
	ServiceAccount           *string `json:"service_account"`
}

func (s *Server) handleCreateOrgPrivateRegistry(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req privateRegistryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.RegistryType == nil || !privateRegistryTypes[*req.RegistryType] {
		writeGHValidationError(w, "PrivateRegistry", "registry_type", "invalid")
		return
	}
	if req.URL == nil || *req.URL == "" {
		writeGHValidationError(w, "PrivateRegistry", "url", "missing_field")
		return
	}
	if req.Visibility == nil || !validOrgItemVisibility(*req.Visibility) {
		writeGHValidationError(w, "PrivateRegistry", "visibility", "invalid")
		return
	}
	authType := "token"
	if req.AuthType != nil {
		if !privateRegistryAuthTypes[*req.AuthType] {
			writeGHValidationError(w, "PrivateRegistry", "auth_type", "invalid")
			return
		}
		authType = *req.AuthType
	}
	if privateRegistryAuthIsOIDC(authType) {
		if req.EncryptedValue != nil || req.KeyID != nil {
			writeGHValidationError(w, "PrivateRegistry", "encrypted_value", "invalid")
			return
		}
	} else {
		if req.EncryptedValue == nil || *req.EncryptedValue == "" {
			writeGHError(w, http.StatusUnprocessableEntity, "encrypted_value is required")
			return
		}
		if _, err := base64.StdEncoding.DecodeString(*req.EncryptedValue); err != nil {
			writeGHValidationError(w, "PrivateRegistry", "encrypted_value", "invalid")
			return
		}
		keyID := ""
		if req.KeyID != nil {
			keyID = *req.KeyID
		}
		if !s.validateDependabotSecretKeyID(w, keyID) {
			return
		}
	}
	if authType == "username_password" && (req.Username == nil || *req.Username == "") {
		writeGHError(w, http.StatusUnprocessableEntity, "username is required when auth_type is username_password")
		return
	}
	if len(req.SelectedRepositoryIDs) > 0 && *req.Visibility != "selected" {
		writeGHValidationError(w, "PrivateRegistry", "selected_repository_ids", "invalid")
		return
	}
	for _, id := range req.SelectedRepositoryIDs {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	reg := s.store.CreatePrivateRegistry(org, &req, authType)
	writeJSON(w, http.StatusCreated, privateRegistryJSON(reg, true))
}

func (s *Server) handleGetOrgPrivateRegistriesPublicKey(w http.ResponseWriter, _ *http.Request) {
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetOrgPrivateRegistry(w http.ResponseWriter, r *http.Request) {
	reg := s.store.GetPrivateRegistry(r.PathValue("org"), r.PathValue("secret_name"))
	if reg == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, privateRegistryJSON(reg, false))
}

func (s *Server) handleUpdateOrgPrivateRegistry(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	reg := s.store.GetPrivateRegistry(org, r.PathValue("secret_name"))
	if reg == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req privateRegistryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.RegistryType != nil && !privateRegistryTypes[*req.RegistryType] {
		writeGHValidationError(w, "PrivateRegistry", "registry_type", "invalid")
		return
	}
	// The authentication type cannot change after creation; a provided value
	// must match the existing one.
	if req.AuthType != nil && *req.AuthType != reg.AuthType {
		writeGHValidationError(w, "PrivateRegistry", "auth_type", "invalid")
		return
	}
	if req.Visibility != nil && !validOrgItemVisibility(*req.Visibility) {
		writeGHValidationError(w, "PrivateRegistry", "visibility", "invalid")
		return
	}
	if req.EncryptedValue != nil {
		if privateRegistryAuthIsOIDC(reg.AuthType) {
			writeGHValidationError(w, "PrivateRegistry", "encrypted_value", "invalid")
			return
		}
		if _, err := base64.StdEncoding.DecodeString(*req.EncryptedValue); err != nil {
			writeGHValidationError(w, "PrivateRegistry", "encrypted_value", "invalid")
			return
		}
		keyID := ""
		if req.KeyID != nil {
			keyID = *req.KeyID
		}
		if !s.validateDependabotSecretKeyID(w, keyID) {
			return
		}
	}
	for _, id := range req.SelectedRepositoryIDs {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	s.store.UpdatePrivateRegistry(org, reg.Name, &req)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteOrgPrivateRegistry(w http.ResponseWriter, r *http.Request) {
	if !s.store.DeletePrivateRegistry(r.PathValue("org"), r.PathValue("secret_name")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// privateRegistryJSON renders the org-private-registry-configuration shape;
// withSelected additionally carries selected_repository_ids (the create
// response shape). The sealed secret value is never emitted.
func privateRegistryJSON(reg *PrivateRegistryConfiguration, withSelected bool) map[string]interface{} {
	out := map[string]interface{}{
		"name":          reg.Name,
		"registry_type": reg.RegistryType,
		"auth_type":     reg.AuthType,
		"url":           reg.URL,
		"replaces_base": reg.ReplacesBase,
		"visibility":    reg.Visibility,
		"created_at":    reg.CreatedAt.Format(time.RFC3339),
		"updated_at":    reg.UpdatedAt.Format(time.RFC3339),
	}
	if reg.Username != nil {
		out["username"] = *reg.Username
	}
	if withSelected && reg.Visibility == "selected" {
		ids := reg.SelectedRepositoryIDs
		if ids == nil {
			ids = []int{}
		}
		out["selected_repository_ids"] = ids
	}
	for member, value := range map[string]string{
		"tenant_id":                  reg.TenantID,
		"client_id":                  reg.ClientID,
		"aws_region":                 reg.AWSRegion,
		"account_id":                 reg.AccountID,
		"role_name":                  reg.RoleName,
		"domain":                     reg.Domain,
		"domain_owner":               reg.DomainOwner,
		"jfrog_oidc_provider_name":   reg.JfrogOIDCProviderName,
		"audience":                   reg.Audience,
		"identity_mapping_name":      reg.IdentityMappingName,
		"namespace":                  reg.Namespace,
		"service_slug":               reg.ServiceSlug,
		"api_host":                   reg.APIHost,
		"workload_identity_provider": reg.WorkloadIdentityProvider,
		"service_account":            reg.ServiceAccount,
	} {
		if value != "" {
			out[member] = value
		}
	}
	return out
}

// --- store ---

// ListPrivateRegistries returns the org's registries sorted by name.
func (st *Store) ListPrivateRegistries(orgLogin string) []*PrivateRegistryConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgPrivateRegistries[orgLogin]
	out := make([]*PrivateRegistryConfiguration, 0, len(m))
	for _, reg := range m {
		out = append(out, reg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetPrivateRegistry returns a registry by configuration name, or nil.
func (st *Store) GetPrivateRegistry(orgLogin, name string) *PrivateRegistryConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgPrivateRegistries[orgLogin][name]
}

// applyPrivateRegistryRequest copies every provided member onto the
// configuration.
func applyPrivateRegistryRequest(reg *PrivateRegistryConfiguration, req *privateRegistryRequest) {
	if req.RegistryType != nil {
		reg.RegistryType = *req.RegistryType
	}
	if req.URL != nil {
		reg.URL = *req.URL
	}
	if req.Username != nil {
		reg.Username = req.Username
	}
	if req.ReplacesBase != nil {
		reg.ReplacesBase = *req.ReplacesBase
	}
	if req.EncryptedValue != nil {
		reg.EncryptedValue = *req.EncryptedValue
	}
	if req.KeyID != nil {
		reg.KeyID = *req.KeyID
	}
	if req.Visibility != nil {
		reg.Visibility = *req.Visibility
		if *req.Visibility != "selected" {
			reg.SelectedRepositoryIDs = nil
		}
	}
	if req.SelectedRepositoryIDs != nil {
		reg.SelectedRepositoryIDs = req.SelectedRepositoryIDs
	}
	for dst, src := range map[*string]*string{
		&reg.TenantID:                 req.TenantID,
		&reg.ClientID:                 req.ClientID,
		&reg.AWSRegion:                req.AWSRegion,
		&reg.AccountID:                req.AccountID,
		&reg.RoleName:                 req.RoleName,
		&reg.Domain:                   req.Domain,
		&reg.DomainOwner:              req.DomainOwner,
		&reg.JfrogOIDCProviderName:    req.JfrogOIDCProviderName,
		&reg.Audience:                 req.Audience,
		&reg.IdentityMappingName:      req.IdentityMappingName,
		&reg.Namespace:                req.Namespace,
		&reg.ServiceSlug:              req.ServiceSlug,
		&reg.APIHost:                  req.APIHost,
		&reg.WorkloadIdentityProvider: req.WorkloadIdentityProvider,
		&reg.ServiceAccount:           req.ServiceAccount,
	} {
		if src != nil {
			*dst = *src
		}
	}
}

// CreatePrivateRegistry materializes a configuration. The configuration name
// is derived from the registry type the way real GitHub names them
// (MAVEN_REPOSITORY_SECRET, ...), suffixed on collision.
func (st *Store) CreatePrivateRegistry(orgLogin string, req *privateRegistryRequest, authType string) *PrivateRegistryConfiguration {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgPrivateRegistries[orgLogin] == nil {
		st.OrgPrivateRegistries[orgLogin] = map[string]*PrivateRegistryConfiguration{}
	}
	base := strings.ToUpper(*req.RegistryType) + "_SECRET"
	name := base
	for i := 2; st.OrgPrivateRegistries[orgLogin][name] != nil; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	now := time.Now().UTC()
	reg := &PrivateRegistryConfiguration{
		Name:      name,
		AuthType:  authType,
		CreatedAt: now,
		UpdatedAt: now,
	}
	applyPrivateRegistryRequest(reg, req)
	st.OrgPrivateRegistries[orgLogin][name] = reg
	if st.persist != nil {
		st.persist.MustPut("org_private_registries", orgLogin, st.OrgPrivateRegistries[orgLogin])
	}
	return reg
}

// UpdatePrivateRegistry applies the request to an existing configuration.
func (st *Store) UpdatePrivateRegistry(orgLogin, name string, req *privateRegistryRequest) {
	st.mu.Lock()
	defer st.mu.Unlock()
	reg := st.OrgPrivateRegistries[orgLogin][name]
	if reg == nil {
		return
	}
	applyPrivateRegistryRequest(reg, req)
	reg.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("org_private_registries", orgLogin, st.OrgPrivateRegistries[orgLogin])
	}
}

// DeletePrivateRegistry removes a configuration. Returns true when it
// existed.
func (st *Store) DeletePrivateRegistry(orgLogin, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgPrivateRegistries[orgLogin][name] == nil {
		return false
	}
	delete(st.OrgPrivateRegistries[orgLogin], name)
	if st.persist != nil {
		st.persist.MustPut("org_private_registries", orgLogin, st.OrgPrivateRegistries[orgLogin])
	}
	return true
}
