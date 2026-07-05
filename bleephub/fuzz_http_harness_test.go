package bleephub

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFuzzHarnessReachesRealHandlers is a fast, deterministic guard that the
// HTTP fuzz harness genuinely drives real handlers — not a degenerate all-404
// sweep that would let the fuzzers report green while exercising nothing. It
// replays a deterministic fan of decoded requests and asserts (a) a healthy
// spread of outcomes including successful reads/mutations, and (b) the core
// invariant the fuzzers enforce: no handler ever emits HTTP 500.
func TestFuzzHarnessReachesRealHandlers(t *testing.T) {
	fx := newFuzzFixture(t)
	codes := map[int]int{}
	for i := 0; i < 20000; i++ {
		data := []byte{byte(i), byte(i >> 8), byte(i >> 3), byte(i >> 5), byte(i >> 2), byte(i >> 7), byte(i >> 4), byte(i >> 1)}
		req := fx.decodeFuzzRequest(data)
		w := httptest.NewRecorder()
		fx.handler.ServeHTTP(w, req)
		codes[w.Code]++
	}
	if codes[http.StatusInternalServerError] > 0 {
		t.Fatalf("harness produced %d HTTP 500(s): %v", codes[http.StatusInternalServerError], codes)
	}
	// Real handler engagement: successful reads (200) and mutations (200/201/204).
	if codes[http.StatusOK]+codes[http.StatusNoContent]+codes[http.StatusCreated] == 0 {
		t.Fatalf("harness reached no successful handler — likely degraded to all-4xx: %v", codes)
	}
	t.Logf("fuzz-harness status distribution over 20000 decoded requests: %v", codes)
}
