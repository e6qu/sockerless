package azf

import (
	"github.com/sockerless/api"
)

// PodStart starts every member through the cloud-aware ContainerStart.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	return s.CloudPodStart(name, s.ContainerStart)
}

// PodStop stops every running member through the cloud-aware ContainerStop.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	return s.CloudPodStop(name, timeout, s.ContainerStop)
}

// PodKill signals every running member through the cloud-aware ContainerKill.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	return s.CloudPodKill(name, signal, s.ContainerKill)
}

// PodRemove removes every member through the cloud-aware ContainerRemove,
// so no Function App is orphaned, then deletes the pod.
func (s *Server) PodRemove(name string, force bool) error {
	return s.CloudPodRemove(name, force, s.ContainerRemove)
}
