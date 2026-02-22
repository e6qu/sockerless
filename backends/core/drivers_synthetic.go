package core

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
)

// SyntheticExecDriver echoes commands as output without real execution.
// Used as the fallback when no process or agent is available.
type SyntheticExecDriver struct{}

func (d *SyntheticExecDriver) Exec(_ context.Context, _ string, _ string,
	cmd []string, _ []string, _ string, tty bool,
	conn net.Conn) int {

	cmdStr := strings.Join(cmd, " ")
	output := []byte(cmdStr + "\n")

	if tty {
		_, _ = conn.Write(output)
	} else {
		size := len(output)
		header := []byte{1, 0, 0, 0, byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
		_, _ = conn.Write(header)
		_, _ = conn.Write(output)
	}
	return 0
}

// SyntheticFilesystemDriver stores files in staging directories only.
// Used as the fallback when no process or agent provides a real filesystem.
type SyntheticFilesystemDriver struct {
	Store *Store
}

func (d *SyntheticFilesystemDriver) PutArchive(containerID string, path string, tarStream io.Reader) error {
	stagingRoot := d.getOrCreateStagingDir(containerID)
	destDir := joinCleanPath(stagingRoot, path)
	return extractTar(tarStream, destDir)
}

func (d *SyntheticFilesystemDriver) GetArchive(containerID string, path string, w io.Writer) (os.FileInfo, error) {
	if sd, ok := d.Store.StagingDirs.Load(containerID); ok {
		realPath := joinCleanPath(sd.(string), path)
		if info, err := os.Stat(realPath); err == nil {
			createTar(w, realPath, info.Name())
			return info, nil
		}
	}
	return nil, os.ErrNotExist
}

func (d *SyntheticFilesystemDriver) StatPath(containerID string, path string) (os.FileInfo, error) {
	if sd, ok := d.Store.StagingDirs.Load(containerID); ok {
		realPath := joinCleanPath(sd.(string), path)
		return os.Stat(realPath)
	}
	return nil, os.ErrNotExist
}

func (d *SyntheticFilesystemDriver) RootPath(_ string) (string, error) {
	return "", nil
}

func (d *SyntheticFilesystemDriver) getOrCreateStagingDir(containerID string) string {
	if v, ok := d.Store.StagingDirs.Load(containerID); ok {
		return v.(string)
	}
	short := containerID
	if len(short) > 12 {
		short = short[:12]
	}
	dir, _ := os.MkdirTemp("", "sockerless-staging-"+short+"-")
	d.Store.StagingDirs.Store(containerID, dir)
	return dir
}

// SyntheticStreamDriver sends buffered log output and waits for exit.
type SyntheticStreamDriver struct {
	Store *Store
}

func (d *SyntheticStreamDriver) LogBytes(containerID string) []byte {
	logData, _ := d.Store.LogBuffers.Load(containerID)
	if logData != nil {
		return logData.([]byte)
	}
	return nil
}

func (d *SyntheticStreamDriver) Attach(_ context.Context, containerID string, _ bool, conn net.Conn) error {
	logData, _ := d.Store.LogBuffers.Load(containerID)
	if logData != nil {
		data := logData.([]byte)
		if len(data) > 0 {
			size := len(data)
			header := []byte{1, 0, 0, 0, byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
			_, _ = conn.Write(header)
			_, _ = conn.Write(data)
		}
	}
	if ch, ok := d.Store.WaitChs.Load(containerID); ok {
		<-ch.(chan struct{})
	}
	return nil
}

// SyntheticProcessLifecycleDriver provides no-op process lifecycle.
type SyntheticProcessLifecycleDriver struct{}

func (d *SyntheticProcessLifecycleDriver) Start(_ string, _ []string, _ []string, _ map[string]string) (bool, error) {
	return false, nil
}

func (d *SyntheticProcessLifecycleDriver) Stop(_ string) {}

func (d *SyntheticProcessLifecycleDriver) Kill(_ string) {}

func (d *SyntheticProcessLifecycleDriver) Cleanup(_ string) {}

func (d *SyntheticProcessLifecycleDriver) WaitCh(_ string) <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (d *SyntheticProcessLifecycleDriver) Top(_ string) ([]ProcessTopEntry, error) {
	return nil, nil
}

func (d *SyntheticProcessLifecycleDriver) Stats(_ string) (*ProcessStats, error) {
	return &ProcessStats{}, nil
}

func (d *SyntheticProcessLifecycleDriver) IsSynthetic(_ string) bool {
	return true
}
