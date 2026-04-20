package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Secret Manager v1 slice. Sockerless's GCP Cloud Build integration
// references secret versions via `availableSecrets.secretManager
// [].versionName`; the simulator must return the secret payload so
// Cloud Build can expand them into env vars before executing the build
// step. Real API: https://cloud.google.com/secret-manager/docs/reference/rest

// Secret represents a Cloud Secret Manager secret resource.
type Secret struct {
	Name       string            `json:"name"`
	CreateTime string            `json:"createTime"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// SecretVersion represents a single version of a secret. The real
// API stores versions as enabled / disabled / destroyed; for this
// simulator we keep only the enabled payload.
type SecretVersion struct {
	Name       string `json:"name"`
	CreateTime string `json:"createTime"`
	State      string `json:"state"`
	payload    []byte // unexported raw payload bytes
}

// Package-level stores so cloudbuild.go can resolve secret versions
// during build-step env expansion.
var (
	smSecrets        sim.Store[Secret]
	smSecretVersions sim.Store[SecretVersion]
)

func registerSecretManager(srv *sim.Server) {
	smSecrets = sim.MakeStore[Secret](srv.DB(), "sm_secrets")
	smSecretVersions = sim.MakeStore[SecretVersion](srv.DB(), "sm_secret_versions")

	// CreateSecret: POST /v1/projects/{project}/secrets?secretId=X
	srv.HandleFunc("POST /v1/projects/{project}/secrets", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		secretID := r.URL.Query().Get("secretId")
		if secretID == "" {
			sim.GCPError(w, http.StatusBadRequest, "secretId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var req struct {
			Labels map[string]string `json:"labels,omitempty"`
		}
		_ = sim.ReadJSON(r, &req)

		name := fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
		if _, exists := smSecrets.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "secret %s already exists", secretID)
			return
		}

		secret := Secret{
			Name:       name,
			CreateTime: time.Now().UTC().Format(time.RFC3339),
			Labels:     req.Labels,
		}
		smSecrets.Put(name, secret)
		sim.WriteJSON(w, http.StatusOK, secret)
	})

	// GetSecret: GET /v1/projects/{project}/secrets/{secret}
	srv.HandleFunc("GET /v1/projects/{project}/secrets/{secret}", func(w http.ResponseWriter, r *http.Request) {
		name := fmt.Sprintf("projects/%s/secrets/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "secret"))
		secret, ok := smSecrets.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret %s not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, secret)
	})

	// AddSecretVersion: POST /v1/projects/{project}/secrets/{secret}:addVersion.
	// Go's ServeMux doesn't allow `{wild}:suffix` — register a generic
	// POST /secrets/{secretAction} handler and parse the colon suffix.
	srv.HandleFunc("POST /v1/projects/{project}/secrets/{secretAction}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		secretAction := sim.PathParam(r, "secretAction")
		secretID, action, found := strings.Cut(secretAction, ":")
		if !found || action != "addVersion" {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown secret action %q", secretAction)
			return
		}
		secretName := fmt.Sprintf("projects/%s/secrets/%s", project, secretID)

		if _, ok := smSecrets.Get(secretName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret %s not found", secretName)
			return
		}

		var req struct {
			Payload struct {
				Data string `json:"data"` // base64-encoded
			} `json:"payload"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(req.Payload.Data)
		if err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "payload.data must be base64: %v", err)
			return
		}

		// Version IDs are monotonically increasing; count existing
		// versions of this secret to pick the next.
		var n int
		for _, v := range smSecretVersions.List() {
			if strings.HasPrefix(v.Name, secretName+"/versions/") {
				n++
			}
		}
		versionID := fmt.Sprintf("%d", n+1)
		versionName := fmt.Sprintf("%s/versions/%s", secretName, versionID)
		ver := SecretVersion{
			Name:       versionName,
			CreateTime: time.Now().UTC().Format(time.RFC3339),
			State:      "ENABLED",
			payload:    raw,
		}
		smSecretVersions.Put(versionName, ver)
		sim.WriteJSON(w, http.StatusOK, ver)
	})

	// AccessSecretVersion: GET /v1/projects/{project}/secrets/{secret}/versions/{version}:access.
	// Same Go mux workaround as AddSecretVersion.
	srv.HandleFunc("GET /v1/projects/{project}/secrets/{secret}/versions/{versionAction}",
		func(w http.ResponseWriter, r *http.Request) {
			project := sim.PathParam(r, "project")
			secretID := sim.PathParam(r, "secret")
			versionAction := sim.PathParam(r, "versionAction")
			versionID, action, found := strings.Cut(versionAction, ":")
			if !found || action != "access" {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown version action %q", versionAction)
				return
			}
			payload, err := accessSecretPayload(project, secretID, versionID)
			if err != nil {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s", err.Error())
				return
			}
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"name": fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secretID, versionID),
				"payload": map[string]string{
					"data": base64.StdEncoding.EncodeToString(payload),
				},
			})
		})
}

// accessSecretPayload resolves a secret-version reference to its raw
// payload. Handles both explicit versions (e.g. "3") and the special
// "latest" alias. Exported for cloudbuild.go's build-step secretEnv
// expansion.
func accessSecretPayload(project, secretID, version string) ([]byte, error) {
	secretName := fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
	if version == "latest" {
		// Pick the highest-numbered version for this secret.
		var latestN int
		var latestPayload []byte
		for _, v := range smSecretVersions.List() {
			if !strings.HasPrefix(v.Name, secretName+"/versions/") {
				continue
			}
			idStr := strings.TrimPrefix(v.Name, secretName+"/versions/")
			var n int
			_, _ = fmt.Sscanf(idStr, "%d", &n)
			if n > latestN {
				latestN = n
				latestPayload = v.payload
			}
		}
		if latestN == 0 {
			return nil, fmt.Errorf("no enabled versions for secret %s", secretName)
		}
		return latestPayload, nil
	}

	versionName := fmt.Sprintf("%s/versions/%s", secretName, version)
	ver, ok := smSecretVersions.Get(versionName)
	if !ok {
		return nil, fmt.Errorf("secret version %s not found", versionName)
	}
	return ver.payload, nil
}

// resolveSecretManagerReference parses a `projects/{p}/secrets/{s}/versions/{v}`
// reference (as used in Cloud Build's availableSecrets.secretManager[].versionName)
// and returns the resolved payload. Returns an error if the reference is
// malformed or the version doesn't exist.
func resolveSecretManagerReference(ref string) ([]byte, error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "secrets" || parts[4] != "versions" {
		return nil, fmt.Errorf("invalid secret reference %q; expected projects/P/secrets/S/versions/V", ref)
	}
	return accessSecretPayload(parts[1], parts[3], parts[5])
}
