package main

import (
	"testing"
	"time"
)

func TestSchedulerCronNext(t *testing.T) {
	// A Wednesday, 2026-06-10 12:34:00 UTC.
	base := time.Date(2026, 6, 10, 12, 34, 0, 0, time.UTC)

	cases := []struct {
		name string
		expr string
		want time.Time
	}{
		{
			name: "daily at 02:00 next day",
			expr: "cron(0 2 * * ? *)",
			want: time.Date(2026, 6, 11, 2, 0, 0, 0, time.UTC),
		},
		{
			name: "every 15 minutes -> next quarter hour",
			expr: "cron(*/15 * * * ? *)",
			want: time.Date(2026, 6, 10, 12, 45, 0, 0, time.UTC),
		},
		{
			name: "weekdays 14:30 -> same Wednesday",
			expr: "cron(30 14 ? * MON-FRI *)",
			want: time.Date(2026, 6, 10, 14, 30, 0, 0, time.UTC),
		},
		{
			name: "specific minute list -> next listed minute",
			expr: "cron(0,30 * * * ? *)",
			want: time.Date(2026, 6, 10, 13, 0, 0, 0, time.UTC),
		},
		{
			name: "Jan 1 midnight -> next year",
			expr: "cron(0 0 1 JAN ? *)",
			want: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "day-of-month restricted (15th 09:00)",
			expr: "cron(0 9 15 * ? *)",
			want: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := schedulerCronNext(c.expr, base)
			if !ok {
				t.Fatalf("schedulerCronNext(%q) returned ok=false", c.expr)
			}
			if !got.Equal(c.want) {
				t.Fatalf("schedulerCronNext(%q) = %s, want %s", c.expr, got.Format(time.RFC3339), c.want.Format(time.RFC3339))
			}
		})
	}
}

// TestSchedulerCronWiring covers the firing-loop integration of cron: the loop
// must treat a cron schedule as recurring and compute a future first-fire time
// (the in-process dispatch itself is covered by TestScheduler_FiresECSTarget).
func TestSchedulerCronWiring(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s := Schedule{
		Name:               "cron-wire",
		GroupName:          "default",
		ScheduleExpression: "cron(0 2 * * ? *)",
		State:              "ENABLED",
		CreationDate:       float64(now.Unix()),
	}
	next, ok := schedulerFirstFire(s, now)
	if !ok || !next.After(now) {
		t.Fatalf("schedulerFirstFire(cron) = %s ok=%v, want a future time", next, ok)
	}
	if !schedulerRecurring(s.ScheduleExpression) {
		t.Fatal("cron(...) must be recurring so it re-fires and isn't auto-deleted")
	}
}

func TestSchedulerCronNext_UnsupportedOrInvalid(t *testing.T) {
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	for _, expr := range []string{
		"cron(0 2 L * ? *)",   // L qualifier unsupported
		"cron(0 2 ? * 6#3 *)", // # qualifier unsupported
		"cron(0 2 * * ?)",     // only 5 fields (not 6)
		"rate(5 minutes)",     // not a cron expression
		"cron(99 2 * * ? *)",  // minute out of range
	} {
		if _, ok := schedulerCronNext(expr, base); ok {
			t.Fatalf("schedulerCronNext(%q) returned ok=true, want false", expr)
		}
	}
}
