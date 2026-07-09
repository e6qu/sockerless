package bleephub

import (
	"testing"
)

func TestGenerateRepositoryFromTemplate(t *testing.T) {
	template := createRepoWriteRepo(t, true)

	// Generating from a repository that is not a template is rejected.
	resp := ghPost(t, "/api/v3/repos/admin/"+template+"/generate", defaultToken,
		map[string]interface{}{"name": template + "-copy"})
	requireStatus(t, resp, 400)

	resp = ghPatch(t, "/api/v3/repos/admin/"+template, defaultToken, map[string]interface{}{"is_template": true})
	requireStatus(t, resp, 200)

	newName := template + "-gen"
	resp = ghPost(t, "/api/v3/repos/admin/"+template+"/generate", defaultToken, map[string]interface{}{
		"name":        newName,
		"description": "generated from template",
		"private":     true,
	})
	repo := decodeJSONWithStatus(t, resp, 201)
	if repo["full_name"] != "admin/"+newName {
		t.Fatalf("full_name = %v, want admin/%s", repo["full_name"], newName)
	}
	if repo["private"] != true {
		t.Fatalf("private = %v, want true", repo["private"])
	}
	if repo["default_branch"] != "main" {
		t.Fatalf("default_branch = %v, want main", repo["default_branch"])
	}
	gqlResp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){templateRepository{name,nameWithOwner,owner{login}}}}`,
		"variables": map[string]interface{}{
			"owner": "admin",
			"name":  newName,
		},
	})
	gqlData := decodeJSONWithStatus(t, gqlResp, 200)
	data, _ := gqlData["data"].(map[string]interface{})
	gqlRepo, _ := data["repository"].(map[string]interface{})
	templateRepo, _ := gqlRepo["templateRepository"].(map[string]interface{})
	if templateRepo == nil {
		t.Fatalf("templateRepository = nil, want admin/%s", template)
	}
	if templateRepo["name"] != template || templateRepo["nameWithOwner"] != "admin/"+template {
		t.Fatalf("templateRepository = %v, want admin/%s", templateRepo, template)
	}

	// The template's files exist in the generated repo as real git content.
	resp = ghGet(t, "/api/v3/repos/admin/"+newName+"/contents/README.md", defaultToken)
	contents := decodeJSONWithStatus(t, resp, 200)
	if contents["name"] != "README.md" {
		t.Fatalf("contents name = %v, want README.md", contents["name"])
	}

	// The generated history is exactly one fresh initial commit, not the
	// template's history.
	resp = ghGet(t, "/api/v3/repos/admin/"+newName+"/commits", defaultToken)
	commits := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(commits) != 1 {
		t.Fatalf("generated repo has %d commits, want 1", len(commits))
	}
	commit, _ := commits[0]["commit"].(map[string]interface{})
	if commit == nil || commit["message"] != "Initial commit" {
		t.Fatalf("initial commit = %v, want message Initial commit", commits[0])
	}

	// A taken name is a validation failure.
	resp = ghPost(t, "/api/v3/repos/admin/"+template+"/generate", defaultToken,
		map[string]interface{}{"name": newName})
	requireStatus(t, resp, 422)

	// Name is required.
	resp = ghPost(t, "/api/v3/repos/admin/"+template+"/generate", defaultToken, map[string]interface{}{})
	requireStatus(t, resp, 422)

	// Unknown template repo → 404.
	resp = ghPost(t, "/api/v3/repos/admin/definitely-not-a-repo/generate", defaultToken,
		map[string]interface{}{"name": "whatever"})
	requireStatus(t, resp, 404)
}

func TestGenerateRepositoryFromTemplate_OrgOwner(t *testing.T) {
	template := createRepoWriteRepo(t, true)
	resp := ghPatch(t, "/api/v3/repos/admin/"+template, defaultToken, map[string]interface{}{"is_template": true})
	requireStatus(t, resp, 200)

	org := createTestOrg(t)
	newName := template + "-org-gen"
	resp = ghPost(t, "/api/v3/repos/admin/"+template+"/generate", defaultToken, map[string]interface{}{
		"owner": org,
		"name":  newName,
	})
	repo := decodeJSONWithStatus(t, resp, 201)
	if repo["full_name"] != org+"/"+newName {
		t.Fatalf("full_name = %v, want %s/%s", repo["full_name"], org, newName)
	}
	resp = ghGet(t, "/api/v3/repos/"+org+"/"+newName+"/contents/README.md", defaultToken)
	requireStatus(t, resp, 200)

	// An unknown owner account is rejected.
	resp = ghPost(t, "/api/v3/repos/admin/"+template+"/generate", defaultToken, map[string]interface{}{
		"owner": "nobody-here",
		"name":  template + "-nope",
	})
	requireStatus(t, resp, 403)
}
