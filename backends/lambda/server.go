package lambda

import (
	"context"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Lambda backend server.
type Server struct {
	*core.BaseServer
	config Config
	aws    *AWSClients
	Lambda *core.StateStore[LambdaState]
}

// NewServer creates a new Lambda backend server.
func NewServer(config Config, awsClients *AWSClients, logger zerolog.Logger) *Server {
	s := &Server{
		config: config,
		aws:    awsClients,
		Lambda: core.NewStateStore[LambdaState](),
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "lambda-backend",
		Name:            "sockerless-lambda",
		ServerVersion:   "0.1.0",
		Driver:          "lambda",
		OperatingSystem: "AWS Lambda",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            2,
		MemTotal:        4294967296,
	}, core.RouteOverrides{
		ContainerCreate:  s.handleContainerCreate,
		ContainerStart:   s.handleContainerStart,
		ContainerStop:    s.handleContainerStop,
		ContainerKill:    s.handleContainerKill,
		ContainerRemove:  s.handleContainerRemove,
		ContainerLogs:    s.handleContainerLogs,
		ContainerPrune: s.handleContainerPrune,
		ImagePull:      s.handleImagePull,
		ImageLoad:      s.handleImageLoad,
	}, logger)

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
