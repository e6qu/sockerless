package docker

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sockerless/api"
)

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.Info()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// Info returns backend system information.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.docker.Info(context.Background())
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
