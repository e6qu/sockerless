package bleephub

import (
	"fmt"
	"sort"
	"strconv"
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
					l, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("label source: unexpected type %T", p.Source)
					}
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
					m, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("milestone source: unexpected type %T", p.Source)
					}
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
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
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
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["author"], nil
				},
			},
			"authorAssociation": &graphql.Field{Type: graphql.String},
			// Fields gh CLI's `gh issue view` queries on IssueComment — defaults
			// fine for bleephub (we don't model edit history or moderation).
			"includesCreatedEdit": &graphql.Field{
				Type: graphql.Boolean,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["includesCreatedEdit"], nil
				},
			},
			"lastEditedAt": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["lastEditedAt"], nil
				},
			},
			"editor": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["editor"], nil
				},
			},
			"isMinimized": &graphql.Field{
				Type: graphql.Boolean,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["isMinimized"], nil
				},
			},
			"isPinned": &graphql.Field{
				Type: graphql.Boolean,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["isPinned"], nil
				},
			},
			"minimizedReason": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["minimizedReason"], nil
				},
			},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(reactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["reactionGroups"], nil
				},
			},
			// gh's shared comments fragment (issue view + pr view) selects
			// viewerDidAuthor; mirrors PRComment's resolver.
			"viewerDidAuthor": &graphql.Field{
				Type: graphql.Boolean,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					viewer := ghUserFromContext(p.Context)
					authorID, _ := c["authorID"].(int)
					return viewer != nil && authorID == viewer.ID, nil
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

	// --- Issue-type and sub-issue support types ---
	// gh CLI's `gh issue view` selects GitHub's issue-type and sub-issue
	// fields. Issue types resolve from the organization definitions assigned
	// to the issue row. Sub-issues are backed by the same ordered store links
	// used by the REST API.
	issueTypeMetaType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueType",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"name":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description": &graphql.Field{Type: graphql.String},
			"color":       &graphql.Field{Type: graphql.String},
		},
	})

	relatedIssueRepoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "RelatedIssueRepository",
		Fields: graphql.Fields{
			"nameWithOwner": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	relatedIssueType := graphql.NewObject(graphql.ObjectConfig{
		Name: "RelatedIssue",
		Fields: graphql.Fields{
			"id":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"number":     &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"url":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"state":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"repository": &graphql.Field{Type: relatedIssueRepoType},
		},
	})

	subIssueConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SubIssueConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(relatedIssueType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	subIssuesSummaryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SubIssuesSummary",
		Fields: graphql.Fields{
			"total":            &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"completed":        &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"percentCompleted": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	issueFieldValueConnectionType := issueFieldValueGraphQLConnectionType()

	// --- Issue type ---
	issueType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Issue",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return i["nodeID"], nil
				},
			},
			"databaseId":  &graphql.Field{Type: graphql.Int},
			"number":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"body":        &graphql.Field{Type: graphql.String},
			"state":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"stateReason": &graphql.Field{Type: graphql.String},
			// gh's shared issue/PR field set selects `closed`.
			"closed": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					state, _ := i["state"].(string)
					return state == "CLOSED", nil
				},
			},
			"url":              &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"createdAt":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"closedAt":         &graphql.Field{Type: graphql.String},
			"isPinned":         &graphql.Field{Type: graphql.Boolean},
			"locked":           &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"activeLockReason": &graphql.Field{Type: graphql.String},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return i["author"], nil
				},
			},
			"labels": &graphql.Field{
				Type: labelConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(i["labels"], p.Args), nil
				},
			},
			"assignees": &graphql.Field{
				Type: assigneeConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(i["assignees"], p.Args), nil
				},
			},
			"milestone": &graphql.Field{
				Type: issueMilestoneType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
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
			// as a second round-trip. Returns the real ProjectV2Item nodes the
			// issue has been added to via addProjectV2ItemById.
			"projectItems": &graphql.Field{
				Type: projectV2ItemConnectionType(),
				Args: relayConnectionArgs(),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					issueID, _ := i["databaseId"].(int)
					return projectItemsConnectionForIssue(s.store, issueID, p.Args), nil
				},
			},
			"comments": &graphql.Field{
				Type: commentConnectionType,
				Args: graphql.FieldConfigArgument{
					"first":  &graphql.ArgumentConfig{Type: graphql.Int},
					"last":   &graphql.ArgumentConfig{Type: graphql.Int},
					"after":  &graphql.ArgumentConfig{Type: graphql.String},
					"before": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(i["comments"], p.Args), nil
				},
			},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(reactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return i["reactionGroups"], nil
				},
			},
			"issueType": &graphql.Field{
				Type: issueTypeMetaType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					it, ok := i["issueType"].(map[string]interface{})
					if !ok || it == nil {
						return nil, nil
					}
					return it, nil
				},
			},
			"parent": &graphql.Field{
				Type: relatedIssueType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					parent, ok := i["parent"].(map[string]interface{})
					if !ok || parent == nil {
						return nil, nil
					}
					return parent, nil
				},
			},
			"subIssues": &graphql.Field{
				Type: subIssueConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(i["subIssues"], p.Args), nil
				},
			},
			"subIssuesSummary": &graphql.Field{
				Type: subIssuesSummaryType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return i["subIssuesSummary"], nil
				},
			},
			"issueFieldValues": &graphql.Field{
				Type: issueFieldValueConnectionType,
				Args: graphql.FieldConfigArgument{
					"first":  &graphql.ArgumentConfig{Type: graphql.Int},
					"last":   &graphql.ArgumentConfig{Type: graphql.Int},
					"after":  &graphql.ArgumentConfig{Type: graphql.String},
					"before": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					i, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(i["issueFieldValues"], p.Args), nil
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
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["hasIssues"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing hasIssues")
			}
			return v, nil
		},
	})

	repoType.AddFieldConfig("viewerPermission", &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			// Real GitHub computes viewerPermission from the viewer's actual
			// access — ADMIN/WRITE/READ (bleephub models pull/push/admin; it
			// does not track MAINTAIN/TRIAGE). Return null for no access.
			src, _ := p.Source.(map[string]interface{})
			fullName, _ := src["nameWithOwner"].(string)
			parts := strings.SplitN(fullName, "/", 2)
			if len(parts) != 2 {
				return nil, nil
			}
			repo := s.store.GetRepo(parts[0], parts[1])
			if repo == nil {
				return nil, nil
			}
			viewer := ghUserFromContext(p.Context)
			switch {
			case canAdminRepo(s.store, viewer, repo):
				return "ADMIN", nil
			case canPushRepo(s.store, viewer, repo):
				return "WRITE", nil
			case canReadRepo(s.store, viewer, repo):
				return "READ", nil
			default:
				return nil, nil
			}
		},
	})

	repoType.AddFieldConfig("mergeCommitAllowed", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["allowMergeCommit"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing allowMergeCommit")
			}
			return v, nil
		},
	})

	repoType.AddFieldConfig("rebaseMergeAllowed", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["allowRebaseMerge"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing allowRebaseMerge")
			}
			return v, nil
		},
	})

	repoType.AddFieldConfig("squashMergeAllowed", &graphql.Field{
		Type: graphql.Boolean,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["allowSquashMerge"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing allowSquashMerge")
			}
			return v, nil
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
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
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
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, _ := repo["databaseId"].(int)
			number, _ := p.Args["number"].(int)

			issue := s.store.GetIssueByNumber(repoID, number)
			if issue == nil {
				// Real GitHub returns a typed NOT_FOUND error, not bare null.
				return nil, &ghNotFoundError{
					message: fmt.Sprintf("Could not resolve to an Issue with the number of %d.", number),
				}
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
			// gh sends literal enum names (gh label list/create issue
			// `labels(orderBy: {field: NAME, direction: ASC})`), so
			// field/direction must be enums — string-typed inputs reject
			// the literals.
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "LabelOrder",
				Fields: graphql.InputObjectConfigFieldMap{
					"field": &graphql.InputObjectFieldConfig{Type: graphql.NewEnum(graphql.EnumConfig{
						Name: "LabelOrderField",
						Values: graphql.EnumValueConfigMap{
							"NAME":       &graphql.EnumValueConfig{Value: "NAME"},
							"CREATED_AT": &graphql.EnumValueConfig{Value: "CREATED_AT"},
						},
					})},
					"direction": &graphql.InputObjectFieldConfig{Type: graphql.NewEnum(graphql.EnumConfig{
						Name: "LabelOrderDirection",
						Values: graphql.EnumValueConfigMap{
							"ASC":  &graphql.EnumValueConfig{Value: "ASC"},
							"DESC": &graphql.EnumValueConfig{Value: "DESC"},
						},
					})},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
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

			first := 0
			if f, ok := p.Args["first"].(int); ok {
				first = f
			}
			after, _ := p.Args["after"].(string)
			return paginateGQL(labels, first, after, labelToGQL), nil
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
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
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

			first := 0
			if f, ok := p.Args["first"].(int); ok {
				first = f
			}
			after, _ := p.Args["after"].(string)
			return paginateGQL(milestones, first, after, milestoneToGQL), nil
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
			// Return all users — real GH scopes this to org members or repo
			// collaborators; bleephub doesn't model membership/collab graphs yet
			// (`p.Source` carries owner.login but it's intentionally unused
			// until that surface lands).
			s.store.mu.RLock()
			var users []*User
			for _, u := range s.store.Users {
				users = append(users, u)
			}
			s.store.mu.RUnlock()

			// Filter by query
			if q, ok := p.Args["query"].(string); ok && q != "" {
				q = strings.ToLower(q)
				var filtered []*User
				for _, u := range users {
					if strings.Contains(strings.ToLower(u.Login), q) || strings.Contains(strings.ToLower(u.Name), q) {
						filtered = append(filtered, u)
					}
				}
				users = filtered
			}

			// assignableUsers iterates a Go map, so order is nondeterministic;
			// sort by ID to make cursor pagination stable across pages.
			sort.Slice(users, func(a, b int) bool { return users[a].ID < users[b].ID })

			first := 0
			if f, ok := p.Args["first"].(int); ok {
				first = f
			}
			after, _ := p.Args["after"].(string)
			return paginateGQL(users, first, after, userToGraphQL), nil
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
			"issueTypeId":  &graphql.InputObjectFieldConfig{Type: graphql.ID},
			// gh's IssueCreate mutation always serializes projectIds (null
			// unless --project) and issueTemplate when a template applies —
			// the input must declare them or variable coercion rejects the
			// whole mutation. Classic (v1) projects aren't modeled, and gh
			// resolves --project against the repo's (empty) project lists
			// before mutating, so non-null projectIds never arrive.
			"projectIds":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
			"issueTemplate": &graphql.InputObjectFieldConfig{Type: graphql.String},
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
			var issueTypeID int
			if itNodeID, ok := input["issueTypeId"].(string); ok && itNodeID != "" {
				it := findIssueTypeByNodeID(s.store, itNodeID)
				if it == nil || s.store.GetAssignableIssueTypeForRepo(repo, it.ID) == nil {
					return nil, fmt.Errorf("could not resolve to an IssueType with the global id of '%s'", itNodeID)
				}
				issueTypeID = it.ID
			}

			issue := s.store.CreateIssue(repo.ID, user.ID, title, body, labelIDs, assigneeIDs, milestoneID)
			if issue == nil {
				return nil, fmt.Errorf("issue creation failed")
			}
			if issueTypeID > 0 {
				s.store.UpdateIssue(issue.ID, func(i *Issue) {
					i.IssueTypeID = issueTypeID
				})
				issue = s.store.GetIssue(issue.ID)
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

			// On real GitHub a PR is an issue, so addComment's subjectId may be
			// either; `gh pr comment` passes a PR node id. Resolve both.
			if issue := findIssueByNodeID(s.store, subjectNodeID); issue != nil {
				comment := s.store.CreateComment(issue.ID, user.ID, body)
				if comment == nil {
					return nil, fmt.Errorf("comment creation failed")
				}
				return map[string]interface{}{
					"commentEdge": map[string]interface{}{"node": commentToGQL(comment, s.store)},
					"subject":     issueToGQL(issue, s.store),
				}, nil
			}
			if pr := findPullRequestByNodeID(s.store, subjectNodeID); pr != nil {
				comment := s.store.CreateCommentFor("pull_request", pr.ID, user.ID, body)
				if comment == nil {
					return nil, fmt.Errorf("comment creation failed")
				}
				return map[string]interface{}{
					"commentEdge": map[string]interface{}{"node": commentToGQL(comment, s.store)},
					// subject is the Issue type; `gh pr comment` reads only
					// commentEdge.node, so a PR subject is not queried here.
					"subject": nil,
				}, nil
			}
			return nil, fmt.Errorf("could not resolve to a node with the global id of '%s'", subjectNodeID)
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
			"issueTypeId": &graphql.InputObjectFieldConfig{Type: graphql.ID},
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
			var issueTypeID *int
			if raw, present := input["issueTypeId"]; present {
				if itNodeID, ok := raw.(string); ok && itNodeID != "" {
					repo := s.store.GetRepoByID(issue.RepoID)
					it := findIssueTypeByNodeID(s.store, itNodeID)
					if it == nil || s.store.GetAssignableIssueTypeForRepo(repo, it.ID) == nil {
						return nil, fmt.Errorf("could not resolve to an IssueType with the global id of '%s'", itNodeID)
					}
					resolved := it.ID
					issueTypeID = &resolved
				} else {
					cleared := 0
					issueTypeID = &cleared
				}
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
				if issueTypeID != nil {
					i.IssueTypeID = *issueTypeID
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

func issueFieldValueGraphQLConnectionType() *graphql.Object {
	dataTypeEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueFieldDataType",
		Values: graphql.EnumValueConfigMap{
			"TEXT":          &graphql.EnumValueConfig{Value: "TEXT"},
			"SINGLE_SELECT": &graphql.EnumValueConfig{Value: "SINGLE_SELECT"},
			"DATE":          &graphql.EnumValueConfig{Value: "DATE"},
			"NUMBER":        &graphql.EnumValueConfig{Value: "NUMBER"},
			"MULTI_SELECT":  &graphql.EnumValueConfig{Value: "MULTI_SELECT"},
		},
	})
	visibilityEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueFieldVisibility",
		Values: graphql.EnumValueConfigMap{
			"ORG_ONLY": &graphql.EnumValueConfig{Value: "ORG_ONLY"},
			"ALL":      &graphql.EnumValueConfig{Value: "ALL"},
		},
	})
	colorEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "IssueFieldSingleSelectOptionColor",
		Values: graphql.EnumValueConfigMap{
			"GRAY":   &graphql.EnumValueConfig{Value: "GRAY"},
			"BLUE":   &graphql.EnumValueConfig{Value: "BLUE"},
			"GREEN":  &graphql.EnumValueConfig{Value: "GREEN"},
			"YELLOW": &graphql.EnumValueConfig{Value: "YELLOW"},
			"ORANGE": &graphql.EnumValueConfig{Value: "ORANGE"},
			"RED":    &graphql.EnumValueConfig{Value: "RED"},
			"PINK":   &graphql.EnumValueConfig{Value: "PINK"},
			"PURPLE": &graphql.EnumValueConfig{Value: "PURPLE"},
		},
	})

	optionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueFieldSingleSelectOption",
		Fields: graphql.Fields{
			"id":             &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"databaseId":     &graphql.Field{Type: graphql.Int},
			"fullDatabaseId": &graphql.Field{Type: graphql.String},
			"name":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description":    &graphql.Field{Type: graphql.String},
			"color":          &graphql.Field{Type: graphql.NewNonNull(colorEnum)},
			"priority":       &graphql.Field{Type: graphql.Int},
		},
	})

	commonFieldFields := func(withOptions bool) graphql.Fields {
		fields := graphql.Fields{
			"id":             &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"fullDatabaseId": &graphql.Field{Type: graphql.String},
			"name":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description":    &graphql.Field{Type: graphql.String},
			"dataType":       &graphql.Field{Type: graphql.NewNonNull(dataTypeEnum)},
			"visibility":     &graphql.Field{Type: graphql.NewNonNull(visibilityEnum)},
			"createdAt":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		}
		if withOptions {
			fields["options"] = &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(optionType)))}
		}
		return fields
	}
	textFieldType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldText", Fields: commonFieldFields(false)})
	dateFieldType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldDate", Fields: commonFieldFields(false)})
	numberFieldType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldNumber", Fields: commonFieldFields(false)})
	singleSelectFieldType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldSingleSelect", Fields: commonFieldFields(true)})
	multiSelectFieldType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldMultiSelect", Fields: commonFieldFields(true)})

	fieldUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:  "IssueFields",
		Types: []*graphql.Object{textFieldType, dateFieldType, numberFieldType, singleSelectFieldType, multiSelectFieldType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			src, _ := p.Value.(map[string]interface{})
			switch src["__typename"] {
			case "IssueFieldDate":
				return dateFieldType
			case "IssueFieldNumber":
				return numberFieldType
			case "IssueFieldSingleSelect":
				return singleSelectFieldType
			case "IssueFieldMultiSelect":
				return multiSelectFieldType
			default:
				return textFieldType
			}
		},
	})

	commonValueFields := func(valueType graphql.Output) graphql.Fields {
		return graphql.Fields{
			"id":    &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"field": &graphql.Field{Type: fieldUnion},
			"value": &graphql.Field{Type: valueType},
		}
	}
	textValueType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldTextValue", Fields: commonValueFields(graphql.NewNonNull(graphql.String))})
	dateValueType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldDateValue", Fields: commonValueFields(graphql.NewNonNull(graphql.String))})
	numberValueType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldNumberValue", Fields: commonValueFields(graphql.NewNonNull(graphql.Float))})
	singleSelectValueFields := commonValueFields(graphql.NewNonNull(graphql.String))
	singleSelectValueFields["name"] = &graphql.Field{Type: graphql.NewNonNull(graphql.String)}
	singleSelectValueFields["description"] = &graphql.Field{Type: graphql.String}
	singleSelectValueFields["color"] = &graphql.Field{Type: graphql.NewNonNull(colorEnum)}
	singleSelectValueFields["optionId"] = &graphql.Field{Type: graphql.String}
	singleSelectValueType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldSingleSelectValue", Fields: singleSelectValueFields})
	multiSelectValueFields := commonValueFields(graphql.String)
	multiSelectValueFields["options"] = &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(optionType)))}
	multiSelectValueType := graphql.NewObject(graphql.ObjectConfig{Name: "IssueFieldMultiSelectValue", Fields: multiSelectValueFields})

	valueUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:  "IssueFieldValue",
		Types: []*graphql.Object{dateValueType, multiSelectValueType, numberValueType, singleSelectValueType, textValueType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			src, _ := p.Value.(map[string]interface{})
			switch src["__typename"] {
			case "IssueFieldDateValue":
				return dateValueType
			case "IssueFieldMultiSelectValue":
				return multiSelectValueType
			case "IssueFieldNumberValue":
				return numberValueType
			case "IssueFieldSingleSelectValue":
				return singleSelectValueType
			default:
				return textValueType
			}
		},
	})
	edgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueFieldValueEdge",
		Fields: graphql.Fields{
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"node":   &graphql.Field{Type: valueUnion},
		},
	})
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "IssueFieldValueConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(valueUnion)},
			"edges":      &graphql.Field{Type: graphql.NewList(edgeType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(gqlPageInfoType())},
		},
	})
}

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
	var issueType map[string]interface{}
	if it := st.issueTypeForIssueLocked(issue); it != nil {
		issueType = map[string]interface{}{
			"id":          it.NodeID,
			"name":        it.Name,
			"description": nilStrPtr(it.Description),
			"color":       nilStrPtr(it.Color),
		}
	}

	// Comments
	commentNodes := make([]map[string]interface{}, 0)
	for _, c := range st.Comments {
		if c.ParentType == "issue" && c.IssueID == issue.ID {
			commentNodes = append(commentNodes, commentToGQLLocked(c, st))
		}
	}
	// st.Comments is a map, so iteration order is nondeterministic; sort for
	// stable cursor pagination (oldest first, like GitHub's comments feed).
	sortGQLNodesByCreatedAt(commentNodes)

	// Resolve repo for URL
	repo := st.Repos[issue.RepoID]
	url := ""
	if repo != nil {
		url = "/" + repo.FullName + "/issues/" + strconv.Itoa(issue.Number)
	}

	var parent map[string]interface{}
	if parentID, ok := st.SubIssueParent[issue.ID]; ok {
		if parentIssue := st.Issues[parentID]; parentIssue != nil {
			parent = relatedIssueToGQLLocked(parentIssue, st)
		}
	}
	subIssueNodes := make([]map[string]interface{}, 0, len(st.SubIssueLists[issue.ID]))
	completedSubIssues := 0
	for _, childID := range st.SubIssueLists[issue.ID] {
		child := st.Issues[childID]
		if child == nil {
			continue
		}
		if child.State == "CLOSED" {
			completedSubIssues++
		}
		subIssueNodes = append(subIssueNodes, relatedIssueToGQLLocked(child, st))
	}
	percentCompleted := 0
	if len(subIssueNodes) > 0 {
		percentCompleted = completedSubIssues * 100 / len(subIssueNodes)
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
		"nodeID":           issue.NodeID,
		"databaseId":       issue.ID,
		"number":           issue.Number,
		"title":            issue.Title,
		"body":             issue.Body,
		"state":            issue.State,
		"stateReason":      stateReason,
		"url":              url,
		"createdAt":        issue.CreatedAt.Format(time.RFC3339),
		"updatedAt":        issue.UpdatedAt.Format(time.RFC3339),
		"closedAt":         closedAt,
		"isPinned":         false,
		"locked":           issue.Locked,
		"activeLockReason": nilStr(issue.ActiveLockReason),
		"author":           author,
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
		"issueType": issueType,
		"parent":    parent,
		"subIssues": map[string]interface{}{
			"nodes":      subIssueNodes,
			"totalCount": len(subIssueNodes),
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
		"subIssuesSummary": map[string]interface{}{
			"total":            len(subIssueNodes),
			"completed":        completedSubIssues,
			"percentCompleted": percentCompleted,
		},
		"issueFieldValues": issueFieldValuesConnectionLocked(st, issue),
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

var gqlPageInfoTypeMemo *graphql.Object

func gqlPageInfoType() *graphql.Object {
	if gqlPageInfoTypeMemo != nil {
		return gqlPageInfoTypeMemo
	}
	gqlPageInfoTypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "PageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})
	return gqlPageInfoTypeMemo
}

func relayConnectionArgs() graphql.FieldConfigArgument {
	return graphql.FieldConfigArgument{
		"first":  &graphql.ArgumentConfig{Type: graphql.Int},
		"after":  &graphql.ArgumentConfig{Type: graphql.String},
		"last":   &graphql.ArgumentConfig{Type: graphql.Int},
		"before": &graphql.ArgumentConfig{Type: graphql.String},
	}
}

func issueFieldValuesConnectionLocked(st *Store, issue *Issue) map[string]interface{} {
	repo := st.Repos[issue.RepoID]
	org := ""
	if repo != nil {
		org = issueFieldsOrgLocked(st, repo)
	}
	values := st.IssueFieldValues[issue.ID]
	fieldIDs := make([]int, 0, len(values))
	for id := range values {
		fieldIDs = append(fieldIDs, id)
	}
	sort.Ints(fieldIDs)
	nodes := make([]map[string]interface{}, 0, len(fieldIDs))
	for _, fieldID := range fieldIDs {
		field := st.OrgIssueFields[org][fieldID]
		if field == nil {
			continue
		}
		nodes = append(nodes, issueFieldValueToGQLLocked(field, issue.ID, values[fieldID]))
	}
	return paginateGQL(nodes, len(nodes), "", func(n map[string]interface{}) map[string]interface{} { return n })
}

func issueFieldsOrgLocked(st *Store, repo *Repo) string {
	orgLogin, _, _ := strings.Cut(repo.FullName, "/")
	if st.OrgsByLogin[orgLogin] == nil {
		return ""
	}
	return orgLogin
}

func issueFieldValueToGQLLocked(field *IssueField, issueID int, value interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"id":    fmt.Sprintf("IFV_kwDO%08d%08d", issueID, field.ID),
		"field": issueFieldToGQLLocked(field),
		"value": value,
	}
	switch field.DataType {
	case "date":
		out["__typename"] = "IssueFieldDateValue"
	case "number":
		out["__typename"] = "IssueFieldNumberValue"
	case "single_select":
		out["__typename"] = "IssueFieldSingleSelectValue"
		if name, ok := value.(string); ok {
			out["name"] = name
			if opt := issueFieldOptionByName(field, name); opt != nil {
				out["optionId"] = issueFieldOptionNodeID(opt.ID)
				out["description"] = nilStrPtr(opt.Description)
				out["color"] = issueFieldColorEnum(opt.Color)
			} else {
				out["description"] = nil
				out["color"] = "GRAY"
			}
		}
	case "multi_select":
		out["__typename"] = "IssueFieldMultiSelectValue"
		names := toStringSlice(value)
		opts := make([]map[string]interface{}, 0, len(names))
		for _, name := range names {
			if opt := issueFieldOptionByName(field, name); opt != nil {
				opts = append(opts, issueFieldOptionToGQL(opt))
			}
		}
		out["options"] = opts
		out["value"] = nil
	default:
		out["__typename"] = "IssueFieldTextValue"
	}
	return out
}

func issueFieldToGQLLocked(field *IssueField) map[string]interface{} {
	out := map[string]interface{}{
		"id":             field.NodeID,
		"fullDatabaseId": strconv.Itoa(field.ID),
		"name":           field.Name,
		"description":    nilStrPtr(field.Description),
		"dataType":       strings.ToUpper(field.DataType),
		"visibility":     issueFieldVisibilityEnum(field.Visibility),
		"createdAt":      field.CreatedAt.Format(time.RFC3339),
	}
	switch field.DataType {
	case "date":
		out["__typename"] = "IssueFieldDate"
	case "number":
		out["__typename"] = "IssueFieldNumber"
	case "single_select":
		out["__typename"] = "IssueFieldSingleSelect"
		out["options"] = issueFieldOptionsToGQL(field.Options)
	case "multi_select":
		out["__typename"] = "IssueFieldMultiSelect"
		out["options"] = issueFieldOptionsToGQL(field.Options)
	default:
		out["__typename"] = "IssueFieldText"
	}
	return out
}

func issueFieldOptionsToGQL(options []*IssueFieldOption) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(options))
	for _, opt := range options {
		out = append(out, issueFieldOptionToGQL(opt))
	}
	return out
}

func issueFieldOptionToGQL(opt *IssueFieldOption) map[string]interface{} {
	return map[string]interface{}{
		"id":             issueFieldOptionNodeID(opt.ID),
		"databaseId":     opt.ID,
		"fullDatabaseId": strconv.Itoa(opt.ID),
		"name":           opt.Name,
		"description":    nilStrPtr(opt.Description),
		"color":          issueFieldColorEnum(opt.Color),
		"priority":       opt.Priority,
	}
}

func issueFieldOptionByName(field *IssueField, name string) *IssueFieldOption {
	for _, opt := range field.Options {
		if opt.Name == name {
			return opt
		}
	}
	return nil
}

func issueFieldOptionNodeID(id int) string {
	return fmt.Sprintf("IFO_kwDO%08d", id)
}

func issueFieldColorEnum(color string) string {
	color = strings.ToUpper(color)
	if color == "" {
		return "GRAY"
	}
	return color
}

func issueFieldVisibilityEnum(visibility string) string {
	if visibility == "all" {
		return "ALL"
	}
	return "ORG_ONLY"
}

func labelToGQL(l *IssueLabel) map[string]interface{} {
	return map[string]interface{}{
		"nodeID":      l.NodeID,
		"name":        l.Name,
		"description": l.Description,
		"color":       l.Color,
	}
}

func relatedIssueToGQLLocked(issue *Issue, st *Store) map[string]interface{} {
	repo := st.Repos[issue.RepoID]
	nameWithOwner := ""
	url := ""
	if repo != nil {
		nameWithOwner = repo.FullName
		url = "/" + repo.FullName + "/issues/" + strconv.Itoa(issue.Number)
	}
	return map[string]interface{}{
		"id":     issue.NodeID,
		"nodeID": issue.NodeID,
		"number": issue.Number,
		"title":  issue.Title,
		"url":    url,
		"state":  issue.State,
		"repository": map[string]interface{}{
			"nameWithOwner": nameWithOwner,
		},
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
	var editor map[string]interface{}
	var lastEditedAt interface{}
	if c.LastEditedAt != nil {
		lastEditedAt = c.LastEditedAt.Format(time.RFC3339)
		if u, ok := st.Users[c.EditorID]; ok {
			editor = userToGraphQL(u)
		}
	}
	return map[string]interface{}{
		"_dbID":               c.ID,
		"nodeID":              c.NodeID,
		"body":                c.Body,
		"url":                 "",
		"authorID":            c.AuthorID,
		"createdAt":           c.CreatedAt.Format(time.RFC3339),
		"updatedAt":           c.UpdatedAt.Format(time.RFC3339),
		"author":              author,
		"authorAssociation":   "OWNER",
		"includesCreatedEdit": c.LastEditedAt != nil,
		"lastEditedAt":        lastEditedAt,
		"editor":              editor,
		"isMinimized":         c.MinimizedReason != "",
		"isPinned":            c.Pinned,
		"minimizedReason":     nilStr(c.MinimizedReason),
		"reactionGroups":      reactionGroupsForGraphQL(st.Reactions, "issue_comment", c.ID),
	}
}

// nilStr returns nil for empty strings (so nullable GraphQL String fields
// resolve to null rather than ""), or the string itself.
func nilStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nilStrPtr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
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
	return paginateGQL(issues, first, after, func(i *Issue) map[string]interface{} {
		return issueToGQL(i, st)
	})
}

// Some GraphQL fields queried by gh CLI are not mutable through the REST
// surfaces Bleephub implements today. Those fields resolve to their persisted
// value when modeled, or to the GitHub-shaped zero value when the feature is
// absent.
// projectV2ItemConnectionType returns a singleton wiring for the
// ProjectV2 connection on Issue + PullRequest. Real lookups against
// the ProjectV2Store; resolvers read from the source map populated by
// projectItemsForGraphQL.
var (
	projectV2TypeMemo                *graphql.Object
	projectV2FieldTypeMemo           *graphql.Object
	projectV2FieldConnectionMemo     *graphql.Object
	projectV2ViewTypeMemo            *graphql.Object
	projectV2ViewConnectionMemo      *graphql.Object
	projectV2ItemTypeMemo            *graphql.Object
	projectV2ItemConnectionTypeMemo  *graphql.Object
	projectV2ItemsFieldAdded         bool
	projectV2SingleSelectValueMemo   *graphql.Object
	projectV2TextValueMemo           *graphql.Object
	projectV2NumberValueMemo         *graphql.Object
	projectV2DateValueMemo           *graphql.Object
	projectV2IterationValueMemo      *graphql.Object
	projectV2ItemFieldValueUnionMemo *graphql.Union
)

func projectV2GraphQLTypes() *graphql.Object {
	if projectV2TypeMemo != nil {
		return projectV2TypeMemo
	}
	projectV2TypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2",
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
			"number": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"closed": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"public": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"url":    &graphql.Field{Type: graphql.String},
		},
	})
	projectV2TypeMemo.AddFieldConfig("fields", &graphql.Field{
		Type: projectV2FieldConnectionType(),
		Args: relayConnectionArgs(),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			st, projectID, err := projectV2SourceStoreAndID(p.Source)
			if err != nil {
				return nil, err
			}
			fields := st.ProjectsV2.FieldsForProject(projectID)
			nodes := make([]map[string]interface{}, 0, len(fields))
			for _, f := range fields {
				nodes = append(nodes, projectV2FieldToGQL(f))
			}
			return paginateGQLMaps(nodes, p.Args), nil
		},
	})
	projectV2TypeMemo.AddFieldConfig("views", &graphql.Field{
		Type: projectV2ViewConnectionType(),
		Args: relayConnectionArgs(),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			st, projectID, err := projectV2SourceStoreAndID(p.Source)
			if err != nil {
				return nil, err
			}
			views := st.ProjectsV2.ViewsForProject(projectID)
			nodes := make([]map[string]interface{}, 0, len(views))
			for _, v := range views {
				nodes = append(nodes, projectV2ViewToGQL(v))
			}
			return paginateGQLMaps(nodes, p.Args), nil
		},
	})
	return projectV2TypeMemo
}

func ensureProjectV2ItemsField() {
	if projectV2TypeMemo == nil || projectV2ItemConnectionTypeMemo == nil || projectV2ItemsFieldAdded {
		return
	}
	projectV2TypeMemo.AddFieldConfig("items", &graphql.Field{
		Type: projectV2ItemConnectionTypeMemo,
		Args: relayConnectionArgs(),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			st, projectID, err := projectV2SourceStoreAndID(p.Source)
			if err != nil {
				return nil, err
			}
			items := st.ProjectsV2.ListItemsForProject(projectID)
			nodes := make([]map[string]interface{}, 0, len(items))
			for _, it := range items {
				nodes = append(nodes, projectV2ItemToGQL(it, st))
			}
			return paginateGQLMaps(nodes, p.Args), nil
		},
	})
	projectV2ItemsFieldAdded = true
}

func projectV2FieldConnectionType() *graphql.Object {
	if projectV2FieldConnectionMemo != nil {
		return projectV2FieldConnectionMemo
	}
	optionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2SingleSelectFieldOption",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"name":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"color":       &graphql.Field{Type: graphql.String},
			"description": &graphql.Field{Type: graphql.String},
		},
	})
	iterationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2Iteration",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"title":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"startDate": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"duration":  &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})
	iterationConfigurationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2IterationConfiguration",
		Fields: graphql.Fields{
			"startDate":  &graphql.Field{Type: graphql.String},
			"duration":   &graphql.Field{Type: graphql.Int},
			"iterations": &graphql.Field{Type: graphql.NewList(iterationType)},
		},
	})
	projectV2FieldTypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2Field",
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
			"name":                   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"dataType":               &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"options":                &graphql.Field{Type: graphql.NewList(optionType)},
			"iterationConfiguration": &graphql.Field{Type: iterationConfigurationType},
			"createdAt":              &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":              &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	projectV2FieldConnectionMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2FieldConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"nodes":      &graphql.Field{Type: graphql.NewList(projectV2FieldTypeMemo)},
			"edges":      &graphql.Field{Type: graphql.NewList(graphql.NewObject(graphql.ObjectConfig{Name: "ProjectV2FieldEdge", Fields: graphql.Fields{"node": &graphql.Field{Type: projectV2FieldTypeMemo}, "cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)}}}))},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(gqlPageInfoType())},
		},
	})
	return projectV2FieldConnectionMemo
}

func projectV2ViewConnectionType() *graphql.Object {
	if projectV2ViewConnectionMemo != nil {
		return projectV2ViewConnectionMemo
	}
	projectV2ViewTypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2View",
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
			"number":    &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"name":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"layout":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"filter":    &graphql.Field{Type: graphql.String},
			"createdAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"visibleFieldIds": &graphql.Field{
				Type: graphql.NewList(graphql.NewNonNull(graphql.Int)),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["visibleFieldIds"], nil
				},
			},
		},
	})
	projectV2ViewConnectionMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ViewConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"nodes":      &graphql.Field{Type: graphql.NewList(projectV2ViewTypeMemo)},
			"edges":      &graphql.Field{Type: graphql.NewList(graphql.NewObject(graphql.ObjectConfig{Name: "ProjectV2ViewEdge", Fields: graphql.Fields{"node": &graphql.Field{Type: projectV2ViewTypeMemo}, "cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)}}}))},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(gqlPageInfoType())},
		},
	})
	return projectV2ViewConnectionMemo
}

func projectV2ItemConnectionType() *graphql.Object {
	if projectV2ItemConnectionTypeMemo != nil {
		return projectV2ItemConnectionTypeMemo
	}
	projectV2Type := projectV2GraphQLTypes()
	projectV2SingleSelectValueMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemFieldSingleSelectValue",
		Fields: graphql.Fields{
			"optionId": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["optionId"], nil
				},
			},
			"name": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["name"], nil
				},
			},
		},
	})
	projectV2TextValueMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemFieldTextValue",
		Fields: graphql.Fields{
			"text": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["text"], nil
				},
			},
		},
	})
	projectV2NumberValueMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemFieldNumberValue",
		Fields: graphql.Fields{
			"number": &graphql.Field{
				Type: graphql.Float,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["number"], nil
				},
			},
		},
	})
	projectV2DateValueMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemFieldDateValue",
		Fields: graphql.Fields{
			"date": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["date"], nil
				},
			},
		},
	})
	projectV2IterationValueMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemFieldIterationValue",
		Fields: graphql.Fields{
			"iterationId": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["iterationId"], nil
				},
			},
			"title": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["title"], nil
				},
			},
			"startDate": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["startDate"], nil
				},
			},
			"duration": &graphql.Field{
				Type: graphql.Int,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["duration"], nil
				},
			},
		},
	})
	projectV2ItemFieldValueUnionMemo = graphql.NewUnion(graphql.UnionConfig{
		Name:  "ProjectV2ItemFieldValue",
		Types: []*graphql.Object{projectV2SingleSelectValueMemo, projectV2TextValueMemo, projectV2NumberValueMemo, projectV2DateValueMemo, projectV2IterationValueMemo},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			src, _ := p.Value.(map[string]interface{})
			switch src["kind"] {
			case string(ProjectV2FieldText):
				return projectV2TextValueMemo
			case string(ProjectV2FieldNumber):
				return projectV2NumberValueMemo
			case string(ProjectV2FieldDate):
				return projectV2DateValueMemo
			case string(ProjectV2FieldIteration):
				return projectV2IterationValueMemo
			default:
				return projectV2SingleSelectValueMemo
			}
		},
	})
	projectV2ItemTypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2Item",
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
			"project": &graphql.Field{
				Type: projectV2Type,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return src["project"], nil
				},
			},
			// fieldValueByName — looks up the named field on the item's
			// project, returns the stored value (nil when unset).
			"fieldValueByName": &graphql.Field{
				Type: projectV2ItemFieldValueUnionMemo,
				Args: graphql.FieldConfigArgument{
					"name": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					src, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					name, _ := p.Args["name"].(string)
					byName, _ := src["fieldValuesByName"].(map[string]interface{})
					if byName == nil {
						return nil, nil
					}
					v, ok := byName[name]
					if !ok {
						return nil, nil
					}
					return v, nil
				},
			},
		},
	})
	projectV2ItemEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemEdge",
		Fields: graphql.Fields{
			"node":   &graphql.Field{Type: projectV2ItemTypeMemo},
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	projectV2ItemConnectionTypeMemo = graphql.NewObject(graphql.ObjectConfig{
		Name: "ProjectV2ItemConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"nodes":      &graphql.Field{Type: graphql.NewList(projectV2ItemTypeMemo)},
			"edges":      &graphql.Field{Type: graphql.NewList(projectV2ItemEdgeType)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(gqlPageInfoType())},
		},
	})
	ensureProjectV2ItemsField()
	return projectV2ItemConnectionTypeMemo
}

// projectV2ItemToGQL builds the GraphQL source map for a single project
// item, embedding the parent project's map so ProjectV2Item.project
// resolves cleanly without a second lookup. Field values are
// pre-resolved into fieldValuesByName so the fieldValueByName(name:)
// resolver is a direct map lookup.
func projectV2ItemToGQL(it *ProjectV2Item, st *Store) map[string]interface{} {
	var projectMap map[string]interface{}
	if p := st.ProjectsV2.GetProject(it.ProjectID); p != nil {
		projectMap = projectV2ToGQL(p, st)
	}
	byName := map[string]interface{}{}
	for fieldID, val := range it.FieldValues {
		field := st.ProjectsV2.GetField(fieldID)
		if field == nil {
			continue
		}
		byName[field.Name] = projectV2FieldValueToGQL(val, field)
	}
	return map[string]interface{}{
		"nodeID":            it.NodeID,
		"project":           projectMap,
		"fieldValuesByName": byName,
	}
}

// projectV2FieldValueToGQL renders a persisted ProjectV2 field value as
// the matching GraphQL union source map.
func projectV2FieldValueToGQL(v *ProjectV2ItemFieldValue, f *ProjectV2Field) map[string]interface{} {
	out := map[string]interface{}{"kind": string(f.DataType)}
	switch f.DataType {
	case ProjectV2FieldText:
		out["text"] = v.TextValue
	case ProjectV2FieldNumber:
		out["number"] = v.NumberValue
	case ProjectV2FieldDate:
		out["date"] = v.DateValue
	case ProjectV2FieldIteration:
		out["iterationId"] = v.IterationID
		if f.Iteration != nil {
			for _, it := range f.Iteration.Iterations {
				if it.ID == v.IterationID {
					out["title"] = it.Title
					out["startDate"] = it.StartDate
					out["duration"] = it.Duration
					break
				}
			}
		}
	default:
		out["optionId"] = v.OptionID
		out["name"] = v.OptionName
	}
	return out
}

// projectV2ToGQL renders a project as a GraphQL source map.
func projectV2ToGQL(p *ProjectV2, st *Store) map[string]interface{} {
	return map[string]interface{}{
		"id":     p.ID,
		"store":  st,
		"nodeID": p.NodeID,
		"number": p.Number,
		"title":  p.Title,
		"closed": p.Closed,
		"public": p.Public,
		"url":    p.URL,
	}
}

func projectV2SourceStoreAndID(source interface{}) (*Store, int, error) {
	src, ok := source.(map[string]interface{})
	if !ok {
		return nil, 0, fmt.Errorf("resolve source: unexpected type %T", source)
	}
	st, ok := src["store"].(*Store)
	if !ok || st == nil {
		return nil, 0, fmt.Errorf("project source missing store")
	}
	id, ok := src["id"].(int)
	if !ok || id == 0 {
		return nil, 0, fmt.Errorf("project source missing id")
	}
	return st, id, nil
}

func projectV2FieldToGQL(f *ProjectV2Field) map[string]interface{} {
	options := make([]map[string]interface{}, 0, len(f.Options))
	for _, opt := range f.Options {
		options = append(options, map[string]interface{}{
			"id":          opt.ID,
			"name":        opt.Name,
			"color":       opt.Color,
			"description": opt.Description,
		})
	}
	var iteration map[string]interface{}
	if f.Iteration != nil {
		iterations := make([]map[string]interface{}, 0, len(f.Iteration.Iterations))
		for _, it := range f.Iteration.Iterations {
			iterations = append(iterations, map[string]interface{}{
				"id":        it.ID,
				"title":     it.Title,
				"startDate": it.StartDate,
				"duration":  it.Duration,
			})
		}
		iteration = map[string]interface{}{
			"startDate":  f.Iteration.StartDate,
			"duration":   f.Iteration.Duration,
			"iterations": iterations,
		}
	}
	return map[string]interface{}{
		"nodeID":                 f.NodeID,
		"name":                   f.Name,
		"dataType":               string(f.DataType),
		"options":                options,
		"iterationConfiguration": iteration,
		"createdAt":              f.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":              f.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func projectV2ViewToGQL(v *ProjectV2View) map[string]interface{} {
	var filter interface{}
	if v.Filter != nil {
		filter = *v.Filter
	}
	visible := append([]int(nil), v.VisibleFields...)
	return map[string]interface{}{
		"nodeID":          v.NodeID,
		"number":          v.Number,
		"name":            v.Name,
		"layout":          v.Layout,
		"filter":          filter,
		"visibleFieldIds": visible,
		"createdAt":       v.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":       v.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// projectItemsConnectionForIssue returns the source map for the
// Issue.projectItems / PullRequest.projectItems connection.
func projectItemsConnectionForIssue(st *Store, issueID int, args map[string]interface{}) map[string]interface{} {
	items := st.ProjectsV2.ListItemsForIssue(issueID)
	nodes := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		nodes = append(nodes, projectV2ItemToGQL(it, st))
	}
	return paginateGQLMaps(nodes, args)
}
