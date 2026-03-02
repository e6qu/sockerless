package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Artifact Registry types

// Repository represents an Artifact Registry repository.
type Repository struct {
	Name        string `json:"name"`
	Format      string `json:"format"`
	Description string `json:"description,omitempty"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
}

// DockerImage represents a Docker image in Artifact Registry.
type DockerImage struct {
	Name       string   `json:"name"`
	URI        string   `json:"uri"`
	Tags       []string `json:"tags,omitempty"`
	UploadTime string   `json:"uploadTime"`
	MediaType  string   `json:"mediaType,omitempty"`
	BuildTime  string   `json:"buildTime,omitempty"`
}

// OCI Distribution types

// OCIManifest represents a stored OCI manifest.
type OCIManifest struct {
	ContentType string
	Data        []byte
}

// OCIBlob represents a stored OCI blob.
type OCIBlob struct {
	Data        []byte
	ContentType string
}

// OCIUpload tracks an in-progress blob upload.
type OCIUpload struct {
	Data []byte
}

// Package-level store for dashboard access.
var arRepos *sim.StateStore[Repository]

func registerArtifactRegistry(srv *sim.Server) {
	repos := sim.NewStateStore[Repository]()
	arRepos = repos
	dockerImages := sim.NewStateStore[DockerImage]()

	// OCI Distribution stores
	manifests := sim.NewStateStore[OCIManifest]()
	blobs := sim.NewStateStore[OCIBlob]()
	uploads := sim.NewStateStore[OCIUpload]()

	// Create repository
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/repositories", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		// The terraform google provider sends repository_id (snake_case),
		// while the SDK sends repositoryId (camelCase). Accept both.
		repoID := r.URL.Query().Get("repositoryId")
		if repoID == "" {
			repoID = r.URL.Query().Get("repository_id")
		}
		if repoID == "" {
			sim.GCPError(w, http.StatusBadRequest, "repositoryId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var repo Repository
		if err := sim.ReadJSON(r, &repo); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)
		if _, exists := repos.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "repository %q already exists", name)
			return
		}

		now := nowTimestamp()
		repo.Name = name
		if repo.Format == "" {
			repo.Format = "DOCKER"
		}
		repo.CreateTime = now
		repo.UpdateTime = now

		repos.Put(name, repo)

		lro := newLRO(project, location, repo, "type.googleapis.com/google.devtools.artifactregistry.v1.Repository")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Get repository (also handles :getIamPolicy/:setIamPolicy suffixes from terraform)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/repositories/{repo}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		repoID := sim.PathParam(r, "repo")

		// Don't match if path continues with /dockerImages
		if strings.Contains(r.URL.Path, "/dockerImages") {
			return
		}

		// Handle IAM operations — terraform google provider uses GET for these
		if base, action, ok := strings.Cut(repoID, ":"); ok {
			resource := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, base)
			handleResourceIAM(w, r, gcpResourcePolicies, resource, action)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)
		repo, ok := repos.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "repository %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, repo)
	})

	// Artifact registry repository IAM (POST variant)
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/repositories/{repoAction}", func(w http.ResponseWriter, r *http.Request) {
		repoAction := sim.PathParam(r, "repoAction")
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")

		repo, action, ok := strings.Cut(repoAction, ":")
		if !ok {
			http.NotFound(w, r)
			return
		}

		resource := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repo)
		handleResourceIAM(w, r, gcpResourcePolicies, resource, action)
	})

	// List repositories
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/repositories", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/repositories/", project, location)

		result := repos.Filter(func(repo Repository) bool {
			return strings.HasPrefix(repo.Name, prefix)
		})
		if result == nil {
			result = []Repository{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"repositories": result,
		})
	})

	// Delete repository
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/repositories/{repo}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		repoID := sim.PathParam(r, "repo")
		name := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)

		repo, ok := repos.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "repository %q not found", name)
			return
		}
		repos.Delete(name)

		// Clean up docker images for this repo
		images := dockerImages.Filter(func(img DockerImage) bool {
			return strings.HasPrefix(img.Name, name+"/")
		})
		for _, img := range images {
			dockerImages.Delete(img.Name)
		}

		lro := newLRO(project, location, repo, "type.googleapis.com/google.devtools.artifactregistry.v1.Repository")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// List docker images
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/repositories/{repo}/dockerImages", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		repoID := sim.PathParam(r, "repo")
		repoName := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)

		if _, ok := repos.Get(repoName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "repository %q not found", repoName)
			return
		}

		prefix := repoName + "/dockerImages/"
		result := dockerImages.Filter(func(img DockerImage) bool {
			return strings.HasPrefix(img.Name, prefix)
		})
		if result == nil {
			result = []DockerImage{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"dockerImages": result,
		})
	})

	// OCI Distribution API
	// We use a single catch-all handler under /v2/ and manually parse the path
	// because OCI image names can contain multiple path segments (e.g., project/repo/image)
	// and Go 1.22+ ServeMux only supports {wildcard...} as the last path element.
	ociHandler := buildOCIHandler(manifests, blobs, uploads, dockerImages)
	srv.Handle("/v2/", ociHandler)
}

// buildOCIHandler returns an http.Handler that routes OCI Distribution API requests.
// It manually parses the path to extract the image name (which can span multiple segments)
// and the operation type (manifests, blobs, uploads).
func buildOCIHandler(manifests *sim.StateStore[OCIManifest], blobs *sim.StateStore[OCIBlob], uploads *sim.StateStore[OCIUpload], dockerImages *sim.StateStore[DockerImage]) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// API version check: GET /v2/
		if path == "/v2/" && r.Method == http.MethodGet {
			sim.WriteJSON(w, http.StatusOK, map[string]any{})
			return
		}

		// Skip paths that are GCP API routes (Cloud Run, Cloud Functions, operations)
		// These start with /v2/projects/
		if strings.HasPrefix(path, "/v2/projects/") {
			// Not an OCI route; let the default mux 404 handle it
			http.NotFound(w, r)
			return
		}

		// Strip /v2/ prefix
		rest := strings.TrimPrefix(path, "/v2/")
		if rest == "" {
			http.NotFound(w, r)
			return
		}

		// Parse OCI paths: /v2/{name}/manifests/{reference}
		//                   /v2/{name}/blobs/{digest}
		//                   /v2/{name}/blobs/uploads/
		//                   /v2/{name}/blobs/uploads/{uuid}
		if idx := strings.Index(rest, "/manifests/"); idx >= 0 {
			imageName := rest[:idx]
			reference := rest[idx+len("/manifests/"):]
			handleOCIManifest(w, r, manifests, dockerImages, imageName, reference)
			return
		}

		if idx := strings.Index(rest, "/blobs/uploads/"); idx >= 0 {
			imageName := rest[:idx]
			uploadPart := rest[idx+len("/blobs/uploads/"):]
			handleOCIBlobUpload(w, r, blobs, uploads, imageName, uploadPart)
			return
		}

		if idx := strings.Index(rest, "/blobs/"); idx >= 0 {
			imageName := rest[:idx]
			digest := rest[idx+len("/blobs/"):]
			handleOCIBlob(w, r, blobs, imageName, digest)
			return
		}

		http.NotFound(w, r)
	})
}

func handleOCIManifest(w http.ResponseWriter, r *http.Request, manifests *sim.StateStore[OCIManifest], dockerImages *sim.StateStore[DockerImage], imageName, reference string) {
	key := imageName + "/manifests/" + reference

	switch r.Method {
	case http.MethodGet:
		manifest, ok := manifests.Get(key)
		if !ok {
			sim.WriteJSON(w, http.StatusNotFound, map[string]any{
				"errors": []map[string]any{
					{"code": "MANIFEST_UNKNOWN", "message": "manifest unknown"},
				},
			})
			return
		}

		w.Header().Set("Content-Type", manifest.ContentType)
		w.Header().Set("Docker-Content-Digest", reference)
		w.WriteHeader(http.StatusOK)
		w.Write(manifest.Data)

	case http.MethodPut:
		data, err := io.ReadAll(r.Body)
		if err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "failed to read body: %v", err)
			return
		}
		defer r.Body.Close()

		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/vnd.docker.distribution.manifest.v2+json"
		}

		manifests.Put(key, OCIManifest{
			ContentType: contentType,
			Data:        data,
		})

		registerDockerImageFromManifest(dockerImages, imageName, reference, contentType, data)

		w.Header().Set("Docker-Content-Digest", reference)
		w.WriteHeader(http.StatusCreated)

	default:
		w.Header().Set("Allow", "GET, PUT")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleOCIBlob(w http.ResponseWriter, r *http.Request, blobs *sim.StateStore[OCIBlob], imageName, digest string) {
	key := imageName + "/blobs/" + digest

	switch r.Method {
	case http.MethodGet:
		blob, ok := blobs.Get(key)
		if !ok {
			sim.WriteJSON(w, http.StatusNotFound, map[string]any{
				"errors": []map[string]any{
					{"code": "BLOB_UNKNOWN", "message": "blob unknown"},
				},
			})
			return
		}

		w.Header().Set("Content-Type", blob.ContentType)
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusOK)
		w.Write(blob.Data)

	case http.MethodHead:
		blob, ok := blobs.Get(key)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob.Data)))
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusOK)

	default:
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleOCIBlobUpload(w http.ResponseWriter, r *http.Request, blobs *sim.StateStore[OCIBlob], uploads *sim.StateStore[OCIUpload], imageName, uploadPart string) {
	switch r.Method {
	case http.MethodPost:
		// Initiate blob upload: POST /v2/{name}/blobs/uploads/
		uploadID := generateUUID()
		uploads.Put(uploadID, OCIUpload{})

		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", imageName, uploadID))
		w.Header().Set("Docker-Upload-UUID", uploadID)
		w.WriteHeader(http.StatusAccepted)

	case http.MethodPut:
		// Complete blob upload: PUT /v2/{name}/blobs/uploads/{uuid}?digest=...
		uploadID := uploadPart
		digest := r.URL.Query().Get("digest")

		if _, ok := uploads.Get(uploadID); !ok {
			sim.WriteJSON(w, http.StatusNotFound, map[string]any{
				"errors": []map[string]any{
					{"code": "BLOB_UPLOAD_UNKNOWN", "message": "upload not found"},
				},
			})
			return
		}

		data, err := io.ReadAll(r.Body)
		if err != nil {
			sim.GCPErrorf(w, http.StatusInternalServerError, "INTERNAL", "failed to read body: %v", err)
			return
		}
		defer r.Body.Close()

		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		key := imageName + "/blobs/" + digest
		blobs.Put(key, OCIBlob{
			Data:        data,
			ContentType: contentType,
		})

		uploads.Delete(uploadID)

		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", imageName, digest))
		w.WriteHeader(http.StatusCreated)

	default:
		w.Header().Set("Allow", "POST, PUT")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// registerDockerImageFromManifest registers a docker image in the Artifact Registry
// docker images store when a manifest is pushed.
func registerDockerImageFromManifest(dockerImages *sim.StateStore[DockerImage], imageName, reference, contentType string, data []byte) {
	// Try to parse the manifest to extract media type
	var manifest struct {
		MediaType string `json:"mediaType"`
	}
	_ = json.Unmarshal(data, &manifest) // best-effort: manifest may not contain mediaType

	now := nowTimestamp()
	imgName := imageName + "/dockerImages/" + reference
	tags := []string{}
	if !strings.HasPrefix(reference, "sha256:") {
		tags = append(tags, reference)
	}

	img := DockerImage{
		Name:       imgName,
		URI:        imageName + ":" + reference,
		Tags:       tags,
		UploadTime: now,
		MediaType:  contentType,
	}
	dockerImages.Put(imgName, img)
}
