package core

import (
	"github.com/sockerless/api"
)

// BuildAgentEntrypoint wraps the container entrypoint/command with the agent.
// Returns (entrypoint, command) for use in the container definition.
func BuildAgentEntrypoint(config api.ContainerConfig) (entrypoint, command []string) {
	agentBin := "/sockerless/bin/sockerless-agent"
	agentPort := "9111"

	// Detect tail -f /dev/null pattern (used by GitLab Runner, GitHub Actions)
	if IsTailDevNull(config.Entrypoint, config.Cmd) {
		// Replace with agent in keep-alive mode — the agent itself acts as the idle process
		return []string{agentBin, "--addr", ":" + agentPort, "--keep-alive", "--",
			"tail", "-f", "/dev/null"}, nil
	}

	// General case: wrap the original entrypoint+command with agent in keep-alive mode
	originalCmd := BuildOriginalCommand(config.Entrypoint, config.Cmd)
	if len(originalCmd) == 0 {
		// No entrypoint or command — just run agent in keep-alive with shell
		return []string{agentBin, "--addr", ":" + agentPort, "--keep-alive", "--",
			"/bin/sh"}, nil
	}

	// Agent wraps the original command
	args := []string{agentBin, "--addr", ":" + agentPort, "--keep-alive", "--"}
	args = append(args, originalCmd...)
	return args, nil
}

// BuildOriginalCommand combines entrypoint and cmd into a single command slice.
func BuildOriginalCommand(entrypoint, cmd []string) []string {
	var result []string
	result = append(result, entrypoint...)
	result = append(result, cmd...)
	return result
}

// BuildAgentCallbackEntrypoint wraps the container entrypoint/command with the agent
// in callback mode. Instead of listening on a port, the agent dials out to callbackURL.
func BuildAgentCallbackEntrypoint(config api.ContainerConfig, callbackURL string) (entrypoint []string) {
	agentBin := "/sockerless/bin/sockerless-agent"

	originalCmd := BuildOriginalCommand(config.Entrypoint, config.Cmd)
	if len(originalCmd) == 0 {
		originalCmd = []string{"/bin/sh"}
	}

	args := []string{agentBin, "--callback", callbackURL, "--keep-alive", "--"}
	return append(args, originalCmd...)
}

// IsTailDevNull detects variations of the "tail -f /dev/null" idle pattern.
func IsTailDevNull(entrypoint, cmd []string) bool {
	combined := BuildOriginalCommand(entrypoint, cmd)
	if len(combined) == 0 {
		return false
	}

	// Direct: ["tail", "-f", "/dev/null"]
	if len(combined) == 3 && combined[0] == "tail" && combined[1] == "-f" && combined[2] == "/dev/null" {
		return true
	}

	// Shell wrapped: ["sh", "-c", "tail -f /dev/null"]
	if len(combined) == 3 && (combined[0] == "sh" || combined[0] == "/bin/sh" || combined[0] == "bash" || combined[0] == "/bin/bash") {
		if combined[1] == "-c" {
			inner := combined[2]
			if inner == "tail -f /dev/null" || inner == "tail -f /dev/null\n" {
				return true
			}
		}
	}

	return false
}
