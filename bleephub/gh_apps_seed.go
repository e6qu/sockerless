package bleephub

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"time"
)

// AppSeedSpec describes one pre-registered GitHub App to create at startup,
// so a coordinate-only consumer holds the same (app id + private key + org)
// coordinates against bleephub that it would against real GitHub — where an
// App is registered out of band and the consumer keeps its id + PEM as
// secrets. Supplied via BLEEPHUB_SEED_APPS (inline JSON array) or
// BLEEPHUB_SEED_APPS_FILE (path to a JSON array).
type AppSeedSpec struct {
	ID             int                    `json:"id"`                   // deterministic, caller-chosen app id (required)
	Slug           string                 `json:"slug"`                 // defaults to slugify(name)
	Name           string                 `json:"name"`                 // required
	ClientID       string                 `json:"client_id"`            // defaults to Iv1.<id>
	PrivateKeyPEM  string                 `json:"private_key_pem"`      // caller-supplied RSA key (PKCS1 or PKCS8)
	PrivateKeyFile string                 `json:"private_key_pem_file"` // alternative to inline PEM
	Owner          string                 `json:"owner"`                // owning user login (required)
	Permissions    map[string]string      `json:"permissions"`
	Events         []string               `json:"events"`
	WebhookURL     string                 `json:"webhook_url"`
	WebhookSecret  string                 `json:"webhook_secret"`
	Installations  []InstallationSeedSpec `json:"installations"`
}

// InstallationSeedSpec pre-creates an installation of the seeded App on a
// given account, so the consumer can mint an installation token by
// coordinates alone (no /internal use).
type InstallationSeedSpec struct {
	ID          int               `json:"id"`          // deterministic installation id (optional)
	Account     string            `json:"account"`     // org/user login to install on (required)
	TargetType  string            `json:"target_type"` // "Organization" | "User"; default Organization
	Permissions map[string]string `json:"permissions"`
	Events      []string          `json:"events"`
}

// loadAppSeedSpecs reads seed specs from BLEEPHUB_SEED_APPS_FILE (a JSON file)
// and BLEEPHUB_SEED_APPS (inline JSON), concatenating both when present.
func loadAppSeedSpecs() ([]AppSeedSpec, error) {
	var specs []AppSeedSpec
	if path := os.Getenv("BLEEPHUB_SEED_APPS_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("BLEEPHUB_SEED_APPS_FILE: %w", err)
		}
		var fileSpecs []AppSeedSpec
		if err := json.Unmarshal(b, &fileSpecs); err != nil {
			return nil, fmt.Errorf("BLEEPHUB_SEED_APPS_FILE: invalid JSON: %w", err)
		}
		specs = append(specs, fileSpecs...)
	}
	if inline := os.Getenv("BLEEPHUB_SEED_APPS"); inline != "" {
		var inlineSpecs []AppSeedSpec
		if err := json.Unmarshal([]byte(inline), &inlineSpecs); err != nil {
			return nil, fmt.Errorf("BLEEPHUB_SEED_APPS: invalid JSON: %w", err)
		}
		specs = append(specs, inlineSpecs...)
	}
	return specs, nil
}

// seedConfiguredApps registers the apps described by the seed config. It is
// idempotent across restarts (an app already present — loaded from
// persistence — is left unchanged) and fails loud on a malformed spec, never
// silently degrading.
func (s *Server) seedConfiguredApps() error {
	specs, err := loadAppSeedSpecs()
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if spec.Name == "" {
			return fmt.Errorf("seed app: name is required")
		}
		if spec.ID <= 0 {
			return fmt.Errorf("seed app %q: id must be a positive integer", spec.Name)
		}
		pemKey := spec.PrivateKeyPEM
		if pemKey == "" && spec.PrivateKeyFile != "" {
			b, err := os.ReadFile(spec.PrivateKeyFile)
			if err != nil {
				return fmt.Errorf("seed app %q: read private_key_pem_file: %w", spec.Name, err)
			}
			pemKey = string(b)
		}
		if pemKey == "" {
			return fmt.Errorf("seed app %q: private_key_pem or private_key_pem_file is required", spec.Name)
		}
		if spec.Owner == "" {
			return fmt.Errorf("seed app %q: owner is required", spec.Name)
		}
		if s.store.LookupUserByLogin(spec.Owner) == nil {
			return fmt.Errorf("seed app %q: owner %q is not an existing user", spec.Name, spec.Owner)
		}

		app, created, err := s.store.SeedApp(spec, pemKey, spec.Owner)
		if err != nil {
			return fmt.Errorf("seed app %q: %w", spec.Name, err)
		}
		if !created {
			s.logger.Info().Int("app_id", app.ID).Str("slug", app.Slug).
				Msg("seed GitHub App already present; left unchanged")
			continue
		}
		s.logger.Info().Int("app_id", app.ID).Str("slug", app.Slug).
			Int("installations", len(spec.Installations)).Msg("seeded pre-registered GitHub App")

		for _, ins := range spec.Installations {
			if ins.Account == "" {
				return fmt.Errorf("seed app %q: installation account is required", spec.Name)
			}
			targetType, targetID, err := s.resolveSeedInstallTarget(ins)
			if err != nil {
				return fmt.Errorf("seed app %q: %w", spec.Name, err)
			}
			inst := s.store.SeedInstallation(app.ID, ins.ID, targetType, targetID, ins.Account, ins.Permissions, ins.Events)
			if inst == nil {
				return fmt.Errorf("seed app %q: failed to create installation on %q", spec.Name, ins.Account)
			}
			s.emitInstallationEvent(app, "created", inst)
			s.logger.Info().Int("app_id", app.ID).Int("installation_id", inst.ID).
				Str("account", ins.Account).Msg("seeded App installation")
		}
	}
	return nil
}

// resolveSeedInstallTarget resolves an installation account login to a real
// target type + id. Seed configuration must name an existing account; startup
// fails instead of inventing an organization or silently installing on id 0.
func (s *Server) resolveSeedInstallTarget(ins InstallationSeedSpec) (string, int, error) {
	if ins.TargetType != "" && ins.TargetType != "Organization" && ins.TargetType != "User" {
		return "", 0, fmt.Errorf("installation account %q: target_type must be Organization or User", ins.Account)
	}
	if org := s.store.GetOrg(ins.Account); org != nil {
		if ins.TargetType == "User" {
			return "", 0, fmt.Errorf("installation account %q is an organization, not a user", ins.Account)
		}
		return "Organization", org.ID, nil
	}
	if u := s.store.LookupUserByLogin(ins.Account); u != nil {
		if ins.TargetType == "Organization" {
			return "", 0, fmt.Errorf("installation account %q is a user, not an organization", ins.Account)
		}
		return "User", u.ID, nil
	}
	want := ins.TargetType
	if want == "" {
		want = "user or organization"
	}
	return "", 0, fmt.Errorf("installation account %q does not resolve to an existing %s", ins.Account, want)
}

// normalizeRSAPrivateKeyPEM validates a caller-supplied RSA private key (PKCS1
// "RSA PRIVATE KEY" or PKCS8 "PRIVATE KEY") and re-encodes it as PKCS1 — the
// form the app-JWT verifier (parseAndVerifyAppJWT) expects. The consumer keeps
// the same key, so app JWTs they sign verify against the stored key.
func normalizeRSAPrivateKeyPEM(pemStr string) (string, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return "", fmt.Errorf("private key is not valid PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("private key is not an RSA key")
		}
		key = rk
	} else {
		return "", fmt.Errorf("cannot parse RSA private key (need a PKCS1 or PKCS8 PEM)")
	}
	out := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return string(out), nil
}

// SeedApp creates a GitHub App from a caller-supplied spec with a deterministic
// id, slug, and private key. Idempotent: if an app with the same id or slug is
// already present (e.g. loaded from persistence), it is returned unchanged with
// created=false. Errors only if the supplied PEM is not a usable RSA key.
func (st *Store) SeedApp(spec AppSeedSpec, pemKey, ownerLogin string) (app *App, created bool, err error) {
	normKey, err := normalizeRSAPrivateKeyPEM(pemKey)
	if err != nil {
		return nil, false, err
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	slug := spec.Slug
	if slug == "" {
		slug = slugify(spec.Name)
	}
	owner := st.UsersByLogin[ownerLogin]
	if owner == nil {
		return nil, false, fmt.Errorf("owner %q is not an existing user", ownerLogin)
	}
	if existing := st.Apps[spec.ID]; existing != nil {
		return existing, false, nil
	}
	if existing := st.AppsBySlug[slug]; existing != nil {
		return existing, false, nil
	}

	clientID := spec.ClientID
	if clientID == "" {
		clientID = fmt.Sprintf("Iv1.%016x", spec.ID)
	}

	clientSecret, err := randomHex(20)
	if err != nil {
		return nil, false, fmt.Errorf("generate seeded GitHub App client secret: %w", err)
	}
	webhookSecret := spec.WebhookSecret
	if webhookSecret == "" {
		webhookSecret, err = randomHex(20)
		if err != nil {
			return nil, false, fmt.Errorf("generate seeded GitHub App webhook secret: %w", err)
		}
	}

	now := time.Now().UTC()
	app = &App{
		ID:                 spec.ID,
		NodeID:             fmt.Sprintf("A_kgDO%08d", spec.ID),
		Slug:               slug,
		Name:               spec.Name,
		ClientID:           clientID,
		ClientSecret:       clientSecret,
		ExternalURL:        fmt.Sprintf("https://github.com/apps/%s", slug),
		WebhookURL:         spec.WebhookURL,
		WebhookSecret:      webhookSecret,
		WebhookActive:      spec.WebhookURL != "",
		WebhookContentType: "form",
		WebhookInsecureSSL: "0",
		PEMPrivateKey:      normKey,
		Permissions:        spec.Permissions,
		Events:             spec.Events,
		OwnerID:            owner.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	st.Apps[app.ID] = app
	st.AppsBySlug[slug] = app
	if st.AppsByClientID == nil {
		st.AppsByClientID = make(map[string]*App)
	}
	st.AppsByClientID[clientID] = app
	if spec.ID >= st.NextAppID {
		st.NextAppID = spec.ID + 1
	}
	if st.persist != nil {
		st.persist.MustPut("apps", strconv.Itoa(app.ID), app)
	}
	return app, true, nil
}

// SeedInstallation creates an installation of a seeded App on a target,
// optionally with a deterministic id. Idempotent per (app, target login): an
// existing installation on the same account is returned unchanged. Returns nil
// if the app doesn't exist.
func (st *Store) SeedInstallation(appID, explicitID int, targetType string, targetID int, targetLogin string, perms map[string]string, events []string) *Installation {
	st.mu.Lock()
	defer st.mu.Unlock()

	app := st.Apps[appID]
	if app == nil {
		return nil
	}
	if targetID <= 0 {
		return nil
	}
	if targetType == "" {
		targetType = "Organization"
	}
	for _, inst := range st.Installations {
		if inst.AppID == appID && inst.TargetLogin == targetLogin {
			return inst
		}
	}

	id := explicitID
	if id <= 0 {
		id = st.NextInstallationID
		st.NextInstallationID++
	} else if id >= st.NextInstallationID {
		st.NextInstallationID = id + 1
	}

	var targetNodeID, targetAvatarURL string
	if u := st.UsersByLogin[targetLogin]; u != nil {
		targetNodeID, targetAvatarURL = u.NodeID, u.AvatarURL
	} else if o := st.OrgsByLogin[targetLogin]; o != nil {
		targetNodeID, targetAvatarURL = o.NodeID, o.AvatarURL
	}

	now := time.Now().UTC()
	inst := &Installation{
		ID:                  id,
		AppID:               appID,
		AppSlug:             app.Slug,
		TargetType:          targetType,
		TargetID:            targetID,
		TargetLogin:         targetLogin,
		TargetNodeID:        targetNodeID,
		TargetAvatarURL:     targetAvatarURL,
		Permissions:         perms,
		Events:              events,
		RepositorySelection: "all",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	st.Installations[id] = inst
	if st.persist != nil {
		st.persist.MustPut("installations", strconv.Itoa(id), inst)
	}
	return inst
}
