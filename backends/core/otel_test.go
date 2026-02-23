package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInitTracerNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer shutdown(context.Background())

	// With no endpoint, the global TracerProvider should remain the default no-op.
	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if span.SpanContext().IsValid() {
		t.Error("expected no-op span (invalid SpanContext) when OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}
}

func TestInitTracerWithEndpoint(t *testing.T) {
	// Point at a dummy endpoint — we don't need it to actually accept traces
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")

	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer shutdown(context.Background())

	// The global TracerProvider should now be a real one.
	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Error("expected valid SpanContext when OTEL_EXPORTER_OTLP_ENDPOINT is set")
	}

	// Reset to no-op for other tests
	otel.SetTracerProvider(noop.NewTracerProvider())
}

func TestHTTPMiddlewareCreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(noop.NewTracerProvider())

	handler := otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "test-server")

	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/test")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	resp.Body.Close()

	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span from HTTP middleware")
	}

	found := false
	for _, s := range spans {
		if s.Name == "test-server" || s.Name == "GET" || s.Name == "GET /test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a server span, got spans: %v", spanNames(spans))
	}
}

func spanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	return names
}
