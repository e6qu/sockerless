package scopes

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestWaitFromRateHeaders_Reset(t *testing.T) {
	future := time.Now().Add(60 * time.Second).Unix()
	h := http.Header{}
	h.Set("X-RateLimit-Reset", fmt.Sprintf("%d", future))
	got := WaitFromRateHeaders(h, time.Now())
	// 60s * 1.10 = 66s; +1s = 67s. Allow a small slack for test timing.
	if got < 65*time.Second || got > 70*time.Second {
		t.Fatalf("expected wait near 67s, got %v", got)
	}
}

func TestWaitFromRateHeaders_RetryAfterPreferredWhenLarger(t *testing.T) {
	future := time.Now().Add(10 * time.Second).Unix()
	h := http.Header{}
	h.Set("X-RateLimit-Reset", fmt.Sprintf("%d", future))
	h.Set("Retry-After", "120")
	got := WaitFromRateHeaders(h, time.Now())
	// 120s wins over 10s; 120 * 1.10 + 1 = 133s.
	if got < 130*time.Second || got > 135*time.Second {
		t.Fatalf("expected wait near 133s, got %v", got)
	}
}

func TestWaitFromRateHeaders_NoHeaders(t *testing.T) {
	got := WaitFromRateHeaders(http.Header{}, time.Now())
	if got != 0 {
		t.Fatalf("expected 0 wait, got %v", got)
	}
}

func TestWaitFromRateHeaders_PastReset(t *testing.T) {
	past := time.Now().Add(-60 * time.Second).Unix()
	h := http.Header{}
	h.Set("X-RateLimit-Reset", fmt.Sprintf("%d", past))
	got := WaitFromRateHeaders(h, time.Now())
	if got != 0 {
		t.Fatalf("expected 0 wait for past reset, got %v", got)
	}
}
