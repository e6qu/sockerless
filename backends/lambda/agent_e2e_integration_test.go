//go:build integration

package lambda

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

// TestLambdaAgentE2E_ReverseAgent is the end-to-end proof: drive the
// Lambda backend → AWS simulator → real sockerless-lambda-bootstrap
// container → reverse-agent WebSocket → `docker exec` round-trip.
// Fully offline against the simulator; no docker build/push to
// insecure registries (the backend is configured with
// PrebuiltOverlayImage so ContainerCreate uses the test image
// directly).
//
// Requires the integration TestMain to have brought up the backend +
// docker client (SOCKERLESS_TEST_TARGET=sim|cloud). Needs a real
// docker daemon and a reachable host.docker.internal.
func TestLambdaAgentE2E_ReverseAgent(t *testing.T) {
	ctx := context.Background()

	if agentTestImageName == "" {
		t.Fatal("agentTestImageName unset — TestMain should have built it")
	}

	testID := generateTestID()
	// The simulator's Lambda invocation path runs the container with
	// sleep-long enough to keep the reverse-agent session alive while
	// we run the exec round-trip.
	resp, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"sleep", "300"},
		},
		nil, nil, nil, "agent_e2e_"+testID,
	)
	if err != nil {
		t.Fatalf("container create failed: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true}) })

	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("container start failed: %v", err)
	}

	// Poll docker exec until the bootstrap has dialed the reverse-agent
	// WS + registered. The first attempts will return 126 (no reverse
	// agent session); once connected, the exec proxies through the
	// real WebSocket → real bootstrap → spawned subprocess → stdout.
	var gotStdout []byte
	var lastExitCode int
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		exec, err := dockerClient.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
			Cmd:          []string{"echo", "hello-from-exec"},
			AttachStdout: true,
			AttachStderr: true,
		})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		hr, err := dockerClient.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		raw, _ := io.ReadAll(hr.Reader)
		hr.Close()

		// raw is Docker-multiplexed stdout+stderr; demux to get just stdout.
		gotStdout = demuxDockerStream(raw)

		inspect, err := dockerClient.ContainerExecInspect(ctx, exec.ID)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		lastExitCode = inspect.ExitCode
		if lastExitCode == 0 && bytes.Contains(gotStdout, []byte("hello-from-exec")) {
			// Success — the reverse-agent path delivered stdout.
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !bytes.Contains(gotStdout, []byte("hello-from-exec")) {
		t.Fatalf(
			"docker exec never round-tripped via the reverse-agent within 60s\nlast exit: %d\nlast stdout: %q",
			lastExitCode, string(gotStdout),
		)
	}

	// Post-kill path: stop the container → reverse-agent drops →
	// subsequent exec returns an error (or exit 126) since no session.
	stopTimeout := 1
	if err := dockerClient.ContainerStop(ctx, resp.ID, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		// Stop may report error on already-stopped; OK to proceed.
		t.Logf("container stop returned %v (continuing — may have already exited)", err)
	}

	// Poll until the post-stop state has propagated: a new exec must FAIL —
	// either ExecCreate rejects it ("No such container" / "is not running"),
	// or the exec exits non-zero (reverse-agent session dropped). A fixed
	// sleep races a loaded runner where the registry drop hasn't propagated.
	pollDeadline := time.Now().Add(30 * time.Second)
	for {
		afterExec, err := dockerClient.ContainerExecCreate(ctx, resp.ID, container.ExecOptions{
			Cmd:          []string{"echo", "should-not-succeed"},
			AttachStdout: true,
		})
		if err != nil {
			if strings.Contains(err.Error(), "No such container") ||
				strings.Contains(err.Error(), "is not running") {
				return // expected: the exec was rejected
			}
			t.Fatalf("post-stop exec create returned unexpected error: %v", err)
		}
		if hr2, aerr := dockerClient.ContainerExecAttach(ctx, afterExec.ID, container.ExecAttachOptions{}); aerr == nil {
			io.Copy(io.Discard, hr2.Reader)
			hr2.Close()
		}
		if inspect2, _ := dockerClient.ContainerExecInspect(ctx, afterExec.ID); inspect2.ExitCode != 0 {
			return // expected: the exec failed (agent session gone)
		}
		if time.Now().After(pollDeadline) {
			t.Error("post-stop exec still returned exit 0 after 30s; expected non-zero (reverse-agent session gone)")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// demuxDockerStream decodes a Docker multiplexed stream (8-byte
// header per frame: [0]=stream, [4:8]=payload length) and returns the
// stdout payload. If the input has no valid header (e.g. TTY mode),
// returns it as-is.
func demuxDockerStream(raw []byte) []byte {
	var out bytes.Buffer
	for i := 0; i+8 <= len(raw); {
		streamType := raw[i]
		length := int(binary.BigEndian.Uint32(raw[i+4 : i+8]))
		start := i + 8
		end := start + length
		if end > len(raw) {
			end = len(raw)
		}
		if streamType == 1 { // stdout
			out.Write(raw[start:end])
		}
		i = end
	}
	if out.Len() == 0 {
		// No valid frames parsed — probably TTY or raw mode.
		return raw
	}
	return out.Bytes()
}
