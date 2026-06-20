package azf

// disconnectReverseAgent signals the in-function reverse agent for the
// given container to exit and drops its registry session. The registry's
// Drop does the real work — lives in backend-core.ReverseAgentRegistry.
// Without this, ContainerStop/Kill/Remove leak the reverse-agent WS
// session for every terminated container.
func (s *Server) disconnectReverseAgent(containerID string) {
	if s.reverseAgents == nil {
		return
	}
	s.reverseAgents.Drop(containerID)
}
