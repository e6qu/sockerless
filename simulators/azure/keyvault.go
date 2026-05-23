package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Azure Key Vault — sockerless runner workflows commonly fetch
// secrets via `azure/get-keyvault-secrets`, `Get-AzKeyVaultSecret`
// (PowerShell), `az keyvault secret show` (CLI), or
// `armkeyvault.NewVaultsClient` + `azsecrets.NewClient` (Go SDK).
// Without this slice every credential-bootstrap step 404s.
//
// Real Key Vault has two planes:
//   1. ARM control plane creates/configures the vault resource at
//      `Microsoft.KeyVault/vaults/{name}`.
//   2. Data plane (`https://{vault}.vault.azure.net`) reads/writes
//      secret material via JSON over HTTPS.
//
// The sim mirrors both — control plane lives on the standard ARM
// path; data plane lives at `<vault>.vault.<sim-host>:<port>` and is
// routed by Host header through a WrapHandler middleware so the SDK
// can use the canonical URL pattern with no rewrites.

// KeyVault is a `Microsoft.KeyVault/vaults/{name}` ARM resource.
type KeyVault struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Location   string             `json:"location"`
	Tags       map[string]string  `json:"tags,omitempty"`
	Properties KeyVaultProperties `json:"properties"`
}

// KeyVaultProperties holds the per-vault settings.
type KeyVaultProperties struct {
	TenantID                     string                 `json:"tenantId"`
	Sku                          *KeyVaultSku           `json:"sku,omitempty"`
	AccessPolicies               []KeyVaultAccessPolicy `json:"accessPolicies,omitempty"`
	VaultURI                     string                 `json:"vaultUri,omitempty"`
	EnabledForDeployment         bool                   `json:"enabledForDeployment,omitempty"`
	EnabledForDiskEncryption     bool                   `json:"enabledForDiskEncryption,omitempty"`
	EnabledForTemplateDeployment bool                   `json:"enabledForTemplateDeployment,omitempty"`
	EnableSoftDelete             bool                   `json:"enableSoftDelete,omitempty"`
	EnablePurgeProtection        bool                   `json:"enablePurgeProtection,omitempty"`
	EnableRbacAuthorization      bool                   `json:"enableRbacAuthorization,omitempty"`
	NetworkAcls                  *KeyVaultNetworkAcls   `json:"networkAcls,omitempty"`
	ProvisioningState            string                 `json:"provisioningState,omitempty"`
}

// KeyVaultSku envelope.
type KeyVaultSku struct {
	Family string `json:"family"`
	Name   string `json:"name"`
}

// KeyVaultAccessPolicy entries grant per-principal access — superseded
// by RBAC when `EnableRbacAuthorization=true` but still accepted on
// PUT for legacy callers.
type KeyVaultAccessPolicy struct {
	TenantID    string              `json:"tenantId"`
	ObjectID    string              `json:"objectId"`
	Permissions KeyVaultPermissions `json:"permissions"`
}

// KeyVaultPermissions lists per-policy verbs.
type KeyVaultPermissions struct {
	Keys         []string `json:"keys,omitempty"`
	Secrets      []string `json:"secrets,omitempty"`
	Certificates []string `json:"certificates,omitempty"`
	Storage      []string `json:"storage,omitempty"`
}

// KeyVaultNetworkAcls describes ingress filtering on the vault.
type KeyVaultNetworkAcls struct {
	Bypass              string             `json:"bypass,omitempty"`
	DefaultAction       string             `json:"defaultAction,omitempty"`
	IPRules             []KeyVaultIPRule   `json:"ipRules,omitempty"`
	VirtualNetworkRules []KeyVaultVNetRule `json:"virtualNetworkRules,omitempty"`
}

// KeyVaultIPRule is a per-CIDR allow entry.
type KeyVaultIPRule struct {
	Value string `json:"value"`
}

// KeyVaultVNetRule references a subnet by ID for VNet-scoped access.
type KeyVaultVNetRule struct {
	ID string `json:"id"`
}

// KeyVaultSecret is the data-plane secret resource. Real Azure stores
// per-version material; the sim collapses to the single current
// version (matches the read-most pattern runners use).
type KeyVaultSecret struct {
	Vault       string            `json:"-"`
	Name        string            `json:"-"`
	ID          string            `json:"id"` // Full URL `<vault>/secrets/{name}/<version>`
	Value       string            `json:"value"`
	Attributes  KeyVaultAttrs     `json:"attributes"`
	Tags        map[string]string `json:"tags,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
}

// KeyVaultAttrs mirrors the data-plane SecretAttributes shape.
type KeyVaultAttrs struct {
	Enabled   bool  `json:"enabled"`
	Created   int64 `json:"created,omitempty"`
	Updated   int64 `json:"updated,omitempty"`
	NotBefore int64 `json:"nbf,omitempty"`
	Expires   int64 `json:"exp,omitempty"`
}

var (
	keyVaults    sim.Store[KeyVault]
	keyVaultData sim.Store[KeyVaultSecret] // key: <vault>/<secretName>
)

func registerKeyVault(srv *sim.Server) {
	keyVaults = sim.MakeStore[KeyVault](srv.DB(), "keyvaults")
	keyVaultData = sim.MakeStore[KeyVaultSecret](srv.DB(), "keyvault_secrets")
	keyVaultKeys = sim.MakeStore[KeyVaultKey](srv.DB(), "keyvault_keys")
	keyVaultCertificates = sim.MakeStore[KeyVaultCertificate](srv.DB(), "keyvault_certificates")

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.KeyVault"

	// ARM control plane — vault CRUD.
	srv.HandleFunc("PUT "+armBase+"/vaults/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		var req KeyVault
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent",
				"Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/%s",
			sub, rg, name)
		// vaultUri uses the same subdomain routing as storage so
		// SDK callers reach the data plane through the standard URL.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		hostname := r.Host
		portSuffix := ""
		if i := strings.LastIndex(hostname, ":"); i >= 0 {
			portSuffix = hostname[i:]
			hostname = hostname[:i]
		}
		vaultURI := fmt.Sprintf("%s://%s.vault.%s%s/", scheme, name, hostname, portSuffix)

		if req.Properties.Sku == nil {
			req.Properties.Sku = &KeyVaultSku{Family: "A", Name: "standard"}
		}
		if req.Properties.TenantID == "" {
			req.Properties.TenantID = "00000000-0000-0000-0000-000000000000"
		}
		req.Properties.VaultURI = vaultURI
		req.Properties.ProvisioningState = "Succeeded"

		vault := KeyVault{
			ID:         resourceID,
			Name:       name,
			Type:       "Microsoft.KeyVault/vaults",
			Location:   req.Location,
			Tags:       req.Tags,
			Properties: req.Properties,
		}
		keyVaults.Put(resourceID, vault)
		sim.WriteJSON(w, http.StatusOK, vault)
	})

	srv.HandleFunc("GET "+armBase+"/vaults/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/%s",
			sub, rg, name)
		v, ok := keyVaults.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"Vault %q not found in resource group %q.", name, rg)
			return
		}
		sim.WriteJSON(w, http.StatusOK, v)
	})

	srv.HandleFunc("DELETE "+armBase+"/vaults/{name}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "name")
		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/%s",
			sub, rg, name)
		keyVaults.Delete(resourceID)
		w.WriteHeader(http.StatusOK)
	})

	srv.HandleFunc("GET "+armBase+"/vaults", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/",
			sub, rg)
		all := keyVaults.Filter(func(v KeyVault) bool {
			return strings.HasPrefix(v.ID, prefix)
		})
		if all == nil {
			all = []KeyVault{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"value": all})
	})

	// Data plane — subdomain routing via WrapHandler. Host pattern:
	// `<vault>.vault.<sim-host>:<port>`. Strip the suffix to identify
	// the vault and route to the right handler.
	srv.WrapHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			hostname := host
			if i := strings.LastIndex(hostname, ":"); i >= 0 {
				hostname = hostname[:i]
			}
			// Match "<vault>.vault." prefix — works for both
			// localhost (sim) and vault.azure.net (real cloud) suffixes.
			parts := strings.SplitN(hostname, ".vault.", 2)
			if len(parts) == 2 {
				handleKeyVaultDataPlane(w, r, parts[0])
				return
			}
			next.ServeHTTP(w, r)
		})
	})
}

// handleKeyVaultDataPlane routes requests with `<vault>.vault.*` Host
// to the right secret handler. Path patterns:
//
//	PUT    /secrets/{name}                — SetSecret
//	GET    /secrets/{name}                — GetLatest
//	GET    /secrets/{name}/{version}      — GetSpecific (sim collapses to latest)
//	GET    /secrets                       — ListSecrets
//	DELETE /secrets/{name}                — DeleteSecret
//
// The api-version query param is required by real Azure but ignored
// by the sim.
func handleKeyVaultDataPlane(w http.ResponseWriter, r *http.Request, vault string) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	switch {
	case strings.HasPrefix(path, "secrets/"):
		segs := strings.Split(path, "/")
		// segs: ["secrets", "<name>"] or ["secrets", "<name>", "<version>"]
		if len(segs) < 2 {
			sim.AzureError(w, "BadRequest", "Missing secret name", http.StatusBadRequest)
			return
		}
		name := segs[1]
		switch r.Method {
		case http.MethodPut:
			handleKVSetSecret(w, r, vault, name)
		case http.MethodGet:
			handleKVGetSecret(w, r, vault, name)
		case http.MethodDelete:
			handleKVDeleteSecret(w, r, vault, name)
		default:
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
		}
	case path == "secrets" || path == "secrets/":
		if r.Method != http.MethodGet {
			sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
			return
		}
		handleKVListSecrets(w, r, vault)
	case strings.HasPrefix(path, "keys/") || path == "keys":
		handleKVKey(w, r, vault, path)
	case strings.HasPrefix(path, "certificates/") || path == "certificates":
		handleKVCertificate(w, r, vault, path)
	default:
		sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
			"Key Vault data plane path %q not implemented", path)
	}
}

// KeyVaultKey is a key stored at /keys/{name}. Real KV stores a
// public + private half via Azure-managed HSM; the sim stores
// only the operator-supplied JsonWebKey envelope on Create and
// echoes it on read.
type KeyVaultKey struct {
	Vault      string         `json:"-"`
	Name       string         `json:"-"`
	ID         string         `json:"kid"`
	JsonWebKey map[string]any `json:"key,omitempty"`
	Attributes KeyVaultAttrs  `json:"attributes,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

// KeyVaultCertificate is a certificate at /certificates/{name}.
// Real KV produces a chain (cert + private key + thumbprint); the
// sim stores the operator-supplied content + a deterministic
// thumbprint.
type KeyVaultCertificate struct {
	Vault      string            `json:"-"`
	Name       string            `json:"-"`
	ID         string            `json:"id"`
	X509Thumbprint string        `json:"x5t,omitempty"`
	Policy     map[string]any    `json:"policy,omitempty"`
	Attributes KeyVaultAttrs     `json:"attributes,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

var (
	keyVaultKeys         sim.Store[KeyVaultKey]
	keyVaultCertificates sim.Store[KeyVaultCertificate]
)

// handleKVKey routes /keys/* requests.
//   POST /keys/{name}/create     — generate (stash JsonWebKey on body if present)
//   PUT  /keys/{name}            — import
//   GET  /keys/{name}            — get latest
//   GET  /keys/{name}/{version}  — get specific (sim collapses to latest)
//   GET  /keys                   — list
//   DELETE /keys/{name}          — delete
func handleKVKey(w http.ResponseWriter, r *http.Request, vault, path string) {
	segs := strings.Split(path, "/")
	// segs: ["keys"] or ["keys", "<name>"] or ["keys", "<name>", "<version>"]
	// or ["keys", "<name>", "create"]
	if len(segs) < 2 {
		// GET /keys → list
		if r.Method == http.MethodGet {
			handleKVListKeys(w, r, vault)
			return
		}
		sim.AzureError(w, "BadRequest", "Missing key name", http.StatusBadRequest)
		return
	}
	name := segs[1]
	verb := ""
	if len(segs) >= 3 {
		verb = segs[2]
	}
	switch r.Method {
	case http.MethodPost:
		if verb == "create" {
			handleKVCreateKey(w, r, vault, name)
			return
		}
	case http.MethodPut:
		handleKVImportKey(w, r, vault, name)
		return
	case http.MethodGet:
		handleKVGetKey(w, r, vault, name)
		return
	case http.MethodDelete:
		handleKVDeleteKey(w, r, vault, name)
		return
	}
	sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
}

func keyVaultKeyKey(vault, name string) string { return vault + "/" + name }

func handleKVCreateKey(w http.ResponseWriter, r *http.Request, vault, name string) {
	var body struct {
		Kty        string            `json:"kty"`
		KeySize    int               `json:"key_size,omitempty"`
		Crv        string            `json:"crv,omitempty"`
		Attributes *KeyVaultAttrs    `json:"attributes,omitempty"`
		Tags       map[string]string `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sim.AzureError(w, "InvalidRequest", err.Error(), http.StatusBadRequest)
		return
	}
	version := generateUUID()
	id := buildKVURL(r, vault, "keys", name, version)
	jwk := map[string]any{
		"kid": id,
		"kty": defaultKVKty(body.Kty),
		"n":   "sim-generated-modulus",
		"e":   "AQAB",
	}
	if body.Crv != "" {
		jwk["crv"] = body.Crv
	}
	now := time.Now().Unix()
	key := KeyVaultKey{
		Vault: vault, Name: name, ID: id, JsonWebKey: jwk,
		Attributes: KeyVaultAttrs{Enabled: true, Created: now, Updated: now},
		Tags:       body.Tags,
	}
	if body.Attributes != nil {
		key.Attributes.Enabled = body.Attributes.Enabled
	}
	keyVaultKeys.Put(keyVaultKeyKey(vault, name), key)
	sim.WriteJSON(w, http.StatusOK, key)
}

func handleKVImportKey(w http.ResponseWriter, r *http.Request, vault, name string) {
	var body struct {
		Key        map[string]any    `json:"key"`
		Attributes *KeyVaultAttrs    `json:"attributes,omitempty"`
		Tags       map[string]string `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sim.AzureError(w, "InvalidRequest", err.Error(), http.StatusBadRequest)
		return
	}
	version := generateUUID()
	id := buildKVURL(r, vault, "keys", name, version)
	if body.Key == nil {
		body.Key = map[string]any{}
	}
	body.Key["kid"] = id
	now := time.Now().Unix()
	key := KeyVaultKey{
		Vault: vault, Name: name, ID: id, JsonWebKey: body.Key,
		Attributes: KeyVaultAttrs{Enabled: true, Created: now, Updated: now},
		Tags:       body.Tags,
	}
	if body.Attributes != nil {
		key.Attributes.Enabled = body.Attributes.Enabled
	}
	keyVaultKeys.Put(keyVaultKeyKey(vault, name), key)
	sim.WriteJSON(w, http.StatusOK, key)
}

func handleKVGetKey(w http.ResponseWriter, r *http.Request, vault, name string) {
	k, ok := keyVaultKeys.Get(keyVaultKeyKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "KeyNotFound", http.StatusNotFound,
			"A key with (name/id) %q was not found in this key vault.", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, k)
}

func handleKVListKeys(w http.ResponseWriter, r *http.Request, vault string) {
	var out []KeyVaultKey
	for _, k := range keyVaultKeys.List() {
		if k.Vault == vault {
			out = append(out, k)
		}
	}
	if out == nil {
		out = []KeyVaultKey{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleKVDeleteKey(w http.ResponseWriter, r *http.Request, vault, name string) {
	k, ok := keyVaultKeys.Get(keyVaultKeyKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "KeyNotFound", http.StatusNotFound,
			"A key with (name/id) %q was not found in this key vault.", name)
		return
	}
	keyVaultKeys.Delete(keyVaultKeyKey(vault, name))
	sim.WriteJSON(w, http.StatusOK, k)
}

// handleKVCertificate routes /certificates/* requests.
//   POST /certificates/{name}/create   — issue cert
//   GET  /certificates/{name}          — get
//   GET  /certificates                 — list
//   DELETE /certificates/{name}        — delete
func handleKVCertificate(w http.ResponseWriter, r *http.Request, vault, path string) {
	segs := strings.Split(path, "/")
	if len(segs) < 2 {
		if r.Method == http.MethodGet {
			handleKVListCertificates(w, r, vault)
			return
		}
		sim.AzureError(w, "BadRequest", "Missing certificate name", http.StatusBadRequest)
		return
	}
	name := segs[1]
	verb := ""
	if len(segs) >= 3 {
		verb = segs[2]
	}
	switch r.Method {
	case http.MethodPost:
		if verb == "create" {
			handleKVCreateCertificate(w, r, vault, name)
			return
		}
	case http.MethodGet:
		handleKVGetCertificate(w, r, vault, name)
		return
	case http.MethodDelete:
		handleKVDeleteCertificate(w, r, vault, name)
		return
	}
	sim.AzureError(w, "MethodNotAllowed", "Method not supported", http.StatusMethodNotAllowed)
}

func keyVaultCertKey(vault, name string) string { return vault + "/" + name }

func handleKVCreateCertificate(w http.ResponseWriter, r *http.Request, vault, name string) {
	var body struct {
		Policy     map[string]any    `json:"policy,omitempty"`
		Attributes *KeyVaultAttrs    `json:"attributes,omitempty"`
		Tags       map[string]string `json:"tags,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	version := generateUUID()
	id := buildKVURL(r, vault, "certificates", name, version)
	now := time.Now().Unix()
	thumbprint := strings.ToUpper(generateUUID()[:8])
	c := KeyVaultCertificate{
		Vault: vault, Name: name, ID: id, X509Thumbprint: thumbprint,
		Policy:     body.Policy,
		Attributes: KeyVaultAttrs{Enabled: true, Created: now, Updated: now},
		Tags:       body.Tags,
	}
	if body.Attributes != nil {
		c.Attributes.Enabled = body.Attributes.Enabled
	}
	keyVaultCertificates.Put(keyVaultCertKey(vault, name), c)
	sim.WriteJSON(w, http.StatusOK, c)
}

func handleKVGetCertificate(w http.ResponseWriter, r *http.Request, vault, name string) {
	c, ok := keyVaultCertificates.Get(keyVaultCertKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "CertificateNotFound", http.StatusNotFound,
			"A certificate with (name/id) %q was not found in this key vault.", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, c)
}

func handleKVListCertificates(w http.ResponseWriter, r *http.Request, vault string) {
	var out []KeyVaultCertificate
	for _, c := range keyVaultCertificates.List() {
		if c.Vault == vault {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []KeyVaultCertificate{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleKVDeleteCertificate(w http.ResponseWriter, r *http.Request, vault, name string) {
	c, ok := keyVaultCertificates.Get(keyVaultCertKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "CertificateNotFound", http.StatusNotFound,
			"A certificate with (name/id) %q was not found in this key vault.", name)
		return
	}
	keyVaultCertificates.Delete(keyVaultCertKey(vault, name))
	sim.WriteJSON(w, http.StatusOK, c)
}

func defaultKVKty(s string) string {
	if s == "" {
		return "RSA"
	}
	return s
}

// buildKVURL constructs the canonical KV data-plane resource ID
// (`<scheme>://<vault>.vault.<host>/<kind>/<name>/<version>`). Falls
// back to a relative-URL form when r.URL.Scheme is empty (the sim's
// mux passes a relative URL).
func buildKVURL(r *http.Request, vault, kind, name, version string) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s.vault.%s/%s/%s/%s", scheme, vault, r.Host, kind, name, version)
}


func keyVaultSecretKey(vault, name string) string { return vault + "/" + name }

func handleKVSetSecret(w http.ResponseWriter, r *http.Request, vault, name string) {
	var body struct {
		Value       string            `json:"value"`
		Tags        map[string]string `json:"tags,omitempty"`
		ContentType string            `json:"contentType,omitempty"`
		Attributes  *KeyVaultAttrs    `json:"attributes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sim.AzureError(w, "InvalidRequest",
			"Failed to parse request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	version := generateUUID()
	id := fmt.Sprintf("%s://%s.vault.%s/secrets/%s/%s",
		r.URL.Scheme, vault, r.Host, name, version)
	if id == "" || strings.HasPrefix(id, "://") {
		// Fallback when r.URL.Scheme is empty (the sim's mux passes a
		// relative URL). Reconstruct from Host so the SDK can parse
		// the returned ID like a real Key Vault response.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		id = fmt.Sprintf("%s://%s/secrets/%s/%s", scheme, r.Host, name, version)
	}
	secret := KeyVaultSecret{
		Vault:       vault,
		Name:        name,
		ID:          id,
		Value:       body.Value,
		Tags:        body.Tags,
		ContentType: body.ContentType,
		Attributes: KeyVaultAttrs{
			Enabled: true,
			Created: now,
			Updated: now,
		},
	}
	if body.Attributes != nil {
		secret.Attributes.Enabled = body.Attributes.Enabled
		secret.Attributes.NotBefore = body.Attributes.NotBefore
		secret.Attributes.Expires = body.Attributes.Expires
	}
	keyVaultData.Put(keyVaultSecretKey(vault, name), secret)
	sim.WriteJSON(w, http.StatusOK, secret)
}

func handleKVGetSecret(w http.ResponseWriter, r *http.Request, vault, name string) {
	secret, ok := keyVaultData.Get(keyVaultSecretKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "SecretNotFound", http.StatusNotFound,
			"A secret with (name/id) %q was not found in this key vault.", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, secret)
}

func handleKVDeleteSecret(w http.ResponseWriter, r *http.Request, vault, name string) {
	key := keyVaultSecretKey(vault, name)
	secret, ok := keyVaultData.Get(key)
	if !ok {
		sim.AzureErrorf(w, "SecretNotFound", http.StatusNotFound,
			"A secret with (name/id) %q was not found in this key vault.", name)
		return
	}
	keyVaultData.Delete(key)
	// Real Key Vault returns the deleted secret + a recovery URL. The
	// sim returns the secret bytes (sufficient for SDK callers that
	// just check the response body for the deleted resource ID).
	secret.Attributes.Enabled = false
	sim.WriteJSON(w, http.StatusOK, secret)
}

func handleKVListSecrets(w http.ResponseWriter, r *http.Request, vault string) {
	prefix := vault + "/"
	all := keyVaultData.Filter(func(s KeyVaultSecret) bool {
		return s.Vault == vault
	})
	_ = prefix
	if all == nil {
		all = []KeyVaultSecret{}
	}
	out := make([]map[string]any, 0, len(all))
	for _, s := range all {
		out = append(out, map[string]any{
			"id":         s.ID,
			"attributes": s.Attributes,
			"tags":       s.Tags,
		})
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}
