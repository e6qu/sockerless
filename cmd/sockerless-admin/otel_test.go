package main

import (
	"context"
	"testing"
)

func TestInitObservabilityNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability failed: %v", err)
	}
	if obs == nil || obs.Shutdown == nil {
		t.Fatal("expected non-nil Observability with Shutdown")
	}
	if err := obs.Shutdown(context.Background()); err != nil {
		t.Errorf("no-op Shutdown should return nil, got %v", err)
	}
}

func TestInitObservabilityWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability failed: %v", err)
	}
	if obs == nil || obs.Shutdown == nil {
		t.Fatal("expected non-nil Observability with Shutdown")
	}
	// Don't wait for the OTLP exporter to flush — Shutdown returns
	// once the pending batch flushes or the context is done.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = obs.Shutdown(ctx)
}
