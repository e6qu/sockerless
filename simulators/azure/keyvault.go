package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// ecCurveByJWKName maps JWK curve names (P-256, P-384, P-521) to
// crypto/elliptic.Curve. Returns (nil, false) for unknown values.
func ecCurveByJWKName(name string) (elliptic.Curve, bool) {
	switch name {
	case "P-256":
		return elliptic.P256(), true
	case "P-384":
		return elliptic.P384(), true
	case "P-521":
		return elliptic.P521(), true
	}
	return nil, false
}

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
//
// This is the wire shape only — what handler responses serialise.
// The persistence shape (kvSecretStored) wraps it with Vault+Name
// fields needed for List filters; those fields must not appear on
// the wire so they live on the wrapper, not here.
type KeyVaultSecret struct {
	ID          string            `json:"id"` // Full URL `<vault>/secrets/{name}/<version>`
	Value       string            `json:"value"`
	Attributes  KeyVaultAttrs     `json:"attributes"`
	Tags        map[string]string `json:"tags,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
}

// kvSecretStored is the persistence record. Vault+Name are exported
// with normal JSON tags so sim.Store's json.Marshal round-trip
// preserves them. KeyVaultSecret is embedded so handlers can emit
// the embedded value directly; the Vault+Name fields don't reach
// the wire because they aren't on the embedded type. The List
// handlers iterate over kvSecretStored to filter by Vault.
type kvSecretStored struct {
	Vault string `json:"vault"`
	Name  string `json:"name"`
	KeyVaultSecret
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
	keyVaultData sim.Store[kvSecretStored] // key: <vault>/<secretName>
)

func registerKeyVault(srv *sim.Server) {
	keyVaults = sim.MakeStore[KeyVault](srv.DB(), "keyvaults")
	keyVaultData = sim.MakeStore[kvSecretStored](srv.DB(), "keyvault_secrets")
	keyVaultKeys = sim.MakeStore[kvKeyStored](srv.DB(), "keyvault_keys")
	keyVaultCertificates = sim.MakeStore[kvCertStored](srv.DB(), "keyvault_certificates")

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
		// Real Azure ARM `properties.vaultUri` is always `https://`
		// regardless of TLS termination at the load balancer; the
		// sim hard-codes it for consistency with the data-plane URL
		// emitter (buildKVURL), so SDKs that follow vaultUri into
		// the data plane don't trip on cross-API scheme drift.
		hostname := r.Host
		portSuffix := ""
		if i := strings.LastIndex(hostname, ":"); i >= 0 {
			portSuffix = hostname[i:]
			hostname = hostname[:i]
		}
		vaultURI := fmt.Sprintf("https://%s.vault.%s%s/", name, hostname, portSuffix)

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
	//
	// Requests without an `Authorization` header receive a 401 +
	// `WWW-Authenticate: Bearer` challenge so the Azure SDK's KV
	// clients (azsecrets/azkeys/azcertificates) can complete their
	// challenge-then-retry token-acquisition flow. Real KV is
	// HTTPS-only and the SDK refuses to attach the token until it
	// has read the challenge; the sim trusts any Bearer token
	// thereafter (validation is real-AAD's job).
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
				if r.Header.Get("Authorization") == "" {
					// `authorization` points at the AAD authority the
					// SDK should fetch a token from. Real KV emits
					// `https://login.microsoftonline.com/<tenant>` —
					// external by design (the sim does not host an
					// AAD/OIDC discovery endpoint). The SDK's
					// challenge-response code uses its own configured
					// credential provider against the real AAD; only
					// `resource` is read from the challenge.
					// `resource` is the canonical KV resource URI; SDKs
					// compare it for scope matching.
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(
						`Bearer authorization="http://%s", resource="https://vault.azure.net"`,
						r.Host)) // external: real-Azure authorization=https://login.microsoftonline.com/<tenant>
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
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
// echoes it on read. Wire shape only; persistence wrapper is
// kvKeyStored (same shape as kvSecretStored).
type KeyVaultKey struct {
	ID         string            `json:"kid"`
	JsonWebKey map[string]any    `json:"key,omitempty"`
	Attributes KeyVaultAttrs     `json:"attributes,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type kvKeyStored struct {
	Vault string `json:"vault"`
	Name  string `json:"name"`
	KeyVaultKey
}

// KeyVaultCertificate is a certificate at /certificates/{name}.
// Real KV produces a chain (cert + private key + thumbprint); the
// sim stores the operator-supplied content + a deterministic
// thumbprint. Wire shape only; persistence wrapper is kvCertStored.
type KeyVaultCertificate struct {
	ID             string            `json:"id"`
	X509Thumbprint string            `json:"x5t,omitempty"`
	Policy         map[string]any    `json:"policy,omitempty"`
	Attributes     KeyVaultAttrs     `json:"attributes,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
}

type kvCertStored struct {
	Vault string `json:"vault"`
	Name  string `json:"name"`
	KeyVaultCertificate
}

var (
	keyVaultKeys         sim.Store[kvKeyStored]
	keyVaultCertificates sim.Store[kvCertStored]
)

// handleKVKey routes /keys/* requests.
//
//	POST /keys/{name}/create     — generate (stash JsonWebKey on body if present)
//	PUT  /keys/{name}            — import
//	GET  /keys/{name}            — get latest
//	GET  /keys/{name}/{version}  — get specific (sim collapses to latest)
//	GET  /keys                   — list
//	DELETE /keys/{name}          — delete
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
	kty := defaultKVKty(body.Kty)
	jwk := map[string]any{
		"kid": id,
		"kty": kty,
	}
	// Generate a real RSA modulus when the request asks for RSA, so
	// consumers parsing the JWK can reconstruct a valid public key
	// and not get a placeholder string back. Falls back to a
	// placeholder for non-RSA types since the sim doesn't simulate
	// EC / oct curves end-to-end.
	switch kty {
	case "RSA", "RSA-HSM":
		bits := body.KeySize
		if bits <= 0 {
			bits = 2048
		}
		k, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			sim.AzureError(w, "InternalServerError",
				"failed to generate RSA key: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		jwk["n"] = base64.RawURLEncoding.EncodeToString(k.N.Bytes())
		jwk["e"] = base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())
	case "EC", "EC-HSM":
		// Generate a real EC public key on the requested curve so
		// JWK consumers (go-jose, crypto/ecdsa) can parse the
		// resulting `x` / `y` / `crv` triple. The curve name is
		// the JWK form (P-256 / P-384 / P-521); map to crypto/elliptic.
		curveName := body.Crv
		if curveName == "" {
			curveName = "P-256"
		}
		curve, ok := ecCurveByJWKName(curveName)
		if !ok {
			sim.AzureError(w, "InvalidRequest",
				"unsupported curve: "+curveName, http.StatusBadRequest)
			return
		}
		k, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			sim.AzureError(w, "InternalServerError",
				"failed to generate EC key: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		jwk["crv"] = curveName
		jwk["x"] = base64.RawURLEncoding.EncodeToString(k.X.Bytes())
		jwk["y"] = base64.RawURLEncoding.EncodeToString(k.Y.Bytes())
	case "oct", "oct-HSM":
		// Symmetric key: emit a real random `k` field of the
		// requested size (default 256 bits). Real Azure KV's
		// symmetric keys never expose the material on the wire,
		// but the sim does so SDK consumers can decode `kty` +
		// `k` to a usable key for end-to-end smoke tests.
		bits := body.KeySize
		if bits <= 0 {
			bits = 256
		}
		keyMaterial := make([]byte, bits/8)
		if _, err := rand.Read(keyMaterial); err != nil {
			sim.AzureError(w, "InternalServerError",
				"failed to generate symmetric key: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
		jwk["k"] = base64.RawURLEncoding.EncodeToString(keyMaterial)
	default:
		sim.AzureError(w, "InvalidRequest",
			"unsupported kty: "+kty, http.StatusBadRequest)
		return
	}
	if body.Crv != "" {
		jwk["crv"] = body.Crv
	}
	now := time.Now().Unix()
	key := KeyVaultKey{
		ID: id, JsonWebKey: jwk,
		Attributes: KeyVaultAttrs{Enabled: true, Created: now, Updated: now},
		Tags:       body.Tags,
	}
	if body.Attributes != nil {
		key.Attributes.Enabled = body.Attributes.Enabled
	}
	keyVaultKeys.Put(keyVaultKeyKey(vault, name), kvKeyStored{Vault: vault, Name: name, KeyVaultKey: key})
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
		ID: id, JsonWebKey: body.Key,
		Attributes: KeyVaultAttrs{Enabled: true, Created: now, Updated: now},
		Tags:       body.Tags,
	}
	if body.Attributes != nil {
		key.Attributes.Enabled = body.Attributes.Enabled
	}
	keyVaultKeys.Put(keyVaultKeyKey(vault, name), kvKeyStored{Vault: vault, Name: name, KeyVaultKey: key})
	sim.WriteJSON(w, http.StatusOK, key)
}

func handleKVGetKey(w http.ResponseWriter, r *http.Request, vault, name string) {
	rec, ok := keyVaultKeys.Get(keyVaultKeyKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "KeyNotFound", http.StatusNotFound,
			"A key with (name/id) %q was not found in this key vault.", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, rec.KeyVaultKey)
}

func handleKVListKeys(w http.ResponseWriter, r *http.Request, vault string) {
	var out []KeyVaultKey
	for _, k := range keyVaultKeys.List() {
		if k.Vault == vault {
			out = append(out, k.KeyVaultKey)
		}
	}
	if out == nil {
		out = []KeyVaultKey{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleKVDeleteKey(w http.ResponseWriter, r *http.Request, vault, name string) {
	rec, ok := keyVaultKeys.Get(keyVaultKeyKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "KeyNotFound", http.StatusNotFound,
			"A key with (name/id) %q was not found in this key vault.", name)
		return
	}
	keyVaultKeys.Delete(keyVaultKeyKey(vault, name))
	sim.WriteJSON(w, http.StatusOK, rec.KeyVaultKey)
}

// handleKVCertificate routes /certificates/* requests.
//
//	POST /certificates/{name}/create   — issue cert
//	GET  /certificates/{name}          — get
//	GET  /certificates                 — list
//	DELETE /certificates/{name}        — delete
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		sim.AzureError(w, "InvalidRequest",
			"Failed to parse request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	version := generateUUID()
	id := buildKVURL(r, vault, "certificates", name, version)
	now := time.Now().Unix()
	thumbprint := strings.ToUpper(generateUUID()[:8])
	c := KeyVaultCertificate{
		ID: id, X509Thumbprint: thumbprint,
		Policy:     body.Policy,
		Attributes: KeyVaultAttrs{Enabled: true, Created: now, Updated: now},
		Tags:       body.Tags,
	}
	if body.Attributes != nil {
		c.Attributes.Enabled = body.Attributes.Enabled
	}
	keyVaultCertificates.Put(keyVaultCertKey(vault, name), kvCertStored{Vault: vault, Name: name, KeyVaultCertificate: c})
	sim.WriteJSON(w, http.StatusOK, c)
}

func handleKVGetCertificate(w http.ResponseWriter, r *http.Request, vault, name string) {
	rec, ok := keyVaultCertificates.Get(keyVaultCertKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "CertificateNotFound", http.StatusNotFound,
			"A certificate with (name/id) %q was not found in this key vault.", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, rec.KeyVaultCertificate)
}

func handleKVListCertificates(w http.ResponseWriter, r *http.Request, vault string) {
	var out []KeyVaultCertificate
	for _, c := range keyVaultCertificates.List() {
		if c.Vault == vault {
			out = append(out, c.KeyVaultCertificate)
		}
	}
	if out == nil {
		out = []KeyVaultCertificate{}
	}
	sim.WriteJSON(w, http.StatusOK, map[string]any{"value": out})
}

func handleKVDeleteCertificate(w http.ResponseWriter, r *http.Request, vault, name string) {
	rec, ok := keyVaultCertificates.Get(keyVaultCertKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "CertificateNotFound", http.StatusNotFound,
			"A certificate with (name/id) %q was not found in this key vault.", name)
		return
	}
	keyVaultCertificates.Delete(keyVaultCertKey(vault, name))
	sim.WriteJSON(w, http.StatusOK, rec.KeyVaultCertificate)
}

func defaultKVKty(s string) string {
	if s == "" {
		return "RSA"
	}
	return s
}

// buildKVURL constructs the canonical KV data-plane resource ID
// (`https://<vault>.vault.<host>/<kind>/<name>/<version>`).
//
// Real Key Vault always emits https URLs; SDKs (azsecrets,
// azkeys, azcertificates) parse the returned `id`/`kid` and reject
// http-scheme URLs at the URL-validation stage. The sim hard-codes
// https for fidelity even though its own listener may be HTTP —
// clients that follow the URL with their own HTTPS resolver
// against the canonical `<vault>.vault.azure.net` host succeed.
//
// `r.Host` already carries the `<vault>.vault.<sim-or-real-host>`
// subdomain the client connected on (the WrapHandler dispatch
// extracted `vault` from this same r.Host). `r.Host` IS the
// canonical host; prepending another `<vault>.vault.` would
// duplicate host segments like `kv.vault.kv.vault.azure.net`.
// Use `r.Host` directly.
func buildKVURL(r *http.Request, vault, kind, name, version string) string {
	host := r.Host
	if host == "" {
		host = vault + ".vault.azure.net"
	}
	return fmt.Sprintf("https://%s/%s/%s/%s", host, kind, name, version)
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
	// All KV ID emitters route through buildKVURL so the scheme +
	// host shape stays consistent across handlers. r.URL.Scheme is
	// unreliable behind the sim's mux (typically empty); the helper
	// hard-codes `https://` to match real Azure KV.
	id := buildKVURL(r, vault, "secrets", name, version)
	secret := KeyVaultSecret{
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
	keyVaultData.Put(keyVaultSecretKey(vault, name), kvSecretStored{Vault: vault, Name: name, KeyVaultSecret: secret})
	sim.WriteJSON(w, http.StatusOK, secret)
}

func handleKVGetSecret(w http.ResponseWriter, r *http.Request, vault, name string) {
	rec, ok := keyVaultData.Get(keyVaultSecretKey(vault, name))
	if !ok {
		sim.AzureErrorf(w, "SecretNotFound", http.StatusNotFound,
			"A secret with (name/id) %q was not found in this key vault.", name)
		return
	}
	sim.WriteJSON(w, http.StatusOK, rec.KeyVaultSecret)
}

func handleKVDeleteSecret(w http.ResponseWriter, r *http.Request, vault, name string) {
	key := keyVaultSecretKey(vault, name)
	rec, ok := keyVaultData.Get(key)
	if !ok {
		sim.AzureErrorf(w, "SecretNotFound", http.StatusNotFound,
			"A secret with (name/id) %q was not found in this key vault.", name)
		return
	}
	keyVaultData.Delete(key)
	// Real Key Vault returns the deleted secret + a recovery URL. The
	// sim returns the secret bytes (sufficient for SDK callers that
	// just check the response body for the deleted resource ID).
	rec.Attributes.Enabled = false
	sim.WriteJSON(w, http.StatusOK, rec.KeyVaultSecret)
}

func handleKVListSecrets(w http.ResponseWriter, r *http.Request, vault string) {
	all := keyVaultData.Filter(func(s kvSecretStored) bool {
		return s.Vault == vault
	})
	if all == nil {
		all = []kvSecretStored{}
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
