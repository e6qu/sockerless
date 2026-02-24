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
	Description   string            `json:"description"`
	ExternalURL   string            `json:"external_url"`
	WebhookSecret string            `json:"webhook_secret"`
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
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// InstallationToken is a short-lived token scoped to an installation.
type InstallationToken struct {
	Token          string            `json:"token"`
	ExpiresAt      time.Time         `json:"expires_at"`
	Permissions    map[string]string `json:"permissions"`
	InstallationID int               `json:"installation_id"`
	AppID          int               `json:"app_id"`
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

	app := &App{
		ID:            id,
		NodeID:        fmt.Sprintf("A_kgDO%08d", id),
		Slug:          slug,
		Name:          name,
		ClientID:      fmt.Sprintf("Iv1.%016x", id),
		Description:   description,
		ExternalURL:   fmt.Sprintf("https://github.com/apps/%s", slug),
		PEMPrivateKey: string(privPEM),
		Permissions:   perms,
		Events:        events,
		OwnerID:       ownerID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	st.Apps[id] = app
	st.AppsBySlug[slug] = app
	return app
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

// CreateInstallationToken generates a ghs_-prefixed token with 1h expiry.
func (st *Store) CreateInstallationToken(installationID, appID int, perms map[string]string) *InstallationToken {
	st.mu.Lock()
	defer st.mu.Unlock()

	b := make([]byte, 20)
	_, _ = rand.Read(b)
	tokenStr := "ghs_" + hex.EncodeToString(b)

	token := &InstallationToken{
		Token:          tokenStr,
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		Permissions:    perms,
		InstallationID: installationID,
		AppID:          appID,
	}
	st.InstallationTokens[tokenStr] = token
	return token
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
