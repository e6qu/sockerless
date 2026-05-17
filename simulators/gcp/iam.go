package main

import (
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
	Bindings []IAMBinding `json:"bindings"`
	Etag     string       `json:"etag"`
	Version  int          `json:"version"`
}

type IAMBinding struct {
	Role    string   `json:"role"`
	Members []string `json:"members"`
}

// gcpResourcePolicies is the shared IAM policy store for GCP resources
// (artifact registry, storage buckets, etc.). It's package-level so that
// resource-specific handlers can process :getIamPolicy / :setIamPolicy requests.
var gcpResourcePolicies sim.Store[IAMPolicy]

func registerIAM(srv *sim.Server) {
	serviceAccounts := sim.MakeStore[GCPServiceAccount](srv.DB(), "iam_service_accounts")
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
					Etag:     generateUUID()[:8],
					Version:  1,
				}
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

			req.Policy.Etag = generateUUID()[:8]
			if req.Policy.Version == 0 {
				req.Policy.Version = 1
			}
			projectPolicies.Put(project, req.Policy)
			sim.WriteJSON(w, http.StatusOK, req.Policy)
		default:
			http.NotFound(w, r)
		}
	})

	// Resource IAM (for artifact registry, etc.) - getIamPolicy / setIamPolicy
	srv.HandleFunc("POST /v1/{resource...}", func(w http.ResponseWriter, r *http.Request) {
		resource := sim.PathParam(r, "resource")

		if strings.HasSuffix(resource, ":getIamPolicy") {
			resource = strings.TrimSuffix(resource, ":getIamPolicy")
			policy, ok := resourcePolicies.Get(resource)
			if !ok {
				policy = IAMPolicy{
					Bindings: []IAMBinding{},
					Etag:     generateUUID()[:8],
					Version:  1,
				}
			}
			sim.WriteJSON(w, http.StatusOK, policy)
		} else if strings.HasSuffix(resource, ":setIamPolicy") {
			resource = strings.TrimSuffix(resource, ":setIamPolicy")
			var req struct {
				Policy IAMPolicy `json:"policy"`
			}
			if err := sim.ReadJSON(r, &req); err != nil {
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
				return
			}

			req.Policy.Etag = generateUUID()[:8]
			if req.Policy.Version == 0 {
				req.Policy.Version = 1
			}
			resourcePolicies.Put(resource, req.Policy)
			sim.WriteJSON(w, http.StatusOK, req.Policy)
		} else {
			http.NotFound(w, r)
		}
	})

	// Bucket IAM - getIamPolicy
	srv.HandleFunc("GET /storage/v1/b/{bucket}/iam", func(w http.ResponseWriter, r *http.Request) {
		bucket := sim.PathParam(r, "bucket")

		policy, ok := resourcePolicies.Get("bucket/" + bucket)
		if !ok {
			policy = IAMPolicy{
				Bindings: []IAMBinding{},
				Etag:     generateUUID()[:8],
				Version:  1,
			}
		}
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

		policy.Etag = generateUUID()[:8]
		if policy.Version == 0 {
			policy.Version = 1
		}
		resourcePolicies.Put("bucket/"+bucket, policy)

		sim.WriteJSON(w, http.StatusOK, policy)
	})
}

// handleResourceIAM processes :getIamPolicy and :setIamPolicy for a named resource.
func handleResourceIAM(w http.ResponseWriter, r *http.Request, store sim.Store[IAMPolicy], resource, action string) {
	switch action {
	case "getIamPolicy":
		policy, ok := store.Get(resource)
		if !ok {
			policy = IAMPolicy{
				Bindings: []IAMBinding{},
				Etag:     generateUUID()[:8],
				Version:  1,
			}
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
		req.Policy.Etag = generateUUID()[:8]
		if req.Policy.Version == 0 {
			req.Policy.Version = 1
		}
		store.Put(resource, req.Policy)
		sim.WriteJSON(w, http.StatusOK, req.Policy)
	default:
		http.NotFound(w, r)
	}
}
