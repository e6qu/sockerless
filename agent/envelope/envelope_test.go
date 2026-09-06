package envelope

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bootstrapHandler answers the way every sockerless bootstrap does: it
// decodes the envelope, runs the command, and returns 200 with the exit
// code inside the body. The command here is the bootstrap's own echo of
// what it received, which is what the tests assert on.
func bootstrapHandler(t *testing.T, wantBearer, wantHost string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if wantBearer != "" && r.Header.Get("Authorization") != "Bearer "+wantBearer {
			t.Errorf("Authorization = %q, want bearer %q", r.Header.Get("Authorization"), wantBearer)
		}
		if wantHost != "" && r.Host != wantHost {
			t.Errorf("Host = %q, want %q", r.Host, wantHost)
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		exec := req.Sockerless.Exec
		if exec == nil {
			t.Fatal("request carried no exec")
		}
		var stdin []byte
		if exec.Stdin != "" {
			decoded, err := ParseResult([]byte(`{"sockerlessExecResult":{"exitCode":0,"stdout":"` + exec.Stdin + `","stderr":""}}`))
			if err != nil {
				t.Fatalf("decode stdin: %v", err)
			}
			stdin = decoded.Stdout
		}
		exit := 0
		if len(exec.Argv) == 3 && exec.Argv[0] == "sh" && exec.Argv[2] == "exit 17" {
			exit = 17
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(NewResponse(exit, append([]byte(strings.Join(exec.Argv, " ")+"\n"), stdin...), []byte("err:"+exec.Workdir)))
	}
}

func TestPost_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(bootstrapHandler(t, "test-token", ""))
	defer srv.Close()

	res, err := Post(context.Background(), nil, srv.URL, PostOptions{BearerToken: "test-token"}, Exec{
		Argv:    []string{"echo", "hi"},
		Workdir: "/w",
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", res.ExitCode)
	}
	if string(res.Stdout) != "echo hi\n" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
	if string(res.Stderr) != "err:/w" {
		t.Fatalf("stderr = %q", res.Stderr)
	}
}

func TestPost_HostHeader(t *testing.T) {
	srv := httptest.NewServer(bootstrapHandler(t, "", "app.internal.example"))
	defer srv.Close()

	if _, err := Post(context.Background(), nil, srv.URL, PostOptions{Host: "app.internal.example"}, Exec{Argv: []string{"true"}}); err != nil {
		t.Fatalf("Post: %v", err)
	}
}

func TestPost_StdinPayload(t *testing.T) {
	srv := httptest.NewServer(bootstrapHandler(t, "", ""))
	defer srv.Close()

	res, err := Post(context.Background(), nil, srv.URL, PostOptions{}, Exec{
		Argv:  []string{"cat"},
		Stdin: EncodeStdin([]byte("input-bytes")),
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !strings.HasSuffix(string(res.Stdout), "input-bytes") {
		t.Fatalf("stdout = %q, want stdin echoed back", res.Stdout)
	}
}

func TestPost_NonZeroExit(t *testing.T) {
	srv := httptest.NewServer(bootstrapHandler(t, "", ""))
	defer srv.Close()

	res, err := Post(context.Background(), nil, srv.URL, PostOptions{}, Exec{Argv: []string{"sh", "-c", "exit 17"}})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if res.ExitCode != 17 {
		t.Fatalf("exitCode = %d, want 17", res.ExitCode)
	}
}

func TestPost_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream connect failed"))
	}))
	defer srv.Close()

	_, err := Post(context.Background(), nil, srv.URL, PostOptions{}, Exec{Argv: []string{"echo", "hi"}})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want status 502 surfaced", err)
	}
}

func TestPost_InputValidation(t *testing.T) {
	if _, err := Post(context.Background(), nil, "", PostOptions{}, Exec{Argv: []string{"x"}}); err == nil {
		t.Fatal("expected error on empty url")
	}
	if _, err := Post(context.Background(), nil, "http://example.invalid", PostOptions{}, Exec{}); err == nil {
		t.Fatal("expected error on empty argv")
	}
}

func TestParse(t *testing.T) {
	body, err := json.Marshal(NewRequest(Exec{Argv: []string{"ls", "-l"}, Env: []string{"A=1"}}))
	if err != nil {
		t.Fatal(err)
	}
	exec, ok := Parse(append([]byte("  \n"), body...))
	if !ok {
		t.Fatal("Parse rejected a valid envelope")
	}
	if len(exec.Argv) != 2 || exec.Argv[1] != "-l" || len(exec.Env) != 1 {
		t.Fatalf("Parse = %+v", exec)
	}
	for _, notAnEnvelope := range []string{
		"",
		"raw bytes",
		`{"event":"ordinary"}`,
		`{"sockerless":{"exec":{"argv":[]}}}`,
		`{"sockerless":{}}`,
		`{not json`,
	} {
		if _, ok := Parse([]byte(notAnEnvelope)); ok {
			t.Errorf("Parse(%q) accepted a body that is not an exec envelope", notAnEnvelope)
		}
	}
}

func TestEncodeStdinEmptyOmitted(t *testing.T) {
	if EncodeStdin(nil) != "" {
		t.Fatal("empty stdin must encode to the empty string so the field is omitted")
	}
	body, _ := json.Marshal(NewRequest(Exec{Argv: []string{"x"}}))
	if strings.Contains(string(body), "stdin") {
		t.Fatalf("request with no stdin must omit the field: %s", body)
	}
}

func TestExecWorkloadRoundTrips(t *testing.T) {
	raw, err := json.Marshal(NewRequest(Exec{Argv: []string{"tail", "-f", "/dev/null"}, Workdir: "/src", Workload: true}))
	if err != nil {
		t.Fatal(err)
	}
	parsed, ok := Parse(raw)
	if !ok || !parsed.Workload || parsed.Workdir != "/src" {
		t.Fatalf("Parse = %+v, %v", parsed, ok)
	}
	plain, _ := json.Marshal(NewRequest(Exec{Argv: []string{"id"}}))
	if bytes.Contains(plain, []byte("workload")) {
		t.Errorf("an exec envelope does not carry the workload mark: %s", plain)
	}
}
