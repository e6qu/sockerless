package sandbox

import (
	"context"
	"io"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"
)

// DirMount specifies an additional directory to mount into the WASM sandbox.
type DirMount struct {
	HostPath      string
	ContainerPath string
}

// RunCommand instantiates the compiled busybox module and runs a single
// applet. args[0] determines the applet (e.g., "echo", "ls", "cat").
// The rootDir is mounted as "/" inside the WASM sandbox.
func (r *Runtime) RunCommand(ctx context.Context, args []string, env []string,
	rootDir string, mounts []DirMount, stdin io.Reader, stdout, stderr io.Writer) (exitCode int, err error) {

	if stdin == nil {
		stdin = emptyReader{}
	}

	fsConfig := wazero.NewFSConfig().WithDirMount(rootDir, "/")
	for _, m := range mounts {
		fsConfig = fsConfig.WithDirMount(m.HostPath, m.ContainerPath)
	}

	cfg := wazero.NewModuleConfig().
		WithName(""). // anonymous — allows concurrent instantiation
		WithArgs(args...).
		WithStdin(stdin).
		WithStdout(stdout).
		WithStderr(stderr).
		WithFSConfig(fsConfig)

	for _, e := range env {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				cfg = cfg.WithEnv(e[:i], e[i+1:])
				break
			}
		}
	}

	mod, err := r.runtime.InstantiateModule(ctx, r.compiled, cfg)
	if mod != nil {
		_ = mod.Close(ctx)
	}
	if err != nil {
		if exitErr, ok := err.(*sys.ExitError); ok {
			return int(exitErr.ExitCode()), nil
		}
		return 1, err
	}
	return 0, nil
}

// emptyReader is an io.Reader that always returns EOF.
type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }
