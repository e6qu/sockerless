package bleephub

import (
	"regexp"
	"sort"
	"strconv"
	"time"
)

// Organization artifact metadata: storage records (where an artifact
// lives) and deployment records (where it runs), keyed by the
// artifact's subject digest.

// ArtifactStorageRecord records where an artifact identified by digest
// is stored.
type ArtifactStorageRecord struct {
	ID               int       `json:"id"`
	OrgID            int       `json:"org_id"`
	Name             string    `json:"name"`
	Digest           string    `json:"digest"`
	Version          string    `json:"version"`
	ArtifactURL      string    `json:"artifact_url"`
	Path             string    `json:"path"`
	RegistryURL      string    `json:"registry_url"`
	Repository       string    `json:"repository"`
	Status           string    `json:"status"` // active, eol, deleted
	GitHubRepository string    `json:"github_repository"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ArtifactDeploymentRecord records one deployment of an artifact. A
// deployment is identified by (logical environment, physical
// environment, cluster, deployment name); repeated posts for the same
// identity update the record in place.
type ArtifactDeploymentRecord struct {
	ID                  int               `json:"id"`
	OrgID               int               `json:"org_id"`
	Name                string            `json:"name"`
	Digest              string            `json:"digest"`
	Version             string            `json:"version"`
	Status              string            `json:"status"` // deployed, decommissioned
	LogicalEnvironment  string            `json:"logical_environment"`
	PhysicalEnvironment string            `json:"physical_environment"`
	Cluster             string            `json:"cluster"`
	DeploymentName      string            `json:"deployment_name"`
	Tags                map[string]string `json:"tags"`
	RuntimeRisks        []string          `json:"runtime_risks"`
	GitHubRepository    string            `json:"github_repository"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

var artifactDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// CreateArtifactStorageRecord appends a storage record for the org.
func (st *Store) CreateArtifactStorageRecord(rec *ArtifactStorageRecord) *ArtifactStorageRecord {
	st.mu.Lock()
	defer st.mu.Unlock()
	rec.ID = st.NextArtifactStorageRecordID
	st.NextArtifactStorageRecordID++
	now := time.Now().UTC()
	rec.CreatedAt = now
	rec.UpdatedAt = now
	st.ArtifactStorageRecords[rec.ID] = rec
	if st.persist != nil {
		st.persist.MustPut("artifact_storage_records", strconv.Itoa(rec.ID), rec)
	}
	return rec
}

// ListArtifactStorageRecords returns the org's storage records for a
// digest (any digest when empty), ascending by ID.
func (st *Store) ListArtifactStorageRecords(orgID int, digest string) []*ArtifactStorageRecord {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*ArtifactStorageRecord, 0)
	for _, rec := range st.ArtifactStorageRecords {
		if rec.OrgID != orgID {
			continue
		}
		if digest != "" && rec.Digest != digest {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// UpsertArtifactDeploymentRecord creates or updates the deployment
// record identified by (org, logical env, physical env, cluster,
// deployment name).
func (st *Store) UpsertArtifactDeploymentRecord(rec *ArtifactDeploymentRecord) *ArtifactDeploymentRecord {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().UTC()
	for _, existing := range st.ArtifactDeploymentRecords {
		if existing.OrgID == rec.OrgID &&
			existing.LogicalEnvironment == rec.LogicalEnvironment &&
			existing.PhysicalEnvironment == rec.PhysicalEnvironment &&
			existing.Cluster == rec.Cluster &&
			existing.DeploymentName == rec.DeploymentName {
			existing.Name = rec.Name
			existing.Digest = rec.Digest
			existing.Version = rec.Version
			existing.Status = rec.Status
			existing.Tags = rec.Tags
			existing.RuntimeRisks = rec.RuntimeRisks
			existing.GitHubRepository = rec.GitHubRepository
			existing.UpdatedAt = now
			if st.persist != nil {
				st.persist.MustPut("artifact_deployment_records", strconv.Itoa(existing.ID), existing)
			}
			return existing
		}
	}
	rec.ID = st.NextArtifactDeploymentRecordID
	st.NextArtifactDeploymentRecordID++
	rec.CreatedAt = now
	rec.UpdatedAt = now
	st.ArtifactDeploymentRecords[rec.ID] = rec
	if st.persist != nil {
		st.persist.MustPut("artifact_deployment_records", strconv.Itoa(rec.ID), rec)
	}
	return rec
}

// ListArtifactDeploymentRecords returns the org's deployment records
// for a digest (any digest when empty), ascending by ID.
func (st *Store) ListArtifactDeploymentRecords(orgID int, digest string) []*ArtifactDeploymentRecord {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*ArtifactDeploymentRecord, 0)
	for _, rec := range st.ArtifactDeploymentRecords {
		if rec.OrgID != orgID {
			continue
		}
		if digest != "" && rec.Digest != digest {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
