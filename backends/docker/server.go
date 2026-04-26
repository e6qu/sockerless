package docker

import (
	"context"
	"fmt"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Docker passthrough backend server.
type Server struct {
	*core.BaseServer
	docker *dockerclient.Client
}

// NewServer creates a new Docker backend server. BUG-830: previously
// the BackendDescriptor hardcoded `NCPU: 4` and `MemTotal: 8 GB` as
// fallbacks for when the daemon's `/info` query failed. Per the
// project no-fallbacks rule, that meant `docker info` could ship
// fabricated CPU/memory values to clients without any signal. Now the
// daemon is queried at startup with a 5-second deadline; failure is
// fatal so the operator gets a clear error instead of a sockerless
// pretending the daemon has 4 CPUs and 8 GB RAM.
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

	// Query the underlying daemon for real CPU / memory / kernel /
	// architecture so the BackendDescriptor reflects truth from the
	// first /info request, not a placeholder.
	infoCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := cli.Info(infoCtx)
	if err != nil {
		return nil, fmt.Errorf("docker daemon Info() failed (host=%q): %w — sockerless docker backend requires a reachable daemon at startup", cli.DaemonHost(), err)
	}

	osType := info.OSType
	if osType == "" {
		osType = "linux"
	}
	arch := info.Architecture
	if arch == "" {
		arch = "amd64"
	}
	operatingSystem := info.OperatingSystem
	if operatingSystem == "" {
		operatingSystem = "Docker Desktop"
	}
	serverVersion := info.ServerVersion
	if serverVersion == "" {
		serverVersion = "0.1.0"
	}

	s := &Server{
		docker: cli,
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "docker-local",
		Name:            "sockerless-docker",
		ServerVersion:   serverVersion,
		Driver:          "docker",
		OperatingSystem: operatingSystem,
		OSType:          osType,
		Architecture:    arch,
		NCPU:            info.NCPU,
		MemTotal:        info.MemTotal,
	}, logger)
	s.SetSelf(s)

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
