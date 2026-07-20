package ecs

// Typed accessors for values stored in the sync.Map-backed stores
// (Store.WaitChs, Store.TmpfsDirs, stdinPipes). Each store only ever holds
// one concrete type, so a failed assertion indicates a programming error;
// these helpers guard the assertion rather than panic, treating an
// unexpected type as a no-op (the value can't have come from the matching
// writer). Use them everywhere a value comes back out of those maps.

// closeWaitCh closes a wait channel pulled from Store.WaitChs.
func closeWaitCh(v any) {
	if ch, ok := v.(chan struct{}); ok {
		close(ch)
	}
}

// asWaitCh returns the wait channel pulled from Store.WaitChs, or nil.
func asWaitCh(v any) chan struct{} {
	ch, _ := v.(chan struct{})
	return ch
}

// beginWaitCycle gives each cloud task launch its own completion channel. A
// restarted Docker container keeps its ID, but its previous Amazon ECS task
// poller must never own or close the new launch's channel.
func (s *Server) beginWaitCycle(containerID string) chan struct{} {
	if previous, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
		closeWaitCh(previous)
	}
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(containerID, exitCh)
	return exitCh
}

// finishWaitCycle closes only the channel belonging to the task cycle that
// finished. CompareAndDelete prevents a delayed poll for an older Amazon ECS
// task from deleting a restarted container's current completion channel.
func (s *Server) finishWaitCycle(containerID string, exitCh chan struct{}) {
	if exitCh != nil && s.Store.WaitChs.CompareAndDelete(containerID, exitCh) {
		close(exitCh)
	}
}

// asStdinPipe returns the stdin pipe pulled from stdinPipes, or nil.
func asStdinPipe(v any) *stdinPipe {
	p, _ := v.(*stdinPipe)
	return p
}

// asStringSlice returns the directory slice pulled from Store.TmpfsDirs, or nil.
func asStringSlice(v any) []string {
	s, _ := v.([]string)
	return s
}
