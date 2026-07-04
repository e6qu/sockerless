package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"testing"
)

// seedJSON posts a JSON body to an internal seeding endpoint and decodes the
// created entity.
func seedJSON(t *testing.T, path string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp, err := authedPost(path, "application/json", bytes.NewReader(mustJSON(body)))
	if err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("seed %s: %d %s", path, resp.StatusCode, b)
	}
	out := map[string]interface{}{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode seed response: %v", err)
	}
	return out
}

func TestGitHubClassroomSurface(t *testing.T) {
	org := createTestOrg(t)
	starterRepo := createTestRepo(t)
	studentRepo := createTestRepo(t)
	student := createTestUser(t, "classroom-student")

	classroom := seedJSON(t, "/internal/classrooms", map[string]interface{}{
		"name": "Programming Go",
		"org":  org,
	})
	classroomID := strconv.Itoa(int(classroom["id"].(float64)))

	assignment := seedJSON(t, "/internal/classrooms/"+classroomID+"/assignments", map[string]interface{}{
		"title":                   "Intro to Binaries",
		"type":                    "individual",
		"starter_code_repository": starterRepo,
		"public_repo":             true,
		"invitations_enabled":     true,
		"editor":                  "codespaces",
		"language":                "go",
	})
	assignmentID := strconv.Itoa(int(assignment["id"].(float64)))

	seedJSON(t, "/internal/assignments/"+assignmentID+"/accepted_assignments", map[string]interface{}{
		"students": []map[string]interface{}{
			{"login": student.Login, "roster_identifier": "student-1"},
		},
		"repository":   studentRepo,
		"submitted":    true,
		"passing":      true,
		"commit_count": 5,
		"grade":        "8/10",
	})

	// GET /classrooms
	resp := ghGet(t, "/api/v3/classrooms", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list classrooms status = %d", resp.StatusCode)
	}
	classrooms := decodeJSONArray(t, resp)
	var listed map[string]interface{}
	for _, c := range classrooms {
		if c["name"] == "Programming Go" {
			listed = c
		}
	}
	if listed == nil {
		t.Fatal("seeded classroom missing from GET /classrooms")
	}
	if listed["archived"] != false || listed["url"] == nil {
		t.Fatalf("classroom shape: %v", listed)
	}

	// GET /classrooms requires authentication on real GitHub.
	resp = ghGet(t, "/api/v3/classrooms", "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated list status = %d, want 401", resp.StatusCode)
	}

	// GET /classrooms/{classroom_id}
	resp = ghGet(t, "/api/v3/classrooms/"+classroomID, defaultToken)
	full := decodeJSONWithStatus(t, resp, 200)
	orgJSON, _ := full["organization"].(map[string]interface{})
	if orgJSON == nil || orgJSON["login"] != org {
		t.Fatalf("classroom organization = %v", full["organization"])
	}

	// GET /classrooms/{classroom_id}/assignments
	resp = ghGet(t, "/api/v3/classrooms/"+classroomID+"/assignments", defaultToken)
	assignments := decodeJSONArray(t, resp)
	if len(assignments) != 1 {
		t.Fatalf("classroom has %d assignments, want 1", len(assignments))
	}
	simple := assignments[0]
	if simple["title"] != "Intro to Binaries" || simple["slug"] != "intro-to-binaries" {
		t.Fatalf("assignment shape: %v", simple)
	}
	// Counters derive from the accepted assignment.
	if simple["accepted"] != float64(1) || simple["submitted"] != float64(1) || simple["passing"] != float64(1) {
		t.Fatalf("assignment counters: accepted=%v submitted=%v passing=%v",
			simple["accepted"], simple["submitted"], simple["passing"])
	}

	// GET /assignments/{assignment_id} (full shape with starter code repo)
	resp = ghGet(t, "/api/v3/assignments/"+assignmentID, defaultToken)
	fullAssignment := decodeJSONWithStatus(t, resp, 200)
	starter, _ := fullAssignment["starter_code_repository"].(map[string]interface{})
	if starter == nil || starter["full_name"] != starterRepo {
		t.Fatalf("starter_code_repository = %v", fullAssignment["starter_code_repository"])
	}
	nested, _ := fullAssignment["classroom"].(map[string]interface{})
	if nested == nil || nested["organization"] == nil {
		t.Fatal("full assignment must nest the full classroom")
	}

	// GET /assignments/{assignment_id}/accepted_assignments
	resp = ghGet(t, "/api/v3/assignments/"+assignmentID+"/accepted_assignments", defaultToken)
	accepted := decodeJSONArray(t, resp)
	if len(accepted) != 1 {
		t.Fatalf("accepted assignments = %d, want 1", len(accepted))
	}
	students, _ := accepted[0]["students"].([]interface{})
	if len(students) != 1 {
		t.Fatalf("students = %v", accepted[0]["students"])
	}
	studentJSON, _ := students[0].(map[string]interface{})
	if studentJSON["login"] != student.Login {
		t.Fatalf("student login = %v", studentJSON["login"])
	}
	repoJSON, _ := accepted[0]["repository"].(map[string]interface{})
	if repoJSON == nil || repoJSON["full_name"] != studentRepo {
		t.Fatalf("accepted repository = %v", accepted[0]["repository"])
	}
	if accepted[0]["grade"] != "8/10" || accepted[0]["commit_count"] != float64(5) {
		t.Fatalf("accepted shape: %v", accepted[0])
	}

	// GET /assignments/{assignment_id}/grades
	resp = ghGet(t, "/api/v3/assignments/"+assignmentID+"/grades", defaultToken)
	grades := decodeJSONArray(t, resp)
	if len(grades) != 1 {
		t.Fatalf("grades = %d, want 1", len(grades))
	}
	g := grades[0]
	if g["github_username"] != student.Login || g["roster_identifier"] != "student-1" {
		t.Fatalf("grade identity: %v", g)
	}
	if g["points_awarded"] != float64(8) || g["points_available"] != float64(10) {
		t.Fatalf("grade points: awarded=%v available=%v", g["points_awarded"], g["points_available"])
	}
	if g["assignment_name"] != "Intro to Binaries" {
		t.Fatalf("grade assignment_name = %v", g["assignment_name"])
	}
}

func TestGitHubClassroomNotFound(t *testing.T) {
	for _, path := range []string{
		"/api/v3/classrooms/999999",
		"/api/v3/assignments/999999",
		"/api/v3/assignments/999999/grades",
		"/api/v3/assignments/999999/accepted_assignments",
	} {
		resp := ghGet(t, path, defaultToken)
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("%s status = %d, want 404", path, resp.StatusCode)
		}
	}
}
