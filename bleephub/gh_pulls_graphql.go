package bleephub

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
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

	// MergeStateStatus carries real GitHub's full value set; bleephub derives
	// CLEAN/DIRTY/UNKNOWN from the PR's stored mergeability (the only merge
	// gates it models).
	mergeStateStatusEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "MergeStateStatus",
		Values: graphql.EnumValueConfigMap{
			"BEHIND":    &graphql.EnumValueConfig{Value: "BEHIND"},
			"BLOCKED":   &graphql.EnumValueConfig{Value: "BLOCKED"},
			"CLEAN":     &graphql.EnumValueConfig{Value: "CLEAN"},
			"DIRTY":     &graphql.EnumValueConfig{Value: "DIRTY"},
			"DRAFT":     &graphql.EnumValueConfig{Value: "DRAFT"},
			"HAS_HOOKS": &graphql.EnumValueConfig{Value: "HAS_HOOKS"},
			"UNKNOWN":   &graphql.EnumValueConfig{Value: "UNKNOWN"},
			"UNSTABLE":  &graphql.EnumValueConfig{Value: "UNSTABLE"},
		},
	})

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

	// --- PR Label types (PR-prefixed to avoid name collision) ---
	prLabelType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRLabel",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					l, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
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

	// --- Commit + status-check rollup types ---
	// gh selects PR check state through the commits connection:
	//   statusCheckRollup: commits(last:1){nodes{commit{statusCheckRollup{
	//     contexts(first:100){nodes{...on StatusContext, ...on CheckRun}}}}}}
	// and the merge path's lastCommit pseudo-field as commits(last:1){nodes
	// {commit{oid}}}. CheckRun nodes are backed by the real checks store and
	// StatusContext nodes are backed by the real REST commit-status store.
	statusContextType := graphql.NewObject(graphql.ObjectConfig{
		Name: "StatusContext",
		Fields: graphql.Fields{
			"context":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"state":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"targetUrl":   &graphql.Field{Type: graphql.String},
			"createdAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description": &graphql.Field{Type: graphql.String},
		},
	})

	checkRunType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CheckRun",
		Fields: graphql.Fields{
			"name":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"status":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"conclusion": &graphql.Field{Type: graphql.String},
			"startedAt":  &graphql.Field{Type: graphql.String},
			"completedAt": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					cr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return cr["completedAt"], nil
				},
			},
			"detailsUrl": &graphql.Field{Type: graphql.String},
			"checkSuite": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "CheckSuiteRef",
					Fields: graphql.Fields{
						"workflowRun": &graphql.Field{
							Type: graphql.NewObject(graphql.ObjectConfig{
								Name: "CheckWorkflowRun",
								Fields: graphql.Fields{
									"workflow": &graphql.Field{
										Type: graphql.NewObject(graphql.ObjectConfig{
											Name: "CheckWorkflow",
											Fields: graphql.Fields{
												"name": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
											},
										}),
									},
								},
							}),
							Resolve: func(p graphql.ResolveParams) (interface{}, error) {
								suite, ok := p.Source.(map[string]interface{})
								if !ok {
									return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
								}
								return suite["workflowRun"], nil
							},
						},
					},
				}),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					cr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return cr["checkSuite"], nil
				},
			},
		},
	})

	statusCheckRollupContextUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:  "StatusCheckRollupContext",
		Types: []*graphql.Object{statusContextType, checkRunType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			if m, ok := p.Value.(map[string]interface{}); ok {
				if tn, _ := m["__typename"].(string); tn == "StatusContext" {
					return statusContextType
				}
			}
			return checkRunType
		},
	})

	statusCheckStateCountType := func(name string) *graphql.Object {
		return graphql.NewObject(graphql.ObjectConfig{
			Name: name,
			Fields: graphql.Fields{
				"state": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
				"count": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			},
		})
	}

	statusCheckRollupContextConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "StatusCheckRollupContextConnection",
		Fields: graphql.Fields{
			"nodes":                      &graphql.Field{Type: graphql.NewList(statusCheckRollupContextUnion)},
			"totalCount":                 &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"checkRunCount":              &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"checkRunCountsByState":      &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(statusCheckStateCountType("CheckRunStateCount"))))},
			"statusContextCount":         &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"statusContextCountsByState": &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(statusCheckStateCountType("StatusContextStateCount"))))},
			"pageInfo": &graphql.Field{Type: graphql.NewNonNull(graphql.NewObject(graphql.ObjectConfig{
				Name: "StatusCheckRollupContextPageInfo",
				Fields: graphql.Fields{
					"hasNextPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
					"endCursor":   &graphql.Field{Type: graphql.String},
				},
			}))},
		},
	})

	statusCheckRollupType := graphql.NewObject(graphql.ObjectConfig{
		Name: "StatusCheckRollup",
		Fields: graphql.Fields{
			"state": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"contexts": &graphql.Field{
				Type: graphql.NewNonNull(statusCheckRollupContextConnectionType),
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["contexts"], nil
				},
			},
		},
	})

	gitActorConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "GitActorConnection",
		Fields: graphql.Fields{
			"nodes": &graphql.Field{Type: graphql.NewList(graphql.NewObject(graphql.ObjectConfig{
				Name: "GitActor",
				Fields: graphql.Fields{
					"name":  &graphql.Field{Type: graphql.String},
					"email": &graphql.Field{Type: graphql.String},
					"user": &graphql.Field{
						Type: userType,
						Resolve: func(p graphql.ResolveParams) (interface{}, error) {
							a, ok := p.Source.(map[string]interface{})
							if !ok {
								return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
							}
							return a["user"], nil
						},
					},
				},
			}))},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	commitType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Commit",
		Fields: graphql.Fields{
			"oid":             &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"messageHeadline": &graphql.Field{Type: graphql.String},
			"messageBody":     &graphql.Field{Type: graphql.String},
			"committedDate":   &graphql.Field{Type: graphql.String},
			"authoredDate":    &graphql.Field{Type: graphql.String},
			"authors": &graphql.Field{
				Type: gitActorConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["authors"], nil
				},
			},
			"statusCheckRollup": &graphql.Field{
				// Null when no checks exist for the commit — matches real
				// GitHub for a commit with no statuses or check runs.
				Type: statusCheckRollupType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["statusCheckRollup"], nil
				},
			},
		},
	})

	pullRequestCommitType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestCommit",
		Fields: graphql.Fields{
			"commit": &graphql.Field{
				Type: graphql.NewNonNull(commitType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					n, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return n["commit"], nil
				},
			},
		},
	})

	// --- Review types ---
	prReviewType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReview",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["nodeID"], nil
				},
			},
			"body":  &graphql.Field{Type: graphql.String},
			"state": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["author"], nil
				},
			},
			"authorAssociation": &graphql.Field{Type: graphql.String},
			"createdAt":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			// gh's reviews fragment selects submittedAt, commit{oid}, and
			// reactionGroups. Bleephub reviews are submitted on creation, so
			// submittedAt == createdAt; commit is the PR head the review was
			// recorded against (same derivation as REST's commit_id).
			"submittedAt": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["submittedAt"], nil
				},
			},
			"commit": &graphql.Field{
				Type: commitType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["commit"], nil
				},
			},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(prReactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["reactionGroups"], nil
				},
			},
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
	// gh's reviewRequests fragment unions `...on User`, `...on Bot` (Copilot
	// as reviewer), and `...on Team`. Bot and Team exist so the fragments
	// type-check; bleephub currently stores user review requests.
	botType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Bot",
		Fields: graphql.Fields{
			"id":    &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"login": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	teamType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Team",
		Fields: graphql.Fields{
			"id": &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			// Nullable (real GitHub: String!) — graphql-go enforces the
			// strict SameResponseShape merge rule, so Team.name must match
			// User.name's nullability for gh's `...on User{name}` +
			// `...on Team{name}` requestedReviewer selection to validate.
			// GitHub's validator is laxer; the value is never null here.
			"name": &graphql.Field{Type: graphql.String},
			"slug": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"organization": &graphql.Field{
				Type: graphql.NewNonNull(graphql.NewObject(graphql.ObjectConfig{
					Name: "TeamOrganization",
					Fields: graphql.Fields{
						"login": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
					},
				})),
			},
		},
	})
	requestedReviewerUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:  "RequestedReviewer",
		Types: []*graphql.Object{userType, botType, teamType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			return userType
		},
	})
	reviewRequestType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReviewRequest",
		Fields: graphql.Fields{
			"requestedReviewer": &graphql.Field{
				Type: requestedReviewerUnion,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
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

	// PRComment type — must match IssueComment's nullability for shared
	// fields (body, createdAt). gh CLI's `gh issue view` and `gh pr view`
	// queries union Issue|PR with shared `comments.nodes` field
	// selections; the field types must merge. Resolvers read from the
	// source map populated by prCommentToGQLLocked above.
	prCommentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRComment",
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
			"body": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["body"], nil
				},
			},
			"createdAt": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["createdAt"], nil
				},
			},
			"authorAssociation": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["authorAssociation"], nil
				},
			},
			"url": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["url"], nil
				},
			},
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
				Type: graphql.NewList(prReactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["reactionGroups"], nil
				},
			},
		},
	})

	prCommentConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PRCommentConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(prCommentType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(prCommentPageInfoType)},
		},
	})

	// --- PR Review thread types ---
	prReviewCommentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReviewComment",
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
			"databaseId": &graphql.Field{Type: graphql.Int},
			"body":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"path":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"diffHunk":   &graphql.Field{Type: graphql.String},
			"line":       &graphql.Field{Type: graphql.Int},
			"position":   &graphql.Field{Type: graphql.Int},
			"createdAt":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"state":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
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
		},
	})
	prReviewCommentConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReviewCommentConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(prReviewCommentType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})
	prReviewThreadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReviewThread",
		Fields: graphql.Fields{
			"id":         &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"isResolved": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isOutdated": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"resolvedBy": &graphql.Field{Type: userType},
			"path":       &graphql.Field{Type: graphql.String},
			"line":       &graphql.Field{Type: graphql.Int},
			"comments":   &graphql.Field{Type: graphql.NewNonNull(prReviewCommentConnectionType)},
		},
	})
	prReviewThreadConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestReviewThreadConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(prReviewThreadType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	// --- PR Commit connection ---
	prCommitConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequestCommitConnection",
		Fields: graphql.Fields{
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"nodes":      &graphql.Field{Type: graphql.NewList(pullRequestCommitType)},
		},
	})

	// --- PullRequest type ---
	pullRequestType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PullRequest",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["nodeID"], nil
				},
			},
			"databaseId":       &graphql.Field{Type: graphql.Int},
			"number":           &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"title":            &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"body":             &graphql.Field{Type: graphql.String},
			"state":            &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"isDraft":          &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"url":              &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"headRefName":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"baseRefName":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"headRefOid":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"mergeable":        &graphql.Field{Type: graphql.NewNonNull(mergeableStateEnum)},
			"merged":           &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"mergedAt":         &graphql.Field{Type: graphql.String},
			"additions":        &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"deletions":        &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"changedFiles":     &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"locked":           &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"activeLockReason": &graphql.Field{Type: graphql.String},
			"createdAt":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"closedAt":         &graphql.Field{Type: graphql.String},
			"reviewDecision":   &graphql.Field{Type: pullRequestReviewDecisionEnum},
			"author": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["author"], nil
				},
			},
			"mergedBy": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["mergedBy"], nil
				},
			},
			"labels": &graphql.Field{
				Type: prLabelConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(pr["labels"], p.Args), nil
				},
			},
			"assignees": &graphql.Field{
				Type: prAssigneeConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(pr["assignees"], p.Args), nil
				},
			},
			"reviews": &graphql.Field{
				Type: prReviewConnectionType,
				Args: graphql.FieldConfigArgument{
					"first":  &graphql.ArgumentConfig{Type: graphql.Int},
					"last":   &graphql.ArgumentConfig{Type: graphql.Int},
					"after":  &graphql.ArgumentConfig{Type: graphql.String},
					"before": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(pr["reviews"], p.Args), nil
				},
			},
			"reviewRequests": &graphql.Field{
				Type: reviewRequestConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["reviewRequests"], nil
				},
			},
			"comments": &graphql.Field{
				Type: prCommentConnectionType,
				Args: graphql.FieldConfigArgument{
					"first":  &graphql.ArgumentConfig{Type: graphql.Int},
					"last":   &graphql.ArgumentConfig{Type: graphql.Int},
					"after":  &graphql.ArgumentConfig{Type: graphql.String},
					"before": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return repaginateConnection(pr["comments"], p.Args), nil
				},
			},
			"reviewThreads": &graphql.Field{
				Type: prReviewThreadConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"last":  &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["reviewThreads"], nil
				},
			},
			// ProjectV2 items — gh pr view fetches PullRequest.projectItems
			// as a second round-trip (api.ProjectsV2ItemsForPullRequest).
			// Returns the real items the PR was added to via
			// addProjectV2ItemById.
			"projectItems": &graphql.Field{
				Type: projectV2ItemConnectionType(),
				Args: relayConnectionArgs(),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					prID, _ := pr["databaseId"].(int)
					items := s.store.ProjectsV2.ListItemsForPR(prID)
					nodes := make([]map[string]interface{}, 0, len(items))
					for _, it := range items {
						nodes = append(nodes, projectV2ItemToGQL(it, s.store))
					}
					return paginateGQLMaps(nodes, p.Args), nil
				},
			},
			// PR.milestone — real GH PRs are issues internally so they
			// share the Milestone table. pullRequestToGQL embeds the
			// resolved milestone map in pr["milestone"] (or nil when the
			// PR has no milestone assigned).
			"milestone": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "PRMilestone",
					Fields: graphql.Fields{
						"number":      &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
						"title":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
						"description": &graphql.Field{Type: graphql.String},
						"dueOn":       &graphql.Field{Type: graphql.String},
					},
				}),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					m, ok := pr["milestone"].(map[string]interface{})
					if !ok || m == nil {
						return nil, nil
					}
					return m, nil
				},
			},
			// gh reads check state and the lastCommit field through
			// commits(last:1){nodes{commit{...}}} — GitHub's PullRequest has
			// no top-level statusCheckRollup field; gh aliases the commits
			// connection instead. The nodes carry the same real git commits
			// the REST surface reports.
			"commits": &graphql.Field{
				Type: prCommitConnectionType,
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"last":  &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["commits"], nil
				},
			},
			"reactionGroups": &graphql.Field{
				Type: graphql.NewList(prReactionGroupType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["reactionGroups"], nil
				},
			},
			"closed": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					state, _ := pr["state"].(string)
					return state == "CLOSED" || state == "MERGED", nil
				},
			},
			"headRepository": &graphql.Field{
				Type: repoType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["headRepository"], nil
				},
			},
			"headRepositoryOwner": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["headRepositoryOwner"], nil
				},
			},
			"isCrossRepository": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					v, ok := pr["isCrossRepository"].(bool)
					if !ok {
						return nil, fmt.Errorf("pull request source missing isCrossRepository")
					}
					return v, nil
				},
			},
			"maintainerCanModify": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					v, ok := pr["maintainerCanModify"].(bool)
					if !ok {
						return nil, fmt.Errorf("pull request source missing maintainerCanModify")
					}
					return v, nil
				},
			},
			"autoMergeRequest": &graphql.Field{
				// No auto-merge feature: null, the value real GitHub returns
				// when auto-merge isn't enabled on the PR.
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "AutoMergeRequest",
					Fields: graphql.Fields{
						"authorEmail":    &graphql.Field{Type: graphql.String},
						"commitBody":     &graphql.Field{Type: graphql.String},
						"commitHeadline": &graphql.Field{Type: graphql.String},
						"mergeMethod":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
						"enabledAt":      &graphql.Field{Type: graphql.String},
						"enabledBy":      &graphql.Field{Type: userType},
					},
				}),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return nil, nil
				},
			},
			"baseRefOid": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"fullDatabaseId": &graphql.Field{
				// BigInt scalar on real GitHub — serializes as a string.
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					id, _ := pr["databaseId"].(int)
					return strconv.Itoa(id), nil
				},
			},
			"files": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "PullRequestChangedFileConnection",
					Fields: graphql.Fields{
						"nodes": &graphql.Field{Type: graphql.NewList(graphql.NewObject(graphql.ObjectConfig{
							Name: "PullRequestChangedFile",
							Fields: graphql.Fields{
								"additions":  &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
								"deletions":  &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
								"path":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
								"changeType": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
							},
						}))},
						"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
					},
				}),
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, repo, err := pullRequestAndRepoFromGQLSource(p.Source, s.store)
					if err != nil {
						return nil, err
					}
					files, err := pullRequestChangedFiles(s.store, repo, pr, "")
					if err != nil {
						return nil, err
					}
					nodes := make([]map[string]interface{}, 0, len(files))
					for _, file := range files {
						nodes = append(nodes, pullRequestChangedFileToGQL(file))
					}
					return repaginateConnection(map[string]interface{}{"nodes": nodes}, p.Args), nil
				},
			},
			"closingIssuesReferences": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "ClosingIssuesReferencesConnection",
					Fields: graphql.Fields{
						"nodes": &graphql.Field{Type: graphql.NewList(issueType)},
						"pageInfo": &graphql.Field{Type: graphql.NewNonNull(graphql.NewObject(graphql.ObjectConfig{
							Name: "ClosingIssuesReferencesPageInfo",
							Fields: graphql.Fields{
								"hasNextPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
								"endCursor":   &graphql.Field{Type: graphql.String},
							},
						}))},
						"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
					},
				}),
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
					"after": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, repo, err := pullRequestAndRepoFromGQLSource(p.Source, s.store)
					if err != nil {
						return nil, err
					}
					issues := closingIssuesForPullRequest(s.store, repo, pr)
					first, _ := intArg(p.Args, "first")
					after, _ := p.Args["after"].(string)
					return paginateGQL(issues, first, after, func(issue *Issue) map[string]interface{} {
						return issueToGQL(issue, s.store)
					}), nil
				},
			},
			"mergeCommit": &graphql.Field{
				// Merges aren't materialised as git commits; null matches
				// REST's merge_commit_sha staying null.
				Type: commitType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return nil, nil
				},
			},
			"potentialMergeCommit": &graphql.Field{
				Type: commitType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return nil, nil
				},
			},
			// latestReviews — the newest review per author, derived from the
			// same review store as `reviews`.
			"latestReviews": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "LatestReviewConnection",
					Fields: graphql.Fields{
						"nodes":      &graphql.Field{Type: graphql.NewList(prReviewType)},
						"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
					},
				}),
				Args: graphql.FieldConfigArgument{
					"first": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["latestReviews"], nil
				},
			},
			"mergeStateStatus": &graphql.Field{
				Type: graphql.NewNonNull(mergeStateStatusEnum),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return pr["mergeStateStatus"], nil
				},
			},
			// gh pr status selects baseRef{branchProtectionRule{...}}; resolve
			// from the typed branch-protection model.
			"baseRef": &graphql.Field{
				Type: graphql.NewObject(graphql.ObjectConfig{
					Name: "PRBaseRef",
					Fields: graphql.Fields{
						"name": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
						"branchProtectionRule": &graphql.Field{
							Type: graphql.NewObject(graphql.ObjectConfig{
								Name: "BranchProtectionRule",
								Fields: graphql.Fields{
									"requiresStrictStatusChecks":   &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
									"requiredApprovingReviewCount": &graphql.Field{Type: graphql.Int},
								},
							}),
							Resolve: func(p graphql.ResolveParams) (interface{}, error) {
								pr, ok := p.Source.(map[string]interface{})
								if !ok {
									return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
								}
								prID, _ := pr["databaseId"].(int)
								prObj := s.store.GetPullRequest(prID)
								if prObj == nil {
									return nil, nil
								}
								repo := s.store.GetRepoByID(prObj.RepoID)
								if repo == nil {
									return nil, nil
								}
								return s.branchProtectionRuleForPR(repo, prObj.BaseRefName), nil
							},
						},
					},
				}),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					pr, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					base, _ := pr["baseRefName"].(string)
					return map[string]interface{}{"name": base}, nil
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
			"first":       &graphql.ArgumentConfig{Type: graphql.Int},
			"after":       &graphql.ArgumentConfig{Type: graphql.String},
			"states":      &graphql.ArgumentConfig{Type: graphql.NewList(pullRequestStateEnum)},
			"labels":      &graphql.ArgumentConfig{Type: graphql.NewList(graphql.String)},
			"headRefName": &graphql.ArgumentConfig{Type: graphql.String},
			"baseRefName": &graphql.ArgumentConfig{Type: graphql.String},
			// gh sends orderBy as literal enum names — `orderBy: {field:
			// CREATED_AT, direction: DESC}` — so field/direction must be
			// enums (string-typed inputs fail validation), exactly like the
			// issues connection's IssueOrder.
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "IssueOrder2",
				Fields: graphql.InputObjectConfigFieldMap{
					"field": &graphql.InputObjectFieldConfig{Type: graphql.NewEnum(graphql.EnumConfig{
						Name: "PullRequestOrderField",
						Values: graphql.EnumValueConfigMap{
							"CREATED_AT": &graphql.EnumValueConfig{Value: "CREATED_AT"},
							"UPDATED_AT": &graphql.EnumValueConfig{Value: "UPDATED_AT"},
						},
					})},
					"direction": &graphql.InputObjectFieldConfig{Type: graphql.NewEnum(graphql.EnumConfig{
						Name: "PullRequestOrderDirection",
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
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, _ := repo["databaseId"].(int)
			number, _ := p.Args["number"].(int)

			pr := s.store.GetPullRequestByNumber(repoID, number)
			if pr == nil {
				// Real GitHub returns a typed NOT_FOUND error, not bare null.
				return nil, &ghNotFoundError{
					message: fmt.Sprintf("Could not resolve to a PullRequest with the number of %d.", number),
				}
			}
			return pullRequestToGQL(pr, s.store), nil
		},
	})

	// fix — issueOrPullRequest as a real Issue|PullRequest union so
	// gh CLI's `gh issue view <N>` `...on Issue` + `...on PullRequest`
	// fragments type-check.
	issueOrPRUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:        "IssueOrPullRequest",
		Description: "Either an Issue or a PullRequest (matches GitHub's polymorphic lookup by number).",
		Types:       []*graphql.Object{issueType, pullRequestType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			if m, ok := p.Value.(map[string]interface{}); ok {
				if tn, _ := m["__typename"].(string); tn == "PullRequest" {
					return pullRequestType
				}
			}
			return issueType
		},
	})
	repoType.AddFieldConfig("issueOrPullRequest", &graphql.Field{
		Type: issueOrPRUnion,
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

			// Issue first; if not found, fall through to PR.
			if issue := s.store.GetIssueByNumber(repoID, number); issue != nil {
				result := issueToGQL(issue, s.store)
				result["__typename"] = "Issue"
				return result, nil
			}
			if pr := s.store.GetPullRequestByNumber(repoID, number); pr != nil {
				result := pullRequestToGQL(pr, s.store)
				result["__typename"] = "PullRequest"
				return result, nil
			}
			// Real GitHub returns a typed NOT_FOUND error, not bare null.
			return nil, &ghNotFoundError{
				message: fmt.Sprintf("Could not resolve to an issue or pull request with the number of %d.", number),
			}
		},
	})

	// --- Query.search (ISSUE type) ---
	// gh pr status sends search(query:$q, type:ISSUE, first:$limit) with
	// `repo: state:open is:pr author:/review-requested:` qualifiers, and
	// gh issue/pr list filters send search(type:$type, last:$limit,
	// after:$after, query:$query). SearchType deliberately declares ONLY
	// ISSUE: gh introspects the enum and opts into ISSUE_ADVANCED only when
	// present, so omitting it keeps gh on the plain ISSUE backend.
	searchTypeEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "SearchType",
		Values: graphql.EnumValueConfigMap{
			"ISSUE": &graphql.EnumValueConfig{Value: "ISSUE"},
		},
	})
	searchResultItemUnion := graphql.NewUnion(graphql.UnionConfig{
		Name:  "SearchResultItem",
		Types: []*graphql.Object{issueType, pullRequestType},
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			if m, ok := p.Value.(map[string]interface{}); ok {
				if tn, _ := m["__typename"].(string); tn == "PullRequest" {
					return pullRequestType
				}
			}
			return issueType
		},
	})
	searchResultEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchResultItemEdge",
		Fields: graphql.Fields{
			"node":   &graphql.Field{Type: searchResultItemUnion},
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	searchResultConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchResultItemConnection",
		Fields: graphql.Fields{
			"issueCount": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Int),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					c, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return c["totalCount"], nil
				},
			},
			"nodes": &graphql.Field{Type: graphql.NewList(searchResultItemUnion)},
			"edges": &graphql.Field{Type: graphql.NewList(searchResultEdgeType)},
			"pageInfo": &graphql.Field{Type: graphql.NewNonNull(graphql.NewObject(graphql.ObjectConfig{
				Name: "SearchPageInfo",
				Fields: graphql.Fields{
					"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
					"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
					"startCursor":     &graphql.Field{Type: graphql.String},
					"endCursor":       &graphql.Field{Type: graphql.String},
				},
			}))},
		},
	})
	queryType.AddFieldConfig("search", &graphql.Field{
		Type: searchResultConnectionType,
		Args: graphql.FieldConfigArgument{
			"query": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"type":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(searchTypeEnum)},
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
			"last":  &graphql.ArgumentConfig{Type: graphql.Int},
			"after": &graphql.ArgumentConfig{Type: graphql.String},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			q, _ := p.Args["query"].(string)

			limit := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				limit = f
			} else if l, ok := p.Args["last"].(int); ok && l > 0 {
				// gh's issue-search query passes the page size as `last`.
				limit = l
			}
			after, _ := p.Args["after"].(string)

			nodes := s.searchIssuesAndPRs(q, ghUserFromContext(p.Context))
			return paginateGQL(nodes, limit, after, func(n map[string]interface{}) map[string]interface{} {
				return n
			}), nil
		},
	})

	// --- Mutations ---

	createPRInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreatePullRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"repositoryId":        &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"title":               &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"body":                &graphql.InputObjectFieldConfig{Type: graphql.String},
			"headRefName":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"baseRefName":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"draft":               &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
			"maintainerCanModify": &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
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
			maintainerCanModify, _ := input["maintainerCanModify"].(bool)

			repo := findRepoByNodeID(s.store, repoNodeID)
			if repo == nil {
				return nil, fmt.Errorf("could not resolve to a Repository with the global id of '%s'", repoNodeID)
			}

			headRepo, headRefName := resolvePullRequestHead(s.store, repo, headRefName)
			if headRepo == nil || headRefName == "" {
				return nil, fmt.Errorf("pull request creation failed")
			}
			pr := s.store.CreatePullRequest(repo.ID, user.ID, title, body, headRefName, baseRefName, draft, nil, nil, 0, PullRequestOptions{
				HeadRepoID:          headRepo.ID,
				MaintainerCanModify: maintainerCanModify,
			})
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

			priorState := pr.State
			s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				p.State = "CLOSED"
				now := time.Now()
				p.ClosedAt = &now
			})
			if priorState == "OPEN" {
				s.store.RecordPullRequestEvent(pr.RepoID, pr.ID, user.ID, "closed", "", 0)
			}

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

			priorState := pr.State
			s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
				p.State = "OPEN"
				p.ClosedAt = nil
			})
			if priorState == "CLOSED" {
				s.store.RecordPullRequestEvent(pr.RepoID, pr.ID, user.ID, "reopened", "", 0)
			}

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
			// gh sets authorEmail (--author-email) and expectedHeadOid
			// (--match-head-commit); accepted so input coercion succeeds.
			"authorEmail":      &graphql.InputObjectFieldConfig{Type: graphql.String},
			"expectedHeadOid":  &graphql.InputObjectFieldConfig{Type: graphql.String},
			"clientMutationId": &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})

	mergePRPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MergePullRequestPayload",
		Fields: graphql.Fields{
			"pullRequest": &graphql.Field{Type: pullRequestType},
			// gh's PullRequestMerge mutation selects only clientMutationId.
			"clientMutationId": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					m, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return m["clientMutationId"], nil
				},
			},
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

			repo := s.store.GetRepoByID(pr.RepoID)
			if repo == nil {
				return nil, fmt.Errorf("could not resolve to a Repository")
			}

			method := "merge"
			if v, ok := input["mergeMethod"].(string); ok && v != "" {
				method = strings.ToLower(v)
			}
			commitHeadline, _ := input["commitHeadline"].(string)
			commitBody, _ := input["commitBody"].(string)
			if _, errMsg := s.completePullRequestMerge(repo, pr, user, method, commitHeadline, commitBody); errMsg != "" {
				return nil, fmt.Errorf("%s", errMsg)
			}

			updated := s.store.GetPullRequest(pr.ID)
			mergedPayload := buildPullRequestPayload(s.store, repo, updated, user, "closed")
			s.emitWebhookEvent(repo.FullName, "pull_request", "closed", mergedPayload)
			s.triggerWorkflowsForEvent(repo.FullName, "pull_request", "closed", "refs/heads/"+updated.HeadRefName, mergedPayload)

			var clientMutationID interface{}
			if v, ok := input["clientMutationId"].(string); ok && v != "" {
				clientMutationID = v
			}
			return map[string]interface{}{
				"pullRequest":      pullRequestToGQL(updated, s.store),
				"clientMutationId": clientMutationID,
			}, nil
		},
	})

	// --- addPullRequestReview (gh pr review) ---
	// gh submits reviews via mutation PullRequestReviewAdd($input:
	// AddPullRequestReviewInput!){addPullRequestReview(input:$input)
	// {clientMutationId}} with pullRequestId + event + body. Mapped onto the
	// same review store as REST POST .../pulls/{n}/reviews.
	pullRequestReviewEventEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "PullRequestReviewEvent",
		Values: graphql.EnumValueConfigMap{
			"APPROVE":         &graphql.EnumValueConfig{Value: "APPROVE"},
			"COMMENT":         &graphql.EnumValueConfig{Value: "COMMENT"},
			"DISMISS":         &graphql.EnumValueConfig{Value: "DISMISS"},
			"REQUEST_CHANGES": &graphql.EnumValueConfig{Value: "REQUEST_CHANGES"},
		},
	})

	addPRReviewInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "AddPullRequestReviewInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"pullRequestId":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"event":            &graphql.InputObjectFieldConfig{Type: pullRequestReviewEventEnum},
			"body":             &graphql.InputObjectFieldConfig{Type: graphql.String},
			"commitOID":        &graphql.InputObjectFieldConfig{Type: graphql.String},
			"clientMutationId": &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})

	addPRReviewPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AddPullRequestReviewPayload",
		Fields: graphql.Fields{
			"pullRequestReview": &graphql.Field{Type: prReviewType},
			"clientMutationId": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					m, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return m["clientMutationId"], nil
				},
			},
		},
	})

	mutationType.AddFieldConfig("addPullRequestReview", &graphql.Field{
		Type: addPRReviewPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(addPRReviewInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}

			input, _ := p.Args["input"].(map[string]interface{})
			prNodeID, _ := input["pullRequestId"].(string)
			event, _ := input["event"].(string)
			body, _ := input["body"].(string)

			pr := findPullRequestByNodeID(s.store, prNodeID)
			if pr == nil {
				return nil, &ghNotFoundError{
					message: fmt.Sprintf("Could not resolve to a node with the global id of '%s'", prNodeID),
				}
			}

			state := "COMMENTED"
			switch event {
			case "APPROVE":
				state = "APPROVED"
			case "REQUEST_CHANGES":
				state = "CHANGES_REQUESTED"
			case "DISMISS":
				state = "DISMISSED"
			}

			review := s.store.CreatePRReview(pr.ID, user.ID, state, body)
			if review == nil {
				return nil, fmt.Errorf("review creation failed")
			}

			var clientMutationID interface{}
			if v, ok := input["clientMutationId"].(string); ok && v != "" {
				clientMutationID = v
			}
			return map[string]interface{}{
				"pullRequestReview": prReviewToGQL(review, s.store),
				"clientMutationId":  clientMutationID,
			}, nil
		},
	})

	resolveReviewThreadInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "ResolveReviewThreadInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"threadId":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"clientMutationId": &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})
	resolveReviewThreadPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ResolveReviewThreadPayload",
		Fields: graphql.Fields{
			"thread":           &graphql.Field{Type: prReviewThreadType},
			"clientMutationId": &graphql.Field{Type: graphql.String},
		},
	})
	unresolveReviewThreadInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UnresolveReviewThreadInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"threadId":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"clientMutationId": &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})
	unresolveReviewThreadPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UnresolveReviewThreadPayload",
		Fields: graphql.Fields{
			"thread":           &graphql.Field{Type: prReviewThreadType},
			"clientMutationId": &graphql.Field{Type: graphql.String},
		},
	})

	mutationType.AddFieldConfig("resolveReviewThread", &graphql.Field{
		Type: resolveReviewThreadPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(resolveReviewThreadInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return s.resolveReviewThreadGraphQL(p, true)
		},
	})
	mutationType.AddFieldConfig("unresolveReviewThread", &graphql.Field{
		Type: unresolveReviewThreadPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(unresolveReviewThreadInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return s.resolveReviewThreadGraphQL(p, false)
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

func pullRequestAndRepoFromGQLSource(src interface{}, st *Store) (*PullRequest, *Repo, error) {
	prMap, ok := src.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("resolve source: unexpected type %T", src)
	}
	prID, ok := prMap["databaseId"].(int)
	if !ok || prID <= 0 {
		return nil, nil, fmt.Errorf("pull request source missing databaseId")
	}
	pr := st.GetPullRequest(prID)
	if pr == nil {
		return nil, nil, fmt.Errorf("pull request not found")
	}
	repo := st.GetRepoByID(pr.RepoID)
	if repo == nil {
		return nil, nil, fmt.Errorf("repository not found")
	}
	return pr, repo, nil
}

func pullRequestChangedFileToGQL(file map[string]interface{}) map[string]interface{} {
	status, _ := file["status"].(string)
	changeType := "CHANGED"
	switch status {
	case "added":
		changeType = "ADDED"
	case "removed":
		changeType = "DELETED"
	case "renamed":
		changeType = "RENAMED"
	case "modified", "changed":
		changeType = "CHANGED"
	}
	path, _ := file["filename"].(string)
	additions, _ := file["additions"].(int)
	deletions, _ := file["deletions"].(int)
	return map[string]interface{}{
		"path":       path,
		"additions":  additions,
		"deletions":  deletions,
		"changeType": changeType,
	}
}

var (
	closingKeywordRE = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\b([^\n]*)`)
	closingIssueRE   = regexp.MustCompile(`(?i)(?:\b([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+))?#([0-9]+)`)
)

func closingIssuesForPullRequest(st *Store, repo *Repo, pr *PullRequest) []*Issue {
	refs := closingIssueRefs(repo.FullName, pr.Body)
	if len(refs) == 0 {
		return nil
	}
	st.mu.RLock()
	defer st.mu.RUnlock()

	issues := make([]*Issue, 0, len(refs))
	seen := map[int]bool{}
	for _, ref := range refs {
		targetRepo := st.ReposByName[ref.repoFullName]
		if targetRepo == nil {
			continue
		}
		issue := st.IssuesByRepo[targetRepo.ID][ref.number]
		if issue == nil || seen[issue.ID] {
			continue
		}
		seen[issue.ID] = true
		issues = append(issues, issue)
	}
	return issues
}

type closingIssueRef struct {
	repoFullName string
	number       int
}

func closingIssueRefs(defaultRepoFullName, body string) []closingIssueRef {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	refs := []closingIssueRef{}
	for _, match := range closingKeywordRE.FindAllStringSubmatch(body, -1) {
		tail := match[1]
		if idx := strings.IndexAny(tail, ".;"); idx >= 0 {
			tail = tail[:idx]
		}
		for _, refMatch := range closingIssueRE.FindAllStringSubmatch(tail, -1) {
			repoFullName := defaultRepoFullName
			if refMatch[1] != "" {
				repoFullName = refMatch[1]
			}
			number, err := strconv.Atoi(refMatch[2])
			if err != nil || number <= 0 {
				continue
			}
			refs = append(refs, closingIssueRef{repoFullName: repoFullName, number: number})
		}
	}
	return refs
}

func pullRequestToGQL(pr *PullRequest, st *Store) map[string]interface{} {
	baseRepo := st.GetRepoByID(pr.RepoID)
	stor, repoFullNameForCommits := pullRequestGitStorage(st, baseRepo, pr)
	realCommits := []*object.Commit(nil)
	if stor != nil {
		if commits, err := pullRequestCommitObjectsFromStorage(stor, pr); err == nil {
			realCommits = commits
		}
	}

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
	prReviews := st.PRReviewsByPR[pr.ID]
	reviewNodes := make([]map[string]interface{}, 0, len(prReviews))
	for _, r := range prReviews {
		reviewNodes = append(reviewNodes, prReviewSourceLocked(r, st))
	}
	sortGQLNodesByCreatedAt(reviewNodes)

	// latestReviews — the newest review per author.
	latestByAuthor := map[int]*PullRequestReview{}
	for _, r := range prReviews {
		if cur, ok := latestByAuthor[r.AuthorID]; !ok || r.CreatedAt.After(cur.CreatedAt) {
			latestByAuthor[r.AuthorID] = r
		}
	}
	latestReviewNodes := make([]map[string]interface{}, 0, len(latestByAuthor))
	for _, r := range latestByAuthor {
		latestReviewNodes = append(latestReviewNodes, prReviewSourceLocked(r, st))
	}
	sort.Slice(latestReviewNodes, func(a, b int) bool {
		ca, _ := latestReviewNodes[a]["createdAt"].(string)
		cb, _ := latestReviewNodes[b]["createdAt"].(string)
		return ca < cb
	})

	// Derive review decision
	var reviewDecision interface{}
	if rd := deriveReviewDecisionLocked(st, pr.ID); rd != "" {
		reviewDecision = rd
	}

	// Conversation comments (real GitHub: PRs and Issues share the same
	// comment table; bleephub mirrors that via Comment.ParentType).
	prCommentNodes := make([]map[string]interface{}, 0)
	for _, c := range st.Comments {
		if c.ParentType == "pull_request" && c.IssueID == pr.ID {
			prCommentNodes = append(prCommentNodes, prCommentToGQLLocked(c, st))
		}
	}
	// st.Comments is a map, so iteration order is nondeterministic; sort for
	// stable cursor pagination (oldest first, like GitHub's comments feed).
	sortGQLNodesByCreatedAt(prCommentNodes)

	// Review threads — inline file-line review comments grouped by thread.
	reviewThreadNodes := reviewThreadsForGraphQL(st.PRReviewComments.ListThreads(pr.ID), st)

	// URL
	repo := st.Repos[pr.RepoID]
	url := ""
	if repo != nil {
		url = "/" + repo.FullName + "/pull/" + strconv.Itoa(pr.Number)
	}

	sha := pullRequestHeadSHALocked(pr, st)
	baseSha := pr.BaseSHA

	var closedAt interface{}
	if pr.ClosedAt != nil {
		closedAt = pr.ClosedAt.Format(time.RFC3339)
	}
	var mergedAt interface{}
	if pr.MergedAt != nil {
		mergedAt = pr.MergedAt.Format(time.RFC3339)
	}

	// mergeStateStatus from the stored mergeability — the only merge gate
	// bleephub models (no protected branches / required checks in GraphQL).
	mergeStateStatus := "UNKNOWN"
	switch pr.Mergeable {
	case "MERGEABLE":
		mergeStateStatus = "CLEAN"
	case "CONFLICTING":
		mergeStateStatus = "DIRTY"
	}

	var headRepository map[string]interface{}
	headRepo := pullRequestHeadRepoLocked(st, pr)
	if headRepo != nil {
		headRepository = repoToGraphQLLocked(st, headRepo)
	}
	headRepositoryOwner := repoOwnerGraphQLLocked(headRepo, st)

	commitNodes := make([]interface{}, 0)
	var commitAuthors []interface{}
	if u, ok := st.Users[pr.AuthorID]; ok {
		commitAuthors = append(commitAuthors, map[string]interface{}{
			"name":  u.Name,
			"email": u.Email,
			"user":  userToGraphQL(u),
		})
	}
	repoFullName := ""
	if repo != nil {
		repoFullName = repo.FullName
	}
	if repoFullName == "" {
		repoFullName = repoFullNameForCommits
	}
	for _, c := range realCommits {
		commitNodes = append(commitNodes, map[string]interface{}{"commit": gitCommitToGQLLocked(c, st, repoFullName)})
	}
	if len(commitNodes) == 0 && sha != "" {
		headCommit := map[string]interface{}{
			"oid":               sha,
			"messageHeadline":   pr.Title,
			"messageBody":       nil,
			"committedDate":     pr.CreatedAt.Format(time.RFC3339),
			"authoredDate":      pr.CreatedAt.Format(time.RFC3339),
			"authors":           map[string]interface{}{"nodes": commitAuthors, "totalCount": len(commitAuthors)},
			"statusCheckRollup": statusCheckRollupSourceLocked(st, repoFullName, sha),
		}
		commitNodes = append(commitNodes, map[string]interface{}{"commit": headCommit})
	}

	return map[string]interface{}{
		"__typename":       "PullRequest",
		"nodeID":           pr.NodeID,
		"databaseId":       pr.ID,
		"repoID":           pr.RepoID,
		"number":           pr.Number,
		"title":            pr.Title,
		"body":             pr.Body,
		"state":            pr.State,
		"isDraft":          pr.IsDraft,
		"url":              url,
		"headRefName":      pr.HeadRefName,
		"baseRefName":      pr.BaseRefName,
		"headRefOid":       sha,
		"baseRefOid":       baseSha,
		"mergeable":        pr.Mergeable,
		"mergeStateStatus": mergeStateStatus,
		"merged":           pr.State == "MERGED",
		"mergedAt":         mergedAt,
		"mergedBy":         mergedBy,
		"additions":        pr.Additions,
		"deletions":        pr.Deletions,
		"changedFiles":     pr.ChangedFiles,
		"reviewDecision":   reviewDecision,
		"author":           author,
		"createdAt":        pr.CreatedAt.Format(time.RFC3339),
		"updatedAt":        pr.UpdatedAt.Format(time.RFC3339),
		"closedAt":         closedAt,
		"locked":           pr.Locked,
		"activeLockReason": nilStr(pr.ActiveLockReason),
		"milestone":        prMilestoneToGQLLocked(pr, st),
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
		"latestReviews": map[string]interface{}{
			"nodes":      latestReviewNodes,
			"totalCount": len(latestReviewNodes),
		},
		"headRepository":      headRepository,
		"headRepositoryOwner": headRepositoryOwner,
		"isCrossRepository":   pullRequestHeadRepoID(pr) != pr.RepoID,
		"maintainerCanModify": pr.MaintainerCanModify,
		"reviewRequests": map[string]interface{}{
			"nodes":      pullRequestReviewRequestNodesLocked(pr, st),
			"totalCount": len(pr.RequestedReviewerIDs),
		},
		"comments": map[string]interface{}{
			"nodes":      prCommentNodes,
			"totalCount": len(prCommentNodes),
			"pageInfo": map[string]interface{}{
				"hasNextPage":     false,
				"hasPreviousPage": false,
				"startCursor":     nil,
				"endCursor":       nil,
			},
		},
		"commits": map[string]interface{}{
			"totalCount": len(commitNodes),
			"nodes":      commitNodes,
		},
		"reactionGroups": reactionGroupsForGraphQL(st.Reactions, "pull_request", pr.ID),
		"reviewThreads": map[string]interface{}{
			"nodes":      reviewThreadNodes,
			"totalCount": len(reviewThreadNodes),
		},
	}
}

// reviewThreadsForGraphQL renders ReviewThread + PRReviewComment
// records as the GraphQL source map shape expected by the
// PullRequestReviewThread / PullRequestReviewComment types below.
// Caller must hold st.mu.RLock.
func reviewThreadsForGraphQL(threads []*ReviewThread, st *Store) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(threads))
	for _, t := range threads {
		commentNodes := make([]map[string]interface{}, 0, len(t.Comments))
		for _, c := range t.Comments {
			var author map[string]interface{}
			if u, ok := st.Users[c.AuthorID]; ok {
				author = userToGraphQL(u)
			}
			var line interface{}
			if c.Line != nil {
				line = *c.Line
			}
			var position interface{}
			if c.Position != nil {
				position = *c.Position
			}
			commentNodes = append(commentNodes, map[string]interface{}{
				"nodeID":     c.NodeID,
				"databaseId": c.ID,
				"body":       c.Body,
				"path":       c.Path,
				"diffHunk":   c.DiffHunk,
				"line":       line,
				"position":   position,
				"createdAt":  c.CreatedAt.Format(time.RFC3339),
				"updatedAt":  c.UpdatedAt.Format(time.RFC3339),
				"author":     author,
				"state":      "SUBMITTED",
			})
		}
		// The thread's path/line tracks the root comment.
		var threadPath string
		var threadLine interface{}
		if len(t.Comments) > 0 {
			root := t.Comments[0]
			threadPath = root.Path
			if root.Line != nil {
				threadLine = *root.Line
			}
		}
		out = append(out, map[string]interface{}{
			"id":         prReviewThreadNodeID(t.ID),
			"isResolved": t.IsResolved,
			"isOutdated": false,
			"resolvedBy": nil,
			"path":       threadPath,
			"line":       threadLine,
			"comments": map[string]interface{}{
				"nodes":      commentNodes,
				"totalCount": len(commentNodes),
			},
		})
	}
	return out
}

func (s *Server) resolveReviewThreadGraphQL(p graphql.ResolveParams, resolved bool) (interface{}, error) {
	user := ghUserFromContext(p.Context)
	if user == nil {
		return nil, fmt.Errorf("authentication required")
	}
	input, _ := p.Args["input"].(map[string]interface{})
	threadNodeID, _ := input["threadId"].(string)
	threadID, ok := parsePRReviewThreadNodeID(threadNodeID)
	if !ok {
		return nil, &ghNotFoundError{
			message: fmt.Sprintf("Could not resolve to a PullRequestReviewThread with the global id of '%s'", threadNodeID),
		}
	}
	if !s.store.PRReviewComments.ResolveThread(threadID, resolved) {
		return nil, &ghNotFoundError{
			message: fmt.Sprintf("Could not resolve to a PullRequestReviewThread with the global id of '%s'", threadNodeID),
		}
	}
	thread := s.store.PRReviewComments.GetThread(threadID)
	if thread == nil {
		return nil, &ghNotFoundError{
			message: fmt.Sprintf("Could not resolve to a PullRequestReviewThread with the global id of '%s'", threadNodeID),
		}
	}
	s.store.mu.RLock()
	nodes := reviewThreadsForGraphQL([]*ReviewThread{thread}, s.store)
	s.store.mu.RUnlock()
	var gqlThread interface{}
	if len(nodes) == 1 {
		gqlThread = nodes[0]
	}
	var clientMutationID interface{}
	if v, ok := input["clientMutationId"].(string); ok && v != "" {
		clientMutationID = v
	}
	return map[string]interface{}{
		"thread":           gqlThread,
		"clientMutationId": clientMutationID,
	}, nil
}

func prReviewThreadNodeID(threadID int) string {
	return fmt.Sprintf("PRT_kgDO%08d", threadID)
}

func parsePRReviewThreadNodeID(nodeID string) (int, bool) {
	const prefix = "PRT_kgDO"
	if !strings.HasPrefix(nodeID, prefix) {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimPrefix(nodeID, prefix))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// prMilestoneToGQLLocked returns the GraphQL source map for the PR's
// milestone, or nil when the PR has no milestone or the referenced
// milestone has been deleted. Real GitHub shares the Milestone table
// between Issues and PRs; bleephub mirrors that.
func prMilestoneToGQLLocked(pr *PullRequest, st *Store) interface{} {
	if pr.MilestoneID == 0 {
		return nil
	}
	ms, ok := st.Milestones[pr.MilestoneID]
	if !ok {
		return nil
	}
	var dueOn interface{}
	if ms.DueOn != nil {
		dueOn = ms.DueOn.Format(time.RFC3339)
	}
	return map[string]interface{}{
		"number":      ms.Number,
		"title":       ms.Title,
		"description": ms.Description,
		"dueOn":       dueOn,
	}
}

// prCommentToGQLLocked builds the GraphQL source map for a single
// PR conversation comment. Caller must hold st.mu.RLock.
func prCommentToGQLLocked(c *Comment, st *Store) map[string]interface{} {
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
		"author":              author,
		"authorAssociation":   "OWNER",
		"includesCreatedEdit": c.LastEditedAt != nil,
		"lastEditedAt":        lastEditedAt,
		"editor":              editor,
		"isMinimized":         c.MinimizedReason != "",
		"minimizedReason":     nilStr(c.MinimizedReason),
		"reactionGroups":      reactionGroupsForGraphQL(st.Reactions, "issue_comment", c.ID),
	}
}

// deriveReviewDecisionLocked derives the review decision from reviews.
// Must be called while holding st.mu.RLock().
func deriveReviewDecisionLocked(st *Store, prID int) string {
	hasApproved := false
	hasChangesRequested := false
	for _, r := range st.PRReviewsByPR[prID] {
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
	return paginateGQL(prs, first, after, func(pr *PullRequest) map[string]interface{} {
		return pullRequestToGQL(pr, st)
	})
}

// prReviewSourceLocked builds the GraphQL source map for a single PR review.
// Caller must hold st.mu.RLock. submittedAt mirrors createdAt (bleephub
// reviews are submitted on creation) and commit carries the PR head the
// review was recorded against — the same oid REST's commit_id reports.
func prReviewSourceLocked(r *PullRequestReview, st *Store) map[string]interface{} {
	var reviewAuthor map[string]interface{}
	if u, ok := st.Users[r.AuthorID]; ok {
		reviewAuthor = userToGraphQL(u)
	}
	commitSHA := ""
	if pr := st.PullRequests[r.PRID]; pr != nil {
		commitSHA = pullRequestHeadSHALocked(pr, st)
	}
	return map[string]interface{}{
		"_dbID":             r.ID,
		"nodeID":            r.NodeID,
		"body":              r.Body,
		"state":             r.State,
		"author":            reviewAuthor,
		"authorAssociation": "OWNER",
		"createdAt":         r.CreatedAt.Format(time.RFC3339),
		"updatedAt":         r.UpdatedAt.Format(time.RFC3339),
		"submittedAt":       r.CreatedAt.Format(time.RFC3339),
		"commit":            map[string]interface{}{"oid": commitSHA},
		"reactionGroups":    reactionGroupsForGraphQL(st.Reactions, "pull_request_review", r.ID),
	}
}

func pullRequestReviewRequestNodesLocked(pr *PullRequest, st *Store) []interface{} {
	nodes := make([]interface{}, 0, len(pr.RequestedReviewerIDs))
	for _, id := range pr.RequestedReviewerIDs {
		if u := st.Users[id]; u != nil {
			nodes = append(nodes, map[string]interface{}{
				"requestedReviewer": userToGraphQL(u),
			})
		}
	}
	return nodes
}

func gitCommitToGQLLocked(c *object.Commit, st *Store, repoFullName string) map[string]interface{} {
	authors := []interface{}{
		map[string]interface{}{
			"name":  c.Author.Name,
			"email": c.Author.Email,
			"user":  userGraphQLByEmailLocked(st, c.Author.Email),
		},
	}
	return map[string]interface{}{
		"oid":               c.Hash.String(),
		"messageHeadline":   strings.SplitN(c.Message, "\n", 2)[0],
		"messageBody":       commitMessageBody(c.Message),
		"committedDate":     c.Committer.When.UTC().Format(time.RFC3339),
		"authoredDate":      c.Author.When.UTC().Format(time.RFC3339),
		"authors":           map[string]interface{}{"nodes": authors, "totalCount": len(authors)},
		"statusCheckRollup": statusCheckRollupSourceLocked(st, repoFullName, c.Hash.String()),
	}
}

func userGraphQLByEmailLocked(st *Store, email string) map[string]interface{} {
	for _, u := range st.Users {
		if strings.EqualFold(u.Email, email) {
			return userToGraphQL(u)
		}
	}
	return nil
}

func commitMessageBody(message string) interface{} {
	parts := strings.SplitN(message, "\n", 2)
	if len(parts) < 2 {
		return nil
	}
	body := strings.TrimSpace(parts[1])
	if body == "" {
		return nil
	}
	return body
}

// prReviewToGQL is the unlocked wrapper around prReviewSourceLocked.
func prReviewToGQL(r *PullRequestReview, st *Store) map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return prReviewSourceLocked(r, st)
}

// statusCheckRollupSourceLocked builds Commit.statusCheckRollup from the real
// checks and commit-status stores for (repoKey, sha): one StatusContext node
// per latest REST status context and one CheckRun node per stored check run.
// Returns nil when neither store has data, matching real GitHub for a commit
// with no statuses or check runs. Caller must hold st.mu.RLock.
func statusCheckRollupSourceLocked(st *Store, repoKey, sha string) interface{} {
	if repoKey == "" {
		return nil
	}
	_, _, statuses := st.CommitStatuses.Combined(repoKey, sha)
	var runs []*CheckRun
	for _, cr := range st.CheckRuns {
		if cr.RepoKey == repoKey && cr.HeadSHA == sha {
			runs = append(runs, cr)
		}
	}
	if len(statuses) == 0 && len(runs) == 0 {
		return nil
	}
	sort.Slice(statuses, func(a, b int) bool {
		return statuses[a].CreatedAt.Before(statuses[b].CreatedAt)
	})
	sort.Slice(runs, func(a, b int) bool { return runs[a].ID < runs[b].ID })

	allCompleted := true
	anyFailed := false
	nodes := make([]interface{}, 0, len(statuses)+len(runs))
	statusContextCounts := map[string]int{}
	for _, status := range statuses {
		if status.State == "pending" {
			allCompleted = false
		}
		statusState := strings.ToUpper(status.State)
		statusContextCounts[statusState]++
		switch status.State {
		case "failure", "error":
			anyFailed = true
		}
		nodes = append(nodes, map[string]interface{}{
			"__typename":  "StatusContext",
			"context":     status.Context,
			"state":       statusState,
			"targetUrl":   nilStr(status.TargetURL),
			"createdAt":   status.CreatedAt.Format(time.RFC3339),
			"description": nilStr(status.Description),
		})
	}
	checkRunCounts := map[string]int{}
	for _, cr := range runs {
		var conclusion interface{}
		if cr.Conclusion != "" {
			conclusion = strings.ToUpper(cr.Conclusion)
		}
		var completedAt interface{}
		if cr.CompletedAt != nil {
			completedAt = cr.CompletedAt.Format(time.RFC3339)
		}
		if cr.Status != "completed" {
			allCompleted = false
		}
		checkRunCounts[checkRunCountState(cr)]++
		switch cr.Conclusion {
		case "failure", "timed_out", "cancelled", "startup_failure":
			anyFailed = true
		}
		var suiteSource interface{}
		if suite := st.CheckSuites[cr.SuiteID]; suite != nil {
			suiteSource = checkSuiteGraphQLSourceLocked(st, suite)
		}
		nodes = append(nodes, map[string]interface{}{
			"__typename":  "CheckRun",
			"name":        cr.Name,
			"status":      strings.ToUpper(cr.Status),
			"conclusion":  conclusion,
			"startedAt":   cr.StartedAt.Format(time.RFC3339),
			"completedAt": completedAt,
			"detailsUrl":  nilStr(cr.DetailsURL),
			"checkSuite":  suiteSource,
		})
	}

	state := "SUCCESS"
	switch {
	case anyFailed:
		state = "FAILURE"
	case !allCompleted:
		state = "PENDING"
	}

	return map[string]interface{}{
		"state": state,
		"contexts": map[string]interface{}{
			"nodes":                      nodes,
			"totalCount":                 len(nodes),
			"checkRunCount":              len(runs),
			"checkRunCountsByState":      stateCountNodes(checkRunStatesForCounts(), checkRunCounts),
			"statusContextCount":         len(statuses),
			"statusContextCountsByState": stateCountNodes(statusContextStatesForCounts(), statusContextCounts),
			"pageInfo":                   map[string]interface{}{"hasNextPage": false, "endCursor": nil},
		},
	}
}

func checkRunCountState(cr *CheckRun) string {
	if cr == nil {
		return "PENDING"
	}
	if cr.Status == "completed" && cr.Conclusion != "" {
		return strings.ToUpper(cr.Conclusion)
	}
	if cr.Status != "" {
		return strings.ToUpper(cr.Status)
	}
	return "PENDING"
}

func checkRunStatesForCounts() []string {
	return []string{
		"ACTION_REQUIRED",
		"CANCELLED",
		"COMPLETED",
		"FAILURE",
		"IN_PROGRESS",
		"NEUTRAL",
		"PENDING",
		"QUEUED",
		"SKIPPED",
		"STALE",
		"STARTUP_FAILURE",
		"SUCCESS",
		"TIMED_OUT",
		"WAITING",
	}
}

func statusContextStatesForCounts() []string {
	return []string{"EXPECTED", "ERROR", "FAILURE", "PENDING", "SUCCESS"}
}

func stateCountNodes(states []string, counts map[string]int) []interface{} {
	out := make([]interface{}, 0, len(states))
	for _, state := range states {
		out = append(out, map[string]interface{}{
			"state": state,
			"count": counts[state],
		})
	}
	return out
}

func checkSuiteGraphQLSourceLocked(st *Store, suite *CheckSuite) map[string]interface{} {
	if suite == nil {
		return nil
	}
	workflowRun := checkSuiteWorkflowRunSourceLocked(st, suite)
	return map[string]interface{}{"workflowRun": workflowRun}
}

func checkSuiteWorkflowRunSourceLocked(st *Store, suite *CheckSuite) map[string]interface{} {
	if suite == nil || suite.WorkflowRunID == 0 {
		return nil
	}
	workflowName := suite.WorkflowName
	for _, wf := range st.Workflows {
		if wf.RunID == suite.WorkflowRunID && wf.RepoFullName == suite.RepoKey {
			workflowName = wf.Name
			break
		}
	}
	if workflowName == "" {
		return nil
	}
	return map[string]interface{}{
		"workflow": map[string]interface{}{"name": workflowName},
	}
}

// searchIssuesAndPRs evaluates the issue-search query string gh sends to
// Query.search (type: ISSUE) against the real issue/PR stores. Supported
// qualifiers are evaluated for real: repo:, state:/is:/type:, author:,
// assignee:, label:, mentions:, involves: (author OR assignee OR commenter).
// review-requested: matches nothing — bleephub stores no review requests, so
// zero results IS the true answer. A qualifier bleephub cannot evaluate at
// all yields honest empty results (never an over-matching ignore). Bare
// keywords match title/body substrings. Results are newest-first.
func (s *Server) searchIssuesAndPRs(query string, viewer *User) []map[string]interface{} {
	type searchSpec struct {
		repos      []string // repo full names; empty = all
		states     []string // OPEN / CLOSED / MERGED; empty = all
		entity     string   // "issue", "pr", or "" for both
		author     string
		assignee   string
		mentions   string
		involves   string
		labels     []string
		keywords   []string
		draft      *bool
		impossible bool
	}
	spec := searchSpec{}
	boolPtr := func(b bool) *bool { return &b }

	for _, tok := range strings.Fields(query) {
		// The advanced-search syntax gh builds may group qualifiers with
		// parentheses and OR; strip the punctuation and treat the qualifier
		// tokens individually (single-valued groups, which is what gh emits
		// for these commands, evaluate identically).
		tok = strings.Trim(tok, "()")
		if tok == "" || strings.EqualFold(tok, "OR") || strings.EqualFold(tok, "AND") {
			continue
		}
		key, val, isQualifier := strings.Cut(tok, ":")
		if isQualifier {
			val = strings.Trim(val, `"`)
		}
		if !isQualifier {
			spec.keywords = append(spec.keywords, strings.Trim(tok, `"`))
			continue
		}
		switch strings.ToLower(key) {
		case "repo":
			spec.repos = append(spec.repos, val)
		case "state":
			spec.states = append(spec.states, strings.ToUpper(val))
		case "is":
			switch strings.ToLower(val) {
			case "open", "closed", "merged":
				spec.states = append(spec.states, strings.ToUpper(val))
			case "pr", "issue":
				spec.entity = strings.ToLower(val)
			case "draft":
				spec.entity = "pr"
				spec.draft = boolPtr(true)
			default:
				spec.impossible = true
			}
		case "type":
			spec.entity = strings.ToLower(val)
		case "draft":
			switch strings.ToLower(val) {
			case "true":
				spec.draft = boolPtr(true)
			case "false":
				spec.draft = boolPtr(false)
			default:
				spec.impossible = true
			}
		case "author":
			spec.author = val
		case "assignee":
			spec.assignee = val
		case "mentions":
			spec.mentions = val
		case "involves":
			spec.involves = val
		case "label":
			spec.labels = append(spec.labels, val)
		case "review-requested", "user-review-requested":
			// No review-request store: nothing can match.
			spec.impossible = true
		case "sort":
			// Default newest-first ordering is the only one modeled.
		default:
			spec.impossible = true
		}
	}
	if spec.impossible {
		return nil
	}

	s.store.mu.RLock()
	// Real GitHub search only returns results from repositories the
	// authenticated viewer can access; a private repo the viewer can't read
	// must never contribute issues/PRs. Mirror canReadRepo's logic inline
	// (the store RLock is already held here, so we access its maps directly
	// rather than calling the helpers, which take the lock themselves).
	repoReadable := func(repo *Repo) bool {
		if repo == nil {
			return false
		}
		if !repo.Private {
			return true
		}
		if viewer == nil {
			return false
		}
		if repo.Owner != nil && repo.Owner.ID == viewer.ID {
			return true
		}
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) != 2 {
			return false
		}
		if m := s.store.Memberships[membershipKey(parts[0], viewer.ID)]; m != nil && m.State == MembershipStateActive {
			return true
		}
		// Team-level pull access.
		if org := s.store.OrgsByLogin[parts[0]]; org != nil {
			for _, team := range s.store.TeamsBySlug {
				if team.OrgID != org.ID || !permissionAtLeast(team.Permission, TeamPermissionPull) {
					continue
				}
				inRepo := false
				for _, rn := range team.RepoNames {
					if rn == repo.FullName {
						inRepo = true
						break
					}
				}
				if !inRepo {
					continue
				}
				for _, mid := range team.MemberIDs {
					if mid == viewer.ID {
						return true
					}
				}
			}
		}
		return false
	}
	// Candidate repos.
	var repoIDs []int
	if len(spec.repos) > 0 {
		for _, full := range spec.repos {
			if r, ok := s.store.ReposByName[full]; ok {
				repoIDs = append(repoIDs, r.ID)
			}
		}
	} else {
		for id := range s.store.Repos {
			repoIDs = append(repoIDs, id)
		}
	}
	repoSet := make(map[int]bool, len(repoIDs))
	for _, id := range repoIDs {
		if repoReadable(s.store.Repos[id]) {
			repoSet[id] = true
		}
	}

	loginOf := func(userID int) string {
		if u, ok := s.store.Users[userID]; ok {
			return u.Login
		}
		return ""
	}
	stateMatches := func(state string) bool {
		if len(spec.states) == 0 {
			return true
		}
		for _, want := range spec.states {
			if state == want {
				return true
			}
		}
		return false
	}
	keywordsMatch := func(title, body string) bool {
		haystack := strings.ToLower(title + "\n" + body)
		for _, kw := range spec.keywords {
			if !strings.Contains(haystack, strings.ToLower(kw)) {
				return false
			}
		}
		return true
	}
	commenterMatch := func(parentType string, parentID int, login string) bool {
		for _, c := range s.store.Comments {
			if c.ParentType == parentType && c.IssueID == parentID && loginOf(c.AuthorID) == login {
				return true
			}
		}
		return false
	}
	assigneeMatch := func(assigneeIDs []int, login string) bool {
		for _, id := range assigneeIDs {
			if loginOf(id) == login {
				return true
			}
		}
		return false
	}
	labelsMatch := func(labelIDs []int) bool {
		for _, want := range spec.labels {
			found := false
			for _, lid := range labelIDs {
				if l, ok := s.store.Labels[lid]; ok && l.Name == want {
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

	var matchedIssues []*Issue
	var matchedPRs []*PullRequest

	if (spec.entity == "" || spec.entity == "issue") && spec.draft == nil {
		for _, issue := range s.store.Issues {
			if !repoSet[issue.RepoID] || !stateMatches(issue.State) {
				continue
			}
			if spec.author != "" && loginOf(issue.AuthorID) != spec.author {
				continue
			}
			if spec.assignee != "" && !assigneeMatch(issue.AssigneeIDs, spec.assignee) {
				continue
			}
			if spec.mentions != "" && !strings.Contains(issue.Body, "@"+spec.mentions) {
				continue
			}
			if spec.involves != "" &&
				loginOf(issue.AuthorID) != spec.involves &&
				!assigneeMatch(issue.AssigneeIDs, spec.involves) &&
				!commenterMatch("issue", issue.ID, spec.involves) {
				continue
			}
			if !labelsMatch(issue.LabelIDs) || !keywordsMatch(issue.Title, issue.Body) {
				continue
			}
			matchedIssues = append(matchedIssues, issue)
		}
	}
	if spec.entity == "" || spec.entity == "pr" {
		for _, pr := range s.store.PullRequests {
			if !repoSet[pr.RepoID] || !stateMatches(pr.State) {
				continue
			}
			if spec.draft != nil && pr.IsDraft != *spec.draft {
				continue
			}
			if spec.author != "" && loginOf(pr.AuthorID) != spec.author {
				continue
			}
			if spec.assignee != "" && !assigneeMatch(pr.AssigneeIDs, spec.assignee) {
				continue
			}
			if spec.mentions != "" && !strings.Contains(pr.Body, "@"+spec.mentions) {
				continue
			}
			if spec.involves != "" &&
				loginOf(pr.AuthorID) != spec.involves &&
				!assigneeMatch(pr.AssigneeIDs, spec.involves) &&
				!commenterMatch("pull_request", pr.ID, spec.involves) {
				continue
			}
			if !labelsMatch(pr.LabelIDs) || !keywordsMatch(pr.Title, pr.Body) {
				continue
			}
			matchedPRs = append(matchedPRs, pr)
		}
	}
	s.store.mu.RUnlock()

	// Render outside the lock (the toGQL converters take it themselves),
	// newest-first across both entity kinds.
	type dated struct {
		created time.Time
		node    map[string]interface{}
	}
	out := make([]dated, 0, len(matchedIssues)+len(matchedPRs))
	for _, issue := range matchedIssues {
		node := issueToGQL(issue, s.store)
		node["__typename"] = "Issue"
		out = append(out, dated{issue.CreatedAt, node})
	}
	for _, pr := range matchedPRs {
		node := pullRequestToGQL(pr, s.store)
		out = append(out, dated{pr.CreatedAt, node})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].created.After(out[b].created) })

	nodes := make([]map[string]interface{}, 0, len(out))
	for _, d := range out {
		nodes = append(nodes, d.node)
	}
	return nodes
}
