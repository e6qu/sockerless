package lambda

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Lambda backend server.
type Server struct {
	*core.BaseServer
	config    Config
	aws       *AWSClients
	Lambda    *core.StateStore[LambdaState]
	ipCounter atomic.Int32
}

// NewServer creates a new Lambda backend server.
func NewServer(config Config, awsClients *AWSClients, logger zerolog.Logger) *Server {
	s := &Server{
		config: config,
		aws:    awsClients,
		Lambda: core.NewStateStore[LambdaState](),
	}
	s.ipCounter.Store(2)

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
	}, logger)
	s.SetSelf(s)

	mode := "cloud"
	if config.EndpointURL != "" {
		mode = "custom-endpoint"
	}
	s.ProviderInfo = &core.ProviderInfo{
		Provider: "aws",
		Mode:     mode,
		Region:   config.Region,
		Endpoint: config.EndpointURL,
		Resources: map[string]string{
			"role_arn":    config.RoleARN,
			"memory_size": fmt.Sprintf("%d", config.MemorySize),
			"timeout":     fmt.Sprintf("%d", config.Timeout),
		},
	}

	registerUI(s.BaseServer)

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
