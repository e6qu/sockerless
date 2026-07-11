package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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

	return st.createSecretScanningAlertLocked(repoKey, secretType, locations)
}

func (st *Store) createSecretScanningAlertLocked(repoKey, secretType string, locations []SecretScanningLocation) *SecretScanningAlert {
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

// CreateSecretScanningAlertIfNew records a content-derived alert unless the
// same repository already has the same secret type at the same blob location.
func (st *Store) CreateSecretScanningAlertIfNew(repoKey, secretType string, locations []SecretScanningLocation) *SecretScanningAlert {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, existing := range st.SecretScanningAlertsByRepo[repoKey] {
		if existing.SecretType != secretType || len(existing.Locations) != len(locations) {
			continue
		}
		same := true
		for i := range existing.Locations {
			if !sameSecretScanningLocation(existing.Locations[i], locations[i]) {
				same = false
				break
			}
		}
		if same {
			return existing
		}
	}
	return st.createSecretScanningAlertLocked(repoKey, secretType, locations)
}

func sameSecretScanningLocation(a, b SecretScanningLocation) bool {
	return a.Type == b.Type &&
		a.Details.Path == b.Details.Path &&
		a.Details.StartLine == b.Details.StartLine &&
		a.Details.EndLine == b.Details.EndLine &&
		a.Details.StartColumn == b.Details.StartColumn &&
		a.Details.EndColumn == b.Details.EndColumn &&
		a.Details.BlobSHA == b.Details.BlobSHA
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

// secretScanningProviderPatterns is the catalog of partner patterns the
// pattern-configurations surface exposes and validates against.
var secretScanningProviderPatterns = []struct {
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

func isSecretScanningProviderPattern(tokenType string) bool {
	for _, p := range secretScanningProviderPatterns {
		if p.patternID == tokenType {
			return true
		}
	}
	return false
}

// OrgSecretScanningPatternConfig holds an organization's push-protection
// pattern settings and the optimistic-concurrency row version updates must
// present.
type OrgSecretScanningPatternConfig struct {
	Version          string            `json:"version"`
	ProviderSettings map[string]string `json:"provider_settings"` // token_type → not-set | disabled | enabled
	CustomSettings   map[string]string `json:"custom_settings"`   // token_type → disabled | enabled
	UpdatedAt        time.Time         `json:"updated_at"`
}

// ListSecretScanningPatternConfigurations returns the secret scanning
// pattern overrides exposed by GitHub's pattern-configurations endpoint for
// the org, reflecting any stored push-protection settings and computing the
// alert totals from the org's real alerts.
func (st *Store) ListSecretScanningPatternConfigurations(orgLogin string) map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()

	cfg := st.SecretScanningPatternConfigs[orgLogin]
	org := st.OrgsByLogin[orgLogin]

	alertTotals := map[string]int{}
	falsePositives := map[string]int{}
	orgAlertTotal := 0
	if org != nil {
		for repoKey, byNumber := range st.SecretScanningAlertsByRepo {
			repo := st.ReposByName[repoKey]
			if repo == nil || repo.OwnerType != "Organization" || repo.OwnerID != org.ID {
				continue
			}
			for _, a := range byNumber {
				alertTotals[a.SecretType]++
				orgAlertTotal++
				if a.Resolution == "false_positive" {
					falsePositives[a.SecretType]++
				}
			}
		}
	}

	overrides := make([]map[string]interface{}, 0, len(secretScanningProviderPatterns))
	for _, p := range secretScanningProviderPatterns {
		setting := "not-set"
		if cfg != nil && cfg.ProviderSettings[p.patternID] != "" {
			setting = cfg.ProviderSettings[p.patternID]
		}
		total := alertTotals[p.slug] + alertTotals[p.patternID]
		fps := falsePositives[p.slug] + falsePositives[p.patternID]
		totalPct := 0.0
		if orgAlertTotal > 0 {
			totalPct = float64(total) / float64(orgAlertTotal) * 100
		}
		fpRate := 0.0
		if total > 0 {
			fpRate = float64(fps) / float64(total)
		}
		overrides = append(overrides, map[string]interface{}{
			"token_type":             p.patternID,
			"custom_pattern_version": nil,
			"slug":                   p.slug,
			"display_name":           p.displayName,
			"alert_total":            total,
			"alert_total_percentage": totalPct,
			"false_positives":        fps,
			"false_positive_rate":    fpRate,
			"bypass_rate":            0,
			"default_setting":        "enabled",
			"enterprise_setting":     nil,
			"setting":                setting,
		})
	}
	var version interface{}
	if cfg != nil {
		version = cfg.Version
	}
	return map[string]interface{}{
		"pattern_config_version":     version,
		"provider_pattern_overrides": overrides,
		"custom_pattern_overrides":   []map[string]interface{}{},
	}
}

// UpdateSecretScanningPatternConfig applies push-protection setting changes
// for the org. expectedVersion, when non-nil, must match the current row
// version — a mismatch reports a conflict without changing anything.
// Returns the new version.
func (st *Store) UpdateSecretScanningPatternConfig(orgLogin string, expectedVersion *string, provider, custom map[string]string) (string, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	cfg := st.SecretScanningPatternConfigs[orgLogin]
	current := ""
	if cfg != nil {
		current = cfg.Version
	}
	if expectedVersion != nil && *expectedVersion != current {
		return "", false
	}
	if cfg == nil {
		cfg = &OrgSecretScanningPatternConfig{
			ProviderSettings: map[string]string{},
			CustomSettings:   map[string]string{},
		}
		st.SecretScanningPatternConfigs[orgLogin] = cfg
	}
	for tokenType, setting := range provider {
		if setting == "not-set" {
			delete(cfg.ProviderSettings, tokenType)
			continue
		}
		cfg.ProviderSettings[tokenType] = setting
	}
	for tokenType, setting := range custom {
		cfg.CustomSettings[tokenType] = setting
	}
	cfg.Version = uuid.New().String()
	cfg.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("secret_scanning_pattern_configs", orgLogin, cfg)
	}
	return cfg.Version, true
}

// SecretScanningPushProtectionEnabled reports whether an organization has
// explicitly enabled push protection for a provider pattern on this repository.
func (st *Store) SecretScanningPushProtectionEnabled(repo *Repo, patternID string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if repo == nil || repo.OwnerType != "Organization" {
		return false
	}
	org := st.Orgs[repo.OwnerID]
	if org == nil {
		return false
	}
	cfg := st.SecretScanningPatternConfigs[org.Login]
	return cfg != nil && cfg.ProviderSettings[patternID] == "enabled"
}

// SecretScanningPushProtectionPlaceholder is one blocked-push placeholder:
// the identity a pusher presents when requesting a push protection bypass.
type SecretScanningPushProtectionPlaceholder struct {
	ID        string    `json:"id"`
	RepoKey   string    `json:"repo_key"`
	TokenType string    `json:"token_type"`
	CreatedAt time.Time `json:"created_at"`
}

// SecretScanningPushProtectionBypass is a granted push protection bypass.
type SecretScanningPushProtectionBypass struct {
	PlaceholderID string    `json:"placeholder_id"`
	RepoKey       string    `json:"repo_key"`
	Reason        string    `json:"reason"`
	TokenType     string    `json:"token_type"`
	ExpireAt      time.Time `json:"expire_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateSecretScanningPushProtectionPlaceholder records a blocked push's
// placeholder for the repository.
func (st *Store) CreateSecretScanningPushProtectionPlaceholder(repoKey, tokenType string) *SecretScanningPushProtectionPlaceholder {
	st.mu.Lock()
	defer st.mu.Unlock()

	ph := &SecretScanningPushProtectionPlaceholder{
		ID:        uuid.New().String(),
		RepoKey:   repoKey,
		TokenType: tokenType,
		CreatedAt: time.Now().UTC(),
	}
	if st.SecretScanningPushPlaceholders[repoKey] == nil {
		st.SecretScanningPushPlaceholders[repoKey] = map[string]*SecretScanningPushProtectionPlaceholder{}
	}
	st.SecretScanningPushPlaceholders[repoKey][ph.ID] = ph
	if st.persist != nil {
		st.persist.MustPut("secret_scanning_push_placeholders", repoKey, st.SecretScanningPushPlaceholders[repoKey])
	}
	return ph
}

// secretScanningPushProtectionBypassTTL is how long a granted bypass stays
// valid for the pusher to complete the push.
const secretScanningPushProtectionBypassTTL = 2 * time.Hour

// CreateSecretScanningPushProtectionBypass consumes a placeholder and grants
// the bypass. Returns nil when the placeholder does not exist for the repo.
func (st *Store) CreateSecretScanningPushProtectionBypass(repoKey, placeholderID, reason string) *SecretScanningPushProtectionBypass {
	st.mu.Lock()
	defer st.mu.Unlock()

	ph := st.SecretScanningPushPlaceholders[repoKey][placeholderID]
	if ph == nil {
		return nil
	}
	delete(st.SecretScanningPushPlaceholders[repoKey], placeholderID)
	now := time.Now().UTC()
	bypass := &SecretScanningPushProtectionBypass{
		PlaceholderID: placeholderID,
		RepoKey:       repoKey,
		Reason:        reason,
		TokenType:     ph.TokenType,
		ExpireAt:      now.Add(secretScanningPushProtectionBypassTTL),
		CreatedAt:     now,
	}
	st.SecretScanningPushBypasses[repoKey] = append(st.SecretScanningPushBypasses[repoKey], bypass)
	if st.persist != nil {
		st.persist.MustPut("secret_scanning_push_placeholders", repoKey, st.SecretScanningPushPlaceholders[repoKey])
		st.persist.MustPut("secret_scanning_push_bypasses", repoKey, st.SecretScanningPushBypasses[repoKey])
	}
	return bypass
}

// HasActiveSecretScanningPushProtectionBypass reports whether a previously
// granted bypass still permits a protected write for this token type.
func (st *Store) HasActiveSecretScanningPushProtectionBypass(repoKey, tokenType string, now time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	bypasses := st.SecretScanningPushBypasses[repoKey]
	if len(bypasses) == 0 {
		return false
	}
	active := bypasses[:0]
	found := false
	for _, bypass := range bypasses {
		if bypass == nil || !bypass.ExpireAt.After(now) {
			continue
		}
		active = append(active, bypass)
		if bypass.TokenType == tokenType {
			found = true
		}
	}
	if len(active) != len(bypasses) {
		if len(active) == 0 {
			delete(st.SecretScanningPushBypasses, repoKey)
		} else {
			st.SecretScanningPushBypasses[repoKey] = active
		}
		if st.persist != nil {
			st.persist.MustPut("secret_scanning_push_bypasses", repoKey, st.SecretScanningPushBypasses[repoKey])
		}
	}
	return found
}

// SecretScanningScanHistory derives the repository's scan history from the
// recorded alert state: each alert-producing scan event appears as a
// completed incremental scan, the earliest as the initial backfill, and an
// org-level pattern configuration update as a pattern-update scan. A
// repository with no recorded scanning activity has an honestly empty
// history.
func (st *Store) SecretScanningScanHistory(repo *Repo) (incremental, patternUpdate, backfill []*SecretScanningScanRecord) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var alertTimes []time.Time
	seen := map[time.Time]bool{}
	for _, a := range st.SecretScanningAlertsByRepo[repo.FullName] {
		t := a.CreatedAt.UTC().Truncate(time.Second)
		if !seen[t] {
			seen[t] = true
			alertTimes = append(alertTimes, t)
		}
	}
	sort.Slice(alertTimes, func(i, j int) bool { return alertTimes[i].Before(alertTimes[j]) })

	incremental = []*SecretScanningScanRecord{}
	backfill = []*SecretScanningScanRecord{}
	patternUpdate = []*SecretScanningScanRecord{}
	for i, t := range alertTimes {
		rec := &SecretScanningScanRecord{Type: "incremental", Status: "completed", StartedAt: t, CompletedAt: t}
		if i == 0 {
			backfill = append(backfill, &SecretScanningScanRecord{Type: "backfill", Status: "completed", StartedAt: t, CompletedAt: t})
		}
		incremental = append(incremental, rec)
	}

	if repo.OwnerType == "Organization" {
		ownerLogin, _, _ := strings.Cut(repo.FullName, "/")
		if cfg := st.SecretScanningPatternConfigs[ownerLogin]; cfg != nil {
			t := cfg.UpdatedAt.UTC().Truncate(time.Second)
			patternUpdate = append(patternUpdate, &SecretScanningScanRecord{Type: "pattern_update", Status: "completed", StartedAt: t, CompletedAt: t})
		}
	}
	return incremental, patternUpdate, backfill
}

// SecretScanningScanRecord is one scan in the repository's scan history.
type SecretScanningScanRecord struct {
	Type        string
	Status      string
	StartedAt   time.Time
	CompletedAt time.Time
}
