package simulator

import (
	"strings"
	"testing"
)

func TestDrainImagePullSurfacesStreamErrors(t *testing.T) {
	// Pull errors arrive as JSON events inside a 200 body.
	failed := `{"status":"Pulling from library/node"}
{"errorDetail":{"message":"received unexpected HTTP status: 503 Service Unavailable"},"error":"received unexpected HTTP status: 503 Service Unavailable"}
`
	err := drainImagePull(strings.NewReader(failed), "public.ecr.aws/docker/library/node:20-alpine")
	if err == nil {
		t.Fatal("failed pull stream must surface an error")
	}
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "node:20-alpine") {
		t.Errorf("error should carry the stream failure + image: %v", err)
	}

	ok := `{"status":"Pulling from library/alpine"}
{"status":"Pull complete"}
{"status":"Status: Downloaded newer image for alpine:latest"}
`
	if err := drainImagePull(strings.NewReader(ok), "alpine:latest"); err != nil {
		t.Errorf("clean pull stream errored: %v", err)
	}

	if err := drainImagePull(strings.NewReader("not-json"), "x"); err == nil {
		t.Error("malformed stream should error, not pass silently")
	}
}
