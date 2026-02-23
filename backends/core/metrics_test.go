package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMetricsRecord(t *testing.T) {
	m := NewMetrics()
	m.Record("GET", "/test", 10*time.Millisecond)
	m.Record("GET", "/test", 20*time.Millisecond)
	m.Record("POST", "/other", 5*time.Millisecond)

	snap := m.Snapshot()
	if snap.Requests["GET /test"] != 2 {
		t.Errorf("GET /test count = %d, want 2", snap.Requests["GET /test"])
	}
	if snap.Requests["POST /other"] != 1 {
		t.Errorf("POST /other count = %d, want 1", snap.Requests["POST /other"])
	}
}

func TestMetricsLatencyPercentiles(t *testing.T) {
	m := NewMetrics()
	for i := 1; i <= 100; i++ {
		m.Record("GET", "/p", time.Duration(i)*time.Millisecond)
	}

	snap := m.Snapshot()
	stats := snap.Latency["GET /p"]
	// With 100 samples [1..100]ms, index = len*pct/100
	// p50 → index 50 → value 51ms, p95 → index 95 → value 96ms, p99 → index 99 → value 100ms
	if stats.P50 != 51 {
		t.Errorf("p50 = %d, want 51", stats.P50)
	}
	if stats.P95 != 96 {
		t.Errorf("p95 = %d, want 96", stats.P95)
	}
	if stats.P99 != 100 {
		t.Errorf("p99 = %d, want 100", stats.P99)
	}
}

func TestMetricsMiddleware(t *testing.T) {
	m := NewMetrics()
	handler := MetricsMiddleware(m, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/hello", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	snap := m.Snapshot()
	if snap.Requests["GET /hello"] != 1 {
		t.Errorf("GET /hello count = %d, want 1", snap.Requests["GET /hello"])
	}
}

func TestHandleMetrics(t *testing.T) {
	s := newMgmtTestServer()
	s.Metrics = NewMetrics()
	s.Metrics.Record("GET", "/test", 10*time.Millisecond)

	req := httptest.NewRequest("GET", "/internal/v1/metrics", nil)
	w := httptest.NewRecorder()
	s.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var snap MetricsSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Requests["GET /test"] != 1 {
		t.Errorf("GET /test count = %d, want 1", snap.Requests["GET /test"])
	}
	if snap.Goroutines == 0 {
		t.Error("goroutines should be > 0")
	}
}
