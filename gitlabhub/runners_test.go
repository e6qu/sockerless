package gitlabhub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger().Level(zerolog.WarnLevel)
	return NewServer(":0", logger)
}

func doRequest(s *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	var bodyReader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)
	return rr
}

func TestRegisterRunner(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token:       "test-reg-token",
		Description: "test runner",
		TagList:     []string{"docker", "linux"},
	})

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp RunnerRegistrationResponse
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.ID == 0 {
		t.Fatal("expected non-zero runner ID")
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if resp.Token[:5] != "glrt-" {
		t.Fatalf("expected token prefix glrt-, got %s", resp.Token[:5])
	}
}

func TestRegisterRunnerMissingToken(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Description: "test",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestVerifyRunner(t *testing.T) {
	s := newTestServer(t)

	// Register first
	rr := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token:       "test-reg-token",
		Description: "test runner",
	})
	var reg RunnerRegistrationResponse
	json.NewDecoder(rr.Body).Decode(&reg)

	// Verify with valid token
	rr2 := doRequest(s, "POST", "/api/v4/runners/verify", RunnerVerifyRequest{
		Token: reg.Token,
	})
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}
}

func TestVerifyRunnerInvalidToken(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "POST", "/api/v4/runners/verify", RunnerVerifyRequest{
		Token: "invalid-token",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestUnregisterRunner(t *testing.T) {
	s := newTestServer(t)

	// Register
	rr := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token:       "test-reg-token",
		Description: "to delete",
	})
	var reg RunnerRegistrationResponse
	json.NewDecoder(rr.Body).Decode(&reg)

	// Unregister
	rr2 := doRequest(s, "DELETE", "/api/v4/runners", RunnerVerifyRequest{
		Token: reg.Token,
	})
	if rr2.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr2.Code)
	}

	// Verify should now fail
	rr3 := doRequest(s, "POST", "/api/v4/runners/verify", RunnerVerifyRequest{
		Token: reg.Token,
	})
	if rr3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 after unregister, got %d", rr3.Code)
	}
}

func TestRegisterRunnerWithTags(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
		Token:       "test-reg-token",
		Description: "tagged runner",
		TagList:     []string{"docker", "linux", "arm64"},
	})

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	var resp RunnerRegistrationResponse
	json.NewDecoder(rr.Body).Decode(&resp)

	runner := s.store.LookupRunnerByToken(resp.Token)
	if runner == nil {
		t.Fatal("runner not found by token")
	}
	if len(runner.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(runner.Tags))
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "GET", "/health", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["service"] != "gitlabhub" {
		t.Fatalf("expected service=gitlabhub, got %s", resp["service"])
	}
}

func TestMultipleRunnerRegistrations(t *testing.T) {
	s := newTestServer(t)

	tokens := make(map[string]bool)
	for i := 0; i < 5; i++ {
		rr := doRequest(s, "POST", "/api/v4/runners", RunnerRegistrationRequest{
			Token:       "reg-token",
			Description: "runner",
		})
		var resp RunnerRegistrationResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if tokens[resp.Token] {
			t.Fatalf("duplicate token: %s", resp.Token)
		}
		tokens[resp.Token] = true
	}

	if len(tokens) != 5 {
		t.Fatalf("expected 5 unique tokens, got %d", len(tokens))
	}
}
