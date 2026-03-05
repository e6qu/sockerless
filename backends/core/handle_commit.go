package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	_ = r.URL.Query().Get("pause") // accepted but no-op for synthetic containers

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
		if err := json.NewDecoder(r.Body).Decode(&overrides); err != nil && err != io.EOF {
			WriteError(w, &api.InvalidParameterError{Message: "invalid commit config: " + err.Error()})
			return
		}
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

	// Apply changes query param (Dockerfile instructions)
	if changesParam := r.URL.Query().Get("changes"); changesParam != "" {
		for _, change := range strings.Split(changesParam, "\n") {
			change = strings.TrimSpace(change)
			if change == "" {
				continue
			}
			parts := strings.SplitN(change, " ", 2)
			if len(parts) < 2 {
				continue
			}
			instruction, value := strings.ToUpper(parts[0]), parts[1]
			switch instruction {
			case "CMD":
				imgConfig.Cmd = parseJSONOrShell(value)
			case "ENTRYPOINT":
				imgConfig.Entrypoint = parseJSONOrShell(value)
			case "ENV":
				k, v, _ := strings.Cut(value, "=")
				imgConfig.Env = append(imgConfig.Env, k+"="+v)
			case "WORKDIR":
				imgConfig.WorkingDir = value
			case "USER":
				imgConfig.User = value
			case "LABEL":
				k, v, _ := strings.Cut(value, "=")
				if imgConfig.Labels == nil {
					imgConfig.Labels = map[string]string{}
				}
				imgConfig.Labels[k] = strings.Trim(v, "\"")
			case "EXPOSE":
				if imgConfig.ExposedPorts == nil {
					imgConfig.ExposedPorts = map[string]struct{}{}
				}
				imgConfig.ExposedPorts[value+"/tcp"] = struct{}{}
			}
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

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	img := api.Image{
		ID:           imageID,
		RepoTags:     repoTags,
		Created:      nowStr,
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Author:       author,
		Comment:      comment,
		Config:       imgConfig,
		RootFS:   api.RootFS{Type: "layers", Layers: []string{"sha256:" + GenerateID()}}, // BUG-453
		GraphDriver: api.GraphDriverData{ // BUG-454
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: nowStr},
	}

	if repo != "" {
		StoreImageWithAliases(s.Store, ref, img)
	} else {
		s.Store.Images.Put(imageID, img)
	}

	s.emitEvent("container", "commit", c.ID, map[string]string{
		"comment": comment, "imageID": imageID, "imageName": repo + ":" + tag,
	})

	WriteJSON(w, http.StatusCreated, api.ContainerCommitResponse{ID: imageID})
}

// parseJSONOrShell tries to parse value as a JSON array, falling back to shell form.
func parseJSONOrShell(value string) []string {
	var result []string
	if err := json.Unmarshal([]byte(value), &result); err == nil {
		return result
	}
	return []string{"/bin/sh", "-c", value}
}
