package gcf

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Status == "created" {
		core.WriteError(w, &api.InvalidParameterError{
			Message: "can not get logs from container which is dead or marked for removal",
		})
		return
	}

	params := core.ParseCloudLogParams(r, c.Config.Labels)

	gcfState, _ := s.GCF.Get(id)
	funcName := gcfState.FunctionName

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// BUG-426: Early return if stdout suppressed
	if !params.ShouldWrite() {
		return
	}

	// BUG-435: Filter LogBuffers output through params
	if buf, ok := s.Store.LogBuffers.Load(id); ok {
		data := buf.([]byte)
		if len(data) > 0 {
			params.FilterBufferedOutput(w, data)
		}
	}

	// Build filter for Cloud Logging — Cloud Run Functions 2nd gen run as Cloud Run services
	baseFilter := fmt.Sprintf(
		`resource.type="cloud_run_revision" AND resource.labels.service_name="%s"`,
		funcName,
	)

	// BUG-423, BUG-424: Apply since/until to initial query
	initialFilter := baseFilter
	initialFilter += params.CloudLoggingSinceFilter()
	initialFilter += params.CloudLoggingUntilFilter()

	ctx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
	defer cancel()

	it := s.gcp.LogAdmin.Entries(ctx, logadmin.Filter(initialFilter))

	// BUG-425: Collect entries for tail support
	type logEntry struct {
		line string
		ts   time.Time
	}
	var entries []logEntry
	var lastTimestamp time.Time

	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.Logger.Debug().Err(err).Msg("failed to read log entry")
			break
		}

		line := extractLogLine(entry)
		if line == "" {
			continue
		}

		entries = append(entries, logEntry{line: line, ts: entry.Timestamp})

		if entry.Timestamp.After(lastTimestamp) {
			lastTimestamp = entry.Timestamp
		}
	}

	// BUG-425: Apply tail
	if params.Tail >= 0 && params.Tail < len(entries) {
		entries = entries[len(entries)-params.Tail:]
	}

	for _, e := range entries {
		formatted := params.FormatLine(e.line, e.ts) // BUG-427: details + timestamps
		core.WriteMuxLine(w, formatted)
	}
	core.FlushIfNeeded(w)

	// BUG-429: Follow mode support
	if !params.Follow {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				s.fetchFollowLogs(w, baseFilter, lastTimestamp, params, &lastTimestamp)
				return
			}
			s.fetchFollowLogs(w, baseFilter, lastTimestamp, params, &lastTimestamp)
		}
	}
}

// fetchFollowLogs queries Cloud Logging for entries after lastTS.
func (s *Server) fetchFollowLogs(w http.ResponseWriter, baseFilter string, after time.Time, params core.CloudLogParams, lastTS *time.Time) {
	logFilter := baseFilter
	if !after.IsZero() {
		logFilter += fmt.Sprintf(` AND timestamp>"%s"`, after.UTC().Format(time.RFC3339Nano))
	}

	ctx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
	defer cancel()

	it := s.gcp.LogAdmin.Entries(ctx, logadmin.Filter(logFilter))

	wrote := false
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.Logger.Debug().Err(err).Msg("failed to read log entry")
			break
		}

		line := extractLogLine(entry)
		if line == "" {
			continue
		}

		formatted := params.FormatLine(line, entry.Timestamp)
		core.WriteMuxLine(w, formatted)
		wrote = true

		if entry.Timestamp.After(*lastTS) {
			*lastTS = entry.Timestamp
		}
	}

	if wrote {
		core.FlushIfNeeded(w)
	}
}

// extractLogLine gets the text content from a Cloud Logging entry.
func extractLogLine(entry *logging.Entry) string {
	if entry.Payload == nil {
		return ""
	}
	switch p := entry.Payload.(type) {
	case string:
		return p
	default:
		return fmt.Sprintf("%v", p)
	}
}
