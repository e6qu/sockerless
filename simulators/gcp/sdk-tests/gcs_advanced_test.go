package gcp_sdk_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gcsRESTCreate creates a bucket via raw REST (used by the advanced
// tests that need to drive multipart / resumable / compose directly).
func gcsRESTCreate(t *testing.T, bucket string) {
	t.Helper()
	body := strings.NewReader(`{"name":"` + bucket + `"}`)
	req, _ := http.NewRequest("POST", baseURL+"/storage/v1/b?project=p", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

// gcsObjectMetadataRaw fetches GCS object metadata via the JSON API.
func gcsObjectMetadataRaw(t *testing.T, bucket, object string) map[string]any {
	t.Helper()
	resp, err := http.Get(baseURL + "/storage/v1/b/" + bucket + "/o/" + object)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var m map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	return m
}

// TestGCS_EmittedURLsHTTPS asserts the JSON API emits selfLink +
// mediaLink with https://, matching real GCS (HTTPS-only).
func TestGCS_EmittedURLsHTTPS(t *testing.T) {
	bucket := "https-bucket"
	gcsRESTCreate(t, bucket)

	uploadReq, _ := http.NewRequest("POST",
		baseURL+"/upload/storage/v1/b/"+bucket+"/o?uploadType=media&name=hk",
		strings.NewReader("https-test"))
	uploadReq.Header.Set("Content-Type", "text/plain")
	uploadResp, err := http.DefaultClient.Do(uploadReq)
	require.NoError(t, err)
	uploadBody, _ := io.ReadAll(uploadResp.Body)
	uploadResp.Body.Close()
	require.Equal(t, http.StatusOK, uploadResp.StatusCode, "upload status: %s", uploadBody)

	var uploadMeta map[string]any
	require.NoError(t, json.Unmarshal(uploadBody, &uploadMeta))
	assert.Truef(t, strings.HasPrefix(uploadMeta["selfLink"].(string), "https://"),
		"upload selfLink must be https://; got %q", uploadMeta["selfLink"])
	assert.Truef(t, strings.HasPrefix(uploadMeta["mediaLink"].(string), "https://"),
		"upload mediaLink must be https://; got %q", uploadMeta["mediaLink"])

	getMeta := gcsObjectMetadataRaw(t, bucket, "hk")
	assert.Truef(t, strings.HasPrefix(getMeta["selfLink"].(string), "https://"),
		"GET selfLink must be https://; got %q", getMeta["selfLink"])
	assert.Truef(t, strings.HasPrefix(getMeta["mediaLink"].(string), "https://"),
		"GET mediaLink must be https://; got %q", getMeta["mediaLink"])
}

// TestGCS_ResumableUpload exercises the full resumable upload contract:
// 1. POST init with metadata in body → 200 + Location with upload_id
// 2. PUT partial chunk with Content-Range → 308 Resume Incomplete
// 3. PUT final chunk → 200 + object metadata
// 4. GET on the destination returns the concatenated payload
func TestGCS_ResumableUpload(t *testing.T) {
	bucket := "resumable-bucket"
	gcsRESTCreate(t, bucket)

	initBody := strings.NewReader(`{"name":"resumable-obj","contentType":"text/plain"}`)
	initReq, _ := http.NewRequest("POST",
		baseURL+"/upload/storage/v1/b/"+bucket+"/o?uploadType=resumable",
		initBody)
	initReq.Header.Set("Content-Type", "application/json")
	initResp, err := http.DefaultClient.Do(initReq)
	require.NoError(t, err)
	initResp.Body.Close()
	require.Equal(t, http.StatusOK, initResp.StatusCode)
	location := initResp.Header.Get("Location")
	require.NotEmpty(t, location, "Location header must carry the session URL")
	assert.True(t, strings.HasPrefix(location, "https://"),
		"sim emits https:// to match real GCS fidelity")
	assert.Contains(t, location, "upload_id=")

	// Sim listens on plain HTTP under test (`baseURL` is `http://...`).
	// Real GCS is HTTPS-only so the sim's Location header is https; the
	// SDK follows it verbatim against a TLS-fronted sim in production
	// deployments. For local HTTP testing, swap the scheme back so the
	// chunk PUT reaches the same listener.
	if strings.HasPrefix(baseURL, "http://") {
		location = strings.Replace(location, "https://", "http://", 1)
	}

	payload := []byte("0123456789abcdef")
	totalSize := len(payload)
	half := totalSize / 2

	chunk1 := payload[:half]
	c1Req, _ := http.NewRequest("PUT", location, bytes.NewReader(chunk1))
	c1Req.Header.Set("Content-Range",
		fmt.Sprintf("bytes 0-%d/%d", half-1, totalSize))
	c1Resp, err := http.DefaultClient.Do(c1Req)
	require.NoError(t, err)
	c1Resp.Body.Close()
	require.Equal(t, 308, c1Resp.StatusCode,
		"partial chunk must return 308 Resume Incomplete")
	rangeHdr := c1Resp.Header.Get("Range")
	assert.Equal(t, fmt.Sprintf("bytes=0-%d", half-1), rangeHdr)

	chunk2 := payload[half:]
	c2Req, _ := http.NewRequest("PUT", location, bytes.NewReader(chunk2))
	c2Req.Header.Set("Content-Range",
		fmt.Sprintf("bytes %d-%d/%d", half, totalSize-1, totalSize))
	c2Resp, err := http.DefaultClient.Do(c2Req)
	require.NoError(t, err)
	c2Body, _ := io.ReadAll(c2Resp.Body)
	c2Resp.Body.Close()
	require.Equal(t, http.StatusOK, c2Resp.StatusCode,
		"final chunk must return 200; got %d body=%s", c2Resp.StatusCode, c2Body)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(c2Body, &meta))
	assert.Equal(t, "resumable-obj", meta["name"])
	assert.Equal(t, "16", meta["size"])

	xmlResp, err := http.Get(baseURL + "/" + bucket + "/resumable-obj")
	require.NoError(t, err)
	defer xmlResp.Body.Close()
	got, _ := io.ReadAll(xmlResp.Body)
	assert.Equal(t, payload, got, "object bytes must equal concatenated chunks")
}

// TestGCS_MultipartUploadBodyName exercises name-in-multipart-metadata
// (the form the Go SDK uses when Object.Name is set on the struct).
func TestGCS_MultipartUploadBodyName(t *testing.T) {
	bucket := "multipart-bucket"
	gcsRESTCreate(t, bucket)

	boundary := "BB"
	mpBody := strings.Join([]string{
		"--" + boundary,
		"Content-Type: application/json",
		"",
		`{"name":"mpk"}`,
		"--" + boundary,
		"Content-Type: application/octet-stream",
		"",
		"multipart-payload",
		"--" + boundary + "--",
	}, "\r\n")
	req, _ := http.NewRequest("POST",
		baseURL+"/upload/storage/v1/b/"+bucket+"/o?uploadType=multipart",
		strings.NewReader(mpBody))
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "multipart status: %s", body)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(body, &meta))
	assert.Equal(t, "mpk", meta["name"])
	assert.Truef(t, strings.HasPrefix(meta["selfLink"].(string), "https://"),
		"multipart upload selfLink must be https://")
}

// TestGCS_ObjectsCompose exercises POST /storage/v1/b/{bucket}/o/{name}/compose.
func TestGCS_ObjectsCompose(t *testing.T) {
	bucket := "compose-bucket"
	gcsRESTCreate(t, bucket)

	for _, p := range []struct{ name, body string }{
		{"part1", "hello "}, {"part2", "world"},
	} {
		req, _ := http.NewRequest("POST",
			baseURL+"/upload/storage/v1/b/"+bucket+"/o?uploadType=media&name="+p.name,
			strings.NewReader(p.body))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "upload %s", p.name)
	}

	composeBody := `{"sourceObjects":[{"name":"part1"},{"name":"part2"}],"destination":{"contentType":"text/plain"}}`
	req, _ := http.NewRequest("POST",
		baseURL+"/storage/v1/b/"+bucket+"/o/composed/compose",
		strings.NewReader(composeBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "compose status: %s", body)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(body, &meta))
	assert.Equal(t, "composed", meta["name"])
	assert.Equal(t, "11", meta["size"], "concat size = 6+5 = 11")
	assert.Equal(t, "text/plain", meta["contentType"])

	xmlResp, err := http.Get(baseURL + "/" + bucket + "/composed")
	require.NoError(t, err)
	defer xmlResp.Body.Close()
	got, _ := io.ReadAll(xmlResp.Body)
	assert.Equal(t, "hello world", string(got))
}

func TestGCS_ObjectsCopyToAndSortedPrefixes(t *testing.T) {
	bucket := "copyto-bucket"
	gcsRESTCreate(t, bucket)

	for _, p := range []struct{ name, body string }{
		{"z-dir/source.txt", "copyTo payload"},
		{"b-dir/object.txt", "b"},
		{"a-dir/object.txt", "a"},
	} {
		req, _ := http.NewRequest("POST",
			baseURL+"/upload/storage/v1/b/"+bucket+"/o?uploadType=media&name="+url.QueryEscape(p.name),
			strings.NewReader(p.body))
		req.Header.Set("Content-Type", "text/plain")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "upload %s: %s", p.name, body)
	}

	copyBody := strings.NewReader(`{
		"contentType":"application/x-copy",
		"contentEncoding":"identity",
		"contentLanguage":"fr",
		"contentDisposition":"attachment; filename=\"result.txt\"",
		"cacheControl":"private, max-age=30",
		"storageClass":"NEARLINE",
		"customTime":"2026-01-02T03:04:05Z",
		"metadata":{"reviewed":"true","source":"copyTo"}
	}`)
	copyURL := baseURL + "/storage/v1/b/" + bucket + "/o/" + url.PathEscape("z-dir/source.txt") +
		"/copyTo/b/" + bucket + "/o/" + url.PathEscape("copied/result.txt")
	copyReq, _ := http.NewRequest("POST", copyURL, copyBody)
	copyReq.Header.Set("Content-Type", "application/json")
	copyResp, err := http.DefaultClient.Do(copyReq)
	require.NoError(t, err)
	body, _ := io.ReadAll(copyResp.Body)
	copyResp.Body.Close()
	require.Equal(t, http.StatusOK, copyResp.StatusCode, "copyTo status: %s", body)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(body, &meta))
	assert.Equal(t, "copied/result.txt", meta["name"])
	assert.Equal(t, "application/x-copy", meta["contentType"])
	assert.Equal(t, "identity", meta["contentEncoding"])
	assert.Equal(t, "fr", meta["contentLanguage"])
	assert.Equal(t, `attachment; filename="result.txt"`, meta["contentDisposition"])
	assert.Equal(t, "private, max-age=30", meta["cacheControl"])
	assert.Equal(t, "NEARLINE", meta["storageClass"])
	assert.Equal(t, "2026-01-02T03:04:05Z", meta["customTime"])
	assert.Equal(t, map[string]any{"reviewed": "true", "source": "copyTo"}, meta["metadata"])

	getMeta := gcsObjectMetadataRaw(t, bucket, url.PathEscape("copied/result.txt"))
	assert.Equal(t, "application/x-copy", getMeta["contentType"])
	assert.Equal(t, "identity", getMeta["contentEncoding"])
	assert.Equal(t, "fr", getMeta["contentLanguage"])
	assert.Equal(t, `attachment; filename="result.txt"`, getMeta["contentDisposition"])
	assert.Equal(t, "private, max-age=30", getMeta["cacheControl"])
	assert.Equal(t, "NEARLINE", getMeta["storageClass"])
	assert.Equal(t, "2026-01-02T03:04:05Z", getMeta["customTime"])
	assert.Equal(t, map[string]any{"reviewed": "true", "source": "copyTo"}, getMeta["metadata"])

	headReq, _ := http.NewRequest("HEAD", baseURL+"/"+bucket+"/copied/result.txt", nil)
	headReq.Header.Set("Accept-Encoding", "identity")
	headResp, err := http.DefaultClient.Do(headReq)
	require.NoError(t, err)
	headResp.Body.Close()
	require.Equal(t, http.StatusOK, headResp.StatusCode)
	assert.Equal(t, "application/x-copy", headResp.Header.Get("Content-Type"))
	assert.Equal(t, "identity", headResp.Header.Get("Content-Encoding"))
	assert.Equal(t, "fr", headResp.Header.Get("Content-Language"))
	assert.Equal(t, `attachment; filename="result.txt"`, headResp.Header.Get("Content-Disposition"))
	assert.Equal(t, "private, max-age=30", headResp.Header.Get("Cache-Control"))
	assert.Equal(t, "true", headResp.Header.Get("X-Goog-Meta-Reviewed"))

	xmlResp, err := http.Get(baseURL + "/" + bucket + "/copied/result.txt")
	require.NoError(t, err)
	defer xmlResp.Body.Close()
	got, _ := io.ReadAll(xmlResp.Body)
	assert.Equal(t, "copyTo payload", string(got))

	listResp, err := http.Get(baseURL + "/storage/v1/b/" + bucket + "/o?delimiter=/")
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var listing struct {
		Prefixes []string `json:"prefixes"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listing))
	assert.Equal(t, []string{"a-dir/", "b-dir/", "copied/", "z-dir/"}, listing.Prefixes)
}

func TestGCS_ObjectMetadataValidation(t *testing.T) {
	bucket := "metadata-validation-bucket"
	gcsRESTCreate(t, bucket)

	uploadReq, _ := http.NewRequest("POST",
		baseURL+"/upload/storage/v1/b/"+bucket+"/o?uploadType=media&name=source.txt",
		strings.NewReader("source payload"))
	uploadReq.Header.Set("Content-Type", "text/plain")
	uploadResp, err := http.DefaultClient.Do(uploadReq)
	require.NoError(t, err)
	uploadBody, _ := io.ReadAll(uploadResp.Body)
	uploadResp.Body.Close()
	require.Equal(t, http.StatusOK, uploadResp.StatusCode, "source upload: %s", uploadBody)

	boundary := "META"
	multipartBody := strings.Join([]string{
		"--" + boundary,
		"Content-Type: application/json",
		"",
		`{"name":"bad-multipart","customTime":"not-rfc3339"}`,
		"--" + boundary,
		"Content-Type: application/octet-stream",
		"",
		"bad",
		"--" + boundary + "--",
	}, "\r\n")
	longLanguage := strings.Repeat("x", 101)
	cases := []struct {
		name        string
		url         string
		body        string
		contentType string
		want        string
	}{
		{
			name:        "multipart upload rejects invalid customTime",
			url:         baseURL + "/upload/storage/v1/b/" + bucket + "/o?uploadType=multipart",
			body:        multipartBody,
			contentType: "multipart/related; boundary=" + boundary,
			want:        "customTime",
		},
		{
			name:        "resumable init rejects invalid customTime",
			url:         baseURL + "/upload/storage/v1/b/" + bucket + "/o?uploadType=resumable",
			body:        `{"name":"bad-resumable","customTime":"not-rfc3339"}`,
			contentType: "application/json",
			want:        "customTime",
		},
		{
			name:        "compose rejects invalid customTime",
			url:         baseURL + "/storage/v1/b/" + bucket + "/o/bad-composed/compose",
			body:        `{"sourceObjects":[{"name":"source.txt"}],"destination":{"customTime":"not-rfc3339"}}`,
			contentType: "application/json",
			want:        "customTime",
		},
		{
			name: "copyTo rejects invalid customTime",
			url: baseURL + "/storage/v1/b/" + bucket + "/o/" + url.PathEscape("source.txt") +
				"/copyTo/b/" + bucket + "/o/bad-copy",
			body:        `{"customTime":"not-rfc3339"}`,
			contentType: "application/json",
			want:        "customTime",
		},
		{
			name: "rewriteTo rejects too-long contentLanguage",
			url: baseURL + "/storage/v1/b/" + bucket + "/o/" + url.PathEscape("source.txt") +
				"/rewriteTo/b/" + bucket + "/o/bad-rewrite",
			body:        `{"contentLanguage":"` + longLanguage + `"}`,
			contentType: "application/json",
			want:        "contentLanguage",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", tc.url, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
			assert.Contains(t, string(body), tc.want)
		})
	}
}
