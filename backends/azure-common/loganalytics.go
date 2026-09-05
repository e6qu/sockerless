package azurecommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	core "github.com/sockerless/backend-core"
)

// LogsQuerier runs a Kusto query against a Log Analytics workspace. It is
// the one method of azquery.LogsClient the backends use, so the client
// that reaches the workspace over plain HTTP can stand in for it.
type LogsQuerier interface {
	QueryWorkspace(ctx context.Context, workspaceID string, body azquery.Body, options *azquery.LogsClientQueryWorkspaceOptions) (azquery.LogsClientQueryWorkspaceResponse, error)
}

// logAnalyticsScope is the Microsoft Entra scope a Log Analytics query
// token is requested for.
const logAnalyticsScope = "https://api.loganalytics.io/.default"

// NewLogsQuerier returns the Log Analytics client for the given
// coordinates. The Azure SDK's bearer-token policy refuses to send a
// credential over plain HTTP regardless of InsecureAllowCredentialWithHTTP,
// so an `http://` endpoint is served by a client that requests the same
// Microsoft Entra token for the Log Analytics scope and sends it itself;
// every other endpoint, including the real service, is the SDK client.
// Both send the same credential to the same path; only the transport
// differs.
func NewLogsQuerier(cred azcore.TokenCredential, opts *azcore.ClientOptions, endpointURL string) (LogsQuerier, error) {
	if strings.HasPrefix(endpointURL, "http://") {
		return &bearerLogsClient{endpoint: strings.TrimRight(endpointURL, "/"), cred: cred}, nil
	}
	var lo *azquery.LogsClientOptions
	if opts != nil {
		lo = &azquery.LogsClientOptions{ClientOptions: *opts}
	}
	return azquery.NewLogsClient(cred, lo)
}

// bearerLogsClient posts to `{endpoint}/v1/workspaces/{id}/query`, the
// path the SDK builds, with a Microsoft Entra token for the Log Analytics
// scope.
type bearerLogsClient struct {
	endpoint string
	cred     azcore.TokenCredential
}

func (c *bearerLogsClient) QueryWorkspace(ctx context.Context, workspaceID string, body azquery.Body, _ *azquery.LogsClientQueryWorkspaceOptions) (azquery.LogsClientQueryWorkspaceResponse, error) {
	token, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{logAnalyticsScope}})
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, fmt.Errorf("acquire Log Analytics token: %w", err)
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/workspaces/%s/query", c.endpoint, workspaceID), bytes.NewReader(reqBody))
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return azquery.LogsClientQueryWorkspaceResponse{}, fmt.Errorf("log analytics query returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var result azquery.LogsClientQueryWorkspaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result.Results); err != nil {
		return azquery.LogsClientQueryWorkspaceResponse{}, fmt.Errorf("decode Log Analytics response: %w", err)
	}
	return result, nil
}

// LogAnalyticsFetch returns a CloudLogFetchFunc that reads a workload's
// log lines from an Azure Monitor Log Analytics workspace with a Kusto
// query over `table`, filtered by `where`, projecting TimeGenerated and
// `messageColumn`. The cursor is the latest TimeGenerated seen, so a
// follow reads only newer rows. An empty workspace yields no lines.
func LogAnalyticsFetch(client LogsQuerier, workspace, table, where, messageColumn string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		if workspace == "" {
			return nil, cursor, nil
		}
		var lastTS time.Time
		if cursor != nil {
			ts, ok := cursor.(time.Time)
			if !ok {
				return nil, cursor, fmt.Errorf("azure logs cursor held unexpected type %T", cursor)
			}
			lastTS = ts
		}

		query := fmt.Sprintf(`%s | where %s`, table, where)
		if !lastTS.IsZero() {
			query += fmt.Sprintf(` | where TimeGenerated > datetime("%s")`, lastTS.UTC().Format(time.RFC3339Nano))
		} else {
			query += params.KQLSinceFilter()
			query += params.KQLUntilFilter()
		}
		query += fmt.Sprintf(` | order by TimeGenerated asc | project TimeGenerated, %s`, messageColumn)

		resp, err := client.QueryWorkspace(ctx, workspace, azquery.Body{Query: &query}, nil)
		if err != nil {
			return nil, lastTS, err
		}

		var entries []core.CloudLogEntry
		for _, tbl := range resp.Tables {
			timeIdx, msgIdx := -1, -1
			for i, col := range tbl.Columns {
				if col.Name == nil {
					continue
				}
				switch *col.Name {
				case "TimeGenerated":
					timeIdx = i
				case messageColumn:
					msgIdx = i
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
