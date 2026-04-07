package aca

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	core "github.com/sockerless/backend-core"
)

// azureLogsFetch returns a CloudLogFetchFunc that queries Azure Monitor KQL.
// cursor is a time.Time tracking the latest seen timestamp for dedup.
func (s *Server) azureLogsFetch(table, whereClause, messageColumn string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		if s.config.LogAnalyticsWorkspace == "" {
			return nil, cursor, nil
		}

		var lastTS time.Time
		if cursor != nil {
			lastTS = cursor.(time.Time)
		}

		query := fmt.Sprintf(`%s | where %s`, table, whereClause)
		if !lastTS.IsZero() {
			query += fmt.Sprintf(` | where TimeGenerated > datetime("%s")`, lastTS.UTC().Format(time.RFC3339Nano))
		} else {
			query += params.KQLSinceFilter()
			query += params.KQLUntilFilter()
		}
		query += fmt.Sprintf(` | order by TimeGenerated asc | project TimeGenerated, %s`, messageColumn)

		var resp azquery.LogsClientQueryWorkspaceResponse
		var err error
		if s.azure.LogsHTTP != nil {
			resp, err = s.azure.LogsHTTP.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
				Query: &query,
			}, nil)
		} else {
			resp, err = s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
				Query: &query,
			}, nil)
		}
		if err != nil {
			return nil, lastTS, err
		}

		var entries []core.CloudLogEntry
		for _, tbl := range resp.Tables {
			timeIdx, msgIdx := -1, -1
			for i, col := range tbl.Columns {
				if col.Name != nil {
					switch *col.Name {
					case "TimeGenerated":
						timeIdx = i
					case messageColumn:
						msgIdx = i
					}
				}
			}

			for _, row := range tbl.Rows {
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
				entries = append(entries, core.CloudLogEntry{Timestamp: ts, Message: line})
				if !ts.IsZero() && ts.After(lastTS) {
					lastTS = ts
				}
			}
		}

		return entries, lastTS, nil
	}
}
