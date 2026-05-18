package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseExecEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantOK bool
	}{
		{name: "empty", body: "", wantOK: false},
		{name: "raw", body: "hello", wantOK: false},
		{name: "missing argv", body: `{"sockerless":{"exec":{"argv":[]}}}`, wantOK: false},
		{name: "valid", body: `{"sockerless":{"exec":{"argv":["echo","hi"]}}}`, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseExecEnvelope([]byte(tt.body))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestRunExecEnvelope_StdinPiped(t *testing.T) {
	env := execEnvelopeExec{
		Argv:  []string{"sh", "-c", "cat"},
		Stdin: base64.StdEncoding.EncodeToString([]byte("azf-stdin-ok\n")),
	}
	w := httptest.NewRecorder()
	runExecEnvelope(w, httptest.NewRequest("POST", "/", nil).Context(), env)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%q", w.Code, w.Body.String())
	}
	var res execEnvelopeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.SockerlessExecResult.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", res.SockerlessExecResult.ExitCode)
	}
	stdout, err := base64.StdEncoding.DecodeString(res.SockerlessExecResult.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), "azf-stdin-ok") {
		t.Fatalf("stdout = %q, want stdin echo", string(stdout))
	}
}
