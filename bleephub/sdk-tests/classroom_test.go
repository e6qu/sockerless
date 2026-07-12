package sdktests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	github "github.com/google/go-github/v88/github"
)

func classroomBrowserWrite(t *testing.T, method, path string, body interface{}, out interface{}) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal Classroom browser request: %v", err)
	}
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new Classroom browser request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := rawHTTP.Do(req)
	if err != nil {
		t.Fatalf("Classroom browser request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode Classroom browser response: %v (%s)", err, raw)
		}
	}
}

func TestGitHubClassroomOfficialSDKReadsBrowserCreatedCoursework(t *testing.T) {
	org := uniqueName("classroom-sdk")
	if status := createOrganizationViaAdminAPI(t, org, "Classroom SDK", nil); status != http.StatusCreated {
		t.Fatalf("create organization status = %d", status)
	}
	starter, _, err := client.Repositories.Create(ctx(), org, &github.Repository{Name: github.Ptr("starter"), AutoInit: github.Ptr(true), Private: github.Ptr(false)})
	if err != nil {
		t.Fatalf("create starter repository: %v", err)
	}

	var classroom struct {
		ID int64 `json:"id"`
	}
	classroomBrowserWrite(t, http.MethodPost, "/classroom-data/classrooms", map[string]interface{}{"name": "SDK Classroom", "organization": org}, &classroom)
	var assignment struct {
		ID         int64  `json:"id"`
		InviteLink string `json:"invite_link"`
	}
	classroomBrowserWrite(t, http.MethodPost, fmt.Sprintf("/classroom-data/classrooms/%d/assignments", classroom.ID), map[string]interface{}{
		"title": "Real repository assignment", "type": "individual", "starter_code_repository": starter.GetFullName(),
		"invitations_enabled": true, "autograding_tests": []map[string]interface{}{{"name": "Repository test", "command": "test -f README.md", "points": 10}},
	}, &assignment)
	inviteCode := assignment.InviteLink[strings.LastIndex(assignment.InviteLink, "/")+1:]
	classroomBrowserWrite(t, http.MethodPost, "/classroom-data/invitations/"+inviteCode+"/accept", map[string]interface{}{}, nil)

	classrooms, _, err := client.Classroom.ListClassrooms(ctx(), nil)
	if err != nil {
		t.Fatalf("ListClassrooms: %v", err)
	}
	found := false
	for _, item := range classrooms {
		if item.GetID() == classroom.ID {
			found = item.GetName() == "SDK Classroom"
		}
	}
	if !found {
		t.Fatalf("browser-created classroom missing from official SDK list: %+v", classrooms)
	}
	gotClassroom, _, err := client.Classroom.GetClassroom(ctx(), classroom.ID)
	if err != nil || gotClassroom.GetOrganization().GetLogin() != org {
		t.Fatalf("GetClassroom = %+v, %v", gotClassroom, err)
	}
	assignments, _, err := client.Classroom.ListClassroomAssignments(ctx(), classroom.ID, nil)
	if err != nil || len(assignments) != 1 {
		t.Fatalf("ListClassroomAssignments = %+v, %v", assignments, err)
	}
	gotAssignment, _, err := client.Classroom.GetAssignment(ctx(), assignment.ID)
	if err != nil || gotAssignment.GetStarterCodeRepository().GetFullName() != starter.GetFullName() {
		t.Fatalf("GetAssignment = %+v, %v", gotAssignment, err)
	}
	accepted, _, err := client.Classroom.ListAcceptedAssignments(ctx(), assignment.ID, nil)
	if err != nil || len(accepted) != 1 || accepted[0].GetRepository().GetFullName() != org+"/real-repository-assignment-admin" {
		t.Fatalf("ListAcceptedAssignments = %+v, %v", accepted, err)
	}
	grades, _, err := client.Classroom.GetAssignmentGrades(ctx(), assignment.ID)
	if err != nil || len(grades) != 1 || grades[0].GetPointsAvailable() != 10 {
		t.Fatalf("GetAssignmentGrades = %+v, %v", grades, err)
	}
}
