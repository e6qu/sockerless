package lambda

import (
	"context"
	"errors"
	"net"

	"github.com/rs/zerolog"
)

// errNoReverseAgent surfaces when a container has no live reverse-agent
// session. Phase 86 D.3: the Lambda bootstrap dials on invoke start;
// if it hasn't connected yet (cold start) or the container was killed,
// exec/attach fails with this error rather than hanging.
var errNoReverseAgent = errors.New("no reverse-agent session registered for container")

// lambdaExecDriver routes `docker exec` through the reverse-agent
// WebSocket that the Lambda bootstrap dialed at init time (Phase 86
// D.3 / D.4). When no session is registered for the container — the
// bootstrap may not have dialed yet, or the container was killed —
// the driver returns exit code 126 so the caller sees a clear failure.
type lambdaExecDriver struct {
	registry *reverseAgentRegistry
	logger   zerolog.Logger
}

func (d *lambdaExecDriver) Exec(
	_ context.Context,
	containerID string,
	execID string,
	cmd []string,
	env []string,
	workDir string,
	tty bool,
	conn net.Conn,
) int {
	if d.registry == nil {
		d.logger.Warn().Str("container", containerID).Msg("no reverse-agent registry")
		return 126
	}
	rc, ok := d.registry.resolve(containerID)
	if !ok {
		d.logger.Warn().
			Str("container", containerID).
			Msg("no reverse-agent session (container not up, or killed)")
		return 126
	}

	// BridgeExec drives the session until the child process exits on
	// the bootstrap side, then returns the exit code. Session ID
	// uniquely tags this exec invocation inside the multiplexed WS.
	return rc.BridgeExec(conn, execID, cmd, env, workDir, tty)
}

// lambdaStreamDriver wires `docker attach` + `docker logs` through the
// reverse-agent when one is connected. `docker logs` (non-follow) still
// reads from CloudWatch via the BaseServer's log path; this driver
// only fills in `Attach` for the reverse-agent case.
type lambdaStreamDriver struct {
	registry *reverseAgentRegistry
	logger   zerolog.Logger
}

func (d *lambdaStreamDriver) Attach(_ context.Context, containerID string, tty bool, conn net.Conn) error {
	if d.registry == nil {
		return errNoReverseAgent
	}
	rc, ok := d.registry.resolve(containerID)
	if !ok {
		return errNoReverseAgent
	}
	// Use a synthetic session ID — `docker attach` is just one
	// persistent session per container; the bootstrap side treats it
	// like a long-lived exec of the user's entrypoint.
	rc.BridgeAttach(conn, "attach-"+containerID, tty)
	return nil
}

func (d *lambdaStreamDriver) LogBytes(_ string) []byte             { return nil }
func (d *lambdaStreamDriver) LogSubscribe(_, _ string) chan []byte { return nil }
func (d *lambdaStreamDriver) LogUnsubscribe(_, _ string)           {}
