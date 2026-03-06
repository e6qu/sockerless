package core

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// CloudLogParams holds parsed Docker log query parameters for cloud backends.
type CloudLogParams struct {
	Follow     bool
	Timestamps bool
	Tail       int // -1 means all
	Since      time.Time
	Until      time.Time
	WantStdout bool
	Details    bool
	Labels     map[string]string
}

// CloudLogParamsFromOpts creates CloudLogParams from typed ContainerLogsOptions.
// Used by cloud backends implementing api.Backend methods directly.
func CloudLogParamsFromOpts(opts api.ContainerLogsOptions, labels map[string]string) CloudLogParams {
	p := CloudLogParams{
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
		Tail:       -1,
		WantStdout: opts.ShowStdout,
		Labels:     labels,
	}
	if opts.Tail != "" && opts.Tail != "all" {
		if n, err := strconv.Atoi(opts.Tail); err == nil && n >= 0 {
			p.Tail = n
		}
	}
	if opts.Since != "" {
		if t, err := ParseDockerTimestamp(opts.Since); err == nil {
			p.Since = t
		}
	}
	if opts.Until != "" {
		if t, err := ParseDockerTimestamp(opts.Until); err == nil {
			p.Until = t
		}
	}
	return p
}

// ParseCloudLogParams parses Docker log query parameters from an HTTP request.
// labels should be the container's Config.Labels for details support.
func ParseCloudLogParams(r *http.Request, labels map[string]string) CloudLogParams {
	q := r.URL.Query()

	p := CloudLogParams{
		Follow:     q.Get("follow") == "1" || q.Get("follow") == "true",
		Timestamps: q.Get("timestamps") == "1" || q.Get("timestamps") == "true",
		Tail:       -1, // all by default
		WantStdout: true,
		Details:    q.Get("details") == "1" || q.Get("details") == "true",
		Labels:     labels,
	}

	// Parse tail
	if tail := q.Get("tail"); tail != "" && tail != "all" {
		if n, err := strconv.Atoi(tail); err == nil && n >= 0 {
			p.Tail = n
		}
	}

	// Parse since
	if s := q.Get("since"); s != "" {
		if t, err := ParseDockerTimestamp(s); err == nil {
			p.Since = t
		}
	}

	// Parse until
	if u := q.Get("until"); u != "" {
		if t, err := ParseDockerTimestamp(u); err == nil {
			p.Until = t
		}
	}

	// Parse stdout/stderr — cloud logs are always stdout stream.
	// If only stderr requested (stdout not set or false, stderr true), suppress output.
	stdoutParam := q.Get("stdout")
	stderrParam := q.Get("stderr")
	// Default: both true when neither specified.
	// If stderr=true and stdout not explicitly set, only stderr wanted → suppress stdout.
	if stdoutParam == "0" || stdoutParam == "false" {
		p.WantStdout = false
	} else if (stderrParam == "1" || stderrParam == "true") && stdoutParam == "" {
		p.WantStdout = false
	}

	return p
}

// ShouldWrite returns false if stdout is suppressed (all cloud logs are stdout).
func (p CloudLogParams) ShouldWrite() bool {
	return p.WantStdout
}

// FormatLine prepends detail labels and/or timestamp to a log message.
func (p CloudLogParams) FormatLine(message string, ts time.Time) string {
	line := message
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}

	if p.Timestamps && !ts.IsZero() {
		line = ts.UTC().Format(time.RFC3339Nano) + " " + line
	}

	if p.Details && len(p.Labels) > 0 {
		// Sort label keys for deterministic output
		keys := make([]string, 0, len(p.Labels))
		for k := range p.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, k+"="+p.Labels[k])
		}
		line = strings.Join(parts, ",") + " " + line
	}

	return line
}

// ApplyTail returns the last p.Tail entries from a slice. No-op if Tail == -1.
func (p CloudLogParams) ApplyTail(entries []string) []string {
	if p.Tail < 0 || p.Tail >= len(entries) {
		return entries
	}
	return entries[len(entries)-p.Tail:]
}

// FilterByTime returns true if a log entry at time ts should be included
// based on since/until parameters.
func (p CloudLogParams) FilterByTime(ts time.Time) bool {
	if !p.Since.IsZero() && ts.Before(p.Since) {
		return false
	}
	if !p.Until.IsZero() && !ts.Before(p.Until) {
		return false
	}
	return true
}

// WriteMuxLine writes a Docker multiplexed stream frame (stdout=1) to w.
func WriteMuxLine(w http.ResponseWriter, line string) {
	data := []byte(line)
	header := make([]byte, 8)
	header[0] = 1 // stdout
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	w.Write(header)
	w.Write(data)
}

// FilterBufferedOutput splits buffered bytes into lines, applies since/until/tail
// filtering, formats each line, and writes the filtered output as mux frames.
// Used by FaaS backends (Lambda, GCF, AZF) for LogBuffers output.
func (p CloudLogParams) FilterBufferedOutput(w http.ResponseWriter, buf []byte) {
	if len(buf) == 0 {
		return
	}

	raw := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	now := time.Now().UTC()

	// Apply since/until filtering
	var filtered []string
	for _, line := range raw {
		if line == "" {
			continue
		}
		// Buffered output has no timestamps — use current time for filtering
		if !p.FilterByTime(now) {
			continue
		}
		filtered = append(filtered, line)
	}

	// Apply tail
	filtered = p.ApplyTail(filtered)

	// Format and write each line
	for _, line := range filtered {
		formatted := p.FormatLine(line, now)
		WriteMuxLine(w, formatted)
	}
}

// FlushIfNeeded flushes the http.ResponseWriter if it supports http.Flusher.
func FlushIfNeeded(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// CloudLogTailInt32 returns the Tail value as *int32 for CloudWatch APIs,
// or nil if tail is "all" (-1).
func (p CloudLogParams) CloudLogTailInt32() *int32 {
	if p.Tail < 0 {
		return nil
	}
	n := int32(p.Tail)
	return &n
}

// SinceMillis returns the Since time as milliseconds since epoch for CloudWatch,
// or nil if no since is set.
func (p CloudLogParams) SinceMillis() *int64 {
	if p.Since.IsZero() {
		return nil
	}
	ms := p.Since.UnixMilli()
	return &ms
}

// UntilMillis returns the Until time as milliseconds since epoch for CloudWatch,
// or nil if no until is set.
func (p CloudLogParams) UntilMillis() *int64 {
	if p.Until.IsZero() {
		return nil
	}
	ms := p.Until.UnixMilli()
	return &ms
}

// CloudLoggingSinceFilter returns a Cloud Logging timestamp filter clause
// for the since parameter, or empty string if not set.
func (p CloudLogParams) CloudLoggingSinceFilter() string {
	if p.Since.IsZero() {
		return ""
	}
	return fmt.Sprintf(` AND timestamp>="%s"`, p.Since.UTC().Format(time.RFC3339Nano))
}

// CloudLoggingUntilFilter returns a Cloud Logging timestamp filter clause
// for the until parameter, or empty string if not set.
func (p CloudLogParams) CloudLoggingUntilFilter() string {
	if p.Until.IsZero() {
		return ""
	}
	return fmt.Sprintf(` AND timestamp<"%s"`, p.Until.UTC().Format(time.RFC3339Nano))
}

// KQLSinceFilter returns a KQL where clause for the since parameter,
// or empty string if not set.
func (p CloudLogParams) KQLSinceFilter() string {
	if p.Since.IsZero() {
		return ""
	}
	return fmt.Sprintf(` | where TimeGenerated >= datetime("%s")`, p.Since.UTC().Format(time.RFC3339Nano)) // BUG-568
}

// KQLUntilFilter returns a KQL where clause for the until parameter,
// or empty string if not set.
func (p CloudLogParams) KQLUntilFilter() string {
	if p.Until.IsZero() {
		return ""
	}
	return fmt.Sprintf(` | where TimeGenerated < datetime("%s")`, p.Until.UTC().Format(time.RFC3339Nano)) // BUG-568
}

