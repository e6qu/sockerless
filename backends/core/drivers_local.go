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
)

// LocalExecDriver runs commands via the process driver chain.
// Returns an error exit code when no driver is available.
type LocalExecDriver struct {
	Store  *Store
	Logger zerolog.Logger
}

func (d *LocalExecDriver) Exec(ctx context.Context, containerID string, execID string,
	cmd []string, env []string, workDir string, tty bool,
	conn net.Conn) int {

	d.Logger.Error().Str("container", containerID).Msg("no exec driver connected")
	return 1
}

// LocalFilesystemDriver reads/writes files via host filesystem.
// Uses staging directories for pre-start file operations.
type LocalFilesystemDriver struct {
	Store  *Store
	Logger zerolog.Logger
}

func (d *LocalFilesystemDriver) PutArchive(containerID string, path string, tarStream io.Reader) error {
	cleanPath := filepath.Clean(path)

	// Try direct extraction first (works when path is writable, e.g. /tmp)
	if err := os.MkdirAll(cleanPath, 0755); err == nil {
		return extractTar(tarStream, cleanPath)
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

func (d *LocalFilesystemDriver) GetArchive(containerID string, path string, w io.Writer) (os.FileInfo, error) {
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

func (d *LocalFilesystemDriver) StatPath(containerID string, path string) (os.FileInfo, error) {
	realPath := resolveContainerPath(containerID, path, d.Store)
	return os.Stat(realPath)
}

func (d *LocalFilesystemDriver) RootPath(_ string) (string, error) {
	return "", nil
}

// getOrCreateStagingDir returns or creates a temp directory for pre-start file staging.
func (d *LocalFilesystemDriver) getOrCreateStagingDir(containerID string) string {
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

// LocalStreamDriver provides log access and stream attachment via local buffers.
type LocalStreamDriver struct {
	Store  *Store
	Logger zerolog.Logger
}

func (d *LocalStreamDriver) LogBytes(containerID string) []byte {
	if v, ok := d.Store.LogBuffers.Load(containerID); ok {
		return v.([]byte)
	}
	return nil
}

func (d *LocalStreamDriver) LogSubscribe(_ string, _ string) chan []byte {
	ch := make(chan []byte)
	close(ch)
	return ch
}

func (d *LocalStreamDriver) LogUnsubscribe(_, _ string) {}

func (d *LocalStreamDriver) Attach(_ context.Context, containerID string, _ bool, _ net.Conn) error {
	return fmt.Errorf("attach: no stream driver connected for container %s", containerID)
}

// --- Path mapping helpers ---

// addPathMapping records a container-path → host-path mapping for a container.
func addPathMapping(store *Store, containerID string, containerPath, hostPath string) {
	v, _ := store.PathMappings.LoadOrStore(containerID, make(map[string]string))
	m := v.(map[string]string)
	m[containerPath] = hostPath
	store.PathMappings.Store(containerID, m)
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
