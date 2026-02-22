package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Registry represents an Azure Container Registry.
type Registry struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	Location   string             `json:"location"`
	Sku        *RegistrySku       `json:"sku,omitempty"`
	Tags       map[string]string  `json:"tags,omitempty"`
	Properties RegistryProperties `json:"properties"`
}

// RegistrySku holds the SKU for a container registry.
type RegistrySku struct {
	Name string `json:"name"`
	Tier string `json:"tier,omitempty"`
}

// RegistryProperties holds the properties of a container registry.
type RegistryProperties struct {
	LoginServer         string `json:"loginServer"`
	ProvisioningState   string `json:"provisioningState"`
	AdminUserEnabled    bool   `json:"adminUserEnabled"`
	PublicNetworkAccess string `json:"publicNetworkAccess,omitempty"`
	ZoneRedundancy      string `json:"zoneRedundancy,omitempty"`
}

// OCIManifest represents a stored OCI manifest.
type OCIManifest struct {
	ContentType string `json:"contentType"`
	Digest      string `json:"digest"`
	Data        []byte `json:"data"`
}

// BlobData represents a stored blob.
type BlobData struct {
	Digest string `json:"digest"`
	Data   []byte `json:"data"`
}

// BlobUpload represents an in-progress blob upload.
type BlobUpload struct {
	UUID string `json:"uuid"`
	Repo string `json:"repo"`
	Data []byte `json:"data"`
}

func registerACR(srv *sim.Server) {
	registries := sim.NewStateStore[Registry]()
	// manifests stores manifests keyed by "repo:reference" (tag or digest)
	manifests := sim.NewStateStore[OCIManifest]()
	// blobs stores blobs keyed by "repo@digest"
	blobs := sim.NewStateStore[BlobData]()
	// uploads stores in-progress uploads keyed by uuid
	uploads := sim.NewStateStore[BlobUpload]()

	const armBase = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ContainerRegistry"

	// POST - Check name availability (azurerm v3 calls this before creating a registry)
	srv.HandleFunc("POST /subscriptions/{subscriptionId}/providers/Microsoft.ContainerRegistry/checkNameAvailability", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		sim.ReadJSON(r, &req)
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"nameAvailable": true,
		})
	})

	// PUT - Create or update registry
	srv.HandleFunc("PUT "+armBase+"/registries/{registryName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")

		var req Registry
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)

		sku := req.Sku
		if sku == nil {
			sku = &RegistrySku{Name: "Basic", Tier: "Basic"}
		}

		reg := Registry{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.ContainerRegistry/registries",
			Location: req.Location,
			Sku:      sku,
			Tags:     req.Tags,
			Properties: RegistryProperties{
				LoginServer:         strings.ToLower(name) + ".azurecr.io",
				ProvisioningState:   "Succeeded",
				AdminUserEnabled:    req.Properties.AdminUserEnabled,
				PublicNetworkAccess: "Enabled",
				ZoneRedundancy:      "Disabled",
			},
		}

		registries.Put(resourceID, reg)

		// go-azure-sdk expects 200 for sync creates
		sim.WriteJSON(w, http.StatusOK, reg)
	})

	// GET - Get registry
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)

		reg, ok := registries.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.ContainerRegistry/registries/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, reg)
	})

	// GET - List replications (azurerm provider reads this after creating a registry)
	srv.HandleFunc("GET "+armBase+"/registries/{registryName}/replications", func(w http.ResponseWriter, r *http.Request) {
		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": []any{},
		})
	})

	// DELETE - Delete registry
	srv.HandleFunc("DELETE "+armBase+"/registries/{registryName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "registryName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerRegistry/registries/%s", sub, rg, name)

		if registries.Delete(resourceID) {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// --- OCI Distribution API ---

	// GET /v2/ and GET /v2/{path...} - OCI Distribution API
	// Combined into a single handler because Go's ServeMux doesn't allow both patterns.
	srv.HandleFunc("GET /v2/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fullPath := sim.PathParam(r, "path")
		// /v2/ version check (empty path)
		if fullPath == "" {
			w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
			sim.WriteJSON(w, http.StatusOK, map[string]any{})
			return
		}
		if repo, ref, ok := parseManifestPath(fullPath); ok {
			key := repo + ":" + ref
			manifest, found := manifests.Get(key)
			if !found {
				// Try by digest if ref looks like a tag
				if !strings.HasPrefix(ref, "sha256:") {
					// Search all manifests for this repo with matching tag
					writeOCIError(w, "MANIFEST_UNKNOWN", fmt.Sprintf("manifest tagged by %q is not found", ref), http.StatusNotFound)
					return
				}
				writeOCIError(w, "MANIFEST_UNKNOWN", fmt.Sprintf("manifest %q is not found", ref), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", manifest.ContentType)
			w.Header().Set("Docker-Content-Digest", manifest.Digest)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(manifest.Data)))
			w.WriteHeader(http.StatusOK)
			w.Write(manifest.Data)
			return
		}
		if repo, digest, ok := parseBlobPath(fullPath); ok {
			key := repo + "@" + digest
			blob, found := blobs.Get(key)
			if !found {
				writeOCIError(w, "BLOB_UNKNOWN", fmt.Sprintf("blob %q is not found", digest), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", blob.Digest)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob.Data)))
			w.WriteHeader(http.StatusOK)
			w.Write(blob.Data)
			return
		}
		if repo, ok := parseBlobUploadInitPath(fullPath); ok {
			uuid := generateUUID()
			uploads.Put(uuid, BlobUpload{
				UUID: uuid,
				Repo: repo,
			})
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", repo, uuid))
			w.Header().Set("Docker-Upload-UUID", uuid)
			w.Header().Set("Range", "0-0")
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if repo, uuid, ok := parseBlobUploadPath(fullPath); ok {
			upload, found := uploads.Get(uuid)
			if !found {
				writeOCIError(w, "BLOB_UPLOAD_UNKNOWN", "upload not found", http.StatusNotFound)
				return
			}
			_ = repo

			digest := r.URL.Query().Get("digest")
			if digest == "" {
				writeOCIError(w, "DIGEST_INVALID", "digest parameter is required", http.StatusBadRequest)
				return
			}

			// Read remaining body data
			body, _ := io.ReadAll(r.Body)
			data := append(upload.Data, body...)

			blobKey := upload.Repo + "@" + digest
			blobs.Put(blobKey, BlobData{
				Digest: digest,
				Data:   data,
			})
			uploads.Delete(uuid)

			w.Header().Set("Docker-Content-Digest", digest)
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", upload.Repo, digest))
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusCreated)
			return
		}

		http.NotFound(w, r)
	})

	// HEAD /v2/{name}/blobs/{digest} - Check blob existence
	srv.HandleFunc("HEAD /v2/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fullPath := sim.PathParam(r, "path")
		if repo, digest, ok := parseBlobPath(fullPath); ok {
			key := repo + "@" + digest
			blob, found := blobs.Get(key)
			if !found {
				writeOCIError(w, "BLOB_UNKNOWN", fmt.Sprintf("blob %q is not found", digest), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Docker-Content-Digest", blob.Digest)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob.Data)))
			w.WriteHeader(http.StatusOK)
			return
		}
		if repo, ref, ok := parseManifestPath(fullPath); ok {
			key := repo + ":" + ref
			manifest, found := manifests.Get(key)
			if !found {
				writeOCIError(w, "MANIFEST_UNKNOWN", fmt.Sprintf("manifest %q is not found", ref), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", manifest.ContentType)
			w.Header().Set("Docker-Content-Digest", manifest.Digest)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(manifest.Data)))
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	// PUT /v2/{name}/manifests/{reference} - Put manifest
	srv.HandleFunc("PUT /v2/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fullPath := sim.PathParam(r, "path")
		if repo, ref, ok := parseManifestPath(fullPath); ok {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				writeOCIError(w, "MANIFEST_INVALID", "failed to read body", http.StatusBadRequest)
				return
			}

			digest := fmt.Sprintf("sha256:%x", sha256.Sum256(body))
			contentType := r.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/vnd.docker.distribution.manifest.v2+json"
			}

			manifest := OCIManifest{
				ContentType: contentType,
				Digest:      digest,
				Data:        body,
			}

			// Store by tag reference
			manifests.Put(repo+":"+ref, manifest)
			// Also store by digest
			manifests.Put(repo+":"+digest, manifest)

			w.Header().Set("Docker-Content-Digest", digest)
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/manifests/%s", repo, digest))
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusCreated)
			return
		}
		if repo, uuid, ok := parseBlobUploadPath(fullPath); ok {
			upload, found := uploads.Get(uuid)
			if !found {
				writeOCIError(w, "BLOB_UPLOAD_UNKNOWN", "upload not found", http.StatusNotFound)
				return
			}
			_ = repo

			digest := r.URL.Query().Get("digest")
			if digest == "" {
				writeOCIError(w, "DIGEST_INVALID", "digest parameter is required", http.StatusBadRequest)
				return
			}

			body, _ := io.ReadAll(r.Body)
			data := append(upload.Data, body...)

			blobKey := upload.Repo + "@" + digest
			blobs.Put(blobKey, BlobData{
				Digest: digest,
				Data:   data,
			})
			uploads.Delete(uuid)

			w.Header().Set("Docker-Content-Digest", digest)
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/%s", upload.Repo, digest))
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusCreated)
			return
		}

		http.NotFound(w, r)
	})

	// POST /v2/{name}/blobs/uploads/ - Initiate upload
	srv.HandleFunc("POST /v2/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fullPath := sim.PathParam(r, "path")
		if repo, ok := parseBlobUploadInitPath(fullPath); ok {
			uuid := generateUUID()
			uploads.Put(uuid, BlobUpload{
				UUID: uuid,
				Repo: repo,
			})
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", repo, uuid))
			w.Header().Set("Docker-Upload-UUID", uuid)
			w.Header().Set("Range", "0-0")
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})

	// PATCH /v2/{name}/blobs/uploads/{uuid} - Chunked upload
	srv.HandleFunc("PATCH /v2/{path...}", func(w http.ResponseWriter, r *http.Request) {
		fullPath := sim.PathParam(r, "path")
		if repo, uuid, ok := parseBlobUploadPath(fullPath); ok {
			upload, found := uploads.Get(uuid)
			if !found {
				writeOCIError(w, "BLOB_UPLOAD_UNKNOWN", "upload not found", http.StatusNotFound)
				return
			}

			body, _ := io.ReadAll(r.Body)
			upload.Data = append(upload.Data, body...)
			uploads.Put(uuid, upload)

			end := len(upload.Data) - 1
			if end < 0 {
				end = 0
			}
			w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", repo, uuid))
			w.Header().Set("Docker-Upload-UUID", uuid)
			w.Header().Set("Range", fmt.Sprintf("0-%d", end))
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	})
}

// parseManifestPath parses paths like "{name}/manifests/{reference}" from the /v2/ prefix.
// {name} can contain slashes (e.g., "myrepo/myimage").
func parseManifestPath(path string) (repo, reference string, ok bool) {
	const marker = "/manifests/"
	idx := strings.LastIndex(path, marker)
	if idx < 0 {
		return "", "", false
	}
	repo = path[:idx]
	reference = path[idx+len(marker):]
	if repo == "" || reference == "" {
		return "", "", false
	}
	return repo, reference, true
}

// parseBlobPath parses paths like "{name}/blobs/{digest}" from the /v2/ prefix.
func parseBlobPath(path string) (repo, digest string, ok bool) {
	const marker = "/blobs/"
	idx := strings.LastIndex(path, marker)
	if idx < 0 {
		return "", "", false
	}
	rest := path[idx+len(marker):]
	// Must not be an upload path
	if strings.HasPrefix(rest, "uploads/") || rest == "uploads" {
		return "", "", false
	}
	repo = path[:idx]
	digest = rest
	if repo == "" || digest == "" {
		return "", "", false
	}
	return repo, digest, true
}

// parseBlobUploadInitPath parses paths like "{name}/blobs/uploads/" from the /v2/ prefix.
func parseBlobUploadInitPath(path string) (repo string, ok bool) {
	const marker = "/blobs/uploads/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		// Also match without trailing slash
		const marker2 = "/blobs/uploads"
		if strings.HasSuffix(path, marker2) {
			repo = path[:len(path)-len(marker2)]
			return repo, repo != ""
		}
		return "", false
	}
	// If there's content after uploads/, it's not an init path
	rest := path[idx+len(marker):]
	if rest != "" {
		return "", false
	}
	repo = path[:idx]
	return repo, repo != ""
}

// parseBlobUploadPath parses paths like "{name}/blobs/uploads/{uuid}" from the /v2/ prefix.
func parseBlobUploadPath(path string) (repo, uuid string, ok bool) {
	const marker = "/blobs/uploads/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return "", "", false
	}
	uuid = path[idx+len(marker):]
	repo = path[:idx]
	if repo == "" || uuid == "" {
		return "", "", false
	}
	return repo, uuid, true
}

// writeOCIError writes an OCI Distribution API error response.
func writeOCIError(w http.ResponseWriter, code, message string, statusCode int) {
	sim.WriteJSON(w, statusCode, map[string]any{
		"errors": []map[string]any{
			{
				"code":    code,
				"message": message,
				"detail":  map[string]any{},
			},
		},
	})
}
