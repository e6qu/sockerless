package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// LabelsEnvVar is the single authoritative carrier for a container's Docker
// labels across every cloud backend. Backends write the labels as base64-JSON
// into this env var on the container/main spec and read them back from the
// cloud resource's env during stateless reconstruction.
//
// An env var on the container spec is the reliable carrier: unlike resource
// labels/annotations it is never normalized or stripped by a cloud control
// plane, so it round-trips identically on every backend. There is no
// second-source reconstruction fallback — a present-but-malformed value is a
// writer bug and is surfaced, not silently degraded.
const LabelsEnvVar = "SOCKERLESS_LABELS"

// EncodeLabelsEnvValue returns the base64-JSON value to store under
// LabelsEnvVar, and ok=false when there are no labels to carry (the env var
// should then be omitted).
func EncodeLabelsEnvValue(labels map[string]string) (value string, ok bool) {
	if len(labels) == 0 {
		return "", false
	}
	raw, _ := json.Marshal(labels)
	return base64.StdEncoding.EncodeToString(raw), true
}

// DecodeLabelsEnvValue decodes a LabelsEnvVar value. An empty value yields
// (nil, nil) — the container simply has no Docker labels. A present-but-
// undecodable value returns an error: the writing backend produced garbage and
// the resource is inconsistent, which must surface rather than reconstruct an
// empty label set.
func DecodeLabelsEnvValue(value string) (map[string]string, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("malformed %s base64: %w", LabelsEnvVar, err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("malformed %s JSON: %w", LabelsEnvVar, err)
	}
	return out, nil
}

// LabelsFromEnvSlice scans a []string env (each "KEY=VALUE") for LabelsEnvVar
// and decodes it. Returns (nil, nil) when the var is absent.
func LabelsFromEnvSlice(env []string) (map[string]string, error) {
	prefix := LabelsEnvVar + "="
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, prefix); ok {
			return DecodeLabelsEnvValue(v)
		}
	}
	return nil, nil
}
