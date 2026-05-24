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

// SecretVersion is the wire shape for a single secret version —
// metadata only. Real GCP's GetSecretVersion + ListSecretVersions
// return this shape (no payload bytes); the raw payload appears
// only in `:access` responses. The sim stores payload bytes in a
// parallel `smSecretPayloads` store keyed by version name so this
// struct stays payload-free even after sim.Store JSON round-trips.
type SecretVersion struct {
	Name       string `json:"name"`
	CreateTime string `json:"createTime"`
	State      string `json:"state"`
}

// smPayloadRecord stores raw secret bytes keyed by full version name.
// Separate from SecretVersion so the wire-shaped struct stays
// payload-free (real GCP only returns payloads from :access).
type smPayloadRecord struct {
	Data []byte `json:"data"`
}

// Package-level stores so cloudbuild.go can resolve secret versions
// during build-step env expansion.
var (
	smSecrets        sim.Store[Secret]
	smSecretVersions sim.Store[SecretVersion]
	smSecretPayloads sim.Store[smPayloadRecord]
)

func registerSecretManager(srv *sim.Server) {
	smSecrets = sim.MakeStore[Secret](srv.DB(), "sm_secrets")
	smSecretVersions = sim.MakeStore[SecretVersion](srv.DB(), "sm_secret_versions")
	smSecretPayloads = sim.MakeStore[smPayloadRecord](srv.DB(), "sm_secret_payloads")

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
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

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
	// ListSecrets: GET /v1/projects/{project}/secrets
	// Registered explicitly because the global GCS catch-all at the
	// same path prefix used to swallow this request and return a
	// GCS-shaped 404 with `bucket "v1"` error message.
	srv.HandleFunc("GET /v1/projects/{project}/secrets", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		prefix := fmt.Sprintf("projects/%s/secrets/", project)
		var out []Secret
		for _, s := range smSecrets.List() {
			if strings.HasPrefix(s.Name, prefix) {
				out = append(out, s)
			}
		}
		if out == nil {
			out = []Secret{}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{"secrets": out})
	})

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
		}
		smSecretVersions.Put(versionName, ver)
		smSecretPayloads.Put(versionName, smPayloadRecord{Data: raw})
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
			if !found {
				// Plain GetSecretVersion (no `:action` suffix): return the
				// version metadata. tf-google reads back the version after
				// create to populate the resource state.
				versionID = versionAction
				if versionID == "latest" {
					resolved, ok := resolveLatestVersionID(project, secretID)
					if !ok {
						sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
							"no enabled versions for secret projects/%s/secrets/%s", project, secretID)
						return
					}
					versionID = resolved
				}
				versionName := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secretID, versionID)
				ver, ok := smSecretVersions.Get(versionName)
				if !ok {
					sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret version %s not found", versionName)
					return
				}
				sim.WriteJSON(w, http.StatusOK, ver)
				return
			}
			if action != "access" {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown version action %q", versionAction)
				return
			}
			payload, resolvedID, err := accessSecretPayloadResolved(project, secretID, versionID)
			if err != nil {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "%s", err.Error())
				return
			}
			// Real GCP resolves `latest` to the concrete version
			// number in the response `name` so clients can pin
			// downstream calls, detect rotation, and log the
			// exact version that served a request.
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"name": fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secretID, resolvedID),
				"payload": map[string]string{
					"data": base64.StdEncoding.EncodeToString(payload),
				},
			})
		})

	// Enable / Disable / Destroy secret versions:
	//   POST /v1/projects/{project}/secrets/{secret}/versions/{version}:enable
	//   POST /v1/projects/{project}/secrets/{secret}/versions/{version}:disable
	//   POST /v1/projects/{project}/secrets/{secret}/versions/{version}:destroy
	// The terraform-provider-google secret_version resource POSTs :enable
	// after creating a version (versions default to ENABLED on create; the
	// explicit enable is a no-op but the provider still expects 200).
	srv.HandleFunc("POST /v1/projects/{project}/secrets/{secret}/versions/{versionAction}",
		func(w http.ResponseWriter, r *http.Request) {
			project := sim.PathParam(r, "project")
			secretID := sim.PathParam(r, "secret")
			versionAction := sim.PathParam(r, "versionAction")
			versionID, action, found := strings.Cut(versionAction, ":")
			if !found {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "missing :action suffix on version %q", versionAction)
				return
			}
			// Resolve `latest` alias to the concrete version number
			// per real GCP behaviour — :enable/:disable/:destroy on
			// `latest` act on the resolved version, and the response
			// `name` carries that version (not the literal "latest").
			if versionID == "latest" {
				resolved, ok := resolveLatestVersionID(project, secretID)
				if !ok {
					sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND",
						"no enabled versions for secret projects/%s/secrets/%s", project, secretID)
					return
				}
				versionID = resolved
			}
			versionName := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secretID, versionID)
			ver, ok := smSecretVersions.Get(versionName)
			if !ok {
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret version %s not found", versionName)
				return
			}
			switch action {
			case "enable":
				ver.State = "ENABLED"
			case "disable":
				ver.State = "DISABLED"
			case "destroy":
				ver.State = "DESTROYED"
				smSecretPayloads.Delete(versionName)
			default:
				sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown version action %q", versionAction)
				return
			}
			smSecretVersions.Put(versionName, ver)
			sim.WriteJSON(w, http.StatusOK, ver)
		})
}

// accessSecretPayload resolves a secret-version reference to its raw
// payload. Handles both explicit versions (e.g. "3") and the special
// "latest" alias. Exported for cloudbuild.go's build-step secretEnv
// expansion.
func accessSecretPayload(project, secretID, version string) ([]byte, error) {
	payload, _, err := accessSecretPayloadResolved(project, secretID, version)
	return payload, err
}

// accessSecretPayloadResolved is like accessSecretPayload but also
// returns the concrete version identifier that "latest" resolved to.
// Real GCP Secret Manager echoes the resolved version number in
// every `:access` response's `name` field — without that, rotation-
// tracking + audit-logging clients see `"latest"` forever and
// can't detect when the underlying version changes.
func accessSecretPayloadResolved(project, secretID, version string) ([]byte, string, error) {
	secretName := fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
	if version == "latest" {
		// Pick the highest-numbered version for this secret.
		var latestN int
		var latestName string
		for _, v := range smSecretVersions.List() {
			if !strings.HasPrefix(v.Name, secretName+"/versions/") {
				continue
			}
			idStr := strings.TrimPrefix(v.Name, secretName+"/versions/")
			var n int
			_, _ = fmt.Sscanf(idStr, "%d", &n)
			if n > latestN {
				latestN = n
				latestName = v.Name
			}
		}
		if latestN == 0 {
			return nil, "", fmt.Errorf("no enabled versions for secret %s", secretName)
		}
		pl, ok := smSecretPayloads.Get(latestName)
		if !ok {
			return nil, "", fmt.Errorf("payload for %s not found", latestName)
		}
		return pl.Data, fmt.Sprintf("%d", latestN), nil
	}

	versionName := fmt.Sprintf("%s/versions/%s", secretName, version)
	if _, ok := smSecretVersions.Get(versionName); !ok {
		return nil, "", fmt.Errorf("secret version %s not found", versionName)
	}
	pl, ok := smSecretPayloads.Get(versionName)
	if !ok {
		return nil, "", fmt.Errorf("payload for %s not found", versionName)
	}
	return pl.Data, version, nil
}

// resolveLatestVersionID returns the concrete version number of the
// "latest" alias for the given secret. Returns "" + false if no
// versions exist.
func resolveLatestVersionID(project, secretID string) (string, bool) {
	secretName := fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
	var latestN int
	for _, v := range smSecretVersions.List() {
		if !strings.HasPrefix(v.Name, secretName+"/versions/") {
			continue
		}
		idStr := strings.TrimPrefix(v.Name, secretName+"/versions/")
		var n int
		_, _ = fmt.Sscanf(idStr, "%d", &n)
		if n > latestN {
			latestN = n
		}
	}
	if latestN == 0 {
		return "", false
	}
	return fmt.Sprintf("%d", latestN), true
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
