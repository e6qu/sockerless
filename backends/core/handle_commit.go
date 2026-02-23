package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sockerless/api"
)

// handleContainerCommit creates a new image from a container's current state.
// Query params: container, repo, tag, comment, author.
// Optional JSON body overrides: Cmd, Entrypoint, Env, WorkingDir.
func (s *BaseServer) handleContainerCommit(w http.ResponseWriter, r *http.Request) {
	containerRef := r.URL.Query().Get("container")
	if containerRef == "" {
		WriteError(w, &api.InvalidParameterError{Message: "container query parameter is required"})
		return
	}

	c, ok := s.Store.ResolveContainer(containerRef)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: containerRef})
		return
	}

	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	comment := r.URL.Query().Get("comment")
	author := r.URL.Query().Get("author")

	// Start with the container's config
	imgConfig := c.Config

	// Apply optional body overrides
	var overrides struct {
		Cmd        []string `json:"Cmd,omitempty"`
		Entrypoint []string `json:"Entrypoint,omitempty"`
		Env        []string `json:"Env,omitempty"`
		WorkingDir string   `json:"WorkingDir,omitempty"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		json.NewDecoder(r.Body).Decode(&overrides)
		if len(overrides.Cmd) > 0 {
			imgConfig.Cmd = overrides.Cmd
		}
		if len(overrides.Entrypoint) > 0 {
			imgConfig.Entrypoint = overrides.Entrypoint
		}
		if len(overrides.Env) > 0 {
			imgConfig.Env = overrides.Env
		}
		if overrides.WorkingDir != "" {
			imgConfig.WorkingDir = overrides.WorkingDir
		}
	}

	// Generate image ID from container ID + timestamp
	hash := sha256.Sum256([]byte(c.ID + time.Now().UTC().String()))
	imageID := fmt.Sprintf("sha256:%x", hash)

	ref := repo + ":" + tag
	var repoTags []string
	if repo != "" {
		repoTags = []string{ref}
	}

	img := api.Image{
		ID:           imageID,
		RepoTags:     repoTags,
		Created:      time.Now().UTC().Format(time.RFC3339Nano),
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Author:       author,
		Comment:      comment,
		Config:       imgConfig,
		RootFS:       api.RootFS{Type: "layers"},
	}

	if repo != "" {
		StoreImageWithAliases(s.Store, ref, img)
	} else {
		s.Store.Images.Put(imageID, img)
	}

	WriteJSON(w, http.StatusCreated, api.ContainerCommitResponse{ID: imageID})
}
