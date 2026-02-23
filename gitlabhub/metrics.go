package gitlabhub

import (
	"runtime"
	"sync"
	"time"
)

// Metrics collects gitlabhub-specific operational metrics.
type Metrics struct {
	mu                   sync.Mutex
	PipelineSubmissions  int64            `json:"pipeline_submissions"`
	JobDispatches        int64            `json:"job_dispatches"`
	JobCompletions       map[string]int64 `json:"job_completions"`
	ActivePipelines      int64            `json:"active_pipelines"`
	RegisteredRunners    int64            `json:"registered_runners"`
	StartedAt            time.Time        `json:"-"`
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		JobCompletions: make(map[string]int64),
		StartedAt:      time.Now(),
	}
}

// RecordPipelineSubmit increments the pipeline submission counter.
func (m *Metrics) RecordPipelineSubmit() {
	m.mu.Lock()
	m.PipelineSubmissions++
	m.ActivePipelines++
	m.mu.Unlock()
}

// RecordPipelineComplete decrements active pipelines.
func (m *Metrics) RecordPipelineComplete() {
	m.mu.Lock()
	if m.ActivePipelines > 0 {
		m.ActivePipelines--
	}
	m.mu.Unlock()
}

// RecordJobDispatch increments the job dispatch counter.
func (m *Metrics) RecordJobDispatch() {
	m.mu.Lock()
	m.JobDispatches++
	m.mu.Unlock()
}

// RecordJobCompletion records a job completion with its result.
func (m *Metrics) RecordJobCompletion(result string) {
	m.mu.Lock()
	m.JobCompletions[result]++
	m.mu.Unlock()
}

// RecordRunnerRegister increments the registered runner gauge.
func (m *Metrics) RecordRunnerRegister() {
	m.mu.Lock()
	m.RegisteredRunners++
	m.mu.Unlock()
}

// MetricsSnapshot is a point-in-time metrics report.
type MetricsSnapshot struct {
	PipelineSubmissions int64            `json:"pipeline_submissions"`
	JobDispatches       int64            `json:"job_dispatches"`
	JobCompletions      map[string]int64 `json:"job_completions"`
	ActivePipelines     int64            `json:"active_pipelines"`
	RegisteredRunners   int64            `json:"registered_runners"`
	UptimeSeconds       int              `json:"uptime_seconds"`
	Goroutines          int              `json:"goroutines"`
	HeapAllocMB         float64          `json:"heap_alloc_mb"`
}

// Snapshot returns a point-in-time copy of all metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	completions := make(map[string]int64, len(m.JobCompletions))
	for k, v := range m.JobCompletions {
		completions[k] = v
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return MetricsSnapshot{
		PipelineSubmissions: m.PipelineSubmissions,
		JobDispatches:       m.JobDispatches,
		JobCompletions:      completions,
		ActivePipelines:     m.ActivePipelines,
		RegisteredRunners:   m.RegisteredRunners,
		UptimeSeconds:       int(time.Since(m.StartedAt).Seconds()),
		Goroutines:          runtime.NumGoroutine(),
		HeapAllocMB:         float64(memStats.HeapAlloc) / (1024 * 1024),
	}
}
