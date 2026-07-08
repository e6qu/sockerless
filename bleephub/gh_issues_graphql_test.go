package bleephub

import (
	"fmt"
	"testing"
)

// TestIssueGraphQL_SubIssueFields exercises the exact selections the gh CLI's
// `gh issue view` sends on `...on Issue` for issue-types and sub-issues.
// Sub-issue fields are backed by the same ordered issue links as the REST API.
func TestIssueGraphQL_SubIssueFields(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	parentID, parentNum := createIssueForTest(t, repo, "parent")
	openChildID, openChildNum := createIssueForTest(t, repo, "open child")
	closedChildID, closedChildNum := createIssueForTest(t, repo, "closed child")
	parentPath := fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d", repo, parentNum)
	requireStatus(t, ghPost(t, parentPath+"/sub_issues", defaultToken, map[string]interface{}{"sub_issue_id": openChildID}), 201)
	requireStatus(t, ghPost(t, parentPath+"/sub_issues", defaultToken, map[string]interface{}{"sub_issue_id": closedChildID}), 201)
	requireStatus(t, ghPatch(t, fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d", repo, closedChildNum), defaultToken, map[string]interface{}{"state": "closed"}), 200)

	// The exact selection set gh CLI's `gh issue view` sends for these four
	// fields on `...on Issue`.
	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			parentIssue: issue(number:$number){
				number
				stateReason
				issueType{id,name,description,color}
				parent{id,number,title,url,state,repository{nameWithOwner}}
				subIssues(first:100){nodes{id,number,title,url,state,repository{nameWithOwner}},totalCount}
				subIssuesSummary{total,completed,percentCompleted}
			}
			childIssue: issue(number:` + fmt.Sprintf("%d", openChildNum) + `){
				number
				parent{id,number,title,url,state,repository{nameWithOwner}}
				subIssues(first:100){nodes{id,number,title,url,state,repository{nameWithOwner}},totalCount}
				subIssuesSummary{total,completed,percentCompleted}
			}
		}
	}`

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": query,
		"variables": map[string]interface{}{
			"owner":  "admin",
			"name":   repo,
			"number": parentNum,
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if errs, ok := data["errors"]; ok && errs != nil {
		t.Fatalf("expected no errors, got: %v", errs)
	}

	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got %v", data)
	}
	gqlRepo, _ := d["repository"].(map[string]interface{})
	parentIssue, _ := gqlRepo["parentIssue"].(map[string]interface{})
	childIssue, _ := gqlRepo["childIssue"].(map[string]interface{})
	if parentIssue == nil || childIssue == nil {
		t.Fatalf("expected parent and child issue in response, got %v", data)
	}

	// issueType → null
	if v, present := parentIssue["issueType"]; !present || v != nil {
		t.Fatalf("expected issueType=null, got present=%v value=%v", present, v)
	}
	// parent → null for the top-level parent issue.
	if v, present := parentIssue["parent"]; !present || v != nil {
		t.Fatalf("expected parent=null, got present=%v value=%v", present, v)
	}
	subIssues, _ := parentIssue["subIssues"].(map[string]interface{})
	if subIssues == nil {
		t.Fatalf("expected subIssues object, got %v", parentIssue["subIssues"])
	}
	if tc, _ := subIssues["totalCount"].(float64); tc != 2 {
		t.Fatalf("expected subIssues.totalCount=2, got %v", subIssues["totalCount"])
	}
	nodes, ok := subIssues["nodes"].([]interface{})
	if !ok || len(nodes) != 2 {
		t.Fatalf("expected two subIssue nodes, got %v", subIssues["nodes"])
	}
	firstNode, _ := nodes[0].(map[string]interface{})
	secondNode, _ := nodes[1].(map[string]interface{})
	if int(firstNode["number"].(float64)) != openChildNum || int(secondNode["number"].(float64)) != closedChildNum {
		t.Fatalf("subIssue order = [%v %v], want [%d %d]", firstNode["number"], secondNode["number"], openChildNum, closedChildNum)
	}
	if firstNode["state"] != "OPEN" || secondNode["state"] != "CLOSED" {
		t.Fatalf("subIssue states = %v/%v", firstNode["state"], secondNode["state"])
	}
	if gotRepo := firstNode["repository"].(map[string]interface{})["nameWithOwner"]; gotRepo != "admin/"+repo {
		t.Fatalf("subIssue repository = %v", gotRepo)
	}
	summary, _ := parentIssue["subIssuesSummary"].(map[string]interface{})
	if summary == nil {
		t.Fatalf("expected subIssuesSummary object, got %v", parentIssue["subIssuesSummary"])
	}
	if tot, _ := summary["total"].(float64); tot != 2 {
		t.Fatalf("expected subIssuesSummary.total=2, got %v", summary["total"])
	}
	if comp, _ := summary["completed"].(float64); comp != 1 {
		t.Fatalf("expected subIssuesSummary.completed=1, got %v", summary["completed"])
	}
	if pct, _ := summary["percentCompleted"].(float64); pct != 50 {
		t.Fatalf("expected subIssuesSummary.percentCompleted=50, got %v", summary["percentCompleted"])
	}

	childParent, _ := childIssue["parent"].(map[string]interface{})
	if childParent == nil || int(childParent["number"].(float64)) != parentNum {
		t.Fatalf("child parent = %v, want parent #%d", childIssue["parent"], parentNum)
	}
	childSubIssues, _ := childIssue["subIssues"].(map[string]interface{})
	if childSubIssues == nil || int(childSubIssues["totalCount"].(float64)) != 0 {
		t.Fatalf("child subIssues = %v, want empty", childIssue["subIssues"])
	}
	if parentID == 0 {
		t.Fatal("parent ID must be non-zero")
	}
}

func TestIssueGraphQL_IssueTypeAssignment(t *testing.T) {
	org := createTestOrg(t)
	repoName, _ := createOrgRepoForGovernance(t, org)
	repoFullName := org + "/" + repoName
	createdType := decodeJSONWithStatus(t, ghPost(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken, map[string]interface{}{
		"name":        "Epic",
		"description": "Tracks a coordinated body of work",
		"is_enabled":  true,
		"color":       "purple",
	}), 200)
	typeID := int(createdType["id"].(float64))

	issue := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+repoFullName+"/issues", defaultToken, map[string]interface{}{
		"title":         "typed through REST",
		"issue_type_id": typeID,
	}), 201)
	number := int(issue["number"].(float64))

	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			issue(number:$number){
				number
				issueType{id,name,description,color}
			}
		}
	}`
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": query,
		"variables": map[string]interface{}{
			"owner":  org,
			"name":   repoName,
			"number": number,
		},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if errs, ok := data["errors"]; ok && errs != nil {
		t.Fatalf("expected no errors, got: %v", errs)
	}
	gqlData := data["data"].(map[string]interface{})
	gqlRepo := gqlData["repository"].(map[string]interface{})
	gqlIssue := gqlRepo["issue"].(map[string]interface{})
	gqlType := gqlIssue["issueType"].(map[string]interface{})
	if gqlType == nil || gqlType["id"] != createdType["node_id"] || gqlType["name"] != "Epic" || gqlType["color"] != "purple" {
		t.Fatalf("GraphQL issueType = %v", gqlIssue["issueType"])
	}
}
