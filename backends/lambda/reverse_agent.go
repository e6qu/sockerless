package lambda

// disconnectReverseAgent signals the in-function reverse agent for the
// given container to exit, causing the Lambda invocation to return.
// This is currently a no-op placeholder — the agent-as-handler pattern
// that gives Lambda containers a reverse agent to disconnect is
// introduced in P86-005. Until then, ContainerStop relies on the
// best-effort UpdateFunctionConfiguration(Timeout=1) and the 15-minute
// Lambda hard cap as the only termination paths.
//
// Once P86-005 lands, this function should look up the container's
// reverse agent session (via a new registry on the BaseServer) and
// close the WebSocket.
func (s *Server) disconnectReverseAgent(containerID string) {
	_ = containerID
}
