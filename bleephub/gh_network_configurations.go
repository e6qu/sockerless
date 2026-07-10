package bleephub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// GitHub hosted compute networking: organizations create network
// configurations that bind hosted compute (Actions) to network settings
// resources. On real GitHub the settings resources are provisioned by the
// Azure private networking onboarding flow, not the REST API; bleephub
// mirrors that with the /internal/orgs/{org}/network-settings seed endpoint.

// NetworkConfiguration is a hosted compute network configuration.
type NetworkConfiguration struct {
	ID                         string    `json:"id"`
	OrgLogin                   string    `json:"org_login"`
	Name                       string    `json:"name"`
	ComputeService             string    `json:"compute_service"`
	NetworkSettingsIDs         []string  `json:"network_settings_ids"`
	FailoverNetworkSettingsIDs []string  `json:"failover_network_settings_ids"`
	FailoverNetworkEnabled     bool      `json:"failover_network_enabled"`
	CreatedOn                  time.Time `json:"created_on"`
}

// NetworkSettingsResource is a hosted compute network settings resource.
type NetworkSettingsResource struct {
	ID                     string `json:"id"`
	OrgLogin               string `json:"org_login"`
	Name                   string `json:"name"`
	SubnetID               string `json:"subnet_id"`
	Region                 string `json:"region"`
	NetworkConfigurationID string `json:"network_configuration_id"`
}

func (s *Server) registerGHNetworkConfigurationRoutes() {
	s.route("GET /api/v3/orgs/{org}/settings/network-configurations",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOrgNetworkConfigurations)))
	s.route("POST /api/v3/orgs/{org}/settings/network-configurations",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleCreateOrgNetworkConfiguration)))
	s.route("GET /api/v3/orgs/{org}/settings/network-configurations/{network_configuration_id}",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgNetworkConfiguration)))
	s.route("PATCH /api/v3/orgs/{org}/settings/network-configurations/{network_configuration_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleUpdateOrgNetworkConfiguration)))
	s.route("DELETE /api/v3/orgs/{org}/settings/network-configurations/{network_configuration_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleDeleteOrgNetworkConfiguration)))
	s.route("GET /api/v3/orgs/{org}/settings/network-settings/{network_settings_id}",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgNetworkSettings)))

	// Seed endpoint standing in for the Azure private networking onboarding
	// flow that provisions network settings resources on real GitHub.
	s.route("POST /internal/orgs/{org}/network-settings", s.orgGated(s.handleSeedOrgNetworkSettings))
}

var networkConfigurationNameRE = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,100}$`)

// newHostedComputeID mints the uppercase-hex resource IDs hosted compute
// networking uses.
func newHostedComputeID() (string, error) {
	return newHostedComputeIDFromReader(rand.Reader)
}

func newHostedComputeIDFromReader(random io.Reader) (string, error) {
	buf := make([]byte, 16)
	if _, err := io.ReadFull(random, buf); err != nil {
		return "", fmt.Errorf("generate hosted compute resource id: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}

func (s *Server) handleListOrgNetworkConfigurations(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	configs := s.store.ListNetworkConfigurations(org)
	total := len(configs)
	configs = paginateAndLink(w, r, configs)
	out := make([]map[string]interface{}, 0, len(configs))
	for _, c := range configs {
		out = append(out, networkConfigurationJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":            total,
		"network_configurations": out,
	})
}

type networkConfigurationRequest struct {
	Name                       *string  `json:"name"`
	ComputeService             *string  `json:"compute_service"`
	NetworkSettingsIDs         []string `json:"network_settings_ids"`
	FailoverNetworkSettingsIDs []string `json:"failover_network_settings_ids"`
	FailoverNetworkEnabled     *bool    `json:"failover_network_enabled"`
}

// validateNetworkConfigurationRequest checks the shared create/update
// constraints, writing the validation error on failure.
func (s *Server) validateNetworkConfigurationRequest(w http.ResponseWriter, org string, req *networkConfigurationRequest, settingsRequired bool) bool {
	if req.Name != nil && !networkConfigurationNameRE.MatchString(*req.Name) {
		writeGHValidationError(w, "NetworkConfiguration", "name", "invalid")
		return false
	}
	if req.ComputeService != nil && *req.ComputeService != "none" && *req.ComputeService != "actions" {
		writeGHValidationError(w, "NetworkConfiguration", "compute_service", "invalid")
		return false
	}
	if settingsRequired && len(req.NetworkSettingsIDs) != 1 {
		writeGHValidationError(w, "NetworkConfiguration", "network_settings_ids", "invalid")
		return false
	}
	if len(req.NetworkSettingsIDs) > 1 || len(req.FailoverNetworkSettingsIDs) > 1 {
		writeGHValidationError(w, "NetworkConfiguration", "network_settings_ids", "invalid")
		return false
	}
	for _, id := range append(append([]string{}, req.NetworkSettingsIDs...), req.FailoverNetworkSettingsIDs...) {
		if s.store.GetNetworkSettings(org, id) == nil {
			writeGHValidationError(w, "NetworkConfiguration", "network_settings_ids", "invalid")
			return false
		}
	}
	return true
}

func (s *Server) handleCreateOrgNetworkConfiguration(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req networkConfigurationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == nil {
		writeGHValidationError(w, "NetworkConfiguration", "name", "missing_field")
		return
	}
	if !s.validateNetworkConfigurationRequest(w, org, &req, true) {
		return
	}
	c, err := s.store.CreateNetworkConfiguration(org, &req)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, networkConfigurationJSON(c))
}

func (s *Server) handleGetOrgNetworkConfiguration(w http.ResponseWriter, r *http.Request) {
	c := s.store.GetNetworkConfiguration(r.PathValue("org"), r.PathValue("network_configuration_id"))
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, networkConfigurationJSON(c))
}

func (s *Server) handleUpdateOrgNetworkConfiguration(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	c := s.store.GetNetworkConfiguration(org, r.PathValue("network_configuration_id"))
	if c == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req networkConfigurationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.validateNetworkConfigurationRequest(w, org, &req, false) {
		return
	}
	updated := s.store.UpdateNetworkConfiguration(org, c.ID, &req)
	writeJSON(w, http.StatusOK, networkConfigurationJSON(updated))
}

func (s *Server) handleDeleteOrgNetworkConfiguration(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if !s.store.DeleteNetworkConfiguration(org, r.PathValue("network_configuration_id")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOrgNetworkSettings(w http.ResponseWriter, r *http.Request) {
	res := s.store.GetNetworkSettings(r.PathValue("org"), r.PathValue("network_settings_id"))
	if res == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, networkSettingsJSON(res))
}

func (s *Server) handleSeedOrgNetworkSettings(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	var req struct {
		Name     string `json:"name"`
		SubnetID string `json:"subnet_id"`
		Region   string `json:"region"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" || req.SubnetID == "" || req.Region == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "name, subnet_id, and region are required")
		return
	}
	res, err := s.store.CreateNetworkSettings(org, req.Name, req.SubnetID, req.Region)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, networkSettingsJSON(res))
}

func networkConfigurationJSON(c *NetworkConfiguration) map[string]interface{} {
	settingsIDs := c.NetworkSettingsIDs
	if settingsIDs == nil {
		settingsIDs = []string{}
	}
	out := map[string]interface{}{
		"id":                       c.ID,
		"name":                     c.Name,
		"compute_service":          c.ComputeService,
		"network_settings_ids":     settingsIDs,
		"failover_network_enabled": c.FailoverNetworkEnabled,
		"created_on":               c.CreatedOn.Format(time.RFC3339),
	}
	if len(c.FailoverNetworkSettingsIDs) > 0 {
		out["failover_network_settings_ids"] = c.FailoverNetworkSettingsIDs
	}
	return out
}

func networkSettingsJSON(res *NetworkSettingsResource) map[string]interface{} {
	out := map[string]interface{}{
		"id":        res.ID,
		"name":      res.Name,
		"subnet_id": res.SubnetID,
		"region":    res.Region,
	}
	if res.NetworkConfigurationID != "" {
		out["network_configuration_id"] = res.NetworkConfigurationID
	}
	return out
}

// --- store ---

// ListNetworkConfigurations returns the org's configurations sorted by
// creation time then ID.
func (st *Store) ListNetworkConfigurations(orgLogin string) []*NetworkConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	m := st.OrgNetworkConfigurations[orgLogin]
	out := make([]*NetworkConfiguration, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedOn.Equal(out[j].CreatedOn) {
			return out[i].CreatedOn.Before(out[j].CreatedOn)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// GetNetworkConfiguration returns a configuration by ID, or nil.
func (st *Store) GetNetworkConfiguration(orgLogin, id string) *NetworkConfiguration {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgNetworkConfigurations[orgLogin][id]
}

// relinkNetworkSettingsLocked points the settings resources the
// configuration references back at it, and clears stale back-references.
func (st *Store) relinkNetworkSettingsLocked(orgLogin string, c *NetworkConfiguration) {
	linked := map[string]bool{}
	for _, id := range c.NetworkSettingsIDs {
		linked[id] = true
	}
	for _, id := range c.FailoverNetworkSettingsIDs {
		linked[id] = true
	}
	for id, res := range st.OrgNetworkSettings[orgLogin] {
		switch {
		case linked[id]:
			res.NetworkConfigurationID = c.ID
		case res.NetworkConfigurationID == c.ID:
			res.NetworkConfigurationID = ""
		}
	}
	if st.persist != nil {
		st.persist.MustPut("org_network_settings", orgLogin, st.OrgNetworkSettings[orgLogin])
	}
}

// CreateNetworkConfiguration creates a configuration and links its settings
// resources.
func (st *Store) CreateNetworkConfiguration(orgLogin string, req *networkConfigurationRequest) (*NetworkConfiguration, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	id, err := newHostedComputeID()
	if err != nil {
		return nil, err
	}
	c := &NetworkConfiguration{
		ID:                         id,
		OrgLogin:                   orgLogin,
		Name:                       *req.Name,
		ComputeService:             "none",
		NetworkSettingsIDs:         req.NetworkSettingsIDs,
		FailoverNetworkSettingsIDs: req.FailoverNetworkSettingsIDs,
		CreatedOn:                  time.Now().UTC(),
	}
	if req.ComputeService != nil {
		c.ComputeService = *req.ComputeService
	}
	if req.FailoverNetworkEnabled != nil {
		c.FailoverNetworkEnabled = *req.FailoverNetworkEnabled
	}
	if st.OrgNetworkConfigurations[orgLogin] == nil {
		st.OrgNetworkConfigurations[orgLogin] = map[string]*NetworkConfiguration{}
	}
	st.OrgNetworkConfigurations[orgLogin][c.ID] = c
	st.relinkNetworkSettingsLocked(orgLogin, c)
	if st.persist != nil {
		st.persist.MustPut("org_network_configurations", orgLogin, st.OrgNetworkConfigurations[orgLogin])
	}
	return c, nil
}

// UpdateNetworkConfiguration applies provided members and relinks settings.
func (st *Store) UpdateNetworkConfiguration(orgLogin, id string, req *networkConfigurationRequest) *NetworkConfiguration {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.OrgNetworkConfigurations[orgLogin][id]
	if c == nil {
		return nil
	}
	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.ComputeService != nil {
		c.ComputeService = *req.ComputeService
	}
	if req.NetworkSettingsIDs != nil {
		c.NetworkSettingsIDs = req.NetworkSettingsIDs
	}
	if req.FailoverNetworkSettingsIDs != nil {
		c.FailoverNetworkSettingsIDs = req.FailoverNetworkSettingsIDs
	}
	if req.FailoverNetworkEnabled != nil {
		c.FailoverNetworkEnabled = *req.FailoverNetworkEnabled
	}
	st.relinkNetworkSettingsLocked(orgLogin, c)
	if st.persist != nil {
		st.persist.MustPut("org_network_configurations", orgLogin, st.OrgNetworkConfigurations[orgLogin])
	}
	return c
}

// DeleteNetworkConfiguration removes a configuration and unlinks its
// settings resources. Returns true when it existed.
func (st *Store) DeleteNetworkConfiguration(orgLogin, id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c := st.OrgNetworkConfigurations[orgLogin][id]
	if c == nil {
		return false
	}
	delete(st.OrgNetworkConfigurations[orgLogin], id)
	for _, res := range st.OrgNetworkSettings[orgLogin] {
		if res.NetworkConfigurationID == id {
			res.NetworkConfigurationID = ""
		}
	}
	if st.persist != nil {
		st.persist.MustPut("org_network_configurations", orgLogin, st.OrgNetworkConfigurations[orgLogin])
		st.persist.MustPut("org_network_settings", orgLogin, st.OrgNetworkSettings[orgLogin])
	}
	return true
}

// GetNetworkSettings returns a settings resource by ID, or nil.
func (st *Store) GetNetworkSettings(orgLogin, id string) *NetworkSettingsResource {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OrgNetworkSettings[orgLogin][id]
}

// CreateNetworkSettings provisions a settings resource for the org.
func (st *Store) CreateNetworkSettings(orgLogin, name, subnetID, region string) (*NetworkSettingsResource, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	id, err := newHostedComputeID()
	if err != nil {
		return nil, err
	}
	res := &NetworkSettingsResource{
		ID:       id,
		OrgLogin: orgLogin,
		Name:     name,
		SubnetID: subnetID,
		Region:   region,
	}
	if st.OrgNetworkSettings[orgLogin] == nil {
		st.OrgNetworkSettings[orgLogin] = map[string]*NetworkSettingsResource{}
	}
	st.OrgNetworkSettings[orgLogin][res.ID] = res
	if st.persist != nil {
		st.persist.MustPut("org_network_settings", orgLogin, st.OrgNetworkSettings[orgLogin])
	}
	return res, nil
}
