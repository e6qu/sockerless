package lambda

// disconnectReverseAgent signals the in-function reverse agent for the
// given container to exit, causing the Lambda invocation to return.
// Phase-86 D.3: real implementation backed by the reverse-agent
// registry. Closing the WebSocket is enough — the bootstrap's
// `sendHeartbeats` + `serveReverseAgent` goroutines unblock when
// `rc.Done()` closes, and the next `/next` poll will see the socket
// gone and exit.
func (s *Server) disconnectReverseAgent(containerID string) {
	if s.reverseAgents == nil {
		return
	}
	s.reverseAgents.drop(containerID)
}
