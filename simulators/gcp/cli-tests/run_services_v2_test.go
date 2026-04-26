package gcp_cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BUG-833 — CLI-level smoke for the v2 Cloud Run Services routes added
// to the sim in Phase 108. The sim previously only handled v1 Knative
// service paths (`/v1/namespaces/{ns}/services`); this exercises the
// v2 REST surface used by run.NewServicesRESTClient.

func servicesBaseURL() string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s/services", baseURL, project, location)
}

func runServiceURL(name string) string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s/services/%s", baseURL, project, location, name)
}

func TestCloudRunV2Services_CLI_CreateGetDelete(t *testing.T) {
	createBody := `{
		"labels": {"sockerless_managed": "true"},
		"template": {
			"containers": [{"image": "gcr.io/test-project/hello"}],
			"scaling": {"minInstanceCount": 1, "maxInstanceCount": 1}
		}
	}`
	createOut := httpDoJSON(t, "POST", servicesBaseURL()+"?serviceId=cli-svc-roundtrip", createBody)

	var lro struct {
		Done     bool `json:"done"`
		Response struct {
			Name                string            `json:"name"`
			UID                 string            `json:"uid"`
			Generation          string            `json:"generation"`
			Labels              map[string]string `json:"labels"`
			LatestReadyRevision string            `json:"latestReadyRevision"`
			TerminalCondition   struct {
				State string `json:"state"`
			} `json:"terminalCondition"`
		} `json:"response"`
	}
	parseJSON(t, createOut, &lro)
	require.True(t, lro.Done, "CreateService LRO should be done immediately in the sim")
	assert.Contains(t, lro.Response.Name, "cli-svc-roundtrip")
	assert.NotEmpty(t, lro.Response.UID)
	assert.Equal(t, "1", lro.Response.Generation)
	assert.Equal(t, "true", lro.Response.Labels["sockerless_managed"])
	assert.NotEmpty(t, lro.Response.LatestReadyRevision)
	assert.Equal(t, "CONDITION_SUCCEEDED", lro.Response.TerminalCondition.State)

	getOut := httpDoJSON(t, "GET", runServiceURL("cli-svc-roundtrip"), "")
	var got struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	}
	parseJSON(t, getOut, &got)
	assert.Contains(t, got.Name, "cli-svc-roundtrip")
	assert.Equal(t, "true", got.Labels["sockerless_managed"])

	httpDoJSON(t, "DELETE", runServiceURL("cli-svc-roundtrip"), "")

	resp, err := httpDo("GET", runServiceURL("cli-svc-roundtrip"), "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "GetService after delete must 404")
}
