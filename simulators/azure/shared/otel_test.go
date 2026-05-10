package simulator

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
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = shutdown(ctx)
}
