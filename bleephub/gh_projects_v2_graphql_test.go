package bleephub

import "testing"

func TestProjectsV2GraphQL_FieldValueKinds(t *testing.T) {
	owner, repoName := sweepRepo(t, "gql-project-v2-fields")
	issue := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+owner+"/"+repoName+"/issues", defaultToken, map[string]interface{}{
		"title": "project item",
	}), 201)
	issueNumber := int(issue["number"].(float64))
	repo := testServer.store.GetRepo(owner, repoName)
	admin := testServer.store.UsersByLogin["admin"]
	project := testServer.store.ProjectsV2.CreateProject(admin.ID, "User", "GraphQL fields", admin.ID)
	item := testServer.store.ProjectsV2.AddItem(project.ID, "Issue", int(issue["id"].(float64)), admin.ID)

	textField := testServer.store.ProjectsV2.CreateField(project.ID, "Notes", ProjectV2FieldText, nil, nil)
	numberField := testServer.store.ProjectsV2.CreateField(project.ID, "Effort", ProjectV2FieldNumber, nil, nil)
	dateField := testServer.store.ProjectsV2.CreateField(project.ID, "Due", ProjectV2FieldDate, nil, nil)
	selectField := testServer.store.ProjectsV2.CreateField(project.ID, "Priority", ProjectV2FieldSingleSelect, []*ProjectV2SingleSelectOption{
		{Name: "High", Color: "RED"},
		{Name: "Low", Color: "GREEN"},
	}, nil)
	iterationField := testServer.store.ProjectsV2.CreateField(project.ID, "Sprint", ProjectV2FieldIteration, nil, &ProjectV2IterationConfiguration{
		StartDate: "2026-07-06",
		Duration:  7,
		Iterations: []*ProjectV2Iteration{
			{Title: "Sprint 1", StartDate: "2026-07-06", Duration: 7},
			{Title: "Sprint 2", StartDate: "2026-07-13", Duration: 7},
		},
	})

	update := func(field *ProjectV2Field, value map[string]interface{}) {
		t.Helper()
		data := gqlData(t, `mutation($project:ID!,$item:ID!,$field:ID!,$value:ProjectV2FieldValueInput!){
			updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field,value:$value}){
				projectV2Item { id }
			}
		}`, map[string]interface{}{
			"project": project.NodeID,
			"item":    item.NodeID,
			"field":   field.NodeID,
			"value":   value,
		})
		got := data["updateProjectV2ItemFieldValue"].(map[string]interface{})["projectV2Item"].(map[string]interface{})["id"]
		if got != item.NodeID {
			t.Fatalf("updated item id = %v, want %s", got, item.NodeID)
		}
	}
	update(textField, map[string]interface{}{"text": "ready"})
	update(numberField, map[string]interface{}{"number": 8})
	update(dateField, map[string]interface{}{"date": "2030-12-31"})
	update(selectField, map[string]interface{}{"singleSelectOptionId": selectField.Options[0].ID})
	update(iterationField, map[string]interface{}{"iterationId": iterationField.Iteration.Iterations[1].ID})

	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			issue(number:$number){
				projectItems(first:10){
					totalCount
					nodes{
						notes: fieldValueByName(name:"Notes"){ __typename ... on ProjectV2ItemFieldTextValue { text } }
						effort: fieldValueByName(name:"Effort"){ __typename ... on ProjectV2ItemFieldNumberValue { number } }
						due: fieldValueByName(name:"Due"){ __typename ... on ProjectV2ItemFieldDateValue { date } }
						priority: fieldValueByName(name:"Priority"){ __typename ... on ProjectV2ItemFieldSingleSelectValue { optionId name } }
						sprint: fieldValueByName(name:"Sprint"){ __typename ... on ProjectV2ItemFieldIterationValue { iterationId title startDate duration } }
					}
				}
			}
		}
	}`
	data := gqlData(t, query, map[string]interface{}{"owner": owner, "name": repoName, "number": issueNumber})
	items := data["repository"].(map[string]interface{})["issue"].(map[string]interface{})["projectItems"].(map[string]interface{})
	if got := int(items["totalCount"].(float64)); got != 1 {
		t.Fatalf("projectItems.totalCount = %d, want 1: %v", got, items)
	}
	node := items["nodes"].([]interface{})[0].(map[string]interface{})
	if got := node["notes"].(map[string]interface{}); got["__typename"] != "ProjectV2ItemFieldTextValue" || got["text"] != "ready" {
		t.Fatalf("notes value = %v", got)
	}
	if got := node["effort"].(map[string]interface{}); got["__typename"] != "ProjectV2ItemFieldNumberValue" || got["number"].(float64) != 8 {
		t.Fatalf("effort value = %v", got)
	}
	if got := node["due"].(map[string]interface{}); got["__typename"] != "ProjectV2ItemFieldDateValue" || got["date"] != "2030-12-31" {
		t.Fatalf("due value = %v", got)
	}
	if got := node["priority"].(map[string]interface{}); got["__typename"] != "ProjectV2ItemFieldSingleSelectValue" || got["optionId"] != selectField.Options[0].ID || got["name"] != "High" {
		t.Fatalf("priority value = %v", got)
	}
	if got := node["sprint"].(map[string]interface{}); got["__typename"] != "ProjectV2ItemFieldIterationValue" || got["iterationId"] != iterationField.Iteration.Iterations[1].ID || got["title"] != "Sprint 2" || got["startDate"] != "2026-07-13" || got["duration"].(float64) != 7 {
		t.Fatalf("sprint value = %v", got)
	}

	resp := gqlDo(t, `mutation($project:ID!,$item:ID!,$field:ID!){
		updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field,value:{text:"wrong"}}){
			projectV2Item { id }
		}
	}`, map[string]interface{}{"project": project.NodeID, "item": item.NodeID, "field": numberField.NodeID})
	if errs, ok := resp["errors"]; !ok || errs == nil {
		t.Fatalf("wrong value kind unexpectedly succeeded: %v", resp)
	}
	if repo == nil {
		t.Fatal("repo disappeared during Projects v2 GraphQL test")
	}
}
