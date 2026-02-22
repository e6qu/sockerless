package azf

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

	timestamps := r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true"

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

	if s.config.LogAnalyticsWorkspace == "" {
		return
	}

	// Build KQL query for Azure Monitor -- query AppTraces for the function app
	query := fmt.Sprintf(
		`AppTraces | where AppRoleName == "%s"`,
		functionAppName,
	)
	query += ` | order by TimeGenerated asc | project TimeGenerated, Message`

	resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
		Query: &query,
	}, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to query logs")
		return
	}

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

			s.writeLogEvent(w, line, timestamps, ts)
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
