package memory

import (
	"context"
	"io"
	"time"

	core "github.com/sockerless/backend-core"
	"github.com/sockerless/sandbox"
)

// sandboxFactory implements core.ProcessFactory using the WASM sandbox.
type sandboxFactory struct {
	runtime *sandbox.Runtime
}

func newSandboxFactory(ctx context.Context) (*sandboxFactory, error) {
	rt, err := sandbox.NewRuntime(ctx)
	if err != nil {
		return nil, err
	}
	return &sandboxFactory{runtime: rt}, nil
}

func (f *sandboxFactory) NewProcess(cmd, env []string, binds map[string]string) (core.ContainerProcess, error) {
	p, err := sandbox.NewProcess(f.runtime, cmd, env, binds)
	if err != nil {
		return nil, err
	}
	return &sandboxProcess{p: p}, nil
}

func (f *sandboxFactory) IsShellCommand(cmd []string) bool {
	return sandbox.IsShellCommand(cmd)
}

func (f *sandboxFactory) Close(ctx context.Context) error {
	return f.runtime.Close(ctx)
}

// sandboxProcess wraps *sandbox.Process to implement core.ContainerProcess.
type sandboxProcess struct {
	p *sandbox.Process
}

func (sp *sandboxProcess) Wait() int                       { return sp.p.Wait() }
func (sp *sandboxProcess) Signal()                          { sp.p.Signal() }
func (sp *sandboxProcess) Close()                           { sp.p.Close() }
func (sp *sandboxProcess) Done() <-chan struct{}             { return sp.p.Done() }
func (sp *sandboxProcess) LogBytes() []byte                 { return sp.p.LogBytes() }
func (sp *sandboxProcess) Subscribe(id string) chan []byte   { return sp.p.Subscribe(id) }
func (sp *sandboxProcess) Unsubscribe(id string)            { sp.p.Unsubscribe(id) }
func (sp *sandboxProcess) StdinWriter() io.WriteCloser      { return sp.p.StdinWriter() }

func (sp *sandboxProcess) RunExec(ctx context.Context, cmd []string, env []string,
	workDir string, stdin io.Reader, stdout, stderr io.Writer) int {
	return sp.p.RunExec(ctx, cmd, env, workDir, stdin, stdout, stderr)
}

func (sp *sandboxProcess) RunInteractiveShell(ctx context.Context, env []string,
	stdin io.Reader, stdout, stderr io.Writer) int {
	return sp.p.RunInteractiveShell(ctx, env, stdin, stdout, stderr)
}

func (sp *sandboxProcess) Stats() core.ProcessStats {
	s := sp.p.Stats()
	return core.ProcessStats{
		MemoryUsage: s.MemoryUsage,
		CPUNanos:    s.CPUNanos,
		PIDs:        s.PIDs,
	}
}

func (sp *sandboxProcess) Top() []core.ProcessTopEntry {
	entries := sp.p.Top()
	result := make([]core.ProcessTopEntry, len(entries))
	for i, e := range entries {
		result[i] = core.ProcessTopEntry{PID: e.PID, Command: e.Command}
	}
	return result
}

func (sp *sandboxProcess) StartTime() time.Time   { return sp.p.StartTime() }
func (sp *sandboxProcess) FormatStartTime() string { return sp.p.FormatStartTime() }
func (sp *sandboxProcess) RootPath() string        { return sp.p.RootPath() }
