package aca

import (
	"time"

	core "github.com/sockerless/backend-core"
)

// newAttachStream registers a buffered attach for the container. Writes
// go to its stdin pipe, which the App's buffered HTTP invoke replays as
// the bootstrap's stdin; reads return the invocation's output once it is
// published. One stream per container at a time: a CI runner cycles
// attach → start → stop per stage, and each new stage gets a fresh stream.
func (s *Server) newAttachStream(containerID string, pipe *core.StdinPipe) *core.BufferedAttachStream {
	a := core.NewBufferedAttachStream(pipe, s.attachDeadline(), func() { s.attachStreams.Delete(containerID) })
	s.attachStreams.Store(containerID, a)
	return a
}

// attachDeadline bounds a buffered attach read. The clock starts at
// ContainerStart, before the App has bootstrapped, so the budget covers
// the bootstrap window plus the job-run budget; bounding it by the run
// budget alone strands a reader whenever a healthy bootstrap is slow on a
// contended host.
func (s *Server) attachDeadline() time.Duration {
	bootstrap, err := core.BootstrapTimeoutFromEnv("aca")
	if err != nil || bootstrap <= 0 {
		bootstrap = 90 * time.Second
	}
	run := time.Duration(core.JobTimeoutDefault()) * time.Second
	if run <= 0 {
		run = 600 * time.Second
	}
	return bootstrap + run
}
