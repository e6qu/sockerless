package aca

import (
	"fmt"
	"net/http"
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

	params := core.ParseCloudLogParams(r, c.Config.Labels)

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

	// BUG-426: Early return if stdout suppressed
	if !params.ShouldWrite() {
		return
	}

	// Track latest timestamp to avoid duplicate entries
	var lastTimestamp time.Time

	// Fetch initial logs with since/until filtering
	s.fetchAndWriteLogs(w, jobName, lastTimestamp, params, &lastTimestamp, true)

	if !params.Follow {
		return
	}

	// BUG-434: Follow mode poll interval 1s (was 2s, inconsistent with ECS/CloudRun)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				// BUG-433: Follow queries don't use since/until
				s.fetchAndWriteLogs(w, jobName, lastTimestamp, params, &lastTimestamp, false)
				return
			}
			// BUG-433: Follow queries don't use since/until
			s.fetchAndWriteLogs(w, jobName, lastTimestamp, params, &lastTimestamp, false)
		}
	}
}

// fetchAndWriteLogs queries Azure Monitor Log Analytics and writes new entries.
// applyTimeFilters controls whether since/until are added (initial=true, follow=false).
func (s *Server) fetchAndWriteLogs(w http.ResponseWriter, jobName string, after time.Time, params core.CloudLogParams, lastTS *time.Time, applyTimeFilters bool) {
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
	// BUG-423, BUG-424: Apply since/until only on initial fetch
	if applyTimeFilters {
		query += params.KQLSinceFilter()
		query += params.KQLUntilFilter()
	}
	query += ` | order by TimeGenerated asc | project TimeGenerated, Log_s`

	resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
		Query: &query,
	}, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to query logs")
		return
	}

	type logEntry struct {
		line string
		ts   time.Time
	}
	var entries []logEntry

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

			entries = append(entries, logEntry{line: line, ts: ts})

			if !ts.IsZero() && ts.After(*lastTS) {
				*lastTS = ts
			}
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
