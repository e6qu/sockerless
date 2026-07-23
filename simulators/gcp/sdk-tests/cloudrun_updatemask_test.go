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

// TestCloudRun_UpdateServiceHonorsUpdateMask pins Cloud Run v2 UpdateService
// field-mask semantics: a PATCH with updateMask=labels updates only the labels
// and preserves every unmasked field (e.g. the template/image), even when the
// request body carries a different value for them. Before the fix the handler
// did a wholesale replace, dropping unmasked fields → terraform drift.
func TestCloudRun_UpdateServiceHonorsUpdateMask(t *testing.T) {
	parent := "projects/test-project/locations/us-central1"
	base := baseURL + "/v2/" + parent + "/services"
	sid := "mask-svc"

	doJSON := func(method, url, body string) (*http.Response, []byte) {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, url, r)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", simBearerHeader(t))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, b
	}

	resp, body := doJSON("POST", base+"?serviceId="+sid,
		`{"template":{"containers":[{"image":"img-a"}]},"labels":{"k":"v1"}}`)
	require.Less(t, resp.StatusCode, 300, "create: %d %s", resp.StatusCode, body)

	svcURL := base + "/" + sid
	// updateMask=labels: change the label, and send a DIFFERENT image that must
	// be ignored because template is not in the mask.
	resp, body = doJSON("PATCH", svcURL+"?updateMask=labels",
		`{"labels":{"k":"v2"},"template":{"containers":[{"image":"img-b"}]}}`)
	require.Less(t, resp.StatusCode, 300, "patch: %d %s", resp.StatusCode, body)

	resp, body = doJSON("GET", svcURL, "")
	require.Equal(t, 200, resp.StatusCode, "get: %s", body)
	var svc struct {
		Labels   map[string]string `json:"labels"`
		Template struct {
			Containers []struct {
				Image string `json:"image"`
			} `json:"containers"`
		} `json:"template"`
	}
	require.NoError(t, json.Unmarshal(body, &svc))
	assert.Equal(t, "v2", svc.Labels["k"], "masked field (labels) was updated")
	require.NotEmpty(t, svc.Template.Containers)
	assert.Equal(t, "img-a", svc.Template.Containers[0].Image,
		"unmasked field (template) preserved despite img-b in the request body")
}

// TestCloudRun_UpdateServiceSubPathMask verifies that a sub-path
// updateMask (e.g. "template.containers") merges only that leaf into
// the template, preserving other template sub-fields like volumes and
// scaling. Before the fix the handler swapped the whole template when
// any "template.*" mask path appeared.
func TestCloudRun_UpdateServiceSubPathMask(t *testing.T) {
	parent := "projects/test-project/locations/us-central1"
	base := baseURL + "/v2/" + parent + "/services"
	sid := "subpath-mask-svc"

	doJSON := func(method, url, body string) (*http.Response, []byte) {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, url, r)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", simBearerHeader(t))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, b
	}

	// Create with containers + volumes + scaling.
	resp, body := doJSON("POST", base+"?serviceId="+sid,
		`{"template":{"containers":[{"image":"img-a","name":"c1"}],"volumes":[{"name":"data","emptyDir":{}}],"scaling":{"minInstanceCount":1,"maxInstanceCount":3}}}`)
	require.Less(t, resp.StatusCode, 300, "create: %d %s", resp.StatusCode, body)

	svcURL := base + "/" + sid

	// PATCH with updateMask=template.containers — only containers change.
	resp, body = doJSON("PATCH", svcURL+"?updateMask=template.containers",
		`{"template":{"containers":[{"image":"img-b","name":"c1"}]}}`)
	require.Less(t, resp.StatusCode, 300, "patch: %d %s", resp.StatusCode, body)

	resp, body = doJSON("GET", svcURL, "")
	require.Equal(t, 200, resp.StatusCode, "get: %s", body)
	var svc struct {
		Template struct {
			Containers []struct {
				Image string `json:"image"`
			} `json:"containers"`
			Volumes []struct {
				Name string `json:"name"`
			} `json:"volumes"`
			Scaling struct {
				MinInstanceCount int `json:"minInstanceCount"`
				MaxInstanceCount int `json:"maxInstanceCount"`
			} `json:"scaling"`
		} `json:"template"`
	}
	require.NoError(t, json.Unmarshal(body, &svc))

	// Containers updated to img-b.
	require.NotEmpty(t, svc.Template.Containers)
	assert.Equal(t, "img-b", svc.Template.Containers[0].Image,
		"masked sub-path template.containers must be updated")

	// Volumes preserved.
	require.NotEmpty(t, svc.Template.Volumes, "unmasked template.volumes must be preserved")
	assert.Equal(t, "data", svc.Template.Volumes[0].Name)

	// Scaling preserved.
	assert.Equal(t, 3, svc.Template.Scaling.MaxInstanceCount,
		"unmasked template.scaling must be preserved")
}
