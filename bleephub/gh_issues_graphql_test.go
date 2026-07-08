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

func TestIssueGraphQL_IssueCommentPinned(t *testing.T) {
	repo := createRepoWriteRepo(t, false)
	_, number := createIssueForTest(t, repo, "comment pin")
	comment := decodeJSONWithStatus(t, ghPost(t, fmt.Sprintf("/api/v3/repos/admin/%s/issues/%d/comments", repo, number), defaultToken, map[string]interface{}{
		"body": "pinned through REST",
	}), 201)
	commentID := int(comment["id"].(float64))
	requireStatus(t, ghPut(t, fmt.Sprintf("/api/v3/repos/admin/%s/issues/comments/%d/pin", repo, commentID), defaultToken, nil), 200)

	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			issue(number:$number){
				comments(first:10){
					nodes{id body isPinned isMinimized reactionGroups{content users{totalCount}}}
				}
			}
		}
	}`
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": query,
		"variables": map[string]interface{}{
			"owner":  "admin",
			"name":   repo,
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
	comments := gqlIssue["comments"].(map[string]interface{})
	nodes := comments["nodes"].([]interface{})
	if len(nodes) != 1 {
		t.Fatalf("comments nodes = %v, want one comment", nodes)
	}
	node := nodes[0].(map[string]interface{})
	if node["body"] != "pinned through REST" || node["isPinned"] != true {
		t.Fatalf("GraphQL pinned comment = %v", node)
	}
}

func TestIssueGraphQL_IssueFieldValues(t *testing.T) {
	org := createTestOrg(t)
	repoName, _ := createOrgRepoForGovernance(t, org)
	repoFullName := org + "/" + repoName
	issue := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+repoFullName+"/issues", defaultToken, map[string]interface{}{
		"title": "custom fields through REST",
	}), 201)
	number := int(issue["number"].(float64))

	mkField := func(body map[string]interface{}) int {
		t.Helper()
		created := decodeJSONWithStatus(t, ghPost(t, "/api/v3/orgs/"+org+"/issue-fields", defaultToken, body), 200)
		return int(created["id"].(float64))
	}
	textID := mkField(map[string]interface{}{"name": "Team notes", "data_type": "text", "visibility": "all"})
	numberID := mkField(map[string]interface{}{"name": "Story points", "data_type": "number", "visibility": "organization_members_only"})
	singleID := mkField(map[string]interface{}{
		"name": "Priority", "data_type": "single_select", "visibility": "all",
		"options": []map[string]interface{}{
			{"name": "High", "color": "red"},
			{"name": "Low", "color": "green"},
		},
	})
	multiID := mkField(map[string]interface{}{
		"name": "Areas", "data_type": "multi_select", "visibility": "all",
		"options": []map[string]interface{}{
			{"name": "backend", "color": "blue"},
			{"name": "frontend", "color": "pink"},
		},
	})
	dateID := mkField(map[string]interface{}{"name": "Due", "data_type": "date", "visibility": "all"})

	valuesResp := ghPost(t, "/api/v3/repos/"+repoFullName+"/issues/"+itoa(number)+"/issue-field-values", defaultToken, map[string]interface{}{
		"issue_field_values": []map[string]interface{}{
			{"field_id": textID, "value": "needs design review"},
			{"field_id": numberID, "value": 5},
			{"field_id": singleID, "value": "High"},
			{"field_id": multiID, "value": []string{"backend", "frontend"}},
			{"field_id": dateID, "value": "2030-12-31"},
		},
	})
	if valuesResp.StatusCode != 200 {
		valuesResp.Body.Close()
		t.Fatalf("set issue field values: %d", valuesResp.StatusCode)
	}
	if got := len(decodeJSONArray(t, valuesResp)); got != 5 {
		t.Fatalf("REST issue field values = %d, want 5", got)
	}

	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			issue(number:$number){
				issueFieldValues(first:10){
					totalCount
					nodes{
						__typename
						... on IssueFieldTextValue {
							textValue: value
							field { __typename ... on IssueFieldText { name dataType visibility } }
						}
						... on IssueFieldNumberValue {
							numberValue: value
							field { __typename ... on IssueFieldNumber { name dataType visibility } }
						}
						... on IssueFieldSingleSelectValue {
							singleValue: value name color optionId
							field { __typename ... on IssueFieldSingleSelect { name dataType visibility options { name color } } }
						}
						... on IssueFieldMultiSelectValue {
							multiValue: value
							options { name color }
							field { __typename ... on IssueFieldMultiSelect { name dataType visibility options { name color } } }
						}
						... on IssueFieldDateValue {
							dateValue: value
							field { __typename ... on IssueFieldDate { name dataType visibility } }
						}
					}
				}
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
	values := data["data"].(map[string]interface{})["repository"].(map[string]interface{})["issue"].(map[string]interface{})["issueFieldValues"].(map[string]interface{})
	if got := int(values["totalCount"].(float64)); got != 5 {
		t.Fatalf("issueFieldValues.totalCount = %d, want 5: %v", got, values)
	}
	byType := map[string]map[string]interface{}{}
	for _, raw := range values["nodes"].([]interface{}) {
		node := raw.(map[string]interface{})
		byType[node["__typename"].(string)] = node
	}
	if got := byType["IssueFieldTextValue"]["textValue"]; got != "needs design review" {
		t.Fatalf("text value = %v", byType["IssueFieldTextValue"])
	}
	if got := byType["IssueFieldNumberValue"]["numberValue"].(float64); got != 5 {
		t.Fatalf("number value = %v", byType["IssueFieldNumberValue"])
	}
	single := byType["IssueFieldSingleSelectValue"]
	if single["singleValue"] != "High" || single["name"] != "High" || single["color"] != "RED" || single["optionId"] == "" {
		t.Fatalf("single-select value = %v", single)
	}
	singleField := single["field"].(map[string]interface{})
	if singleField["name"] != "Priority" || singleField["dataType"] != "SINGLE_SELECT" || singleField["visibility"] != "ALL" {
		t.Fatalf("single-select field = %v", singleField)
	}
	multi := byType["IssueFieldMultiSelectValue"]
	if multi["multiValue"] != nil {
		t.Fatalf("multi-select value = %v, want null", multi["multiValue"])
	}
	if opts := multi["options"].([]interface{}); len(opts) != 2 {
		t.Fatalf("multi-select options = %v", multi["options"])
	}
	if got := byType["IssueFieldDateValue"]["dateValue"]; got != "2030-12-31" {
		t.Fatalf("date value = %v", byType["IssueFieldDateValue"])
	}
	numberField := byType["IssueFieldNumberValue"]["field"].(map[string]interface{})
	if numberField["visibility"] != "ORG_ONLY" {
		t.Fatalf("number field visibility = %v", numberField)
	}
}
