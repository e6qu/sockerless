package aca

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
)

func (s *Server) recentACAAppLogTail(id string) string {
	state, ok := s.ACA.Get(id)
	if !ok || state.AppName == "" || s.config.LogAnalyticsWorkspace == "" {
		return ""
	}
	query := fmt.Sprintf(`ContainerAppConsoleLogs_CL | where ContainerAppName_s == "%s" | project TimeGenerated, Log_s`, state.AppName)
	body := azquery.Body{Query: &query}
	resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, body, nil)
	if err != nil {
		return fmt.Sprintf(" Recent ACA app log query failed: %v.", err)
	}

	var lines []string
	for _, tbl := range resp.Tables {
		logIdx := -1
		for i, col := range tbl.Columns {
			if col.Name != nil && *col.Name == "Log_s" {
				logIdx = i
				break
			}
		}
		if logIdx < 0 {
			continue
		}
		for _, row := range tbl.Rows {
			if logIdx >= len(row) {
				continue
			}
			line, ok := row[logIdx].(string)
			if !ok || strings.TrimSpace(line) == "" {
				continue
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return " Recent ACA app logs: " + strings.Join(lines, " | ")
}
