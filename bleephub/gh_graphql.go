package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
)

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
	s.mux.HandleFunc("POST /api/graphql", s.handleGraphQL)
}

// handleGraphQL executes a GraphQL query.
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	result := graphql.Do(graphql.Params{
		Schema:         s.graphqlSchema,
		RequestString:  req.Query,
		VariableValues: req.Variables,
		OperationName:  req.OperationName,
		Context:        r.Context(),
	})

	writeJSON(w, http.StatusOK, result)
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
