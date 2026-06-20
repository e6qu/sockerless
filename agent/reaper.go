package agent

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
)

// childReaper is the single waiter for child processes when the agent runs
// as container init (PID 1). A blanket SIGCHLD reaper that calls
// Wait4(-1, WNOHANG) and discards every status would steal the exit status
// of children the agent owns (the keep-alive main process and each exec
// session child) — their own cmd.Wait()/Wait4 would then race it and get
// ECHILD ("no child processes"), losing the real exit code.
//
// To stay the reason it exists (reaping genuine orphan grandchildren that
// re-parent to PID 1) WITHOUT stealing owned statuses, the reaper is the
// ONE place that calls wait4. Owners register their PID before the child can
// exit and receive that child's WaitStatus on a channel; statuses for PIDs
// no owner registered are genuine orphans and are discarded.
type childReaper struct {
	mu      sync.Mutex
	waiters map[int]chan syscall.WaitStatus
	logger  zerolog.Logger
}

func newChildReaper(logger zerolog.Logger) *childReaper {
	return &childReaper{
		waiters: make(map[int]chan syscall.WaitStatus),
		logger:  logger,
	}
}

// startAndRegister runs start (which must fork the child, e.g. cmd.Start or
// pty.Start) while holding the reaper lock, then registers a waiter for the
// child's PID and returns a channel that delivers exactly one WaitStatus.
//
// Holding the lock across start+register closes the lost-status window: the
// reaper's Wait4 loop runs lock-free and may reap the child before we
// register, but deliver() — which routes the captured status to a waiter —
// takes the same lock, so it blocks until register has run. The status is
// therefore always routed to our channel, never discarded as an orphan.
func (r *childReaper) startAndRegister(start func() (int, error)) (<-chan syscall.WaitStatus, error) {
	ch := make(chan syscall.WaitStatus, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	pid, err := start()
	if err != nil {
		return nil, err
	}
	r.waiters[pid] = ch
	return ch, nil
}

// run installs the SIGCHLD handler and reaps children. For each reaped PID it
// delivers the status to a registered owner (and unregisters it) or discards
// it as a genuine orphan. It runs for the lifetime of the agent.
func (r *childReaper) run() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGCHLD)
	for range sigCh {
		for {
			var status syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if pid <= 0 || err != nil {
				break
			}
			r.deliver(pid, status)
		}
	}
}

func (r *childReaper) deliver(pid int, status syscall.WaitStatus) {
	r.mu.Lock()
	ch, ok := r.waiters[pid]
	if ok {
		delete(r.waiters, pid)
	}
	r.mu.Unlock()
	if ok {
		ch <- status // buffered cap 1, exactly one delivery
		return
	}
	r.logger.Debug().Int("pid", pid).Msg("reaped orphan child")
}
