package sdktests

import (
	"testing"
)

// TestGitData is skipped: bleephub serves repository git data through the git
// smart-HTTP protocol (git_http.go), not the GitHub Git Data REST API. There
// are no POST handlers for git/blobs, git/trees, git/commits, or git/refs, and
// no GET for git/refs (only DELETE git/refs and GET git/blobs|trees by SHA),
// so go-github's Git.CreateRef / GetRef / CreateBlob / CreateTree /
// CreateCommit have no endpoint to hit. Seeding objects would require a real
// git push over smart-HTTP, which is outside the REST SDK's surface.
func TestGitData(t *testing.T) {
	t.Skip("bleephub has no Git Data REST API (no POST git/blobs|trees|commits|refs, no GET git/refs); git data is served via smart-HTTP, not the REST Git Data endpoints go-github's Git service targets")
}
