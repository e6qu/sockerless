package bleephub

import (
	"fmt"

	"github.com/graphql-go/graphql"
)

// addProjectV2MutationsToSchema registers the minimum GraphQL mutations
// gh CLI's `gh project create` + `gh project item-add` use:
//   - createProjectV2(input{ownerId, title}) → ProjectV2
//   - addProjectV2ItemById(input{projectId, contentId}) → ProjectV2Item
//
// Field-level mutations (updateProjectV2ItemFieldValue, etc.) aren't
// modeled yet; bleephub returns nil-valued field connections so callers
// can introspect but can't write.
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

			ownerID, ownerType, ok := resolveProjectOwner(s.store, ownerNodeID, user)
			if !ok {
				return nil, fmt.Errorf("could not resolve to an owner with the global id of '%s'", ownerNodeID)
			}
			proj := s.store.ProjectsV2.CreateProject(ownerID, ownerType, title)
			return map[string]interface{}{
				"projectV2": projectV2ToGQL(proj),
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
								return p.Source.(map[string]interface{})["nodeID"], nil
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
			item := s.store.ProjectsV2.AddItem(proj.ID, contentType, contentID)
			return map[string]interface{}{
				"item": map[string]interface{}{
					"nodeID": item.NodeID,
				},
			}, nil
		},
	})
}

// resolveProjectOwner maps a GraphQL node ID to (ownerID, ownerType).
// Supports User + Organization nodes. Falls back to the authenticated
// user when the node ID can't be resolved (gh CLI passes the org's id
// by default).
func resolveProjectOwner(st *Store, nodeID string, fallback *User) (int, string, bool) {
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
	if fallback != nil {
		return fallback.ID, "User", true
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
