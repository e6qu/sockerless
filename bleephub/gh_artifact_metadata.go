package bleephub

import (
	"net/http"
	"strings"
	"time"
)

// Organization artifact metadata REST API: storage and deployment
// records for artifacts identified by subject digest.

func (s *Server) registerGHOrgArtifactMetadataRoutes() {
	s.route("POST /api/v3/orgs/{org}/artifacts/metadata/storage-record", s.handleOrgCreateArtifactStorageRecord)
	s.route("POST /api/v3/orgs/{org}/artifacts/metadata/deployment-record", s.handleOrgCreateArtifactDeploymentRecord)
	s.route("POST /api/v3/orgs/{org}/artifacts/metadata/deployment-record/cluster/{cluster}", s.handleOrgSetClusterDeploymentRecords)
	s.route("GET /api/v3/orgs/{org}/artifacts/{subject_digest}/metadata/storage-records", s.handleOrgListArtifactStorageRecords)
	s.route("GET /api/v3/orgs/{org}/artifacts/{subject_digest}/metadata/deployment-records", s.handleOrgListArtifactDeploymentRecords)
}

// requireOrgArtifactMetadataWrite resolves {org} and enforces org-admin
// rights for metadata writes; 404 for an unknown org, 401/403 on auth
// failures (the documented denial for these endpoints).
func (s *Server) requireOrgArtifactMetadataWrite(w http.ResponseWriter, r *http.Request) (*Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil, false
	}
	if !user.SiteAdmin && !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization admin.")
		return nil, false
	}
	return org, true
}

// requireOrgArtifactMetadataRead resolves {org} and hides it (404) from
// callers without an active membership, matching how org-internal
// resources are concealed.
func (s *Server) requireOrgArtifactMetadataRead(w http.ResponseWriter, r *http.Request) (*Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	user := ghUserFromContext(r.Context())
	if user == nil || (!user.SiteAdmin && !isActiveOrgMember(s.store, user, org.Login)) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return org, true
}

func artifactStorageRecordJSON(rec *ArtifactStorageRecord) map[string]interface{} {
	var artifactURL, repository interface{}
	if rec.ArtifactURL != "" {
		artifactURL = rec.ArtifactURL
	}
	if rec.Repository != "" {
		repository = rec.Repository
	}
	return map[string]interface{}{
		"id":           rec.ID,
		"name":         rec.Name,
		"digest":       rec.Digest,
		"artifact_url": artifactURL,
		"registry_url": rec.RegistryURL,
		"repository":   repository,
		"status":       rec.Status,
		"created_at":   rec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   rec.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// artifactDeploymentRecordJSON renders the artifact-deployment-record
// shape. attestation_id links the deployment to the org's provenance
// attestation for the same digest when one exists.
func (s *Server) artifactDeploymentRecordJSON(rec *ArtifactDeploymentRecord, orgLogin string) map[string]interface{} {
	tags := rec.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	risks := rec.RuntimeRisks
	if risks == nil {
		risks = []string{}
	}
	var attestationID interface{}
	for _, a := range s.store.ListAttestations(s.store.RepoIDsOwnedBy(orgLogin), rec.Digest, "provenance") {
		attestationID = a.ID
		break
	}
	return map[string]interface{}{
		"id":                   rec.ID,
		"digest":               rec.Digest,
		"logical_environment":  rec.LogicalEnvironment,
		"physical_environment": rec.PhysicalEnvironment,
		"cluster":              rec.Cluster,
		"deployment_name":      rec.DeploymentName,
		"tags":                 tags,
		"runtime_risks":        risks,
		"created_at":           rec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":           rec.UpdatedAt.UTC().Format(time.RFC3339),
		"attestation_id":       attestationID,
	}
}

func validArtifactRuntimeRisks(risks []string) bool {
	if len(risks) > 4 {
		return false
	}
	for _, risk := range risks {
		switch risk {
		case "critical-resource", "internet-exposed", "lateral-movement", "sensitive-data":
		default:
			return false
		}
	}
	return true
}

func (s *Server) handleOrgCreateArtifactStorageRecord(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgArtifactMetadataWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Name             string  `json:"name"`
		Digest           string  `json:"digest"`
		Version          string  `json:"version"`
		ArtifactURL      string  `json:"artifact_url"`
		Path             string  `json:"path"`
		RegistryURL      string  `json:"registry_url"`
		Repository       string  `json:"repository"`
		Status           *string `json:"status"`
		GitHubRepository string  `json:"github_repository"`
		ReturnRecords    *bool   `json:"return_records"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "ArtifactStorageRecord", "name", "missing_field")
		return
	}
	if !artifactDigestPattern.MatchString(req.Digest) {
		writeGHValidationError(w, "ArtifactStorageRecord", "digest", "invalid")
		return
	}
	if req.RegistryURL == "" {
		writeGHValidationError(w, "ArtifactStorageRecord", "registry_url", "missing_field")
		return
	}
	status := "active"
	if req.Status != nil {
		switch *req.Status {
		case "active", "eol", "deleted":
			status = *req.Status
		default:
			writeGHValidationError(w, "ArtifactStorageRecord", "status", "invalid")
			return
		}
	}
	rec := s.store.CreateArtifactStorageRecord(&ArtifactStorageRecord{
		OrgID:            org.ID,
		Name:             req.Name,
		Digest:           req.Digest,
		Version:          req.Version,
		ArtifactURL:      req.ArtifactURL,
		Path:             req.Path,
		RegistryURL:      req.RegistryURL,
		Repository:       req.Repository,
		Status:           status,
		GitHubRepository: req.GitHubRepository,
	})
	out := map[string]interface{}{"total_count": 1}
	if req.ReturnRecords == nil || *req.ReturnRecords {
		out["storage_records"] = []map[string]interface{}{artifactStorageRecordJSON(rec)}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOrgCreateArtifactDeploymentRecord(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgArtifactMetadataWrite(w, r)
	if !ok {
		return
	}
	var req struct {
		Name                string            `json:"name"`
		Digest              string            `json:"digest"`
		Version             string            `json:"version"`
		Status              string            `json:"status"`
		LogicalEnvironment  string            `json:"logical_environment"`
		PhysicalEnvironment string            `json:"physical_environment"`
		Cluster             string            `json:"cluster"`
		DeploymentName      string            `json:"deployment_name"`
		Tags                map[string]string `json:"tags"`
		RuntimeRisks        []string          `json:"runtime_risks"`
		GitHubRepository    string            `json:"github_repository"`
		ReturnRecords       *bool             `json:"return_records"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "name", "missing_field")
		return
	}
	if !artifactDigestPattern.MatchString(req.Digest) {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "digest", "invalid")
		return
	}
	if req.Status != "deployed" && req.Status != "decommissioned" {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "status", "invalid")
		return
	}
	if req.LogicalEnvironment == "" {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "logical_environment", "missing_field")
		return
	}
	if req.DeploymentName == "" {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "deployment_name", "missing_field")
		return
	}
	if len(req.Tags) > 5 {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "tags", "invalid")
		return
	}
	if !validArtifactRuntimeRisks(req.RuntimeRisks) {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "runtime_risks", "invalid")
		return
	}
	rec := s.store.UpsertArtifactDeploymentRecord(&ArtifactDeploymentRecord{
		OrgID:               org.ID,
		Name:                req.Name,
		Digest:              req.Digest,
		Version:             req.Version,
		Status:              req.Status,
		LogicalEnvironment:  req.LogicalEnvironment,
		PhysicalEnvironment: req.PhysicalEnvironment,
		Cluster:             req.Cluster,
		DeploymentName:      req.DeploymentName,
		Tags:                req.Tags,
		RuntimeRisks:        req.RuntimeRisks,
		GitHubRepository:    req.GitHubRepository,
	})
	out := map[string]interface{}{"total_count": 1}
	if req.ReturnRecords == nil || *req.ReturnRecords {
		out["deployment_records"] = []map[string]interface{}{s.artifactDeploymentRecordJSON(rec, org.Login)}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOrgSetClusterDeploymentRecords(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgArtifactMetadataWrite(w, r)
	if !ok {
		return
	}
	cluster := r.PathValue("cluster")
	var req struct {
		LogicalEnvironment  string `json:"logical_environment"`
		PhysicalEnvironment string `json:"physical_environment"`
		Deployments         []struct {
			Name             string            `json:"name"`
			Digest           string            `json:"digest"`
			Version          string            `json:"version"`
			Status           string            `json:"status"`
			DeploymentName   string            `json:"deployment_name"`
			GitHubRepository string            `json:"github_repository"`
			Tags             map[string]string `json:"tags"`
			RuntimeRisks     []string          `json:"runtime_risks"`
		} `json:"deployments"`
		ReturnRecords *bool `json:"return_records"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.LogicalEnvironment == "" {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "logical_environment", "missing_field")
		return
	}
	if len(req.Deployments) == 0 || len(req.Deployments) > 1000 {
		writeGHValidationError(w, "ArtifactDeploymentRecord", "deployments", "invalid")
		return
	}
	seenNames := map[string]bool{}
	for _, d := range req.Deployments {
		if d.Name == "" || d.DeploymentName == "" {
			writeGHValidationError(w, "ArtifactDeploymentRecord", "deployments", "missing_field")
			return
		}
		if !artifactDigestPattern.MatchString(d.Digest) {
			writeGHValidationError(w, "ArtifactDeploymentRecord", "digest", "invalid")
			return
		}
		key := strings.ToLower(d.DeploymentName)
		if seenNames[key] {
			// deployment_name must be unique across the deployments array.
			writeGHValidationError(w, "ArtifactDeploymentRecord", "deployment_name", "already_exists")
			return
		}
		seenNames[key] = true
		if d.Status != "" && d.Status != "deployed" && d.Status != "decommissioned" {
			writeGHValidationError(w, "ArtifactDeploymentRecord", "status", "invalid")
			return
		}
		if !validArtifactRuntimeRisks(d.RuntimeRisks) {
			writeGHValidationError(w, "ArtifactDeploymentRecord", "runtime_risks", "invalid")
			return
		}
	}

	records := make([]map[string]interface{}, 0, len(req.Deployments))
	for _, d := range req.Deployments {
		status := d.Status
		if status == "" {
			status = "deployed"
		}
		rec := s.store.UpsertArtifactDeploymentRecord(&ArtifactDeploymentRecord{
			OrgID:               org.ID,
			Name:                d.Name,
			Digest:              d.Digest,
			Version:             d.Version,
			Status:              status,
			LogicalEnvironment:  req.LogicalEnvironment,
			PhysicalEnvironment: req.PhysicalEnvironment,
			Cluster:             cluster,
			DeploymentName:      d.DeploymentName,
			Tags:                d.Tags,
			RuntimeRisks:        d.RuntimeRisks,
			GitHubRepository:    d.GitHubRepository,
		})
		records = append(records, s.artifactDeploymentRecordJSON(rec, org.Login))
	}
	out := map[string]interface{}{"total_count": len(records)}
	if req.ReturnRecords == nil || *req.ReturnRecords {
		out["deployment_records"] = records
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOrgListArtifactStorageRecords(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgArtifactMetadataRead(w, r)
	if !ok {
		return
	}
	records := s.store.ListArtifactStorageRecords(org.ID, r.PathValue("subject_digest"))
	out := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		out = append(out, artifactStorageRecordJSON(rec))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":     len(out),
		"storage_records": out,
	})
}

func (s *Server) handleOrgListArtifactDeploymentRecords(w http.ResponseWriter, r *http.Request) {
	org, ok := s.requireOrgArtifactMetadataRead(w, r)
	if !ok {
		return
	}
	records := s.store.ListArtifactDeploymentRecords(org.ID, r.PathValue("subject_digest"))
	out := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		out = append(out, s.artifactDeploymentRecordJSON(rec, org.Login))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":        len(out),
		"deployment_records": out,
	})
}
