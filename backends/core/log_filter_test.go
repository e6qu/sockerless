package core

import (
	"testing"
	"time"
)

func TestFilterLogTail_LastN(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	got := FilterLogTail(lines, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
	if got[0] != "h" || got[1] != "i" || got[2] != "j" {
		t.Fatalf("expected [h i j], got %v", got)
	}
}

func TestFilterLogTail_All(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := FilterLogTail(lines, 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
}

func TestFilterLogTail_Zero(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := FilterLogTail(lines, 0)
	if len(got) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(got))
	}
}

func TestFilterLogSince(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	lines := make([]string, 5)
	for i := range lines {
		ts := base.Add(time.Duration(i) * time.Minute)
		lines[i] = ts.Format(time.RFC3339Nano) + " line " + string(rune('A'+i))
	}
	// since = base + 2m → should keep lines at 2m, 3m, 4m
	since := base.Add(2 * time.Minute)
	got := FilterLogSince(lines, since)
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(got), got)
	}
}

func TestFilterLogUntil(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	lines := make([]string, 5)
	for i := range lines {
		ts := base.Add(time.Duration(i) * time.Minute)
		lines[i] = ts.Format(time.RFC3339Nano) + " line " + string(rune('A'+i))
	}
	// until = base + 2m → should keep lines at 0m, 1m (timestamps < until)
	until := base.Add(2 * time.Minute)
	got := FilterLogUntil(lines, until)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(got), got)
	}
}

func TestParseDockerTimestamp(t *testing.T) {
	// RFC3339Nano
	ts, err := ParseDockerTimestamp("2024-01-01T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Year() != 2024 || ts.Month() != 1 || ts.Hour() != 12 {
		t.Fatalf("unexpected parsed time: %v", ts)
	}

	// Unix epoch integer
	ts, err = ParseDockerTimestamp("1704110400")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Unix() != 1704110400 {
		t.Fatalf("unexpected parsed epoch: %d", ts.Unix())
	}
}
