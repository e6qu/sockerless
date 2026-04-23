package core

import (
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// MainPIDConventionPath is where the in-container bootstrap writes the
// PID of the user's main subprocess. Pause/unpause target this file so
// they can SIGSTOP/SIGCONT the real workload without racing against the
// bootstrap itself.
const MainPIDConventionPath = "/tmp/.sockerless-mainpid"

// MapPauseErr converts a pause/unpause error from
// RunContainerPauseViaAgent / RunContainerUnpauseViaAgent into the
// corresponding Docker-API error. Shared by every backend that routes
// pause through the reverse-agent so callers see a consistent surface.
func MapPauseErr(err error) error {
	switch err {
	case nil:
		return nil
	case ErrNoReverseAgent:
		return &api.NotImplementedError{Message: "docker pause/unpause requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	case ErrBootstrapNoPIDFile:
		return &api.NotImplementedError{Message: "docker pause/unpause requires a bootstrap that writes " + MainPIDConventionPath}
	default:
		return &api.ServerError{Message: fmt.Sprintf("pause via reverse-agent: %v", err)}
	}
}

// RunContainerPauseViaAgent sends SIGSTOP to the user subprocess via
// the reverse-agent.
func RunContainerPauseViaAgent(reg *ReverseAgentRegistry, containerID string) error {
	return sendSignalToMainPID(reg, containerID, "STOP", "pause-")
}

// RunContainerUnpauseViaAgent sends SIGCONT to the user subprocess via
// the reverse-agent.
func RunContainerUnpauseViaAgent(reg *ReverseAgentRegistry, containerID string) error {
	return sendSignalToMainPID(reg, containerID, "CONT", "unpause-")
}

func sendSignalToMainPID(reg *ReverseAgentRegistry, containerID, signal, sessionPrefix string) error {
	if reg == nil {
		return ErrNoReverseAgent
	}
	// Exit code 64 is reserved for "bootstrap did not participate" so
	// the caller can distinguish a missing convention from a real
	// signalling failure.
	argv := []string{"sh", "-c", fmt.Sprintf(
		"test -r %s || { echo 'sockerless bootstrap PID file not found at %s' >&2; exit 64; }; kill -%s $(cat %s)",
		MainPIDConventionPath, MainPIDConventionPath, signal, MainPIDConventionPath,
	)}
	stdout, stderr, exit, err := reg.RunAndCapture(containerID, sessionPrefix+containerID, argv, nil, "")
	if err != nil {
		return err
	}
	if exit != 0 {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(stdout))
		}
		if exit == 64 {
			return fmt.Errorf("%w: %s", ErrBootstrapNoPIDFile, msg)
		}
		return fmt.Errorf("kill -%s failed (exit %d): %s", signal, exit, msg)
	}
	return nil
}
