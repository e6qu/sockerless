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

func TestProjectsV2GraphQL_ProjectLevelConnections(t *testing.T) {
	owner, repoName := sweepRepo(t, "gql-project-v2-project-connections")
	issue := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+owner+"/"+repoName+"/issues", defaultToken, map[string]interface{}{
		"title": "project item",
	}), 201)
	issueID := int(issue["id"].(float64))
	issueNumber := int(issue["number"].(float64))
	admin := testServer.store.UsersByLogin["admin"]
	project := testServer.store.ProjectsV2.CreateProject(admin.ID, "User", "GraphQL project", admin.ID)
	testServer.store.ProjectsV2.AddItem(project.ID, "Issue", issueID, admin.ID)
	testServer.store.ProjectsV2.AddDraftItem(project.ID, "Draft item", "Body", admin.ID)
	stage := testServer.store.ProjectsV2.CreateField(project.ID, "Stage", ProjectV2FieldSingleSelect, []*ProjectV2SingleSelectOption{
		{Name: "Todo", Color: "GRAY", Description: "ready to schedule"},
		{Name: "Done", Color: "GREEN"},
	}, nil)
	sprint := testServer.store.ProjectsV2.CreateField(project.ID, "Sprint", ProjectV2FieldIteration, nil, &ProjectV2IterationConfiguration{
		StartDate: "2026-07-06",
		Duration:  14,
		Iterations: []*ProjectV2Iteration{
			{Title: "Sprint 1", StartDate: "2026-07-06", Duration: 14},
		},
	})
	filter := "is:issue"
	view := testServer.store.ProjectsV2.CreateView(project.ID, "Issues board", "board", &filter, []int{stage.ID, sprint.ID}, admin.ID)
	if view == nil {
		t.Fatal("CreateView returned nil")
	}

	query := `query($owner:String!,$name:String!,$number:Int!){
		repository(owner:$owner,name:$name){
			issue(number:$number){
				projectItems(first:10){
					nodes{
						project{
							fields(first:10){
								totalCount
								nodes{
									id
									name
									dataType
									options { id name color description }
									iterationConfiguration { startDate duration iterations { id title startDate duration } }
								}
							}
							views(first:10){
								totalCount
								nodes{ id number name layout filter visibleFieldIds }
							}
							items(first:10){
								totalCount
								nodes{ id }
								pageInfo{ hasNextPage hasPreviousPage startCursor endCursor }
							}
						}
					}
				}
			}
		}
	}`
	data := gqlData(t, query, map[string]interface{}{"owner": owner, "name": repoName, "number": issueNumber})
	projectNode := data["repository"].(map[string]interface{})["issue"].(map[string]interface{})["projectItems"].(map[string]interface{})["nodes"].([]interface{})[0].(map[string]interface{})["project"].(map[string]interface{})

	fields := projectNode["fields"].(map[string]interface{})
	if got := int(fields["totalCount"].(float64)); got != 2 {
		t.Fatalf("fields.totalCount = %d, want 2: %v", got, fields)
	}
	fieldNodes := fields["nodes"].([]interface{})
	firstField := fieldNodes[0].(map[string]interface{})
	if firstField["id"] != stage.NodeID || firstField["name"] != "Stage" || firstField["dataType"] != string(ProjectV2FieldSingleSelect) {
		t.Fatalf("first field = %v", firstField)
	}
	options := firstField["options"].([]interface{})
	if len(options) != 2 || options[0].(map[string]interface{})["name"] != "Todo" || options[0].(map[string]interface{})["description"] != "ready to schedule" {
		t.Fatalf("stage options = %v", options)
	}
	secondField := fieldNodes[1].(map[string]interface{})
	if secondField["id"] != sprint.NodeID || secondField["dataType"] != string(ProjectV2FieldIteration) {
		t.Fatalf("second field = %v", secondField)
	}
	iteration := secondField["iterationConfiguration"].(map[string]interface{})
	if iteration["startDate"] != "2026-07-06" || iteration["duration"].(float64) != 14 {
		t.Fatalf("iteration configuration = %v", iteration)
	}
	iterations := iteration["iterations"].([]interface{})
	if len(iterations) != 1 || iterations[0].(map[string]interface{})["title"] != "Sprint 1" {
		t.Fatalf("iterations = %v", iterations)
	}

	views := projectNode["views"].(map[string]interface{})
	if got := int(views["totalCount"].(float64)); got != 1 {
		t.Fatalf("views.totalCount = %d, want 1: %v", got, views)
	}
	viewNode := views["nodes"].([]interface{})[0].(map[string]interface{})
	if viewNode["id"] != view.NodeID || viewNode["name"] != "Issues board" || viewNode["layout"] != "board" || viewNode["filter"] != "is:issue" {
		t.Fatalf("view node = %v", viewNode)
	}
	visible := viewNode["visibleFieldIds"].([]interface{})
	if len(visible) != 2 || int(visible[0].(float64)) != stage.ID || int(visible[1].(float64)) != sprint.ID {
		t.Fatalf("visibleFieldIds = %v", visible)
	}

	items := projectNode["items"].(map[string]interface{})
	if got := int(items["totalCount"].(float64)); got != 2 {
		t.Fatalf("items.totalCount = %d, want 2: %v", got, items)
	}
	if got := len(items["nodes"].([]interface{})); got != 2 {
		t.Fatalf("items nodes len = %d, want 2: %v", got, items)
	}
	pageInfo := items["pageInfo"].(map[string]interface{})
	if pageInfo["hasNextPage"] != false || pageInfo["hasPreviousPage"] != false || pageInfo["startCursor"] == nil || pageInfo["endCursor"] == nil {
		t.Fatalf("items pageInfo = %v", pageInfo)
	}
}
