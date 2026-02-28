package simulator

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"time"
)

// ProcessConfig describes what to execute.
type ProcessConfig struct {
	Command []string          // entrypoint + args (e.g. ["echo", "hello"])
	Env     map[string]string // environment variables
	Dir     string            // working directory (optional)
	Timeout time.Duration     // max execution time (0 = no timeout)
}

// LogLine is a single line of captured output.
type LogLine struct {
	Stream    string    // "stdout" or "stderr"
	Text      string
	Timestamp time.Time
}

// LogSink receives log lines as they are produced.
// Each cloud implements its own sink (CloudWatch, Cloud Logging, Log Analytics).
type LogSink interface {
	WriteLog(line LogLine)
}

// ProcessResult is returned when the process completes.
type ProcessResult struct {
	ExitCode  int
	StartedAt time.Time
	StoppedAt time.Time
	Error     error // non-nil if process failed to start
}

// ProcessHandle allows waiting on or cancellation of a running process.
type ProcessHandle struct {
	cancel context.CancelFunc
	done   <-chan ProcessResult
}

// Wait blocks until the process completes.
func (h *ProcessHandle) Wait() ProcessResult { return <-h.done }

// Cancel kills the process.
func (h *ProcessHandle) Cancel() { h.cancel() }

// NoopSink discards all log output.
type NoopSink struct{}

func (NoopSink) WriteLog(LogLine) {}

// FuncSink wraps a function as a LogSink.
type FuncSink func(LogLine)

func (f FuncSink) WriteLog(line LogLine) { f(line) }

// StartProcess launches a command and streams output to the sink.
// Returns a handle for waiting/cancellation. Non-blocking.
func StartProcess(cfg ProcessConfig, sink LogSink) *ProcessHandle {
	resultCh := make(chan ProcessResult, 1)

	ctx, cancel := context.WithCancel(context.Background())
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), cfg.Timeout)
	}

	cmd := exec.CommandContext(ctx, cfg.Command[0], cfg.Command[1:]...)

	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Set up pipes for stdout and stderr
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		cancel()
		resultCh <- ProcessResult{
			ExitCode:  -1,
			StartedAt: startedAt,
			StoppedAt: time.Now(),
			Error:     err,
		}
		return &ProcessHandle{cancel: func() {}, done: resultCh}
	}

	// Scan stdout and stderr in separate goroutines
	scanDone := make(chan struct{}, 2)
	scanStream := func(reader io.Reader, stream string) {
		defer func() { scanDone <- struct{}{} }()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			sink.WriteLog(LogLine{
				Stream:    stream,
				Text:      scanner.Text(),
				Timestamp: time.Now(),
			})
		}
	}

	go scanStream(stdoutPipe, "stdout")
	go scanStream(stderrPipe, "stderr")

	go func() {
		// Wait for both scanners to finish before calling cmd.Wait
		<-scanDone
		<-scanDone

		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		cancel()
		resultCh <- ProcessResult{
			ExitCode:  exitCode,
			StartedAt: startedAt,
			StoppedAt: time.Now(),
		}
	}()

	return &ProcessHandle{cancel: cancel, done: resultCh}
}
