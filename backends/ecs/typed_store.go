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
