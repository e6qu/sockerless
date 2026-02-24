package gitlabhub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestVariableCRUD(t *testing.T) {
	s := newTestServer(t)

	// Create project
	project := s.store.CreateProject("test-proj")

	// Create variable
	rr := doRequest(s, "POST", fmt.Sprintf("/api/v4/projects/%d/variables", project.ID), map[string]interface{}{
		"key":   "MY_SECRET",
		"value": "secret-val",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// List variables
	rr2 := doRequest(s, "GET", fmt.Sprintf("/api/v4/projects/%d/variables", project.ID), nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr2.Code)
	}

	var vars []map[string]interface{}
	json.NewDecoder(rr2.Body).Decode(&vars)
	if len(vars) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(vars))
	}
	if vars[0]["key"] != "MY_SECRET" {
		t.Fatalf("expected key=MY_SECRET, got %s", vars[0]["key"])
	}

	// Delete variable
	rr3 := doRequest(s, "DELETE", fmt.Sprintf("/api/v4/projects/%d/variables/MY_SECRET", project.ID), nil)
	if rr3.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rr3.Code)
	}

	// List should be empty
	rr4 := doRequest(s, "GET", fmt.Sprintf("/api/v4/projects/%d/variables", project.ID), nil)
	json.NewDecoder(rr4.Body).Decode(&vars)
	if len(vars) != 0 {
		t.Fatalf("expected 0 variables after delete, got %d", len(vars))
	}
}

func TestMaskedVariableHandling(t *testing.T) {
	s := newTestServer(t)

	project := s.store.CreateProject("proj")

	rr := doRequest(s, "POST", fmt.Sprintf("/api/v4/projects/%d/variables", project.ID), map[string]interface{}{
		"key":    "SECRET",
		"value":  "hidden-value",
		"masked": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	// Verify it's stored as masked
	s.store.mu.RLock()
	v := project.Variables["SECRET"]
	s.store.mu.RUnlock()

	if v == nil {
		t.Fatal("variable not found")
	}
	if !v.Masked {
		t.Fatal("expected masked=true")
	}
}

func TestProtectedVariableFilter(t *testing.T) {
	s := newTestServer(t)

	project := s.store.CreateProject("proj")

	rr := doRequest(s, "POST", fmt.Sprintf("/api/v4/projects/%d/variables", project.ID), map[string]interface{}{
		"key":       "PROTECTED_VAR",
		"value":     "protected-value",
		"protected": true,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	s.store.mu.RLock()
	v := project.Variables["PROTECTED_VAR"]
	s.store.mu.RUnlock()

	if v == nil {
		t.Fatal("variable not found")
	}
	if !v.Protected {
		t.Fatal("expected protected=true")
	}
}

func TestDeleteNonexistentVariable(t *testing.T) {
	s := newTestServer(t)
	project := s.store.CreateProject("proj")

	rr := doRequest(s, "DELETE", fmt.Sprintf("/api/v4/projects/%d/variables/NOPE", project.ID), nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestProjectNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := doRequest(s, "GET", "/api/v4/projects/9999/variables", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestVariableRequiresKey(t *testing.T) {
	s := newTestServer(t)
	project := s.store.CreateProject("proj")

	rr := doRequest(s, "POST", fmt.Sprintf("/api/v4/projects/%d/variables", project.ID), map[string]interface{}{
		"value": "no-key",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
