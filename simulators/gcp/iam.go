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
	"sort"
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

// GCPCustomRole mirrors the iam#Role resource for project- and
// organization-scoped custom roles. Name is the fully-qualified resource path
// (projects/{p}/roles/{id} or organizations/{o}/roles/{id}). Deleted roles are
// soft-deleted (Deleted=true) and can be undeleted within GCP's retention
// window; the sim keeps them in the store so UndeleteRole can revive them.
type GCPCustomRole struct {
	Name                string   `json:"name"`
	Title               string   `json:"title,omitempty"`
	Description         string   `json:"description,omitempty"`
	IncludedPermissions []string `json:"includedPermissions,omitempty"`
	Stage               string   `json:"stage,omitempty"`
	Etag                string   `json:"etag"`
	Deleted             bool     `json:"deleted,omitempty"`
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
	customRoles := sim.MakeStore[GCPCustomRole](srv.DB(), "iam_custom_roles")
	if iamLROs == nil {
		iamLROs = sim.MakeStore[Operation](srv.DB(), "iam_lro_operations")
	}
	if iamResources == nil {
		iamResources = sim.MakeStore[map[string]any](srv.DB(), "iam_admin_resources")
	}

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
			UniqueId:    gcpNumericID(21),
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
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)

		sa, ok := serviceAccounts.Get(name)
		if !ok {
			sim.GCPErrorf(w, 404, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		sim.WriteJSON(w, http.StatusOK, sa)
	})

	// Update service account — PATCH with an updateMask over the mutable
	// fields (displayName / description). Real GCP's UpdateServiceAccount
	// wraps the account under a `serviceAccount` envelope alongside the mask.
	srv.HandleFunc("PATCH /v1/projects/{project}/serviceAccounts/{email}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
		sa, ok := serviceAccounts.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		var req struct {
			ServiceAccount struct {
				DisplayName string `json:"displayName"`
				Description string `json:"description"`
			} `json:"serviceAccount"`
			UpdateMask string `json:"updateMask"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		// The mask also rides as a query param in some client paths; prefer
		// the body mask, fall back to the query.
		mask := req.UpdateMask
		if mask == "" {
			mask = r.URL.Query().Get("updateMask")
		}
		for _, field := range strings.Split(mask, ",") {
			switch strings.TrimSpace(field) {
			case "displayName":
				sa.DisplayName = req.ServiceAccount.DisplayName
			case "description":
				sa.Description = req.ServiceAccount.Description
			}
		}
		serviceAccounts.Put(name, sa)
		sim.WriteJSON(w, http.StatusOK, sa)
	})

	// Update service account (legacy full-replace) — PUT carries the whole
	// ServiceAccount resource (no updateMask). Real GCP's
	// serviceAccounts.update replaces the mutable fields (displayName /
	// description) from the request body.
	srv.HandleFunc("PUT /v1/projects/{project}/serviceAccounts/{email}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		email := sim.PathParam(r, "email")
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
		sa, ok := serviceAccounts.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		var req struct {
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		sa.DisplayName = req.DisplayName
		sa.Description = req.Description
		serviceAccounts.Put(name, sa)
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
		sa, ok := serviceAccounts.Get(saName)
		if !ok {
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
		privateKeyData, err := gcpMakeSAKeyJSON(project, keyID, email, sa.UniqueId, key.ValidAfterTime, key.ValidBeforeTime)
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
		if project == "-" {
			project = gcpProjectFromEmail(email)
		}
		name := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
		if _, ok := serviceAccounts.Get(name); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "Service account %s not found", email)
			return
		}
		switch action {
		case "disable":
			sa, _ := serviceAccounts.Get(name)
			sa.Disabled = true
			serviceAccounts.Put(name, sa)
			sim.WriteJSON(w, http.StatusOK, map[string]any{})
			return
		case "enable":
			sa, _ := serviceAccounts.Get(name)
			sa.Disabled = false
			serviceAccounts.Put(name, sa)
			sim.WriteJSON(w, http.StatusOK, map[string]any{})
			return
		case "getIamPolicy", "setIamPolicy", "testIamPermissions":
			// The service account is itself a resource that carries an IAM
			// policy (e.g. granting roles/iam.serviceAccountUser to a member).
			// Reuse the shared resource-IAM store so the etag / member-
			// validation / optimistic-concurrency behavior matches buckets
			// and projects.
			handleResourceIAM(w, r, resourcePolicies, "serviceAccount/"+email, action)
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
		sort.Slice(accounts, func(i, j int) bool { return accounts[i].Name < accounts[j].Name })
		page, next, ok := paginateList(w, r, accounts)
		if !ok {
			return
		}
		resp := map[string]any{"accounts": page}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
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

			if err := validateIAMMembers(req.Policy.Bindings); err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%v", err)
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

	// QueryTestablePermissions — the catalog of permissions that can be tested
	// (and thus included in a custom role) on a given resource. `gcloud iam
	// roles create/update` calls this to validate the --permissions flag before
	// issuing CreateRole/UpdateRole. Real GCP returns a paginated list scoped to
	// the resource's service surface; the sim returns the representative catalog
	// it knows about (the union of permissions its predefined roles reference,
	// plus the common service-prefixed permissions the repo exercises).
	srv.HandleFunc("POST /v1/permissions:queryTestablePermissions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FullResourceName string `json:"fullResourceName"`
			PageSize         int    `json:"pageSize"`
			PageToken        string `json:"pageToken"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		perms := gcpTestablePermissions()
		out := make([]map[string]any, 0, len(perms))
		for _, p := range perms {
			out = append(out, map[string]any{
				"name":                    p,
				"stage":                   "GA",
				"customRolesSupportLevel": "SUPPORTED",
			})
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"permissions": out})
	})

	// Custom roles — projects.roles.* and organizations.roles.*. A custom
	// role is a tenant-defined IAM role with its own includedPermissions; the
	// two scopes share identical CRUD semantics, differing only in the parent
	// prefix (projects/{p} vs organizations/{o}).
	registerCustomRoles(srv, customRoles, "project", "GET /v1/projects/{parent}/roles", "GET /v1/projects/{parent}/roles/{role}",
		"POST /v1/projects/{parent}/roles", "POST /v1/projects/{parent}/roles/{roleAction}",
		"PATCH /v1/projects/{parent}/roles/{role}", "DELETE /v1/projects/{parent}/roles/{role}")
	registerCustomRoles(srv, customRoles, "organization", "GET /v1/organizations/{parent}/roles", "GET /v1/organizations/{parent}/roles/{role}",
		"POST /v1/organizations/{parent}/roles", "POST /v1/organizations/{parent}/roles/{roleAction}",
		"PATCH /v1/organizations/{parent}/roles/{role}", "DELETE /v1/organizations/{parent}/roles/{role}")

	// Workload Identity Federation pools (project-scoped) + workforce pools
	// (location-scoped, org-level) + OAuth clients. Each is a real CRUD
	// surface whose create/patch/delete/undelete return a long-running
	// Operation; the sim settles the operation synchronously (done=true).
	registerWorkloadIdentityPools(srv)
	registerWorkforcePools(srv)
	registerOAuthClients(srv)

	// Predefined roles — roles.list / roles.get. The catalog of curated
	// (Google-managed) roles. The sim carries a bounded representative set
	// (the basic roles plus the IAM/storage roles the repo references), not
	// the full ~1500-role catalog. Custom-role CRUD is a staged epic and is
	// not handled here.
	srv.HandleFunc("GET /v1/roles", func(w http.ResponseWriter, r *http.Request) {
		roles := gcpPredefinedRoles()
		// roles.list omits includedPermissions unless view=FULL.
		full := r.URL.Query().Get("view") == "FULL"
		out := make([]map[string]any, 0, len(roles))
		for _, role := range roles {
			out = append(out, gcpRoleJSON(role, full))
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"roles": out})
	})

	srv.HandleFunc("GET /v1/roles/{role...}", func(w http.ResponseWriter, r *http.Request) {
		name := "roles/" + sim.PathParam(r, "role")
		for _, role := range gcpPredefinedRoles() {
			if role.Name == name {
				// roles.get returns the full role including includedPermissions.
				sim.WriteJSON(w, http.StatusOK, gcpRoleJSON(role, true))
				return
			}
		}
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "The role named %s was not found.", name)
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
			// Persist the synthesized default so its etag is stable across
			// reads — the optimistic-concurrency check on setIamPolicy
			// validates against the etag a prior getIamPolicy returned.
			resourcePolicies.Put("bucket/"+bucket, policy)
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
		if err := validateIAMMembers(policy.Bindings); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%v", err)
			return
		}

		current, present := resourcePolicies.Get("bucket/" + bucket)
		if gcpIAMETagConflict(w, policy.Etag, current.Etag, present) {
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

// registerCustomRoles mounts the custom-role CRUD surface for one scope
// (projects or organizations). The route patterns differ only in the parent
// segment, so both scopes share these handlers. `scope` is "project" or
// "organization"; `parentPrefix` derives the resource-name prefix
// (projects/{p} / organizations/{o}).
func registerCustomRoles(srv *sim.Server, store sim.Store[GCPCustomRole], scope, listPat, getPat, createPat, actionPat, patchPat, deletePat string) {
	parentPrefix := func(parent string) string {
		if scope == "organization" {
			return "organizations/" + parent
		}
		return "projects/" + parent
	}

	// ListRoles
	srv.HandleFunc(listPat, func(w http.ResponseWriter, r *http.Request) {
		prefix := parentPrefix(sim.PathParam(r, "parent")) + "/roles/"
		// roles.list defaults to BASIC view (no includedPermissions);
		// view=FULL returns permissions. showDeleted controls whether
		// soft-deleted roles are returned.
		full := r.URL.Query().Get("view") == "FULL"
		showDeleted := r.URL.Query().Get("showDeleted") == "true"
		roles := store.Filter(func(role GCPCustomRole) bool {
			if !strings.HasPrefix(role.Name, prefix) {
				return false
			}
			return showDeleted || !role.Deleted
		})
		sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
		out := make([]map[string]any, 0, len(roles))
		for _, role := range roles {
			out = append(out, gcpCustomRoleJSON(role, full))
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"roles": out})
	})

	// GetRole
	srv.HandleFunc(getPat, func(w http.ResponseWriter, r *http.Request) {
		name := parentPrefix(sim.PathParam(r, "parent")) + "/roles/" + sim.PathParam(r, "role")
		role, ok := store.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "The role named %s was not found.", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, gcpCustomRoleJSON(role, true))
	})

	// CreateRole
	srv.HandleFunc(createPat, func(w http.ResponseWriter, r *http.Request) {
		parent := parentPrefix(sim.PathParam(r, "parent"))
		var req struct {
			RoleId string `json:"roleId"`
			Role   struct {
				Title               string   `json:"title"`
				Description         string   `json:"description"`
				IncludedPermissions []string `json:"includedPermissions"`
				Stage               string   `json:"stage"`
			} `json:"role"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if req.RoleId == "" {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "roleId is required")
			return
		}
		name := parent + "/roles/" + req.RoleId
		if _, exists := store.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "A role named %s already exists.", name)
			return
		}
		stage := req.Role.Stage
		if stage == "" {
			stage = "ALPHA"
		}
		role := GCPCustomRole{
			Name:                name,
			Title:               req.Role.Title,
			Description:         req.Role.Description,
			IncludedPermissions: req.Role.IncludedPermissions,
			Stage:               stage,
			Etag:                gcpPolicyETag(),
		}
		store.Put(name, role)
		sim.WriteJSON(w, http.StatusOK, gcpCustomRoleJSON(role, true))
	})

	// UndeleteRole — POST .../roles/{role}:undelete
	srv.HandleFunc(actionPat, func(w http.ResponseWriter, r *http.Request) {
		roleAction := sim.PathParam(r, "roleAction")
		roleID, action, found := strings.Cut(roleAction, ":")
		if !found || action != "undelete" {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unsupported role action %q", roleAction)
			return
		}
		name := parentPrefix(sim.PathParam(r, "parent")) + "/roles/" + roleID
		role, ok := store.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "The role named %s was not found.", name)
			return
		}
		role.Deleted = false
		role.Etag = gcpPolicyETag()
		store.Put(name, role)
		sim.WriteJSON(w, http.StatusOK, gcpCustomRoleJSON(role, true))
	})

	// UpdateRole — PATCH with an updateMask over the mutable fields.
	srv.HandleFunc(patchPat, func(w http.ResponseWriter, r *http.Request) {
		name := parentPrefix(sim.PathParam(r, "parent")) + "/roles/" + sim.PathParam(r, "role")
		role, ok := store.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "The role named %s was not found.", name)
			return
		}
		var req GCPCustomRole
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		// Honor optimistic concurrency: a stale etag on the request body is
		// rejected with 409 ABORTED (same contract as setIamPolicy).
		if gcpIAMETagConflict(w, req.Etag, role.Etag, true) {
			return
		}
		// An empty updateMask updates every mutable field present in the body
		// (matches real GCP, which treats a missing mask as "full update").
		mask := r.URL.Query().Get("updateMask")
		fields := strings.Split(mask, ",")
		if mask == "" {
			fields = []string{"title", "description", "includedPermissions", "stage"}
		}
		for _, field := range fields {
			switch strings.TrimSpace(field) {
			case "title":
				role.Title = req.Title
			case "description":
				role.Description = req.Description
			case "includedPermissions":
				role.IncludedPermissions = req.IncludedPermissions
			case "stage":
				role.Stage = req.Stage
			}
		}
		role.Etag = gcpPolicyETag()
		store.Put(name, role)
		sim.WriteJSON(w, http.StatusOK, gcpCustomRoleJSON(role, true))
	})

	// DeleteRole — soft-delete: set deleted=true and return the role.
	srv.HandleFunc(deletePat, func(w http.ResponseWriter, r *http.Request) {
		name := parentPrefix(sim.PathParam(r, "parent")) + "/roles/" + sim.PathParam(r, "role")
		role, ok := store.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "The role named %s was not found.", name)
			return
		}
		role.Deleted = true
		role.Etag = gcpPolicyETag()
		store.Put(name, role)
		sim.WriteJSON(w, http.StatusOK, gcpCustomRoleJSON(role, true))
	})
}

// gcpTestablePermissions returns the representative catalog of permissions the
// sim advertises as testable (includable in a custom role). It's the union of
// every permission referenced by the predefined roles plus the common
// service-prefixed permissions the repo's tests exercise. This bounds the
// catalog to what the sim can faithfully model rather than GCP's full ~7000.
func gcpTestablePermissions() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, role := range gcpPredefinedRoles() {
		for _, p := range role.IncludedPermissions {
			add(p)
		}
	}
	for _, p := range []string{
		"resourcemanager.projects.get",
		"resourcemanager.projects.update",
		"resourcemanager.projects.list",
		"resourcemanager.projects.setIamPolicy",
		"resourcemanager.projects.getIamPolicy",
		"storage.buckets.get",
		"storage.buckets.list",
		"storage.buckets.create",
		"storage.buckets.update",
		"storage.buckets.delete",
		"storage.buckets.getIamPolicy",
		"storage.buckets.setIamPolicy",
		"storage.objects.get",
		"storage.objects.list",
		"storage.objects.create",
		"storage.objects.delete",
		"storage.objects.update",
		"iam.serviceAccounts.actAs",
		"iam.serviceAccounts.get",
		"iam.serviceAccounts.list",
		"iam.serviceAccounts.create",
		"iam.serviceAccounts.delete",
		"iam.serviceAccounts.getIamPolicy",
		"iam.serviceAccounts.setIamPolicy",
		"iam.roles.get",
		"iam.roles.list",
		"iam.roles.create",
		"iam.roles.update",
		"iam.roles.delete",
	} {
		add(p)
	}
	sort.Strings(out)
	return out
}

// gcpCustomRoleJSON renders a custom role into the roles.get / roles.list wire
// shape. includedPermissions is omitted unless full (roles.list BASIC view
// carries no permissions; roles.get and create/update return FULL).
func gcpCustomRoleJSON(role GCPCustomRole, full bool) map[string]any {
	out := map[string]any{
		"name":  role.Name,
		"title": role.Title,
		"etag":  role.Etag,
	}
	if role.Description != "" {
		out["description"] = role.Description
	}
	if role.Stage != "" {
		out["stage"] = role.Stage
	}
	if role.Deleted {
		out["deleted"] = true
	}
	if full {
		out["includedPermissions"] = role.IncludedPermissions
	}
	return out
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

// validateIAMMembers checks every member in every binding against the member
// syntax real Cloud IAM accepts, rejecting malformed members with an error the
// caller surfaces as 400 INVALID_ARGUMENT — matching real GCP, which rejects a
// setIamPolicy carrying a member like "robot@x.com" (no type prefix) or an
// unknown prefix. Typed members ("user:", "serviceAccount:", "group:",
// "domain:", "principal:", "principalSet:") must carry a non-empty identifier;
// "allUsers" and "allAuthenticatedUsers" are the only bare (untyped) members.
func validateIAMMembers(bindings []IAMBinding) error {
	typedPrefixes := []string{"user:", "serviceAccount:", "group:", "domain:", "principal:", "principalSet:"}
	for _, b := range bindings {
		for _, m := range b.Members {
			if m == "allUsers" || m == "allAuthenticatedUsers" {
				continue
			}
			matched := false
			for _, p := range typedPrefixes {
				if strings.HasPrefix(m, p) {
					id := strings.TrimPrefix(m, p)
					if id == "" {
						return fmt.Errorf("invalid member: %s", m)
					}
					// user:/serviceAccount:/group:/domain: carry an email or
					// domain; principal:/principalSet: carry an IAM resource
					// path. Require a structurally-plausible identifier for the
					// email/domain forms (a dot, e.g. "@example.com" or
					// "example.com") so a bare token like "user:bob" is rejected
					// as real GCP rejects it.
					switch p {
					case "user:", "serviceAccount:", "group:", "domain:":
						if !strings.Contains(id, ".") {
							return fmt.Errorf("invalid member: %s", m)
						}
					}
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("invalid member: %s", m)
			}
		}
	}
	return nil
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
		if err := validateIAMMembers(req.Policy.Bindings); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%v", err)
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
		// admin, so the full echo is the truthful response. A real-subset
		// evaluation against an authz model is a staged epic; the
		// admin-echo behavior is intentionally unchanged here.
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

// gcpPredefinedRole is a curated (Google-managed) IAM role as returned by
// roles.get / roles.list.
type gcpPredefinedRole struct {
	Name                string
	Title               string
	Description         string
	IncludedPermissions []string
}

// gcpPredefinedRoles returns the bounded representative set of predefined roles
// the sim serves. Etag is deterministic per-role (these roles are immutable),
// so a roles.get round-trips a stable etag. This is intentionally a handful of
// roles, not the full GCP catalog.
func gcpPredefinedRoles() []gcpPredefinedRole {
	return []gcpPredefinedRole{
		{
			Name:        "roles/viewer",
			Title:       "Viewer",
			Description: "Read access to all resources.",
			IncludedPermissions: []string{
				"resourcemanager.projects.get",
				"storage.buckets.get",
				"storage.objects.get",
				"storage.objects.list",
			},
		},
		{
			Name:        "roles/editor",
			Title:       "Editor",
			Description: "Edit access to all resources.",
			IncludedPermissions: []string{
				"resourcemanager.projects.get",
				"storage.buckets.get",
				"storage.buckets.update",
				"storage.objects.create",
				"storage.objects.delete",
				"storage.objects.get",
				"storage.objects.list",
			},
		},
		{
			Name:        "roles/owner",
			Title:       "Owner",
			Description: "Full access to all resources.",
			IncludedPermissions: []string{
				"resourcemanager.projects.get",
				"resourcemanager.projects.setIamPolicy",
				"iam.serviceAccounts.create",
				"iam.serviceAccounts.delete",
				"storage.buckets.setIamPolicy",
			},
		},
		{
			Name:        "roles/iam.serviceAccountUser",
			Title:       "Service Account User",
			Description: "Run operations as the service account.",
			IncludedPermissions: []string{
				"iam.serviceAccounts.actAs",
				"iam.serviceAccounts.get",
				"iam.serviceAccounts.list",
			},
		},
		{
			Name:        "roles/storage.objectViewer",
			Title:       "Storage Object Viewer",
			Description: "Read access to GCS objects.",
			IncludedPermissions: []string{
				"storage.objects.get",
				"storage.objects.list",
			},
		},
	}
}

// gcpRoleJSON renders a predefined role into the roles.get / roles.list wire
// shape. includedPermissions is omitted unless full (roles.list defaults to
// BASIC view, which carries no permissions; roles.get returns FULL).
func gcpRoleJSON(role gcpPredefinedRole, full bool) map[string]any {
	out := map[string]any{
		"name":        role.Name,
		"title":       role.Title,
		"description": role.Description,
		"stage":       "GA",
		// Predefined roles are immutable; a deterministic etag keeps
		// roles.get idempotent across reads.
		"etag": base64.StdEncoding.EncodeToString([]byte(role.Name)),
	}
	if full {
		out["includedPermissions"] = role.IncludedPermissions
	}
	return out
}

// gcpNumericID returns a random decimal string of the given length, matching
// the shape of GCP's service-account uniqueId / client_id (a ~21-digit numeric
// principal identifier, not a hex UUID). The first digit is 1-9 so the value
// is a full-length number with no leading zero.
func gcpNumericID(digits int) string {
	if digits <= 0 {
		digits = 21
	}
	b := make([]byte, digits)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand on a healthy host does not fail; if it does the
		// process is in an unrecoverable state. Panic rather than emit a
		// predictable or zero-valued identifier.
		panic(fmt.Sprintf("gcpNumericID: read random: %v", err))
	}
	out := make([]byte, digits)
	out[0] = byte('1' + int(b[0])%9)
	for i := 1; i < digits; i++ {
		out[i] = byte('0' + int(b[i])%10)
	}
	return string(out)
}

// gcpMakeSAKeyJSON generates a real RSA-2048 key pair and returns it encoded
// as a base64 GCP service-account JSON credential file — matching the exact
// shape real GCP returns for CreateServiceAccountKey.
func gcpMakeSAKeyJSON(project, keyID, email, clientID, validAfter, validBefore string) (string, error) {
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
		"client_id":                   clientID,
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

// iamLROs holds the long-running operations the IAM admin surface returns for
// its mutating verbs (pool/provider/oauth-client create/patch/delete/undelete).
// Each operation's GET endpoint lives under the resource path that produced it
// (e.g. .../workloadIdentityPools/{p}/operations/{op}), so the IAM operations
// are kept in their own store keyed by the full operation resource name.
var iamLROs sim.Store[Operation]

// iamResources is the generic store for every IAM admin resource the sim
// models as a JSON map (workload identity pools + their providers / namespaces
// / managed identities / keys, workforce pools + their providers / subjects /
// scim tenants / tokens / keys, and OAuth clients + credentials). Each row is
// the resource's wire JSON keyed by its fully-qualified resource name; soft-
// deleted resources keep their row (state=DELETED) so undelete can revive them.
var iamResources sim.Store[map[string]any]

// newIAMLRO settles a long-running operation synchronously (done=true) with the
// resource embedded as the protobuf Any response, stores it under
// {opCollection}/operations/{id}, and returns it. opCollection is the resource
// path the operation hangs off (e.g. the pool name), matching real GCP where a
// pool operation's name is .../workloadIdentityPools/{p}/operations/{id}.
func newIAMLRO(opCollection string, resource map[string]any, typeName string) Operation {
	if iamLROs == nil {
		// The operations store is created in registerIAM; a nil here means a
		// handler ran before registration, which never happens at runtime.
		panic("newIAMLRO: iamLROs store not initialized")
	}
	opID := generateUUID()
	response := map[string]any{"@type": typeName}
	for k, v := range resource {
		response[k] = v
	}
	var target string
	if n, ok := resource["name"].(string); ok {
		target = n
	}
	op := Operation{
		Name: opCollection + "/operations/" + opID,
		Metadata: map[string]any{
			"@type":      "type.googleapis.com/google.iam.admin.v1.OperationMetadata",
			"createTime": nowTimestamp(),
			"target":     target,
		},
		Done:     true,
		Response: response,
	}
	iamLROs.Put(op.Name, op)
	return op
}

// iamApplyMask copies the masked fields from src into dst. An empty mask copies
// every field present in src (real GCP treats a missing updateMask on these
// resources as a full update of the provided fields).
func iamApplyMask(dst, src map[string]any, mask string) {
	if strings.TrimSpace(mask) == "" {
		for k, v := range src {
			if k == "name" || k == "state" {
				continue
			}
			dst[k] = v
		}
		return
	}
	for _, f := range strings.Split(mask, ",") {
		f = strings.TrimSpace(f)
		if f == "" || f == "name" || f == "state" {
			continue
		}
		if v, ok := src[f]; ok {
			dst[f] = v
		} else {
			delete(dst, f)
		}
	}
}

// iamGenericCRUD wires the standard list/get/create/patch/delete/undelete +
// per-resource operations.get surface for one IAM admin collection. It is the
// shared engine behind workload-identity pools (and their nested providers /
// namespaces / managed identities / keys) and workforce pools (and their
// providers / subjects / scim tenants / tokens / keys).
//
//   - parentPat captures the collection's parent in the {parent...} param, so a
//     POST/GET-list resolves the parent name; createParam is the query
//     parameter carrying the new resource ID (e.g. workloadIdentityPoolId).
//   - childField is the list-response array field name (e.g.
//     "workloadIdentityPools").
//   - resType is the protobuf Any @type for this resource's LRO response.
//   - mutatingLRO controls whether create/patch/delete return an Operation
//     (true for pools/providers/identities) or the resource/Empty directly
//     (true for every IAM admin collection here; oauth clients differ and are
//     handled separately).
type iamCollection struct {
	srv         *sim.Server
	collPath    string // e.g. "/v1/projects/{project}/locations/{location}/workloadIdentityPools"
	parentName  func(r *http.Request) string
	childField  string
	createParam string
	resType     string
	stateField  bool // emit state=ACTIVE / DELETED
}

func (c iamCollection) name(r *http.Request, id string) string {
	return c.parentName(r) + "/" + lastCollSegment(c.collPath) + "/" + id
}

// lastCollSegment returns the trailing literal collection segment of a route
// pattern (e.g. "workloadIdentityPools" from ".../workloadIdentityPools").
func lastCollSegment(pat string) string {
	seg := pat[strings.LastIndex(pat, "/")+1:]
	return seg
}

func (c iamCollection) register() {
	// List
	c.srv.HandleFunc("GET "+c.collPath, func(w http.ResponseWriter, r *http.Request) {
		prefix := c.parentName(r) + "/" + lastCollSegment(c.collPath) + "/"
		showDeleted := r.URL.Query().Get("showDeleted") == "true"
		rows := iamResources.Filter(func(m map[string]any) bool {
			name, _ := m["name"].(string)
			if !strings.HasPrefix(name, prefix) {
				return false
			}
			// only direct children (no extra "/" after the id)
			if strings.Contains(strings.TrimPrefix(name, prefix), "/") {
				return false
			}
			if !showDeleted && m["state"] == "DELETED" {
				return false
			}
			return true
		})
		sort.Slice(rows, func(i, j int) bool {
			ni, _ := rows[i]["name"].(string)
			nj, _ := rows[j]["name"].(string)
			return ni < nj
		})
		out := make([]map[string]any, 0, len(rows))
		out = append(out, rows...)
		sim.WriteJSON(w, http.StatusOK, map[string]any{c.childField: out})
	})

	// Get / listAttestationRules (colon-verb fan-in on the same path)
	c.srv.HandleFunc("GET "+c.collPath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := sim.PathParam(r, "id")
		if base, verb, found := strings.Cut(id, ":"); found {
			if verb == "listAttestationRules" {
				name := c.name(r, base)
				if _, ok := iamResources.Get(name); !ok {
					sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
					return
				}
				sim.WriteJSON(w, http.StatusOK, map[string]any{"attestationRules": []any{}})
				return
			}
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unsupported verb %q", verb)
			return
		}
		name := c.name(r, id)
		m, ok := iamResources.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, m)
	})

	// Create (returns an LRO) / colon-verb POSTs (undelete, attestation rules)
	c.srv.HandleFunc("POST "+c.collPath, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get(c.createParam)
		if id == "" {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "%s is required", c.createParam)
			return
		}
		var body map[string]any
		if err := sim.ReadJSON(r, &body); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		name := c.name(r, id)
		if _, exists := iamResources.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "%s already exists", name)
			return
		}
		res := map[string]any{}
		for k, v := range body {
			res[k] = v
		}
		res["name"] = name
		if c.stateField {
			res["state"] = "ACTIVE"
		}
		iamResources.Put(name, res)
		op := newIAMLRO(name, res, c.resType)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Colon-verb POSTs on a specific resource: undelete (revives a soft-
	// deleted resource) and the attestation-rule mutators. All return an LRO.
	c.srv.HandleFunc("POST "+c.collPath+"/{idAction}", func(w http.ResponseWriter, r *http.Request) {
		idAction := sim.PathParam(r, "idAction")
		id, verb, found := strings.Cut(idAction, ":")
		if !found {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unsupported request %q", idAction)
			return
		}
		name := c.name(r, id)
		switch verb {
		case "undelete":
			res, ok := iamResources.Get(name)
			if !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
				return
			}
			if c.stateField {
				res["state"] = "ACTIVE"
				iamResources.Put(name, res)
			}
			op := newIAMLRO(name, res, c.resType)
			sim.WriteJSON(w, http.StatusOK, op)
		case "addAttestationRule", "removeAttestationRule", "setAttestationRules":
			res, ok := iamResources.Get(name)
			if !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
				return
			}
			op := newIAMLRO(name, res, c.resType)
			sim.WriteJSON(w, http.StatusOK, op)
		case "getIamPolicy", "setIamPolicy", "testIamPermissions":
			// The pool/provider is itself an IAM resource; reuse the shared
			// resource-IAM store so its policy round-trips like any other.
			handleResourceIAM(w, r, gcpResourcePolicies, name, verb)
		default:
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unsupported verb %q", verb)
		}
	})

	// Patch (returns an LRO)
	c.srv.HandleFunc("PATCH "+c.collPath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := c.name(r, sim.PathParam(r, "id"))
		res, ok := iamResources.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
			return
		}
		var body map[string]any
		if err := sim.ReadJSON(r, &body); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		iamApplyMask(res, body, r.URL.Query().Get("updateMask"))
		iamResources.Put(name, res)
		op := newIAMLRO(name, res, c.resType)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Delete — soft-delete (state=DELETED) so undelete can revive it. Returns
	// an LRO whose embedded response is the deleted resource.
	c.srv.HandleFunc("DELETE "+c.collPath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		name := c.name(r, sim.PathParam(r, "id"))
		res, ok := iamResources.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
			return
		}
		if c.stateField {
			res["state"] = "DELETED"
			iamResources.Put(name, res)
		} else {
			iamResources.Delete(name)
		}
		op := newIAMLRO(name, res, c.resType)
		sim.WriteJSON(w, http.StatusOK, op)
	})

	// Per-resource operations.get
	c.srv.HandleFunc("GET "+c.collPath+"/{id}/operations/{op}", func(w http.ResponseWriter, r *http.Request) {
		opName := c.name(r, sim.PathParam(r, "id")) + "/operations/" + sim.PathParam(r, "op")
		serveIAMOperation(w, opName)
	})
}

// serveIAMOperation returns a stored IAM LRO by name (or a settled, done=true
// operation if the name is well-formed but no record exists — real GCP keeps
// completed operations queryable; the sim settles synchronously so any
// operation it minted is in the store).
func serveIAMOperation(w http.ResponseWriter, opName string) {
	if op, ok := iamLROs.Get(opName); ok {
		sim.WriteJSON(w, http.StatusOK, op)
		return
	}
	sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "operation %q not found", opName)
}

// registerWorkloadIdentityPools mounts the project-scoped Workload Identity
// Federation surface: pools, providers, namespaces, managed identities,
// provider keys, and the operations.get endpoints at every level.
func registerWorkloadIdentityPools(srv *sim.Server) {
	const base = "/v1/projects/{project}/locations/{location}"
	wipParent := func(r *http.Request) string {
		return fmt.Sprintf("projects/%s/locations/%s", sim.PathParam(r, "project"), sim.PathParam(r, "location"))
	}

	// Pools.
	iamCollection{
		srv: srv, collPath: base + "/workloadIdentityPools",
		parentName:  wipParent,
		childField:  "workloadIdentityPools",
		createParam: "workloadIdentityPoolId",
		resType:     "type.googleapis.com/google.iam.v1.WorkloadIdentityPool",
		stateField:  true,
	}.register()

	// Providers.
	iamCollection{
		srv: srv, collPath: base + "/workloadIdentityPools/{pool}/providers",
		parentName: func(r *http.Request) string {
			return wipParent(r) + "/workloadIdentityPools/" + sim.PathParam(r, "pool")
		},
		childField:  "workloadIdentityPoolProviders",
		createParam: "workloadIdentityPoolProviderId",
		resType:     "type.googleapis.com/google.iam.v1.WorkloadIdentityPoolProvider",
		stateField:  true,
	}.register()

	// Provider keys.
	iamCollection{
		srv: srv, collPath: base + "/workloadIdentityPools/{pool}/providers/{provider}/keys",
		parentName: func(r *http.Request) string {
			return wipParent(r) + "/workloadIdentityPools/" + sim.PathParam(r, "pool") + "/providers/" + sim.PathParam(r, "provider")
		},
		childField:  "workloadIdentityPoolProviderKeys",
		createParam: "workloadIdentityPoolProviderKeyId",
		resType:     "type.googleapis.com/google.iam.v1.WorkloadIdentityPoolProviderKey",
		stateField:  true,
	}.register()

	// Namespaces.
	iamCollection{
		srv: srv, collPath: base + "/workloadIdentityPools/{pool}/namespaces",
		parentName: func(r *http.Request) string {
			return wipParent(r) + "/workloadIdentityPools/" + sim.PathParam(r, "pool")
		},
		childField:  "workloadIdentityPoolNamespaces",
		createParam: "workloadIdentityPoolNamespaceId",
		resType:     "type.googleapis.com/google.iam.v1.WorkloadIdentityPoolNamespace",
		stateField:  true,
	}.register()

	// Managed identities.
	iamCollection{
		srv: srv, collPath: base + "/workloadIdentityPools/{pool}/namespaces/{namespace}/managedIdentities",
		parentName: func(r *http.Request) string {
			return wipParent(r) + "/workloadIdentityPools/" + sim.PathParam(r, "pool") + "/namespaces/" + sim.PathParam(r, "namespace")
		},
		childField:  "workloadIdentityPoolManagedIdentities",
		createParam: "workloadIdentityPoolManagedIdentityId",
		resType:     "type.googleapis.com/google.iam.v1.WorkloadIdentityPoolManagedIdentity",
		stateField:  true,
	}.register()

	// Workload-source operations.get — the only operations endpoint the
	// generic collection doesn't already mint (workloadSources has no CRUD in
	// the doc, just a nested operations.get).
	srv.HandleFunc("GET "+base+"/workloadIdentityPools/{pool}/namespaces/{namespace}/managedIdentities/{mi}/workloadSources/{ws}/operations/{op}", func(w http.ResponseWriter, r *http.Request) {
		serveIAMOperation(w, fmt.Sprintf("projects/%s/locations/%s/workloadIdentityPools/%s/namespaces/%s/managedIdentities/%s/workloadSources/%s/operations/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "pool"), sim.PathParam(r, "namespace"), sim.PathParam(r, "mi"), sim.PathParam(r, "ws"), sim.PathParam(r, "op")))
	})

	// managedIdentities :listAttestationRules — colon-verb fan-in handled by
	// the generic Get handler already (the {id} param captures "id:verb").
}

// registerWorkforcePools mounts the location-scoped (organization-level)
// workforce-pool surface: pools, providers, subjects, scim tenants, scim
// tokens, provider keys, and operations.get at the relevant levels.
func registerWorkforcePools(srv *sim.Server) {
	const base = "/v1/locations/{location}"
	wfpParent := func(r *http.Request) string {
		return "locations/" + sim.PathParam(r, "location")
	}

	// Pools.
	iamCollection{
		srv: srv, collPath: base + "/workforcePools",
		parentName:  wfpParent,
		childField:  "workforcePools",
		createParam: "workforcePoolId",
		resType:     "type.googleapis.com/google.iam.v1.WorkforcePool",
		stateField:  true,
	}.register()

	// Providers.
	iamCollection{
		srv: srv, collPath: base + "/workforcePools/{pool}/providers",
		parentName: func(r *http.Request) string {
			return wfpParent(r) + "/workforcePools/" + sim.PathParam(r, "pool")
		},
		childField:  "workforcePoolProviders",
		createParam: "workforcePoolProviderId",
		resType:     "type.googleapis.com/google.iam.v1.WorkforcePoolProvider",
		stateField:  true,
	}.register()

	// Provider keys.
	iamCollection{
		srv: srv, collPath: base + "/workforcePools/{pool}/providers/{provider}/keys",
		parentName: func(r *http.Request) string {
			return wfpParent(r) + "/workforcePools/" + sim.PathParam(r, "pool") + "/providers/" + sim.PathParam(r, "provider")
		},
		childField:  "workforcePoolProviderKeys",
		createParam: "workforcePoolProviderKeyId",
		resType:     "type.googleapis.com/google.iam.v1.WorkforcePoolProviderKey",
		stateField:  true,
	}.register()

	// SCIM tenants.
	iamCollection{
		srv: srv, collPath: base + "/workforcePools/{pool}/providers/{provider}/scimTenants",
		parentName: func(r *http.Request) string {
			return wfpParent(r) + "/workforcePools/" + sim.PathParam(r, "pool") + "/providers/" + sim.PathParam(r, "provider")
		},
		childField:  "workforcePoolProviderScimTenants",
		createParam: "workforcePoolProviderScimTenantId",
		resType:     "type.googleapis.com/google.iam.v1.WorkforcePoolProviderScimTenant",
		stateField:  true,
	}.register()

	// SCIM tokens.
	iamCollection{
		srv: srv, collPath: base + "/workforcePools/{pool}/providers/{provider}/scimTenants/{tenant}/tokens",
		parentName: func(r *http.Request) string {
			return wfpParent(r) + "/workforcePools/" + sim.PathParam(r, "pool") + "/providers/" + sim.PathParam(r, "provider") + "/scimTenants/" + sim.PathParam(r, "tenant")
		},
		childField:  "workforcePoolProviderScimTokens",
		createParam: "workforcePoolProviderScimTokenId",
		resType:     "type.googleapis.com/google.iam.v1.WorkforcePoolProviderScimToken",
		stateField:  true,
	}.register()

	// Subjects — only delete (soft) + undelete + operations.get in the doc.
	srv.HandleFunc("DELETE "+base+"/workforcePools/{pool}/subjects/{subject}", func(w http.ResponseWriter, r *http.Request) {
		name := fmt.Sprintf("locations/%s/workforcePools/%s/subjects/%s",
			sim.PathParam(r, "location"), sim.PathParam(r, "pool"), sim.PathParam(r, "subject"))
		res := map[string]any{"name": name, "state": "DELETED"}
		iamResources.Put(name, res)
		op := newIAMLRO(name, res, "type.googleapis.com/google.iam.v1.WorkforcePoolSubject")
		sim.WriteJSON(w, http.StatusOK, op)
	})
	srv.HandleFunc("GET "+base+"/workforcePools/{pool}/subjects/{subject}/operations/{op}", func(w http.ResponseWriter, r *http.Request) {
		serveIAMOperation(w, fmt.Sprintf("locations/%s/workforcePools/%s/subjects/%s/operations/%s",
			sim.PathParam(r, "location"), sim.PathParam(r, "pool"), sim.PathParam(r, "subject"), sim.PathParam(r, "op")))
	})
	// Pool-, provider-, and key-level operations.get are minted by the generic
	// collection registrations above (their per-resource operations.get).
}

// registerOAuthClients mounts the project-scoped OAuth-client surface. Unlike
// pools, oauthClients return the resource directly (not an Operation) on
// create/patch; delete returns the soft-deleted resource and credential delete
// returns Empty. Credentials carry a clientSecret only on the resource itself.
func registerOAuthClients(srv *sim.Server) {
	const base = "/v1/projects/{project}/locations/{location}/oauthClients"
	clientName := func(r *http.Request) string {
		return fmt.Sprintf("projects/%s/locations/%s/oauthClients/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "client"))
	}

	// List clients.
	srv.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		prefix := fmt.Sprintf("projects/%s/locations/%s/oauthClients/", sim.PathParam(r, "project"), sim.PathParam(r, "location"))
		rows := iamResources.Filter(func(m map[string]any) bool {
			name, _ := m["name"].(string)
			return strings.HasPrefix(name, prefix) && !strings.Contains(strings.TrimPrefix(name, prefix), "/")
		})
		sort.Slice(rows, func(i, j int) bool {
			ni, _ := rows[i]["name"].(string)
			nj, _ := rows[j]["name"].(string)
			return ni < nj
		})
		out := make([]map[string]any, 0, len(rows))
		out = append(out, rows...)
		sim.WriteJSON(w, http.StatusOK, map[string]any{"oauthClients": out})
	})

	// Get client.
	srv.HandleFunc("GET "+base+"/{client}", func(w http.ResponseWriter, r *http.Request) {
		m, ok := iamResources.Get(clientName(r))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", clientName(r))
			return
		}
		sim.WriteJSON(w, http.StatusOK, m)
	})

	// Create client — returns the resource directly.
	srv.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("oauthClientId")
		if id == "" {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "oauthClientId is required")
			return
		}
		var body map[string]any
		if err := sim.ReadJSON(r, &body); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		name := fmt.Sprintf("projects/%s/locations/%s/oauthClients/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "location"), id)
		if _, exists := iamResources.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "%s already exists", name)
			return
		}
		res := map[string]any{}
		for k, v := range body {
			res[k] = v
		}
		res["name"] = name
		res["clientId"] = id
		res["state"] = "ACTIVE"
		iamResources.Put(name, res)
		sim.WriteJSON(w, http.StatusOK, res)
	})

	// Undelete client — POST .../oauthClients/{client}:undelete. Returns the
	// revived resource directly (oauthClients are not LRO-based).
	srv.HandleFunc("POST "+base+"/{clientAction}", func(w http.ResponseWriter, r *http.Request) {
		clientAction := sim.PathParam(r, "clientAction")
		id, verb, found := strings.Cut(clientAction, ":")
		if !found || verb != "undelete" {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unsupported request %q", clientAction)
			return
		}
		name := fmt.Sprintf("projects/%s/locations/%s/oauthClients/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "location"), id)
		res, ok := iamResources.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", name)
			return
		}
		res["state"] = "ACTIVE"
		iamResources.Put(name, res)
		sim.WriteJSON(w, http.StatusOK, res)
	})

	// Patch client.
	srv.HandleFunc("PATCH "+base+"/{client}", func(w http.ResponseWriter, r *http.Request) {
		res, ok := iamResources.Get(clientName(r))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", clientName(r))
			return
		}
		var body map[string]any
		if err := sim.ReadJSON(r, &body); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		iamApplyMask(res, body, r.URL.Query().Get("updateMask"))
		iamResources.Put(clientName(r), res)
		sim.WriteJSON(w, http.StatusOK, res)
	})

	// Delete client — soft-delete, returns the resource.
	srv.HandleFunc("DELETE "+base+"/{client}", func(w http.ResponseWriter, r *http.Request) {
		res, ok := iamResources.Get(clientName(r))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", clientName(r))
			return
		}
		res["state"] = "DELETED"
		iamResources.Put(clientName(r), res)
		sim.WriteJSON(w, http.StatusOK, res)
	})

	// Credentials — list / get / create / patch / delete (Empty).
	credName := func(r *http.Request) string {
		return clientName(r) + "/credentials/" + sim.PathParam(r, "cred")
	}
	srv.HandleFunc("GET "+base+"/{client}/credentials", func(w http.ResponseWriter, r *http.Request) {
		prefix := clientName(r) + "/credentials/"
		rows := iamResources.Filter(func(m map[string]any) bool {
			name, _ := m["name"].(string)
			return strings.HasPrefix(name, prefix)
		})
		sort.Slice(rows, func(i, j int) bool {
			ni, _ := rows[i]["name"].(string)
			nj, _ := rows[j]["name"].(string)
			return ni < nj
		})
		out := make([]map[string]any, 0, len(rows))
		out = append(out, rows...)
		sim.WriteJSON(w, http.StatusOK, map[string]any{"oauthClientCredentials": out})
	})
	srv.HandleFunc("GET "+base+"/{client}/credentials/{cred}", func(w http.ResponseWriter, r *http.Request) {
		m, ok := iamResources.Get(credName(r))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", credName(r))
			return
		}
		sim.WriteJSON(w, http.StatusOK, m)
	})
	srv.HandleFunc("POST "+base+"/{client}/credentials", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("oauthClientCredentialId")
		if id == "" {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "oauthClientCredentialId is required")
			return
		}
		var body map[string]any
		if err := sim.ReadJSON(r, &body); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		name := clientName(r) + "/credentials/" + id
		if _, exists := iamResources.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "%s already exists", name)
			return
		}
		res := map[string]any{}
		for k, v := range body {
			res[k] = v
		}
		res["name"] = name
		res["clientSecret"] = "sim-secret-" + generateUUID()
		iamResources.Put(name, res)
		sim.WriteJSON(w, http.StatusOK, res)
	})
	srv.HandleFunc("PATCH "+base+"/{client}/credentials/{cred}", func(w http.ResponseWriter, r *http.Request) {
		res, ok := iamResources.Get(credName(r))
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", credName(r))
			return
		}
		var body map[string]any
		if err := sim.ReadJSON(r, &body); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		iamApplyMask(res, body, r.URL.Query().Get("updateMask"))
		iamResources.Put(credName(r), res)
		sim.WriteJSON(w, http.StatusOK, res)
	})
	srv.HandleFunc("DELETE "+base+"/{client}/credentials/{cred}", func(w http.ResponseWriter, r *http.Request) {
		if !iamResources.Delete(credName(r)) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s not found", credName(r))
			return
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})
}
