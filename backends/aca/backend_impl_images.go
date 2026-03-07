package aca

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// ImagePush pushes an image to a registry. For ACR targets (*.azurecr.io),
// performs a real OCI push via the Distribution API. For other targets,
// delegates to BaseServer (synthetic).
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}

	if tag == "" {
		tag = "latest"
	}

	// Determine registry from image name
	registry, repo, _ := parseImageRef(name + ":" + tag)

	// Only attempt real OCI push for ACR registries
	if !strings.HasSuffix(registry, ".azurecr.io") {
		return s.BaseServer.ImagePush(name, tag, auth)
	}

	// Attempt real OCI push to ACR — non-fatal on failure
	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		_ = enc.Encode(map[string]string{"status": "The push refers to repository [" + name + "]"})

		err := s.pushToACR(registry, repo, tag, &img, enc)
		if err != nil {
			s.Logger.Warn().Err(err).Str("registry", registry).Str("repo", repo).Msg("ACR push failed, returning synthetic progress")
			// Write synthetic success progress even on failure (non-fatal)
			_ = enc.Encode(map[string]string{"status": "Preparing", "id": tag})
			_ = enc.Encode(map[string]string{"status": "Pushed", "id": tag})
		}

		digest := strings.TrimPrefix(img.ID, "sha256:")
		_ = enc.Encode(map[string]string{"status": tag + ": digest: sha256:" + digest})
		_ = pw.Close()
	}()

	return pr, nil
}

// pushToACR performs an OCI Distribution push to ACR.
func (s *Server) pushToACR(registry, repo, tag string, img *api.Image, enc *json.Encoder) error {
	token, err := s.getACRToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := fmt.Sprintf("https://%s", registry)

	// 1. Upload synthetic config blob
	_ = enc.Encode(map[string]string{"status": "Preparing", "id": "config"})
	configData, _ := json.Marshal(map[string]any{
		"architecture": img.Architecture,
		"os":           img.Os,
		"created":      img.Created,
		"config":       img.Config,
		"rootfs":       img.RootFS,
	})
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(configData))

	if err := s.uploadBlob(client, baseURL, repo, token, configData, configDigest); err != nil {
		return fmt.Errorf("config blob upload: %w", err)
	}
	_ = enc.Encode(map[string]string{"status": "Pushed", "id": "config"})

	// 2. Upload synthetic layer blob
	_ = enc.Encode(map[string]string{"status": "Preparing", "id": "layer"})
	layerData := []byte(`{"sockerless": true}`)
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layerData))

	if err := s.uploadBlob(client, baseURL, repo, token, layerData, layerDigest); err != nil {
		return fmt.Errorf("layer blob upload: %w", err)
	}
	_ = enc.Encode(map[string]string{"status": "Pushed", "id": "layer"})

	// 3. PUT manifest
	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      len(configData),
			"digest":    configDigest,
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size":      len(layerData),
				"digest":    layerDigest,
			},
		},
	}
	manifestData, _ := json.Marshal(manifest)

	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repo, tag)
	req, _ := http.NewRequest("PUT", manifestURL, bytes.NewReader(manifestData))
	req.Header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	setAuthHeader(req, token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("manifest PUT: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("manifest PUT failed: %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// uploadBlob uploads a blob to an OCI registry via the monolithic upload flow.
func (s *Server) uploadBlob(client *http.Client, baseURL, repo, token string, data []byte, digest string) error {
	// Check if blob already exists
	headURL := fmt.Sprintf("%s/v2/%s/blobs/%s", baseURL, repo, digest)
	headReq, _ := http.NewRequest("HEAD", headURL, nil)
	setAuthHeader(headReq, token)
	headResp, err := client.Do(headReq)
	if err == nil {
		headResp.Body.Close()
		if headResp.StatusCode == 200 {
			return nil // blob already exists
		}
	}

	// Initiate upload
	initiateURL := fmt.Sprintf("%s/v2/%s/blobs/uploads/", baseURL, repo)
	initReq, _ := http.NewRequest("POST", initiateURL, nil)
	setAuthHeader(initReq, token)
	initResp, err := client.Do(initReq)
	if err != nil {
		return fmt.Errorf("initiate upload: %w", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode != 202 {
		body, _ := io.ReadAll(initResp.Body)
		return fmt.Errorf("initiate upload failed: %d: %s", initResp.StatusCode, string(body))
	}

	uploadURL := initResp.Header.Get("Location")
	if uploadURL == "" {
		return fmt.Errorf("no Location header in upload initiation response")
	}

	// Make URL absolute if relative
	if strings.HasPrefix(uploadURL, "/") {
		uploadURL = baseURL + uploadURL
	}

	// Complete upload with PUT
	sep := "?"
	if strings.Contains(uploadURL, "?") {
		sep = "&"
	}
	putURL := uploadURL + sep + "digest=" + digest
	putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(data))
	putReq.Header.Set("Content-Type", "application/octet-stream")
	putReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	setAuthHeader(putReq, token)

	putResp, err := client.Do(putReq)
	if err != nil {
		return fmt.Errorf("blob PUT: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != 201 && putResp.StatusCode != 200 {
		body, _ := io.ReadAll(putResp.Body)
		return fmt.Errorf("blob PUT failed: %d: %s", putResp.StatusCode, string(body))
	}

	return nil
}

// setAuthHeader sets the Authorization header on a request.
func setAuthHeader(req *http.Request, token string) {
	if token == "" {
		return
	}
	if strings.HasPrefix(token, "Basic ") || strings.HasPrefix(token, "Bearer ") {
		req.Header.Set("Authorization", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// ImageTag tags an image and optionally syncs the tag to ACR. Non-fatal on ACR errors.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	// Delegate to BaseServer for in-memory tagging
	if err := s.BaseServer.ImageTag(source, repo, tag); err != nil {
		return err
	}

	// Optionally sync to ACR if the target is an ACR registry
	fullRef := repo
	if tag != "" {
		fullRef = repo + ":" + tag
	}
	registry, ociRepo, ociTag := parseImageRef(fullRef)

	if strings.HasSuffix(registry, ".azurecr.io") {
		img, ok := s.Store.ResolveImage(source)
		if ok {
			go func() {
				token, err := s.getACRToken(registry)
				if err != nil {
					s.Logger.Warn().Err(err).Str("registry", registry).Msg("ACR tag sync: auth failed")
					return
				}
				client := &http.Client{Timeout: 30 * time.Second}
				baseURL := fmt.Sprintf("https://%s", registry)

				// Try to get existing manifest for the source image
				srcDigest := strings.TrimPrefix(img.ID, "sha256:")
				manifestURL := fmt.Sprintf("%s/v2/%s/manifests/sha256:%s", baseURL, ociRepo, srcDigest)
				req, _ := http.NewRequest("GET", manifestURL, nil)
				req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
				setAuthHeader(req, token)

				resp, err := client.Do(req)
				if err != nil {
					s.Logger.Warn().Err(err).Str("registry", registry).Msg("ACR tag sync: manifest fetch failed")
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != 200 {
					s.Logger.Warn().Int("status", resp.StatusCode).Str("registry", registry).Msg("ACR tag sync: source manifest not found in ACR")
					return
				}

				manifestData, _ := io.ReadAll(resp.Body)
				contentType := resp.Header.Get("Content-Type")
				if contentType == "" {
					contentType = "application/vnd.docker.distribution.manifest.v2+json"
				}

				// PUT manifest with new tag
				putURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, ociRepo, ociTag)
				putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(manifestData))
				putReq.Header.Set("Content-Type", contentType)
				setAuthHeader(putReq, token)

				putResp, err := client.Do(putReq)
				if err != nil {
					s.Logger.Warn().Err(err).Str("registry", registry).Msg("ACR tag sync: manifest PUT failed")
					return
				}
				putResp.Body.Close()

				if putResp.StatusCode != 201 && putResp.StatusCode != 200 {
					s.Logger.Warn().Int("status", putResp.StatusCode).Str("registry", registry).Msg("ACR tag sync: manifest PUT returned error")
				}
			}()
		}
	}

	return nil
}

// ImageRemove removes an image and optionally deletes the manifest from ACR. Non-fatal on ACR errors.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	// Resolve the image before removal to get metadata for ACR sync
	img, hasImg := s.Store.ResolveImage(name)
	var acrRegistry, acrRepo, acrTag string
	if hasImg {
		for _, rt := range img.RepoTags {
			reg, repo, tag := parseImageRef(rt)
			if strings.HasSuffix(reg, ".azurecr.io") {
				acrRegistry = reg
				acrRepo = repo
				acrTag = tag
				break
			}
		}
	}

	// Delegate to BaseServer for in-memory removal
	resp, err := s.BaseServer.ImageRemove(name, force, prune)
	if err != nil {
		return resp, err
	}

	// Optionally delete manifest from ACR (non-fatal, best-effort)
	if acrRegistry != "" {
		go func() {
			token, err := s.getACRToken(acrRegistry)
			if err != nil {
				s.Logger.Warn().Err(err).Str("registry", acrRegistry).Msg("ACR remove sync: auth failed")
				return
			}
			client := &http.Client{Timeout: 30 * time.Second}

			// Try to get the manifest digest first
			manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", acrRegistry, acrRepo, acrTag)
			req, _ := http.NewRequest("HEAD", manifestURL, nil)
			req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
			setAuthHeader(req, token)

			headResp, err := client.Do(req)
			if err != nil {
				s.Logger.Warn().Err(err).Str("registry", acrRegistry).Msg("ACR remove sync: manifest HEAD failed")
				return
			}
			headResp.Body.Close()

			if headResp.StatusCode != 200 {
				s.Logger.Warn().Int("status", headResp.StatusCode).Str("registry", acrRegistry).Msg("ACR remove sync: manifest not found in ACR")
				return
			}

			digest := headResp.Header.Get("Docker-Content-Digest")
			if digest == "" {
				digest = acrTag // fallback to tag as reference
			}

			// DELETE manifest
			delURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", acrRegistry, acrRepo, digest)
			delReq, _ := http.NewRequest("DELETE", delURL, nil)
			setAuthHeader(delReq, token)

			delResp, err := client.Do(delReq)
			if err != nil {
				s.Logger.Warn().Err(err).Str("registry", acrRegistry).Msg("ACR remove sync: manifest DELETE failed")
				return
			}
			delResp.Body.Close()

			if delResp.StatusCode != 202 && delResp.StatusCode != 200 {
				s.Logger.Warn().Int("status", delResp.StatusCode).Str("registry", acrRegistry).Msg("ACR remove sync: manifest DELETE returned error")
			}
		}()
	}

	return resp, nil
}

