package aca

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
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

	acaState, _ := s.ACA.Get(id)
	jobName := acaState.JobName
	if jobName == "" {
		jobName = buildJobName(id)
	}

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)

	if f, ok := w.(http.Flusher); ok {
		defer f.Flush()
	}

	// Track latest timestamp to avoid duplicate entries
	var lastTimestamp time.Time

	// Fetch initial logs
	s.fetchAndWriteLogs(w, jobName, lastTimestamp, timestamps, &lastTimestamp)

	if !follow {
		return
	}

	// Follow mode: poll for new events (2s interval for Azure Monitor)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				s.fetchAndWriteLogs(w, jobName, lastTimestamp, timestamps, &lastTimestamp)
				return
			}
			s.fetchAndWriteLogs(w, jobName, lastTimestamp, timestamps, &lastTimestamp)
		}
	}
}

// fetchAndWriteLogs queries Azure Monitor Log Analytics and writes new entries.
func (s *Server) fetchAndWriteLogs(w http.ResponseWriter, jobName string, after time.Time, addTimestamp bool, lastTS *time.Time) {
	if s.config.LogAnalyticsWorkspace == "" {
		return
	}

	query := fmt.Sprintf(
		`ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "%s"`,
		jobName,
	)
	if !after.IsZero() {
		query += fmt.Sprintf(` | where TimeGenerated > datetime("%s")`, after.UTC().Format(time.RFC3339Nano))
	}
	query += ` | order by TimeGenerated asc | project TimeGenerated, Log_s`

	resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
		Query: &query,
	}, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to query logs")
		return
	}

	wrote := false
	for _, table := range resp.Tables {
		timeIdx := -1
		logIdx := -1
		for i, col := range table.Columns {
			if col.Name != nil {
				switch *col.Name {
				case "TimeGenerated":
					timeIdx = i
				case "Log_s":
					logIdx = i
				}
			}
		}

		for _, row := range table.Rows {
			if logIdx < 0 || logIdx >= len(row) {
				continue
			}

			line, ok := row[logIdx].(string)
			if !ok || line == "" {
				continue
			}

			var ts time.Time
			if timeIdx >= 0 && timeIdx < len(row) {
				if tsStr, ok := row[timeIdx].(string); ok {
					ts, _ = time.Parse(time.RFC3339Nano, tsStr)
				}
			}

			s.writeLogEvent(w, line, addTimestamp, ts)
			wrote = true

			if !ts.IsZero() && ts.After(*lastTS) {
				*lastTS = ts
			}
		}
	}

	if wrote {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
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
