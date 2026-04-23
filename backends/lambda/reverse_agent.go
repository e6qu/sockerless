package lambda

// disconnectReverseAgent signals the in-function reverse agent for the
// given container to exit. Delegates to the registry's Drop — Phase 96
// moved the real implementation to backend-core.ReverseAgentRegistry.
func (s *Server) disconnectReverseAgent(containerID string) {
	if s.reverseAgents == nil {
		return
	}
	s.reverseAgents.Drop(containerID)
}
