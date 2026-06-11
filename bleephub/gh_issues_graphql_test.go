package bleephub

import (
	"testing"
)

// TestIssueGraphQL_SubIssueFields exercises the exact selections the gh CLI's
// `gh issue view` sends on `...on Issue` for issue-types and sub-issues. bleephub
// does not implement those GitHub features, so the fields must resolve to the
// empty/null real-GitHub shape for an issue with none — without errors.
func TestIssueGraphQL_SubIssueFields(t *testing.T) {
	// Create a repo.
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-subissue-fields",
	})
	repoData := decodeJSON(t, resp)
	if repoData["node_id"] == nil {
		t.Fatalf("expected repo node_id, got %v", repoData)
	}
	owner, _ := repoData["owner"].(map[string]interface{})
	login, _ := owner["login"].(string)
	name, _ := repoData["name"].(string)

	// Create an issue via REST.
	resp2 := ghPost(t, "/api/v3/repos/"+login+"/"+name+"/issues", defaultToken, map[string]interface{}{
		"title": "sub-issue field probe",
		"body":  "probe",
	})
	issueData := decodeJSON(t, resp2)
	num, ok := issueData["number"].(float64)
	if !ok {
		t.Fatalf("expected issue number, got %v", issueData)
	}

	// The exact selection set gh CLI's `gh issue view` sends for these four
	// fields on `...on Issue`.
	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			issue(number:$number){
				number
				stateReason
				issueType{id,name,description,color}
				parent{id,number,title,url,state,repository{nameWithOwner}}
				subIssues(first:100){nodes{id,number,title,url,state,repository{nameWithOwner}},totalCount}
				subIssuesSummary{total,completed,percentCompleted}
			}
		}
	}`

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": query,
		"variables": map[string]interface{}{
			"owner":  login,
			"name":   name,
			"number": int(num),
		},
	})
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	data := decodeJSON(t, resp3)

	if errs, ok := data["errors"]; ok && errs != nil {
		t.Fatalf("expected no errors, got: %v", errs)
	}

	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got %v", data)
	}
	repo, _ := d["repository"].(map[string]interface{})
	issue, _ := repo["issue"].(map[string]interface{})
	if issue == nil {
		t.Fatalf("expected issue in response, got %v", data)
	}

	// issueType → null
	if v, present := issue["issueType"]; !present || v != nil {
		t.Fatalf("expected issueType=null, got present=%v value=%v", present, v)
	}
	// parent → null
	if v, present := issue["parent"]; !present || v != nil {
		t.Fatalf("expected parent=null, got present=%v value=%v", present, v)
	}
	// subIssues → { nodes: [], totalCount: 0 }
	subIssues, _ := issue["subIssues"].(map[string]interface{})
	if subIssues == nil {
		t.Fatalf("expected subIssues object, got %v", issue["subIssues"])
	}
	if tc, _ := subIssues["totalCount"].(float64); tc != 0 {
		t.Fatalf("expected subIssues.totalCount=0, got %v", subIssues["totalCount"])
	}
	if nodes, ok := subIssues["nodes"].([]interface{}); !ok || len(nodes) != 0 {
		t.Fatalf("expected subIssues.nodes=[], got %v", subIssues["nodes"])
	}
	// subIssuesSummary → { total: 0, completed: 0, percentCompleted: 0 }
	summary, _ := issue["subIssuesSummary"].(map[string]interface{})
	if summary == nil {
		t.Fatalf("expected subIssuesSummary object, got %v", issue["subIssuesSummary"])
	}
	if tot, _ := summary["total"].(float64); tot != 0 {
		t.Fatalf("expected subIssuesSummary.total=0, got %v", summary["total"])
	}
	if comp, _ := summary["completed"].(float64); comp != 0 {
		t.Fatalf("expected subIssuesSummary.completed=0, got %v", summary["completed"])
	}
	if pct, _ := summary["percentCompleted"].(float64); pct != 0 {
		t.Fatalf("expected subIssuesSummary.percentCompleted=0, got %v", summary["percentCompleted"])
	}
}
