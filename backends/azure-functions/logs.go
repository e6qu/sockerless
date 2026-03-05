package azf

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

	azfState, _ := s.AZF.Get(id)
	functionAppName := azfState.FunctionAppName
	if functionAppName == "" {
		functionAppName = "skls-" + id[:12]
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

	// BUG-435: Filter LogBuffers output through params
	if buf, ok := s.Store.LogBuffers.Load(id); ok {
		data := buf.([]byte)
		if len(data) > 0 {
			params.FilterBufferedOutput(w, data)
		}
	}

	if s.config.LogAnalyticsWorkspace == "" {
		return
	}

	// Build KQL query for Azure Monitor -- query AppTraces for the function app
	query := fmt.Sprintf(
		`AppTraces | where AppRoleName == "%s"`,
		functionAppName,
	)
	// BUG-423, BUG-424: Apply since/until to initial query
	query += params.KQLSinceFilter()
	query += params.KQLUntilFilter()
	query += ` | order by TimeGenerated asc | project TimeGenerated, Message`

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
		// Find column indices
		timeIdx := -1
		msgIdx := -1
		for i, col := range table.Columns {
			if col.Name != nil {
				switch *col.Name {
				case "TimeGenerated":
					timeIdx = i
				case "Message":
					msgIdx = i
				}
			}
		}

		for _, row := range table.Rows {
			if msgIdx < 0 || msgIdx >= len(row) {
				continue
			}

			line, ok := row[msgIdx].(string)
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

	// BUG-430: Follow mode support
	if !params.Follow {
		return
	}

	// Track last timestamp for follow dedup
	var lastTimestamp time.Time
	if len(entries) > 0 {
		lastTimestamp = entries[len(entries)-1].ts
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
				s.fetchFollowLogs(w, functionAppName, lastTimestamp, params, &lastTimestamp)
				return
			}
			s.fetchFollowLogs(w, functionAppName, lastTimestamp, params, &lastTimestamp)
		}
	}
}

// fetchFollowLogs queries Azure Monitor for entries after lastTS.
func (s *Server) fetchFollowLogs(w http.ResponseWriter, functionAppName string, after time.Time, params core.CloudLogParams, lastTS *time.Time) {
	query := fmt.Sprintf(
		`AppTraces | where AppRoleName == "%s"`,
		functionAppName,
	)
	if !after.IsZero() {
		query += fmt.Sprintf(` | where TimeGenerated > datetime(%s)`, after.UTC().Format(time.RFC3339Nano))
	}
	query += ` | order by TimeGenerated asc | project TimeGenerated, Message`

	resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
		Query: &query,
	}, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to query follow logs")
		return
	}

	wrote := false
	for _, table := range resp.Tables {
		timeIdx := -1
		msgIdx := -1
		for i, col := range table.Columns {
			if col.Name != nil {
				switch *col.Name {
				case "TimeGenerated":
					timeIdx = i
				case "Message":
					msgIdx = i
				}
			}
		}

		for _, row := range table.Rows {
			if msgIdx < 0 || msgIdx >= len(row) {
				continue
			}

			line, ok := row[msgIdx].(string)
			if !ok || line == "" {
				continue
			}

			var ts time.Time
			if timeIdx >= 0 && timeIdx < len(row) {
				if tsStr, ok := row[timeIdx].(string); ok {
					ts, _ = time.Parse(time.RFC3339Nano, tsStr)
				}
			}

			formatted := params.FormatLine(line, ts)
			core.WriteMuxLine(w, formatted)
			wrote = true

			if !ts.IsZero() && ts.After(*lastTS) {
				*lastTS = ts
			}
		}
	}

	if wrote {
		core.FlushIfNeeded(w)
	}
}
