package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestProjectsClassic_ProjectCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	testServer.store.CreateRepo(admin, "proj-classic-crud", "", false)

	// Create
	resp := ghPost(t, "/api/v3/repos/admin/proj-classic-crud/projects", defaultToken, map[string]any{"name": "Roadmap", "body": "Q3 plans"})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create project: %d %s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	if created["name"] != "Roadmap" {
		t.Fatalf("expected name Roadmap, got %v", created["name"])
	}
	projID := int(created["id"].(float64))

	// List
	resp = ghGet(t, "/api/v3/repos/admin/proj-classic-crud/projects", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list projects: %d %s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("expected 1 project, got %d", len(list))
	}

	// Get
	resp = ghGet(t, "/api/v3/projects/"+itoa(projID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get project: %d %s", resp.StatusCode, b)
	}
	got := decodeJSON(t, resp)
	if got["body"] != "Q3 plans" {
		t.Fatalf("expected body Q3 plans, got %v", got["body"])
	}

	// Update
	resp = ghPatch(t, "/api/v3/projects/"+itoa(projID), defaultToken, map[string]any{"name": "Roadmap 2", "state": "closed"})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("update project: %d %s", resp.StatusCode, b)
	}
	updated := decodeJSON(t, resp)
	if updated["name"] != "Roadmap 2" || updated["state"] != "closed" {
		t.Fatalf("unexpected updated project: %v", updated)
	}

	// Delete
	resp = ghDelete(t, "/api/v3/projects/"+itoa(projID), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete project: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Get 404
	resp = ghGet(t, "/api/v3/projects/"+itoa(projID), defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProjectsClassic_ColumnCRUDAndMove(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "proj-classic-col", "", false)
	proj := testServer.store.CreateProjectClassic(repo, admin.ID, "Board", "", "open")

	// Create columns
	c1 := createColumn(t, proj.ID, "Todo")
	c2 := createColumn(t, proj.ID, "Done")

	// List
	resp := ghGet(t, "/api/v3/projects/"+itoa(proj.ID)+"/columns", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list columns: %d %s", resp.StatusCode, b)
	}
	var cols []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cols); err != nil {
		t.Fatalf("decode columns: %v", err)
	}
	resp.Body.Close()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}

	// Get column
	resp = ghGet(t, "/api/v3/projects/columns/"+itoa(c1), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get column: %d %s", resp.StatusCode, b)
	}
	col := decodeJSON(t, resp)
	if col["name"] != "Todo" {
		t.Fatalf("expected Todo, got %v", col["name"])
	}

	// Update
	resp = ghPatch(t, "/api/v3/projects/columns/"+itoa(c1), defaultToken, map[string]any{"name": "Backlog"})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("update column: %d %s", resp.StatusCode, b)
	}
	updated := decodeJSON(t, resp)
	if updated["name"] != "Backlog" {
		t.Fatalf("expected Backlog, got %v", updated["name"])
	}

	// Move c2 first
	moveResp := moveColumn(t, c2, "first")
	if moveResp["id"] == nil {
		t.Fatalf("move column failed: %v", moveResp)
	}
	colsAfter := listColumns(t, proj.ID)
	if int(colsAfter[0]["id"].(float64)) != c2 {
		t.Fatalf("expected c2 first after move, got %v", colsAfter)
	}

	// Move c2 after c1
	moveResp = moveColumn(t, c2, "after:"+itoa(c1))
	if moveResp["id"] == nil {
		t.Fatalf("move column after failed: %v", moveResp)
	}
	colsAfter = listColumns(t, proj.ID)
	if int(colsAfter[1]["id"].(float64)) != c2 {
		t.Fatalf("expected c2 second after after-move, got %v", colsAfter)
	}

	// Delete
	resp = ghDelete(t, "/api/v3/projects/columns/"+itoa(c2), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete column: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestProjectsClassic_CardNoteAndIssue(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "proj-classic-cards", "", false)
	proj := testServer.store.CreateProjectClassic(repo, admin.ID, "Board", "", "open")
	col := testServer.store.CreateProjectColumn(proj.ID, "Col")
	issue := testServer.store.CreateIssue(repo.ID, admin.ID, "tracked issue", "body", nil, nil, 0)

	// Note card
	card1 := createCard(t, col.ID, map[string]any{"note": "remember this"})
	if card1["note"] != "remember this" {
		t.Fatalf("expected note, got %v", card1["note"])
	}

	// Issue card
	card2 := createCard(t, col.ID, map[string]any{"content_id": issue.ID, "content_type": "Issue"})
	if card2["content_url"] == nil {
		t.Fatalf("expected content_url for issue card, got nil")
	}
	if card2["note"] != nil {
		t.Fatalf("expected note nil for issue card, got %v", card2["note"])
	}

	// List
	resp := ghGet(t, "/api/v3/projects/columns/"+itoa(col.ID)+"/cards", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list cards: %d %s", resp.StatusCode, b)
	}
	var cards []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&cards); err != nil {
		t.Fatalf("decode cards: %v", err)
	}
	resp.Body.Close()
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}

	// Get card
	cardID := int(card1["id"].(float64))
	resp = ghGet(t, "/api/v3/projects/columns/cards/"+itoa(cardID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get card: %d %s", resp.StatusCode, b)
	}
	got := decodeJSON(t, resp)
	if got["note"] != "remember this" {
		t.Fatalf("expected note, got %v", got["note"])
	}

	// Update note
	resp = ghPatch(t, "/api/v3/projects/columns/cards/"+itoa(cardID), defaultToken, map[string]any{"note": "updated note"})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("update card: %d %s", resp.StatusCode, b)
	}
	updated := decodeJSON(t, resp)
	if updated["note"] != "updated note" {
		t.Fatalf("expected updated note, got %v", updated["note"])
	}

	// Delete
	resp = ghDelete(t, "/api/v3/projects/columns/cards/"+itoa(cardID), defaultToken)
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete card: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestProjectsClassic_CardMove(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "proj-classic-move", "", false)
	proj := testServer.store.CreateProjectClassic(repo, admin.ID, "Board", "", "open")
	col1 := testServer.store.CreateProjectColumn(proj.ID, "Col1")
	col2 := testServer.store.CreateProjectColumn(proj.ID, "Col2")
	cardA := testServer.store.CreateProjectCard(col1.ID, admin.ID, "A", 0)
	cardB := testServer.store.CreateProjectCard(col1.ID, admin.ID, "B", 0)
	cardC := testServer.store.CreateProjectCard(col1.ID, admin.ID, "C", 0)
	_ = cardA

	// Move B to first
	moveCard(t, cardB.ID, 0, "first")
	cards := listCards(t, col1.ID)
	if int(cards[0]["id"].(float64)) != cardB.ID {
		t.Fatalf("expected B first, got %v", cards)
	}

	// Move B after C
	moveCard(t, cardB.ID, 0, "after:"+itoa(cardC.ID))
	cards = listCards(t, col1.ID)
	if int(cards[2]["id"].(float64)) != cardB.ID {
		t.Fatalf("expected B last, got %v", cards)
	}

	// Move B to col2 last
	moveCard(t, cardB.ID, col2.ID, "last")
	cards = listCards(t, col2.ID)
	if len(cards) != 1 || int(cards[0]["id"].(float64)) != cardB.ID {
		t.Fatalf("expected B in col2, got %v", cards)
	}
	cards = listCards(t, col1.ID)
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards in col1, got %d", len(cards))
	}
}

func TestProjectsClassic_404s(t *testing.T) {
	// Missing project
	resp := ghGet(t, "/api/v3/projects/999999", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing project, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing column
	resp = ghGet(t, "/api/v3/projects/columns/999999", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing column, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing card
	resp = ghGet(t, "/api/v3/projects/columns/cards/999999", defaultToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing card, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestProjectsClassic_RequiresAuth(t *testing.T) {
	resp := ghGet(t, "/api/v3/repos/admin/proj-classic-crud/projects", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func createColumn(t *testing.T, projectID int, name string) int {
	t.Helper()
	resp := ghPost(t, "/api/v3/projects/"+itoa(projectID)+"/columns", defaultToken, map[string]any{"name": name})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create column %s: %d %s", name, resp.StatusCode, b)
	}
	data := decodeJSON(t, resp)
	return int(data["id"].(float64))
}

func createCard(t *testing.T, columnID int, body map[string]any) map[string]any {
	t.Helper()
	resp := ghPost(t, "/api/v3/projects/columns/"+itoa(columnID)+"/cards", defaultToken, body)
	if resp.StatusCode != http.StatusCreated {
		bb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create card: %d %s", resp.StatusCode, bb)
	}
	return decodeJSON(t, resp)
}

func moveColumn(t *testing.T, columnID int, position string) map[string]any {
	t.Helper()
	resp := ghPost(t, "/api/v3/projects/columns/"+itoa(columnID)+"/moves", defaultToken, map[string]any{"position": position})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("move column: %d %s", resp.StatusCode, b)
	}
	return decodeJSON(t, resp)
}

func listColumns(t *testing.T, projectID int) []map[string]any {
	t.Helper()
	resp := ghGet(t, "/api/v3/projects/"+itoa(projectID)+"/columns", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list columns: %d %s", resp.StatusCode, b)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode columns: %v", err)
	}
	resp.Body.Close()
	return out
}

func moveCard(t *testing.T, cardID, columnID int, position string) map[string]any {
	t.Helper()
	body := map[string]any{"position": position}
	if columnID != 0 {
		body["column_id"] = columnID
	}
	resp := ghPost(t, "/api/v3/projects/columns/cards/"+itoa(cardID)+"/moves", defaultToken, body)
	if resp.StatusCode != http.StatusCreated {
		bb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("move card: %d %s", resp.StatusCode, bb)
	}
	return decodeJSON(t, resp)
}

func listCards(t *testing.T, columnID int) []map[string]any {
	t.Helper()
	resp := ghGet(t, "/api/v3/projects/columns/"+itoa(columnID)+"/cards", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list cards: %d %s", resp.StatusCode, b)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode cards: %v", err)
	}
	resp.Body.Close()
	return out
}
