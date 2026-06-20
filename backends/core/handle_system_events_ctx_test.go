package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sockerless/api"
)

// ctxEventsBackend wraps a BaseServer and implements the optional
// SystemEventsCtx interface that handleSystemEvents prefers. It blocks
// the returned stream until its context is canceled, recording that the
// context was threaded through (a disconnecting client must cancel it).
type ctxEventsBackend struct {
	api.Backend
	gotCtx     context.Context
	canceledCh chan struct{}
}

func (b *ctxEventsBackend) SystemEventsCtx(ctx context.Context, _ api.EventsOptions) (io.ReadCloser, error) {
	b.gotCtx = ctx
	pr, pw := io.Pipe()
	go func() {
		<-ctx.Done()
		close(b.canceledCh)
		pw.CloseWithError(ctx.Err())
	}()
	return pr, nil
}

// TestHandleSystemEvents_PrefersCtxVariantAndCancels verifies the handler
// routes to SystemEventsCtx when the backend implements it, and that a
// client disconnect (request-context cancellation) propagates to the
// upstream stream instead of leaking the producer.
func TestHandleSystemEvents_PrefersCtxVariantAndCancels(t *testing.T) {
	s := newExtendedTestServer()
	backend := &ctxEventsBackend{Backend: s, canceledCh: make(chan struct{})}
	s.self = backend

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleSystemEvents(rec, req)
		close(done)
	}()

	// Simulate the client disconnecting.
	cancel()

	select {
	case <-backend.canceledCh:
	case <-time.After(2 * time.Second):
		t.Fatal("SystemEventsCtx stream was not canceled on client disconnect")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after stream canceled")
	}

	if backend.gotCtx == nil {
		t.Fatal("SystemEventsCtx was not invoked")
	}
}
