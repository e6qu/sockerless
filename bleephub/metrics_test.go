package bleephub

import (
	"testing"
	"time"
)

func TestMetricsSubmitIncrement(t *testing.T) {
	m := NewMetrics()
	m.RecordWorkflowSubmit()
	m.RecordWorkflowSubmit()

	snap := m.Snapshot()
	if snap.WorkflowSubmissions != 2 {
		t.Errorf("submissions = %d, want 2", snap.WorkflowSubmissions)
	}
	if snap.ActiveWorkflows != 2 {
		t.Errorf("active = %d, want 2", snap.ActiveWorkflows)
	}
}

func TestMetricsCompletionByResult(t *testing.T) {
	m := NewMetrics()
	m.RecordJobCompletion("success", 100*time.Millisecond)
	m.RecordJobCompletion("success", 200*time.Millisecond)
	m.RecordJobCompletion("failure", 50*time.Millisecond)

	snap := m.Snapshot()
	if snap.JobCompletions["success"] != 2 {
		t.Errorf("success = %d, want 2", snap.JobCompletions["success"])
	}
	if snap.JobCompletions["failure"] != 1 {
		t.Errorf("failure = %d, want 1", snap.JobCompletions["failure"])
	}
}

func TestMetricsActiveSessions(t *testing.T) {
	m := NewMetrics()
	m.SetActiveSessions(3)

	snap := m.Snapshot()
	if snap.ActiveSessions != 3 {
		t.Errorf("sessions = %d, want 3", snap.ActiveSessions)
	}

	m.SetActiveSessions(1)
	snap = m.Snapshot()
	if snap.ActiveSessions != 1 {
		t.Errorf("sessions = %d, want 1", snap.ActiveSessions)
	}
}
