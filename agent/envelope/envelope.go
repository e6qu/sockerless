// Package envelope is the wire contract between a sockerless FaaS backend
// and the bootstrap binary it runs inside a cloud workload
// (sockerless-lambda-bootstrap, sockerless-cloudrun-bootstrap,
// sockerless-gcf-bootstrap, sockerless-azf-bootstrap).
//
// A backend that needs a command run inside a workload without a
// long-lived inbound channel posts a Request; the bootstrap runs
// Request.Sockerless.Exec and answers with a Response whose exit code and
// base64 streams the backend decodes into a Result. Both halves of the
// conversation import this package, so the JSON shape is defined once.
// The transport differs per cloud (an AWS Lambda Invoke payload, an
// HTTP POST to a Cloud Run service URL, an HTTP POST with the Azure
// Container Apps ingress host in the Host header); the body does not.
package envelope

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Request is the JSON body a backend sends. `exec` is a pointer so a
// payload that is valid JSON but not an envelope (an ordinary AWS Lambda
// event, for example) decodes to a nil Exec and the bootstrap runs the
// image's own command instead.
type Request struct {
	Sockerless struct {
		Exec *Exec `json:"exec,omitempty"`
	} `json:"sockerless"`
}

// Exec describes the command the bootstrap runs.
type Exec struct {
	Argv    []string `json:"argv"`
	Tty     bool     `json:"tty,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Stdin   string   `json:"stdin,omitempty"` // base64
}

// Response is the JSON body the bootstrap answers with. Stdout and
// Stderr are base64 so arbitrary bytes round-trip through JSON.
type Response struct {
	SockerlessExecResult struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"` // base64
		Stderr   string `json:"stderr"` // base64
	} `json:"sockerlessExecResult"`
}

// Result is a decoded Response.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// NewRequest wraps an Exec in the request shape.
func NewRequest(exec Exec) Request {
	var req Request
	req.Sockerless.Exec = &exec
	return req
}

// NewResponse encodes an exit code and the two output streams.
func NewResponse(exitCode int, stdout, stderr []byte) Response {
	var res Response
	res.SockerlessExecResult.ExitCode = exitCode
	res.SockerlessExecResult.Stdout = base64.StdEncoding.EncodeToString(stdout)
	res.SockerlessExecResult.Stderr = base64.StdEncoding.EncodeToString(stderr)
	return res
}

// Parse decodes body as a Request and returns its Exec when the body is
// an envelope carrying a command. A body that is empty, is not a JSON
// object, does not decode, or has no argv is not an envelope; callers
// then run their default path.
func Parse(body []byte) (Exec, bool) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return Exec{}, false
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return Exec{}, false
	}
	if req.Sockerless.Exec == nil || len(req.Sockerless.Exec.Argv) == 0 {
		return Exec{}, false
	}
	return *req.Sockerless.Exec, true
}

// ParseResult decodes a Response body, base64-decoding both streams.
func ParseResult(body []byte) (*Result, error) {
	var res Response
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("parse exec response envelope: %w (raw=%q)", err, truncate(body, 256))
	}
	stdout, err := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if err != nil {
		return nil, fmt.Errorf("decode response stdout: %w", err)
	}
	stderr, err := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stderr)
	if err != nil {
		return nil, fmt.Errorf("decode response stderr: %w", err)
	}
	return &Result{ExitCode: res.SockerlessExecResult.ExitCode, Stdout: stdout, Stderr: stderr}, nil
}

// EncodeStdin base64-encodes bytes for Exec.Stdin. Empty input yields
// the empty string so the field is omitted.
func EncodeStdin(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// PostOptions are the per-cloud transport details of an HTTP invoke.
type PostOptions struct {
	// BearerToken, when set, is sent as `Authorization: Bearer`. Google
	// Cloud Run and Cloud Run Functions require an ID token for the
	// service URL.
	BearerToken string
	// Host, when set, overrides the request Host header. Azure Container
	// Apps ingress routes on the app's fully qualified domain name while
	// the URL host may be a different coordinate.
	Host string
}

// Post sends exec to url over HTTP and decodes the bootstrap's Response.
// The bootstrap always answers 200 with the exit code inside the body; any
// other status means the bootstrap itself did not run and is returned as
// an error carrying the status and body.
//
// Interactive TTY sessions do not use this call: an HTTP request and
// response cannot stream. Those go through the reverse-agent WebSocket.
func Post(ctx context.Context, client *http.Client, url string, opts PostOptions, exec Exec) (*Result, error) {
	if url == "" {
		return nil, fmt.Errorf("envelope.Post: url is required")
	}
	if len(exec.Argv) == 0 {
		return nil, fmt.Errorf("envelope.Post: exec.Argv is required")
	}
	body, err := json.Marshal(NewRequest(exec))
	if err != nil {
		return nil, fmt.Errorf("marshal exec envelope: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build exec request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.BearerToken)
	}
	if opts.Host != "" {
		req.Host = opts.Host
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post exec envelope: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read exec response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exec invoke returned status %d: %s", resp.StatusCode, truncate(respBody, 512))
	}
	return ParseResult(respBody)
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
