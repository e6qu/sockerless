package bleephub

import (
	"testing"
	"time"
)

func TestParseCron(t *testing.T) {
	cases := []struct {
		expr    string
		t       time.Time
		want    bool
		wantErr bool
	}{
		// every minute
		{expr: "* * * * *", t: time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC), want: true},
		// specific minute/hour
		{expr: "30 10 * * *", t: time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC), want: true},
		{expr: "30 10 * * *", t: time.Date(2026, 6, 12, 10, 31, 0, 0, time.UTC), want: false},
		// steps
		{expr: "*/15 * * * *", t: time.Date(2026, 6, 12, 10, 45, 0, 0, time.UTC), want: true},
		{expr: "*/15 * * * *", t: time.Date(2026, 6, 12, 10, 40, 0, 0, time.UTC), want: false},
		// ranges with step
		{expr: "0 9-17/2 * * *", t: time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC), want: true},
		{expr: "0 9-17/2 * * *", t: time.Date(2026, 6, 12, 14, 0, 0, 0, time.UTC), want: false},
		// weekday range (2026-06-12 is a Friday)
		{expr: "0 4 * * 1-5", t: time.Date(2026, 6, 12, 4, 0, 0, 0, time.UTC), want: true},
		{expr: "0 4 * * 1-5", t: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC), want: false}, // Sunday
		// names
		{expr: "0 0 * JUN FRI", t: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), want: true},
		{expr: "0 0 * JUL *", t: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), want: false},
		// dow 7 == Sunday
		{expr: "0 0 * * 7", t: time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), want: true},
		// dom/dow OR rule: both restricted → either matches
		{expr: "0 0 1 * FRI", t: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), want: true},  // Friday, not the 1st
		{expr: "0 0 1 * FRI", t: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), want: true},   // the 1st (a Wednesday)
		{expr: "0 0 1 * FRI", t: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), want: false}, // Saturday the 13th
		// dom restricted, dow star → dom decides
		{expr: "0 0 13 * *", t: time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC), want: true},
		{expr: "0 0 13 * *", t: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), want: false},
		// lists
		{expr: "0,30 0 * * *", t: time.Date(2026, 6, 12, 0, 30, 0, 0, time.UTC), want: true},
		// errors
		{expr: "* * * *", wantErr: true},     // 4 fields
		{expr: "60 * * * *", wantErr: true},  // minute out of range
		{expr: "* 24 * * *", wantErr: true},  // hour out of range
		{expr: "* * 0 * *", wantErr: true},   // dom out of range
		{expr: "* * * 13 *", wantErr: true},  // month out of range
		{expr: "* * * * 8", wantErr: true},   // dow out of range
		{expr: "*/0 * * * *", wantErr: true}, // zero step
		{expr: "5-1 * * * *", wantErr: true}, // inverted range
		{expr: "x * * * *", wantErr: true},   // garbage
		{expr: "* * * BOB *", wantErr: true}, // bad name
	}
	for _, tc := range cases {
		cs, err := parseCron(tc.expr)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseCron(%q) expected error", tc.expr)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseCron(%q): %v", tc.expr, err)
			continue
		}
		if got := cs.matches(tc.t); got != tc.want {
			t.Errorf("cron %q at %s = %v, want %v", tc.expr, tc.t.Format("2006-01-02 15:04 Mon"), got, tc.want)
		}
	}
}

func TestFireDueSchedules(t *testing.T) {
	repoKey := "cronowner/cron-repo"
	cancelRepoRunsCleanup(t, repoKey)
	commitWorkflowYAMLToStorage(t, testServer, repoKey, ".github/workflows/nightly.yml", `name: nightly
on:
  schedule:
    - cron: '30 4 * * *'
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo nightly
`)

	countRuns := func() int {
		testServer.store.mu.RLock()
		defer testServer.store.mu.RUnlock()
		n := 0
		for _, w := range testServer.store.Workflows {
			if w.RepoFullName == repoKey && w.EventName == "schedule" {
				n++
			}
		}
		return n
	}

	// Non-matching minute: nothing fires.
	testServer.fireDueSchedules(time.Date(2026, 6, 12, 4, 29, 0, 0, time.UTC))
	if got := countRuns(); got != 0 {
		t.Fatalf("4:29 fired %d runs, want 0", got)
	}

	// Matching minute fires exactly once.
	at := time.Date(2026, 6, 12, 4, 30, 0, 0, time.UTC)
	testServer.fireDueSchedules(at)
	if got := countRuns(); got != 1 {
		t.Fatalf("4:30 fired %d runs, want 1", got)
	}

	// Same minute again: deduped.
	testServer.fireDueSchedules(at.Add(10 * time.Second))
	if got := countRuns(); got != 1 {
		t.Fatalf("4:30 re-tick fired %d runs total, want still 1", got)
	}

	// Next day fires again.
	testServer.fireDueSchedules(at.Add(24 * time.Hour))
	if got := countRuns(); got != 2 {
		t.Fatalf("next-day 4:30 fired %d runs total, want 2", got)
	}

	// The run carries schedule event metadata.
	testServer.store.mu.RLock()
	var run *Workflow
	for _, w := range testServer.store.Workflows {
		if w.RepoFullName == repoKey && w.EventName == "schedule" {
			run = w
			break
		}
	}
	testServer.store.mu.RUnlock()
	if run == nil {
		t.Fatal("no schedule run found")
	}
	if run.EventPayload["schedule"] != "30 4 * * *" {
		t.Errorf("payload schedule = %v", run.EventPayload["schedule"])
	}
	if run.Ref != "refs/heads/main" && run.Ref != "refs/heads/master" {
		t.Errorf("schedule run ref = %q, want default branch", run.Ref)
	}
}
