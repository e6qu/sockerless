package bleephub

import (
	"testing"
)

func TestMilestoneLabels_ListFromMilestoneIssues(t *testing.T) {
	repo := createRepoWriteRepo(t, false)

	for _, l := range []map[string]interface{}{
		{"name": "bug", "color": "d73a4a"},
		{"name": "feature", "color": "a2eeef"},
		{"name": "unused", "color": "cccccc"},
	} {
		resp := ghPost(t, "/api/v3/repos/admin/"+repo+"/labels", defaultToken, l)
		requireStatus(t, resp, 201)
	}

	resp := ghPost(t, "/api/v3/repos/admin/"+repo+"/milestones", defaultToken, map[string]interface{}{"title": "v1.0"})
	milestone := decodeJSONWithStatus(t, resp, 201)
	msNumber := int(milestone["number"].(float64))
	if msNumber != 1 {
		t.Fatalf("milestone number = %d, want 1", msNumber)
	}

	// Two issues in the milestone sharing the "bug" label; a third issue
	// outside the milestone carries "unused", which must not appear.
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/issues", defaultToken, map[string]interface{}{
		"title": "one", "labels": []string{"bug"}, "milestone": msNumber,
	})
	requireStatus(t, resp, 201)
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/issues", defaultToken, map[string]interface{}{
		"title": "two", "labels": []string{"bug", "feature"}, "milestone": msNumber,
	})
	requireStatus(t, resp, 201)
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/issues", defaultToken, map[string]interface{}{
		"title": "outside", "labels": []string{"unused"},
	})
	requireStatus(t, resp, 201)

	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/milestones/1/labels", defaultToken)
	labels := decodeJSONWithStatus2xxArray(t, resp, 200)
	names := map[string]bool{}
	for _, l := range labels {
		names[l["name"].(string)] = true
	}
	if len(labels) != 2 || !names["bug"] || !names["feature"] {
		t.Fatalf("milestone labels = %v, want exactly [bug feature]", labels)
	}

	resp = ghGet(t, "/api/v3/repos/admin/"+repo+"/milestones/42/labels", defaultToken)
	requireStatus(t, resp, 404)
}
