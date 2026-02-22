package bleephub

import (
	"sort"
	"time"

	"github.com/graphql-go/graphql"
)

// addOrgFieldsToSchema adds Organization types, queries, and viewer.organizations to the schema.
func (s *Server) addOrgFieldsToSchema(userType, queryType *graphql.Object) {
	orgType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Organization",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					o := p.Source.(map[string]interface{})
					return o["nodeID"], nil
				},
			},
			"databaseId":  &graphql.Field{Type: graphql.Int},
			"login":       &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"name":        &graphql.Field{Type: graphql.String},
			"description": &graphql.Field{Type: graphql.String},
			"email":       &graphql.Field{Type: graphql.String},
			"url":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"avatarUrl":   &graphql.Field{Type: graphql.String},
			"createdAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	orgPageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "OrgPageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	orgEdgeType := graphql.NewObject(graphql.ObjectConfig{
		Name: "OrganizationEdge",
		Fields: graphql.Fields{
			"node":   &graphql.Field{Type: orgType},
			"cursor": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	orgConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "OrganizationConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(orgType)},
			"edges":      &graphql.Field{Type: graphql.NewList(orgEdgeType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(orgPageInfoType)},
		},
	})

	// Add organizations field to User type (for viewer.organizations)
	userType.AddFieldConfig("organizations", &graphql.Field{
		Type: orgConnectionType,
		Args: graphql.FieldConfigArgument{
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
			"after": &graphql.ArgumentConfig{Type: graphql.String},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			u := p.Source.(map[string]interface{})
			dbID, _ := u["databaseId"].(int)

			orgs := s.store.ListOrgsByUser(dbID)

			// Sort by creation time (newest first)
			sort.Slice(orgs, func(i, j int) bool {
				return orgs[i].CreatedAt.After(orgs[j].CreatedAt)
			})

			first := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				first = f
			}
			after, _ := p.Args["after"].(string)

			return paginateOrgs(orgs, first, after), nil
		},
	})

	// Add organization query to queryType
	queryType.AddFieldConfig("organization", &graphql.Field{
		Type: orgType,
		Args: graphql.FieldConfigArgument{
			"login": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			login, _ := p.Args["login"].(string)
			org := s.store.GetOrg(login)
			if org == nil {
				return nil, nil
			}
			return orgToGraphQL(org), nil
		},
	})
}

// orgToGraphQL converts an Org to a map for GraphQL resolvers.
func orgToGraphQL(org *Org) map[string]interface{} {
	return map[string]interface{}{
		"nodeID":      org.NodeID,
		"databaseId":  org.ID,
		"login":       org.Login,
		"name":        org.Name,
		"description": org.Description,
		"email":       org.Email,
		"url":         "/" + org.Login,
		"avatarUrl":   org.AvatarURL,
		"createdAt":   org.CreatedAt.Format(time.RFC3339),
		"updatedAt":   org.UpdatedAt.Format(time.RFC3339),
	}
}

// paginateOrgs implements Relay-style cursor pagination for organizations.
func paginateOrgs(orgs []*Org, first int, after string) map[string]interface{} {
	total := len(orgs)

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

	page := orgs[startIdx:endIdx]

	nodes := make([]map[string]interface{}, 0, len(page))
	edges := make([]map[string]interface{}, 0, len(page))
	for i, o := range page {
		gql := orgToGraphQL(o)
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
