package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// SecretScanningLocation describes where a secret was detected.
type SecretScanningLocation struct {
	Type    string                        `json:"type"`
	Details SecretScanningLocationDetails `json:"details"`
}

// SecretScanningLocationDetails holds the commit-level details for a location.
type SecretScanningLocationDetails struct {
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	StartColumn int    `json:"start_column"`
	EndColumn   int    `json:"end_column"`
	BlobSHA     string `json:"blob_sha"`
	BlobURL     string `json:"blob_url"`
	CommitSHA   string `json:"commit_sha"`
	CommitURL   string `json:"commit_url"`
	HTMLURL     string `json:"html_url"`
}

// SecretScanningAlert is a repo-scoped secret scanning alert.
type SecretScanningAlert struct {
	ID                    int                      `json:"id"`
	NodeID                string                   `json:"node_id"`
	Number                int                      `json:"number"`
	RepoKey               string                   `json:"repo_key"`
	SecretType            string                   `json:"secret_type"`
	SecretTypeDisplayName string                   `json:"secret_type_display_name"`
	State                 string                   `json:"state"`
	Resolution            string                   `json:"resolution"`
	ResolutionComment     string                   `json:"resolution_comment"`
	Locations             []SecretScanningLocation `json:"locations"`
	HTMLURL               string                   `json:"html_url"`
	URL                   string                   `json:"url"`
	LocationsURL          string                   `json:"locations_url"`
	CreatedAt             time.Time                `json:"created_at"`
	UpdatedAt             time.Time                `json:"updated_at"`
	ResolvedAt            *time.Time               `json:"resolved_at"`
}

// CreateSecretScanningAlert seeds a new secret scanning alert for a repo.
// The real API has no create endpoint; this is the internal bleephub seeding path.
func (st *Store) CreateSecretScanningAlert(repoKey, secretType string, locations []SecretScanningLocation) *SecretScanningAlert {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.SecretScanningAlertsByRepo[repoKey] == nil {
		st.SecretScanningAlertsByRepo[repoKey] = make(map[int]*SecretScanningAlert)
	}
	if st.SecretScanningNextNumber[repoKey] == 0 {
		st.SecretScanningNextNumber[repoKey] = 1
	}

	now := time.Now().UTC()
	number := st.SecretScanningNextNumber[repoKey]
	st.SecretScanningNextNumber[repoKey] = number + 1

	a := &SecretScanningAlert{
		ID:                    st.NextSecretScanningAlertID,
		NodeID:                fmt.Sprintf("SSA_%d", st.NextSecretScanningAlertID),
		Number:                number,
		RepoKey:               repoKey,
		SecretType:            secretType,
		SecretTypeDisplayName: secretTypeDisplayName(secretType),
		State:                 "open",
		Locations:             locations,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	st.NextSecretScanningAlertID++

	st.SecretScanningAlerts[a.ID] = a
	st.SecretScanningAlertsByRepo[repoKey][number] = a
	st.persistSecretScanningAlert(a)
	return a
}

func secretTypeDisplayName(secretType string) string {
	switch secretType {
	case "github_personal_access_token":
		return "GitHub Personal Access Token"
	case "aws_access_key_id":
		return "AWS Access Key ID"
	case "google_api_key":
		return "Google API Key"
	case "slack_incoming_webhook_url":
		return "Slack Incoming Webhook URL"
	default:
		return secretType
	}
}

// GetSecretScanningAlert returns an alert by repo + alert number.
func (st *Store) GetSecretScanningAlert(repoKey string, number int) *SecretScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.SecretScanningAlertsByRepo[repoKey][number]
}

// ListSecretScanningAlerts returns repo alerts filtered/sorted per GitHub's list endpoint.
func (st *Store) ListSecretScanningAlerts(repoKey, state, secretType, resolution, sortField, direction string) []*SecretScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()

	byRepo := st.SecretScanningAlertsByRepo[repoKey]
	out := make([]*SecretScanningAlert, 0, len(byRepo))
	for _, a := range byRepo {
		if state != "" && a.State != state {
			continue
		}
		if secretType != "" && a.SecretType != secretType {
			continue
		}
		if resolution != "" && a.Resolution != resolution {
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

// UpdateSecretScanningAlert applies a state/resolution transition to a single alert.
func (st *Store) UpdateSecretScanningAlert(a *SecretScanningAlert, state, resolution, resolutionComment string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if err := validateSecretScanningTransition(a.State, state, resolution); err != nil {
		return err
	}

	now := time.Now().UTC()
	if state != "" {
		a.State = state
	}
	if state == "resolved" {
		a.Resolution = resolution
		a.ResolutionComment = resolutionComment
		a.ResolvedAt = &now
	} else if state == "open" {
		a.Resolution = ""
		a.ResolutionComment = ""
		a.ResolvedAt = nil
	}
	a.UpdatedAt = now
	st.persistSecretScanningAlert(a)
	return nil
}

// BulkUpdateSecretScanningAlerts updates every alert matching the repo filters to the given resolution.
func (st *Store) BulkUpdateSecretScanningAlerts(repoKey, stateFilter, secretTypeFilter, resolutionFilter, newResolution, resolutionComment string) ([]*SecretScanningAlert, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	byRepo := st.SecretScanningAlertsByRepo[repoKey]
	now := time.Now().UTC()
	var updated []*SecretScanningAlert
	for _, a := range byRepo {
		if stateFilter != "" && a.State != stateFilter {
			continue
		}
		if secretTypeFilter != "" && a.SecretType != secretTypeFilter {
			continue
		}
		if resolutionFilter != "" && a.Resolution != resolutionFilter {
			continue
		}
		if err := validateSecretScanningTransition(a.State, "resolved", newResolution); err != nil {
			return nil, err
		}
		a.State = "resolved"
		a.Resolution = newResolution
		a.ResolutionComment = resolutionComment
		a.ResolvedAt = &now
		a.UpdatedAt = now
		updated = append(updated, a)
		st.persistSecretScanningAlert(a)
	}
	sort.SliceStable(updated, func(i, j int) bool { return updated[i].Number < updated[j].Number })
	return updated, nil
}

func validateSecretScanningTransition(currentState, newState, resolution string) error {
	if newState != "" && newState != "open" && newState != "resolved" {
		return fmt.Errorf("invalid state %q", newState)
	}
	if newState == "resolved" {
		if !isValidResolution(resolution) {
			return fmt.Errorf("invalid resolution %q", resolution)
		}
	}
	if newState == "open" && currentState == "resolved" {
		return nil
	}
	return nil
}

func isValidResolution(r string) bool {
	switch r {
	case "false_positive", "wont_fix", "revoked", "used_in_tests", "pattern_deleted", "pattern_edited":
		return true
	}
	return false
}

func (st *Store) persistSecretScanningAlert(a *SecretScanningAlert) {
	if st.persist != nil {
		st.persist.MustPut("secret_scanning_alerts", strconv.Itoa(a.ID), a)
	}
}

// ListSecretScanningAlertsByOrg returns all secret scanning alerts for
// repositories owned by the given organization, sorted by creation time descending.
func (st *Store) ListSecretScanningAlertsByOrg(orgID int) []*SecretScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []*SecretScanningAlert
	for repoKey, byNumber := range st.SecretScanningAlertsByRepo {
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

// ListSecretScanningAlertsByUser returns all secret scanning alerts for
// repositories owned by the given user, sorted by creation time descending.
func (st *Store) ListSecretScanningAlertsByUser(userID int) []*SecretScanningAlert {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var out []*SecretScanningAlert
	for repoKey, byNumber := range st.SecretScanningAlertsByRepo {
		repo := st.ReposByName[repoKey]
		if repo == nil || repo.OwnerID != userID {
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

// ListSecretScanningPatternConfigurations returns the default secret scanning
// pattern overrides exposed by GitHub's pattern-configurations endpoint. The
// shape is an object grouping partner patterns under provider_pattern_overrides.
func (st *Store) ListSecretScanningPatternConfigurations() map[string]interface{} {
	patterns := []struct {
		patternID   string
		slug        string
		displayName string
	}{
		{"ghp", "github_personal_access_token", "GitHub Personal Access Token"},
		{"gho", "github_oauth_access_token", "GitHub OAuth Access Token"},
		{"ghu", "github_user_to_server_token", "GitHub User-to-Server Token"},
		{"ghs", "github_server_to_server_token", "GitHub Server-to-Server Token"},
		{"ghr", "github_refresh_token", "GitHub Refresh Token"},
		{"aws", "aws_access_key_id", "AWS Access Key ID"},
		{"google", "google_api_key", "Google API Key"},
		{"slack", "slack_incoming_webhook_url", "Slack Incoming Webhook URL"},
	}
	overrides := make([]map[string]interface{}, 0, len(patterns))
	for _, p := range patterns {
		overrides = append(overrides, map[string]interface{}{
			"token_type":             p.patternID,
			"custom_pattern_version": nil,
			"slug":                   p.slug,
			"display_name":           p.displayName,
			"alert_total":            0,
			"alert_total_percentage": 0,
			"false_positives":        0,
			"false_positive_rate":    0,
			"bypass_rate":            0,
			"default_setting":        "enabled",
			"enterprise_setting":     nil,
			"setting":                "not-set",
		})
	}
	return map[string]interface{}{
		"pattern_config_version":     nil,
		"provider_pattern_overrides": overrides,
		"custom_pattern_overrides":   []map[string]interface{}{},
	}
}
