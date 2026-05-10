package main

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// ObservabilityConfig is what the UI reads at startup to decide
// whether to render OTel deep-link buttons and which dashboards to
// point at.
//
// Driven by env vars admin reads at boot (NOT on each request) — the
// operator brings up the observability stack with `make
// stack-observability-up` then exports OTEL_LOGS_DASHBOARD +
// OTEL_TRACES_DASHBOARD before starting admin.
//
//	OTEL_LOGS_DASHBOARD=http://localhost:9428/select/vmui
//	OTEL_TRACES_DASHBOARD=http://localhost:16686/search
//
// Empty / unset = observability stack not in use → UI hides the deep
// links. Explicit URLs = stack present → UI renders them with the
// instance name as a query filter.
type ObservabilityConfig struct {
	Enabled            bool   `json:"enabled"`
	LogsDashboard      string `json:"logs_dashboard,omitempty"`
	TracesDashboard    string `json:"traces_dashboard,omitempty"`
	LogsServiceParam   string `json:"logs_service_param,omitempty"`
	TracesServiceParam string `json:"traces_service_param,omitempty"`
}

// loadObservabilityConfig reads the dashboard env vars at admin boot.
// Returns Enabled=false when neither dashboard URL is set.
func loadObservabilityConfig() ObservabilityConfig {
	cfg := ObservabilityConfig{
		LogsDashboard:   strings.TrimSpace(os.Getenv("OTEL_LOGS_DASHBOARD")),
		TracesDashboard: strings.TrimSpace(os.Getenv("OTEL_TRACES_DASHBOARD")),
		// Default to the canonical OTel resource-attribute name.
		// Operators using a custom collector pipeline can override.
		LogsServiceParam:   envOrDefault("OTEL_LOGS_SERVICE_PARAM", "service.name"),
		TracesServiceParam: envOrDefault("OTEL_TRACES_SERVICE_PARAM", "service"),
	}
	cfg.Enabled = cfg.LogsDashboard != "" || cfg.TracesDashboard != ""
	return cfg
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// handleObservabilityConfig serves the static config so the UI knows
// whether to render deep-link buttons.
func handleObservabilityConfig(cfg ObservabilityConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, cfg)
	}
}

// BuildLogsURL composes a dashboard URL with the instance name as a
// query filter. Returns empty string when LogsDashboard is unset.
//
// Exported (lower-case in Go since it's package-internal, but
// reachable from other admin files) so future endpoints can construct
// the same URL without re-implementing the encoding logic.
func (c ObservabilityConfig) BuildLogsURL(serviceName string) string {
	return appendServiceFilter(c.LogsDashboard, c.LogsServiceParam, serviceName)
}

// BuildTracesURL is the traces analogue of BuildLogsURL.
func (c ObservabilityConfig) BuildTracesURL(serviceName string) string {
	return appendServiceFilter(c.TracesDashboard, c.TracesServiceParam, serviceName)
}

func appendServiceFilter(dashboard, param, service string) string {
	if dashboard == "" || service == "" {
		return dashboard
	}
	u, err := url.Parse(dashboard)
	if err != nil {
		return dashboard
	}
	q := u.Query()
	q.Set(param, service)
	u.RawQuery = q.Encode()
	return u.String()
}
