package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DependabotAlert is a repo-scoped Dependabot security alert.
type DependabotAlert struct {
	ID                     int        `json:"id"`
	NodeID                 string     `json:"node_id"`
	Number                 int        `json:"number"`
	RepoKey                string     `json:"repo_key"`
	PackageName            string     `json:"package_name"`
	PackageEcosystem       string     `json:"package_ecosystem"`
	ManifestPath           string     `json:"manifest_path"`
	VulnerabilityID        string     `json:"vulnerability_id"` // GHSA id
	CVEID                  string     `json:"cve_id"`
	Severity               string     `json:"severity"`
	State                  string     `json:"state"`
	DismissedReason        string     `json:"dismissed_reason"`
	DismissedComment       string     `json:"dismissed_comment"`
	DismissedByLogin       string     `json:"dismissed_by_login"`
	DismissedAt            *time.Time `json:"dismissed_at"`
	FixedAt                *time.Time `json:"fixed_at"`
	AutoDismissedAt        *time.Time `json:"auto_dismissed_at"`
	Summary                string     `json:"summary"`
	Description            string     `json:"description"`
	VulnerableVersionRange string     `json:"vulnerable_version_range"`
	FirstPatchedVersion    string     `json:"first_patched_version"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// DependabotSecret is a repository-level Dependabot secret. Value stores the
// libsodium sealed-box ciphertext uploaded by the client; bleephub never needs
// to decrypt it for the REST API.
type DependabotSecret struct {
	Name      string    `json:"name"`
	Value     string    `json:"value"` // encrypted (base64 sealed box)
	KeyID     string    `json:"key_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DependabotOrgSecret is an organization-level Dependabot secret with the
// org-only visibility scoping (all|private|selected).
type DependabotOrgSecret struct {
	DependabotSecret
	Visibility      string `json:"visibility"`
	SelectedRepoIDs []int  `json:"selected_repository_ids,omitempty"`
}

func (st *Store) CreateDependabotAlertIfNew(repoKey, pkgName, ecosystem, manifest, vulnID, cveID, severity, summary, description, vulnRange, patched string) *DependabotAlert {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, alert := range st.DependabotAlertsByRepo[repoKey] {
		if strings.EqualFold(alert.PackageName, pkgName) &&
			strings.EqualFold(alert.PackageEcosystem, ecosystem) &&
			alert.ManifestPath == manifest &&
			alert.VulnerabilityID == vulnID {
			return alert
		}
	}
	return st.createDependabotAlertLocked(repoKey, pkgName, ecosystem, manifest, vulnID, cveID, severity, "open", summary, description, vulnRange, patched)
}

func (st *Store) createDependabotAlertLocked(repoKey, pkgName, ecosystem, manifest, vulnID, cveID, severity, state, summary, description, vulnRange, patched string) *DependabotAlert {
	if st.DependabotAlertsByRepo[repoKey] == nil {
		st.DependabotAlertsByRepo[repoKey] = make(map[int]*DependabotAlert)
	}
	if st.DependabotNextNumber[repoKey] == 0 {
		st.DependabotNextNumber[repoKey] = 1
	}

	now := time.Now().UTC()
	if state == "" {
		state = "open"
	}

	number := st.DependabotNextNumber[repoKey]
	st.DependabotNextNumber[repoKey] = number + 1

	a := &DependabotAlert{
		ID:                     st.NextDependabotAlertID,
		NodeID:                 fmt.Sprintf("DPA_%d", st.NextDependabotAlertID),
		Number:                 number,
		RepoKey:                repoKey,
		PackageName:            pkgName,
		PackageEcosystem:       ecosystem,
		ManifestPath:           manifest,
		VulnerabilityID:        vulnID,
		CVEID:                  cveID,
		Severity:               severity,
		State:                  state,
		Summary:                summary,
		Description:            description,
		VulnerableVersionRange: vulnRange,
		FirstPatchedVersion:    patched,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	st.NextDependabotAlertID++

	st.DependabotAlerts[a.ID] = a
	st.DependabotAlertsByRepo[repoKey][number] = a
	st.persistDependabotAlert(a)
	return a
}

// GetDependabotAlert returns an alert by repo + alert number.
func (st *Store) GetDependabotAlert(repoKey string, number int) *DependabotAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.DependabotAlertsByRepo[repoKey][number]
}

// ListDependabotAlerts returns repo alerts filtered/sorted per GitHub's list
// endpoint.
func (st *Store) ListDependabotAlerts(repoKey, state, severity, packageName, ecosystem, manifest, sortField, direction string) []*DependabotAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()

	byRepo := st.DependabotAlertsByRepo[repoKey]
	out := make([]*DependabotAlert, 0, len(byRepo))
	for _, a := range byRepo {
		if state != "" && a.State != state {
			continue
		}
		if severity != "" && a.Severity != severity {
			continue
		}
		if packageName != "" && !strings.EqualFold(a.PackageName, packageName) {
			continue
		}
		if ecosystem != "" && !strings.EqualFold(a.PackageEcosystem, ecosystem) {
			continue
		}
		if manifest != "" && a.ManifestPath != manifest {
			continue
		}
		out = append(out, a)
	}

	if sortField == "" {
		sortField = "created"
	}
	if direction == "" {
		direction = "desc"
	}

	sort.SliceStable(out, func(i, j int) bool {
		var less bool
		switch sortField {
		case "updated":
			less = out[i].UpdatedAt.Before(out[j].UpdatedAt)
		default:
			less = out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		if direction == "asc" {
			return less
		}
		return !less
	})
	return out
}

// UpdateDependabotAlert applies a state/dismissed_reason transition to a single
// alert. Valid transitions mirror real GitHub: open → dismissed, dismissed → open.
func (st *Store) UpdateDependabotAlert(a *DependabotAlert, state, dismissedReason, dismissedComment string, dismissedBy *User) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if err := validateDependabotTransition(a.State, state, dismissedReason); err != nil {
		return err
	}

	now := time.Now().UTC()
	switch state {
	case "dismissed":
		a.State = "dismissed"
		a.DismissedReason = dismissedReason
		a.DismissedComment = dismissedComment
		a.DismissedAt = &now
		if dismissedBy != nil {
			a.DismissedByLogin = dismissedBy.Login
		}
		a.FixedAt = nil
	case "open":
		a.State = "open"
		a.DismissedReason = ""
		a.DismissedComment = ""
		a.DismissedAt = nil
		a.DismissedByLogin = ""
		a.FixedAt = nil
	}
	a.UpdatedAt = now
	st.persistDependabotAlert(a)
	return nil
}

func validateDependabotTransition(currentState, newState, dismissedReason string) error {
	if newState != "" && newState != "open" && newState != "dismissed" {
		return fmt.Errorf("invalid state %q", newState)
	}
	if newState == "dismissed" && !isValidDependabotDismissedReason(dismissedReason) {
		return fmt.Errorf("invalid dismissed_reason %q", dismissedReason)
	}
	if newState == currentState {
		return nil
	}
	if newState == "dismissed" && currentState == "open" {
		return nil
	}
	if newState == "open" && currentState == "dismissed" {
		return nil
	}
	return fmt.Errorf("invalid transition from %q to %q", currentState, newState)
}

func isValidDependabotDismissedReason(r string) bool {
	switch r {
	case "fix_started", "inaccurate", "no_bandwidth", "not_used", "tolerable_risk":
		return true
	}
	return false
}

func (st *Store) persistDependabotAlert(a *DependabotAlert) {
	if st.persist != nil {
		st.persist.MustPut("dependabot_alerts", strconv.Itoa(a.ID), a)
	}
}

// UpsertDependabotSecret creates or updates a repository-level Dependabot secret.
func (st *Store) UpsertDependabotSecret(repoKey, name, value, keyID string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	m := st.DependabotSecrets[repoKey]
	if m == nil {
		m = make(map[string]*DependabotSecret)
		st.DependabotSecrets[repoKey] = m
	}
	existing := m[name]
	if existing != nil {
		existing.Value = value
		existing.KeyID = keyID
		existing.UpdatedAt = now
	} else {
		m[name] = &DependabotSecret{Name: name, Value: value, KeyID: keyID, CreatedAt: now, UpdatedAt: now}
	}
	if st.persist != nil {
		st.persist.MustPut("dependabot_secrets", repoKey, m)
	}
	return existing == nil
}

// DeleteDependabotSecret removes a repository-level Dependabot secret.
func (st *Store) DeleteDependabotSecret(repoKey, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	m, ok := st.DependabotSecrets[repoKey]
	if !ok || m[name] == nil {
		return false
	}
	delete(m, name)
	if st.persist != nil {
		if len(m) > 0 {
			st.persist.MustPut("dependabot_secrets", repoKey, m)
		} else {
			st.persist.MustDelete("dependabot_secrets", repoKey)
		}
	}
	return true
}

// UpsertDependabotOrgSecret creates or updates an organization-level Dependabot secret.
func (st *Store) UpsertDependabotOrgSecret(orgLogin, name, value, keyID, visibility string, selectedRepoIDs []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	m := st.DependabotOrgSecrets[orgLogin]
	if m == nil {
		m = make(map[string]*DependabotOrgSecret)
		st.DependabotOrgSecrets[orgLogin] = m
	}
	if visibility != "selected" {
		selectedRepoIDs = nil
	}
	existing := m[name]
	if existing != nil {
		existing.Value = value
		existing.KeyID = keyID
		existing.Visibility = visibility
		existing.SelectedRepoIDs = selectedRepoIDs
		existing.UpdatedAt = now
	} else {
		m[name] = &DependabotOrgSecret{
			DependabotSecret: DependabotSecret{Name: name, Value: value, KeyID: keyID, CreatedAt: now, UpdatedAt: now},
			Visibility:       visibility,
			SelectedRepoIDs:  selectedRepoIDs,
		}
	}
	if st.persist != nil {
		st.persist.MustPut("dependabot_org_secrets", orgLogin, m)
	}
	return existing == nil
}

// DeleteDependabotOrgSecret removes an organization-level Dependabot secret.
func (st *Store) DeleteDependabotOrgSecret(orgLogin, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	m, ok := st.DependabotOrgSecrets[orgLogin]
	if !ok || m[name] == nil {
		return false
	}
	delete(m, name)
	if st.persist != nil {
		if len(m) > 0 {
			st.persist.MustPut("dependabot_org_secrets", orgLogin, m)
		} else {
			st.persist.MustDelete("dependabot_org_secrets", orgLogin)
		}
	}
	return true
}

// SetDependabotOrgSecretSelectedRepos replaces the selected repository IDs for
// an org secret.
func (st *Store) SetDependabotOrgSecretSelectedRepos(orgLogin, name string, ids []int) (*DependabotOrgSecret, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	m := st.DependabotOrgSecrets[orgLogin]
	if m == nil || m[name] == nil {
		return nil, false
	}
	sec := m[name]
	sec.SelectedRepoIDs = append([]int(nil), ids...)
	sec.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("dependabot_org_secrets", orgLogin, m)
	}
	return sec, true
}

// --- user secrets ---

// DependabotUserSecret is a user-level Dependabot secret.
type DependabotUserSecret struct {
	DependabotSecret
}

// UpsertDependabotUserSecret creates or updates a user-level Dependabot secret.
func (st *Store) UpsertDependabotUserSecret(userLogin, name, value, keyID string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	m := st.DependabotUserSecrets[userLogin]
	if m == nil {
		m = make(map[string]*DependabotUserSecret)
		st.DependabotUserSecrets[userLogin] = m
	}
	existing := m[name]
	if existing != nil {
		existing.Value = value
		existing.KeyID = keyID
		existing.UpdatedAt = now
	} else {
		m[name] = &DependabotUserSecret{DependabotSecret{Name: name, Value: value, KeyID: keyID, CreatedAt: now, UpdatedAt: now}}
	}
	if st.persist != nil {
		st.persist.MustPut("dependabot_user_secrets", userLogin, m)
	}
	return existing == nil
}

// DeleteDependabotUserSecret removes a user-level Dependabot secret.
func (st *Store) DeleteDependabotUserSecret(userLogin, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	m, ok := st.DependabotUserSecrets[userLogin]
	if !ok || m[name] == nil {
		return false
	}
	delete(m, name)
	if st.persist != nil {
		if len(m) > 0 {
			st.persist.MustPut("dependabot_user_secrets", userLogin, m)
		} else {
			st.persist.MustDelete("dependabot_user_secrets", userLogin)
		}
	}
	return true
}

// GetDependabotUserSecret returns a user-level Dependabot secret by name.
func (st *Store) GetDependabotUserSecret(userLogin, name string) *DependabotUserSecret {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.DependabotUserSecrets[userLogin][name]
}

// ListDependabotUserSecrets returns all user-level Dependabot secrets sorted by name.
func (st *Store) ListDependabotUserSecrets(userLogin string) []*DependabotUserSecret {
	st.mu.RLock()
	defer st.mu.RUnlock()

	m := st.DependabotUserSecrets[userLogin]
	out := make([]*DependabotUserSecret, 0, len(m))
	for _, sec := range m {
		out = append(out, sec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// --- org repository access ---

// SetDependabotRepositoryAccess replaces the repository access list for an org.
// Returns true when the list did not previously exist.
func (st *Store) SetDependabotRepositoryAccess(orgLogin string, repoIDs []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	_, existed := st.DependabotRepositoryAccess[orgLogin]
	st.DependabotRepositoryAccess[orgLogin] = append([]int(nil), repoIDs...)
	if st.persist != nil {
		if len(repoIDs) > 0 {
			st.persist.MustPut("dependabot_repo_access", orgLogin, st.DependabotRepositoryAccess[orgLogin])
		} else {
			st.persist.MustDelete("dependabot_repo_access", orgLogin)
		}
	}
	return !existed
}

// GetDependabotRepositoryAccess returns the repository access list for an org.
func (st *Store) GetDependabotRepositoryAccess(orgLogin string) []int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return append([]int(nil), st.DependabotRepositoryAccess[orgLogin]...)
}

// --- org alerts ---

// ListDependabotAlertsByOrg returns all Dependabot alerts for repositories owned
// by the given organization, sorted by creation time descending.
func (st *Store) ListDependabotAlertsByOrg(orgID int) []*DependabotAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []*DependabotAlert
	for repoKey, byNumber := range st.DependabotAlertsByRepo {
		repo := st.ReposByName[repoKey]
		if repo == nil || repo.OwnerType != "Organization" || repo.OwnerID != orgID {
			continue
		}
		for _, a := range byNumber {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}
