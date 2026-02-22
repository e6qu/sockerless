package bleephub

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
)

// addRepoFieldsToSchema adds repository types, queries, and mutations to the schema.
// Called from initGraphQLSchema after userType and queryType are created.
func (s *Server) addRepoFieldsToSchema(userType, queryType *graphql.Object) *graphql.Object {
	refType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Ref",
		Fields: graphql.Fields{
			"name":   &graphql.Field{Type: graphql.String},
			"prefix": &graphql.Field{Type: graphql.String},
		},
	})

	repoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Repository",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r := p.Source.(map[string]interface{})
					return r["nodeID"], nil
				},
			},
			"databaseId":   &graphql.Field{Type: graphql.Int},
			"name":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"nameWithOwner": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description":  &graphql.Field{Type: graphql.String},
			"url":          &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"sshUrl":       &graphql.Field{Type: graphql.String},
			"isPrivate":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isFork":       &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isArchived":   &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"visibility":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"createdAt":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"pushedAt":     &graphql.Field{Type: graphql.String},
			"stargazerCount": &graphql.Field{Type: graphql.Int},
			"owner": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r := p.Source.(map[string]interface{})
					return r["owner"], nil
				},
			},
			"defaultBranchRef": &graphql.Field{
				Type: refType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r := p.Source.(map[string]interface{})
					branch, _ := r["defaultBranch"].(string)
					if branch == "" {
						return nil, nil
					}
					return map[string]interface{}{
						"name":   branch,
						"prefix": "refs/heads/",
					}, nil
				},
			},
		},
	})

	pageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":    &graphql.Field{Type: graphql.String},
			"endCursor":      &graphql.Field{Type: graphql.String},
		},
	})

	repoEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "RepositoryEdge",
		Fields: graphql.Fields{
			"node":   &graphql.Field{Type: repoType},
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	repoConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "RepositoryConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(repoType)},
			"edges":      &graphql.Field{Type: graphql.NewList(repoEdgeType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(pageInfoType)},
		},
	})

	// Add repositories field to User type
	userType.AddFieldConfig("repositories", &graphql.Field{
		Type: repoConnectionType,
		Args: graphql.FieldConfigArgument{
			"first":             &graphql.ArgumentConfig{Type: graphql.Int},
			"after":             &graphql.ArgumentConfig{Type: graphql.String},
			"privacy":           &graphql.ArgumentConfig{Type: graphql.String},
			"isFork":            &graphql.ArgumentConfig{Type: graphql.Boolean},
			"ownerAffiliations": &graphql.ArgumentConfig{Type: graphql.NewList(graphql.String)},
			"orderBy":           &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "RepositoryOrder",
				Fields: graphql.InputObjectConfigFieldMap{
					"field":     &graphql.InputObjectFieldConfig{Type: graphql.String},
					"direction": &graphql.InputObjectFieldConfig{Type: graphql.String},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			u := p.Source.(map[string]interface{})
			login, _ := u["login"].(string)
			repos := s.store.ListReposByOwner(login)

			// Filter by privacy
			if privacy, ok := p.Args["privacy"].(string); ok {
				var filtered []*Repo
				for _, r := range repos {
					switch strings.ToUpper(privacy) {
					case "PUBLIC":
						if !r.Private {
							filtered = append(filtered, r)
						}
					case "PRIVATE":
						if r.Private {
							filtered = append(filtered, r)
						}
					}
				}
				repos = filtered
			}

			// Filter by isFork
			if isFork, ok := p.Args["isFork"].(bool); ok {
				var filtered []*Repo
				for _, r := range repos {
					if r.Fork == isFork {
						filtered = append(filtered, r)
					}
				}
				repos = filtered
			}

			// Sort by creation time (newest first by default)
			sort.Slice(repos, func(i, j int) bool {
				return repos[i].CreatedAt.After(repos[j].CreatedAt)
			})

			first := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				first = f
			}
			after, _ := p.Args["after"].(string)

			return paginateRepos(repos, first, after), nil
		},
	})

	// Add repository query to queryType
	queryType.AddFieldConfig("repository", &graphql.Field{
		Type: repoType,
		Args: graphql.FieldConfigArgument{
			"owner": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"name":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			owner, _ := p.Args["owner"].(string)
			name, _ := p.Args["name"].(string)
			repo := s.store.GetRepo(owner, name)
			if repo == nil {
				return nil, nil
			}
			return repoToGraphQL(repo), nil
		},
	})

	// Build mutation type
	createRepoInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateRepositoryInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name":             &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"visibility":       &graphql.InputObjectFieldConfig{Type: graphql.String},
			"description":      &graphql.InputObjectFieldConfig{Type: graphql.String},
			"hasIssuesEnabled": &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
			"hasWikiEnabled":   &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
		},
	})

	deleteRepoInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "DeleteRepositoryInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"repositoryId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
		},
	})

	createRepoPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CreateRepositoryPayload",
		Fields: graphql.Fields{
			"repository": &graphql.Field{Type: repoType},
		},
	})

	deleteRepoPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "DeleteRepositoryPayload",
		Fields: graphql.Fields{
			"clientMutationId": &graphql.Field{Type: graphql.String},
		},
	})

	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createRepository": &graphql.Field{
				Type: createRepoPayloadType,
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createRepoInputType)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					user := ghUserFromContext(p.Context)
					if user == nil {
						return nil, fmt.Errorf("authentication required")
					}

					input, _ := p.Args["input"].(map[string]interface{})
					name, _ := input["name"].(string)
					description, _ := input["description"].(string)
					visibility, _ := input["visibility"].(string)

					private := strings.ToUpper(visibility) == "PRIVATE"

					repo := s.store.CreateRepo(user, name, description, private)
					if repo == nil {
						return nil, fmt.Errorf("repository creation failed")
					}

					return map[string]interface{}{
						"repository": repoToGraphQL(repo),
					}, nil
				},
			},
			"deleteRepository": &graphql.Field{
				Type: deleteRepoPayloadType,
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(deleteRepoInputType)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					user := ghUserFromContext(p.Context)
					if user == nil {
						return nil, fmt.Errorf("authentication required")
					}

					input, _ := p.Args["input"].(map[string]interface{})
					repoID, _ := input["repositoryId"].(string)

					// Find repo by node ID
					s.store.mu.RLock()
					var found *Repo
					for _, r := range s.store.Repos {
						if r.NodeID == repoID {
							found = r
							break
						}
					}
					s.store.mu.RUnlock()

					if found == nil {
						return nil, fmt.Errorf("could not resolve to a Repository with the global id of '%s'", repoID)
					}

					s.store.DeleteRepo(found.Owner.Login, found.Name)

					return map[string]interface{}{
						"clientMutationId": nil,
					}, nil
				},
			},
		},
	})

	return mutationType
}

// repoToGraphQL converts a Repo to a map for GraphQL resolvers.
func repoToGraphQL(repo *Repo) map[string]interface{} {
	var ownerMap map[string]interface{}
	if repo.Owner != nil {
		ownerMap = userToGraphQL(repo.Owner)
	}

	return map[string]interface{}{
		"nodeID":         repo.NodeID,
		"databaseId":     repo.ID,
		"name":           repo.Name,
		"nameWithOwner":  repo.FullName,
		"description":    repo.Description,
		"url":            "/" + repo.FullName,
		"sshUrl":         "git@bleephub.local:" + repo.FullName + ".git",
		"isPrivate":      repo.Private,
		"isFork":         repo.Fork,
		"isArchived":     repo.Archived,
		"visibility":     strings.ToUpper(repo.Visibility),
		"defaultBranch":  repo.DefaultBranch,
		"stargazerCount": repo.StargazersCount,
		"owner":          ownerMap,
		"createdAt":      repo.CreatedAt.Format(time.RFC3339),
		"updatedAt":      repo.UpdatedAt.Format(time.RFC3339),
		"pushedAt":       repo.PushedAt.Format(time.RFC3339),
	}
}

// paginateRepos implements Relay-style cursor pagination.
func paginateRepos(repos []*Repo, first int, after string) map[string]interface{} {
	total := len(repos)

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

	page := repos[startIdx:endIdx]

	nodes := make([]map[string]interface{}, 0, len(page))
	edges := make([]map[string]interface{}, 0, len(page))
	for i, r := range page {
		gql := repoToGraphQL(r)
		cursor := encodeCursor(startIdx + i)
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

func encodeCursor(idx int) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("cursor:%d", idx)))
}

func decodeCursor(s string) int {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0
	}
	str := string(b)
	if !strings.HasPrefix(str, "cursor:") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(str, "cursor:"))
	if err != nil {
		return 0
	}
	return n
}
