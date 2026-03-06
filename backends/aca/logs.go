package aca

import (
	"fmt"
	"io"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	core "github.com/sockerless/backend-core"
)

// fetchAndWriteLogsPipe queries Azure Monitor Log Analytics and writes new entries to a PipeWriter.
// Writes raw text lines (no mux framing — core handler adds it).
// applyTimeFilters controls whether since/until are added (initial=true, follow=false).
func (s *Server) fetchAndWriteLogsPipe(pw *io.PipeWriter, jobName string, after time.Time, params core.CloudLogParams, lastTS *time.Time, applyTimeFilters bool) {
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

	for _, e := range entries {
		formatted := params.FormatLine(e.line, e.ts) // BUG-427: details + timestamps
		if _, err := pw.Write([]byte(formatted)); err != nil {
			return
		}
	}
}
