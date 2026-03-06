package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sockerless/api"
)

// mapDockerError maps Docker SDK errors to api error types.
func mapDockerError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	if strings.Contains(msg, "No such") || strings.Contains(msg, "not found") {
		resource, id := parseDockerNotFound(msg)
		return &api.NotFoundError{Resource: resource, ID: id}
	}
	if strings.Contains(msg, "is already") || strings.Contains(msg, "Conflict") || strings.Contains(msg, "conflict") {
		return &api.ConflictError{Message: msg}
	}
	if strings.Contains(msg, "not modified") || strings.Contains(msg, "Not Modified") {
		return &api.NotModifiedError{}
	}

	var dockerErr struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(msg), &dockerErr) == nil && dockerErr.Message != "" {
		msg = dockerErr.Message
	}

	return fmt.Errorf("%s", msg)
}

// parseDockerNotFound extracts the resource type and ID from Docker "No such X: Y" error messages.
func parseDockerNotFound(msg string) (resource, id string) {
	resource = "resource"
	if i := strings.Index(msg, "No such "); i >= 0 {
		rest := msg[i+8:]
		if j := strings.Index(rest, ": "); j >= 0 {
			resource = rest[:j]
			id = rest[j+2:]
			return resource, id
		}
	}
	if i := strings.Index(msg, " not found"); i >= 0 {
		id = msg[:i]
		if j := strings.LastIndex(id, " "); j >= 0 {
			id = id[j+1:]
		}
	}
	return resource, id
}

// getInfo queries the Docker daemon for system information.
func (s *Server) getInfo(ctx context.Context) (*api.BackendInfo, error) {
	info, err := s.docker.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("docker info: %w", err)
	}
	return &api.BackendInfo{
		ID:                info.ID,
		Name:              info.Name,
		ServerVersion:     info.ServerVersion,
		Containers:        info.Containers,
		ContainersRunning: info.ContainersRunning,
		ContainersPaused:  info.ContainersPaused,
		ContainersStopped: info.ContainersStopped,
		Images:            info.Images,
		Driver:            info.Driver,
		OperatingSystem:   info.OperatingSystem,
		OSType:            info.OSType,
		Architecture:      info.Architecture,
		NCPU:              info.NCPU,
		MemTotal:          info.MemTotal,
		KernelVersion:     info.KernelVersion,
	}, nil
}

// httpGet performs a raw HTTP GET to the Docker daemon API.
func (s *Server) httpGet(ctx context.Context, path string) (*http.Response, error) {
	host := s.docker.DaemonHost()
	scheme := "http"
	httpHost := host
	httpClient := s.docker.HTTPClient()

	if len(host) > 7 && host[:7] == "unix://" {
		scheme = "http"
		httpHost = "localhost"
	} else if len(host) > 6 && host[:6] == "tcp://" {
		httpHost = host[6:]
	}

	url := fmt.Sprintf("%s://%s/v1.44%s", scheme, httpHost, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return httpClient.Do(req)
}
