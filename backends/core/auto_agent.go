package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sockerless/api"
)

// autoAgentEntry tracks a spawned auto-agent process and its completion state.
type autoAgentEntry struct {
	cmd  *exec.Cmd
	done chan struct{} // closed when cmd.Wait() returns
}

// autoAgentProcs tracks spawned auto-agent processes by container ID.
var autoAgentProcs sync.Map // containerID → *autoAgentEntry

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

	// Set up volume bind mount path mappings so commands can access
	// volume data through translated paths.
	s.setupVolumePathMappings(containerID, c)

	// Build the agent command arguments — always use --keep-alive to run the command
	agentArgs := []string{"--callback", callbackURL, "--keep-alive", "--log-level", "debug", "--"}
	isExecOnly := len(originalCmd) == 0 ||
		IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) ||
		(c.Config.OpenStdin && c.Config.Tty) // interactive shell — needs long-lived process
	if isExecOnly {
		// Exec-only container (e.g. CI runner, interactive shell) — use long-lived idle process
		agentArgs = append(agentArgs, "sleep", "86400")
	} else {
		// Translate volume paths in the main command
		originalCmd = translateContainerPaths(containerID, originalCmd, s.Store)
		agentArgs = append(agentArgs, originalCmd...)
	}

	cmd := exec.Command(agentBin, agentArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		"SOCKERLESS_CONTAINER_ID="+containerID,
	)

	// Capture main process stdout for log retrieval; agent stderr goes to our stderr
	var stdoutBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("auto-agent spawn failed: %w", err)
	}

	entry := &autoAgentEntry{cmd: cmd, done: make(chan struct{})}
	autoAgentProcs.Store(containerID, entry)

	// Wait for agent to connect
	if err := s.AgentRegistry.WaitForAgent(containerID, 10*time.Second); err != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
		close(entry.done)
		autoAgentProcs.Delete(containerID)
		return fmt.Errorf("auto-agent connect timeout: %w", err)
	}

	s.Store.Containers.Update(containerID, func(c *api.Container) {
		c.AgentAddress = "reverse"
	})

	s.Logger.Debug().Str("container", containerID).Msg("auto-agent connected")

	// When the agent process exits (main command finished), store logs and stop the container.
	// Only this goroutine calls cmd.Wait() — StopAutoAgent waits on entry.done instead.
	go func() {
		_ = cmd.Wait()
		close(entry.done)
		autoAgentProcs.Delete(containerID)

		// Store captured stdout as container logs.
		// Always store even if empty — avoids a race where the wait channel
		// is closed before a concurrent log reader sees the data.
		s.Store.LogBuffers.Store(containerID, stdoutBuf.Bytes())

		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		s.Logger.Debug().Str("container", containerID).Int("exitCode", exitCode).Msg("auto-agent exited, stopping container")
		s.Store.StopContainer(containerID, exitCode)
	}()

	return nil
}

// setupVolumePathMappings creates path mappings for volume bind mounts.
// For each bind like "vol-name:/cache", resolves the volume to its host temp dir
// and records /cache → host-dir so commands can access the volume data.
func (s *BaseServer) setupVolumePathMappings(containerID string, c api.Container) {
	for _, bind := range c.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		volName := parts[0]
		containerPath := parts[1]

		if volDir, ok := s.Store.VolumeDirs.Load(volName); ok {
			addPathMapping(s.Store, containerID, containerPath, volDir.(string))
			s.Logger.Debug().
				Str("container", containerID).
				Str("volume", volName).
				Str("containerPath", containerPath).
				Str("hostPath", volDir.(string)).
				Msg("volume path mapping")
		}
	}
}

// StopAutoAgent kills and cleans up an auto-agent process for the container.
// Kills the entire process group to ensure children (e.g. sleep 86400) are also terminated.
// Waits for the background goroutine's cmd.Wait() to complete via the done channel
// to avoid calling cmd.Wait() twice (which hangs on Go's internal I/O errch drain).
func StopAutoAgent(containerID string) {
	if v, ok := autoAgentProcs.LoadAndDelete(containerID); ok {
		entry := v.(*autoAgentEntry)
		// Kill process group (negative PID) to terminate agent + children
		_ = syscall.Kill(-entry.cmd.Process.Pid, syscall.SIGKILL)
		// Wait for the goroutine's cmd.Wait() to finish — do NOT call cmd.Wait() again
		<-entry.done
	}
}
