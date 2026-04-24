package lambda

// disconnectReverseAgent signals the in-function reverse agent for the
// given container to exit. The registry's Drop does the real work —
// lives in backend-core.ReverseAgentRegistry.
func (s *Server) disconnectReverseAgent(containerID string) {
	if s.reverseAgents == nil {
		return
	}
	s.reverseAgents.Drop(containerID)
}
