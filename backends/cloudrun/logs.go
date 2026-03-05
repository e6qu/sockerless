package cloudrun

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

	crState, _ := s.CloudRun.Get(id)
	jobName := crState.JobName
	if jobName == "" {
		jobName = buildJobName(id)
	}
	// Extract just the job name from the full resource path
	parts := strings.Split(jobName, "/")
	shortJobName := parts[len(parts)-1]

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// BUG-426: Early return if stdout suppressed
	if !params.ShouldWrite() {
		return
	}

	// Build base filter for Cloud Logging
	baseFilter := fmt.Sprintf(
		`resource.type="cloud_run_job" AND resource.labels.job_name="%s"`,
		shortJobName,
	)

	// Track latest timestamp to avoid duplicate entries
	var lastTimestamp time.Time

	// Fetch initial logs with since/until filtering
	initialFilter := baseFilter
	initialFilter += params.CloudLoggingSinceFilter() // BUG-423
	initialFilter += params.CloudLoggingUntilFilter() // BUG-424
	s.fetchAndWriteLogs(w, initialFilter, lastTimestamp, params, &lastTimestamp)

	if !params.Follow {
		return
	}

	// Follow mode: poll for new events (1s interval for Cloud Logging)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				// BUG-433: Follow queries use lastTimestamp only, no since/until
				s.fetchAndWriteLogs(w, baseFilter, lastTimestamp, params, &lastTimestamp)
				return
			}
			// BUG-433: Follow queries use lastTimestamp only, no since/until
			s.fetchAndWriteLogs(w, baseFilter, lastTimestamp, params, &lastTimestamp)
		}
	}
}

// fetchAndWriteLogs queries Cloud Logging and writes new entries.
func (s *Server) fetchAndWriteLogs(w http.ResponseWriter, filter string, after time.Time, params core.CloudLogParams, lastTS *time.Time) {
	logFilter := filter
	if !after.IsZero() {
		logFilter += fmt.Sprintf(` AND timestamp>"%s"`, after.UTC().Format(time.RFC3339Nano))
	}

	ctx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
	defer cancel()

	it := s.gcp.LogAdmin.Entries(ctx, logadmin.Filter(logFilter))

	// BUG-425: Collect entries first for tail support
	type logEntry struct {
		line string
		ts   time.Time
	}
	var entries []logEntry

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

		if entry.Timestamp.After(*lastTS) {
			*lastTS = entry.Timestamp
		}
	}

	// BUG-425: Apply tail
	if params.Tail >= 0 && params.Tail < len(entries) {
		entries = entries[len(entries)-params.Tail:]
	}

	wrote := false
	for _, e := range entries {
		formatted := params.FormatLine(e.line, e.ts) // BUG-427: details + timestamps
		core.WriteMuxLine(w, formatted)
		wrote = true
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
