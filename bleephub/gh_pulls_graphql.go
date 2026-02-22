package bleephub

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
)

// addPullRequestFieldsToSchema adds PR types, queries, and mutations to the schema.
func (s *Server) addPullRequestFieldsToSchema(userType, issueType, repoType, mutationType, queryType *graphql.Object) {
	// --- Enums ---
	pullRequestStateEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "PullRequestState",
		Values: graphql.EnumValueConfigMap{
			"OPEN":   &graphql.EnumValueConfig{Value: "OPEN"},
			"CLOSED": &graphql.EnumValueConfig{Value: "CLOSED"},
			"MERGED": &graphql.EnumValueConfig{Value: "MERGED"},
		},
	})

	mergeableStateEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "MergeableState",
		Values: graphql.EnumValueConfigMap{
			"MERGEABLE":   &graphql.EnumValueConfig{Value: "MERGEABLE"},
			"CONFLICTING": &graphql.EnumValueConfig{Value: "CONFLICTING"},
			"UNKNOWN":     &graphql.EnumValueConfig{Value: "UNKNOWN"},
		},
	})

	_ = mergeableStateEnum // used as field type below

	pullRequestMergeMethodEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "PullRequestMergeMethod",
		Values: graphql.EnumValueConfigMap{
			"MERGE":  &graphql.EnumValueConfig{Value: "MERGE"},
			"SQUASH": &graphql.EnumValueConfig{Value: "SQUASH"},
			"REBASE": &graphql.EnumValueConfig{Value: "REBASE"},
		},
	})

	pullRequestReviewDecisionEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "PullRequestReviewDecision",
		Values: graphql.EnumValueConfigMap{
			"APPROVED":          &graphql.EnumValueConfig{Value: "APPROVED"},
			"CHANGES_REQUESTED": &graphql.EnumValueConfig{Value: "CHANGES_REQUESTED"},
			"REVIEW_REQUIRED":   &graphql.EnumValueConfig{Value: "REVIEW_REQUIRED"},
		},
	})

	_ = pullRequestReviewDecisionEnum // used in PR type

	// --- PR Label types (PR-prefixed to avoid name collision) ---
	prLabelType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRLabel",
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

	prLabelPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRLabelPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	prLabelConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRLabelConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(prLabelType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(prLabelPageInfoType)},
		},
	})

	// --- PR Assignee connection ---
	prAssigneePageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRAssigneePageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	prAssigneeConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRUserConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(userType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(prAssigneePageInfoType)},
		},
	})

	// --- Review types ---
	prReviewType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReview",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r := p.Source.(map[string]interface{})
					return r["nodeID"], nil
				},
			},
			"body":  &graphql.Field{Type: graphql.String},
			"state": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r := p.Source.(map[string]interface{})
					return r["author"], nil
				},
			},
			"authorAssociation": &graphql.Field{Type: graphql.String},
			"createdAt":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	prReviewPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRReviewPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	prReviewConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReviewConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(prReviewType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(prReviewPageInfoType)},
		},
	})

	// --- Review request types ---
	reviewRequestType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReviewRequest",
		Fields: graphql.Fields{
			"requestedReviewer": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r := p.Source.(map[string]interface{})
					return r["requestedReviewer"], nil
				},
			},
		},
	})

	reviewRequestConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReviewRequestConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(reviewRequestType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	// --- Reaction group type for PRs ---
	prReactionGroupType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRReactionGroup",
		Fields: graphql.Fields{
			"content": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"users": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "PRReactingUserConnection",
					Fields: graphql.Fields{
						"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
					},
				}),
			},
		},
	})

	// --- PR Comment connection ---
	prCommentPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRCommentPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	prCommentConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRCommentConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(prCommentPageInfoType)},
		},
	})

	// --- PR Commit connection ---
	prCommitConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestCommitConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	// --- StatusCheckRollup (stub for gh pr view) ---
	statusCheckRollupType := graphql.NewObject(graphql.ObjectConfig{
		Name: "StatusCheckRollup",
		Fields: graphql.Fields{
			"contexts": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "StatusCheckRollupContextConnection",
					Fields: graphql.Fields{
						"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
					},
				}),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{"totalCount": 0}, nil
				},
			},
		},
	})

	// --- PullRequest type ---
	pullRequestType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequest",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["nodeID"], nil
				},
			},
			"databaseId":  &graphql.Field{Type: graphql.Int},
			"number":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"body":        &graphql.Field{Type: graphql.String},
			"state":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"isDraft":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"url":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"headRefName": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"baseRefName": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"headRefOid":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"mergeable":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"merged":      &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"mergedAt":    &graphql.Field{Type: graphql.String},
			"additions":   &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"deletions":   &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"changedFiles": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"createdAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"closedAt":    &graphql.Field{Type: graphql.String},
			"reviewDecision": &graphql.Field{Type: graphql.String},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["author"], nil
				},
			},
			"mergedBy": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["mergedBy"], nil
				},
			},
			"labels": &graphql.Field{
				Type: prLabelConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["labels"], nil
				},
			},
			"assignees": &graphql.Field{
				Type: prAssigneeConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["assignees"], nil
				},
			},
			"reviews": &graphql.Field{
				Type: prReviewConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"last":  &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["reviews"], nil
				},
			},
			"reviewRequests": &graphql.Field{
				Type: reviewRequestConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["reviewRequests"], nil
				},
			},
			"comments": &graphql.Field{
				Type: prCommentConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["comments"], nil
				},
			},
			"commits": &graphql.Field{
				Type: prCommitConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr := p.Source.(map[string]interface{})
					return pr["commits"], nil
				},
			},
			"statusCheckRollup": &graphql.Field{
				Type: statusCheckRollupType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return map[string]interface{}{}, nil
				},
			},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(prReactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return prStaticReactionGroups(), nil
				},
			},
		},
	})

	// --- PR Connection ---
	prPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	prEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestEdge",
		Fields: graphql.Fields{
			"node":   &graphql.Field{Type: pullRequestType},
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	prConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(pullRequestType)},
			"edges":      &graphql.Field{Type: graphql.NewList(prEdgeType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(prPageInfoType)},
		},
	})

	// --- Add fields to Repository type ---

	repoType.AddFieldConfig("pullRequests", &graphql.Field{
		Type: prConnectionType,
		Args: graphql.FieldConfigArgument{
			"first":  &graphql.ArgumentConfig{Type: graphql.Int},
			"after":  &graphql.ArgumentConfig{Type: graphql.String},
			"states": &graphql.ArgumentConfig{Type: graphql.NewList(pullRequestStateEnum)},
			"labels": &graphql.ArgumentConfig{Type: graphql.NewList(graphql.String)},
			"headRefName": &graphql.ArgumentConfig{Type: graphql.String},
			"baseRefName": &graphql.ArgumentConfig{Type: graphql.String},
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "IssueOrder2",
				Fields: graphql.InputObjectConfigFieldMap{
					"field":     &graphql.InputObjectFieldConfig{Type: graphql.String},
					"direction": &graphql.InputObjectFieldConfig{Type: graphql.String},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)

			prs := s.store.ListPullRequests(repoID, "")

			// Filter by states
			if states, ok := p.Args["states"].([]interface{}); ok && len(states) > 0 {
				stateMap := make(map[string]bool)
				for _, st := range states {
					stateMap[fmt.Sprintf("%v", st)] = true
				}
				var filtered []*PullRequest
				for _, pr := range prs {
					if stateMap[pr.State] {
						filtered = append(filtered, pr)
					}
				}
				prs = filtered
			}

			// Filter by labels
			if labelNames, ok := p.Args["labels"].([]interface{}); ok && len(labelNames) > 0 {
				var names []string
				for _, ln := range labelNames {
					names = append(names, fmt.Sprintf("%v", ln))
				}
				var filtered []*PullRequest
				for _, pr := range prs {
					if prHasAllLabels(s.store, pr, names) {
						filtered = append(filtered, pr)
					}
				}
				prs = filtered
			}

			// Filter by headRefName
			if head, ok := p.Args["headRefName"].(string); ok && head != "" {
				var filtered []*PullRequest
				for _, pr := range prs {
					if pr.HeadRefName == head {
						filtered = append(filtered, pr)
					}
				}
				prs = filtered
			}

			// Filter by baseRefName
			if base, ok := p.Args["baseRefName"].(string); ok && base != "" {
				var filtered []*PullRequest
				for _, pr := range prs {
					if pr.BaseRefName == base {
						filtered = append(filtered, pr)
					}
				}
				prs = filtered
			}

			// Sort newest first
			sort.Slice(prs, func(a, b int) bool {
				return prs[a].CreatedAt.After(prs[b].CreatedAt)
			})

			first := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				first = f
			}
			after, _ := p.Args["after"].(string)

			return paginatePullRequestsGQL(prs, s.store, first, after), nil
		},
	})

	repoType.AddFieldConfig("pullRequest", &graphql.Field{
		Type: pullRequestType,
		Args: graphql.FieldConfigArgument{
			"number": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)
			number, _ := p.Args["number"].(int)

			pr := s.store.GetPullRequestByNumber(repoID, number)
			if pr == nil {
				return nil, nil
			}
			return pullRequestToGQL(pr, s.store), nil
		},
	})

	// --- Mutations ---

	createPRInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreatePullRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"repositoryId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"title":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"body":         &graphql.InputObjectFieldConfig{Type: graphql.String},
			"headRefName":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"baseRefName":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"draft":        &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
		},
	})

	createPRPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CreatePullRequestPayload",
		Fields: graphql.Fields{
			"pullRequest": &graphql.Field{Type: pullRequestType},
		},
	})

	mutationType.AddFieldConfig("createPullRequest", &graphql.Field{
		Type: createPRPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createPRInputType)},
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
			headRefName, _ := input["headRefName"].(string)
			baseRefName, _ := input["baseRefName"].(string)
			draft, _ := input["draft"].(bool)

			repo := findRepoByNodeID(s.store, repoNodeID)
			if repo == nil {
				return nil, fmt.Errorf("could not resolve to a Repository with the global id of '%s'", repoNodeID)
			}

			pr := s.store.CreatePullRequest(repo.ID, user.ID, title, body, headRefName, baseRefName, draft, nil, nil, 0)
			if pr == nil {
				return nil, fmt.Errorf("pull request creation failed")
			}

			return map[string]interface{}{
				"pullRequest": pullRequestToGQL(pr, s.store),
			}, nil
		},
	})

	closePRInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ClosePullRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"pullRequestId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})

	closePRPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ClosePullRequestPayload",
		Fields: graphql.Fields{
			"pullRequest": &graphql.Field{Type: pullRequestType},
		},
	})

	mutationType.AddFieldConfig("closePullRequest", &graphql.Field{
		Type: closePRPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(closePRInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			prNodeID, _ := input["pullRequestId"].(string)

			pr := findPullRequestByNodeID(s.store, prNodeID)
			if pr == nil {
				return nil, fmt.Errorf("could not resolve to a PullRequest")
			}

			s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				p.State = "CLOSED"
				now := time.Now()
				p.ClosedAt = &now
			})

			updated := s.store.GetPullRequest(pr.ID)
			return map[string]interface{}{
				"pullRequest": pullRequestToGQL(updated, s.store),
			}, nil
		},
	})

	reopenPRInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ReopenPullRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"pullRequestId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})

	reopenPRPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReopenPullRequestPayload",
		Fields: graphql.Fields{
			"pullRequest": &graphql.Field{Type: pullRequestType},
		},
	})

	mutationType.AddFieldConfig("reopenPullRequest", &graphql.Field{
		Type: reopenPRPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(reopenPRInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			prNodeID, _ := input["pullRequestId"].(string)

			pr := findPullRequestByNodeID(s.store, prNodeID)
			if pr == nil {
				return nil, fmt.Errorf("could not resolve to a PullRequest")
			}

			if pr.State == "MERGED" {
				return nil, fmt.Errorf("pull request is merged and cannot be reopened")
			}

			s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				p.State = "OPEN"
				p.ClosedAt = nil
			})

			updated := s.store.GetPullRequest(pr.ID)
			return map[string]interface{}{
				"pullRequest": pullRequestToGQL(updated, s.store),
			}, nil
		},
	})

	mergePRInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "MergePullRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"pullRequestId":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"mergeMethod":    &graphql.InputObjectFieldConfig{Type: pullRequestMergeMethodEnum},
			"commitHeadline": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"commitBody":     &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})

	mergePRPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MergePullRequestPayload",
		Fields: graphql.Fields{
			"pullRequest": &graphql.Field{Type: pullRequestType},
		},
	})

	mutationType.AddFieldConfig("mergePullRequest", &graphql.Field{
		Type: mergePRPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(mergePRInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			prNodeID, _ := input["pullRequestId"].(string)

			pr := findPullRequestByNodeID(s.store, prNodeID)
			if pr == nil {
				return nil, fmt.Errorf("could not resolve to a PullRequest")
			}

			if pr.State != "OPEN" {
				return nil, fmt.Errorf("pull request is not open")
			}

			s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				now := time.Now()
				p.State = "MERGED"
				p.MergedAt = &now
				p.ClosedAt = &now
				p.MergedByID = user.ID
			})

			updated := s.store.GetPullRequest(pr.ID)
			return map[string]interface{}{
				"pullRequest": pullRequestToGQL(updated, s.store),
			}, nil
		},
	})

	updatePRInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UpdatePullRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"pullRequestId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"title":         &graphql.InputObjectFieldConfig{Type: graphql.String},
			"body":          &graphql.InputObjectFieldConfig{Type: graphql.String},
			"baseRefName":   &graphql.InputObjectFieldConfig{Type: graphql.String},
			"labelIds":      &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
			"assigneeIds":   &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.ID)},
			"milestoneId":   &graphql.InputObjectFieldConfig{Type: graphql.ID},
		},
	})

	updatePRPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UpdatePullRequestPayload",
		Fields: graphql.Fields{
			"pullRequest": &graphql.Field{Type: pullRequestType},
		},
	})

	mutationType.AddFieldConfig("updatePullRequest", &graphql.Field{
		Type: updatePRPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(updatePRInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			prNodeID, _ := input["pullRequestId"].(string)

			pr := findPullRequestByNodeID(s.store, prNodeID)
			if pr == nil {
				return nil, fmt.Errorf("could not resolve to a PullRequest")
			}

			// Resolve label IDs
			var labelIDs []int
			if rawLabels, ok := input["labelIds"].([]interface{}); ok {
				for _, raw := range rawLabels {
					nodeID := fmt.Sprintf("%v", raw)
					if l := findLabelByNodeID(s.store, nodeID); l != nil {
						labelIDs = append(labelIDs, l.ID)
					}
				}
			}

			// Resolve assignee IDs
			var assigneeIDs []int
			if rawAssignees, ok := input["assigneeIds"].([]interface{}); ok {
				for _, raw := range rawAssignees {
					nodeID := fmt.Sprintf("%v", raw)
					if u := findUserByNodeID(s.store, nodeID); u != nil {
						assigneeIDs = append(assigneeIDs, u.ID)
					}
				}
			}

			s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				if v, ok := input["title"].(string); ok {
					p.Title = v
				}
				if v, ok := input["body"].(string); ok {
					p.Body = v
				}
				if v, ok := input["baseRefName"].(string); ok {
					p.BaseRefName = v
				}
				if rawLabels, ok := input["labelIds"].([]interface{}); ok && rawLabels != nil {
					p.LabelIDs = labelIDs
				}
				if rawAssignees, ok := input["assigneeIds"].([]interface{}); ok && rawAssignees != nil {
					p.AssigneeIDs = assigneeIDs
				}
			})

			updated := s.store.GetPullRequest(pr.ID)
			return map[string]interface{}{
				"pullRequest": pullRequestToGQL(updated, s.store),
			}, nil
		},
	})

	// Update issueOrPullRequest to also check PRs
	// (it's already defined in gh_issues_graphql.go for issues;
	// we can't redefine it, but the resolver there already only returns issues.
	// For completeness we'd need to update it, but gh pr view uses pullRequest(number) directly.)
}

// --- GraphQL converter helpers ---

func pullRequestToGQL(pr *PullRequest, st *Store) map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()

	// Author
	var author map[string]interface{}
	if u, ok := st.Users[pr.AuthorID]; ok {
		author = userToGraphQL(u)
	}

	// MergedBy
	var mergedBy map[string]interface{}
	if pr.MergedByID > 0 {
		if u, ok := st.Users[pr.MergedByID]; ok {
			mergedBy = userToGraphQL(u)
		}
	}

	// Labels
	labelNodes := make([]map[string]interface{}, 0)
	for _, lid := range pr.LabelIDs {
		if l, ok := st.Labels[lid]; ok {
			labelNodes = append(labelNodes, labelToGQL(l))
		}
	}

	// Assignees
	assigneeNodes := make([]map[string]interface{}, 0)
	for _, aid := range pr.AssigneeIDs {
		if u, ok := st.Users[aid]; ok {
			assigneeNodes = append(assigneeNodes, userToGraphQL(u))
		}
	}

	// Reviews (inline to avoid deadlock)
	reviewNodes := make([]map[string]interface{}, 0)
	for _, r := range st.PRReviews {
		if r.PRID == pr.ID {
			var reviewAuthor map[string]interface{}
			if u, ok := st.Users[r.AuthorID]; ok {
				reviewAuthor = userToGraphQL(u)
			}
			reviewNodes = append(reviewNodes, map[string]interface{}{
				"nodeID":              r.NodeID,
				"body":                r.Body,
				"state":               r.State,
				"author":              reviewAuthor,
				"authorAssociation":   "OWNER",
				"createdAt":           r.CreatedAt.Format(time.RFC3339),
				"updatedAt":           r.UpdatedAt.Format(time.RFC3339),
			})
		}
	}

	// Derive review decision
	var reviewDecision interface{}
	if rd := deriveReviewDecisionLocked(st, pr.ID); rd != "" {
		reviewDecision = rd
	}

	// URL
	repo := st.Repos[pr.RepoID]
	url := ""
	if repo != nil {
		url = "/" + repo.FullName + "/pull/" + fmt.Sprintf("%d", pr.Number)
	}

	sha := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("head-%d", pr.ID))))[:40]

	var closedAt interface{}
	if pr.ClosedAt != nil {
		closedAt = pr.ClosedAt.Format(time.RFC3339)
	}
	var mergedAt interface{}
	if pr.MergedAt != nil {
		mergedAt = pr.MergedAt.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"__typename":     "PullRequest",
		"nodeID":         pr.NodeID,
		"databaseId":     pr.ID,
		"number":         pr.Number,
		"title":          pr.Title,
		"body":           pr.Body,
		"state":          pr.State,
		"isDraft":        pr.IsDraft,
		"url":            url,
		"headRefName":    pr.HeadRefName,
		"baseRefName":    pr.BaseRefName,
		"headRefOid":     sha,
		"mergeable":      pr.Mergeable,
		"merged":         pr.State == "MERGED",
		"mergedAt":       mergedAt,
		"mergedBy":       mergedBy,
		"additions":      pr.Additions,
		"deletions":      pr.Deletions,
		"changedFiles":   pr.ChangedFiles,
		"reviewDecision": reviewDecision,
		"author":         author,
		"createdAt":      pr.CreatedAt.Format(time.RFC3339),
		"updatedAt":      pr.UpdatedAt.Format(time.RFC3339),
		"closedAt":       closedAt,
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
		"reviews": map[string]interface{}{
			"nodes":      reviewNodes,
			"totalCount": len(reviewNodes),
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
		"reviewRequests": map[string]interface{}{
			"nodes":      []interface{}{},
			"totalCount": 0,
		},
		"comments": map[string]interface{}{
			"totalCount": 0,
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
		"commits": map[string]interface{}{
			"totalCount": 1,
		},
	}
}

// deriveReviewDecisionLocked derives the review decision from reviews.
// Must be called while holding st.mu.RLock().
func deriveReviewDecisionLocked(st *Store, prID int) string {
	hasApproved := false
	hasChangesRequested := false
	for _, r := range st.PRReviews {
		if r.PRID != prID {
			continue
		}
		switch r.State {
		case "APPROVED":
			hasApproved = true
		case "CHANGES_REQUESTED":
			hasChangesRequested = true
		}
	}
	if hasChangesRequested {
		return "CHANGES_REQUESTED"
	}
	if hasApproved {
		return "APPROVED"
	}
	return ""
}

func findPullRequestByNodeID(st *Store, nodeID string) *PullRequest {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, pr := range st.PullRequests {
		if pr.NodeID == nodeID {
			return pr
		}
	}
	return nil
}

func prHasAllLabels(st *Store, pr *PullRequest, labelNames []string) bool {
	for _, name := range labelNames {
		found := false
		for _, lid := range pr.LabelIDs {
			l := st.GetLabel(lid)
			if l != nil && l.Name == strings.TrimSpace(name) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// paginatePullRequestsGQL implements Relay-style cursor pagination for PRs.
func paginatePullRequestsGQL(prs []*PullRequest, st *Store, first int, after string) map[string]interface{} {
	total := len(prs)

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

	page := prs[startIdx:endIdx]

	nodes := make([]map[string]interface{}, 0, len(page))
	edges := make([]map[string]interface{}, 0, len(page))
	for idx, pr := range page {
		gql := pullRequestToGQL(pr, st)
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

func prStaticReactionGroups() []map[string]interface{} {
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
