package docker

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

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

	rc, err := s.docker.ImagePull(r.Context(), req.Reference, image.PullOptions{RegistryAuth: req.Auth, Platform: req.Platform})
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
		Parent:        info.Parent,
		Comment:       info.Comment,
		Created:       info.Created,
		Size:          info.Size,
		VirtualSize:   info.VirtualSize,
		Architecture:  info.Architecture,
		Os:            info.Os,
		Author:        info.Author,
		DockerVersion: info.DockerVersion,
	}

	if info.Config != nil {
		img.Config = api.ContainerConfig{
			Hostname:     info.Config.Hostname,
			Domainname:   info.Config.Domainname,
			User:         info.Config.User,
			AttachStdin:  info.Config.AttachStdin,
			AttachStdout: info.Config.AttachStdout,
			AttachStderr: info.Config.AttachStderr,
			ExposedPorts: mapExposedPorts(info.Config.ExposedPorts),
			Tty:          info.Config.Tty,
			OpenStdin:    info.Config.OpenStdin,
			StdinOnce:    info.Config.StdinOnce,
			Env:          info.Config.Env,
			Cmd:          info.Config.Cmd,
			Image:        info.Config.Image,
			Volumes:      info.Config.Volumes,
			WorkingDir:   info.Config.WorkingDir,
			Entrypoint:   info.Config.Entrypoint,
			Labels:       info.Config.Labels,
			StopSignal:   info.Config.StopSignal,
			StopTimeout:  info.Config.StopTimeout,
			Shell:        info.Config.Shell,
		}
		if info.Config.Healthcheck != nil {
			img.Config.Healthcheck = &api.HealthcheckConfig{
				Test:          info.Config.Healthcheck.Test,
				Interval:      int64(info.Config.Healthcheck.Interval),
				Timeout:       int64(info.Config.Healthcheck.Timeout),
				StartPeriod:   int64(info.Config.Healthcheck.StartPeriod),
				StartInterval: int64(info.Config.Healthcheck.StartInterval), // BUG-564
				Retries:       info.Config.Healthcheck.Retries,
			}
		}
	}

	if info.GraphDriver.Name != "" {
		img.GraphDriver = api.GraphDriverData{
			Name: info.GraphDriver.Name,
			Data: info.GraphDriver.Data,
		}
	}

	if info.RootFS.Type != "" {
		img.RootFS = api.RootFS{
			Type:   info.RootFS.Type,
			Layers: info.RootFS.Layers,
		}
	}

	if !info.Metadata.LastTagTime.IsZero() {
		img.Metadata.LastTagTime = info.Metadata.LastTagTime.Format(time.RFC3339Nano)
	}

	writeJSON(w, http.StatusOK, img)
}

func (s *Server) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	quiet := r.URL.Query().Get("quiet") == "1" || r.URL.Query().Get("quiet") == "true"
	resp, err := s.docker.ImageLoad(r.Context(), r.Body, quiet)
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
	w.WriteHeader(http.StatusOK) // BUG-543: Docker API returns 200 for image tag
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
