package main

import (
	"context"
	"testing"
)

func TestInitTracerNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown should return nil, got %v", err)
	}
}

func TestInitTracerWithEndpoint(t *testing.T) {
	// Mirrors the bleephub + backends/core test pattern: just verify
	// the returned shutdown is non-nil and harmless to call.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	// Don't actually wait for the OTLP exporter to flush — that
	// would hang the test in CI. tp.Shutdown returns once the
	// pending batch flushes or the context is done.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = shutdown(ctx)
}
