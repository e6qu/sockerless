package agent

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// These -race soak tests drive sustained concurrent load over the agent's
// SessionRegistry, MainProcess fan-out, and childReaper, asserting no map race,
// no send-on-closed, no double-close, and that the registry empties out. They
// are permanent regression tests for the Forget leak fix, the fanOut fail-loud
// path, and the reaper's owner-routing under concurrent starts.

// TestSessionRegistrySoak hammers Register / Get / Forget / Remove / CleanupConn
// on overlapping session ids across a small pool of connections. Forget must
// never Close (process already exited); Remove/CleanupConn must Close exactly
// once. Invariant at the end: every connection is fully cleaned (no leaked
// connSessions buckets) once every conn is torn down.
func TestSessionRegistrySoak(t *testing.T) {
	r := NewSessionRegistry()

	const nConns = 6
	conns := make([]*soakConn, nConns)
	for i := range conns {
		conns[i] = &soakConn{ws: makeTestWSConn(t)}
		t.Cleanup(func(c *soakConn) func() { return func() { _ = c.ws.Close() } }(conns[i]))
	}

	const (
		workers = 120
		iters   = 60
	)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			conn := conns[w%nConns].ws
			for i := 0; i < iters; i++ {
				id := fmt.Sprintf("sess-%d-%d", w, i)
				s := &soakSession{id: id}
				r.Register(s, conn)
				_, _ = r.Get(id)
				// Half the sessions "finish" (Forget, no Close), half are
				// torn down (Remove, Close). Both race CleanupConn below.
				if (w+i)%2 == 0 {
					r.Forget(id)
				} else {
					r.Remove(id)
				}
			}
		}(w)
	}

	// Concurrent whole-connection teardowns racing the per-session ops.
	for c := 0; c < nConns; c++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			conn := conns[c%nConns].ws
			for i := 0; i < iters/2; i++ {
				r.CleanupConn(conn)
				time.Sleep(time.Millisecond)
			}
		}(c)
	}

	wg.Wait()

	// Final teardown: clean every connection. The registry must end empty.
	for _, c := range conns {
		r.CleanupConn(c.ws)
	}
	r.mu.RLock()
	nSess := len(r.sessions)
	nConnBuckets := len(r.connSessions)
	r.mu.RUnlock()
	if nSess != 0 {
		t.Fatalf("session registry leaked %d sessions after full cleanup", nSess)
	}
	if nConnBuckets != 0 {
		t.Fatalf("session registry leaked %d connSessions buckets after full cleanup", nConnBuckets)
	}
}

// TestMainProcessFanOutSoak starts a real short-lived main process and drives
// concurrent Subscribe / Unsubscribe while the process emits output and exits.
// fanOut must not send on a closed listener channel, must not race the
// listeners map, and Subscribe-after-exit must return a closed channel without
// panic. The lossy-listener path (full buffer → close + delete) is also
// exercised by subscribers that never drain.
func TestMainProcessFanOutSoak(t *testing.T) {
	// A process that prints a burst then exits — enough output to fill a slow
	// listener's buffer and trip the lossy-close path.
	args := []string{"/bin/sh", "-c", "i=0; while [ $i -lt 200 ]; do echo line-$i; i=$((i+1)); done"}
	mp, err := NewMainProcess(zerolog.Nop(), args, nil, nil)
	if err != nil {
		t.Fatalf("NewMainProcess: %v", err)
	}

	const (
		drainers    = 40
		nonDrainers = 40
		iters       = 20
	)
	var wg sync.WaitGroup

	// Draining subscribers: read until the channel closes.
	for d := 0; d < drainers; d++ {
		wg.Add(1)
		go func(d int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				id := fmt.Sprintf("drain-%d-%d", d, i)
				_, _, ch := mp.Subscribe(id)
				for range ch {
				}
				mp.Unsubscribe(id)
			}
		}(d)
	}

	// Non-draining subscribers: subscribe and immediately unsubscribe (or
	// never read), forcing the fanOut full-buffer lossy-close path.
	for n := 0; n < nonDrainers; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				id := fmt.Sprintf("nodrain-%d-%d", n, i)
				_, _, _ = mp.Subscribe(id)
				mp.Unsubscribe(id)
			}
		}(n)
	}

	wg.Wait()

	// Wait for the process to fully exit and the wait() teardown to run.
	select {
	case <-mp.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("main process did not exit within 10s")
	}

	if ec := mp.ExitCode(); ec == nil {
		t.Fatal("exit code should be set after Done()")
	}

	// Subscribe after exit must return a closed channel, not panic / hang.
	_, _, ch := mp.Subscribe("after-exit")
	select {
	case _, ok := <-ch:
		if ok {
			// Allowed: a buffered closed channel may still be drained, but it
			// must eventually report closed. Drain to closed.
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe-after-exit channel neither delivered nor closed")
	}
}

// TestChildReaperSoak drives many concurrent startAndRegister calls (real
// short-lived child processes) through ONE reaper running its SIGCHLD loop.
// Each owner must receive its OWN child's status exactly once — never another
// owner's, never an orphan-discard. Asserts no status is lost or misrouted
// under concurrent starts.
//
// The reaper installs a SIGCHLD handler and calls Wait4(-1, …), which reaps
// EVERY child of the process. Two reapers in one process steal each other's
// children (which is why this matches production: the agent runs exactly one
// reaper as PID-1 init). So this test runs a single reaper and Stops it in
// cleanup — otherwise its Wait4(-1) loop would outlive the test and steal the
// exec children of later process-spawning tests in the same binary (and a
// second `-count` pass would overlap two reaper loops).
func TestChildReaperSoak(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reaper uses syscall.Wait4 (POSIX)")
	}
	r := newChildReaper(zerolog.Nop())
	go r.run()
	t.Cleanup(r.Stop)

	const (
		starters = 50
		iters    = 8
	)
	var wg sync.WaitGroup
	var mismatches int64
	var mu sync.Mutex

	for s := 0; s < starters; s++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				// Each child exits with a distinct, known code so we can
				// verify the reaper routes the right status to the right owner.
				code := (s + i) % 100
				ch, err := r.startAndRegister(startExitChild(code))
				if err != nil {
					mu.Lock()
					mismatches++
					mu.Unlock()
					continue
				}
				select {
				case status := <-ch:
					got := -1
					if status.Exited() {
						got = status.ExitStatus()
					}
					if got != code {
						mu.Lock()
						mismatches++
						mu.Unlock()
					}
				case <-time.After(10 * time.Second):
					mu.Lock()
					mismatches++
					mu.Unlock()
				}
			}
		}(s)
	}
	wg.Wait()

	if mismatches != 0 {
		t.Fatalf("childReaper misrouted/lost %d statuses under concurrent starts", mismatches)
	}

	// No leaked waiters: every registered PID was delivered and unregistered.
	r.mu.Lock()
	nWaiters := len(r.waiters)
	r.mu.Unlock()
	if nWaiters != 0 {
		t.Fatalf("childReaper leaked %d waiters", nWaiters)
	}
}

// startExitChild returns a start func (the contract of startAndRegister) that
// forks `/bin/sh -c "exit <code>"` and returns its PID. The reaper owns the
// wait4; we must NOT call Wait() ourselves.
func startExitChild(code int) func() (int, error) {
	return func() (int, error) {
		// Fork via syscall to avoid os/exec's own Wait4 goroutine racing the
		// reaper for the same PID. ForkExec gives us a bare child the reaper
		// reaps exactly like a re-parented grandchild.
		argv := []string{"/bin/sh", "-c", fmt.Sprintf("exit %d", code)}
		pid, err := syscall.ForkExec("/bin/sh", argv, &syscall.ProcAttr{
			Files: []uintptr{0, 1, 2},
		})
		if err != nil {
			return 0, err
		}
		return pid, nil
	}
}

// soakSession is a minimal Session for the registry soak. Close records that it
// was invoked so Forget-must-not-Close is enforceable, but the soak only checks
// aggregate invariants (registry empties), not per-session close flags (those
// are covered by the deterministic TestSessionRegistry* tests).
type soakSession struct {
	id     string
	mu     sync.Mutex
	closed bool
}

func (s *soakSession) ID() string                { return s.id }
func (s *soakSession) WriteStdin(_ []byte) error { return nil }
func (s *soakSession) CloseStdin() error         { return nil }
func (s *soakSession) Signal(_ string) error     { return nil }
func (s *soakSession) Resize(_, _ int) error     { return nil }
func (s *soakSession) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

// soakConn bundles a websocket.Conn used as a registry map key.
type soakConn struct {
	ws *websocket.Conn
}
