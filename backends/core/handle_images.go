package core

import (
	"archive/tar"
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

	rc, err := s.self.ImagePull(req.Reference, req.Auth)
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
// Layer tarballs are kept for subsequent push operations.
func parseImageTarFull(body io.Reader) *imageLoadResult {
	tr := tar.NewReader(body)
	var manifestData []byte
	configFiles := make(map[string][]byte)
	layerFiles := make(map[string][]byte)

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
		case strings.HasSuffix(hdr.Name, "/layer.tar"):
			// Preserve layer content
			data, _ := io.ReadAll(tr)
			layerFiles[hdr.Name] = data
		default:
			io.Copy(io.Discard, tr)
		}
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

	result := &imageLoadResult{
		RepoTags: manifest[0].RepoTags,
		Layers:   layerFiles,
	}

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
			result.Config = &api.ContainerConfig{
				Env:        imgJSON.Config.Env,
				Cmd:        imgJSON.Config.Cmd,
				Entrypoint: imgJSON.Config.Entrypoint,
				WorkingDir: imgJSON.Config.WorkingDir,
				Labels:     imgJSON.Config.Labels,
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
	filters := ParseFilters(r.URL.Query().Get("filters"))
	referenceFilters := filters["reference"]
	danglingFilters := filters["dangling"]
	labelFilters := filters["label"]
	beforeFilters := filters["before"]
	sinceFilters := filters["since"]

	// Build image→container count map
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

		// Apply reference filter
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

		// Apply dangling filter
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

		// Apply label filter
		if len(labelFilters) > 0 && !MatchLabels(img.Config.Labels, labelFilters) {
			continue
		}

		// Apply before filter
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

		// Apply since filter
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

	// Accept auth query param (base64-encoded JSON credentials)
	if authB64 := r.URL.Query().Get("auth"); authB64 != "" {
		if data, err := base64.StdEncoding.DecodeString(authB64); err == nil {
			var cred api.AuthRequest
			if json.Unmarshal(data, &cred) == nil && cred.ServerAddress != "" {
				s.Store.Creds.Put(cred.ServerAddress, cred)
			}
		}
	}

	tag := r.URL.Query().Get("tag")
	rc, err := s.self.ImagePush(ref, tag, r.URL.Query().Get("auth"))
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
