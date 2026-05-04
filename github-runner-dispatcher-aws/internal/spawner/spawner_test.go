package spawner

import (
	"context"
	"strings"
	"testing"
)

func TestSpawnRequiredFields(t *testing.T) {
	cases := map[string]Request{
		"missing host":   {Image: "img", RegToken: "tok", Repo: "o/r", RunnerName: "rn"},
		"missing image":  {DockerHost: "tcp://x", RegToken: "tok", Repo: "o/r", RunnerName: "rn"},
		"missing token":  {DockerHost: "tcp://x", Image: "img", Repo: "o/r", RunnerName: "rn"},
		"missing repo":   {DockerHost: "tcp://x", Image: "img", RegToken: "tok", RunnerName: "rn"},
		"missing runner": {DockerHost: "tcp://x", Image: "img", RegToken: "tok", Repo: "o/r"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Spawn(context.Background(), req); err == nil {
				t.Fatalf("Spawn should fail: %s", name)
			}
		})
	}
}

func TestLivenessUnreachable(t *testing.T) {
	// tcp://127.0.0.1:1 is reserved (TCP port reserved by RFC 6335);
	// docker info will fail to connect — exactly the unreachable
	// path Liveness should surface as an error.
	err := Liveness(context.Background(), "tcp://127.0.0.1:1")
	if err == nil {
		t.Fatal("Liveness against unreachable host should error")
	}
	if !strings.Contains(err.Error(), "docker info") {
		t.Errorf("error should mention docker info: %v", err)
	}
}
