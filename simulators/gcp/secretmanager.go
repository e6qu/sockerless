package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// errSecretVersionNotEnabled signals an access attempt on a DISABLED or
// DESTROYED version — real GCP returns FAILED_PRECONDITION (400), not 404.
var errSecretVersionNotEnabled = errors.New("secret version is not enabled")

// Secret Manager v1 slice. Sockerless's GCP Cloud Build integration
// references secret versions via `availableSecrets.secretManager
// [].versionName`; the simulator must return the secret payload so
// Cloud Build can expand them into env vars before executing the build
// step. Real API: https://cloud.google.com/secret-manager/docs/reference/rest

// Secret represents a Cloud Secret Manager secret resource.
type Secret struct {
	Name           string            `json:"name"`
	CreateTime     string            `json:"createTime"`
	Labels         map[string]string `json:"labels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	VersionAliases map[string]string `json:"versionAliases,omitempty"`
	Ttl            string            `json:"ttl,omitempty"`
	ExpireTime     string            `json:"expireTime,omitempty"`
	Replication    map[string]any    `json:"replication,omitempty"`
	// Nested writable objects the sim persists verbatim so create→get
	// round-trips byte-exact for the terraform-provider-google read path.
	Rotation json.RawMessage `json:"rotation,omitempty"`
	Topics   json.RawMessage `json:"topics,omitempty"`
}

// SecretVersion is the wire shape for a single secret version —
// metadata only. Real GCP's GetSecretVersion + ListSecretVersions
// return this shape (no payload bytes); the raw payload appears
// only in `:access` responses. The sim stores payload bytes in a
// parallel `smSecretPayloads` store keyed by version name so this
// struct stays payload-free even after sim.Store JSON round-trips.
type SecretVersion struct {
	Name                           string `json:"name"`
	CreateTime                     string `json:"createTime"`
	State                          string `json:"state"`
	ClientSpecifiedPayloadChecksum bool   `json:"clientSpecifiedPayloadChecksum,omitempty"`
}

// smPayloadRecord stores raw secret bytes keyed by full version name.
// Separate from SecretVersion so the wire-shaped struct stays
// payload-free (real GCP only returns payloads from :access).
type smPayloadRecord struct {
	Data       []byte `json:"data"`
	DataCrc32c int64  `json:"dataCrc32c"`
}

// Package-level stores so cloudbuild.go can resolve secret versions
// during build-step env expansion.
var (
	smSecrets        sim.Store[Secret]
	smSecretVersions sim.Store[SecretVersion]
	smSecretPayloads sim.Store[smPayloadRecord]
	// smVersionSeq holds the monotonic per-secret version counter, keyed by
	// secret name. AddSecretVersion bumps it atomically (store Update holds the
	// write lock) so concurrent adds never collide on a version ID. Kept out of
	// the Secret wire shape because GCP's Secret resource has no such field.
	smVersionSeq  sim.Store[smSeqRecord]
	smCRC32CTable = crc32.MakeTable(crc32.Castagnoli)
)

// smSeqRecord is the persisted monotonic version counter for one secret.
type smSeqRecord struct {
	Next int `json:"next"`
}

func registerSecretManager(srv *sim.Server) {
	smSecrets = sim.MakeStore[Secret](srv.DB(), "sm_secrets")
	smSecretVersions = sim.MakeStore[SecretVersion](srv.DB(), "sm_secret_versions")
	smSecretPayloads = sim.MakeStore[smPayloadRecord](srv.DB(), "sm_secret_payloads")
	smVersionSeq = sim.MakeStore[smSeqRecord](srv.DB(), "sm_version_seq")

	// CreateSecret: POST /v1/projects/{project}/secrets?secretId=X
	srv.HandleFunc("POST /v1/projects/{project}/secrets", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		secretID := r.URL.Query().Get("secretId")
		if secretID == "" {
			sim.GCPError(w, http.StatusBadRequest, "secretId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var req struct {
			Labels         map[string]string `json:"labels,omitempty"`
			Annotations    map[string]string `json:"annotations,omitempty"`
			VersionAliases map[string]string `json:"versionAliases,omitempty"`
			Ttl            string            `json:"ttl,omitempty"`
			ExpireTime     string            `json:"expireTime,omitempty"`
			Replication    map[string]any    `json:"replication,omitempty"`
			Rotation       json.RawMessage   `json:"rotation,omitempty"`
			Topics         json.RawMessage   `json:"topics,omitempty"`
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
			Name:           name,
			CreateTime:     time.Now().UTC().Format(time.RFC3339),
			Labels:         req.Labels,
			Annotations:    req.Annotations,
			VersionAliases: req.VersionAliases,
			Ttl:            req.Ttl,
			ExpireTime:     req.ExpireTime,
			Replication:    req.Replication,
			Rotation:       req.Rotation,
			Topics:         req.Topics,
		}
		smSecrets.Put(name, secret)
		smVersionSeq.Put(name, smSeqRecord{Next: 0})
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
		var all []Secret
		for _, s := range smSecrets.List() {
			if strings.HasPrefix(s.Name, prefix) {
				all = append(all, s)
			}
		}
		if all == nil {
			all = []Secret{}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
		// Honor the `filter` query param (e.g. labels.env=prod) the Secret
		// Manager ListSecrets API supports.
		all = gcpApplyListParams(all, r)
		page, next, ok := paginateList(w, r, all)
		if !ok {
			return
		}
		resp := map[string]any{"secrets": page, "totalSize": len(all)}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
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

	// UpdateSecret: PATCH /v1/projects/{project}/secrets/{secret}?updateMask=labels
	srv.HandleFunc("PATCH /v1/projects/{project}/secrets/{secret}", func(w http.ResponseWriter, r *http.Request) {
		name := fmt.Sprintf("projects/%s/secrets/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "secret"))
		secret, ok := smSecrets.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret %s not found", name)
			return
		}

		updateMask := strings.TrimSpace(r.URL.Query().Get("updateMask"))
		if updateMask == "" {
			sim.GCPError(w, http.StatusBadRequest, "updateMask query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var req struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			Topics      json.RawMessage   `json:"topics"`
			Rotation    json.RawMessage   `json:"rotation"`
		}
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		for _, field := range strings.Split(updateMask, ",") {
			switch strings.TrimSpace(field) {
			case "labels":
				secret.Labels = copyLabels(req.Labels)
			case "annotations":
				secret.Annotations = copyLabels(req.Annotations)
			case "topics":
				secret.Topics = req.Topics
			case "rotation":
				secret.Rotation = req.Rotation
			case "":
			default:
				sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "unsupported updateMask field %q", field)
				return
			}
		}
		smSecrets.Put(name, secret)
		sim.WriteJSON(w, http.StatusOK, secret)
	})

	// DeleteSecret: DELETE /v1/projects/{project}/secrets/{secret}
	srv.HandleFunc("DELETE /v1/projects/{project}/secrets/{secret}", func(w http.ResponseWriter, r *http.Request) {
		name := fmt.Sprintf("projects/%s/secrets/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "secret"))
		if !smSecrets.Delete(name) {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret %s not found", name)
			return
		}

		smVersionSeq.Delete(name)
		prefix := name + "/versions/"
		for _, v := range smSecretVersions.List() {
			if strings.HasPrefix(v.Name, prefix) {
				smSecretVersions.Delete(v.Name)
				smSecretPayloads.Delete(v.Name)
			}
		}
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
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
				Data       string `json:"data"` // base64-encoded
				DataCrc32c *int64 `json:"dataCrc32c,string,omitempty"`
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
		checksum := int64(crc32.Checksum(raw, smCRC32CTable))
		if req.Payload.DataCrc32c != nil && *req.Payload.DataCrc32c != checksum {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT",
				"payload.dataCrc32c mismatch: got %d, want %d", *req.Payload.DataCrc32c, checksum)
			return
		}

		// Version IDs are monotonically increasing. Reserve the next ID
		// atomically by bumping the per-secret counter under the store write
		// lock, so concurrent AddSecretVersion calls never collide on an ID.
		var assigned int
		if !smVersionSeq.Update(secretName, func(s *smSeqRecord) {
			s.Next++
			assigned = s.Next
		}) {
			// Counter absent (secret created before the counter existed): seed
			// it from the current version count, then reserve the next ID.
			n := 0
			for _, v := range smSecretVersions.List() {
				if strings.HasPrefix(v.Name, secretName+"/versions/") {
					n++
				}
			}
			assigned = n + 1
			smVersionSeq.Put(secretName, smSeqRecord{Next: assigned})
		}
		versionID := fmt.Sprintf("%d", assigned)
		versionName := fmt.Sprintf("%s/versions/%s", secretName, versionID)
		ver := SecretVersion{
			Name:                           versionName,
			CreateTime:                     time.Now().UTC().Format(time.RFC3339),
			State:                          "ENABLED",
			ClientSpecifiedPayloadChecksum: req.Payload.DataCrc32c != nil,
		}
		smSecretVersions.Put(versionName, ver)
		smSecretPayloads.Put(versionName, smPayloadRecord{Data: raw, DataCrc32c: checksum})
		sim.WriteJSON(w, http.StatusOK, ver)
	})

	// ListSecretVersions: GET /v1/projects/{project}/secrets/{secret}/versions
	srv.HandleFunc("GET /v1/projects/{project}/secrets/{secret}/versions", func(w http.ResponseWriter, r *http.Request) {
		secretName := fmt.Sprintf("projects/%s/secrets/%s",
			sim.PathParam(r, "project"), sim.PathParam(r, "secret"))
		if _, ok := smSecrets.Get(secretName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "secret %s not found", secretName)
			return
		}
		if filter := strings.TrimSpace(r.URL.Query().Get("filter")); filter != "" {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "unsupported filter %q", filter)
			return
		}

		prefix := secretName + "/versions/"
		var versions []SecretVersion
		for _, v := range smSecretVersions.List() {
			if strings.HasPrefix(v.Name, prefix) {
				versions = append(versions, v)
			}
		}
		sort.Slice(versions, func(i, j int) bool {
			iv, iok := secretVersionNumber(versions[i].Name, prefix)
			jv, jok := secretVersionNumber(versions[j].Name, prefix)
			if iok && jok && iv != jv {
				return iv > jv
			}
			if versions[i].CreateTime != versions[j].CreateTime {
				return versions[i].CreateTime > versions[j].CreateTime
			}
			return versions[i].Name > versions[j].Name
		})

		start, pageSize, ok := secretManagerPagination(w, r, len(versions))
		if !ok {
			return
		}
		end := len(versions)
		if pageSize > 0 && start+pageSize < end {
			end = start + pageSize
		}
		page := versions[start:end]
		if page == nil {
			page = []SecretVersion{}
		}
		resp := map[string]any{
			"versions":  page,
			"totalSize": len(versions),
		}
		if end < len(versions) {
			resp["nextPageToken"] = strconv.Itoa(end)
		}
		sim.WriteJSON(w, http.StatusOK, resp)
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
				if errors.Is(err, errSecretVersionNotEnabled) {
					sim.GCPErrorf(w, http.StatusBadRequest, "FAILED_PRECONDITION", "%s", err.Error())
					return
				}
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
					"data":       base64.StdEncoding.EncodeToString(payload.Data),
					"dataCrc32c": strconv.FormatInt(payload.DataCrc32c, 10),
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

func copyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func secretVersionNumber(name, prefix string) (int, bool) {
	raw := strings.TrimPrefix(name, prefix)
	n, err := strconv.Atoi(raw)
	return n, err == nil
}

func secretManagerPagination(w http.ResponseWriter, r *http.Request, total int) (int, int, bool) {
	start := 0
	if token := r.URL.Query().Get("pageToken"); token != "" {
		n, err := strconv.Atoi(token)
		if err != nil || n < 0 || n > total {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid pageToken %q", token)
			return 0, 0, false
		}
		start = n
	}

	pageSize := 0
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid pageSize %q", raw)
			return 0, 0, false
		}
		if n > 25000 {
			n = 25000
		}
		pageSize = n
	}
	return start, pageSize, true
}

// accessSecretPayload resolves a secret-version reference to its raw
// payload. Handles both explicit versions (e.g. "3") and the special
// "latest" alias. Exported for cloudbuild.go's build-step secretEnv
// expansion.
func accessSecretPayload(project, secretID, version string) ([]byte, error) {
	payload, _, err := accessSecretPayloadResolved(project, secretID, version)
	if err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// accessSecretPayloadResolved is like accessSecretPayload but also
// returns the concrete version identifier that "latest" resolved to.
// Real GCP Secret Manager echoes the resolved version number in
// every `:access` response's `name` field — without that, rotation-
// tracking + audit-logging clients see `"latest"` forever and
// can't detect when the underlying version changes.
func accessSecretPayloadResolved(project, secretID, version string) (smPayloadRecord, string, error) {
	secretName := fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
	resolvedID := version
	if version == "latest" {
		// "latest" is an alias for the highest version number — regardless of
		// state. Accessing it when that version is DISABLED/DESTROYED fails
		// (it does NOT fall back to an older enabled version).
		id, ok := resolveLatestVersionID(project, secretID)
		if !ok {
			return smPayloadRecord{}, "", fmt.Errorf("no versions for secret %s", secretName)
		}
		resolvedID = id
	}
	versionName := fmt.Sprintf("%s/versions/%s", secretName, resolvedID)
	ver, ok := smSecretVersions.Get(versionName)
	if !ok {
		return smPayloadRecord{}, "", fmt.Errorf("secret version %s not found", versionName)
	}
	if ver.State != "ENABLED" {
		return smPayloadRecord{}, "", fmt.Errorf("%w: cannot access the payload of version %s in state %s", errSecretVersionNotEnabled, versionName, ver.State)
	}
	pl, ok := smSecretPayloads.Get(versionName)
	if !ok {
		return smPayloadRecord{}, "", fmt.Errorf("payload for %s not found", versionName)
	}
	return pl, resolvedID, nil
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
