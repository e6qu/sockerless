package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer sets up an OpenTelemetry TracerProvider with an OTLP HTTP exporter
// if OTEL_EXPORTER_OTLP_ENDPOINT is set. Otherwise returns a no-op shutdown function.
//
// Phase 87b — kept for backward compat with callers that don't yet
// want logs export. New callers should prefer InitObservability.
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

// Observability bundles trace + log SDK shutdown + a zerolog Writer
// that mirrors entries to the OTel logs SDK.
type Observability struct {
	// LogWriter is an io.Writer that parses each JSON log line
	// produced by zerolog and emits an OTel log Record. Designed to
	// live alongside the existing zerolog.ConsoleWriter via
	// zerolog.MultiLevelWriter so the operator gets BOTH stderr-
	// formatted output AND OTLP emission. nil when OTel is disabled.
	LogWriter *OTelLogWriter

	// Shutdown flushes both providers (traces + logs) when admin /
	// sim / backend shuts down. No-op when OTel is disabled.
	Shutdown func(context.Context) error
}

// InitObservability sets up both tracer + logger providers when
// OTEL_EXPORTER_OTLP_ENDPOINT is set. Returns a zero-value
// Observability with a no-op Shutdown when OTel is disabled — callers
// can ignore LogWriter being nil and the existing zerolog stderr path
// keeps working unchanged.
//
// Phase 87c. Components-decoupled invariant intact: emission only
// when the env var is set.
func InitObservability(serviceName string) (*Observability, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return &Observability{
			Shutdown: func(context.Context) error { return nil },
		}, nil
	}
	res := resource.NewWithAttributes(
		semconv.SchemaURL, semconv.ServiceNameKey.String(serviceName),
	)

	traceExp, err := otlptracehttp.New(context.Background())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	logExp, err := otlploghttp.New(context.Background())
	if err != nil {
		_ = tp.Shutdown(context.Background())
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	return &Observability{
		LogWriter: &OTelLogWriter{logger: lp.Logger(serviceName)},
		Shutdown: func(ctx context.Context) error {
			return errors.Join(tp.Shutdown(ctx), lp.Shutdown(ctx))
		},
	}, nil
}

// OTelLogWriter is the zerolog → OTel logs bridge. Implements
// io.Writer so it slots into zerolog.MultiLevelWriter alongside the
// existing ConsoleWriter.
type OTelLogWriter struct {
	logger otellog.Logger
}

// Write parses one zerolog JSON line and emits an OTel log Record.
// Best-effort: unparseable lines are silently dropped (the
// consoleWriter half of the MultiLevelWriter still gets them, so the
// operator-visible output is unaffected).
func (w *OTelLogWriter) Write(p []byte) (int, error) {
	if w == nil || w.logger == nil {
		return len(p), nil
	}
	var entry map[string]any
	if err := json.Unmarshal(p, &entry); err != nil {
		return len(p), nil
	}

	var record otellog.Record
	record.SetTimestamp(parseZerologTimestamp(entry))
	record.SetObservedTimestamp(time.Now())

	if msg, ok := entry["message"].(string); ok {
		record.SetBody(otellog.StringValue(msg))
	}
	level, _ := entry["level"].(string)
	severity, severityText := zerologLevelToOTel(level)
	record.SetSeverity(severity)
	record.SetSeverityText(severityText)

	// Promote every non-reserved JSON field to a log attribute.
	for k, v := range entry {
		switch k {
		case "level", "message", "time":
			continue
		}
		record.AddAttributes(otellog.KeyValue{
			Key:   k,
			Value: otelValueOf(v),
		})
	}

	w.logger.Emit(context.Background(), record)
	return len(p), nil
}

func parseZerologTimestamp(entry map[string]any) time.Time {
	if v, ok := entry["time"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Now()
}

// zerologLevelToOTel maps a zerolog level string to the OTel severity
// numbers. zerolog "info" → SeverityInfo, "warn" → SeverityWarn, etc.
// Unknown levels default to SeverityInfo with the original string as
// severity_text so operators can still see what was emitted.
func zerologLevelToOTel(level string) (otellog.Severity, string) {
	switch strings.ToLower(level) {
	case "trace":
		return otellog.SeverityTrace, "TRACE"
	case "debug":
		return otellog.SeverityDebug, "DEBUG"
	case "info":
		return otellog.SeverityInfo, "INFO"
	case "warn", "warning":
		return otellog.SeverityWarn, "WARN"
	case "error":
		return otellog.SeverityError, "ERROR"
	case "fatal":
		return otellog.SeverityFatal, "FATAL"
	case "panic":
		return otellog.SeverityFatal4, "PANIC"
	}
	return otellog.SeverityInfo, level
}

// otelValueOf converts a JSON-decoded value to an OTel log Value.
// Falls back to a JSON-string form for unhandled types (slices,
// nested maps) — preserves visibility without forcing a richer
// schema model on the operator.
func otelValueOf(v any) otellog.Value {
	switch x := v.(type) {
	case nil:
		return otellog.StringValue("")
	case string:
		return otellog.StringValue(x)
	case bool:
		return otellog.BoolValue(x)
	case float64:
		// JSON numbers always decode as float64. Preserve int-shaped
		// values as int64 so dashboards group them naturally.
		if x == float64(int64(x)) {
			return otellog.Int64Value(int64(x))
		}
		return otellog.Float64Value(x)
	case int:
		return otellog.Int64Value(int64(x))
	case int64:
		return otellog.Int64Value(x)
	}
	if b, err := json.Marshal(v); err == nil {
		return otellog.StringValue(string(b))
	}
	return otellog.StringValue("")
}
