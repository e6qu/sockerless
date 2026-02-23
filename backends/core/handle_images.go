package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// StoreImageWithAliases stores an image under all common lookup keys:
// imageID, full reference, name without tag, and short aliases for
// docker.io/library/ and docker.io/ prefixed names.
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

// --- Default image pull (synthetic, memory-like) ---

func (s *BaseServer) handleImagePull(w http.ResponseWriter, r *http.Request) {
	var req api.ImagePullRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	ref := req.Reference
	if ref == "" {
		WriteError(w, &api.InvalidParameterError{Message: "image reference is required"})
		return
	}

	// Normalize: add :latest if no tag
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref = ref + ":latest"
	}

	// If the image already exists (e.g., from a build), don't overwrite it
	if _, exists := s.Store.ResolveImage(ref); exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Pulling from %s", strings.Split(ref, ":")[0])})
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Status: Image is up to date for %s", ref)})
		return
	}

	// Generate a synthetic image ID
	hash := sha256.Sum256([]byte(ref))
	imageID := fmt.Sprintf("sha256:%x", hash)

	// Start with synthetic config defaults
	imgConfig := api.ContainerConfig{
		Env:    []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		Cmd:    []string{"/bin/sh"},
		Labels: make(map[string]string),
	}

	// Build auth credential chain: X-Registry-Auth header → Store.Creds → ~/.docker/config.json
	basicAuth := ""
	rc := parseImageRef(ref)
	if user, pass := decodeRegistryAuth(req.Auth); user != "" {
		basicAuth = base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	} else if cred, ok := s.Store.Creds.Get(rc.Registry); ok {
		basicAuth = base64.StdEncoding.EncodeToString([]byte(cred.Username + ":" + cred.Password))
	} else if cfg, err := LoadDockerConfig(DefaultDockerConfigPath()); err == nil {
		if u, p, ok := cfg.GetRegistryAuth(rc.Registry); ok {
			basicAuth = base64.StdEncoding.EncodeToString([]byte(u + ":" + p))
		}
	}

	// Try to fetch real config from registry
	if realConfig, _ := FetchImageConfig(ref, basicAuth); realConfig != nil {
		if len(realConfig.Env) > 0 {
			imgConfig.Env = realConfig.Env
		}
		if len(realConfig.Cmd) > 0 {
			imgConfig.Cmd = realConfig.Cmd
		}
		if len(realConfig.Entrypoint) > 0 {
			imgConfig.Entrypoint = realConfig.Entrypoint
		}
		if realConfig.WorkingDir != "" {
			imgConfig.WorkingDir = realConfig.WorkingDir
		}
		if len(realConfig.Labels) > 0 {
			imgConfig.Labels = realConfig.Labels
		}
		if len(realConfig.ExposedPorts) > 0 {
			imgConfig.ExposedPorts = realConfig.ExposedPorts
		}
	}

	img := api.Image{
		ID:       imageID,
		RepoTags: []string{ref},
		RepoDigests: []string{
			strings.Split(ref, ":")[0] + "@sha256:" + fmt.Sprintf("%x", hash)[:64],
		},
		Created:      time.Now().UTC().Format(time.RFC3339Nano),
		Size:         7654321,
		VirtualSize:  7654321,
		Architecture: "amd64",
		Os:           "linux",
		Config:       imgConfig,
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: []string{"sha256:" + GenerateID()},
		},
	}

	StoreImageWithAliases(s.Store, ref, img)

	// Stream synthetic pull progress
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	progress := []map[string]any{
		{"status": fmt.Sprintf("Pulling from %s", strings.Split(ref, ":")[0])},
		{"status": "Pulling fs layer", "id": "abc123"},
		{"status": "Download complete", "id": "abc123"},
		{"status": "Pull complete", "id": "abc123"},
		{"status": fmt.Sprintf("Digest: sha256:%x", hash)},
		{"status": fmt.Sprintf("Status: Downloaded newer image for %s", ref)},
	}

	enc := json.NewEncoder(w)
	for _, p := range progress {
		enc.Encode(p)
		if flusher != nil {
			flusher.Flush()
		}
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
	// Read the tar but don't actually process layers
	defer r.Body.Close()
	io.Copy(io.Discard, r.Body)

	// Create a synthetic loaded image
	id := "sha256:" + GenerateID()
	img := api.Image{
		ID:           id,
		RepoTags:     []string{"loaded:latest"},
		Created:      time.Now().UTC().Format(time.RFC3339Nano),
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Config: api.ContainerConfig{
			Labels: make(map[string]string),
		},
		RootFS: api.RootFS{Type: "layers"},
	}
	s.Store.Images.Put(id, img)
	s.Store.Images.Put("loaded:latest", img)
	s.Store.Images.Put("loaded", img)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"stream": "Loaded image: loaded:latest\n",
	})
}

func (s *BaseServer) handleImageTag(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: name})
		return
	}

	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}

	newRef := repo + ":" + tag
	img.RepoTags = append(img.RepoTags, newRef)

	// Update existing and store under new reference
	s.Store.Images.Put(img.ID, img)
	s.Store.Images.Put(newRef, img)
	s.Store.Images.Put(repo, img)

	w.WriteHeader(http.StatusCreated)
}

func (s *BaseServer) handleImageList(w http.ResponseWriter, r *http.Request) {
	var result []*api.ImageSummary
	for _, img := range s.Store.Images.List() {
		created, _ := time.Parse(time.RFC3339Nano, img.Created)
		result = append(result, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     created.Unix(),
			Size:        img.Size,
			VirtualSize: img.VirtualSize,
			Labels:      img.Config.Labels,
			Containers:  0,
		})
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

	s.Store.Images.Delete(img.ID)

	var resp []*api.ImageDeleteResponse
	for _, tag := range img.RepoTags {
		resp = append(resp, &api.ImageDeleteResponse{Untagged: tag})
	}
	resp = append(resp, &api.ImageDeleteResponse{Deleted: img.ID})
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
	WriteJSON(w, http.StatusOK, []*api.ImageHistoryEntry{
		{
			ID:        img.ID,
			Created:   created.Unix(),
			CreatedBy: "/bin/sh -c #(nop) CMD [\"sh\"]",
			Tags:      img.RepoTags,
			Size:      img.Size,
		},
	})
}

func (s *BaseServer) handleImagePrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	// Collect image IDs referenced by running or stopped containers
	referencedImages := make(map[string]bool)
	for _, c := range s.Store.Containers.List() {
		referencedImages[c.Config.Image] = true
		referencedImages[c.Image] = true
	}

	// Check dangling filter
	danglingOnly := false
	if vals, ok := filters["dangling"]; ok {
		for _, v := range vals {
			if v == "true" || v == "1" {
				danglingOnly = true
			}
		}
	}

	var deleted []*api.ImageDeleteResponse
	var spaceReclaimed uint64

	// Deduplicate: Images store has many aliases pointing to the same image.
	// Track which image IDs we've already pruned.
	prunedIDs := make(map[string]bool)

	for _, img := range s.Store.Images.List() {
		if prunedIDs[img.ID] {
			continue
		}

		// Skip images referenced by any container
		inUse := referencedImages[img.ID]
		if !inUse {
			for _, tag := range img.RepoTags {
				if referencedImages[tag] {
					inUse = true
					break
				}
				// Also check name without tag
				if idx := strings.Index(tag, ":"); idx >= 0 {
					if referencedImages[tag[:idx]] {
						inUse = true
						break
					}
				}
			}
		}
		if inUse {
			continue
		}

		// If dangling filter is set, only prune images with no tags
		if danglingOnly && len(img.RepoTags) > 0 {
			hasRealTag := false
			for _, tag := range img.RepoTags {
				if !strings.Contains(tag, "<none>") {
					hasRealTag = true
					break
				}
			}
			if hasRealTag {
				continue
			}
		}

		prunedIDs[img.ID] = true
		for _, tag := range img.RepoTags {
			deleted = append(deleted, &api.ImageDeleteResponse{Untagged: tag})
		}
		deleted = append(deleted, &api.ImageDeleteResponse{Deleted: img.ID})
		spaceReclaimed += uint64(img.Size)

		// Remove from store (all aliases)
		s.Store.Images.Delete(img.ID)
		for _, tag := range img.RepoTags {
			s.Store.Images.Delete(tag)
			if idx := strings.Index(tag, ":"); idx >= 0 {
				s.Store.Images.Delete(tag[:idx])
			}
		}
	}

	if deleted == nil {
		deleted = []*api.ImageDeleteResponse{}
	}
	WriteJSON(w, http.StatusOK, api.ImagePruneResponse{
		ImagesDeleted:  deleted,
		SpaceReclaimed: spaceReclaimed,
	})
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

// decodeRegistryAuth decodes Docker's X-Registry-Auth header value.
// The header is base64-encoded JSON: {"username":"...","password":"..."}.
// Returns empty strings on any decoding failure.
func decodeRegistryAuth(header string) (user, pass string) {
	if header == "" {
		return "", ""
	}
	decoded, err := base64.URLEncoding.DecodeString(header)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(header)
		if err != nil {
			return "", ""
		}
	}
	var auth struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if json.Unmarshal(decoded, &auth) != nil {
		return "", ""
	}
	return auth.Username, auth.Password
}
