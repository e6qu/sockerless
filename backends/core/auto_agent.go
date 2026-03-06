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

	// Spawn agent in callback mode with a long-lived idle process
	cmd := exec.Command(agentBin,
		"--callback", callbackURL,
		"--keep-alive",
		"--log-level", "debug",
		"--",
		"sleep", "86400",
	)
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
		cmd.Wait()
		autoAgentProcs.Delete(containerID)
		return fmt.Errorf("auto-agent connect timeout: %w", err)
	}

	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.AgentAddress = "reverse"
	})

	s.Logger.Debug().Str("container", containerID).Msg("auto-agent connected")

	// Reap process on exit
	go func() {
		cmd.Wait()
		autoAgentProcs.Delete(containerID)
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
