package bleephub

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// App represents a registered GitHub App.
type App struct {
	ID            int               `json:"id"`
	NodeID        string            `json:"node_id"`
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	ClientID      string            `json:"client_id"`
	ClientSecret  string            `json:"-"`
	Description   string            `json:"description"`
	ExternalURL   string            `json:"external_url"`
	WebhookURL    string            `json:"-"`
	WebhookSecret string            `json:"-"`
	WebhookActive bool              `json:"-"`
	WebhookEvents []string          `json:"-"`
	PEMPrivateKey string            `json:"-"`
	Permissions   map[string]string `json:"permissions"`
	Events        []string          `json:"events"`
	OwnerID       int               `json:"owner_id"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Installation represents an app installation on a user or org.
type Installation struct {
	ID                  int               `json:"id"`
	AppID               int               `json:"app_id"`
	AppSlug             string            `json:"app_slug"`
	TargetType          string            `json:"target_type"`
	TargetID            int               `json:"target_id"`
	TargetLogin         string            `json:"target_login"`
	Permissions         map[string]string `json:"permissions"`
	Events              []string          `json:"events"`
	RepositorySelection string            `json:"repository_selection"`
	SelectedRepoIDs     []int             `json:"-"`
	SuspendedAt         *time.Time        `json:"suspended_at"`
	SuspendedBy         *User             `json:"suspended_by"`
	SingleFileName      string            `json:"single_file_name"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// InstallationToken is a short-lived token scoped to an installation.
type InstallationToken struct {
	Token          string            `json:"token"`
	ExpiresAt      time.Time         `json:"expires_at"`
	Permissions    map[string]string `json:"permissions"`
	RepositoryIDs  []int             `json:"-"`
	InstallationID int               `json:"installation_id"`
	AppID          int               `json:"app_id"`
}

// OAuthApp is the OAuth-app entity Basic-authenticated by client_id+client_secret.
// Distinct from GitHub Apps (App above) although a GitHub App also has a client_id+secret pair
// that can be used the same way for OAuth user-to-server flows.
type OAuthApp struct {
	ClientID     string
	ClientSecret string
	Name         string
	Description  string
	URL          string
	CallbackURL  string
	OwnerID      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreateApp generates a new GitHub App with an RSA key pair.
func (st *Store) CreateApp(ownerID int, name, description string, perms map[string]string, events []string) *App {
	st.mu.Lock()
	defer st.mu.Unlock()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("rsa.GenerateKey: " + err.Error())
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	id := st.NextAppID
	st.NextAppID++
	now := time.Now()
	slug := slugify(name)

	secretBytes := make([]byte, 20)
	_, _ = rand.Read(secretBytes)
	wsBytes := make([]byte, 20)
	_, _ = rand.Read(wsBytes)

	app := &App{
		ID:            id,
		NodeID:        fmt.Sprintf("A_kgDO%08d", id),
		Slug:          slug,
		Name:          name,
		ClientID:      fmt.Sprintf("Iv1.%016x", id),
		ClientSecret:  hex.EncodeToString(secretBytes),
		Description:   description,
		ExternalURL:   fmt.Sprintf("https://github.com/apps/%s", slug),
		WebhookSecret: hex.EncodeToString(wsBytes),
		WebhookActive: true,
		PEMPrivateKey: string(privPEM),
		Permissions:   perms,
		Events:        events,
		OwnerID:       ownerID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	st.Apps[id] = app
	st.AppsBySlug[slug] = app
	if st.AppsByClientID == nil {
		st.AppsByClientID = make(map[string]*App)
	}
	st.AppsByClientID[app.ClientID] = app
	return app
}

// UpdateAppHookConfig mutates the app's hook URL/secret/active flags.
func (st *Store) UpdateAppHookConfig(appID int, fn func(a *App)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	app := st.Apps[appID]
	if app == nil {
		return false
	}
	fn(app)
	app.UpdatedAt = time.Now()
	return true
}

// GetApp returns an app by ID, or nil.
func (st *Store) GetApp(id int) *App {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Apps[id]
}

// GetAppBySlug returns an app by slug, or nil.
func (st *Store) GetAppBySlug(slug string) *App {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.AppsBySlug[slug]
}

// CreateInstallation creates a new installation for an app.
func (st *Store) CreateInstallation(appID int, targetType string, targetID int, targetLogin string, perms map[string]string, events []string) *Installation {
	st.mu.Lock()
	defer st.mu.Unlock()

	app := st.Apps[appID]
	if app == nil {
		return nil
	}

	id := st.NextInstallationID
	st.NextInstallationID++
	now := time.Now()

	inst := &Installation{
		ID:                  id,
		AppID:               appID,
		AppSlug:             app.Slug,
		TargetType:          targetType,
		TargetID:            targetID,
		TargetLogin:         targetLogin,
		Permissions:         perms,
		Events:              events,
		RepositorySelection: "all",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	st.Installations[id] = inst
	return inst
}

// GetInstallation returns an installation by ID, or nil.
func (st *Store) GetInstallation(id int) *Installation {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Installations[id]
}

// ListAppInstallations returns all installations for a given app.
func (st *Store) ListAppInstallations(appID int) []*Installation {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var result []*Installation
	for _, inst := range st.Installations {
		if inst.AppID == appID {
			result = append(result, inst)
		}
	}
	return result
}

// GetRepoInstallation finds an installation by target login.
func (st *Store) GetRepoInstallation(ownerLogin string) *Installation {
	st.mu.RLock()
	defer st.mu.RUnlock()

	for _, inst := range st.Installations {
		if inst.TargetLogin == ownerLogin {
			return inst
		}
	}
	return nil
}

// DeleteInstallation removes an installation by ID.
func (st *Store) DeleteInstallation(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if _, ok := st.Installations[id]; !ok {
		return false
	}
	delete(st.Installations, id)
	return true
}

// SuspendInstallation marks the installation suspended. Returns false if not found
// or already suspended.
func (st *Store) SuspendInstallation(id int, by *User) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	inst := st.Installations[id]
	if inst == nil {
		return false
	}
	if inst.SuspendedAt != nil {
		return false
	}
	now := time.Now()
	inst.SuspendedAt = &now
	inst.SuspendedBy = by
	inst.UpdatedAt = now
	return true
}

// UnsuspendInstallation clears the suspension. Returns false if not found
// or wasn't suspended.
func (st *Store) UnsuspendInstallation(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	inst := st.Installations[id]
	if inst == nil {
		return false
	}
	if inst.SuspendedAt == nil {
		return false
	}
	inst.SuspendedAt = nil
	inst.SuspendedBy = nil
	inst.UpdatedAt = time.Now()
	return true
}

// SetInstallationRepositorySelection switches between "all" and "selected" modes.
func (st *Store) SetInstallationRepositorySelection(id int, mode string, repoIDs []int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	inst := st.Installations[id]
	if inst == nil {
		return false
	}
	inst.RepositorySelection = mode
	if mode == "selected" {
		inst.SelectedRepoIDs = append([]int(nil), repoIDs...)
	} else {
		inst.SelectedRepoIDs = nil
	}
	inst.UpdatedAt = time.Now()
	return true
}

// AddInstallationRepo adds a repo to a "selected" installation's allow-list.
// Returns (added, ok) — ok=false if installation not found; added=false if
// repo was already in the list (idempotent).
func (st *Store) AddInstallationRepo(id, repoID int) (bool, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	inst := st.Installations[id]
	if inst == nil {
		return false, false
	}
	for _, r := range inst.SelectedRepoIDs {
		if r == repoID {
			return false, true
		}
	}
	inst.SelectedRepoIDs = append(inst.SelectedRepoIDs, repoID)
	if inst.RepositorySelection != "selected" {
		inst.RepositorySelection = "selected"
	}
	inst.UpdatedAt = time.Now()
	return true, true
}

// RemoveInstallationRepo removes a repo from a "selected" installation's allow-list.
// Returns (removed, ok).
func (st *Store) RemoveInstallationRepo(id, repoID int) (bool, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	inst := st.Installations[id]
	if inst == nil {
		return false, false
	}
	for i, r := range inst.SelectedRepoIDs {
		if r == repoID {
			inst.SelectedRepoIDs = append(inst.SelectedRepoIDs[:i], inst.SelectedRepoIDs[i+1:]...)
			inst.UpdatedAt = time.Now()
			return true, true
		}
	}
	return false, true
}

// GetAppByClientID returns the GitHub App with the given client_id, or nil.
func (st *Store) GetAppByClientID(clientID string) *App {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.AppsByClientID[clientID]
}

// CreateOAuthApp registers a new (classic) OAuth App. Distinct from a GitHub App:
// no JWT, no installations, no permissions table — just client_id/secret + callback URL.
// Both kinds of apps support the OAuth web flow, but the resulting access tokens
// have different prefixes (gho_ for OAuth Apps, ghu_ for GitHub App user-to-server).
func (st *Store) CreateOAuthApp(ownerID int, name, description, url, callbackURL string) *OAuthApp {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OAuthApps == nil {
		st.OAuthApps = make(map[string]*OAuthApp)
	}
	cidBytes := make([]byte, 10)
	_, _ = rand.Read(cidBytes)
	secretBytes := make([]byte, 20)
	_, _ = rand.Read(secretBytes)
	clientID := hex.EncodeToString(cidBytes)
	now := time.Now()
	app := &OAuthApp{
		ClientID:     clientID,
		ClientSecret: hex.EncodeToString(secretBytes),
		Name:         name,
		Description:  description,
		URL:          url,
		CallbackURL:  callbackURL,
		OwnerID:      ownerID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.OAuthApps[clientID] = app
	return app
}

// GetOAuthApp returns the OAuth App with the given client_id, or nil.
func (st *Store) GetOAuthApp(clientID string) *OAuthApp {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.OAuthApps[clientID]
}

// ListOAuthApps returns all OAuth Apps.
func (st *Store) ListOAuthApps() []*OAuthApp {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*OAuthApp, 0, len(st.OAuthApps))
	for _, a := range st.OAuthApps {
		out = append(out, a)
	}
	return out
}

// VerifyOAuthAppSecret returns the OAuth App if client_id+client_secret match, else nil.
func (st *Store) VerifyOAuthAppSecret(clientID, clientSecret string) *OAuthApp {
	st.mu.RLock()
	defer st.mu.RUnlock()
	app := st.OAuthApps[clientID]
	if app == nil || app.ClientSecret != clientSecret {
		return nil
	}
	return app
}

// VerifyAppClientSecret returns the GitHub App if client_id+client_secret match, else nil.
func (st *Store) VerifyAppClientSecret(clientID, clientSecret string) *App {
	st.mu.RLock()
	defer st.mu.RUnlock()
	app := st.AppsByClientID[clientID]
	if app == nil || app.ClientSecret != clientSecret {
		return nil
	}
	return app
}

// CreateInstallationToken generates a ghs_-prefixed token with 1h expiry.
// If repoIDs is non-empty, the token is scoped to those repositories
// (a subset of the installation's accessible repos).
func (st *Store) CreateInstallationToken(installationID, appID int, perms map[string]string, repoIDs []int) *InstallationToken {
	st.mu.Lock()
	defer st.mu.Unlock()

	b := make([]byte, 20)
	_, _ = rand.Read(b)
	tokenStr := "ghs_" + hex.EncodeToString(b)

	token := &InstallationToken{
		Token:          tokenStr,
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		Permissions:    perms,
		RepositoryIDs:  append([]int(nil), repoIDs...),
		InstallationID: installationID,
		AppID:          appID,
	}
	st.InstallationTokens[tokenStr] = token
	return token
}

// RevokeInstallationToken drops the token from the store. Returns
// true if the token existed (so the caller can return 204) and false
// if it didn't (so the caller can return 401 for unknown tokens).
func (st *Store) RevokeInstallationToken(tokenStr string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.InstallationTokens[tokenStr]; !ok {
		return false
	}
	delete(st.InstallationTokens, tokenStr)
	return true
}

// LookupInstallationToken returns the token and its installation, or nil if not found/expired.
func (st *Store) LookupInstallationToken(tokenStr string) (*InstallationToken, *Installation) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	tok, ok := st.InstallationTokens[tokenStr]
	if !ok {
		return nil, nil
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, nil
	}
	inst := st.Installations[tok.InstallationID]
	return tok, inst
}

// RegisterManifestCode creates a one-time-use code that maps to an app ID.
func (st *Store) RegisterManifestCode(appID int) string {
	st.mu.Lock()
	defer st.mu.Unlock()

	code := uuid.New().String()
	st.ManifestCodes[code] = appID
	return code
}

// ConsumeManifestCode redeems a manifest code, returning the app ID. One-time use.
func (st *Store) ConsumeManifestCode(code string) (int, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	appID, ok := st.ManifestCodes[code]
	if !ok {
		return 0, false
	}
	delete(st.ManifestCodes, code)
	return appID, true
}

// slugify is defined in store_orgs.go
