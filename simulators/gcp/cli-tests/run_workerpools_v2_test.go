package gcp_cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cloud Run Worker Pools and Instances from the vendor CLI.
//
// What `gcloud run worker-pools` reaches, and what it does not:
//
//   - The IAM commands (`get-iam-policy`, `set-iam-policy`,
//     `add-iam-policy-binding`, `remove-iam-policy-binding`) are declarative
//     commands bound to the `run.projects.locations.workerpools` collection.
//     They call the Cloud Run Admin v1 IAM triple at
//     `/v1/projects/{p}/locations/{l}/workerpools/{id}:{verb}` on the global
//     endpoint, which addresses the same worker pool the v2 collection does.
//     Those are driven here as real gcloud invocations.
//   - `deploy`, `describe`, `delete`, `update`, `update-instance-split`,
//     `replace` and `list --region` resolve the *regional* Cloud Run host
//     (`{region}-run.googleapis.com`) and speak the Knative surface at
//     `/apis/run.googleapis.com/v1/namespaces/{ns}/workerpools`. gcloud builds
//     that host by prefixing the configured endpoint with `{region}-`, which
//     no endpoint coordinate can point at a loopback simulator, and the
//     simulator does not serve the Knative worker-pool collection. Those
//     commands therefore have no CLI coverage; the v2 collection they would
//     otherwise reach is covered by the SDK tests and the wire round-trips
//     below.
//   - Cloud Run *Instances* have no `gcloud run` command group at any release
//     track (GA, beta or alpha), so the instances collection is exercised
//     through the raw v2 wire here and through the official clients in
//     sdk-tests.

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

// createWorkerPoolForCLI deploys a worker pool over the v2 collection and
// registers its teardown. gcloud's own deploy command cannot reach the
// simulator (see the file comment), so the resource the CLI commands act on is
// created through the same v2 API the Cloud Run backend uses.
func createWorkerPoolForCLI(t *testing.T, id string) {
	t.Helper()
	body := `{"template": {"containers": [{"image": "gcr.io/test-project/` + id + `"}]}}`
	httpDoJSON(t, "POST", workerPoolsBaseURL()+"?workerPoolId="+id, body)
	t.Cleanup(func() {
		resp, err := httpDo("DELETE", workerPoolURL(id), "")
		if err == nil {
			resp.Body.Close()
		}
	})
}

func TestCloudRunWorkerPools_CLI_IAMPolicyLifecycle(t *testing.T) {
	const id = "cli-wp-iam"
	createWorkerPoolForCLI(t, id)

	// A fresh worker pool starts with no bindings.
	out := runCLI(t, gcloudCLI("run", "worker-pools", "get-iam-policy", id,
		"--region="+location, "--format=json"))
	var initial struct {
		Bindings []struct {
			Role    string   `json:"role"`
			Members []string `json:"members"`
		} `json:"bindings"`
		Etag string `json:"etag"`
	}
	parseJSON(t, out, &initial)
	assert.Empty(t, initial.Bindings, "a newly deployed worker pool has no IAM bindings")
	assert.NotEmpty(t, initial.Etag, "getIamPolicy always returns an etag")

	// add-iam-policy-binding reads, mutates and writes the policy back.
	added := runCLI(t, gcloudCLI("run", "worker-pools", "add-iam-policy-binding", id,
		"--region="+location,
		"--member=user:cli@example.com",
		"--role=roles/run.invoker",
		"--format=json"))
	var afterAdd struct {
		Bindings []struct {
			Role    string   `json:"role"`
			Members []string `json:"members"`
		} `json:"bindings"`
	}
	parseJSON(t, added, &afterAdd)
	require.Len(t, afterAdd.Bindings, 1)
	assert.Equal(t, "roles/run.invoker", afterAdd.Bindings[0].Role)
	assert.Equal(t, []string{"user:cli@example.com"}, afterAdd.Bindings[0].Members)

	// The v1 collection gcloud writes through and the v2 collection the SDK
	// reads are two spellings of one resource — the binding is visible on both.
	v2Policy := httpDoJSON(t, "GET", workerPoolURL(id)+":getIamPolicy", "")
	var fromV2 struct {
		Bindings []struct {
			Role    string   `json:"role"`
			Members []string `json:"members"`
		} `json:"bindings"`
	}
	parseJSON(t, v2Policy, &fromV2)
	require.Len(t, fromV2.Bindings, 1, "the v2 collection must see the policy gcloud wrote through v1")
	assert.Equal(t, "roles/run.invoker", fromV2.Bindings[0].Role)

	// remove-iam-policy-binding takes it back off.
	removed := runCLI(t, gcloudCLI("run", "worker-pools", "remove-iam-policy-binding", id,
		"--region="+location,
		"--member=user:cli@example.com",
		"--role=roles/run.invoker",
		"--format=json"))
	var afterRemove struct {
		Bindings []struct {
			Role string `json:"role"`
		} `json:"bindings"`
	}
	parseJSON(t, removed, &afterRemove)
	assert.Empty(t, afterRemove.Bindings, "the only binding is gone after remove")

	// set-iam-policy replaces the whole policy from a file.
	policyPath := filepath.Join(tmpDir, "worker-pool-policy.json")
	policy := map[string]any{
		"bindings": []map[string]any{{
			"role":    "roles/run.developer",
			"members": []string{"serviceAccount:deployer@test-project.iam.gserviceaccount.com"},
		}},
	}
	data, err := json.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(policyPath, data, 0o600))

	set := runCLI(t, gcloudCLI("run", "worker-pools", "set-iam-policy", id, policyPath,
		"--region="+location, "--format=json"))
	var afterSet struct {
		Bindings []struct {
			Role    string   `json:"role"`
			Members []string `json:"members"`
		} `json:"bindings"`
	}
	parseJSON(t, set, &afterSet)
	require.Len(t, afterSet.Bindings, 1)
	assert.Equal(t, "roles/run.developer", afterSet.Bindings[0].Role)
	assert.Equal(t,
		[]string{"serviceAccount:deployer@test-project.iam.gserviceaccount.com"},
		afterSet.Bindings[0].Members)
}

// TestCloudRunWorkerPools_CLI_IAMPolicyOnMissingPool pins the CLI-visible
// error for a worker pool that was never deployed: the IAM verb is NOT_FOUND,
// not an empty policy.
func TestCloudRunWorkerPools_CLI_IAMPolicyOnMissingPool(t *testing.T) {
	cmd := gcloudCLI("run", "worker-pools", "get-iam-policy", "cli-wp-never-deployed",
		"--region="+location, "--format=json")
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "get-iam-policy on an absent worker pool must fail")
	assert.Contains(t, string(out), "NOT_FOUND")
	assert.Contains(t, string(out), "cli-wp-never-deployed")
}

func TestCloudRunV2WorkerPools_Wire_CreateGetListPatchDelete(t *testing.T) {
	createBody := `{
		"template": {"containers": [{"image": "gcr.io/test-project/wp-cli"}]},
		"scaling": {"manualInstanceCount": 3}
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
			Scaling struct {
				ManualInstanceCount int `json:"manualInstanceCount"`
			} `json:"scaling"`
		} `json:"response"`
	}
	parseJSON(t, createOut, &lro)
	require.True(t, lro.Done, "CreateWorkerPool LRO should be done immediately in the sim")
	assert.Contains(t, lro.Response.Name, "cli-wp-roundtrip")
	assert.NotEmpty(t, lro.Response.UID)
	assert.Equal(t, "1", lro.Response.Generation)
	assert.Equal(t, "CONDITION_SUCCEEDED", lro.Response.TerminalCondition.State)
	assert.Equal(t, 3, lro.Response.Scaling.ManualInstanceCount)

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

	patchOut := httpDoJSON(t, "PATCH", workerPoolURL("cli-wp-roundtrip")+"?updateMask=scaling",
		`{"scaling": {"manualInstanceCount": 5}}`)
	var patched struct {
		Response struct {
			Generation string `json:"generation"`
			Scaling    struct {
				ManualInstanceCount int `json:"manualInstanceCount"`
			} `json:"scaling"`
			Template struct {
				Containers []struct {
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"template"`
		} `json:"response"`
	}
	parseJSON(t, patchOut, &patched)
	assert.Equal(t, "2", patched.Response.Generation)
	assert.Equal(t, 5, patched.Response.Scaling.ManualInstanceCount)
	require.Len(t, patched.Response.Template.Containers, 1, "an unmasked field survives the patch")
	assert.Equal(t, "gcr.io/test-project/wp-cli", patched.Response.Template.Containers[0].Image)

	revsOut := httpDoJSON(t, "GET", workerPoolURL("cli-wp-roundtrip")+"/revisions", "")
	var revs struct {
		Revisions []struct {
			Name string `json:"name"`
		} `json:"revisions"`
	}
	parseJSON(t, revsOut, &revs)
	require.GreaterOrEqual(t, len(revs.Revisions), 2, "create + patch each materialize a revision")

	httpDoJSON(t, "DELETE", baseURL+"/v2/"+revs.Revisions[0].Name, "")
	httpDoJSON(t, "DELETE", workerPoolURL("cli-wp-roundtrip"), "")
	resp, err := httpDo("GET", workerPoolURL("cli-wp-roundtrip"), "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "GetWorkerPool after delete must 404")
}

func TestCloudRunV2Instances_Wire_CreatePatchStartStopDelete(t *testing.T) {
	createBody := `{"containers": [{"image": "gcr.io/test-project/inst-cli"}], "labels": {"team": "cli"}}`
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

	listOut := httpDoJSON(t, "GET", instancesBaseURL(), "")
	var list struct {
		Instances []struct {
			Name string `json:"name"`
		} `json:"instances"`
	}
	parseJSON(t, listOut, &list)
	var found bool
	for _, in := range list.Instances {
		if in.Name == lro.Response.Name {
			found = true
		}
	}
	assert.True(t, found, "ListInstances must include the created instance")

	patchOut := httpDoJSON(t, "PATCH", instanceURL("cli-inst-roundtrip")+"?updateMask=containers",
		`{"containers": [{"image": "gcr.io/test-project/inst-cli:v2"}]}`)
	var patched struct {
		Response struct {
			Generation string            `json:"generation"`
			Labels     map[string]string `json:"labels"`
			Containers []struct {
				Image string `json:"image"`
			} `json:"containers"`
		} `json:"response"`
	}
	parseJSON(t, patchOut, &patched)
	assert.Equal(t, "2", patched.Response.Generation)
	require.Len(t, patched.Response.Containers, 1)
	assert.Equal(t, "gcr.io/test-project/inst-cli:v2", patched.Response.Containers[0].Image)
	assert.Equal(t, "cli", patched.Response.Labels["team"], "an unmasked field survives the patch")

	stopOut := httpDoJSON(t, "POST", instanceURL("cli-inst-roundtrip")+":stop", "{}")
	var stopLRO struct {
		Response struct {
			TerminalCondition struct {
				State  string `json:"state"`
				Reason string `json:"reason"`
			} `json:"terminalCondition"`
		} `json:"response"`
	}
	parseJSON(t, stopOut, &stopLRO)
	assert.Equal(t, "CONDITION_PENDING", stopLRO.Response.TerminalCondition.State)
	assert.Equal(t, "Stopped", stopLRO.Response.TerminalCondition.Reason)

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
	assert.Equal(t, "CONDITION_SUCCEEDED", startLRO.Response.TerminalCondition.State)

	httpDoJSON(t, "DELETE", instanceURL("cli-inst-roundtrip"), "")
	resp, err := httpDo("GET", instanceURL("cli-inst-roundtrip"), "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "GetInstance after delete must 404")
}
