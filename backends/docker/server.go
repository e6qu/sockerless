package docker

import (
	dockerclient "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Docker passthrough backend server.
type Server struct {
	*core.BaseServer
	docker *dockerclient.Client
}

// NewServer creates a new Docker backend server.
func NewServer(logger zerolog.Logger, dockerHost string) (*Server, error) {
	opts := []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
	}
	if dockerHost != "" {
		opts = append(opts, dockerclient.WithHost(dockerHost))
	} else {
		opts = append(opts, dockerclient.FromEnv)
	}

	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	s := &Server{
		docker: cli,
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "docker-local",
		Name:            "sockerless-docker",
		ServerVersion:   "0.1.0",
		Driver:          "docker",
		OperatingSystem: "Docker Desktop",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            4,
		MemTotal:        8589934592,
	}, logger)
	s.BaseServer.SetSelf(s)

	dockerHostStr := cli.DaemonHost()
	resources := map[string]string{}
	if dockerHostStr != "" {
		resources["docker_host"] = dockerHostStr
	}
	s.ProviderInfo = &core.ProviderInfo{
		Provider:  "docker",
		Mode:      "local",
		Resources: resources,
	}

	registerUI(s.BaseServer)

	return s, nil
}

// Aliases for core helpers used throughout this backend.
var (
	writeJSON  = core.WriteJSON
	writeError = core.WriteError
	readJSON   = core.ReadJSON
)
