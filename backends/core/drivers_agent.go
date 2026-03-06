package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/sockerless/agent"
)

// ErrNoAgent is returned when an operation requires an agent but none is connected.
var ErrNoAgent = fmt.Errorf("no agent connected")

// AgentExecDriver runs commands via agent bridge (forward or reverse).
// Returns an error exit code when no agent is available.
type AgentExecDriver struct {
	Store         *Store
	AgentRegistry *AgentRegistry
	Logger        zerolog.Logger
}

func (d *AgentExecDriver) Exec(ctx context.Context, containerID string, execID string,
	cmd []string, env []string, workDir string, tty bool,
	conn net.Conn) int {

	c, ok := d.Store.Containers.Get(containerID)
	if !ok {
		d.Logger.Error().Str("container", containerID).Msg("container not found for exec")
		return 1
	}

	// Translate absolute paths in exec args to host staging/volume dirs
	cmd = translateContainerPaths(containerID, cmd, d.Store)

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

	d.Logger.Error().Str("container", containerID).Msg("no agent connected for exec")
	return 1
}

// AgentFilesystemDriver reads/writes files via host filesystem for reverse
// agent containers. Returns errors when no agent is available.
type AgentFilesystemDriver struct {
	Store  *Store
	Logger zerolog.Logger
}

func (d *AgentFilesystemDriver) PutArchive(containerID string, path string, tarStream io.Reader) error {
	cleanPath := filepath.Clean(path)

	// Try direct extraction first (works when path is writable, e.g. /tmp)
	c, ok := d.Store.Containers.Get(containerID)
	if ok && c.AgentAddress == "reverse" {
		if err := os.MkdirAll(cleanPath, 0755); err == nil {
			return extractTar(tarStream, cleanPath)
		}
		// Fall through to staging dir if direct path is not writable
	}

	// Stage files in a temp directory and record the path mapping
	// so exec can translate container paths to host paths
	stagingDir := d.getOrCreateStagingDir(containerID)
	destDir := filepath.Join(stagingDir, cleanPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("staging mkdir: %w", err)
	}
	if err := extractTar(tarStream, destDir); err != nil {
		return err
	}
	// Record mapping: container path → host staging path
	addPathMapping(d.Store, containerID, cleanPath, destDir)
	return nil
}

func (d *AgentFilesystemDriver) GetArchive(containerID string, path string, w io.Writer) (os.FileInfo, error) {
	// Resolve the path: try direct, then path mappings, then staging dir
	realPath := resolveContainerPath(containerID, path, d.Store)

	info, err := os.Stat(realPath)
	if err != nil {
		return nil, err
	}
	if err := createTar(w, realPath, info.Name()); err != nil {
		return info, err
	}
	return info, nil
}

func (d *AgentFilesystemDriver) StatPath(containerID string, path string) (os.FileInfo, error) {
	realPath := resolveContainerPath(containerID, path, d.Store)
	return os.Stat(realPath)
}

func (d *AgentFilesystemDriver) RootPath(_ string) (string, error) {
	return "", nil
}

// getOrCreateStagingDir returns or creates a temp directory for pre-start file staging.
func (d *AgentFilesystemDriver) getOrCreateStagingDir(containerID string) string {
	if sd, ok := d.Store.StagingDirs.Load(containerID); ok {
		return sd.(string)
	}
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	dir, err := os.MkdirTemp("", "staging-"+shortID+"-")
	if err != nil {
		d.Logger.Error().Err(err).Msg("failed to create staging dir")
		return os.TempDir()
	}
	d.Store.StagingDirs.Store(containerID, dir)
	return dir
}

// AgentStreamDriver attaches via agent bridge (forward or reverse).
// Returns errors when no agent is available.
type AgentStreamDriver struct {
	Store         *Store
	AgentRegistry *AgentRegistry
	Logger        zerolog.Logger
}

func (d *AgentStreamDriver) LogBytes(containerID string) []byte {
	if v, ok := d.Store.LogBuffers.Load(containerID); ok {
		return v.([]byte)
	}
	return nil
}

func (d *AgentStreamDriver) LogSubscribe(_ string, _ string) chan []byte {
	ch := make(chan []byte)
	close(ch)
	return ch
}

func (d *AgentStreamDriver) LogUnsubscribe(_, _ string) {}

func (d *AgentStreamDriver) Attach(ctx context.Context, containerID string, tty bool, conn net.Conn) error {
	c, ok := d.Store.Containers.Get(containerID)
	if !ok {
		return fmt.Errorf("attach: %w: container %s", ErrNoAgent, containerID)
	}

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
			return fmt.Errorf("attach: failed to dial agent %s: %w", c.AgentAddress, err)
		}
		defer func() { _ = agentConn.Close() }()
		agentConn.BridgeAttach(conn, containerID, tty)
		return nil
	}

	return fmt.Errorf("attach: %w: container %s", ErrNoAgent, containerID)
}

// --- Path mapping helpers ---

// addPathMapping records a container-path → host-path mapping for a container.
func addPathMapping(store *Store, containerID string, containerPath, hostPath string) {
	v, _ := store.PathMappings.LoadOrStore(containerID, make(map[string]string))
	m := v.(map[string]string)
	m[containerPath] = hostPath
	store.PathMappings.Store(containerID, m)
}

// translateContainerPaths rewrites absolute paths in exec command args using
// known path mappings (from PutArchive staging and volume bind mounts).
func translateContainerPaths(containerID string, cmd []string, store *Store) []string {
	v, ok := store.PathMappings.Load(containerID)
	if !ok {
		return cmd
	}
	mappings := v.(map[string]string)
	if len(mappings) == 0 {
		return cmd
	}

	translated := make([]string, len(cmd))
	for i, arg := range cmd {
		translated[i] = arg
		for containerPath, hostPath := range mappings {
			translated[i] = strings.ReplaceAll(translated[i], containerPath, hostPath)
		}
	}
	return translated
}

// resolveContainerPath resolves a container path to its host path, checking
// path mappings and staging dirs.
func resolveContainerPath(containerID string, path string, store *Store) string {
	cleanPath := filepath.Clean(path)

	// Check path mappings first
	if v, ok := store.PathMappings.Load(containerID); ok {
		for containerPath, hostPath := range v.(map[string]string) {
			if strings.HasPrefix(cleanPath, containerPath) {
				return hostPath + cleanPath[len(containerPath):]
			}
		}
	}

	// Check staging dir
	if sd, ok := store.StagingDirs.Load(containerID); ok {
		stagedPath := filepath.Join(sd.(string), cleanPath)
		if _, err := os.Stat(stagedPath); err == nil {
			return stagedPath
		}
	}

	// Fall back to direct path
	return cleanPath
}

// --- Helpers ---

// writeMuxChunk writes a Docker multiplexed stream chunk.
func writeMuxChunk(w io.Writer, streamType byte, data []byte) {
	size := len(data)
	header := []byte{streamType, 0, 0, 0, byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
	_, _ = w.Write(header)
	_, _ = w.Write(data)
}

// DirSize walks a directory and returns the total size of all files in bytes.
func DirSize(path string) int64 {
	var size int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}
