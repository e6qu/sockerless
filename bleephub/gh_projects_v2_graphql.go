package bleephub

import (
	"fmt"

	"github.com/graphql-go/graphql"
)

// addProjectV2MutationsToSchema registers the ProjectV2 GraphQL mutations
// gh CLI's `gh project create` + `gh project item-add` use:
//   - createProjectV2(input{ownerId, title}) → ProjectV2
//   - addProjectV2ItemById(input{projectId, contentId}) → ProjectV2Item
//   - createProjectV2Field(input{projectId, dataType, name}) → ProjectV2Field
//   - updateProjectV2ItemFieldValue(input{projectId,itemId,fieldId,value}) → ProjectV2Item
func (s *Server) addProjectV2MutationsToSchema(mutationType *graphql.Object) {
	projectV2Type := projectV2GraphQLTypes()

	// createProjectV2
	createProjectInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateProjectV2Input",
		Fields: graphql.InputObjectConfigFieldMap{
			"ownerId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"title":   &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	createProjectPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CreateProjectV2Payload",
		Fields: graphql.Fields{
			"projectV2": &graphql.Field{Type: projectV2Type},
		},
	})

	mutationType.AddFieldConfig("createProjectV2", &graphql.Field{
		Type: createProjectPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createProjectInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			ownerNodeID, _ := input["ownerId"].(string)
			title, _ := input["title"].(string)

			ownerID, ownerType, ok := resolveProjectOwner(s.store, ownerNodeID)
			if !ok {
				return nil, fmt.Errorf("could not resolve to an owner with the global id of '%s'", ownerNodeID)
			}
			proj := s.store.ProjectsV2.CreateProject(ownerID, ownerType, title, user.ID)
			return map[string]interface{}{
				"projectV2": projectV2ToGQL(proj, s.store),
			}, nil
		},
	})

	// addProjectV2ItemById
	addItemInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "AddProjectV2ItemByIdInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"projectId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"contentId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})
	addItemPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AddProjectV2ItemByIdPayload",
		Fields: graphql.Fields{
			"item": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "AddProjectV2ItemByIdPayloadItem",
					Fields: graphql.Fields{
						"id": &graphql.Field{
							Type: graphql.NewNonNull(graphql.ID),
							Resolve: func(p graphql.ResolveParams) (interface{}, error) {
								src, ok := p.Source.(map[string]interface{})
								if !ok {
									return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
								}
								return src["nodeID"], nil
							},
						},
					},
				}),
			},
		},
	})

	mutationType.AddFieldConfig("addProjectV2ItemById", &graphql.Field{
		Type: addItemPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(addItemInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			projectNodeID, _ := input["projectId"].(string)
			contentNodeID, _ := input["contentId"].(string)

			proj := s.store.ProjectsV2.LookupProjectByNodeID(projectNodeID)
			if proj == nil {
				return nil, fmt.Errorf("could not resolve to a project with the global id of '%s'", projectNodeID)
			}
			contentType, contentID, ok := resolveContentByNodeID(s.store, contentNodeID)
			if !ok {
				return nil, fmt.Errorf("could not resolve to an issue or pull request with the global id of '%s'", contentNodeID)
			}
			item := s.store.ProjectsV2.AddItem(proj.ID, contentType, contentID, user.ID)
			return map[string]interface{}{
				"item": map[string]interface{}{
					"nodeID": item.NodeID,
				},
			}, nil
		},
	})

	// --- createProjectV2Field ---

	dataTypeEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "ProjectV2CustomFieldType",
		Values: graphql.EnumValueConfigMap{
			"SINGLE_SELECT": &graphql.EnumValueConfig{Value: "SINGLE_SELECT"},
			"TEXT":          &graphql.EnumValueConfig{Value: "TEXT"},
			"NUMBER":        &graphql.EnumValueConfig{Value: "NUMBER"},
			"DATE":          &graphql.EnumValueConfig{Value: "DATE"},
			"ITERATION":     &graphql.EnumValueConfig{Value: "ITERATION"},
		},
	})

	singleSelectOptionInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ProjectV2SingleSelectFieldOptionInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	iterationInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ProjectV2IterationInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"title":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"startDate": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"duration":  &graphql.InputObjectFieldConfig{Type: graphql.Int},
		},
	})
	iterationConfigInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ProjectV2IterationConfigurationInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"startDate":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"duration":   &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Int)},
			"iterations": &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(iterationInputType))},
		},
	})

	createFieldInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateProjectV2FieldInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"projectId":              &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"dataType":               &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(dataTypeEnum)},
			"name":                   &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"singleSelectOptions":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(singleSelectOptionInputType))},
			"iterationConfiguration": &graphql.InputObjectFieldConfig{Type: iterationConfigInputType},
		},
	})

	projectV2FieldType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2FieldSummary",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["nodeID"], nil
				},
			},
			"name":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"dataType": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	createFieldPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CreateProjectV2FieldPayload",
		Fields: graphql.Fields{
			"projectV2Field": &graphql.Field{Type: projectV2FieldType},
		},
	})

	mutationType.AddFieldConfig("createProjectV2Field", &graphql.Field{
		Type: createFieldPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createFieldInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			projectNodeID, _ := input["projectId"].(string)
			name, _ := input["name"].(string)
			dataType, _ := input["dataType"].(string)
			rawOptions, _ := input["singleSelectOptions"].([]interface{})
			rawIteration, _ := input["iterationConfiguration"].(map[string]interface{})
			options := make([]*ProjectV2SingleSelectOption, 0, len(rawOptions))
			for _, raw := range rawOptions {
				m, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				n, _ := m["name"].(string)
				options = append(options, &ProjectV2SingleSelectOption{Name: n})
			}
			var iteration *ProjectV2IterationConfiguration
			if rawIteration != nil {
				startDate, _ := rawIteration["startDate"].(string)
				duration, _ := rawIteration["duration"].(int)
				iteration = &ProjectV2IterationConfiguration{StartDate: startDate, Duration: duration}
				rawIterations, _ := rawIteration["iterations"].([]interface{})
				for _, raw := range rawIterations {
					m, ok := raw.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("iterationConfiguration.iterations contains an invalid item")
					}
					title, _ := m["title"].(string)
					start, _ := m["startDate"].(string)
					iterDuration := duration
					if d, ok := m["duration"].(int); ok && d > 0 {
						iterDuration = d
					}
					iteration.Iterations = append(iteration.Iterations, &ProjectV2Iteration{
						Title:     title,
						StartDate: start,
						Duration:  iterDuration,
					})
				}
			}

			proj := s.store.ProjectsV2.LookupProjectByNodeID(projectNodeID)
			if proj == nil {
				return nil, fmt.Errorf("could not resolve to a project with the global id of '%s'", projectNodeID)
			}
			if dataType == string(ProjectV2FieldIteration) && iteration == nil {
				return nil, fmt.Errorf("iterationConfiguration is required for ITERATION fields")
			}
			field := s.store.ProjectsV2.CreateField(proj.ID, name, ProjectV2FieldDataType(dataType), options, iteration)
			return map[string]interface{}{
				"projectV2Field": map[string]interface{}{
					"nodeID":   field.NodeID,
					"name":     field.Name,
					"dataType": string(field.DataType),
				},
			}, nil
		},
	})

	// --- updateProjectV2ItemFieldValue ---

	fieldValueInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ProjectV2FieldValueInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"singleSelectOptionId": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"text":                 &graphql.InputObjectFieldConfig{Type: graphql.String},
			"number":               &graphql.InputObjectFieldConfig{Type: graphql.Float},
			"date":                 &graphql.InputObjectFieldConfig{Type: graphql.String},
			"iterationId":          &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})
	updateValueInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpdateProjectV2ItemFieldValueInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"projectId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"itemId":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"fieldId":   &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"value":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(fieldValueInputType)},
		},
	})

	updateValuePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UpdateProjectV2ItemFieldValuePayload",
		Fields: graphql.Fields{
			"projectV2Item": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "UpdateProjectV2ItemFieldValuePayloadItem",
					Fields: graphql.Fields{
						"id": &graphql.Field{
							Type: graphql.NewNonNull(graphql.ID),
							Resolve: func(p graphql.ResolveParams) (interface{}, error) {
								src, ok := p.Source.(map[string]interface{})
								if !ok {
									return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
								}
								return src["nodeID"], nil
							},
						},
					},
				}),
			},
		},
	})

	mutationType.AddFieldConfig("updateProjectV2ItemFieldValue", &graphql.Field{
		Type: updateValuePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(updateValueInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			projectNodeID, _ := input["projectId"].(string)
			itemNodeID, _ := input["itemId"].(string)
			fieldNodeID, _ := input["fieldId"].(string)
			value, _ := input["value"].(map[string]interface{})

			proj := s.store.ProjectsV2.LookupProjectByNodeID(projectNodeID)
			if proj == nil {
				return nil, fmt.Errorf("could not resolve to a project with the global id of '%s'", projectNodeID)
			}
			item := s.store.ProjectsV2.LookupItemByNodeID(itemNodeID)
			if item == nil {
				return nil, fmt.Errorf("could not resolve to an item with the global id of '%s'", itemNodeID)
			}
			if item.ProjectID != proj.ID {
				return nil, fmt.Errorf("project does not contain item with the global id of '%s'", itemNodeID)
			}
			field := s.store.ProjectsV2.LookupFieldByNodeID(fieldNodeID)
			if field == nil {
				return nil, fmt.Errorf("could not resolve to a field with the global id of '%s'", fieldNodeID)
			}
			if field.ProjectID != proj.ID {
				return nil, fmt.Errorf("field does not belong to project with the global id of '%s'", projectNodeID)
			}
			fieldValue, err := projectV2GraphQLFieldValueInput(field, value)
			if err != nil {
				return nil, err
			}
			if err := s.store.ProjectsV2.SetFieldValueAny(item.ID, field.ID, fieldValue); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"projectV2Item": map[string]interface{}{"nodeID": item.NodeID},
			}, nil
		},
	})
}

func projectV2GraphQLFieldValueInput(field *ProjectV2Field, value map[string]interface{}) (interface{}, error) {
	if value == nil {
		return nil, fmt.Errorf("value is required")
	}
	type candidate struct {
		name  string
		value interface{}
	}
	candidates := make([]candidate, 0, 5)
	for _, key := range []string{"singleSelectOptionId", "text", "number", "date", "iterationId"} {
		v, ok := value[key]
		if !ok || v == nil {
			continue
		}
		candidates = append(candidates, candidate{name: key, value: v})
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("exactly one field value must be provided")
	}
	got := candidates[0]
	want := map[ProjectV2FieldDataType]string{
		ProjectV2FieldSingleSelect: "singleSelectOptionId",
		ProjectV2FieldText:         "text",
		ProjectV2FieldNumber:       "number",
		ProjectV2FieldDate:         "date",
		ProjectV2FieldIteration:    "iterationId",
	}[field.DataType]
	if got.name != want {
		return nil, fmt.Errorf("field %q expects %s", field.Name, want)
	}
	if field.DataType == ProjectV2FieldNumber {
		switch n := got.value.(type) {
		case float64:
			return n, nil
		case float32:
			return float64(n), nil
		case int:
			return float64(n), nil
		case int32:
			return float64(n), nil
		case int64:
			return float64(n), nil
		}
	}
	return got.value, nil
}

// resolveProjectOwner maps a GraphQL node ID to (ownerID, ownerType).
// Supports User + Organization nodes.
func resolveProjectOwner(st *Store, nodeID string) (int, string, bool) {
	if nodeID != "" {
		st.mu.RLock()
		for _, u := range st.Users {
			if u.NodeID == nodeID {
				st.mu.RUnlock()
				return u.ID, "User", true
			}
		}
		for _, org := range st.Orgs {
			if org.NodeID == nodeID {
				st.mu.RUnlock()
				return org.ID, "Organization", true
			}
		}
		st.mu.RUnlock()
	}
	return 0, "", false
}

// resolveContentByNodeID maps a GraphQL node ID to either an Issue or
// PullRequest. Returns (contentType, contentID, ok).
func resolveContentByNodeID(st *Store, nodeID string) (string, int, bool) {
	if issue := findIssueByNodeID(st, nodeID); issue != nil {
		return "Issue", issue.ID, true
	}
	if pr := findPullRequestByNodeID(st, nodeID); pr != nil {
		return "PullRequest", pr.ID, true
	}
	return "", 0, false
}
