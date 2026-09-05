package azf

import (
	"time"

	core "github.com/sockerless/backend-core"
)

// newAttachStream registers a buffered attach for the container. Writes
// go to its stdin pipe, which the deferred invoke replays as the
// bootstrap's stdin; reads return the invocation's output once it is
// published. One stream per container at a time: a CI runner cycles
// attach → start → stop per stage, and each new stage gets a fresh stream.
func (s *Server) newAttachStream(containerID string, pipe *core.StdinPipe) *core.BufferedAttachStream {
	a := core.NewBufferedAttachStream(pipe, s.attachDeadline(), func() { s.attachStreams.Delete(containerID) })
	s.attachStreams.Store(containerID, a)
	return a
}

// attachDeadline bounds a buffered attach read: the configured
// AttachTimeout, else the invoke Timeout, else 600 seconds, plus the
// bootstrap window. The clock starts at ContainerStart, before the
// function has bootstrapped, so bounding it by the invoke budget alone
// strands a reader whenever a healthy bootstrap is slow on a contended
// host: it would EOF with empty output before the function could publish.
func (s *Server) attachDeadline() time.Duration {
	secs := s.config.AttachTimeout
	if secs <= 0 {
		secs = s.config.Timeout
	}
	if secs <= 0 {
		secs = 600
	}
	return s.azfBootstrapTimeout() + time.Duration(secs)*time.Second
}
