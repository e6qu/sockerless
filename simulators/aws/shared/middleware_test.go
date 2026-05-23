package simulator

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestLoggingMiddleware_StreamingEnvelopeSentinels asserts that streaming-
// envelope headers surface as structured fields on the request log when
// present. This is the operator-visible footprint that would have made
// the BUG-1099 shape ("handler reads body without consuming aws-chunked
// envelope") greppable. Fields must be absent on plain requests so the
// happy path stays quiet.
func TestLoggingMiddleware_StreamingEnvelopeSentinels(t *testing.T) {
	cases := []struct {
		name       string
		headers    map[string]string
		wantFields []string
		wantAbsent []string
	}{
		{
			name:       "plain request — no envelope fields",
			headers:    map[string]string{},
			wantAbsent: []string{"content_encoding", "transfer_encoding", "streaming_variant", "decoded_content_length", "azure_sse_c", "gcs_sse_c"},
		},
		{
			name: "aws-chunked envelope",
			headers: map[string]string{
				"Content-Encoding":             "aws-chunked",
				"x-amz-content-sha256":         "STREAMING-AWS4-HMAC-SHA256-PAYLOAD",
				"x-amz-decoded-content-length": "11",
			},
			wantFields: []string{
				`"content_encoding":"aws-chunked"`,
				`"streaming_variant":"STREAMING-AWS4-HMAC-SHA256-PAYLOAD"`,
				`"decoded_content_length":"11"`,
			},
		},
		{
			name: "aws-chunked with trailer variant",
			headers: map[string]string{
				"Content-Encoding":     "aws-chunked",
				"x-amz-content-sha256": "STREAMING-UNSIGNED-PAYLOAD-TRAILER",
			},
			wantFields: []string{
				`"streaming_variant":"STREAMING-UNSIGNED-PAYLOAD-TRAILER"`,
			},
		},
		{
			name: "non-streaming sha256 — no variant field",
			headers: map[string]string{
				"x-amz-content-sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			},
			wantAbsent: []string{"streaming_variant"},
		},
		{
			name: "azure SSE-C sentinel",
			headers: map[string]string{
				"x-ms-encryption-key-sha256": "abc123",
			},
			wantFields: []string{`"azure_sse_c":true`},
		},
		{
			name: "gcs SSE-C sentinel",
			headers: map[string]string{
				"x-goog-encryption-key-sha256": "xyz789",
			},
			wantFields: []string{`"gcs_sse_c":true`},
		},
		{
			name: "http chunked transfer-encoding",
			headers: map[string]string{
				"Transfer-Encoding": "chunked",
			},
			wantFields: []string{`"transfer_encoding":"chunked"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := zerolog.New(buf)

			h := LoggingMiddleware(logger, "aws")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPut, "/bucket/key", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)

			out := buf.String()
			for _, want := range tc.wantFields {
				if !strings.Contains(out, want) {
					t.Errorf("expected log to contain %q; got: %s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, `"`+absent+`":`) {
					t.Errorf("expected log to NOT contain field %q; got: %s", absent, out)
				}
			}
		})
	}
}
