package docker

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/sockerless/api"
)

func (s *Server) handleImagePull(w http.ResponseWriter, r *http.Request) {
	var req api.ImagePullRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	rc, err := s.docker.ImagePull(r.Context(), req.Reference, image.PullOptions{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

func (s *Server) handleImageInspect(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	info, _, err := s.docker.ImageInspectWithRaw(r.Context(), name)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	img := api.Image{
		ID:            info.ID,
		RepoTags:      info.RepoTags,
		RepoDigests:   info.RepoDigests,
		Created:       info.Created,
		Size:          info.Size,
		VirtualSize:   info.Size,
		Architecture:  info.Architecture,
		Os:            info.Os,
		Author:        info.Author,
		DockerVersion: info.DockerVersion,
	}

	if info.Config != nil {
		img.Config = api.ContainerConfig{
			Env:        info.Config.Env,
			Cmd:        info.Config.Cmd,
			Entrypoint: info.Config.Entrypoint,
			WorkingDir: info.Config.WorkingDir,
			Labels:     info.Config.Labels,
		}
	}

	if info.RootFS.Type != "" {
		img.RootFS = api.RootFS{
			Type:   info.RootFS.Type,
			Layers: info.RootFS.Layers,
		}
	}

	writeJSON(w, http.StatusOK, img)
}

func (s *Server) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	resp, err := s.docker.ImageLoad(r.Context(), r.Body, false)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}

func (s *Server) handleImageTag(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")
	ref := repo
	if tag != "" {
		ref = repo + ":" + tag
	}

	err := s.docker.ImageTag(r.Context(), name, ref)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req api.AuthRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.docker.RegistryLogin(r.Context(), registry.AuthConfig{
		Username:      req.Username,
		Password:      req.Password,
		Email:         req.Email,
		ServerAddress: req.ServerAddress,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	writeJSON(w, http.StatusOK, api.AuthResponse{
		Status:        resp.Status,
		IdentityToken: resp.IdentityToken,
	})
}

// Prevent unused import
var _ = json.Marshal
