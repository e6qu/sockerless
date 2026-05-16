package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestInitObservabilityTracerNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability failed: %v", err)
	}
	defer obs.Shutdown(context.Background())

	// With no endpoint, the global TracerProvider should remain the default no-op.
	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if span.SpanContext().IsValid() {
		t.Error("expected no-op span (invalid SpanContext) when OTEL_EXPORTER_OTLP_ENDPOINT is unset")
	}
}

func TestInitObservabilityTracerWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")

	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability failed: %v", err)
	}
	defer obs.Shutdown(context.Background())

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if !span.SpanContext().IsValid() {
		t.Error("expected valid SpanContext when OTEL_EXPORTER_OTLP_ENDPOINT is set")
	}

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

func TestInitObservabilitySetsPropagator(t *testing.T) {
	// Reset to a known-empty propagator first so we can detect the change.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = obs.Shutdown(ctx)
	}()

	// Confirm a TraceContext propagator is now installed by injecting
	// a synthetic span context into a header carrier and verifying the
	// traceparent header lands.
	prop := otel.GetTextMapPropagator()
	carrier := propagation.MapCarrier{}
	prop.Inject(context.Background(), carrier)
	// An empty/no-op propagator wouldn't write anything; the composite
	// TraceContext propagator will at least register Fields() containing
	// "traceparent".
	found := false
	for _, f := range prop.Fields() {
		if f == "traceparent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected traceparent in propagator fields, got %v", prop.Fields())
	}
}

func TestInitObservabilitySetsMeterProvider(t *testing.T) {
	otel.SetMeterProvider(metricnoop.NewMeterProvider())
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	obs, err := InitObservability("test-service")
	if err != nil {
		t.Fatalf("InitObservability: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = obs.Shutdown(ctx)
	}()

	// The global MeterProvider should no longer be the noop one we set
	// before the call. We can't equality-check (sdkmetric is internal
	// to the SDK) but we can confirm Meter() returns a non-nil instance
	// that produces a counter without panicking — the noop SDK does too,
	// so we just assert the instance changed type by Type-asserting against
	// the noop interface.
	mp := otel.GetMeterProvider()
	if _, isNoop := mp.(metricnoop.MeterProvider); isNoop {
		t.Error("expected real MeterProvider after InitObservability, got noop")
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
