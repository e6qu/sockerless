package gcp_cli_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CLI-level smoke for the v2 Cloud Run Worker Pools + Instances routes — the
// v2 REST surface wrapped by run.NewWorkerPoolsRESTClient /
// run.NewInstancesRESTClient and `gcloud run worker-pools`. Mirrors the
// raw-wire shape the SDK round-trips assert in sdk-tests.

func workerPoolsBaseURL() string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s/workerPools", baseURL, project, location)
}

func workerPoolURL(name string) string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s/workerPools/%s", baseURL, project, location, name)
}

func instancesBaseURL() string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s/instances", baseURL, project, location)
}

func instanceURL(name string) string {
	return fmt.Sprintf("%s/v2/projects/%s/locations/%s/instances/%s", baseURL, project, location, name)
}

func TestCloudRunV2WorkerPools_CLI_CreateGetListDelete(t *testing.T) {
	createBody := `{
		"template": {"containers": [{"image": "gcr.io/test-project/wp-cli"}]}
	}`
	createOut := httpDoJSON(t, "POST", workerPoolsBaseURL()+"?workerPoolId=cli-wp-roundtrip", createBody)
	var lro struct {
		Done     bool `json:"done"`
		Response struct {
			Name              string `json:"name"`
			UID               string `json:"uid"`
			Generation        string `json:"generation"`
			TerminalCondition struct {
				State string `json:"state"`
			} `json:"terminalCondition"`
		} `json:"response"`
	}
	parseJSON(t, createOut, &lro)
	require.True(t, lro.Done, "CreateWorkerPool LRO should be done immediately in the sim")
	assert.Contains(t, lro.Response.Name, "cli-wp-roundtrip")
	assert.NotEmpty(t, lro.Response.UID)
	assert.Equal(t, "1", lro.Response.Generation)
	assert.Equal(t, "CONDITION_SUCCEEDED", lro.Response.TerminalCondition.State)

	getOut := httpDoJSON(t, "GET", workerPoolURL("cli-wp-roundtrip"), "")
	var got struct {
		Name string `json:"name"`
	}
	parseJSON(t, getOut, &got)
	assert.Contains(t, got.Name, "cli-wp-roundtrip")

	listOut := httpDoJSON(t, "GET", workerPoolsBaseURL(), "")
	var list struct {
		WorkerPools []struct {
			Name string `json:"name"`
		} `json:"workerPools"`
	}
	parseJSON(t, listOut, &list)
	var found bool
	for _, p := range list.WorkerPools {
		if p.Name == got.Name {
			found = true
		}
	}
	assert.True(t, found, "ListWorkerPools must include the created pool")

	httpDoJSON(t, "DELETE", workerPoolURL("cli-wp-roundtrip"), "")
	resp, err := httpDo("GET", workerPoolURL("cli-wp-roundtrip"), "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "GetWorkerPool after delete must 404")
}

func TestCloudRunV2Instances_CLI_CreateStartStopDelete(t *testing.T) {
	createBody := `{"containers": [{"image": "gcr.io/test-project/inst-cli"}]}`
	createOut := httpDoJSON(t, "POST", instancesBaseURL()+"?instanceId=cli-inst-roundtrip", createBody)
	var lro struct {
		Done     bool `json:"done"`
		Response struct {
			Name string `json:"name"`
		} `json:"response"`
	}
	parseJSON(t, createOut, &lro)
	require.True(t, lro.Done)
	assert.Contains(t, lro.Response.Name, "cli-inst-roundtrip")

	httpDoJSON(t, "POST", instanceURL("cli-inst-roundtrip")+":stop", "{}")
	startOut := httpDoJSON(t, "POST", instanceURL("cli-inst-roundtrip")+":start", "{}")
	var startLRO struct {
		Response struct {
			TerminalCondition struct {
				Type  string `json:"type"`
				State string `json:"state"`
			} `json:"terminalCondition"`
		} `json:"response"`
	}
	parseJSON(t, startOut, &startLRO)
	assert.Equal(t, "Ready", startLRO.Response.TerminalCondition.Type)

	httpDoJSON(t, "DELETE", instanceURL("cli-inst-roundtrip"), "")
	resp, err := httpDo("GET", instanceURL("cli-inst-roundtrip"), "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "GetInstance after delete must 404")
}
