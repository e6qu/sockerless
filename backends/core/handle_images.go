package core

import (
	"archive/tar"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// StoreImageWithAliases stores an image under all common lookup keys
// in the in-memory cache: imageID, full reference, name without tag,
// and short aliases for docker.io/library/ and docker.io/ prefixed
// names. Per the cache is purely an in-process
// optimization — after a restart the source of truth is the cloud
// registry that each backend points at (ECR for ECS/Lambda, Artifact
// Registry for Cloud Run/GCF, ACR for ACA/AZF). No disk persistence.
func StoreImageWithAliases(store *Store, ref string, img api.Image) {
	store.Images.Put(img.ID, img)
	store.Images.Put(ref, img)

	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 2 {
		store.Images.Put(parts[0], img)
	}

	// The Docker SDK normalizes image names (e.g., "alpine" → "docker.io/library/alpine")
	// but clients inspect by the original short name. Store under short aliases too.
	nameWithoutTag := parts[0]
	if strings.HasPrefix(nameWithoutTag, "docker.io/library/") {
		short := strings.TrimPrefix(nameWithoutTag, "docker.io/library/")
		tag := "latest"
		if len(parts) == 2 {
			tag = parts[1]
		}
		store.Images.Put(short+":"+tag, img)
		store.Images.Put(short, img)
	} else if strings.HasPrefix(nameWithoutTag, "docker.io/") {
		short := strings.TrimPrefix(nameWithoutTag, "docker.io/")
		tag := "latest"
		if len(parts) == 2 {
			tag = parts[1]
		}
		store.Images.Put(short+":"+tag, img)
		store.Images.Put(short, img)
	}
}

// --- Default image pull (delegates to s.self for virtual dispatch) ---

func (s *BaseServer) handleImagePull(w http.ResponseWriter, r *http.Request) {
	var req api.ImagePullRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	parsed, err := ParseImageRef(req.Reference)
	if err != nil {
		WriteError(w, &api.InvalidParameterError{Message: "invalid image reference: " + err.Error()})
		return
	}
	dctx := DriverContext{Ctx: r.Context(), Backend: s.Desc.Driver, Logger: s.Logger}
	rc, err := s.Typed.Registry.Pull(dctx, parsed, req.Auth)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// --- Common image handlers ---

func (s *BaseServer) handleImageInspect(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: name})
		return
	}
	WriteJSON(w, http.StatusOK, img)
}

func (s *BaseServer) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	rc, err := s.self.ImageLoad(r.Body)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	// Respect quiet param
	quiet := r.URL.Query().Get("quiet") == "1" || r.URL.Query().Get("quiet") == "true"
	if quiet {
		w.WriteHeader(http.StatusOK)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.Copy(w, rc)
	}
}

// imageLoadResult holds parsed data from a docker save tar archive.
type imageLoadResult struct {
	RepoTags []string
	Config   *api.ContainerConfig
	Layers   map[string][]byte // layer path → content (e.g. "abc123/layer.tar" → bytes)
}

// parseImageTarFull parses a docker save tar and preserves layer content.
// Handles two on-disk layouts:
//
//   - Classic docker save layout: config blob at root as `<digest>.json`,
//     layers under `<digest>/layer.tar`. manifest.json's Config field is
//     a bare filename like `1ab49....json`.
//
//   - OCI v1 layout (modern docker / BuildKit): blobs under
//     `blobs/sha256/<digest>` with no extension; manifest.json's Config
//     field is a path like `blobs/sha256/<digest>`. Layer .tar.gz blobs
//     also live under `blobs/sha256/`.
//
// Both formats use the same outer manifest.json structure ([{Config,
// RepoTags, Layers}]); the difference is just where the referenced
// blobs live in the tar. We index every file we encounter by its full
// path so the manifest's Config/Layers references resolve regardless
// of layout.
func parseImageTarFull(body io.Reader) *imageLoadResult {
	tr := tar.NewReader(body)
	var manifestData []byte
	allFiles := make(map[string][]byte)

	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		data, _ := io.ReadAll(tr)
		if hdr.Name == "manifest.json" {
			manifestData = data
			continue
		}
		allFiles[hdr.Name] = data
	}

	if manifestData == nil {
		return nil
	}

	var manifest []struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags"`
		Layers   []string `json:"Layers"`
	}
	if json.Unmarshal(manifestData, &manifest) != nil || len(manifest) == 0 {
		return nil
	}

	// Layer files: index under both their classic key
	// (`<digest>/layer.tar`) and OCI key (`blobs/sha256/<digest>`).
	// Callers downstream may key by either path.
	layerFiles := make(map[string][]byte)
	for _, layerPath := range manifest[0].Layers {
		if data, ok := allFiles[layerPath]; ok {
			layerFiles[layerPath] = data
		}
	}

	result := &imageLoadResult{
		RepoTags: manifest[0].RepoTags,
		Layers:   layerFiles,
	}

	// Parse config blob for container config. The blob path comes
	// directly from manifest.json's Config field — same lookup works
	// for both classic (`<digest>.json`) and OCI (`blobs/sha256/<digest>`)
	// layouts because we indexed every file in the tar. Reuses the
	// canonical `ociImageConfig` schema from registry.go so the
	// in-tar (`docker save`) and over-the-wire (registry-pull) parsers
	// share one source of truth for the OCI image-config shape.
	if configData, ok := allFiles[manifest[0].Config]; ok {
		var ociCfg ociImageConfig
		if json.Unmarshal(configData, &ociCfg) == nil {
			result.Config = &api.ContainerConfig{
				Env:          ociCfg.Config.Env,
				Cmd:          ociCfg.Config.Cmd,
				Entrypoint:   ociCfg.Config.Entrypoint,
				WorkingDir:   ociCfg.Config.WorkingDir,
				User:         ociCfg.Config.User,
				Labels:       ociCfg.Config.Labels,
				ExposedPorts: ociCfg.Config.ExposedPorts,
			}
		}
	}

	return result
}

func (s *BaseServer) handleImageTag(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: name})
		return
	}

	repo := r.URL.Query().Get("repo")
	// Validate that repo is not empty
	if repo == "" {
		WriteError(w, &api.InvalidParameterError{
			Message: "repository name must have at least one component",
		})
		return
	}
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}

	newRef := repo + ":" + tag

	// Check for duplicate tag
	for _, existing := range img.RepoTags {
		if existing == newRef {
			w.WriteHeader(http.StatusCreated)
			return
		}
	}

	img.RepoTags = append(img.RepoTags, newRef)
	img.Metadata.LastTagTime = time.Now().UTC().Format(time.RFC3339Nano)

	// Update all existing aliases to reflect the new RepoTags list
	for _, existingTag := range img.RepoTags {
		StoreImageWithAliases(s.Store, existingTag, img)
	}

	s.emitEvent("image", "tag", img.ID, map[string]string{
		"name": newRef,
	})

	w.WriteHeader(http.StatusCreated)
}

func (s *BaseServer) handleImageList(w http.ResponseWriter, r *http.Request) {
	// Delegate to s.self so passthrough backends (docker) reach the
	// upstream daemon and cloud backends reach their ImageManager
	// (which merges Store + cloud registry). The old in-handler logic
	// read s.Store.Images.List() directly, which returned [] for any
	// backend that doesn't track images in its local Store — exactly
	// the fallback-hiding-bug shape, just for list endpoints.
	opts := api.ImageListOptions{
		All:     r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true",
		Filters: ParseFilters(r.URL.Query().Get("filters")),
	}
	result, err := s.self.ImageList(opts)
	if err != nil {
		WriteError(w, err)
		return
	}
	if result == nil {
		result = []*api.ImageSummary{}
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleImageRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: name})
		return
	}

	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"

	// Check if any container references this image
	if !force {
		for _, c := range s.Store.Containers.List() {
			if c.Image == img.ID || c.Config.Image == name {
				for _, tag := range img.RepoTags {
					if c.Config.Image == tag || c.Config.Image == strings.SplitN(tag, ":", 2)[0] {
						WriteError(w, &api.ConflictError{
							Message: fmt.Sprintf("conflict: unable to remove repository reference \"%s\" (container %s is using its referenced image %s)", name, c.ID[:12], img.ID[:19]),
						})
						return
					}
				}
				if c.Image == img.ID {
					WriteError(w, &api.ConflictError{
						Message: fmt.Sprintf("conflict: unable to delete %s (cannot be forced) - image is being used by running container %s", img.ID[:19], c.ID[:12]),
					})
					return
				}
			}
		}
	}

	// Clean up build context staging directory
	if stagingDir, ok := s.Store.BuildContexts.LoadAndDelete(img.ID); ok {
		os.RemoveAll(stagingDir.(string))
	}

	s.Store.Images.Delete(img.ID)
	for _, tag := range img.RepoTags {
		s.Store.Images.Delete(tag)
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) >= 1 {
			s.Store.Images.Delete(parts[0])
			// Delete docker.io short aliases
			nameWithoutTag := parts[0]
			if strings.HasPrefix(nameWithoutTag, "docker.io/library/") {
				short := strings.TrimPrefix(nameWithoutTag, "docker.io/library/")
				s.Store.Images.Delete(short)
				if len(parts) == 2 {
					s.Store.Images.Delete(short + ":" + parts[1])
				}
			} else if strings.HasPrefix(nameWithoutTag, "docker.io/") {
				short := strings.TrimPrefix(nameWithoutTag, "docker.io/")
				s.Store.Images.Delete(short)
				if len(parts) == 2 {
					s.Store.Images.Delete(short + ":" + parts[1])
				}
			}
		}
	}

	var resp []*api.ImageDeleteResponse
	for _, tag := range img.RepoTags {
		resp = append(resp, &api.ImageDeleteResponse{Untagged: tag})
		s.emitEvent("image", "untag", img.ID, map[string]string{
			"name": tag,
		})
	}
	resp = append(resp, &api.ImageDeleteResponse{Deleted: img.ID})

	s.emitEvent("image", "delete", img.ID, map[string]string{
		"name": name,
	})

	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleImageHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: name})
		return
	}

	created, _ := time.Parse(time.RFC3339Nano, img.Created)
	var history []*api.ImageHistoryEntry

	layers := img.RootFS.Layers
	if len(layers) == 0 {
		layers = []string{img.ID}
	}

	for i, layer := range layers {
		entry := &api.ImageHistoryEntry{
			ID:      layer,
			Created: created.Unix() - int64(len(layers)-1-i),
			Size:    0,
			Comment: "",
		}
		if i == len(layers)-1 {
			entry.ID = img.ID
			entry.Created = created.Unix()
			entry.CreatedBy = "/bin/sh -c #(nop)  CMD [\"sh\"]"
			entry.Tags = img.RepoTags
			entry.Size = img.Size
			entry.Comment = img.Comment
		} else {
			layerShort := layer
			if len(layerShort) > 19 {
				layerShort = layerShort[7:19]
			}
			entry.CreatedBy = "/bin/sh -c #(nop)  ADD file:" + layerShort + " in / "
			entry.Tags = []string{}
		}
		history = append(history, entry)
	}
	WriteJSON(w, http.StatusOK, history)
}

func (s *BaseServer) handleImagePrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))
	resp, err := s.self.ImagePrune(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req api.AuthRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	// Store credentials for later use
	if req.ServerAddress != "" {
		s.Store.Creds.Put(req.ServerAddress, req)
	}

	// Memory backend always accepts auth
	WriteJSON(w, http.StatusOK, api.AuthResponse{
		Status: "Login Succeeded",
	})
}

func (s *BaseServer) handleImagePush(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")

	if authB64 := r.URL.Query().Get("auth"); authB64 != "" {
		data, err := base64.StdEncoding.DecodeString(authB64)
		if err != nil {
			WriteError(w, &api.InvalidParameterError{Message: "auth: invalid base64: " + err.Error()})
			return
		}
		var cred api.AuthRequest
		if err := json.Unmarshal(data, &cred); err != nil {
			WriteError(w, &api.InvalidParameterError{Message: "auth: invalid JSON: " + err.Error()})
			return
		}
		if cred.ServerAddress != "" {
			s.Store.Creds.Put(cred.ServerAddress, cred)
		}
	}

	tag := r.URL.Query().Get("tag")
	pushRefStr := ref
	if tag != "" {
		pushRefStr = ref + ":" + tag
	}
	parsed, err := ParseImageRef(pushRefStr)
	if err != nil {
		WriteError(w, &api.InvalidParameterError{Message: "invalid image reference: " + err.Error()})
		return
	}
	dctx := DriverContext{Ctx: r.Context(), Backend: s.Desc.Driver, Logger: s.Logger}
	rc, err := s.Typed.Registry.Push(dctx, parsed, r.URL.Query().Get("auth"))
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

func (s *BaseServer) handleImageSave(w http.ResponseWriter, r *http.Request) {
	names := r.URL.Query()["names"]
	if len(names) == 0 {
		name := r.PathValue("name")
		if name != "" {
			names = []string{name}
		}
	}

	rc, err := s.self.ImageSave(names)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

func (s *BaseServer) handleImageSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	limit := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}
	filters := ParseFilters(r.URL.Query().Get("filters"))

	results, err := s.self.ImageSearch(term, limit, filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, results)
}
