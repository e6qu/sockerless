package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestWriteErrorJSONFormat(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, &api.ServerError{Message: "test error"})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var resp api.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Message != "test error" {
		t.Errorf("message = %q, want 'test error'", resp.Message)
	}
}

func TestWriteErrorNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, &api.NotFoundError{Resource: "container", ID: "abc"})

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	var resp api.ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Message, "No such container") {
		t.Errorf("message = %q, want 'No such container'", resp.Message)
	}
}

func TestWriteErrorConflict(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, &api.ConflictError{Message: "name in use"})

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestWriteErrorBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, &api.InvalidParameterError{Message: "bad param"})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp api.ErrorResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Message != "bad param" {
		t.Errorf("message = %q, want 'bad param'", resp.Message)
	}
}

func TestServerErrorType(t *testing.T) {
	err := &api.ServerError{Message: "internal"}
	if err.StatusCode() != 500 {
		t.Errorf("expected 500, got %d", err.StatusCode())
	}
	if err.Error() != "internal" {
		t.Errorf("expected 'internal', got %q", err.Error())
	}
}
