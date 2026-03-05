package core

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
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

	now := time.Now().UTC()
	img := api.Image{
		ID:       imageID,
		RepoTags: []string{ref},
		RepoDigests: []string{
			strings.Split(ref, ":")[0] + "@sha256:" + fmt.Sprintf("%x", hash)[:64],
		},
		Created:      now.Format(time.RFC3339Nano),
		Size:         7654321,
		VirtualSize:  7654321,
		Architecture: "amd64",
		Os:           "linux",
		Config:       imgConfig,
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: []string{"sha256:" + GenerateID()},
		},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: now.Format(time.RFC3339Nano)},
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
	defer r.Body.Close()

	// Parse the tar to extract manifest.json and config for image metadata
	repoTags, imgConfig := parseImageTar(r.Body)

	// Fallback to "loaded:latest" if no manifest found
	if len(repoTags) == 0 {
		repoTags = []string{"loaded:latest"}
	}

	id := "sha256:" + GenerateID()
	layerID := GenerateID()
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	img := api.Image{
		ID:           id,
		RepoTags:     repoTags,
		Created:      nowStr,
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Config: api.ContainerConfig{
			Labels: make(map[string]string),
		},
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: []string{"sha256:" + layerID},
		},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + id[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + id[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + id[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: nowStr},
	}

	// Merge parsed config
	if imgConfig != nil {
		if len(imgConfig.Env) > 0 {
			img.Config.Env = imgConfig.Env
		}
		if len(imgConfig.Cmd) > 0 {
			img.Config.Cmd = imgConfig.Cmd
		}
		if len(imgConfig.Entrypoint) > 0 {
			img.Config.Entrypoint = imgConfig.Entrypoint
		}
		if imgConfig.WorkingDir != "" {
			img.Config.WorkingDir = imgConfig.WorkingDir
		}
		if len(imgConfig.Labels) > 0 {
			img.Config.Labels = imgConfig.Labels
		}
	}

	// Store under all tags
	for _, tag := range repoTags {
		StoreImageWithAliases(s.Store, tag, img)
	}
	s.Store.Images.Put(id, img)

	displayTag := repoTags[0]

	s.emitEvent("image", "load", id, map[string]string{"name": displayTag})

	// BUG-493: Respect quiet param
	quiet := r.URL.Query().Get("quiet") == "1" || r.URL.Query().Get("quiet") == "true"
	if quiet {
		w.WriteHeader(http.StatusOK)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"stream": fmt.Sprintf("Loaded image: %s\n", displayTag),
		})
	}
}

// parseImageTar reads a docker save tar and extracts RepoTags from manifest.json
// and Env/Cmd/Entrypoint/WorkingDir/Labels from the image config JSON.
func parseImageTar(body io.Reader) (repoTags []string, config *api.ContainerConfig) {
	tr := tar.NewReader(body)
	var manifestData []byte
	configFiles := make(map[string][]byte)

	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}

		switch {
		case hdr.Name == "manifest.json":
			manifestData, _ = io.ReadAll(tr)
		case strings.HasSuffix(hdr.Name, ".json") && hdr.Name != "manifest.json" && !strings.Contains(hdr.Name, "/"):
			data, _ := io.ReadAll(tr)
			configFiles[hdr.Name] = data
		default:
			io.Copy(io.Discard, tr)
		}
	}

	if manifestData == nil {
		return nil, nil
	}

	var manifest []struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags"`
	}
	if json.Unmarshal(manifestData, &manifest) != nil || len(manifest) == 0 {
		return nil, nil
	}

	repoTags = manifest[0].RepoTags

	// Parse config file for container config
	if configData, ok := configFiles[manifest[0].Config]; ok {
		var imgJSON struct {
			Config struct {
				Env        []string          `json:"Env"`
				Cmd        []string          `json:"Cmd"`
				Entrypoint []string          `json:"Entrypoint"`
				WorkingDir string            `json:"WorkingDir"`
				Labels     map[string]string `json:"Labels"`
			} `json:"config"`
		}
		if json.Unmarshal(configData, &imgJSON) == nil {
			config = &api.ContainerConfig{
				Env:        imgJSON.Config.Env,
				Cmd:        imgJSON.Config.Cmd,
				Entrypoint: imgJSON.Config.Entrypoint,
				WorkingDir: imgJSON.Config.WorkingDir,
				Labels:     imgJSON.Config.Labels,
			}
		}
	}

	return repoTags, config
}

func (s *BaseServer) handleImageTag(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: name})
		return
	}

	repo := r.URL.Query().Get("repo")
	// BUG-499: Validate that repo is not empty
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
	filters := ParseFilters(r.URL.Query().Get("filters"))
	referenceFilters := filters["reference"]
	danglingFilters := filters["dangling"]
	labelFilters := filters["label"]
	beforeFilters := filters["before"] // BUG-490
	sinceFilters := filters["since"]   // BUG-491

	// BUG-431: Build image→container count map
	imgContainerCount := make(map[string]int64)
	for _, c := range s.Store.Containers.List() {
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imgContainerCount[img.ID]++
		}
	}

	seen := make(map[string]bool)
	var result []*api.ImageSummary
	for _, img := range s.Store.Images.List() {
		if seen[img.ID] {
			continue
		}
		seen[img.ID] = true

		// BUG-279: apply reference filter
		if len(referenceFilters) > 0 {
			matched := false
			for _, ref := range referenceFilters {
				for _, tag := range img.RepoTags {
					if m, _ := path.Match(ref, tag); m {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}

		// BUG-279: apply dangling filter
		if len(danglingFilters) > 0 {
			isDangling := true
			for _, tag := range img.RepoTags {
				if !strings.Contains(tag, "<none>") {
					isDangling = false
					break
				}
			}
			wantDangling := danglingFilters[0] == "true" || danglingFilters[0] == "1"
			if wantDangling != isDangling {
				continue
			}
		}

		// BUG-279: apply label filter
		if len(labelFilters) > 0 && !MatchLabels(img.Config.Labels, labelFilters) {
			continue
		}

		// BUG-490: apply before filter
		if len(beforeFilters) > 0 {
			skip := false
			for _, val := range beforeFilters {
				if refImg, ok := s.Store.ResolveImage(val); ok {
					refTime, _ := time.Parse(time.RFC3339Nano, refImg.Created)
					imgTime, _ := time.Parse(time.RFC3339Nano, img.Created)
					if !imgTime.Before(refTime) {
						skip = true
						break
					}
				}
			}
			if skip {
				continue
			}
		}

		// BUG-491: apply since filter
		if len(sinceFilters) > 0 {
			skip := false
			for _, val := range sinceFilters {
				if refImg, ok := s.Store.ResolveImage(val); ok {
					refTime, _ := time.Parse(time.RFC3339Nano, refImg.Created)
					imgTime, _ := time.Parse(time.RFC3339Nano, img.Created)
					if !imgTime.After(refTime) {
						skip = true
						break
					}
				}
			}
			if skip {
				continue
			}
		}

		created, _ := time.Parse(time.RFC3339Nano, img.Created)
		result = append(result, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     created.Unix(),
			Size:        img.Size,
			VirtualSize: img.VirtualSize,
			Labels:      img.Config.Labels,
			Containers:  imgContainerCount[img.ID],
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

		// Clean up build context staging directory
		if stagingDir, ok := s.Store.BuildContexts.LoadAndDelete(img.ID); ok {
			os.RemoveAll(stagingDir.(string))
		}

		for _, tag := range img.RepoTags {
			deleted = append(deleted, &api.ImageDeleteResponse{Untagged: tag})
			s.emitEvent("image", "untag", img.ID, map[string]string{"name": tag})
		}
		deleted = append(deleted, &api.ImageDeleteResponse{Deleted: img.ID})
		s.emitEvent("image", "delete", img.ID, map[string]string{"name": img.ID})
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

func (s *BaseServer) handleImagePush(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("name")
	img, ok := s.Store.ResolveImage(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "image", ID: ref})
		return
	}

	// BUG-494: Accept auth query param (base64-encoded JSON credentials)
	if authB64 := r.URL.Query().Get("auth"); authB64 != "" {
		if data, err := base64.StdEncoding.DecodeString(authB64); err == nil {
			var cred api.AuthRequest
			if json.Unmarshal(data, &cred) == nil && cred.ServerAddress != "" {
				s.Store.Creds.Put(cred.ServerAddress, cred)
			}
		}
	}

	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	_ = enc.Encode(map[string]string{"status": "The push refers to repository [" + ref + "]"})
	_ = enc.Encode(map[string]string{"status": "Preparing", "id": tag})
	_ = enc.Encode(map[string]string{"status": "Pushed", "id": tag})
	digest := strings.TrimPrefix(img.ID, "sha256:")
	_ = enc.Encode(map[string]string{"status": tag + ": digest: sha256:" + digest})
}

func (s *BaseServer) handleImageSave(w http.ResponseWriter, r *http.Request) {
	names := r.URL.Query()["names"]
	if len(names) == 0 {
		name := r.PathValue("name")
		if name != "" {
			names = []string{name}
		}
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()
	var manifests []map[string]any
	for _, name := range names {
		img, ok := s.Store.ResolveImage(name)
		if !ok {
			continue
		}
		layers := img.RootFS.Layers
		if layers == nil {
			layers = []string{}
		}
		manifests = append(manifests, map[string]any{
			"Config":   img.ID + ".json",
			"RepoTags": img.RepoTags,
			"Layers":   layers,
		})

		// BUG-488: Write image config JSON entry so docker load can parse it
		configData, _ := json.Marshal(map[string]any{
			"architecture": img.Architecture,
			"os":           img.Os,
			"created":      img.Created,
			"config":       img.Config,
			"rootfs":       img.RootFS,
		})
		_ = tw.WriteHeader(&tar.Header{Name: img.ID + ".json", Size: int64(len(configData))})
		_, _ = tw.Write(configData)
	}
	if manifests == nil {
		manifests = []map[string]any{}
	}
	data, _ := json.Marshal(manifests)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(data))})
	_, _ = tw.Write(data)
}

func (s *BaseServer) handleImageSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	limitStr := r.URL.Query().Get("limit")
	limit := 0
	if limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}
	var results []map[string]any
	seen := make(map[string]bool)
	for _, img := range s.Store.Images.List() {
		if seen[img.ID] {
			continue
		}
		seen[img.ID] = true
		for _, tag := range img.RepoTags {
			if strings.Contains(tag, term) {
				results = append(results, map[string]any{
					"name": tag, "description": "", "star_count": 0,
					"is_official": false, "is_automated": false,
				})
				break
			}
		}
	}
	if results == nil {
		results = []map[string]any{}
	}
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	WriteJSON(w, http.StatusOK, results)
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
