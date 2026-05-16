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
			// Fields gh CLI's `gh issue view` queries on IssueComment — defaults
			// fine for bleephub (we don't model edit history or moderation).
			"includesCreatedEdit": &graphql.Field{Type: graphql.Boolean, Resolve: alwaysFalse},
			"isMinimized":         &graphql.Field{Type: graphql.Boolean, Resolve: alwaysFalse},
			"minimizedReason":     &graphql.Field{Type: graphql.String, Resolve: alwaysNil},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(reactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c := p.Source.(map[string]interface{})
					return c["reactionGroups"], nil
				},
			},
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
					m, ok := i["milestone"].(map[string]interface{})
					if !ok || m == nil {
						// graphql-go's NonNull checks fire even on a nil-valued
						// map[string]interface{}; return untyped nil so the field
						// resolves to null cleanly.
						return nil, nil
					}
					return m, nil
				},
			},
			// ProjectV2 items — gh CLI's `gh issue view` queries Issue.projectItems
			// as a second round-trip. Bleephub doesn't model Projects v2; return
			// an empty connection so the query type-checks + resolves cleanly.
			"projectItems": &graphql.Field{
				Type: projectV2ItemConnectionType(),
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{
						"totalCount": 0,
						"nodes":      []interface{}{},
						"pageInfo": map[string]interface{}{
							"hasNextPage": false,
							"endCursor":   nil,
						},
					}, nil
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
					i := p.Source.(map[string]interface{})
					return i["reactionGroups"], nil
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

	// IssueOrderField + OrderDirection enums — gh CLI sends enum names like
	// CREATED_AT / DESC, not strings.
	issueOrderFieldEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueOrderField",
		Values: graphql.EnumValueConfigMap{
			"CREATED_AT": &graphql.EnumValueConfig{Value: "CREATED_AT"},
			"UPDATED_AT": &graphql.EnumValueConfig{Value: "UPDATED_AT"},
			"COMMENTS":   &graphql.EnumValueConfig{Value: "COMMENTS"},
		},
	})
	issueOrderDirectionEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueOrderDirection",
		Values: graphql.EnumValueConfigMap{
			"ASC":  &graphql.EnumValueConfig{Value: "ASC"},
			"DESC": &graphql.EnumValueConfig{Value: "DESC"},
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
					"field":     &graphql.InputObjectFieldConfig{Type: issueOrderFieldEnum},
					"direction": &graphql.InputObjectFieldConfig{Type: issueOrderDirectionEnum},
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

	// issueOrPullRequest is defined in addPullRequestFieldsToSchema (after the
	// PullRequest type exists), so it can return a union of Issue|PullRequest.
	// gh CLI's `gh issue view <N>` uses `...on Issue` + `...on PullRequest`
	// fragments which require a real union return type.

	repoType.AddFieldConfig("labels", &graphql.Field{
		Type: labelConnectionType,
		Args: graphql.FieldConfigArgument{
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
			"after": &graphql.ArgumentConfig{Type: graphql.String},
			"query": &graphql.ArgumentConfig{Type: graphql.String},
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
		"reactionGroups": reactionGroupsForGraphQL(st.Reactions, "issue", issue.ID),
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
		"nodeID":            c.NodeID,
		"body":              c.Body,
		"url":               "",
		"createdAt":         c.CreatedAt.Format(time.RFC3339),
		"updatedAt":         c.UpdatedAt.Format(time.RFC3339),
		"author":            author,
		"authorAssociation": "OWNER",
		"reactionGroups":    reactionGroupsForGraphQL(st.Reactions, "issue_comment", c.ID),
	}
}

// reactionGroupsForGraphQL returns a GraphQL-shaped `[ReactionGroup]` list
// for the given parent, querying the real ReactionStore so per-content
// totalCount values reflect actual reactions. Used by Issue, IssueComment,
// and any other reactable type's `reactionGroups` field.
func reactionGroupsForGraphQL(rs *ReactionStore, parentType string, parentID int) []map[string]interface{} {
	counts := map[string]int{
		"+1": 0, "-1": 0, "laugh": 0, "confused": 0,
		"heart": 0, "hooray": 0, "rocket": 0, "eyes": 0,
	}
	if rs != nil && parentID != 0 {
		for _, r := range rs.ListReactions(parentType, parentID, "") {
			counts[r.Content]++
		}
	}
	// Order matches real GitHub's GraphQL response.
	mapping := [...]struct{ rest, gql string }{
		{"+1", "THUMBS_UP"},
		{"-1", "THUMBS_DOWN"},
		{"laugh", "LAUGH"},
		{"hooray", "HOORAY"},
		{"confused", "CONFUSED"},
		{"heart", "HEART"},
		{"rocket", "ROCKET"},
		{"eyes", "EYES"},
	}
	out := make([]map[string]interface{}, 0, len(mapping))
	for _, m := range mapping {
		out = append(out, map[string]interface{}{
			"content": m.gql,
			"users":   map[string]interface{}{"totalCount": counts[m.rest]},
		})
	}
	return out
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

// Schema-stub resolvers — return a default for fields that gh CLI queries
// but bleephub doesn't model (edit history, moderation, reactions).
// Errors-free responses unblock gh's queries; the contract returns defaults.
// projectV2ItemConnectionType returns a singleton stub for GitHub Projects v2
// queries gh CLI's `gh issue view` performs. bleephub doesn't model Projects
// v2; this returns a queryable but always-empty connection.
//
// Defined as a function (not a top-level var) so it's lazily-constructed —
// graphql-go panics if types are constructed before the parent type is built.
var projectV2ItemConnectionTypeMemo *graphql.Object

func projectV2ItemConnectionType() *graphql.Object {
	if projectV2ItemConnectionTypeMemo != nil {
		return projectV2ItemConnectionTypeMemo
	}
	projectV2Type := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2",
		Fields: graphql.Fields{
			"id":    &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: unreachableFieldErr},
			"title": &graphql.Field{Type: graphql.NewNonNull(graphql.String), Resolve: unreachableFieldErr},
		},
	})
	singleSelectValueType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemFieldSingleSelectValue",
		Fields: graphql.Fields{
			"optionId": &graphql.Field{Type: graphql.String, Resolve: alwaysNil},
			"name":     &graphql.Field{Type: graphql.String, Resolve: alwaysNil},
		},
	})
	itemFieldValueUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:  "ProjectV2ItemFieldValue",
		Types: []*graphql.Object{singleSelectValueType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			return singleSelectValueType
		},
	})
	projectV2ItemType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2Item",
		Fields: graphql.Fields{
			"id": &graphql.Field{Type: graphql.NewNonNull(graphql.ID), Resolve: unreachableFieldErr},
			"project": &graphql.Field{
				Type:    projectV2Type,
				Resolve: alwaysNil,
			},
			"fieldValueByName": &graphql.Field{
				Type: itemFieldValueUnion,
				Args: graphql.FieldConfigArgument{
					"name": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: alwaysNil,
			},
		},
	})
	projectV2ItemPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemPageInfo",
		Fields: graphql.Fields{
			"hasNextPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"endCursor":   &graphql.Field{Type: graphql.String},
		},
	})
	projectV2ItemConnectionTypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"nodes":      &graphql.Field{Type: graphql.NewList(projectV2ItemType)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(projectV2ItemPageInfoType)},
		},
	})
	return projectV2ItemConnectionTypeMemo
}

// emptyList resolves a connection's nodes/edges to an empty list. Used by
// connection types representing surfaces bleephub doesn't track (e.g.
// ProjectV2Item, PRComment) so the connection-level shape gh CLI requires
// stays valid while the underlying field-level resolvers below stay
// unreachable. Truthful — bleephub has zero items in those surfaces, not
// "we hid the items."
func emptyList(graphql.ResolveParams) (interface{}, error) { return []interface{}{}, nil }

// alwaysFalse / alwaysNil are truthful defaults for fields representing
// dimensions bleephub doesn't model (comment minimization, edit history).
// Real GitHub returns false / null on the same fields for surfaces that
// haven't been minimized; bleephub matches the spec.
func alwaysFalse(graphql.ResolveParams) (interface{}, error) { return false, nil }
func alwaysNil(graphql.ResolveParams) (interface{}, error)   { return nil, nil }

// unreachableFieldErr resolves a field that should never be invoked at query
// time because its parent connection always returns an empty nodes/edges
// list. If it fires anyway, a feature got partially implemented (e.g. real
// ProjectV2 items added without wiring the field resolvers); the error
// surfaces the gap instead of returning fake "" / nil data.
func unreachableFieldErr(p graphql.ResolveParams) (interface{}, error) {
	return nil, fmt.Errorf("bleephub: field %q on type %q is unreachable; if you see this, the parent connection returned a non-empty list without wiring real resolvers", p.Info.FieldName, p.Info.ParentType.Name())
}
