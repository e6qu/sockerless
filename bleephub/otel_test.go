package bleephub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestInitTracerNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer shutdown(context.Background())

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if span.SpanContext().IsValid() {
		t.Error("expected no-op span when OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}
}

func TestInitTracerWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")

	shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer shutdown(context.Background())

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Error("expected valid SpanContext when OTEL_EXPORTER_OTLP_ENDPOINT is set")
	}

	otel.SetTracerProvider(trace.NewNoopTracerProvider())
}

func TestHTTPMiddlewareCreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(trace.NewNoopTracerProvider())

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
}

func TestWorkflowDispatchCreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(trace.NewNoopTracerProvider())

	s := newTestServer()
	wf := &WorkflowDef{
		Name: "otel-test",
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "echo hi"}}},
		},
	}

	_, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submitWorkflow failed: %v", err)
	}

	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	found := false
	for _, span := range spans {
		if span.Name == "submitWorkflow" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, len(spans))
		for i, span := range spans {
			names[i] = span.Name
		}
		t.Errorf("expected submitWorkflow span, got: %v", names)
	}
}

func TestNoSpansWhenDisabled(t *testing.T) {
	// Use default no-op provider — should not crash
	otel.SetTracerProvider(trace.NewNoopTracerProvider())

	s := newTestServer()
	wf := &WorkflowDef{
		Name: "no-trace-test",
		Jobs: map[string]*JobDef{
			"build": {Steps: []StepDef{{Run: "echo hi"}}},
		},
	}

	_, err := s.submitWorkflow(context.Background(), "http://localhost", wf, "alpine:latest")
	if err != nil {
		t.Fatalf("submitWorkflow with no-op tracer should not fail: %v", err)
	}
}
