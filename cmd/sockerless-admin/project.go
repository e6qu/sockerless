package main

import (
	"fmt"
	"net"
	"regexp"
	"sync"
)

var validProjectNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// isValidProjectName checks if a project name contains only allowed characters.
func isValidProjectName(name string) bool {
	return validProjectNameRE.MatchString(name)
}

// CloudType represents a supported cloud provider.
type CloudType string

const (
	CloudAWS   CloudType = "aws"
	CloudGCP   CloudType = "gcp"
	CloudAzure CloudType = "azure"
)

// BackendType represents a supported backend type.
type BackendType string

const (
	BackendECS      BackendType = "ecs"
	BackendLambda   BackendType = "lambda"
	BackendCloudRun BackendType = "cloudrun"
	BackendGCF      BackendType = "gcf"
	BackendACA      BackendType = "aca"
	BackendAZF      BackendType = "azf"
)

// ProjectConfig defines a project configuration as a name + a list of
// independently lifecyclable Instances (sim / backend / bleephub). Each
// instance carries its own cloud / backend / port / config.
type ProjectConfig struct {
	Name      string     `json:"name" yaml:"name"`
	CreatedAt string     `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Instances []Instance `json:"instances,omitempty" yaml:"instances,omitempty"`
}

// ProjectInstanceStatus reports per-instance runtime status under a
// ProjectStatus. Distinct from cmd/sockerless-admin's other
// InstanceStatus type (in instance_status.go) which serves the
// independent /v1/instance/{name}/status surface.
type ProjectInstanceStatus struct {
	Instance
	Status string `json:"status"`
}

// ProjectStatus combines config with per-instance runtime status. The
// aggregate Status is "running" when every instance is running,
// "stopped" when every instance is stopped, "partial" when some are
// running, "starting"/"stopping" during a transition, "failed" when any
// instance is in a failed state.
type ProjectStatus struct {
	ProjectConfig
	Status    string                  `json:"status"`
	Instances []ProjectInstanceStatus `json:"instance_statuses,omitempty"`
}

// ProjectConnection holds Docker/Podman connection info.
type ProjectConnection struct {
	DockerHost       string `json:"docker_host"`
	EnvExport        string `json:"env_export"`
	PodmanConnection string `json:"podman_connection"`
	SimulatorAddr    string `json:"simulator_addr"`
	BackendAddr      string `json:"backend_addr"`
}

// ValidClouds returns all valid cloud types.
func ValidClouds() []CloudType {
	return []CloudType{CloudAWS, CloudGCP, CloudAzure}
}

// ValidBackends returns the valid backends for a cloud.
func ValidBackends(cloud CloudType) []BackendType {
	switch cloud {
	case CloudAWS:
		return []BackendType{BackendECS, BackendLambda}
	case CloudGCP:
		return []BackendType{BackendCloudRun, BackendGCF}
	case CloudAzure:
		return []BackendType{BackendACA, BackendAZF}
	default:
		return nil
	}
}

// IsValidCloud checks if a cloud type is valid.
func IsValidCloud(cloud CloudType) bool {
	switch cloud {
	case CloudAWS, CloudGCP, CloudAzure:
		return true
	default:
		return false
	}
}

// IsValidBackend checks if a backend type is valid for the given cloud.
func IsValidBackend(cloud CloudType, backend BackendType) bool {
	for _, b := range ValidBackends(cloud) {
		if b == backend {
			return true
		}
	}
	return false
}

// SimulatorBinary returns the simulator binary name for a cloud.
func SimulatorBinary(cloud CloudType) string {
	switch cloud {
	case CloudAWS:
		return "simulator-aws"
	case CloudGCP:
		return "simulator-gcp"
	case CloudAzure:
		return "simulator-azure"
	default:
		return ""
	}
}

// BackendBinary returns the backend binary name for a backend type.
func BackendBinary(backend BackendType) string {
	switch backend {
	case BackendECS:
		return "sockerless-backend-ecs"
	case BackendLambda:
		return "sockerless-backend-lambda"
	case BackendCloudRun:
		return "sockerless-backend-cloudrun"
	case BackendGCF:
		return "sockerless-backend-gcf"
	case BackendACA:
		return "sockerless-backend-aca"
	case BackendAZF:
		return "sockerless-backend-azf"
	default:
		return ""
	}
}

// instanceProcessName returns the ProcessManager process name for a
// given (project, instance) pair. Used by ProjectManager to register +
// look up processes when driving lifecycle from the topology Instances.
func instanceProcessName(projectName, instanceName string) string {
	return fmt.Sprintf("proj-%s-%s", projectName, instanceName)
}

// PortAllocator allocates ephemeral ports and tracks them per project.
type PortAllocator struct {
	mu    sync.Mutex
	taken map[int]string // port -> project name
}

// NewPortAllocator creates a new PortAllocator.
func NewPortAllocator() *PortAllocator {
	return &PortAllocator{
		taken: make(map[int]string),
	}
}

// Allocate allocates n ephemeral ports for a project.
func (pa *PortAllocator) Allocate(project string, n int) ([]int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	ports := make([]int, 0, n)
	listeners := make([]net.Listener, 0, n)

	// Open n listeners to get unique ports
	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			// Close already-opened listeners
			for _, l := range listeners {
				_ = l.Close()
			}
			return nil, fmt.Errorf("failed to allocate port: %w", err)
		}
		listeners = append(listeners, ln)
		port := ln.Addr().(*net.TCPAddr).Port
		ports = append(ports, port)
	}

	// Close listeners so the ports can be used
	for _, ln := range listeners {
		_ = ln.Close()
	}

	// Track ports
	for _, port := range ports {
		pa.taken[port] = project
	}

	return ports, nil
}

// Reserve records specific ports for a project.
func (pa *PortAllocator) Reserve(project string, ports []int) error {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	for _, port := range ports {
		if owner, ok := pa.taken[port]; ok && owner != project {
			return fmt.Errorf("port %d already taken by project %q", port, owner)
		}
	}
	for _, port := range ports {
		if port > 0 {
			pa.taken[port] = project
		}
	}
	return nil
}

// Release releases all ports for a project.
func (pa *PortAllocator) Release(project string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	for port, owner := range pa.taken {
		if owner == project {
			delete(pa.taken, port)
		}
	}
}

// IsPortTaken checks if a port is already allocated.
func (pa *PortAllocator) IsPortTaken(port int) bool {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	_, ok := pa.taken[port]
	return ok
}
