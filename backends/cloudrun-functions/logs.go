package gcf

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

	timestamps := r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true"

	gcfState, _ := s.GCF.Get(id)
	funcName := gcfState.FunctionName

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// Build filter for Cloud Logging — Cloud Run Functions 2nd gen run as Cloud Run services
	filter := fmt.Sprintf(
		`resource.type="cloud_run_revision" AND resource.labels.service_name="%s"`,
		funcName,
	)

	ctx := s.ctx()
	if s.config.EndpointURL != "" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}

	it := s.gcp.LogAdmin.Entries(ctx, logadmin.Filter(filter))

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

		s.writeLogEvent(w, line, timestamps, entry.Timestamp)
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
