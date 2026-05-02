package poller

import (
	"bytes"
	"io"
)

// readAll is the dependency-free equivalent of io.ReadAll. Kept local
// to avoid pulling additional packages — the dispatcher module's only
// external dep is BurntSushi/toml.
func readAll(r io.Reader) []byte {
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.Bytes()
}
