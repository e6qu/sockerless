package bleephub

import (
	"runtime"
	"sync"
	"time"
)

// Metrics collects bleephub-specific operational metrics.
type Metrics struct {
	mu                  sync.Mutex
	WorkflowSubmissions int64            `json:"workflow_submissions"`
	JobDispatches       int64            `json:"job_dispatches"`
	JobCompletions      map[string]int64 `json:"job_completions"` // result → count
	JobDurations        []time.Duration  `json:"-"`
	ActiveWorkflows     int64            `json:"active_workflows"`
	ActiveSessions      int64            `json:"active_sessions"`
	StartedAt           time.Time        `json:"-"`
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		JobCompletions: make(map[string]int64),
		StartedAt:      time.Now(),
	}
}

// RecordWorkflowSubmit increments the workflow submission counter.
func (m *Metrics) RecordWorkflowSubmit() {
	m.mu.Lock()
	m.WorkflowSubmissions++
	m.ActiveWorkflows++
	m.mu.Unlock()
}

// RecordWorkflowComplete decrements active workflows.
func (m *Metrics) RecordWorkflowComplete() {
	m.mu.Lock()
	if m.ActiveWorkflows > 0 {
		m.ActiveWorkflows--
	}
	m.mu.Unlock()
}

// RecordJobDispatch increments the job dispatch counter.
func (m *Metrics) RecordJobDispatch() {
	m.mu.Lock()
	m.JobDispatches++
	m.mu.Unlock()
}

// RecordJobCompletion records a job completion with its result and duration.
func (m *Metrics) RecordJobCompletion(result string, duration time.Duration) {
	m.mu.Lock()
	m.JobCompletions[result]++
	if len(m.JobDurations) < 1000 {
		m.JobDurations = append(m.JobDurations, duration)
	} else {
		m.JobDurations = append(m.JobDurations[500:], duration)
	}
	m.mu.Unlock()
}

// SetActiveSessions sets the active session gauge.
func (m *Metrics) SetActiveSessions(n int64) {
	m.mu.Lock()
	m.ActiveSessions = n
	m.mu.Unlock()
}

// MetricsSnapshot is a point-in-time metrics report.
type MetricsSnapshot struct {
	WorkflowSubmissions int64            `json:"workflow_submissions"`
	JobDispatches       int64            `json:"job_dispatches"`
	JobCompletions      map[string]int64 `json:"job_completions"`
	ActiveWorkflows     int64            `json:"active_workflows"`
	ActiveSessions      int64            `json:"active_sessions"`
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
		WorkflowSubmissions: m.WorkflowSubmissions,
		JobDispatches:       m.JobDispatches,
		JobCompletions:      completions,
		ActiveWorkflows:     m.ActiveWorkflows,
		ActiveSessions:      m.ActiveSessions,
		UptimeSeconds:       int(time.Since(m.StartedAt).Seconds()),
		Goroutines:          runtime.NumGoroutine(),
		HeapAllocMB:         float64(memStats.HeapAlloc) / (1024 * 1024),
	}
}
