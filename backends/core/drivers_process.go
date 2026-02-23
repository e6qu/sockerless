package core

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// WASMExecDriver runs commands via ContainerProcess (WASM sandbox).
// Falls back to Fallback driver when no process exists for the container.
type WASMExecDriver struct {
	Store          *Store
	ProcessFactory ProcessFactory
	Fallback       ExecDriver
}

func (d *WASMExecDriver) Exec(ctx context.Context, containerID string, execID string,
	cmd []string, env []string, workDir string, tty bool,
	conn net.Conn) int {

	wp, ok := d.Store.Processes.Load(containerID)
	if !ok {
		return d.Fallback.Exec(ctx, containerID, execID, cmd, env, workDir, tty, conn)
	}
	proc := wp.(ContainerProcess)

	if d.ProcessFactory.IsShellCommand(cmd) && tty {
		return proc.RunInteractiveShell(ctx, env, conn, conn, conn)
	}

	var stdout, stderr io.Writer = conn, conn
	if !tty {
		stdout = &muxWriter{w: conn, streamType: 1}
		stderr = &muxWriter{w: conn, streamType: 2}
	}
	return proc.RunExec(ctx, cmd, env, workDir, conn, stdout, stderr)
}

// WASMFilesystemDriver reads/writes files via ContainerProcess root path.
// Falls back to staging directory for pre-start archives.
type WASMFilesystemDriver struct {
	Store *Store
	// Fallback is used when no process exists (pre-start staging).
	Fallback FilesystemDriver
}

func (d *WASMFilesystemDriver) PutArchive(containerID string, path string, tarStream io.Reader) error {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		proc := wp.(ContainerProcess)
		destDir := joinCleanPath(proc.RootPath(), path)
		return extractTar(tarStream, destDir)
	}
	return d.Fallback.PutArchive(containerID, path, tarStream)
}

func (d *WASMFilesystemDriver) GetArchive(containerID string, path string, w io.Writer) (os.FileInfo, error) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		proc := wp.(ContainerProcess)
		realPath := joinCleanPath(proc.RootPath(), path)
		info, err := os.Stat(realPath)
		if err != nil {
			return nil, err
		}
		createTar(w, realPath, info.Name())
		return info, nil
	}
	return d.Fallback.GetArchive(containerID, path, w)
}

func (d *WASMFilesystemDriver) StatPath(containerID string, path string) (os.FileInfo, error) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		proc := wp.(ContainerProcess)
		realPath := joinCleanPath(proc.RootPath(), path)
		return os.Stat(realPath)
	}
	return d.Fallback.StatPath(containerID, path)
}

func (d *WASMFilesystemDriver) RootPath(containerID string) (string, error) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		return wp.(ContainerProcess).RootPath(), nil
	}
	return d.Fallback.RootPath(containerID)
}

// WASMStreamDriver attaches to a WASM process for bidirectional streaming.
// Falls back to synthetic when no process exists.
type WASMStreamDriver struct {
	Store    *Store
	Fallback StreamDriver
}

func (d *WASMStreamDriver) Attach(ctx context.Context, containerID string, tty bool, conn net.Conn) error {
	// Wait for process to appear (attach may precede start, e.g. gitlab-runner)
	var proc ContainerProcess
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		proc = wp.(ContainerProcess)
	} else {
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			time.Sleep(100 * time.Millisecond)
			if wp, ok := d.Store.Processes.Load(containerID); ok {
				proc = wp.(ContainerProcess)
				break
			}
		}
	}

	if proc == nil {
		return d.Fallback.Attach(ctx, containerID, tty, conn)
	}

	// Forward stdin from attach connection to process
	if sw := proc.StdinWriter(); sw != nil {
		go func() {
			_, _ = io.Copy(sw, conn)
			_ = sw.Close()
		}()
	}

	// Send buffered output first
	buffered := proc.LogBytes()
	if len(buffered) > 0 {
		if tty {
			_, _ = conn.Write(buffered)
		} else {
			writeMuxChunk(conn, 1, buffered)
		}
	}

	// Subscribe to live output
	subID := GenerateID()[:16]
	ch := proc.Subscribe(subID)
	defer proc.Unsubscribe(subID)

	for chunk := range ch {
		if tty {
			_, _ = conn.Write(chunk)
		} else {
			writeMuxChunk(conn, 1, chunk)
		}
	}
	return nil
}

func (d *WASMStreamDriver) LogSubscribe(containerID, subID string) chan []byte {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		return wp.(ContainerProcess).Subscribe(subID)
	}
	return d.Fallback.LogSubscribe(containerID, subID)
}

func (d *WASMStreamDriver) LogUnsubscribe(containerID, subID string) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		wp.(ContainerProcess).Unsubscribe(subID)
		return
	}
	d.Fallback.LogUnsubscribe(containerID, subID)
}

func (d *WASMStreamDriver) LogBytes(containerID string) []byte {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		return wp.(ContainerProcess).LogBytes()
	}
	return d.Fallback.LogBytes(containerID)
}

// WASMProcessLifecycleDriver manages WASM sandbox processes.
type WASMProcessLifecycleDriver struct {
	Store          *Store
	ProcessFactory ProcessFactory
}

func (d *WASMProcessLifecycleDriver) Start(containerID string, cmd []string, env []string,
	binds map[string]string) (bool, error) {

	if d.ProcessFactory == nil || len(cmd) == 0 || cmd[0] == "" {
		return false, nil
	}

	proc, err := d.ProcessFactory.NewProcess(cmd, env, binds)
	if err != nil {
		return false, err
	}
	d.Store.Processes.Store(containerID, proc)

	// Wait for process exit and stop the container automatically.
	// This was previously done in the handler; moving it here
	// eliminates the handler's need to access the process directly.
	go func() {
		exitCode := proc.Wait()
		d.Store.StopContainer(containerID, exitCode)
	}()

	return true, nil
}

func (d *WASMProcessLifecycleDriver) Stop(containerID string) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		wp.(ContainerProcess).Signal()
	}
}

func (d *WASMProcessLifecycleDriver) Kill(containerID string) {
	d.Stop(containerID)
}

func (d *WASMProcessLifecycleDriver) Cleanup(containerID string) {
	if wp, ok := d.Store.Processes.LoadAndDelete(containerID); ok {
		wp.(ContainerProcess).Close()
	}
}

func (d *WASMProcessLifecycleDriver) WaitCh(containerID string) <-chan struct{} {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		return wp.(ContainerProcess).Done()
	}
	// No process — return a closed channel (immediate)
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (d *WASMProcessLifecycleDriver) Top(containerID string) ([]ProcessTopEntry, error) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		return wp.(ContainerProcess).Top(), nil
	}
	return nil, nil
}

func (d *WASMProcessLifecycleDriver) Stats(containerID string) (*ProcessStats, error) {
	if wp, ok := d.Store.Processes.Load(containerID); ok {
		stats := wp.(ContainerProcess).Stats()
		return &stats, nil
	}
	return &ProcessStats{}, nil
}

func (d *WASMProcessLifecycleDriver) IsSynthetic(_ string) bool {
	return false
}

// --- Helpers ---

// joinCleanPath joins a base path with a cleaned relative path.
func joinCleanPath(base, path string) string {
	return filepath.Join(base, filepath.Clean(path))
}

// writeMuxChunk writes a Docker multiplexed stream chunk.
func writeMuxChunk(w io.Writer, streamType byte, data []byte) {
	size := len(data)
	header := []byte{streamType, 0, 0, 0, byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
	_, _ = w.Write(header)
	_, _ = w.Write(data)
}
