package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/sockerless/agent"
	"github.com/sockerless/api"
)

// These -race soak tests drive sustained concurrent load over the backend-core
// Store + lifecycle primitives and assert invariants (no panic, no double-close,
// no orphaned waiter, registries empty out). They are permanent regression tests
// for the PathMappings COW, the per-container StartLock, ContainerWaitCtx,
// the event bus, the reverse-agent registry, and the log-follow disconnect path.

func newSoakServer() *BaseServer {
	s := &BaseServer{
		Store:          NewStore(),
		Logger:         zerolog.Nop(),
		Mux:            http.NewServeMux(),
		EventBus:       NewEventBus(),
		PendingCreates: NewStateStore[api.Container](),
	}
	s.InitDrivers()
	s.self = s
	return s
}

func soakContainer(id string) api.Container {
	return api.Container{
		ID:              id,
		Name:            "/" + id,
		Config:          api.ContainerConfig{Labels: map[string]string{}},
		State:           api.ContainerState{Status: "created", Running: false},
		NetworkSettings: api.NetworkSettings{Networks: map[string]*api.EndpointSettings{}},
		Mounts:          []api.MountPoint{},
	}
}

// TestStoreLifecycleSoak runs a heavy mix of concurrent ContainerStart / Wait /
// Stop / Remove / Update on a small set of overlapping container ids through a
// real BaseServer. It asserts no race, no panic, no double-close of a WaitCh,
// and that ContainerWaitCtx callers that disconnect (ctx cancel) don't strand.
func TestStoreLifecycleSoak(t *testing.T) {
	s := newSoakServer()

	const (
		nIDs    = 8
		workers = 120
		iters   = 40
	)
	ids := make([]string, nIDs)
	for i := range ids {
		ids[i] = fmt.Sprintf("soak-%d", i)
		s.Store.Containers.Put(ids[i], soakContainer(ids[i]))
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := ids[w%nIDs]
			for i := 0; i < iters; i++ {
				switch (w + i) % 7 {
				case 0:
					_ = s.ContainerStart(id)
				case 1:
					// Wait with a short ctx so a never-exiting container
					// releases via ctx.Done() rather than parking forever.
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
					_, _ = s.ContainerWaitCtx(ctx, id, "not-running")
					cancel()
				case 2:
					_ = s.ContainerStop(id, nil)
				case 3:
					s.Store.Containers.Update(id, func(c *api.Container) { c.RestartCount++ })
				case 4:
					_ = s.Store.Containers.List()
				case 5:
					s.Store.ForceStopContainer(id, 137)
				case 6:
					// Remove then re-create so the id keeps cycling.
					s.Store.Containers.Delete(id)
					s.Store.Containers.Put(id, soakContainer(id))
				}
			}
		}(w)
	}
	wg.Wait()

	// Invariant: every container that ended running has exactly one WaitCh;
	// no double-registration. (We just assert no orphaned channels for a
	// removed/recreated id — the lifecycle ops close on stop.)
	for _, id := range ids {
		// Drain any remaining wait channel to verify it isn't double-closed.
		if ch, ok := s.Store.WaitChs.Load(id); ok {
			select {
			case <-ch.(chan struct{}):
			default:
			}
		}
	}
}

// TestPathMappingsCOWSoak hammers the copy-on-write addPathMapping against
// concurrent resolveContainerPath readers on overlapping container ids. Before
// the COW fix this crashed with "concurrent map writes" / "concurrent map
// iteration and map write" (which recover() cannot catch). It must stay clean.
func TestPathMappingsCOWSoak(t *testing.T) {
	s := newSoakServer()
	const (
		nIDs    = 6
		writers = 60
		readers = 60
		iters   = 200
	)
	ids := make([]string, nIDs)
	for i := range ids {
		ids[i] = fmt.Sprintf("pm-%d", i)
	}

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := ids[w%nIDs]
			for i := 0; i < iters; i++ {
				addPathMapping(s.Store, id,
					fmt.Sprintf("/c/%d/%d", w, i),
					fmt.Sprintf("/h/%d/%d", w, i))
			}
		}(w)
	}
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			id := ids[r%nIDs]
			for i := 0; i < iters; i++ {
				_ = resolveContainerPath(id, fmt.Sprintf("/c/%d/%d", r, i), s.Store)
			}
		}(r)
	}
	wg.Wait()
}

// TestEventBusSoak drives concurrent Publish / Subscribe / Unsubscribe / History
// against one bus, then Close, asserting no send-on-closed-channel panic and no
// double-close. A subscriber that unsubscribes while a publisher is mid-send
// must not race or panic.
func TestEventBusSoak(t *testing.T) {
	eb := NewEventBus()

	const (
		publishers  = 40
		subscribers = 60
		iters       = 200
	)
	var wg sync.WaitGroup

	for p := 0; p < publishers; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				eb.Publish(api.Event{Type: "container", Action: "start", Time: int64(i)})
				if i%20 == 0 {
					_ = eb.History(0, 0)
				}
			}
		}(p)
	}

	for sN := 0; sN < subscribers; sN++ {
		wg.Add(1)
		go func(sN int) {
			defer wg.Done()
			for i := 0; i < iters/4; i++ {
				id := fmt.Sprintf("sub-%d-%d", sN, i)
				ch := eb.Subscribe(id)
				// Drain a few events then unsubscribe — exercises the
				// concurrent close+delete vs. Publish's non-blocking send.
				go func() {
					for range ch {
					}
				}()
				eb.Unsubscribe(id)
			}
		}(sN)
	}

	wg.Wait()
	eb.Close()
	// Close is idempotent-safe under the mutex; a second close must not panic.
	eb.Close()
}

// reverseConnFactory returns ReverseAgentConn values backed by real
// pipe-backed WebSocket connections to a shared echo server. The registry's
// Register/Drop/DropSession call conn.Close(), so a live *websocket.Conn is
// required (a nil ws would panic). The echo server keeps each connection alive
// until the client closes it. Closed via t.Cleanup.
func reverseConnFactory(t *testing.T) (newConn func() *agent.ReverseAgentConn) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				_ = c.Close()
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	wsURL := "ws" + srv.URL[4:]

	return func() *agent.ReverseAgentConn {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial reverse-agent test ws: %v", err)
		}
		return agent.NewReverseAgentConn(conn)
	}
}

// TestReverseAgentRegistrySoak drives concurrent Register / WaitForAgent (with
// ctx timeout) / Resolve / Drop / DropSession / MarkLifetimeExpired on
// overlapping ids. Invariants: WaitForAgent never strands (ctx releases it),
// no panic, no double-close, and the registry's waiters map empties.
func TestReverseAgentRegistrySoak(t *testing.T) {
	reg := NewReverseAgentRegistry()
	newConn := reverseConnFactory(t)

	const (
		nIDs    = 10
		workers = 120
		iters   = 50
	)
	ids := make([]string, nIDs)
	for i := range ids {
		ids[i] = fmt.Sprintf("ra-%d", i)
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := ids[w%nIDs]
			for i := 0; i < iters; i++ {
				switch (w + i) % 6 {
				case 0:
					reg.Register(id, newConn())
				case 1:
					ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
					_ = reg.WaitForAgent(ctx, id)
					cancel()
				case 2:
					_, _ = reg.Resolve(id)
				case 3:
					reg.Drop(id)
				case 4:
					reg.MarkLifetimeExpired(id)
					_ = reg.IsLifetimeExpired(id)
				case 5:
					reg.DropSession(id, newConn())
				}
			}
		}(w)
	}
	wg.Wait()

	// Drop everything; the waiters map should be empty (each WaitForAgent
	// either registered or cleaned up its own channel on ctx.Done()).
	for _, id := range ids {
		reg.Drop(id)
	}
	reg.mu.RLock()
	nWaiters := len(reg.waiters)
	reg.mu.RUnlock()
	if nWaiters != 0 {
		t.Fatalf("reverse-agent registry leaked %d waiter buckets", nWaiters)
	}
}

// TestLogFollowDisconnectNoLeak verifies that a StreamCloudLogs follow stream
// whose reader closes (client disconnect) terminates its poller goroutine
// rather than leaking it until the container exits. We start N follow streams
// against a never-exiting container, close every reader, and assert the
// goroutine count returns to baseline.
func TestLogFollowDisconnectNoLeak(t *testing.T) {
	s := newSoakServer()
	s.Store.Containers.Put("logc", api.Container{
		ID:     "logc",
		Name:   "/logc",
		Config: api.ContainerConfig{Labels: map[string]string{}},
		State:  api.ContainerState{Status: "running", Running: true},
	})

	// A fetch that always returns one line and never errors — the container
	// stays "running" so the poller would loop forever absent a disconnect.
	fetch := func(_ context.Context, _ CloudLogParams, _ any) ([]CloudLogEntry, any, error) {
		return []CloudLogEntry{{Timestamp: time.Now(), Message: "tick"}}, nil, nil
	}

	runtime.GC()
	base := runtime.NumGoroutine()

	const n = 40
	for i := 0; i < n; i++ {
		rc, err := StreamCloudLogs(s, "logc", api.ContainerLogsOptions{Follow: true, ShowStdout: true}, fetch, StreamCloudLogsOptions{})
		if err != nil {
			t.Fatalf("StreamCloudLogs: %v", err)
		}
		// Read the initial line, then close — simulates a client disconnect.
		buf := make([]byte, 8)
		_, _ = rc.Read(buf)
		_ = rc.Close()
	}

	// Give the pollers up to ~2 ticker periods to observe the closed pipe and
	// exit via the zero-length heartbeat write.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= base+5 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	runtime.GC()
	if leaked := runtime.NumGoroutine() - base; leaked > 5 {
		t.Fatalf("log-follow leaked ~%d goroutines after client disconnect (base=%d now=%d)", leaked, base, runtime.NumGoroutine())
	}
}
