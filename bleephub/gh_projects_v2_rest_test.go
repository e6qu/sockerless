package bleephub

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// seedProjectV2Org creates an org (admin as owner) and an org-owned
// Projects v2 project through the shared store.
func seedProjectV2Org(t *testing.T, orgLogin, title string) (*Org, *ProjectV2) {
	t.Helper()
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.GetOrg(orgLogin)
	if org == nil {
		org = testServer.store.CreateOrg(admin, orgLogin, orgLogin, "")
		if org == nil {
			t.Fatalf("create org %s failed", orgLogin)
		}
	}
	p := testServer.store.ProjectsV2.CreateProject(org.ID, "Organization", title, admin.ID)
	return org, p
}

func TestOrgProjectsV2_ListGetAndVisibility(t *testing.T) {
	org, p := seedProjectV2Org(t, "pv2-vis-org", "Roadmap Q3")

	// Unauthenticated → 401 (the projects surface requires a token).
	resp := ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2", "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated list = %d, want 401", resp.StatusCode)
	}

	// Admin (org member) sees the private project.
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list = %d, want 200", resp.StatusCode)
	}
	projects := decodeJSONArray(t, resp)
	found := false
	for _, pj := range projects {
		if int(pj["id"].(float64)) == p.ID {
			found = true
			if pj["title"] != "Roadmap Q3" {
				t.Errorf("title = %v", pj["title"])
			}
			if pj["state"] != "open" {
				t.Errorf("state = %v, want open", pj["state"])
			}
			owner, _ := pj["owner"].(map[string]interface{})
			if owner == nil || owner["login"] != org.Login {
				t.Errorf("owner = %v, want %s", pj["owner"], org.Login)
			}
			creator, _ := pj["creator"].(map[string]interface{})
			if creator == nil || creator["login"] != "admin" {
				t.Errorf("creator = %v, want admin", pj["creator"])
			}
		}
	}
	if !found {
		t.Fatal("project missing from org list")
	}

	// GET by number.
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2/"+strconv.Itoa(p.Number), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get = %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if int(got["number"].(float64)) != p.Number {
		t.Fatalf("number = %v, want %d", got["number"], p.Number)
	}

	// Unknown project number → 404. Unknown org → 404.
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2/99999", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown number = %d, want 404", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/no-such-org/projectsV2", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown org = %d, want 404", resp.StatusCode)
	}

	// A non-member's PAT cannot see the private project (404 / excluded).
	outsider := createTestUser(t, "pv2-outsider")
	outsiderToken := testServer.store.CreateToken(outsider.ID, "repo").Value
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2/"+strconv.Itoa(p.Number), outsiderToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("outsider get private project = %d, want 404", resp.StatusCode)
	}

	// Once public, the outsider can read it.
	public := true
	testServer.store.ProjectsV2.UpdateProject(p.ID, nil, nil, &public)
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2/"+strconv.Itoa(p.Number), outsiderToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("outsider get public project = %d, want 200", resp.StatusCode)
	}
	got = decodeJSON(t, resp)
	if got["public"] != true {
		t.Fatalf("public = %v, want true", got["public"])
	}
}

func TestOrgProjectsV2_ListQueryFilter(t *testing.T) {
	org, _ := seedProjectV2Org(t, "pv2-q-org", "Alpha launch")
	admin := testServer.store.UsersByLogin["admin"]
	closedProj := testServer.store.ProjectsV2.CreateProject(org.ID, "Organization", "Beta cleanup", admin.ID)
	closed := true
	testServer.store.ProjectsV2.UpdateProject(closedProj.ID, nil, &closed, nil)

	resp := ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2?q=is%3Aclosed", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("q=is:closed = %d, want 200", resp.StatusCode)
	}
	projects := decodeJSONArray(t, resp)
	if len(projects) != 1 || projects[0]["title"] != "Beta cleanup" {
		t.Fatalf("q=is:closed matched %v", projects)
	}
	if projects[0]["state"] != "closed" {
		t.Fatalf("state = %v, want closed", projects[0]["state"])
	}
	if projects[0]["closed_at"] == nil {
		t.Fatal("closed_at should be set on a closed project")
	}

	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/projectsV2?q=alpha", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("q=alpha = %d, want 200", resp.StatusCode)
	}
	projects = decodeJSONArray(t, resp)
	if len(projects) != 1 || projects[0]["title"] != "Alpha launch" {
		t.Fatalf("q=alpha matched %v", projects)
	}
}

func TestOrgProjectV2Fields_CreateListGet(t *testing.T) {
	org, p := seedProjectV2Org(t, "pv2-fields-org", "Fields")
	base := "/api/v3/orgs/" + org.Login + "/projectsV2/" + strconv.Itoa(p.Number)

	// text field
	resp := ghPost(t, base+"/fields", defaultToken, map[string]interface{}{
		"name": "Notes", "data_type": "text",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create text field = %d, want 201", resp.StatusCode)
	}
	textField := decodeJSON(t, resp)
	if textField["data_type"] != "text" {
		t.Fatalf("data_type = %v, want text", textField["data_type"])
	}

	// single select field with rich options
	resp = ghPost(t, base+"/fields", defaultToken, map[string]interface{}{
		"name": "Priority", "data_type": "single_select",
		"single_select_options": []map[string]interface{}{
			{"name": "High", "color": "RED", "description": "Do first"},
			{"name": "Low"},
		},
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create single select field = %d, want 201", resp.StatusCode)
	}
	ssField := decodeJSON(t, resp)
	options, _ := ssField["options"].([]interface{})
	if len(options) != 2 {
		t.Fatalf("options = %v, want 2 entries", ssField["options"])
	}
	first, _ := options[0].(map[string]interface{})
	name, _ := first["name"].(map[string]interface{})
	if name["raw"] != "High" || first["color"] != "RED" {
		t.Fatalf("first option = %v", first)
	}
	second, _ := options[1].(map[string]interface{})
	if second["color"] != "GRAY" {
		t.Fatalf("default option color = %v, want GRAY", second["color"])
	}

	// iteration field
	resp = ghPost(t, base+"/fields", defaultToken, map[string]interface{}{
		"name": "Sprint", "data_type": "iteration",
		"iteration_configuration": map[string]interface{}{
			"start_date": "2026-07-06", "duration": 7,
			"iterations": []map[string]interface{}{
				{"title": "Sprint 1", "start_date": "2026-07-06", "duration": 7},
				{"title": "Sprint 2", "start_date": "2026-07-13"},
			},
		},
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create iteration field = %d, want 201", resp.StatusCode)
	}
	iterField := decodeJSON(t, resp)
	cfg, _ := iterField["configuration"].(map[string]interface{})
	if cfg == nil {
		t.Fatal("iteration field missing configuration")
	}
	if cfg["start_day"] != float64(1) { // 2026-07-06 is a Monday
		t.Errorf("start_day = %v, want 1", cfg["start_day"])
	}
	iterations, _ := cfg["iterations"].([]interface{})
	if len(iterations) != 2 {
		t.Fatalf("iterations = %v, want 2", cfg["iterations"])
	}

	// list fields
	resp = ghGet(t, base+"/fields", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list fields = %d, want 200", resp.StatusCode)
	}
	fields := decodeJSONArray(t, resp)
	if len(fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(fields))
	}

	// get single field
	fieldID := int(ssField["id"].(float64))
	resp = ghGet(t, base+"/fields/"+strconv.Itoa(fieldID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get field = %d, want 200", resp.StatusCode)
	}
	gotField := decodeJSON(t, resp)
	if gotField["name"] != "Priority" {
		t.Fatalf("field name = %v", gotField["name"])
	}
	resp = ghGet(t, base+"/fields/999999", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown field = %d, want 404", resp.StatusCode)
	}

	// validation failures
	for i, body := range []map[string]interface{}{
		{"name": "Priority", "data_type": "text"},       // duplicate name
		{"name": "Empty", "data_type": "single_select"}, // no options
		{"name": "Weird", "data_type": "money"},         // bad data type
		{"data_type": "text"},                           // missing name
		{"name": "Ext", "data_type": "iteration"},       // missing iteration configuration
		{"issue_field_id": 12345},                       // no issue fields exist
		{"name": "BadIter", "data_type": "iteration", // malformed start_date
			"iteration_configuration": map[string]interface{}{"start_date": "07/06/2026"}},
	} {
		resp = ghPost(t, base+"/fields", defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid field body #%d = %d, want 422", i, resp.StatusCode)
		}
	}
}

func TestOrgProjectV2Items_AddGetPatchDelete(t *testing.T) {
	org, p := seedProjectV2Org(t, "pv2-items-org", "Items")
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateOrgRepo(org, admin, "pv2-items-repo", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}
	issue := testServer.store.CreateIssue(repo.ID, admin.ID, "Fix the flux capacitor", "", nil, nil, 0)
	base := "/api/v3/orgs/" + org.Login + "/projectsV2/" + strconv.Itoa(p.Number)

	// Add by database ID.
	resp := ghPost(t, base+"/items", defaultToken, map[string]interface{}{
		"type": "Issue", "id": issue.ID,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("add item = %d, want 201", resp.StatusCode)
	}
	item := decodeJSON(t, resp)
	if item["content_type"] != "Issue" {
		t.Fatalf("content_type = %v, want Issue", item["content_type"])
	}
	itemID := int(item["id"].(float64))

	// Adding the same issue again is idempotent (same item ID).
	resp = ghPost(t, base+"/items", defaultToken, map[string]interface{}{
		"type": "Issue", "owner": org.Login, "repo": repo.Name, "number": issue.Number,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("re-add item = %d, want 201", resp.StatusCode)
	}
	again := decodeJSON(t, resp)
	if int(again["id"].(float64)) != itemID {
		t.Fatalf("re-add produced a different item: %v vs %d", again["id"], itemID)
	}

	// Draft item.
	resp = ghPost(t, base+"/drafts", defaultToken, map[string]interface{}{
		"title": "Draft: write docs", "body": "eventually",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create draft = %d, want 201", resp.StatusCode)
	}
	draft := decodeJSON(t, resp)
	if draft["content_type"] != "DraftIssue" {
		t.Fatalf("draft content_type = %v", draft["content_type"])
	}
	draftID := int(draft["id"].(float64))

	// List items — both present, content populated.
	resp = ghGet(t, base+"/items", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list items = %d, want 200", resp.StatusCode)
	}
	items := decodeJSONArray(t, resp)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	// Get single item with content.
	resp = ghGet(t, base+"/items/"+strconv.Itoa(itemID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get item = %d, want 200", resp.StatusCode)
	}
	gotItem := decodeJSON(t, resp)
	content, _ := gotItem["content"].(map[string]interface{})
	if content == nil || content["title"] != "Fix the flux capacitor" {
		t.Fatalf("item content = %v", gotItem["content"])
	}

	// Field values: text + single select set via PATCH, read back.
	textField := testServer.store.ProjectsV2.CreateField(p.ID, "Notes", ProjectV2FieldText, nil, nil)
	ssField := testServer.store.ProjectsV2.CreateField(p.ID, "Status", ProjectV2FieldSingleSelect,
		[]*ProjectV2SingleSelectOption{{Name: "Todo"}, {Name: "Done"}}, nil)
	numField := testServer.store.ProjectsV2.CreateField(p.ID, "Points", ProjectV2FieldNumber, nil, nil)
	resp = ghPatch(t, base+"/items/"+strconv.Itoa(itemID), defaultToken, map[string]interface{}{
		"fields": []map[string]interface{}{
			{"id": textField.ID, "value": "needs review"},
			{"id": ssField.ID, "value": ssField.Options[1].ID},
			{"id": numField.ID, "value": 5},
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("patch item = %d, want 200", resp.StatusCode)
	}
	patched := decodeJSON(t, resp)
	values := map[string]interface{}{}
	for _, raw := range patched["fields"].([]interface{}) {
		entry := raw.(map[string]interface{})
		values[entry["name"].(string)] = entry["value"]
	}
	if values["Notes"] != "needs review" {
		t.Fatalf("Notes value = %v", values["Notes"])
	}
	status, _ := values["Status"].(map[string]interface{})
	if status == nil || status["name"] != "Done" {
		t.Fatalf("Status value = %v", values["Status"])
	}
	if values["Points"] != float64(5) {
		t.Fatalf("Points value = %v", values["Points"])
	}

	// Clearing a value via null; explicit fields selection returns null.
	resp = ghPatch(t, base+"/items/"+strconv.Itoa(itemID), defaultToken, map[string]interface{}{
		"fields": []map[string]interface{}{{"id": textField.ID, "value": nil}},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("clear value = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, base+"/items/"+strconv.Itoa(itemID)+"?fields="+strconv.Itoa(textField.ID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get with fields selection = %d, want 200", resp.StatusCode)
	}
	selected := decodeJSON(t, resp)
	selFields := selected["fields"].([]interface{})
	if len(selFields) != 1 || selFields[0].(map[string]interface{})["value"] != nil {
		t.Fatalf("selected fields = %v, want one null value", selected["fields"])
	}

	// PATCH validation: unknown field ID / wrong value type / empty list.
	for i, body := range []map[string]interface{}{
		{"fields": []map[string]interface{}{{"id": 999999, "value": "x"}}},
		{"fields": []map[string]interface{}{{"id": numField.ID, "value": "not a number"}}},
		{"fields": []map[string]interface{}{}},
	} {
		resp = ghPatch(t, base+"/items/"+strconv.Itoa(itemID), defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid patch #%d = %d, want 422", i, resp.StatusCode)
		}
	}

	// q filter: is:draft matches only the draft.
	resp = ghGet(t, base+"/items?q=is%3Adraft", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("q=is:draft = %d, want 200", resp.StatusCode)
	}
	drafts := decodeJSONArray(t, resp)
	if len(drafts) != 1 || int(drafts[0]["id"].(float64)) != draftID {
		t.Fatalf("q=is:draft matched %v", drafts)
	}

	// Field-value filter: Status:Done matches the patched item.
	resp = ghGet(t, base+"/items?q=Status%3ADone", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("q=Status:Done = %d, want 200", resp.StatusCode)
	}
	done := decodeJSONArray(t, resp)
	if len(done) != 1 || int(done[0]["id"].(float64)) != itemID {
		t.Fatalf("q=Status:Done matched %v", done)
	}

	// Delete item.
	resp = ghDelete(t, base+"/items/"+strconv.Itoa(draftID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete item = %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, base+"/items/"+strconv.Itoa(draftID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("deleted item get = %d, want 404", resp.StatusCode)
	}

	// Add-item validation.
	for i, body := range []map[string]interface{}{
		{"type": "Gist", "id": issue.ID},
		{"type": "Issue"},
		{"type": "Issue", "id": 999999},
		{"type": "Issue", "owner": org.Login, "repo": "nope", "number": 1},
	} {
		resp = ghPost(t, base+"/items", defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid add-item body #%d = %d, want 422", i, resp.StatusCode)
		}
	}
}

func TestOrgProjectV2Views_CreateAndListItems(t *testing.T) {
	org, p := seedProjectV2Org(t, "pv2-views-org", "Views")
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateOrgRepo(org, admin, "pv2-views-repo", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}
	issue := testServer.store.CreateIssue(repo.ID, admin.ID, "An issue", "", nil, nil, 0)
	testServer.store.ProjectsV2.AddItem(p.ID, "Issue", issue.ID, admin.ID)
	testServer.store.ProjectsV2.AddDraftItem(p.ID, "A draft", "", admin.ID)
	field := testServer.store.ProjectsV2.CreateField(p.ID, "Stage", ProjectV2FieldText, nil, nil)
	base := "/api/v3/orgs/" + org.Login + "/projectsV2/" + strconv.Itoa(p.Number)

	resp := ghPost(t, base+"/views", defaultToken, map[string]interface{}{
		"name": "Issues board", "layout": "board", "filter": "is:issue",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create view = %d, want 201", resp.StatusCode)
	}
	view := decodeJSON(t, resp)
	if view["layout"] != "board" || view["filter"] != "is:issue" {
		t.Fatalf("view = %v", view)
	}
	visible, _ := view["visible_fields"].([]interface{})
	if len(visible) != 1 || int(visible[0].(float64)) != field.ID {
		t.Fatalf("default visible_fields = %v, want [%d]", view["visible_fields"], field.ID)
	}
	viewNumber := int(view["number"].(float64))

	// The view's filter hides the draft.
	resp = ghGet(t, base+"/views/"+strconv.Itoa(viewNumber)+"/items", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("view items = %d, want 200", resp.StatusCode)
	}
	items := decodeJSONArray(t, resp)
	if len(items) != 1 || items[0]["content_type"] != "Issue" {
		t.Fatalf("view items = %v, want the one issue", items)
	}

	resp = ghGet(t, base+"/views/999/items", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown view = %d, want 404", resp.StatusCode)
	}

	for i, body := range []map[string]interface{}{
		{"name": "x", "layout": "kanban"},
		{"layout": "board"},
		{"name": "y", "layout": "table", "visible_fields": []int{999999}},
	} {
		resp = ghPost(t, base+"/views", defaultToken, body)
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Fatalf("invalid view body #%d = %d, want 422", i, resp.StatusCode)
		}
	}
}

func TestUserProjectsV2_FlowIncludingUserIDRoutes(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	p := testServer.store.ProjectsV2.CreateProject(admin.ID, "User", "Personal backlog", admin.ID)

	// List + get by login.
	resp := ghGet(t, "/api/v3/users/admin/projectsV2", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user list = %d, want 200", resp.StatusCode)
	}
	projects := decodeJSONArray(t, resp)
	found := false
	for _, pj := range projects {
		if int(pj["id"].(float64)) == p.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("user project missing from list")
	}
	base := "/api/v3/users/admin/projectsV2/" + strconv.Itoa(p.Number)
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user get = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Field create/list/get through /users/{username}.
	resp = ghPost(t, base+"/fields", defaultToken, map[string]interface{}{
		"name": "Effort", "data_type": "number",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("user field create = %d, want 201", resp.StatusCode)
	}
	field := decodeJSON(t, resp)
	fieldID := int(field["id"].(float64))
	resp = ghGet(t, base+"/fields/"+strconv.Itoa(fieldID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user field get = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, base+"/fields", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user fields list = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Draft creation goes through POST /user/{user_id}/… (by user ID).
	resp = ghPost(t, fmt.Sprintf("/api/v3/user/%d/projectsV2/%d/drafts", admin.ID, p.Number), defaultToken,
		map[string]interface{}{"title": "My draft"})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("user draft create = %d, want 201", resp.StatusCode)
	}
	draft := decodeJSON(t, resp)
	draftID := int(draft["id"].(float64))

	// Item PATCH + GET + DELETE via /users/{username}.
	resp = ghPatch(t, base+"/items/"+strconv.Itoa(draftID), defaultToken, map[string]interface{}{
		"fields": []map[string]interface{}{{"id": fieldID, "value": 3}},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user item patch = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, base+"/items", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user items list = %d, want 200", resp.StatusCode)
	}
	items := decodeJSONArray(t, resp)
	if len(items) != 1 {
		t.Fatalf("user items = %d, want 1", len(items))
	}

	// View creation goes through POST /users/{user_id}/… (by user ID).
	resp = ghPost(t, fmt.Sprintf("/api/v3/users/%d/projectsV2/%d/views", admin.ID, p.Number), defaultToken,
		map[string]interface{}{"name": "Table", "layout": "table"})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("user view create = %d, want 201", resp.StatusCode)
	}
	view := decodeJSON(t, resp)
	viewNumber := int(view["number"].(float64))
	resp = ghGet(t, base+"/views/"+strconv.Itoa(viewNumber)+"/items", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("user view items = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Another user cannot write to this project (403), and the
	// authenticated-user draft route rejects an addressee mismatch.
	other := createTestUser(t, "pv2-other-user")
	otherToken := testServer.store.CreateToken(other.ID, "repo").Value
	resp = ghPost(t, fmt.Sprintf("/api/v3/user/%d/projectsV2/%d/drafts", admin.ID, p.Number), otherToken,
		map[string]interface{}{"title": "Sneaky"})
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("mismatched user draft = %d, want 403", resp.StatusCode)
	}

	// Private user project hidden from others.
	resp = ghGet(t, base, otherToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("other user get private project = %d, want 404", resp.StatusCode)
	}

	// Delete the item last.
	resp = ghDelete(t, base+"/items/"+strconv.Itoa(draftID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("user item delete = %d, want 204", resp.StatusCode)
	}
}

func TestOrgProjectV2Items_CursorPagination(t *testing.T) {
	org, p := seedProjectV2Org(t, "pv2-page-org", "Paging")
	admin := testServer.store.UsersByLogin["admin"]
	for i := 0; i < 5; i++ {
		testServer.store.ProjectsV2.AddDraftItem(p.ID, fmt.Sprintf("Draft %d", i), "", admin.ID)
	}
	base := "/api/v3/orgs/" + org.Login + "/projectsV2/" + strconv.Itoa(p.Number)

	resp := ghGet(t, base+"/items?per_page=2", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("page 1 = %d, want 200", resp.StatusCode)
	}
	link := resp.Header.Get("Link")
	page1 := decodeJSONArray(t, resp)
	if len(page1) != 2 {
		t.Fatalf("page 1 size = %d, want 2", len(page1))
	}
	if link == "" || !containsRel(link, "next") {
		t.Fatalf("page 1 Link = %q, want rel=next", link)
	}

	after := extractCursor(t, link, "after")
	resp = ghGet(t, base+"/items?per_page=2&after="+after, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("page 2 = %d, want 200", resp.StatusCode)
	}
	page2 := decodeJSONArray(t, resp)
	if len(page2) != 2 {
		t.Fatalf("page 2 size = %d, want 2", len(page2))
	}
	if page1[0]["id"] == page2[0]["id"] {
		t.Fatal("page 2 repeats page 1")
	}
}

func containsRel(link, rel string) bool {
	return strings.Contains(link, `rel="`+rel+`"`)
}

// extractCursor pulls a cursor query parameter out of a Link header.
func extractCursor(t *testing.T, link, param string) string {
	t.Helper()
	start := strings.Index(link, "<")
	end := strings.Index(link, ">")
	if start < 0 || end < start {
		t.Fatalf("malformed Link header %q", link)
	}
	u, err := url.Parse(link[start+1 : end])
	if err != nil {
		t.Fatalf("parse Link URL: %v", err)
	}
	cursor := u.Query().Get(param)
	if cursor == "" {
		t.Fatalf("Link %q has no %s cursor", link, param)
	}
	return cursor
}
