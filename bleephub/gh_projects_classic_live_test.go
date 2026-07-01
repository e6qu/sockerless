package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Live-server shape test for Projects classic (v1). Exercises every route
// through the shared TestMain server so the OpenAPI response-shape validator
// observes them.
func TestLiveProjectsClassic_FullFlow(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "live-projects-classic", "", false)

	// Create project
	createBody, _ := json.Marshal(map[string]any{"name": "Live Roadmap", "body": "live body"})
	resp, err := authedPost("/api/v3/repos/admin/live-projects-classic/projects", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create project: %d %s", resp.StatusCode, b)
	}
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created project: %v", err)
	}
	resp.Body.Close()
	projID := int(created["id"].(float64))

	// List projects
	resp = authedGet(t, "/api/v3/repos/admin/live-projects-classic/projects")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list projects: %d %s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&[]map[string]any{})
	resp.Body.Close()

	// Get project
	resp = authedGet(t, "/api/v3/projects/"+itoa(projID))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get project: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Create column
	colBody, _ := json.Marshal(map[string]any{"name": "Live Column"})
	resp, err = authedPost("/api/v3/projects/"+itoa(projID)+"/columns", "application/json", bytes.NewReader(colBody))
	if err != nil {
		t.Fatalf("create column: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create column: %d %s", resp.StatusCode, b)
	}
	var col map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&col); err != nil {
		t.Fatalf("decode column: %v", err)
	}
	resp.Body.Close()
	colID := int(col["id"].(float64))

	// Create issue and issue card
	issue := testServer.store.CreateIssue(repo.ID, admin.ID, "live issue", "body", nil, nil, 0)
	cardBody, _ := json.Marshal(map[string]any{"content_id": issue.ID, "content_type": "Issue"})
	resp, err = authedPost("/api/v3/projects/columns/"+itoa(colID)+"/cards", "application/json", bytes.NewReader(cardBody))
	if err != nil {
		t.Fatalf("create issue card: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create issue card: %d %s", resp.StatusCode, b)
	}
	var issueCard map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&issueCard); err != nil {
		t.Fatalf("decode issue card: %v", err)
	}
	resp.Body.Close()

	// Create note card
	noteBody, _ := json.Marshal(map[string]any{"note": "live note"})
	resp, err = authedPost("/api/v3/projects/columns/"+itoa(colID)+"/cards", "application/json", bytes.NewReader(noteBody))
	if err != nil {
		t.Fatalf("create note card: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create note card: %d %s", resp.StatusCode, b)
	}
	var noteCard map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&noteCard); err != nil {
		t.Fatalf("decode note card: %v", err)
	}
	resp.Body.Close()

	// List cards
	resp = authedGet(t, "/api/v3/projects/columns/"+itoa(colID)+"/cards")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list cards: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Move note card
	cardID := int(noteCard["id"].(float64))
	moveBody, _ := json.Marshal(map[string]any{"position": "first"})
	req, _ := http.NewRequest("POST", testBaseURL+"/api/v3/projects/columns/cards/"+itoa(cardID)+"/moves", strings.NewReader(string(moveBody)))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("move card: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("move card: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Update project
	patchBody, _ := json.Marshal(map[string]any{"state": "closed"})
	req, _ = http.NewRequest("PATCH", testBaseURL+"/api/v3/projects/"+itoa(projID), strings.NewReader(string(patchBody)))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch project: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch project: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	_ = issueCard
}
