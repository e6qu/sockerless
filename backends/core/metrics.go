package core

import (
	"net/http"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Metrics collects request-level metrics.
type Metrics struct {
	mu         sync.Mutex
	counts     map[string]int64          // "METHOD path" → count
	latencies  map[string][]time.Duration // "METHOD path" → sorted latencies (ring buffer)
	maxSamples int
}

// NewMetrics creates a new Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		counts:     make(map[string]int64),
		latencies:  make(map[string][]time.Duration),
		maxSamples: 1000,
	}
}

// Record records a request.
func (m *Metrics) Record(method, path string, d time.Duration) {
	key := method + " " + path
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key]++
	samples := m.latencies[key]
	if len(samples) >= m.maxSamples {
		// Drop oldest half
		samples = samples[m.maxSamples/2:]
	}
	m.latencies[key] = append(samples, d)
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	snap := MetricsSnapshot{
		Requests: make(map[string]int64, len(m.counts)),
		Latency:  make(map[string]LatencyStats, len(m.latencies)),
	}
	for k, v := range m.counts {
		snap.Requests[k] = v
	}
	for k, samples := range m.latencies {
		if len(samples) == 0 {
			continue
		}
		sorted := make([]time.Duration, len(samples))
		copy(sorted, samples)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		snap.Latency[k] = LatencyStats{
			P50: sorted[len(sorted)*50/100].Milliseconds(),
			P95: sorted[len(sorted)*95/100].Milliseconds(),
			P99: sorted[len(sorted)*99/100].Milliseconds(),
		}
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	snap.Goroutines = runtime.NumGoroutine()
	snap.HeapAllocMB = float64(memStats.HeapAlloc) / (1024 * 1024)

	return snap
}

// MetricsSnapshot is a point-in-time metrics report.
type MetricsSnapshot struct {
	Requests        map[string]int64        `json:"requests"`
	Latency         map[string]LatencyStats `json:"latency_ms"`
	Goroutines      int                     `json:"goroutines"`
	HeapAllocMB     float64                 `json:"heap_alloc_mb"`
	Uptime          int                     `json:"uptime_seconds,omitempty"`
	Containers      int                     `json:"containers,omitempty"`
	ActiveResources int                     `json:"active_resources,omitempty"`
}

// LatencyStats holds percentile latency values in milliseconds.
type LatencyStats struct {
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	P99 int64 `json:"p99"`
}

// MetricsMiddleware returns an http.Handler that records request metrics.
func MetricsMiddleware(m *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		m.Record(r.Method, r.URL.Path, time.Since(start))
	})
}
