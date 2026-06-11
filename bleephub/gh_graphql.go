package bleephub

import (
	"fmt"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"
)

// ghNotFoundError marks a resolver lookup miss that must surface as a
// GitHub-shaped errors[] entry carrying `"type": "NOT_FOUND"`. That member
// sits OUTSIDE the GraphQL spec's standard error keys — it's GitHub-specific,
// and gh CLI / go-gh key on it (e.g. the PR finder distinguishes "no such PR"
// from transport errors by Type == "NOT_FOUND"). Returning bare null data
// without the typed error makes clients decode a zero-valued object instead
// of reporting "not found".
type ghNotFoundError struct {
	message string
}

func (e *ghNotFoundError) Error() string { return e.message }

// ghErrorIsNotFound unwraps graphql-go's error layering (FormattedError →
// *gqlerrors.Error → resolver error) looking for a ghNotFoundError.
func ghErrorIsNotFound(err error) bool {
	for err != nil {
		if _, ok := err.(*ghNotFoundError); ok {
			return true
		}
		switch e := err.(type) {
		case *gqlerrors.Error:
			err = e.OriginalError
		case gqlerrors.Error:
			err = e.OriginalError
		case gqlerrors.FormattedError:
			err = e.OriginalError()
		default:
			return false
		}
	}
	return false
}

// initGraphQLSchema builds the GraphQL schema with all types and resolvers.
func (s *Server) initGraphQLSchema() {
	userType := graphql.NewObject(graphql.ObjectConfig{
		Name: "User",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					u := p.Source.(map[string]interface{})
					return u["nodeID"], nil
				},
			},
			"databaseId": &graphql.Field{Type: graphql.Int},
			"login":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"name":       &graphql.Field{Type: graphql.String},
			"email":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"avatarUrl":  &graphql.Field{Type: graphql.String},
			"bio":        &graphql.Field{Type: graphql.String},
			"url":        &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"createdAt":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"viewer": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					user := ghUserFromContext(p.Context)
					if user == nil {
						return nil, nil
					}
					return userToGraphQL(user), nil
				},
			},
		},
	})

	// Add repository types, queries, and mutations
	repoType, mutationType := s.addRepoFieldsToSchema(userType, queryType)

	// Add organization types and queries
	s.addOrgFieldsToSchema(userType, queryType)

	// Add issue types, queries, and mutations
	issueType := s.addIssueFieldsToSchema(userType, repoType, mutationType, queryType)

	// Add pull request types, queries, and mutations
	s.addPullRequestFieldsToSchema(userType, issueType, repoType, mutationType, queryType)

	// Add moderation mutations (minimize/unminimize comment, lock/unlock).
	s.addModerationMutationsToSchema(mutationType)

	// Add Projects v2 mutations (createProjectV2, addProjectV2ItemById).
	s.addProjectV2MutationsToSchema(mutationType)

	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create graphql schema: %v", err))
	}
	s.graphqlSchema = schema
}

func (s *Server) registerGHGraphQLRoutes() {
	s.route("POST /api/graphql", s.handleGraphQL)
}

// handleGraphQL executes a GraphQL query.
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	result := graphql.Do(graphql.Params{
		Schema:         s.graphqlSchema,
		RequestString:  req.Query,
		VariableValues: req.Variables,
		OperationName:  req.OperationName,
		Context:        r.Context(),
	})

	// Debug: log the query + any errors so the harness can diagnose which
	// gh CLI queries hit unimplemented fields.
	if len(result.Errors) > 0 {
		s.logger.Debug().
			Str("operation", req.OperationName).
			Str("query", req.Query).
			Interface("errors", result.Errors).
			Msg("graphql errors")
	}

	// Re-shape errors[] into GitHub's wire form: real GitHub adds a
	// non-spec top-level "type" member (NOT_FOUND, FORBIDDEN, ...) that
	// graphql-go's FormattedError cannot carry, so the envelope is built
	// by hand instead of serializing graphql.Result directly.
	out := map[string]interface{}{"data": result.Data}
	if len(result.Errors) > 0 {
		errItems := make([]map[string]interface{}, 0, len(result.Errors))
		for _, fe := range result.Errors {
			item := map[string]interface{}{"message": fe.Message}
			if len(fe.Locations) > 0 {
				item["locations"] = fe.Locations
			}
			if len(fe.Path) > 0 {
				item["path"] = fe.Path
			}
			if len(fe.Extensions) > 0 {
				item["extensions"] = fe.Extensions
			}
			if ghErrorIsNotFound(fe) {
				item["type"] = "NOT_FOUND"
			}
			errItems = append(errItems, item)
		}
		out["errors"] = errItems
	}
	writeJSON(w, http.StatusOK, out)
}

// userToGraphQL converts a User to a map with camelCase keys for GraphQL resolvers.
func userToGraphQL(u *User) map[string]interface{} {
	return map[string]interface{}{
		"nodeID":     u.NodeID,
		"databaseId": u.ID,
		"login":      u.Login,
		"name":       u.Name,
		"email":      u.Email,
		"avatarUrl":  u.AvatarURL,
		"bio":        u.Bio,
		"url":        "/" + u.Login,
		"createdAt":  u.CreatedAt.Format(time.RFC3339),
		"updatedAt":  u.UpdatedAt.Format(time.RFC3339),
	}
}
