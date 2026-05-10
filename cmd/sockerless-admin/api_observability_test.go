package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadObservabilityConfigDisabled(t *testing.T) {
	t.Setenv("OTEL_LOGS_DASHBOARD", "")
	t.Setenv("OTEL_TRACES_DASHBOARD", "")
	cfg := loadObservabilityConfig()
	if cfg.Enabled {
		t.Errorf("Enabled should be false when both URLs unset, got %+v", cfg)
	}
}

func TestLoadObservabilityConfigLogsOnly(t *testing.T) {
	t.Setenv("OTEL_LOGS_DASHBOARD", "http://localhost:9428/select/vmui")
	t.Setenv("OTEL_TRACES_DASHBOARD", "")
	cfg := loadObservabilityConfig()
	if !cfg.Enabled {
		t.Errorf("Enabled should be true when logs URL set, got %+v", cfg)
	}
	if cfg.LogsDashboard == "" {
		t.Errorf("LogsDashboard should be non-empty")
	}
	if cfg.TracesDashboard != "" {
		t.Errorf("TracesDashboard should be empty")
	}
}

func TestLoadObservabilityConfigDefaults(t *testing.T) {
	t.Setenv("OTEL_LOGS_DASHBOARD", "http://logs/")
	t.Setenv("OTEL_LOGS_SERVICE_PARAM", "")
	t.Setenv("OTEL_TRACES_SERVICE_PARAM", "")
	cfg := loadObservabilityConfig()
	if cfg.LogsServiceParam != "service.name" {
		t.Errorf("LogsServiceParam default = %q, want service.name", cfg.LogsServiceParam)
	}
	if cfg.TracesServiceParam != "service" {
		t.Errorf("TracesServiceParam default = %q, want service", cfg.TracesServiceParam)
	}
}

func TestBuildLogsURL(t *testing.T) {
	cfg := ObservabilityConfig{
		LogsDashboard:    "http://localhost:9428/select/vmui",
		LogsServiceParam: "service.name",
	}
	got := cfg.BuildLogsURL("sim-aws")
	// service.name → "service.name=sim-aws" url-encoded.
	if !strings.Contains(got, "service.name") || !strings.Contains(got, "sim-aws") {
		t.Errorf("expected service filter in %q", got)
	}
}

func TestBuildLogsURLEmptyDashboard(t *testing.T) {
	cfg := ObservabilityConfig{LogsDashboard: ""}
	if got := cfg.BuildLogsURL("sim-aws"); got != "" {
		t.Errorf("empty dashboard → empty URL, got %q", got)
	}
}

func TestBuildTracesURL(t *testing.T) {
	cfg := ObservabilityConfig{
		TracesDashboard:    "http://localhost:16686/search",
		TracesServiceParam: "service",
	}
	got := cfg.BuildTracesURL("sim-aws")
	if !strings.Contains(got, "service=sim-aws") {
		t.Errorf("expected service=sim-aws in %q", got)
	}
}

func TestObservabilityEndpoint(t *testing.T) {
	t.Setenv("OTEL_LOGS_DASHBOARD", "http://localhost:9428/select/vmui")
	t.Setenv("OTEL_TRACES_DASHBOARD", "http://localhost:16686/search")
	cfg := loadObservabilityConfig()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/observability", handleObservabilityConfig(cfg))

	req := httptest.NewRequest("GET", "/api/v1/observability", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got ObservabilityConfig
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if !got.Enabled {
		t.Errorf("expected Enabled=true, got %+v", got)
	}
	if got.LogsDashboard == "" || got.TracesDashboard == "" {
		t.Errorf("dashboards missing: %+v", got)
	}
}
