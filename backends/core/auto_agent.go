package core

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/sockerless/api"
)

// autoAgentProcs tracks spawned auto-agent processes by container ID.
var autoAgentProcs sync.Map // containerID → *exec.Cmd

// SpawnAutoAgent spawns a local agent process for the container if
// SOCKERLESS_AUTO_AGENT_BIN is set. The agent connects back to the
// backend via reverse WebSocket, enabling real command execution
// without a cloud-deployed agent.
func (s *BaseServer) SpawnAutoAgent(containerID string) error {
	agentBin := os.Getenv("SOCKERLESS_AUTO_AGENT_BIN")
	if agentBin == "" {
		return nil // auto-agent not configured
	}

	callbackBase := os.Getenv("SOCKERLESS_CALLBACK_URL")
	if callbackBase == "" {
		return fmt.Errorf("SOCKERLESS_AUTO_AGENT_BIN set but SOCKERLESS_CALLBACK_URL not set")
	}

	callbackURL := callbackBase + "/internal/v1/agent/connect?id=" + containerID

	s.AgentRegistry.Prepare(containerID)

	// Use container's actual command so the agent exits when the command finishes
	c, ok := s.Store.Containers.Get(containerID)
	if !ok {
		return fmt.Errorf("container %s not found", containerID)
	}
	originalCmd := BuildOriginalCommand(c.Config.Entrypoint, c.Config.Cmd)

	// Build the agent command arguments
	agentArgs := []string{"--callback", callbackURL, "--log-level", "debug"}
	if len(originalCmd) == 0 || IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		// Exec-only container (e.g. CI runner) — keep agent alive with idle process
		agentArgs = append(agentArgs, "--keep-alive", "--", "sleep", "86400")
	} else {
		// Container has a real command — agent exits when command finishes
		agentArgs = append(agentArgs, "--")
		agentArgs = append(agentArgs, originalCmd...)
	}

	cmd := exec.Command(agentBin, agentArgs...)
	cmd.Env = append(os.Environ(),
		"SOCKERLESS_CONTAINER_ID="+containerID,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("auto-agent spawn failed: %w", err)
	}

	autoAgentProcs.Store(containerID, cmd)

	// Wait for agent to connect
	if err := s.AgentRegistry.WaitForAgent(containerID, 10*time.Second); err != nil {
		cmd.Process.Kill()
		_ = cmd.Wait()
		autoAgentProcs.Delete(containerID)
		return fmt.Errorf("auto-agent connect timeout: %w", err)
	}

	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.AgentAddress = "reverse"
	})

	s.Logger.Debug().Str("container", containerID).Msg("auto-agent connected")

	// When the agent process exits (main command finished), stop the container
	go func() {
		_ = cmd.Wait()
		autoAgentProcs.Delete(containerID)
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		s.Logger.Debug().Str("container", containerID).Int("exitCode", exitCode).Msg("auto-agent exited, stopping container")
		s.Store.StopContainer(containerID, exitCode)
	}()

	return nil
}

// StopAutoAgent kills and cleans up an auto-agent process for the container.
func StopAutoAgent(containerID string) {
	if v, ok := autoAgentProcs.LoadAndDelete(containerID); ok {
		cmd := v.(*exec.Cmd)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}
