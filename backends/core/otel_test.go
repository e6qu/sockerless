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

func TestInitObservabilityNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability: %v", err)
	}
	if obs.LogWriter != nil {
		t.Errorf("LogWriter should be nil when OTel disabled, got %v", obs.LogWriter)
	}
	if err := obs.Shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown should return nil, got %v", err)
	}
}

func TestInitObservabilityWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability: %v", err)
	}
	if obs.LogWriter == nil {
		t.Errorf("LogWriter should be non-nil when OTel enabled")
	}
	// Cancelled context — don't wait for the OTLP exporter to flush.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = obs.Shutdown(ctx)
}

func TestOTelLogWriter_Write_Nil(t *testing.T) {
	// Nil receiver should be a graceful no-op so callers don't have
	// to special-case "OTel disabled" before passing the writer to a
	// MultiLevelWriter.
	var w *OTelLogWriter
	n, err := w.Write([]byte(`{"level":"info","message":"hi"}`))
	if err != nil {
		t.Errorf("nil writer should not error: %v", err)
	}
	if n != len(`{"level":"info","message":"hi"}`) {
		t.Errorf("n = %d, want full length", n)
	}
}

func TestOTelLogWriter_Write_BadJSON(t *testing.T) {
	// Unparseable line is silently dropped — the consoleWriter half
	// of MultiLevelWriter still gets it, so the operator sees the
	// raw line on stderr.
	w := &OTelLogWriter{}
	n, err := w.Write([]byte("not json"))
	if err != nil {
		t.Errorf("bad JSON should not error: %v", err)
	}
	if n != len("not json") {
		t.Errorf("n = %d", n)
	}
}

func TestZerologLevelToOTel(t *testing.T) {
	cases := map[string]string{
		"trace":   "TRACE",
		"debug":   "DEBUG",
		"info":    "INFO",
		"warn":    "WARN",
		"warning": "WARN",
		"error":   "ERROR",
		"fatal":   "FATAL",
		"panic":   "PANIC",
		"weird":   "weird",
	}
	for in, wantText := range cases {
		_, gotText := zerologLevelToOTel(in)
		if gotText != wantText {
			t.Errorf("zerologLevelToOTel(%q) text = %q, want %q", in, gotText, wantText)
		}
	}
}
