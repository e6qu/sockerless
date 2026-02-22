package core

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/sockerless/agent"
)

// AgentExecDriver runs commands via agent bridge (forward or reverse).
// Falls back to the next driver when no agent is available.
type AgentExecDriver struct {
	Store         *Store
	AgentRegistry *AgentRegistry
	Logger        zerolog.Logger
	Fallback      ExecDriver
}

func (d *AgentExecDriver) Exec(ctx context.Context, containerID string, execID string,
	cmd []string, env []string, workDir string, tty bool,
	conn net.Conn) int {

	c, _ := d.Store.Containers.Get(containerID)

	if c.AgentAddress == "reverse" {
		revConn := d.AgentRegistry.Get(containerID)
		if revConn != nil {
			return revConn.BridgeExec(conn, execID, cmd, env, workDir, tty)
		}
		d.Logger.Error().Str("container", containerID).Msg("reverse agent not connected")
		return 1
	}

	if c.AgentAddress != "" {
		agentConn, err := agent.Dial(c.AgentAddress, c.AgentToken)
		if err != nil {
			d.Logger.Error().Err(err).Str("agent", c.AgentAddress).Msg("failed to dial agent for exec")
			return 1
		}
		defer func() { _ = agentConn.Close() }()
		return agentConn.BridgeExec(conn, execID, cmd, env, workDir, tty)
	}

	return d.Fallback.Exec(ctx, containerID, execID, cmd, env, workDir, tty, conn)
}

// AgentFilesystemDriver reads/writes files via host filesystem for reverse
// agent containers. Falls back to the next driver otherwise.
type AgentFilesystemDriver struct {
	Store    *Store
	Logger   zerolog.Logger
	Fallback FilesystemDriver
}

func (d *AgentFilesystemDriver) PutArchive(containerID string, path string, tarStream io.Reader) error {
	c, _ := d.Store.Containers.Get(containerID)
	if c.AgentAddress == "reverse" {
		destDir := filepath.Clean(path)
		return extractTar(tarStream, destDir)
	}
	return d.Fallback.PutArchive(containerID, path, tarStream)
}

func (d *AgentFilesystemDriver) GetArchive(containerID string, path string, w io.Writer) (os.FileInfo, error) {
	c, _ := d.Store.Containers.Get(containerID)
	if c.AgentAddress == "reverse" {
		realPath := filepath.Clean(path)
		info, err := os.Stat(realPath)
		if err != nil {
			return nil, err
		}
		createTar(w, realPath, info.Name())
		return info, nil
	}
	return d.Fallback.GetArchive(containerID, path, w)
}

func (d *AgentFilesystemDriver) StatPath(containerID string, path string) (os.FileInfo, error) {
	c, _ := d.Store.Containers.Get(containerID)
	if c.AgentAddress == "reverse" {
		return os.Stat(filepath.Clean(path))
	}
	return d.Fallback.StatPath(containerID, path)
}

func (d *AgentFilesystemDriver) RootPath(containerID string) (string, error) {
	return d.Fallback.RootPath(containerID)
}

// AgentStreamDriver attaches via agent bridge (forward or reverse).
// Falls back to the next driver when no agent is available.
type AgentStreamDriver struct {
	Store         *Store
	AgentRegistry *AgentRegistry
	Logger        zerolog.Logger
	Fallback      StreamDriver
}

func (d *AgentStreamDriver) LogBytes(containerID string) []byte {
	return d.Fallback.LogBytes(containerID)
}

func (d *AgentStreamDriver) Attach(ctx context.Context, containerID string, tty bool, conn net.Conn) error {
	c, _ := d.Store.Containers.Get(containerID)

	if c.AgentAddress == "reverse" {
		revConn := d.AgentRegistry.Get(containerID)
		if revConn != nil {
			revConn.BridgeAttach(conn, containerID, tty)
			return nil
		}
		d.Logger.Error().Str("container", containerID).Msg("reverse agent not connected for attach")
		return nil
	}

	if c.AgentAddress != "" {
		agentConn, err := agent.Dial(c.AgentAddress, c.AgentToken)
		if err != nil {
			d.Logger.Error().Err(err).Str("agent", c.AgentAddress).Msg("failed to dial agent for attach")
			return err
		}
		defer func() { _ = agentConn.Close() }()
		agentConn.BridgeAttach(conn, containerID, tty)
		return nil
	}

	return d.Fallback.Attach(ctx, containerID, tty, conn)
}
