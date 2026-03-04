package docker

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sockerless/api"
)

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.Info(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// Info returns backend system information.
func (s *Server) Info(ctx context.Context) (*api.BackendInfo, error) {
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

	// Handle unix:// socket hosts
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
