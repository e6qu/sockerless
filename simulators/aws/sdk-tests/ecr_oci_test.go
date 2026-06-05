package aws_sdk_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ociDigest computes the registry digest of content.
func ociDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ociDo(t *testing.T, method, url string, body []byte, contentType string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestECR_OCIDataPlane covers issue #450: the Docker Registry /v2/ data plane —
// a full chunked blob push + manifest push, then pull, against an ECR
// multi-segment repository.
func TestECR_OCIDataPlane(t *testing.T) {
	repo := "oci-test-repo/app"

	// Base / version route.
	resp := ociDo(t, http.MethodGet, baseURL+"/v2/", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "registry/2.0", resp.Header.Get("Docker-Distribution-API-Version"))
	resp.Body.Close()

	// Chunked blob upload: POST (init) → PATCH (chunk) → PUT (finalize).
	layer := []byte("the-image-layer-bytes-go-here")
	digest := ociDigest(layer)

	resp = ociDo(t, http.MethodPost, baseURL+"/v2/"+repo+"/blobs/uploads/", nil, "")
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	loc := resp.Header.Get("Location")
	require.NotEmpty(t, loc)
	require.NotEmpty(t, resp.Header.Get("Docker-Upload-UUID"))
	resp.Body.Close()

	resp = ociDo(t, http.MethodPatch, baseURL+loc, layer, "application/octet-stream")
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	resp = ociDo(t, http.MethodPut, baseURL+loc+"?digest="+digest, nil, "")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, digest, resp.Header.Get("Docker-Content-Digest"))
	resp.Body.Close()

	// Blob is now present.
	resp = ociDo(t, http.MethodHead, baseURL+"/v2/"+repo+"/blobs/"+digest, nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Push a manifest referencing the layer.
	manifest := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":7,"digest":"%s"},"layers":[{"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":%d,"digest":"%s"}]}`, digest, len(layer), digest))
	resp = ociDo(t, http.MethodPut, baseURL+"/v2/"+repo+"/manifests/v1", manifest, "application/vnd.docker.distribution.manifest.v2+json")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	manifestDigest := resp.Header.Get("Docker-Content-Digest")
	assert.Equal(t, ociDigest(manifest), manifestDigest)
	resp.Body.Close()

	// Pull it back by tag.
	resp = ociDo(t, http.MethodGet, baseURL+"/v2/"+repo+"/manifests/v1", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, manifest, got)

	// And by digest.
	resp = ociDo(t, http.MethodGet, baseURL+"/v2/"+repo+"/manifests/"+manifestDigest, nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Pull the layer blob back.
	resp = ociDo(t, http.MethodGet, baseURL+"/v2/"+repo+"/blobs/"+digest, nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	gotLayer, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, layer, gotLayer)

	// A digest that doesn't match the content is rejected.
	resp = ociDo(t, http.MethodPost, baseURL+"/v2/"+repo+"/blobs/uploads/", nil, "")
	bad := resp.Header.Get("Location")
	resp.Body.Close()
	resp = ociDo(t, http.MethodPatch, baseURL+bad, layer, "application/octet-stream")
	resp.Body.Close()
	resp = ociDo(t, http.MethodPut, baseURL+bad+"?digest=sha256:0000000000000000000000000000000000000000000000000000000000000000", nil, "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}
