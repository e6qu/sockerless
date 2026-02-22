package cloudrun

import (
	"context"
	"encoding/binary"
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

	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"
	timestamps := r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true"

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

	// Build filter for Cloud Logging
	filter := fmt.Sprintf(
		`resource.type="cloud_run_job" AND resource.labels.job_name="%s"`,
		shortJobName,
	)

	// Track latest timestamp to avoid duplicate entries
	var lastTimestamp time.Time

	// Fetch initial logs
	s.fetchAndWriteLogs(w, filter, lastTimestamp, timestamps, &lastTimestamp)

	if !follow {
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
				// Fetch any remaining logs
				s.fetchAndWriteLogs(w, filter, lastTimestamp, timestamps, &lastTimestamp)
				return
			}
			s.fetchAndWriteLogs(w, filter, lastTimestamp, timestamps, &lastTimestamp)
		}
	}
}

// fetchAndWriteLogs queries Cloud Logging and writes new entries.
func (s *Server) fetchAndWriteLogs(w http.ResponseWriter, filter string, after time.Time, addTimestamp bool, lastTS *time.Time) {
	logFilter := filter
	if !after.IsZero() {
		logFilter += fmt.Sprintf(` AND timestamp>"%s"`, after.UTC().Format(time.RFC3339Nano))
	}

	ctx := s.ctx()
	if s.config.EndpointURL != "" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}

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

		s.writeLogEvent(w, line, addTimestamp, entry.Timestamp)
		wrote = true

		if entry.Timestamp.After(*lastTS) {
			*lastTS = entry.Timestamp
		}
	}

	if wrote {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
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

func (s *Server) writeLogEvent(w http.ResponseWriter, line string, addTimestamp bool, ts time.Time) {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}

	if addTimestamp {
		line = ts.UTC().Format(time.RFC3339Nano) + " " + line
	}

	data := []byte(line)
	header := make([]byte, 8)
	header[0] = 1 // stdout
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	w.Write(header)
	w.Write(data)
}
