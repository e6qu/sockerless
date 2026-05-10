package simulator

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer sets up an OpenTelemetry TracerProvider with an OTLP
// HTTP exporter when OTEL_EXPORTER_OTLP_ENDPOINT is set. Otherwise
// returns a no-op shutdown function. Mirrors the
// `backends/core/otel.go` pattern; lives in the per-cloud sim shared
// package so each cloud's binary can wire it without depending on
// backend-core.
//
// Phase 87b — Stack A. Spans emit when the operator brings up the
// observability stack (`make stack-observability-up`) and exports
// OTEL_EXPORTER_OTLP_ENDPOINT in the sim's env. Components-decoupled
// invariant: unset = today's behaviour, no admin coupling.
func InitTracer(serviceName string) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(context.Background())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL, semconv.ServiceNameKey.String(serviceName),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
