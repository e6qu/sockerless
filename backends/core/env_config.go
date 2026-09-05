package core

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// Environment-variable readers shared by every cloud backend's Config.
// Each backend names its own variables; the reading, defaulting, and
// parsing of the values is the same everywhere.

// EnvOrDefault returns the variable's value, or def when it is unset or
// empty.
func EnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvOrDefaultInt returns the variable parsed as an integer, or def when
// it is unset, empty, or not an integer.
func EnvOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// FirstNonEmpty returns the first argument that is not the empty string.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// DurationOrDefault parses s as a Go duration, or returns def when s is
// empty or malformed. Startup validation of the underlying variable is
// ValidateDurationEnvs' job; this reader only supplies the value.
func DurationOrDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// SplitCSV splits a comma-separated list, trimming whitespace and
// dropping empty entries. An empty input yields nil.
func SplitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NetworkDiscoveryFromEnv reads the operator's network-discovery choice
// from envVar, or returns def when unset. Validation against the set a
// backend supports happens in that backend's Config.Validate.
func NetworkDiscoveryFromEnv(envVar string, def api.NetworkDiscoveryKind) api.NetworkDiscoveryKind {
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		return api.NetworkDiscoveryKind(v)
	}
	return def
}

// AccessFromEnv reads the operator's access-mechanism choice from envVar,
// or returns def when unset. Validation against the set a backend supports
// happens in that backend's Config.Validate.
func AccessFromEnv(envVar string, def api.AccessMechanism) api.AccessMechanism {
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		return api.AccessMechanism(v)
	}
	return def
}
