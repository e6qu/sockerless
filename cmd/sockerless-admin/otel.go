package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// tracedHTTPClient returns an *http.Client whose RoundTripper is wrapped by
// otelhttp.NewTransport. Outgoing requests carry the W3C traceparent /
// tracestate headers from the active span context, so admin → backend hops
// join the same trace instead of starting fresh.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is unset the propagator is a no-op, so
// this is safe to use unconditionally.
func tracedHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// Observability bundles trace + log SDK shutdown + a zerolog Writer
// that mirrors entries to the OTel logs SDK. Mirror of
// `backends/core.Observability` — bleephub is a separate Go module
// without backend-core as a dep, so the bridge lives here.
type Observability struct {
	LogWriter     *OTelLogWriter
	TextLogWriter *TextLogWriter
	Shutdown      func(context.Context) error
}

// InitObservability sets up both tracer + logger providers when
// OTEL_EXPORTER_OTLP_ENDPOINT is set. Returns a zero-value
// Observability with a no-op Shutdown when OTel is disabled.
//
// Components-decoupled invariant intact.
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

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

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

	metricExp, err := otlpmetrichttp.New(context.Background())
	if err != nil {
		_ = tp.Shutdown(context.Background())
		_ = lp.Shutdown(context.Background())
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	_ = runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second))

	return &Observability{
		LogWriter:     &OTelLogWriter{logger: lp.Logger(serviceName)},
		TextLogWriter: &TextLogWriter{logger: lp.Logger(serviceName)},
		Shutdown: func(ctx context.Context) error {
			return errors.Join(tp.Shutdown(ctx), lp.Shutdown(ctx), mp.Shutdown(ctx))
		},
	}, nil
}

// OTelLogWriter — zerolog → OTel logs bridge. Implements io.Writer so
// it slots into zerolog.MultiLevelWriter alongside ConsoleWriter.
type OTelLogWriter struct {
	logger otellog.Logger
}

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
	for k, v := range entry {
		switch k {
		case "level", "message", "time":
			continue
		}
		record.AddAttributes(otellog.KeyValue{Key: k, Value: otelValueOf(v)})
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

// TextLogWriter — stdlib `log` package → OTel logs bridge. Stdlib log
// emits flat text lines (no JSON, no level), so we record them all at
// INFO severity with the trimmed line as the body.
type TextLogWriter struct {
	logger otellog.Logger
}

func (w *TextLogWriter) Write(p []byte) (int, error) {
	if w == nil || w.logger == nil {
		return len(p), nil
	}
	line := strings.TrimRight(string(p), "\n")
	if line == "" {
		return len(p), nil
	}
	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetObservedTimestamp(time.Now())
	record.SetBody(otellog.StringValue(line))
	record.SetSeverity(otellog.SeverityInfo)
	record.SetSeverityText("INFO")
	w.logger.Emit(context.Background(), record)
	return len(p), nil
}

func otelValueOf(v any) otellog.Value {
	switch x := v.(type) {
	case nil:
		return otellog.StringValue("")
	case string:
		return otellog.StringValue(x)
	case bool:
		return otellog.BoolValue(x)
	case float64:
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
