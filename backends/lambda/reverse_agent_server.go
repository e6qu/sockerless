package lambda

import (
	"github.com/rs/zerolog"

	"github.com/sockerless/agent"
	core "github.com/sockerless/backend-core"
)

// reverseAgentRegistry is a thin alias so existing call sites keep
// compiling. The real implementation lives in
// backend-core.ReverseAgentRegistry so Cloud Run + ACA can share it.
type reverseAgentRegistry = core.ReverseAgentRegistry

func newReverseAgentRegistry() *reverseAgentRegistry {
	return core.NewReverseAgentRegistry()
}

func (s *Server) resolveReverseAgent(containerID string) (*agent.ReverseAgentConn, bool) {
	return s.reverseAgents.Resolve(containerID)
}

// registerReverseAgentRoutes mounts the /v1/lambda/reverse endpoint.
func (s *Server) registerReverseAgentRoutes(logger zerolog.Logger) {
	s.Mux.HandleFunc("/v1/lambda/reverse", core.HandleReverseAgentWS(s.reverseAgents, logger))
}
