package cloudrun

// Typed accessors for values stored in the sync.Map-backed stores
// (Store.WaitChs, Store.TmpfsDirs, stdinPipes, attachStreams,
// networkServices). Each store only ever holds one concrete type, so a
// failed assertion indicates a programming error; these helpers guard the
// assertion rather than panic, treating an unexpected type as a no-op (the
// value can't have come from the matching writer). Use them everywhere a
// value comes back out of those maps.

// closeWaitCh closes a wait channel pulled from Store.WaitChs.
func closeWaitCh(v any) {
	if ch, ok := v.(chan struct{}); ok {
		close(ch)
	}
}

// asStdinPipe returns the stdin pipe pulled from stdinPipes, or nil.
func asStdinPipe(v any) *stdinPipe {
	p, _ := v.(*stdinPipe)
	return p
}

// asAttachStream returns the attach stream pulled from attachStreams, or nil.
func asAttachStream(v any) *attachStream {
	a, _ := v.(*attachStream)
	return a
}

// asStringSlice returns the string slice pulled from Store.TmpfsDirs or
// networkServices, or nil.
func asStringSlice(v any) []string {
	s, _ := v.([]string)
	return s
}
