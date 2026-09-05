package aca

import (
	azurecommon "github.com/sockerless/azure-common"

	core "github.com/sockerless/backend-core"
)

// azureLogsFetch reads a workload's lines from the configured Log
// Analytics workspace with a Kusto query over table, filtered by
// whereClause, projecting messageColumn.
func (s *Server) azureLogsFetch(table, whereClause, messageColumn string) core.CloudLogFetchFunc {
	return azurecommon.LogAnalyticsFetch(s.azure.Logs, s.config.LogAnalyticsWorkspace, table, whereClause, messageColumn)
}
