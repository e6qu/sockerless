package gcp_sdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cloud Run v1 services SDK tests (Knative-style API). Closes the GCP
// parity gap per plan item B.1a — sockerless doesn't use this API on
// the runner path today, but the simulator ships the slice so the
// parity doc has zero ✖ rows. Uses direct REST rather than the
// google.golang.org/api/run/v1 client because the v1 client wraps the
// service resource in unwieldy proto types; REST is cleaner for
// verifying wire-level behavior.

func TestCloudRunServices_CreateGetListDelete(t *testing.T) {
	namespace := "test-project"

	// Create.
	createBody := `{
		"apiVersion":"serving.knative.dev/v1",
		"kind":"Service",
		"metadata":{"name":"svc-roundtrip","labels":{"env":"test"}},
		"spec":{"template":{"spec":{"containers":[{"image":"gcr.io/x/hello","env":[{"name":"FOO","value":"bar"}]}]}}}
	}`
	createResp := doKnativePOST(t, "/v1/namespaces/"+namespace+"/services", createBody)

	var created struct {
		Metadata CRServiceMetaResp `json:"metadata"`
		Status   struct {
			URL        string                          `json:"url"`
			Conditions []struct{ Type, Status string } `json:"conditions"`
		} `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(createResp), &created))
	assert.Equal(t, "svc-roundtrip", created.Metadata.Name)
	assert.Equal(t, namespace, created.Metadata.Namespace)
	assert.NotEmpty(t, created.Metadata.UID)
	assert.NotEmpty(t, created.Status.URL)
	require.NotEmpty(t, created.Status.Conditions)
	assert.Equal(t, "Ready", created.Status.Conditions[0].Type)
	assert.Equal(t, "True", created.Status.Conditions[0].Status)

	// Get.
	getResp := doKnativeGET(t, "/v1/namespaces/"+namespace+"/services/svc-roundtrip")
	var got struct{ Metadata CRServiceMetaResp }
	require.NoError(t, json.Unmarshal([]byte(getResp), &got))
	assert.Equal(t, "svc-roundtrip", got.Metadata.Name)

	// List — should contain the service we created.
	listResp := doKnativeGET(t, "/v1/namespaces/"+namespace+"/services")
	var list struct {
		Items []struct{ Metadata CRServiceMetaResp } `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(listResp), &list))
	names := []string{}
	for _, s := range list.Items {
		names = append(names, s.Metadata.Name)
	}
	assert.Contains(t, names, "svc-roundtrip")

	// Delete.
	doKnativeDELETE(t, "/v1/namespaces/"+namespace+"/services/svc-roundtrip")

	// Verify gone.
	req, _ := http.NewRequest("GET", baseURL+"/v1/namespaces/"+namespace+"/services/svc-roundtrip", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCloudRunServices_ReplaceBumpsGeneration(t *testing.T) {
	namespace := "test-project-replace"

	createBody := `{
		"metadata":{"name":"svc-rev"},
		"spec":{"template":{"spec":{"containers":[{"image":"gcr.io/x/v1"}]}}}
	}`
	doKnativePOST(t, "/v1/namespaces/"+namespace+"/services", createBody)

	replaceBody := `{
		"metadata":{"name":"svc-rev","labels":{"version":"v2"}},
		"spec":{"template":{"spec":{"containers":[{"image":"gcr.io/x/v2"}]}}}
	}`
	updResp := doKnativePUT(t, "/v1/namespaces/"+namespace+"/services/svc-rev", replaceBody)

	var updated struct {
		Metadata CRServiceMetaResp `json:"metadata"`
		Status   struct {
			LatestReadyRevisionName string `json:"latestReadyRevisionName"`
		} `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(updResp), &updated))
	assert.Equal(t, int64(2), updated.Metadata.Generation, "replace should bump generation")
	assert.Equal(t, "svc-rev-00002", updated.Status.LatestReadyRevisionName,
		"each revision should get a new name")
	assert.Equal(t, "v2", updated.Metadata.Labels["version"])

	// Cleanup.
	doKnativeDELETE(t, "/v1/namespaces/"+namespace+"/services/svc-rev")
}

// ---- helpers ----

type CRServiceMetaResp struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	UID        string            `json:"uid"`
	Generation int64             `json:"generation"`
	Labels     map[string]string `json:"labels"`
}

func doKnativePOST(t *testing.T, path, body string) string {
	t.Helper()
	return doKnative(t, "POST", path, body)
}

func doKnativeGET(t *testing.T, path string) string {
	t.Helper()
	return doKnative(t, "GET", path, "")
}

func doKnativePUT(t *testing.T, path, body string) string {
	t.Helper()
	return doKnative(t, "PUT", path, body)
}

func doKnativeDELETE(t *testing.T, path string) {
	t.Helper()
	_ = doKnative(t, "DELETE", path, "")
}

func doKnative(t *testing.T, method, path, body string) string {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, baseURL+path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	require.Less(t, resp.StatusCode, 400, "%s %s failed: %d %s", method, path, resp.StatusCode, string(data))
	return string(data)
}
