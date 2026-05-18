package gcp_sdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactRegistryDockerHubRemoteOCIManifest(t *testing.T) {
	imageName := "test-project/docker-hub/sockerless-eval-arithmetic"
	manifestURL := baseURL + "/v2/" + imageName + "/manifests/test"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	require.NoError(t, err)
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}
	require.NoError(t, json.Unmarshal(body, &manifest))
	require.NotEmpty(t, manifest.Config.Digest)
	require.NotEmpty(t, manifest.Layers)

	blobURL := baseURL + "/v2/" + imageName + "/blobs/" + manifest.Config.Digest
	blobResp, err := http.Get(blobURL)
	require.NoError(t, err)
	defer blobResp.Body.Close()
	require.Equal(t, http.StatusOK, blobResp.StatusCode)

	var cfg struct {
		Config struct {
			Entrypoint []string `json:"Entrypoint"`
		} `json:"config"`
	}
	require.NoError(t, json.NewDecoder(blobResp.Body).Decode(&cfg))
	require.Equal(t, []string{"/usr/local/bin/eval-arithmetic"}, cfg.Config.Entrypoint)
}
