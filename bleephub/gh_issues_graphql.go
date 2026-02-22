package bleephub

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
)

// addIssueFieldsToSchema adds Issue types, queries, and mutations to the schema.
func (s *Server) addIssueFieldsToSchema(userType, repoType, mutationType, queryType *graphql.Object) *graphql.Object {
	// --- Label types ---
	issueLabelType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Label",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					l := p.Source.(map[string]interface{})
					return l["nodeID"], nil
				},
			},
			"name":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description": &graphql.Field{Type: graphql.String},
			"color":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	labelPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LabelPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	labelConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LabelConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(issueLabelType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(labelPageInfoType)},
		},
	})

	// --- Milestone type ---
	issueMilestoneType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Milestone",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					m := p.Source.(map[string]interface{})
					return m["nodeID"], nil
				},
			},
			"number":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description": &graphql.Field{Type: graphql.String},
			"state":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"dueOn":       &graphql.Field{Type: graphql.String},
		},
	})

	milestonePageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MilestonePageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	milestoneConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MilestoneConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(issueMilestoneType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(milestonePageInfoType)},
		},
	})

	// --- Reaction group type (static) ---
	reactionGroupType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReactionGroup",
		Fields: graphql.Fields{
			"content": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"users": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "ReactingUserConnection",
					Fields: graphql.Fields{
						"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
					},
				}),
			},
		},
	})

	// --- Comment types ---
	issueCommentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueComment",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c := p.Source.(map[string]interface{})
					return c["nodeID"], nil
				},
			},
			"body":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"url":       &graphql.Field{Type: graphql.String},
			"createdAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c := p.Source.(map[string]interface{})
					return c["author"], nil
				},
			},
			"authorAssociation": &graphql.Field{Type: graphql.String},
		},
	})

	commentPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueCommentPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	commentConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueCommentConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(issueCommentType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(commentPageInfoType)},
		},
	})

	// --- Assignee connection ---
	assigneePageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AssigneePageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	assigneeConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UserConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(userType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(assigneePageInfoType)},
		},
	})

	// --- Issue type ---
	issueType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Issue",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i := p.Source.(map[string]interface{})
					return i["nodeID"], nil
				},
			},
			"databaseId":  &graphql.Field{Type: graphql.Int},
			"number":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"body":        &graphql.Field{Type: graphql.String},
			"state":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"stateReason": &graphql.Field{Type: graphql.String},
			"url":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"createdAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"closedAt":    &graphql.Field{Type: graphql.String},
			"isPinned":    &graphql.Field{Type: graphql.Boolean},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i := p.Source.(map[string]interface{})
					return i["author"], nil
				},
			},
			"labels": &graphql.Field{
				Type: labelConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i := p.Source.(map[string]interface{})
					return i["labels"], nil
				},
			},
			"assignees": &graphql.Field{
				Type: assigneeConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i := p.Source.(map[string]interface{})
					return i["assignees"], nil
				},
			},
			"milestone": &graphql.Field{
				Type: issueMilestoneType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i := p.Source.(map[string]interface{})
					return i["milestone"], nil
				},
			},
			"comments": &graphql.Field{
				Type: commentConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"last":  &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i := p.Source.(map[string]interface{})
					return i["comments"], nil
				},
			},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(reactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return staticReactionGroups(), nil
				},
			},
		},
	})

	// --- Issue connection ---
	issuePageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssuePageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	issueEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueEdge",
		Fields: graphql.Fields{
			"node":   &graphql.Field{Type: issueType},
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	issueConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(issueType)},
			"edges":      &graphql.Field{Type: graphql.NewList(issueEdgeType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(issuePageInfoType)},
		},
	})

	// --- IssueState enum ---
	issueStateEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueState",
		Values: graphql.EnumValueConfigMap{
			"OPEN":   &graphql.EnumValueConfig{Value: "OPEN"},
			"CLOSED": &graphql.EnumValueConfig{Value: "CLOSED"},
		},
	})

	issueClosedStateReasonEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueClosedStateReason",
		Values: graphql.EnumValueConfigMap{
			"COMPLETED":   &graphql.EnumValueConfig{Value: "COMPLETED"},
			"NOT_PLANNED": &graphql.EnumValueConfig{Value: "NOT_PLANNED"},
		},
	})

	// --- Milestone state enum ---
	milestoneStateEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "MilestoneState",
		Values: graphql.EnumValueConfigMap{
			"OPEN":   &graphql.EnumValueConfig{Value: "OPEN"},
			"CLOSED": &graphql.EnumValueConfig{Value: "CLOSED"},
		},
	})

	// --- Issue filters input ---
	issueFiltersInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "IssueFilters",
		Fields: graphql.InputObjectConfigFieldMap{
			"assignee":  &graphql.InputObjectFieldConfig{Type: graphql.String},
			"createdBy": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"mentioned": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"labels":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.String)},
			"states":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(issueStateEnum)},
		},
	})

	// --- Add fields to Repository type ---

	repoType.AddFieldConfig("hasIssuesEnabled", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return true, nil
		},
	})

	repoType.AddFieldConfig("viewerPermission", &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return "ADMIN", nil
		},
	})

	repoType.AddFieldConfig("mergeCommitAllowed", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return true, nil
		},
	})

	repoType.AddFieldConfig("rebaseMergeAllowed", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return true, nil
		},
	})

	repoType.AddFieldConfig("squashMergeAllowed", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return true, nil
		},
	})

	repoType.AddFieldConfig("issues", &graphql.Field{
		Type: issueConnectionType,
		Args: graphql.FieldConfigArgument{
			"first":    &graphql.ArgumentConfig{Type: graphql.Int},
			"after":    &graphql.ArgumentConfig{Type: graphql.String},
			"states":   &graphql.ArgumentConfig{Type: graphql.NewList(issueStateEnum)},
			"labels":   &graphql.ArgumentConfig{Type: graphql.NewList(graphql.String)},
			"filterBy": &graphql.ArgumentConfig{Type: issueFiltersInput},
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "IssueOrder",
				Fields: graphql.InputObjectConfigFieldMap{
					"field":     &graphql.InputObjectFieldConfig{Type: graphql.String},
					"direction": &graphql.InputObjectFieldConfig{Type: graphql.String},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)

			issues := s.store.ListIssues(repoID, "")

			// Filter by states arg
			if states, ok := p.Args["states"].([]interface{}); ok && len(states) > 0 {
				stateMap := make(map[string]bool)
				for _, st := range states {
					stateMap[fmt.Sprintf("%v", st)] = true
				}
				var filtered []*Issue
				for _, i := range issues {
					if stateMap[i.State] {
						filtered = append(filtered, i)
					}
				}
				issues = filtered
			}

			// Filter by labels arg
			if labelNames, ok := p.Args["labels"].([]interface{}); ok && len(labelNames) > 0 {
				var names []string
				for _, ln := range labelNames {
					names = append(names, fmt.Sprintf("%v", ln))
				}
				var filtered []*Issue
				for _, i := range issues {
					if issueHasAllLabels(s.store, i, names, repoID) {
						filtered = append(filtered, i)
					}
				}
				issues = filtered
			}

			// Filter by filterBy
			if filterBy, ok := p.Args["filterBy"].(map[string]interface{}); ok {
				if assignee, ok := filterBy["assignee"].(string); ok && assignee != "" {
					u := s.store.LookupUserByLogin(assignee)
					if u != nil {
						var filtered []*Issue
						for _, i := range issues {
							for _, aid := range i.AssigneeIDs {
								if aid == u.ID {
									filtered = append(filtered, i)
									break
								}
							}
						}
						issues = filtered
					}
				}
			}

			// Sort newest first
			sort.Slice(issues, func(a, b int) bool {
				return issues[a].CreatedAt.After(issues[b].CreatedAt)
			})

			first := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				first = f
			}
			after, _ := p.Args["after"].(string)

			return paginateIssuesGQL(issues, s.store, first, after), nil
		},
	})

	repoType.AddFieldConfig("issue", &graphql.Field{
		Type: issueType,
		Args: graphql.FieldConfigArgument{
			"number": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)
			number, _ := p.Args["number"].(int)

			issue := s.store.GetIssueByNumber(repoID, number)
			if issue == nil {
				return nil, nil
			}
			return issueToGQL(issue, s.store), nil
		},
	})

	// issueOrPullRequest — gh issue view uses this
	repoType.AddFieldConfig("issueOrPullRequest", &graphql.Field{
		Type: issueType,
		Args: graphql.FieldConfigArgument{
			"number": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)
			number, _ := p.Args["number"].(int)

			issue := s.store.GetIssueByNumber(repoID, number)
			if issue == nil {
				return nil, nil
			}
			result := issueToGQL(issue, s.store)
			result["__typename"] = "Issue"
			return result, nil
		},
	})

	repoType.AddFieldConfig("labels", &graphql.Field{
		Type: labelConnectionType,
		Args: graphql.FieldConfigArgument{
			"first":   &graphql.ArgumentConfig{Type: graphql.Int},
			"after":   &graphql.ArgumentConfig{Type: graphql.String},
			"query":   &graphql.ArgumentConfig{Type: graphql.String},
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "LabelOrder",
				Fields: graphql.InputObjectConfigFieldMap{
					"field":     &graphql.InputObjectFieldConfig{Type: graphql.String},
					"direction": &graphql.InputObjectFieldConfig{Type: graphql.String},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)

			labels := s.store.ListLabels(repoID)

			// Filter by query
			if q, ok := p.Args["query"].(string); ok && q != "" {
				q = strings.ToLower(q)
				var filtered []*IssueLabel
				for _, l := range labels {
					if strings.Contains(strings.ToLower(l.Name), q) {
						filtered = append(filtered, l)
					}
				}
				labels = filtered
			}

			nodes := make([]map[string]interface{}, 0, len(labels))
			for _, l := range labels {
				nodes = append(nodes, labelToGQL(l))
			}

			return map[string]interface{}{
				"nodes":      nodes,
				"totalCount": len(nodes),
				"pageInfo": map[string]interface{}{
					"hasNextPage":     false,
					"hasPreviousPage": false,
					"startCursor":     nil,
					"endCursor":       nil,
				},
			}, nil
		},
	})

	repoType.AddFieldConfig("milestones", &graphql.Field{
		Type: milestoneConnectionType,
		Args: graphql.FieldConfigArgument{
			"first":  &graphql.ArgumentConfig{Type: graphql.Int},
			"after":  &graphql.ArgumentConfig{Type: graphql.String},
			"states": &graphql.ArgumentConfig{Type: graphql.NewList(milestoneStateEnum)},
			"query":  &graphql.ArgumentConfig{Type: graphql.String},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)

			state := ""
			if states, ok := p.Args["states"].([]interface{}); ok && len(states) > 0 {
				// Use first state as filter (or "all" if multiple)
				if len(states) == 1 {
					state = strings.ToLower(fmt.Sprintf("%v", states[0]))
				}
			}

			milestones := s.store.ListMilestones(repoID, state)

			// Filter by query
			if q, ok := p.Args["query"].(string); ok && q != "" {
				q = strings.ToLower(q)
				var filtered []*Milestone
				for _, ms := range milestones {
					if strings.Contains(strings.ToLower(ms.Title), q) {
						filtered = append(filtered, ms)
					}
				}
				milestones = filtered
			}

			nodes := make([]map[string]interface{}, 0, len(milestones))
			for _, ms := range milestones {
				nodes = append(nodes, milestoneToGQL(ms))
			}

			return map[string]interface{}{
				"nodes":      nodes,
				"totalCount": len(nodes),
				"pageInfo": map[string]interface{}{
					"hasNextPage":     false,
					"hasPreviousPage": false,
					"startCursor":     nil,
					"endCursor":       nil,
				},
			}, nil
		},
	})

	repoType.AddFieldConfig("assignableUsers", &graphql.Field{
		Type: assigneeConnectionType,
		Args: graphql.FieldConfigArgument{
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
			"after": &graphql.ArgumentConfig{Type: graphql.String},
			"query": &graphql.ArgumentConfig{Type: graphql.String},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			ownerMap, _ := repo["owner"].(map[string]interface{})
			ownerLogin, _ := ownerMap["login"].(string)

			// Return all users (for simplicity — real GH returns org members or repo collaborators)
			s.store.mu.RLock()
			var users []*User
			for _, u := range s.store.Users {
				users = append(users, u)
			}
			s.store.mu.RUnlock()

			// Filter by query
			if q, ok := p.Args["query"].(string); ok && q != "" {
				_ = ownerLogin // suppress unused
				q = strings.ToLower(q)
				var filtered []*User
				for _, u := range users {
					if strings.Contains(strings.ToLower(u.Login), q) || strings.Contains(strings.ToLower(u.Name), q) {
						filtered = append(filtered, u)
					}
				}
				users = filtered
			}

			nodes := make([]map[string]interface{}, 0, len(users))
			for _, u := range users {
				nodes = append(nodes, userToGraphQL(u))
			}

			return map[string]interface{}{
				"nodes":      nodes,
				"totalCount": len(nodes),
				"pageInfo": map[string]interface{}{
					"hasNextPage":     false,
					"hasPreviousPage": false,
					"startCursor":     nil,
					"endCursor":       nil,
				},
			}, nil
		},
	})

	// --- Mutations ---

	createIssueInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateIssueInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"repositoryId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"title":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"body":         &graphql.InputObjectFieldConfig{Type: graphql.String},
			"labelIds":     &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
			"milestoneId":  &graphql.InputObjectFieldConfig{Type: graphql.ID},
			"assigneeIds":  &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
		},
	})

	createIssuePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CreateIssuePayload",
		Fields: graphql.Fields{
			"issue": &graphql.Field{Type: issueType},
		},
	})

	mutationType.AddFieldConfig("createIssue", &graphql.Field{
		Type: createIssuePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createIssueInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			repoNodeID, _ := input["repositoryId"].(string)
			title, _ := input["title"].(string)
			body, _ := input["body"].(string)

			repo := findRepoByNodeID(s.store, repoNodeID)
			if repo == nil {
				return nil, fmt.Errorf("could not resolve to a Repository with the global id of '%s'", repoNodeID)
			}

			// Resolve label node IDs to store IDs
			var labelIDs []int
			if rawLabels, ok := input["labelIds"].([]interface{}); ok {
				for _, raw := range rawLabels {
					nodeID := fmt.Sprintf("%v", raw)
					if l := findLabelByNodeID(s.store, nodeID); l != nil {
						labelIDs = append(labelIDs, l.ID)
					}
				}
			}

			// Resolve assignee node IDs to store IDs
			var assigneeIDs []int
			if rawAssignees, ok := input["assigneeIds"].([]interface{}); ok {
				for _, raw := range rawAssignees {
					nodeID := fmt.Sprintf("%v", raw)
					if u := findUserByNodeID(s.store, nodeID); u != nil {
						assigneeIDs = append(assigneeIDs, u.ID)
					}
				}
			}

			// Resolve milestone node ID
			var milestoneID int
			if msNodeID, ok := input["milestoneId"].(string); ok && msNodeID != "" {
				if ms := findMilestoneByNodeID(s.store, msNodeID); ms != nil {
					milestoneID = ms.ID
				}
			}

			issue := s.store.CreateIssue(repo.ID, user.ID, title, body, labelIDs, assigneeIDs, milestoneID)
			if issue == nil {
				return nil, fmt.Errorf("issue creation failed")
			}

			return map[string]interface{}{
				"issue": issueToGQL(issue, s.store),
			}, nil
		},
	})

	closeIssueInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CloseIssueInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"issueId":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"stateReason": &graphql.InputObjectFieldConfig{Type: issueClosedStateReasonEnum},
		},
	})

	closeIssuePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CloseIssuePayload",
		Fields: graphql.Fields{
			"issue": &graphql.Field{Type: issueType},
		},
	})

	mutationType.AddFieldConfig("closeIssue", &graphql.Field{
		Type: closeIssuePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(closeIssueInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			issueNodeID, _ := input["issueId"].(string)
			stateReason, _ := input["stateReason"].(string)
			if stateReason == "" {
				stateReason = "COMPLETED"
			}

			issue := findIssueByNodeID(s.store, issueNodeID)
			if issue == nil {
				return nil, fmt.Errorf("could not resolve to an Issue")
			}

			s.store.UpdateIssue(issue.ID, func(i *Issue) {
				i.State = "CLOSED"
				i.StateReason = stateReason
				now := time.Now()
				i.ClosedAt = &now
			})

			updated := s.store.GetIssue(issue.ID)
			return map[string]interface{}{
				"issue": issueToGQL(updated, s.store),
			}, nil
		},
	})

	reopenIssueInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ReopenIssueInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"issueId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})

	reopenIssuePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReopenIssuePayload",
		Fields: graphql.Fields{
			"issue": &graphql.Field{Type: issueType},
		},
	})

	mutationType.AddFieldConfig("reopenIssue", &graphql.Field{
		Type: reopenIssuePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(reopenIssueInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			issueNodeID, _ := input["issueId"].(string)

			issue := findIssueByNodeID(s.store, issueNodeID)
			if issue == nil {
				return nil, fmt.Errorf("could not resolve to an Issue")
			}

			s.store.UpdateIssue(issue.ID, func(i *Issue) {
				i.State = "OPEN"
				i.StateReason = ""
				i.ClosedAt = nil
			})

			updated := s.store.GetIssue(issue.ID)
			return map[string]interface{}{
				"issue": issueToGQL(updated, s.store),
			}, nil
		},
	})

	addCommentInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "AddCommentInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"subjectId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"body":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	commentEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueCommentEdge",
		Fields: graphql.Fields{
			"node": &graphql.Field{Type: issueCommentType},
		},
	})

	addCommentPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AddCommentPayload",
		Fields: graphql.Fields{
			"commentEdge": &graphql.Field{Type: commentEdgeType},
			"subject":     &graphql.Field{Type: issueType},
		},
	})

	mutationType.AddFieldConfig("addComment", &graphql.Field{
		Type: addCommentPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(addCommentInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			subjectNodeID, _ := input["subjectId"].(string)
			body, _ := input["body"].(string)

			issue := findIssueByNodeID(s.store, subjectNodeID)
			if issue == nil {
				return nil, fmt.Errorf("could not resolve to a node with the global id of '%s'", subjectNodeID)
			}

			comment := s.store.CreateComment(issue.ID, user.ID, body)
			if comment == nil {
				return nil, fmt.Errorf("comment creation failed")
			}

			return map[string]interface{}{
				"commentEdge": map[string]interface{}{
					"node": commentToGQL(comment, s.store),
				},
				"subject": issueToGQL(issue, s.store),
			}, nil
		},
	})

	updateIssueInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpdateIssueInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"id":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"title":       &graphql.InputObjectFieldConfig{Type: graphql.String},
			"body":        &graphql.InputObjectFieldConfig{Type: graphql.String},
			"state":       &graphql.InputObjectFieldConfig{Type: graphql.String},
			"milestoneId": &graphql.InputObjectFieldConfig{Type: graphql.ID},
			"labelIds":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
			"assigneeIds": &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
		},
	})

	updateIssuePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UpdateIssuePayload",
		Fields: graphql.Fields{
			"issue": &graphql.Field{Type: issueType},
		},
	})

	mutationType.AddFieldConfig("updateIssue", &graphql.Field{
		Type: updateIssuePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(updateIssueInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			issueNodeID, _ := input["id"].(string)

			issue := findIssueByNodeID(s.store, issueNodeID)
			if issue == nil {
				return nil, fmt.Errorf("could not resolve to an Issue")
			}

			s.store.UpdateIssue(issue.ID, func(i *Issue) {
				if v, ok := input["title"].(string); ok {
					i.Title = v
				}
				if v, ok := input["body"].(string); ok {
					i.Body = v
				}
				if v, ok := input["state"].(string); ok {
					i.State = strings.ToUpper(v)
				}
			})

			updated := s.store.GetIssue(issue.ID)
			return map[string]interface{}{
				"issue": issueToGQL(updated, s.store),
			}, nil
		},
	})

	return issueType
}

// --- GraphQL converter helpers ---

func issueToGQL(issue *Issue, st *Store) map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()

	// Author
	var author map[string]interface{}
	if u, ok := st.Users[issue.AuthorID]; ok {
		author = userToGraphQL(u)
	}

	// Labels
	labelNodes := make([]map[string]interface{}, 0)
	for _, lid := range issue.LabelIDs {
		if l, ok := st.Labels[lid]; ok {
			labelNodes = append(labelNodes, labelToGQL(l))
		}
	}

	// Assignees
	assigneeNodes := make([]map[string]interface{}, 0)
	for _, aid := range issue.AssigneeIDs {
		if u, ok := st.Users[aid]; ok {
			assigneeNodes = append(assigneeNodes, userToGraphQL(u))
		}
	}

	// Milestone
	var milestone map[string]interface{}
	if issue.MilestoneID > 0 {
		if ms, ok := st.Milestones[issue.MilestoneID]; ok {
			milestone = milestoneToGQL(ms)
		}
	}

	// Comments
	commentNodes := make([]map[string]interface{}, 0)
	for _, c := range st.Comments {
		if c.IssueID == issue.ID {
			commentNodes = append(commentNodes, commentToGQLLocked(c, st))
		}
	}

	// Resolve repo for URL
	repo := st.Repos[issue.RepoID]
	url := ""
	if repo != nil {
		url = "/" + repo.FullName + "/issues/" + fmt.Sprintf("%d", issue.Number)
	}

	var closedAt interface{}
	if issue.ClosedAt != nil {
		closedAt = issue.ClosedAt.Format(time.RFC3339)
	}

	var stateReason interface{}
	if issue.StateReason != "" {
		stateReason = issue.StateReason
	}

	return map[string]interface{}{
		"nodeID":      issue.NodeID,
		"databaseId":  issue.ID,
		"number":      issue.Number,
		"title":       issue.Title,
		"body":        issue.Body,
		"state":       issue.State,
		"stateReason": stateReason,
		"url":         url,
		"createdAt":   issue.CreatedAt.Format(time.RFC3339),
		"updatedAt":   issue.UpdatedAt.Format(time.RFC3339),
		"closedAt":    closedAt,
		"isPinned":    false,
		"author":      author,
		"labels": map[string]interface{}{
			"nodes":      labelNodes,
			"totalCount": len(labelNodes),
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
		"assignees": map[string]interface{}{
			"nodes":      assigneeNodes,
			"totalCount": len(assigneeNodes),
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
		"milestone": milestone,
		"comments": map[string]interface{}{
			"nodes":      commentNodes,
			"totalCount": len(commentNodes),
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
	}
}

func labelToGQL(l *IssueLabel) map[string]interface{} {
	return map[string]interface{}{
		"nodeID":      l.NodeID,
		"name":        l.Name,
		"description": l.Description,
		"color":       l.Color,
	}
}

func milestoneToGQL(ms *Milestone) map[string]interface{} {
	var dueOn interface{}
	if ms.DueOn != nil {
		dueOn = ms.DueOn.Format(time.RFC3339)
	}
	return map[string]interface{}{
		"nodeID":      ms.NodeID,
		"number":      ms.Number,
		"title":       ms.Title,
		"description": ms.Description,
		"state":       strings.ToUpper(ms.State),
		"dueOn":       dueOn,
	}
}

func commentToGQL(c *Comment, st *Store) map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return commentToGQLLocked(c, st)
}

func commentToGQLLocked(c *Comment, st *Store) map[string]interface{} {
	var author map[string]interface{}
	if u, ok := st.Users[c.AuthorID]; ok {
		author = userToGraphQL(u)
	}
	return map[string]interface{}{
		"nodeID":              c.NodeID,
		"body":                c.Body,
		"url":                 "",
		"createdAt":           c.CreatedAt.Format(time.RFC3339),
		"updatedAt":           c.UpdatedAt.Format(time.RFC3339),
		"author":              author,
		"authorAssociation":   "OWNER",
	}
}

func staticReactionGroups() []map[string]interface{} {
	contents := []string{"THUMBS_UP", "THUMBS_DOWN", "LAUGH", "HOORAY", "CONFUSED", "HEART", "ROCKET", "EYES"}
	groups := make([]map[string]interface{}, 0, len(contents))
	for _, c := range contents {
		groups = append(groups, map[string]interface{}{
			"content": c,
			"users": map[string]interface{}{
				"totalCount": 0,
			},
		})
	}
	return groups
}

// --- Node ID lookup helpers ---

func findRepoByNodeID(st *Store, nodeID string) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, r := range st.Repos {
		if r.NodeID == nodeID {
			return r
		}
	}
	return nil
}

func findIssueByNodeID(st *Store, nodeID string) *Issue {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, i := range st.Issues {
		if i.NodeID == nodeID {
			return i
		}
	}
	return nil
}

func findLabelByNodeID(st *Store, nodeID string) *IssueLabel {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, l := range st.Labels {
		if l.NodeID == nodeID {
			return l
		}
	}
	return nil
}

func findMilestoneByNodeID(st *Store, nodeID string) *Milestone {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, ms := range st.Milestones {
		if ms.NodeID == nodeID {
			return ms
		}
	}
	return nil
}

func findUserByNodeID(st *Store, nodeID string) *User {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, u := range st.Users {
		if u.NodeID == nodeID {
			return u
		}
	}
	return nil
}

// paginateIssuesGQL implements Relay-style cursor pagination for issues.
func paginateIssuesGQL(issues []*Issue, st *Store, first int, after string) map[string]interface{} {
	total := len(issues)

	startIdx := 0
	if after != "" {
		startIdx = decodeCursor(after) + 1
	}
	if startIdx > total {
		startIdx = total
	}

	endIdx := startIdx + first
	if endIdx > total {
		endIdx = total
	}

	page := issues[startIdx:endIdx]

	nodes := make([]map[string]interface{}, 0, len(page))
	edges := make([]map[string]interface{}, 0, len(page))
	for idx, i := range page {
		gql := issueToGQL(i, st)
		cursor := encodeCursor(startIdx + idx)
		nodes = append(nodes, gql)
		edges = append(edges, map[string]interface{}{
			"node":   gql,
			"cursor": cursor,
		})
	}

	var startCursor, endCursor interface{}
	if len(edges) > 0 {
		startCursor = edges[0]["cursor"]
		endCursor = edges[len(edges)-1]["cursor"]
	}

	return map[string]interface{}{
		"nodes":      nodes,
		"edges":      edges,
		"totalCount": total,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     endIdx < total,
			"hasPreviousPage": startIdx > 0,
			"startCursor":     startCursor,
			"endCursor":       endCursor,
		},
	}
}
