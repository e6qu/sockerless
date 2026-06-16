package azurecommon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ExecEnvelopeRequest is the JSON shape sent to the in-image bootstrap
// (sockerless-cloudrun-bootstrap, reused as the ACA bootstrap) for each
// HTTP buffered-invoke. Path B from specs/CLOUD_RESOURCE_MAPPING.md
// § Lesson 8 — invocation-based exec via HTTP POST. The ACA runner-stage
// path uses this (rather than the reverse-agent WebSocket) so it never
// depends on a persistent backend→container connection — same robustness
// reason cloudrun + gcf route their stages through the bootstrap's HTTP
// listener.
//
// Wire format is identical to gcp-common's ExecEnvelopeRequest because the
// bootstrap is the same binary; kept as a per-cloud copy under the no-
// cross-backend-import rule (azure-common must not import gcp-common).
type ExecEnvelopeRequest struct {
	Sockerless struct {
		Exec ExecEnvelopeExec `json:"exec"`
	} `json:"sockerless"`
}

// ExecEnvelopeExec is the per-exec payload nested inside ExecEnvelopeRequest.
type ExecEnvelopeExec struct {
	Argv    []string `json:"argv"`
	Tty     bool     `json:"tty,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Stdin   string   `json:"stdin,omitempty"` // base64
}

// ExecEnvelopeResponse is the bootstrap's reply.
type ExecEnvelopeResponse struct {
	SockerlessExecResult struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"` // base64
		Stderr   string `json:"stderr"` // base64
	} `json:"sockerlessExecResult"`
}

// ExecResult carries the parsed bootstrap response with stdout/stderr
// already base64-decoded to bytes for the caller.
type ExecResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// PostExecEnvelope sends the envelope to the bootstrap URL via HTTP POST.
// hostHeader, when non-empty, sets the request Host header so the sim's
// virtual-host routing (or real ACA ingress) matches the right App — the
// URL host is the endpoint coordinate, the Host header is the App's FQDN
// (mirrors azf's invokeURLForHost). Returns the parsed {exitCode, stdout,
// stderr} or an error on transport failure / HTTP 5xx without an envelope
// body.
func PostExecEnvelope(ctx context.Context, client *http.Client, url, hostHeader string, env ExecEnvelopeExec) (*ExecResult, error) {
	if url == "" {
		return nil, fmt.Errorf("PostExecEnvelope: url is required")
	}
	if len(env.Argv) == 0 {
		return nil, fmt.Errorf("PostExecEnvelope: env.Argv is required")
	}

	var req ExecEnvelopeRequest
	req.Sockerless.Exec = env
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal exec envelope: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build exec request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if hostHeader != "" {
		httpReq.Host = hostHeader
	}

	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
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

	return ParseExecResult(respBody)
}

// ParseExecResult decodes the bootstrap envelope JSON into an ExecResult,
// base64-decoding the stdout + stderr fields.
func ParseExecResult(respBody []byte) (*ExecResult, error) {
	var envResp ExecEnvelopeResponse
	if err := json.Unmarshal(respBody, &envResp); err != nil {
		return nil, fmt.Errorf("parse exec response envelope: %w (raw=%q)", err, truncate(respBody, 256))
	}
	stdoutBytes, err := base64.StdEncoding.DecodeString(envResp.SockerlessExecResult.Stdout)
	if err != nil {
		return nil, fmt.Errorf("decode response stdout: %w", err)
	}
	stderrBytes, err := base64.StdEncoding.DecodeString(envResp.SockerlessExecResult.Stderr)
	if err != nil {
		return nil, fmt.Errorf("decode response stderr: %w", err)
	}
	return &ExecResult{
		ExitCode: envResp.SockerlessExecResult.ExitCode,
		Stdout:   stdoutBytes,
		Stderr:   stderrBytes,
	}, nil
}

// EncodeStdin base64-encodes the byte slice for the envelope's Stdin field.
func EncodeStdin(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}
