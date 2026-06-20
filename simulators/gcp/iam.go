package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

type GCPServiceAccount struct {
	Name        string `json:"name"`
	ProjectId   string `json:"projectId"`
	UniqueId    string `json:"uniqueId"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled"`
}

type IAMPolicy struct {
	Kind       string       `json:"kind,omitempty"`
	ResourceId string       `json:"resourceId,omitempty"`
	Bindings   []IAMBinding `json:"bindings"`
	Etag       string       `json:"etag"`
	Version    int          `json:"version"`
}

type IAMBinding struct {
	Role    string   `json:"role"`
	Members []string `json:"members"`
	// Condition is a nested writable object (CEL expression + title)
	// the sim persists verbatim so setIamPolicy→getIamPolicy round-trips
	// byte-exact for conditional bindings.
	Condition json.RawMessage `json:"condition,omitempty"`
}

// gcpResourcePolicies is the shared IAM policy store for GCP resources
// (artifact registry, storage buckets, etc.). It's package-level so that
// resource-specific handlers can process :getIamPolicy / :setIamPolicy requests.
var gcpResourcePolicies sim.Store[IAMPolicy]

// GCPServiceAccountKey mirrors the `iam#ServiceAccountKey` resource. Real GCP
// only returns privateKeyData on creation; subsequent Gets omit it.
type GCPServiceAccountKey struct {
	Name            string `json:"name"`
	KeyAlgorithm    string `json:"keyAlgorithm"`
	ValidAfterTime  string `json:"validAfterTime"`
	ValidBeforeTime string `json:"validBeforeTime"`
	KeyType         string `json:"keyType"`
	PrivateKeyData  string `json:"privateKeyData,omitempty"` // only on Create response
	PrivateKeyType  string `json:"privateKeyType,omitempty"` // only on Create response
}

func registerIAM(srv *sim.Server) {
	serviceAccounts := sim.MakeStore[GCPServiceAccount](srv.DB(), "iam_service_accounts")
	saKeys := sim.MakeStore[GCPServiceAccountKey](srv.DB(), "iam_sa_keys")
	projectPolicies := sim.MakeStore[IAMPolicy](srv.DB(), "iam_project_policies")
	gcpResourcePolicies = sim.MakeStore[IAMPolicy](srv.DB(), "iam_resource_policies")
	resourcePolicies := gcpResourcePolicies

	// CRM GetProject (v1) — used by google_project_service to verify project exists
	srv.HandleFunc("GET /v1/projects/{project}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"projectNumber":  "123456789012",
			"projectId":      project,
			"lifecycleState": "ACTIVE",
			"name":           project,
		})
	})

	// CRM GetProject (v3) — used by google_project_iam_member
	srv.HandleFunc("GET /v3/projects/{project}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"name":        "projects/" + project,
			"projectId":   project,
			"state":       "ACTIVE",
			"displayName": project,
		})
	})

	// Create service account
	srv.HandleFunc("POST /v1/projects/{project}/serviceAccounts", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")

		var req struct {
			AccountId      string `json:"accountId"`
			ServiceAccount struct {
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
			} `json:"serviceAccount"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		email := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", req.AccountId, project)
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)

		sa := GCPServiceAccount{
			Name:        name,
			ProjectId:   project,
			UniqueId:    generateUUID()[:20],
			Email:       email,
			DisplayName: req.ServiceAccount.DisplayName,
			Description: req.ServiceAccount.Description,
		}
		serviceAccounts.Put(name, sa)

		sim.WriteJSON(w, http.StatusOK, sa)
	})

	// Get service account
	srv.HandleFunc("GET /v1/projects/{project}/serviceAccounts/{email}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)

		sa, ok := serviceAccounts.Get(name)
		if !ok {
			sim.GCPErrorf(w, 404, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		sim.WriteJSON(w, http.StatusOK, sa)
	})

	// Delete service account
	srv.HandleFunc("DELETE /v1/projects/{project}/serviceAccounts/{email}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)

		serviceAccounts.Delete(name)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})

	// Create service account key — returns full key on creation only.
	// Real GCP wire: POST /v1/projects/{p}/serviceAccounts/{email}/keys
	// project="-" is the GCP wildcard: extract the project from the email.
	srv.HandleFunc("POST /v1/projects/{project}/serviceAccounts/{email}/keys", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		saName := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
		if _, ok := serviceAccounts.Get(saName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		keyID := generateUUID()
		keyName := fmt.Sprintf("%s/keys/%s", saName, keyID)
		now := time.Now().UTC()
		key := GCPServiceAccountKey{
			Name:            keyName,
			KeyAlgorithm:    "KEY_ALG_RSA_2048",
			ValidAfterTime:  now.Format(time.RFC3339),
			ValidBeforeTime: now.AddDate(10, 0, 0).Format(time.RFC3339),
			KeyType:         "USER_MANAGED",
		}
		// Generate the key material before persisting metadata: if generation
		// fails, the store must not retain a key that never had private-key
		// material (a subsequent Get would return a phantom key).
		privateKeyData, err := gcpMakeSAKeyJSON(project, keyID, email, key.ValidAfterTime, key.ValidBeforeTime)
		if err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "generate key: %v", err)
			return
		}
		saKeys.Put(keyName, key)

		resp := key
		resp.PrivateKeyData = privateKeyData
		resp.PrivateKeyType = "TYPE_GOOGLE_CREDENTIALS_FILE"
		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// Get service account key (no private key data after creation).
	srv.HandleFunc("GET /v1/projects/{project}/serviceAccounts/{email}/keys/{keyId}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		keyID := sim.PathParam(r, "keyId")
		keyName := fmt.Sprintf("projects/%s/serviceAccounts/%s/keys/%s", project, email, keyID)
		key, ok := saKeys.Get(keyName)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "key %s not found", keyID)
			return
		}
		sim.WriteJSON(w, http.StatusOK, key)
	})

	// List service account keys.
	srv.HandleFunc("GET /v1/projects/{project}/serviceAccounts/{email}/keys", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		prefix := fmt.Sprintf("projects/%s/serviceAccounts/%s/keys/", project, email)
		keys := saKeys.Filter(func(k GCPServiceAccountKey) bool {
			return strings.HasPrefix(k.Name, prefix)
		})
		if keys == nil {
			keys = []GCPServiceAccountKey{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"keys": keys})
	})

	// Delete service account key.
	srv.HandleFunc("DELETE /v1/projects/{project}/serviceAccounts/{email}/keys/{keyId}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		keyID := sim.PathParam(r, "keyId")
		keyName := fmt.Sprintf("projects/%s/serviceAccounts/%s/keys/%s", project, email, keyID)
		if !saKeys.Delete(keyName) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "key %s not found", keyID)
			return
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})

	// IAM Credentials API — short-lived tokens minted on behalf of a
	// service account. Real GCP paths:
	//   POST /v1/projects/{p}/serviceAccounts/{email}:generateAccessToken
	//   POST /v1/projects/{p}/serviceAccounts/{email}:generateIdToken
	// Sockerless runner setup (gcloud auth application-default,
	// google-github-actions/auth) calls generateAccessToken to mint
	// scoped tokens against the workload-identity-federated SA. The
	// Access driver's `id-token` category calls generateIdToken for
	// cross-Service impersonation flows where the runner SA mints an
	// ID token for a different audience SA. The simulator returns
	// real-shape responses without validating the signature on the
	// resulting tokens — sim audience handlers don't validate either.
	srv.HandleFunc("POST /v1/projects/{project}/serviceAccounts/{emailAction}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		emailAction := sim.PathParam(r, "emailAction")
		email, action, _ := strings.Cut(emailAction, ":")
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
		if _, ok := serviceAccounts.Get(name); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		switch action {
		case "generateAccessToken":
			// Real expiry is RFC3339Nano with timezone offset; the SDK
			// parses it with time.Parse(time.RFC3339).
			expireTime := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"accessToken": "ya29.sim-" + generateUUID(),
				"expireTime":  expireTime,
			})
		case "generateIdToken":
			// Body: { audience, includeEmail, delegates }. Response: { token }.
			// Mint a real-shape JWT whose `aud` claim equals the request's
			// audience so SDKs that pre-decode the token (rare in test
			// paths, common in cross-Service auth chains) accept it.
			var req struct {
				Audience     string   `json:"audience"`
				IncludeEmail bool     `json:"includeEmail"`
				Delegates    []string `json:"delegates"`
			}
			if err := sim.ReadJSON(r, &req); err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
				return
			}
			if req.Audience == "" {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "audience is required")
				return
			}
			now := time.Now()
			expires := now.Add(1 * time.Hour)
			token := mintSimIdToken(idTokenSignKey(), email, req.Audience, req.IncludeEmail, now, expires)
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"token": token,
			})
		default:
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unsupported service-account action %q", action)
		}
	})

	// List service accounts
	srv.HandleFunc("GET /v1/projects/{project}/serviceAccounts", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := fmt.Sprintf("projects/%s/serviceAccounts/", project)

		accounts := serviceAccounts.Filter(func(sa GCPServiceAccount) bool {
			return strings.HasPrefix(sa.Name, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"accounts": accounts,
		})
	})

	// Project IAM - getIamPolicy / setIamPolicy
	srv.HandleFunc("POST /v1/projects/{projectAction}", func(w http.ResponseWriter, r *http.Request) {
		projectAction := sim.PathParam(r, "projectAction")
		project, action, _ := strings.Cut(projectAction, ":")

		switch action {
		case "getIamPolicy":
			policy, ok := projectPolicies.Get(project)
			if !ok {
				policy = IAMPolicy{
					Bindings: []IAMBinding{},
					Etag:     gcpPolicyETag(),
					Version:  1,
				}
				// Persist the synthesized default so its etag is stable across
				// reads — the optimistic-concurrency check on setIamPolicy
				// validates against the etag a prior getIamPolicy returned.
				projectPolicies.Put(project, policy)
			}
			sim.WriteJSON(w, http.StatusOK, policy)
		case "setIamPolicy":
			var req struct {
				Policy IAMPolicy `json:"policy"`
			}
			if err := sim.ReadJSON(r, &req); err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
				return
			}

			current, present := projectPolicies.Get(project)
			if gcpIAMETagConflict(w, req.Policy.Etag, current.Etag, present) {
				return
			}
			req.Policy.Etag = gcpPolicyETag()
			if req.Policy.Version == 0 {
				req.Policy.Version = 1
			}
			projectPolicies.Put(project, req.Policy)
			sim.WriteJSON(w, http.StatusOK, req.Policy)
		default:
			http.NotFound(w, r)
		}
	})

	// Catch-all AIP-141 IAM dispatcher (Artifact Registry + any
	// resource not handled by a more-specific verb dispatcher).
	// Resources with their own verb dispatcher (Pub/Sub topics +
	// subscriptions, Memorystore instances, etc.) delegate to
	// handleResourceIAM directly.
	srv.HandleFunc("POST /v1/{resource...}", func(w http.ResponseWriter, r *http.Request) {
		resource := sim.PathParam(r, "resource")
		var action string
		for _, verb := range []string{":getIamPolicy", ":setIamPolicy", ":testIamPermissions"} {
			if strings.HasSuffix(resource, verb) {
				action = strings.TrimPrefix(verb, ":")
				resource = strings.TrimSuffix(resource, verb)
				break
			}
		}
		if action == "" {
			http.NotFound(w, r)
			return
		}
		handleResourceIAM(w, r, resourcePolicies, resource, action)
	})

	// Bucket IAM - getIamPolicy
	srv.HandleFunc("GET /storage/v1/b/{bucket}/iam", func(w http.ResponseWriter, r *http.Request) {
		bucket := sim.PathParam(r, "bucket")

		policy, ok := resourcePolicies.Get("bucket/" + bucket)
		if !ok {
			policy = IAMPolicy{
				Bindings: []IAMBinding{},
				Etag:     gcpPolicyETag(),
				Version:  1,
			}
		}
		policy.Kind = "storage#policy"
		policy.ResourceId = "projects/_/buckets/" + bucket
		sim.WriteJSON(w, http.StatusOK, policy)
	})

	// Bucket IAM - setIamPolicy
	srv.HandleFunc("PUT /storage/v1/b/{bucket}/iam", func(w http.ResponseWriter, r *http.Request) {
		bucket := sim.PathParam(r, "bucket")

		var policy IAMPolicy
		if err := sim.ReadJSON(r, &policy); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		policy.Etag = gcpPolicyETag()
		if policy.Version == 0 {
			policy.Version = 1
		}
		policy.Kind = "storage#policy"
		policy.ResourceId = "projects/_/buckets/" + bucket
		resourcePolicies.Put("bucket/"+bucket, policy)

		sim.WriteJSON(w, http.StatusOK, policy)
	})
}

// gcpIAMETagConflict enforces the optimistic-concurrency contract real Cloud
// IAM applies on setIamPolicy: a request whose policy carries a non-empty etag
// that does not match the currently-stored policy's etag is rejected with 409
// ABORTED so the caller re-reads and retries. An empty request etag means the
// caller opted out of the check (a blind overwrite), which GCP permits.
func gcpIAMETagConflict(w http.ResponseWriter, reqEtag, currentEtag string, present bool) bool {
	if reqEtag == "" {
		return false
	}
	if !present || reqEtag != currentEtag {
		sim.GCPErrorf(w, http.StatusConflict, "ABORTED",
			"There were concurrent policy changes. Please retry the whole read-modify-write with exponential backoff.")
		return true
	}
	return false
}

// handleResourceIAM processes the three AIP-141 IAM verbs against a named
// resource: getIamPolicy / setIamPolicy / testIamPermissions. Every GCP
// resource type exposes this triple; the sim's per-resource handlers
// delegate the verb branch here.
func handleResourceIAM(w http.ResponseWriter, r *http.Request, store sim.Store[IAMPolicy], resource, action string) {
	switch action {
	case "getIamPolicy":
		policy, ok := store.Get(resource)
		if !ok {
			policy = IAMPolicy{
				Bindings: []IAMBinding{},
				Etag:     gcpPolicyETag(),
				Version:  1,
			}
			// Persist the synthesized default so its etag is stable across
			// reads — the optimistic-concurrency check on setIamPolicy
			// validates against the etag a prior getIamPolicy returned.
			store.Put(resource, policy)
		}
		sim.WriteJSON(w, http.StatusOK, policy)
	case "setIamPolicy":
		var req struct {
			Policy IAMPolicy `json:"policy"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		current, present := store.Get(resource)
		if gcpIAMETagConflict(w, req.Policy.Etag, current.Etag, present) {
			return
		}
		req.Policy.Etag = gcpPolicyETag()
		if req.Policy.Version == 0 {
			req.Policy.Version = 1
		}
		store.Put(resource, req.Policy)
		sim.WriteJSON(w, http.StatusOK, req.Policy)
	case "testIamPermissions":
		var req struct {
			Permissions []string `json:"permissions"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		// Sim doesn't model authorization; echo the requested set as
		// allowed. Real GCP filters to the subset the caller actually
		// has — but every caller in the sim is effectively a project
		// admin, so the full echo is the truthful response.
		sim.WriteJSON(w, http.StatusOK, map[string]any{"permissions": req.Permissions})
	default:
		http.NotFound(w, r)
	}
}

// gcpResourceIAMStore returns the package-level resource-IAM store
// used by per-resource handlers. Centralises the cross-service IAM
// policy persistence so getIamPolicy / setIamPolicy round-trips
// match regardless of which resource type registered the policy.
func gcpResourceIAMStore() sim.Store[IAMPolicy] { return gcpResourcePolicies }

// gcpProjectFromEmail extracts the project ID from a GCP service account email.
// When the GCP API receives project="-" (a valid wildcard), the SDK resolves the
// project from the account email: {accountId}@{project}.iam.gserviceaccount.com.
func gcpProjectFromEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	host := email[at+1:]
	return strings.TrimSuffix(host, ".iam.gserviceaccount.com")
}

// gcpMakeSAKeyJSON generates a real RSA-2048 key pair and returns it encoded
// as a base64 GCP service-account JSON credential file — matching the exact
// shape real GCP returns for CreateServiceAccountKey.
func gcpMakeSAKeyJSON(project, keyID, email, validAfter, validBefore string) (string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate RSA key: %w", err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	payload := map[string]string{
		"type":                        "service_account",
		"project_id":                  project,
		"private_key_id":              keyID,
		"private_key":                 string(privPEM),
		"client_email":                email,
		"client_id":                   generateUUID()[:20],
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/" + email,
		"universe_domain":             "googleapis.com",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal JSON key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
