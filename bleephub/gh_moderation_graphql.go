package bleephub

import (
	"fmt"

	"github.com/graphql-go/graphql"
)

// gqlSourceField reads key from a resolver's p.Source when Source is a
// map[string]interface{}, returning nil otherwise. graphql-go only invokes a
// sub-field resolver when the parent value is non-null, so in normal operation
// Source is always the payload map; the comma-ok keeps a future schema change
// (or a hand-driven fuzz input) from panicking.
func gqlSourceField(p graphql.ResolveParams, key string) interface{} {
	m, ok := p.Source.(map[string]interface{})
	if !ok {
		return nil
	}
	return m[key]
}

// addModerationMutationsToSchema registers comment minimization +
// issue/PR locking GraphQL mutations against the shared mutationType.
// Mirrors real GitHub's mutation surface: minimizeComment /
// unminimizeComment / lockLockable / unlockLockable.
func (s *Server) addModerationMutationsToSchema(mutationType *graphql.Object) {
	// --- minimizeComment / unminimizeComment ---

	classifierEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "ReportedContentClassifier",
		Values: graphql.EnumValueConfigMap{
			"OFF_TOPIC": &graphql.EnumValueConfig{Value: "OFF_TOPIC"},
			"OUTDATED":  &graphql.EnumValueConfig{Value: "OUTDATED"},
			"RESOLVED":  &graphql.EnumValueConfig{Value: "RESOLVED"},
			"DUPLICATE": &graphql.EnumValueConfig{Value: "DUPLICATE"},
			"SPAM":      &graphql.EnumValueConfig{Value: "SPAM"},
			"ABUSE":     &graphql.EnumValueConfig{Value: "ABUSE"},
		},
	})

	minimizeInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "MinimizeCommentInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"subjectId":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"classifier": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(classifierEnum)},
		},
	})

	unminimizeInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UnminimizeCommentInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"subjectId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})

	// Payload type carries the minimized comment by node id + the resolved
	// state — gh CLI / Octokit tests typically read minimizedComment { id
	// isMinimized minimizedReason } to confirm the mutation worked.
	minimizedCommentType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MinimizableComment",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return gqlSourceField(p, "nodeID"), nil
				},
			},
			"isMinimized": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return gqlSourceField(p, "isMinimized"), nil
				},
			},
			"minimizedReason": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return gqlSourceField(p, "minimizedReason"), nil
				},
			},
		},
	})

	minimizePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "MinimizeCommentPayload",
		Fields: graphql.Fields{
			"minimizedComment": &graphql.Field{Type: minimizedCommentType},
		},
	})

	unminimizePayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UnminimizeCommentPayload",
		Fields: graphql.Fields{
			"unminimizedComment": &graphql.Field{Type: minimizedCommentType},
		},
	})

	mutationType.AddFieldConfig("minimizeComment", &graphql.Field{
		Type: minimizePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(minimizeInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			nodeID, _ := input["subjectId"].(string)
			classifier, _ := input["classifier"].(string)
			c := s.store.LookupCommentByNodeID(nodeID)
			if c == nil {
				return nil, fmt.Errorf("could not resolve to a node with the global id of '%s'", nodeID)
			}
			updated := s.store.SetCommentMinimization(c.ID, user.ID, classifier)
			return map[string]interface{}{
				"minimizedComment": map[string]interface{}{
					"nodeID":          updated.NodeID,
					"isMinimized":     updated.MinimizedReason != "",
					"minimizedReason": nilStr(updated.MinimizedReason),
				},
			}, nil
		},
	})

	mutationType.AddFieldConfig("unminimizeComment", &graphql.Field{
		Type: unminimizePayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(unminimizeInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			nodeID, _ := input["subjectId"].(string)
			c := s.store.LookupCommentByNodeID(nodeID)
			if c == nil {
				return nil, fmt.Errorf("could not resolve to a node with the global id of '%s'", nodeID)
			}
			updated := s.store.SetCommentMinimization(c.ID, user.ID, "")
			return map[string]interface{}{
				"unminimizedComment": map[string]interface{}{
					"nodeID":          updated.NodeID,
					"isMinimized":     false,
					"minimizedReason": nil,
				},
			}, nil
		},
	})

	// --- lockLockable / unlockLockable ---

	lockReasonEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "LockReason",
		Values: graphql.EnumValueConfigMap{
			"OFF_TOPIC":  &graphql.EnumValueConfig{Value: "OFF_TOPIC"},
			"RESOLVED":   &graphql.EnumValueConfig{Value: "RESOLVED"},
			"SPAM":       &graphql.EnumValueConfig{Value: "SPAM"},
			"TOO_HEATED": &graphql.EnumValueConfig{Value: "TOO_HEATED"},
		},
	})

	lockInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "LockLockableInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"lockableId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"lockReason": &graphql.InputObjectFieldConfig{Type: lockReasonEnum},
		},
	})

	unlockInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "UnlockLockableInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"lockableId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})

	lockedRecordType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LockableSummary",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return gqlSourceField(p, "nodeID"), nil
				},
			},
			"locked": &graphql.Field{
				Type: graphql.NewNonNull(graphql.Boolean),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return gqlSourceField(p, "locked"), nil
				},
			},
			"activeLockReason": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return gqlSourceField(p, "activeLockReason"), nil
				},
			},
		},
	})

	lockPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LockLockablePayload",
		Fields: graphql.Fields{
			"lockedRecord": &graphql.Field{Type: lockedRecordType},
		},
	})

	unlockPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "UnlockLockablePayload",
		Fields: graphql.Fields{
			"unlockedRecord": &graphql.Field{Type: lockedRecordType},
		},
	})

	mutationType.AddFieldConfig("lockLockable", &graphql.Field{
		Type: lockPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(lockInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			nodeID, _ := input["lockableId"].(string)
			reasonEnum, _ := input["lockReason"].(string)
			restReason := graphqlToRESTLockReason(reasonEnum)
			locked, ok := s.lockByNodeID(nodeID, true, restReason)
			if !ok {
				return nil, fmt.Errorf("could not resolve to a lockable node with the global id of '%s'", nodeID)
			}
			return map[string]interface{}{"lockedRecord": locked}, nil
		},
	})

	mutationType.AddFieldConfig("unlockLockable", &graphql.Field{
		Type: unlockPayloadType,
		Args: graphql.FieldConfigArgument{
			"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(unlockInputType)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user := ghUserFromContext(p.Context)
			if user == nil {
				return nil, fmt.Errorf("authentication required")
			}
			input, _ := p.Args["input"].(map[string]interface{})
			nodeID, _ := input["lockableId"].(string)
			unlocked, ok := s.lockByNodeID(nodeID, false, "")
			if !ok {
				return nil, fmt.Errorf("could not resolve to a lockable node with the global id of '%s'", nodeID)
			}
			return map[string]interface{}{"unlockedRecord": unlocked}, nil
		},
	})
}

// graphqlToRESTLockReason maps the GraphQL LockReason enum (UPPER_SNAKE)
// to the REST API's kebab-cased reason string.
func graphqlToRESTLockReason(enum string) string {
	switch enum {
	case "OFF_TOPIC":
		return "off-topic"
	case "TOO_HEATED":
		return "too heated"
	case "RESOLVED":
		return "resolved"
	case "SPAM":
		return "spam"
	}
	return ""
}

// lockByNodeID resolves nodeID to an Issue or PullRequest, applies the
// requested lock state, and returns a source map suitable for the
// LockableSummary GraphQL type. The bool indicates whether a target was
// found.
func (s *Server) lockByNodeID(nodeID string, locked bool, reason string) (map[string]interface{}, bool) {
	if issue := findIssueByNodeID(s.store, nodeID); issue != nil {
		s.store.SetIssueOrPRLock(issue.RepoID, issue.Number, locked, reason)
		refreshed := s.store.GetIssue(issue.ID)
		if refreshed == nil {
			refreshed = issue
		}
		return map[string]interface{}{
			"nodeID":           refreshed.NodeID,
			"locked":           refreshed.Locked,
			"activeLockReason": nilStr(refreshed.ActiveLockReason),
		}, true
	}
	if pr := findPullRequestByNodeID(s.store, nodeID); pr != nil {
		s.store.SetIssueOrPRLock(pr.RepoID, pr.Number, locked, reason)
		refreshed := s.store.GetPullRequest(pr.ID)
		if refreshed == nil {
			refreshed = pr
		}
		return map[string]interface{}{
			"nodeID":           refreshed.NodeID,
			"locked":           refreshed.Locked,
			"activeLockReason": nilStr(refreshed.ActiveLockReason),
		}, true
	}
	return nil, false
}
