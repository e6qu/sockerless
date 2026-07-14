package bleephub

import (
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/graphql-go/graphql"
)

// addRepoFieldsToSchema adds repository types, queries, and mutations to the schema.
// Called from initGraphQLSchema after userType and queryType are created.
func (s *Server) addRepoFieldsToSchema(userType, queryType *graphql.Object) (*graphql.Object, *graphql.Object) {
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
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["nodeID"], nil
				},
			},
			"databaseId":     &graphql.Field{Type: graphql.Int},
			"name":           &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"nameWithOwner":  &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description":    &graphql.Field{Type: graphql.String},
			"url":            &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"sshUrl":         &graphql.Field{Type: graphql.String},
			"isPrivate":      &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isFork":         &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isArchived":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"visibility":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"createdAt":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"updatedAt":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"pushedAt":       &graphql.Field{Type: graphql.String},
			"stargazerCount": &graphql.Field{Type: graphql.Int},
			"owner": &graphql.Field{
				Type: userType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return r["owner"], nil
				},
			},
			"defaultBranchRef": &graphql.Field{
				Type: refType,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					r, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
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

	// --- Repository fields gh CLI selects (clone/create/view --json) ---
	// gh's `GitHubRepo` query (repo clone, pr create) selects hasWikiEnabled
	// and parent{...repo}; `gh repo view --json` exposes the wider static set
	// below. Fields backed by repository settings or implemented repository
	// features resolve from the same store state as the REST repository shape.

	repoType.AddFieldConfig("hasWikiEnabled", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["hasWiki"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing hasWiki")
			}
			return v, nil
		},
	})
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
	repoType.AddFieldConfig("parent", &graphql.Field{
		Type: repoType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			parentID, ok := r["parentID"].(int)
			if !ok {
				return nil, fmt.Errorf("repository source missing parentID")
			}
			if parentID == 0 {
				return nil, nil
			}
			parent := s.store.GetRepoByID(parentID)
			if parent == nil {
				return nil, fmt.Errorf("repository parent %d not found", parentID)
			}
			return repoToGraphQL(s.store, s.store.snapRepo(parent)), nil
		},
	})
	repoType.AddFieldConfig("templateRepository", &graphql.Field{
		Type: repoType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			templateID, ok := r["templateRepoID"].(int)
			if !ok {
				return nil, fmt.Errorf("repository source missing templateRepoID")
			}
			if templateID == 0 {
				return nil, nil
			}
			templateRepo := s.store.GetRepoByID(templateID)
			if templateRepo == nil {
				return nil, nil
			}
			return repoToGraphQL(s.store, s.store.snapRepo(templateRepo)), nil
		},
	})
	repoType.AddFieldConfig("homepageUrl", &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			homepage, ok := r["homepage"].(string)
			if !ok {
				return nil, fmt.Errorf("repository source missing homepage")
			}
			if homepage == "" {
				return nil, nil
			}
			return homepage, nil
		},
	})
	repoType.AddFieldConfig("hasProjectsEnabled", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["hasProjects"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing hasProjects")
			}
			return v, nil
		},
	})
	repoType.AddFieldConfig("hasDiscussionsEnabled", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["hasDiscussions"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing hasDiscussions")
			}
			return v, nil
		},
	})
	repoType.AddFieldConfig("forkCount", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Int),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, ok := r["databaseId"].(int)
			if !ok || repoID == 0 {
				return nil, fmt.Errorf("repository forkCount source missing databaseId")
			}
			return s.store.CountForks(repoID), nil
		},
	})
	repoType.AddFieldConfig("watchers", &graphql.Field{
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: "RepoWatcherConnection",
			Fields: graphql.Fields{
				"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			},
		}),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, ok := r["databaseId"].(int)
			if !ok || repoID == 0 {
				return nil, fmt.Errorf("repository watcher source missing databaseId")
			}
			return map[string]interface{}{"totalCount": len(s.store.ListRepoSubscribers(repoID))}, nil
		},
	})
	repoType.AddFieldConfig("licenseInfo", &graphql.Field{
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: "RepositoryLicense",
			Fields: graphql.Fields{
				"key":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
				"name":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
				"nickname": &graphql.Field{Type: graphql.String},
				"spdxId":   &graphql.Field{Type: graphql.String},
			},
		}),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			key, _ := r["licenseKey"].(string)
			if key == "" {
				return nil, nil
			}
			name, ok := r["licenseName"].(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("repository source missing licenseName")
			}
			spdxID, _ := r["licenseSPDX"].(string)
			return map[string]interface{}{
				"key":      key,
				"name":     name,
				"nickname": nil,
				"spdxId":   spdxID,
			}, nil
		},
	})
	languageType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Language",
		Fields: graphql.Fields{
			"name": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
	repoType.AddFieldConfig("primaryLanguage", &graphql.Field{
		// Backed by Repo.Language (settable via the REST repo surface);
		// null when unset, exactly like a language-less repo on GitHub.
		Type: languageType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			lang, _ := r["language"].(string)
			if lang == "" {
				return nil, nil
			}
			return map[string]interface{}{"name": lang}, nil
		},
	})
	repoType.AddFieldConfig("languages", &graphql.Field{
		// gh selects languages(first:100){edges{size,node{name}}}.
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: "LanguageConnection",
			Fields: graphql.Fields{
				"edges": &graphql.Field{Type: graphql.NewList(graphql.NewObject(graphql.ObjectConfig{
					Name: "LanguageEdge",
					Fields: graphql.Fields{
						"size": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
						"node": &graphql.Field{Type: graphql.NewNonNull(languageType)},
					},
				}))},
				"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			},
		}),
		Args: graphql.FieldConfigArgument{
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			fullName, _ := r["nameWithOwner"].(string)
			owner, repoName, _ := strings.Cut(fullName, "/")
			repo := s.store.GetRepo(owner, repoName)
			if repo == nil {
				return map[string]interface{}{"edges": []interface{}{}, "totalCount": 0}, nil
			}
			counts := s.store.computeRepoLanguages(repo)
			first := 100
			if n, ok := p.Args["first"].(int); ok && n > 0 && n < first {
				first = n
			}
			edges := make([]interface{}, 0, len(counts))
			// GitHub returns languages sorted by size descending.
			type pair struct {
				lang string
				size int64
			}
			pairs := make([]pair, 0, len(counts))
			for lang, size := range counts {
				pairs = append(pairs, pair{lang, size})
			}
			sort.Slice(pairs, func(i, j int) bool {
				if pairs[i].size != pairs[j].size {
					return pairs[i].size > pairs[j].size
				}
				return pairs[i].lang < pairs[j].lang
			})
			for i, p := range pairs {
				if i >= first {
					break
				}
				edges = append(edges, map[string]interface{}{
					"size": p.size,
					"node": map[string]interface{}{"name": p.lang},
				})
			}
			return map[string]interface{}{"edges": edges, "totalCount": len(pairs)}, nil
		},
	})
	repoType.AddFieldConfig("repositoryTopics", &graphql.Field{
		// Backed by Repo.Topics (REST PUT /repos/{o}/{r}/topics).
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: "RepositoryTopicConnection",
			Fields: graphql.Fields{
				"nodes": &graphql.Field{Type: graphql.NewList(graphql.NewObject(graphql.ObjectConfig{
					Name: "RepositoryTopic",
					Fields: graphql.Fields{
						"topic": &graphql.Field{Type: graphql.NewNonNull(graphql.NewObject(graphql.ObjectConfig{
							Name: "Topic",
							Fields: graphql.Fields{
								"name": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
							},
						}))},
					},
				}))},
				"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			},
		}),
		Args: graphql.FieldConfigArgument{
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			topics, _ := r["topics"].([]string)
			nodes := make([]interface{}, 0, len(topics))
			for _, tp := range topics {
				nodes = append(nodes, map[string]interface{}{
					"topic": map[string]interface{}{"name": tp},
				})
			}
			return map[string]interface{}{"nodes": nodes, "totalCount": len(nodes)}, nil
		},
	})
	repoType.AddFieldConfig("deleteBranchOnMerge", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["deleteBranchOnMerge"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing deleteBranchOnMerge")
			}
			return v, nil
		},
	})
	repoType.AddFieldConfig("isTemplate", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			v, ok := r["isTemplate"].(bool)
			if !ok {
				return nil, fmt.Errorf("repository source missing isTemplate")
			}
			return v, nil
		},
	})
	repoType.AddFieldConfig("isEmpty", &graphql.Field{
		// Real value: true until the repo's git storage has a resolvable
		// HEAD commit (matches GitHub's "repository is empty" semantics).
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			nameWithOwner, _ := r["nameWithOwner"].(string)
			owner, name, ok := strings.Cut(nameWithOwner, "/")
			if !ok {
				return true, nil
			}
			return s.repoHasNoCommits(owner, name), nil
		},
	})
	repoType.AddFieldConfig("archivedAt", &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			return r["archivedAt"], nil
		},
	})

	pageInfoType := gqlPageInfoType()

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

	// Enums that real GitHub exposes — gh CLI sends these by name (CREATED_AT, DESC,
	// PUBLIC, OWNER, ...) not as strings. The schema must declare them so gh's
	// `gh repo list`, `gh issue list`, etc. type-check.
	repositoryPrivacyEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "RepositoryPrivacy",
		Values: graphql.EnumValueConfigMap{
			"PUBLIC":   &graphql.EnumValueConfig{Value: "PUBLIC"},
			"PRIVATE":  &graphql.EnumValueConfig{Value: "PRIVATE"},
			"INTERNAL": &graphql.EnumValueConfig{Value: "INTERNAL"},
		},
	})
	repositoryAffiliationEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "RepositoryAffiliation",
		Values: graphql.EnumValueConfigMap{
			"OWNER":               &graphql.EnumValueConfig{Value: "OWNER"},
			"COLLABORATOR":        &graphql.EnumValueConfig{Value: "COLLABORATOR"},
			"ORGANIZATION_MEMBER": &graphql.EnumValueConfig{Value: "ORGANIZATION_MEMBER"},
		},
	})
	repositoryOrderFieldEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "RepositoryOrderField",
		Values: graphql.EnumValueConfigMap{
			"CREATED_AT": &graphql.EnumValueConfig{Value: "CREATED_AT"},
			"UPDATED_AT": &graphql.EnumValueConfig{Value: "UPDATED_AT"},
			"PUSHED_AT":  &graphql.EnumValueConfig{Value: "PUSHED_AT"},
			"STARGAZERS": &graphql.EnumValueConfig{Value: "STARGAZERS"},
			"NAME":       &graphql.EnumValueConfig{Value: "NAME"},
		},
	})
	orderDirectionEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "OrderDirection",
		Values: graphql.EnumValueConfigMap{
			"ASC":  &graphql.EnumValueConfig{Value: "ASC"},
			"DESC": &graphql.EnumValueConfig{Value: "DESC"},
		},
	})

	// --- Releases (gh release list / view / download / delete) ---
	// `gh release list` queries releases(first:$perPage, orderBy:{field:
	// CREATED_AT, direction:$direction}, after:$endCursor) with $direction
	// typed OrderDirection — the enum above must keep that exact name.
	// `gh release view/download/delete` additionally resolve draft releases
	// via release(tagName:){databaseId,isDraft}. Both are backed by the real
	// release store. The immutable field is derived from the repository and
	// organization immutable-release settings that the REST surface persists.
	releaseType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Release",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					rel, ok := p.Source.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
					}
					return rel["nodeID"], nil
				},
			},
			"databaseId":   &graphql.Field{Type: graphql.Int},
			"name":         &graphql.Field{Type: graphql.String},
			"tagName":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"isDraft":      &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"immutable":    &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isLatest":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"isPrerelease": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"createdAt":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"publishedAt":  &graphql.Field{Type: graphql.String},
			"url":          &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description":  &graphql.Field{Type: graphql.String},
		},
	})

	releasePageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReleasePageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
		},
	})

	releaseConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ReleaseConnection",
		Fields: graphql.Fields{
			"nodes":      &graphql.Field{Type: graphql.NewList(releaseType)},
			"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"pageInfo":   &graphql.Field{Type: graphql.NewNonNull(releasePageInfoType)},
		},
	})

	releaseOrderFieldEnum := graphql.NewEnum(graphql.EnumConfig{
		Name: "ReleaseOrderField",
		Values: graphql.EnumValueConfigMap{
			"CREATED_AT": &graphql.EnumValueConfig{Value: "CREATED_AT"},
			"NAME":       &graphql.EnumValueConfig{Value: "NAME"},
		},
	})

	repoType.AddFieldConfig("releases", &graphql.Field{
		Type: releaseConnectionType,
		Args: graphql.FieldConfigArgument{
			"first": &graphql.ArgumentConfig{Type: graphql.Int},
			"after": &graphql.ArgumentConfig{Type: graphql.String},
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "ReleaseOrder",
				Fields: graphql.InputObjectConfigFieldMap{
					"field":     &graphql.InputObjectFieldConfig{Type: releaseOrderFieldEnum},
					"direction": &graphql.InputObjectFieldConfig{Type: orderDirectionEnum},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, _ := repo["databaseId"].(int)
			repoFullName, _ := repo["nameWithOwner"].(string)

			releases := s.store.Releases.List(repoID)

			orderField, direction := "CREATED_AT", "DESC"
			if orderBy, ok := p.Args["orderBy"].(map[string]interface{}); ok {
				if f, ok := orderBy["field"].(string); ok && f != "" {
					orderField = f
				}
				if d, ok := orderBy["direction"].(string); ok && d != "" {
					direction = d
				}
			}
			sort.SliceStable(releases, func(a, b int) bool {
				var less bool
				if orderField == "NAME" {
					less = releases[a].Name < releases[b].Name
				} else {
					less = releases[a].CreatedAt.Before(releases[b].CreatedAt)
				}
				if direction == "DESC" {
					return !less
				}
				return less
			})

			latestID := 0
			if latest := s.store.Releases.Latest(repoID); latest != nil {
				latestID = latest.ID
			}
			immutable := s.repoImmutableReleasesEnabled(repoID)

			first := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				first = f
			}
			after, _ := p.Args["after"].(string)

			return paginateGQL(releases, first, after, func(rel *Release) map[string]interface{} {
				return releaseToGQL(rel, latestID, repoFullName, immutable)
			}), nil
		},
	})

	repoType.AddFieldConfig("release", &graphql.Field{
		Type: releaseType,
		Args: graphql.FieldConfigArgument{
			"tagName": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, _ := repo["databaseId"].(int)
			repoFullName, _ := repo["nameWithOwner"].(string)
			tagName, _ := p.Args["tagName"].(string)

			rel := s.store.Releases.GetByTag(repoID, tagName)
			if rel == nil {
				// Real GitHub resolves a missing release(tagName:) to plain
				// null — gh's draft-release lookup keys on the null, not on
				// a NOT_FOUND error.
				return nil, nil
			}
			latestID := 0
			if latest := s.store.Releases.Latest(repoID); latest != nil {
				latestID = latest.ID
			}
			return releaseToGQL(rel, latestID, repoFullName, s.repoImmutableReleasesEnabled(repoID)), nil
		},
	})

	repoType.AddFieldConfig("latestRelease", &graphql.Field{
		// gh repo view --json latestRelease selects {publishedAt,tagName,name,url}.
		Type: releaseType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
			repoID, _ := repo["databaseId"].(int)
			repoFullName, _ := repo["nameWithOwner"].(string)
			latest := s.store.Releases.Latest(repoID)
			if latest == nil {
				return nil, nil
			}
			return releaseToGQL(latest, latest.ID, repoFullName, s.repoImmutableReleasesEnabled(repoID)), nil
		},
	})

	// Add repositories field to User type
	userType.AddFieldConfig("repositories", &graphql.Field{
		Type: repoConnectionType,
		Args: graphql.FieldConfigArgument{
			"first":             &graphql.ArgumentConfig{Type: graphql.Int},
			"after":             &graphql.ArgumentConfig{Type: graphql.String},
			"privacy":           &graphql.ArgumentConfig{Type: repositoryPrivacyEnum},
			"isFork":            &graphql.ArgumentConfig{Type: graphql.Boolean},
			"ownerAffiliations": &graphql.ArgumentConfig{Type: graphql.NewList(repositoryAffiliationEnum)},
			"orderBy": &graphql.ArgumentConfig{Type: graphql.NewInputObject(graphql.InputObjectConfig{
				Name: "RepositoryOrder",
				Fields: graphql.InputObjectConfigFieldMap{
					"field":     &graphql.InputObjectFieldConfig{Type: repositoryOrderFieldEnum},
					"direction": &graphql.InputObjectFieldConfig{Type: orderDirectionEnum},
				},
			})},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			u, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("resolve source: unexpected type %T", p.Source)
			}
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

			return paginateRepos(s.store, repos, first, after), nil
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
			// A private repo the viewer can't read must look identical to a
			// missing one: real GitHub returns null data + a NOT_FOUND error
			// rather than leaking the repo's existence or contents. This
			// mirrors the REST handler's canReadRepo gate.
			if repo == nil || (repo.Private && !canReadRepo(s.store, ghUserFromContext(p.Context), repo)) {
				// Real GitHub pairs the null with a typed NOT_FOUND error —
				// gh CLI keys on errors[].type to report "repository not
				// found" instead of decoding an empty object.
				return nil, &ghNotFoundError{
					message: fmt.Sprintf("Could not resolve to a Repository with the name '%s/%s'.", owner, name),
				}
			}
			return repoToGraphQL(s.store, s.store.snapRepo(repo)), nil
		},
	})

	// `repositoryOwner(login)` is the interface real GitHub exposes for "user or
	// organization that owns repos". gh CLI's `gh repo list <login>` queries it.
	// Bleephub's GraphQL schema does not yet model a RepositoryOwner union,
	// so repositoryOwner returns the existing User-shaped type for both users
	// and organizations. Organizations are converted with orgToGraphQL so all
	// shared fields (login, name, email, url, avatarUrl, createdAt, updatedAt)
	// carry real org data instead of a synthetic partial object.
	queryType.AddFieldConfig("repositoryOwner", &graphql.Field{
		Type: userType,
		Args: graphql.FieldConfigArgument{
			"login": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			login, _ := p.Args["login"].(string)
			if u := s.store.LookupUserByLogin(login); u != nil {
				return userToGraphQL(u), nil
			}
			s.store.mu.RLock()
			org := s.store.OrgsByLogin[login]
			s.store.mu.RUnlock()
			if org != nil {
				return orgToGraphQL(org), nil
			}
			return nil, nil
		},
	})

	// Build mutation type
	createRepoInputType := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateRepositoryInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name":             &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"ownerId":          &graphql.InputObjectFieldConfig{Type: graphql.ID},
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
					ownerLogin := user.Login
					var repo *Repo
					if ownerID, _ := input["ownerId"].(string); ownerID != "" && ownerID != user.NodeID {
						var owner *Org
						s.store.mu.RLock()
						for _, candidate := range s.store.Orgs {
							if candidate.NodeID == ownerID {
								owner = candidate
								break
							}
						}
						s.store.mu.RUnlock()
						if owner == nil || !isActiveOrgMember(s.store, user, owner.Login) {
							return nil, fmt.Errorf("repository creation for another owner is not authorized")
						}
						ownerLogin = owner.Login
						repo = s.store.CreateOrgRepo(owner, user, name, description, private)
					} else {
						repo = s.store.CreateRepo(user, name, description, private)
					}
					if repo == nil {
						return nil, fmt.Errorf("repository creation failed")
					}
					if !s.store.UpdateRepo(ownerLogin, name, func(r *Repo) {
						if v, ok := graphQLInputBool(input, "hasIssuesEnabled"); ok {
							r.HasIssues = v
						}
						if v, ok := graphQLInputBool(input, "hasWikiEnabled"); ok {
							r.HasWiki = v
						}
					}) {
						return nil, fmt.Errorf("repository %s/%s not found after creation", ownerLogin, name)
					}
					repo = s.store.GetRepo(ownerLogin, name)
					if repo == nil {
						return nil, fmt.Errorf("repository %s/%s not found after update", ownerLogin, name)
					}

					return map[string]interface{}{
						"repository": repoToGraphQL(s.store, s.store.snapRepo(repo)),
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

					if _, err := s.store.DeleteRepo(found.Owner.Login, found.Name); err != nil {
						return nil, err
					}

					return map[string]interface{}{
						"clientMutationId": nil,
					}, nil
				},
			},
		},
	})

	return repoType, mutationType
}

// repoToGraphQL converts a Repo to a map for GraphQL resolvers. It reads the
// repo's mutable fields (description, topics, timestamps) directly, so the
// caller must pass either a private snapshot (st.snapRepo) or a repo it holds
// the store lock over — never the live shared pointer off-lock, which would
// race a concurrent UpdateRepo. Under-lock callers (the *Locked GraphQL paths)
// pass the live pointer; off-lock resolvers pass a snapshot.
func repoToGraphQL(st *Store, repo *Repo) map[string]interface{} {
	return repoToGraphQLWithOrg(repo, st.GetOrgByID)
}

// repoToGraphQLLocked is repoToGraphQL for callers that already hold st.mu.
func repoToGraphQLLocked(st *Store, repo *Repo) map[string]interface{} {
	return repoToGraphQLWithOrg(repo, func(id int) *Org { return st.Orgs[id] })
}

func repoToGraphQLWithOrg(repo *Repo, getOrg func(int) *Org) map[string]interface{} {
	var ownerMap map[string]interface{}
	if repo.Owner != nil {
		ownerMap = userToGraphQL(repo.Owner)
	} else if repo.OwnerType == "Organization" {
		if org := getOrg(repo.OwnerID); org != nil {
			ownerMap = orgToGraphQL(org)
		}
	}
	webURL := "/" + repo.FullName
	if externalURL := strings.TrimRight(os.Getenv("BLEEPHUB_EXTERNAL_URL"), "/"); externalURL != "" {
		webURL = externalURL + webURL
	}

	return map[string]interface{}{
		"nodeID":              repo.NodeID,
		"databaseId":          repo.ID,
		"name":                repo.Name,
		"nameWithOwner":       repo.FullName,
		"description":         repo.Description,
		"url":                 webURL,
		"sshUrl":              sshGitURL(repo.FullName),
		"isPrivate":           repo.Private,
		"isFork":              repo.Fork,
		"isArchived":          repo.Archived,
		"visibility":          strings.ToUpper(repo.Visibility),
		"defaultBranch":       repo.DefaultBranch,
		"stargazerCount":      repo.StargazersCount,
		"language":            repo.Language,
		"licenseKey":          repo.LicenseKey,
		"licenseName":         repo.LicenseName,
		"licenseSPDX":         repo.LicenseSPDX,
		"homepage":            repo.Homepage,
		"topics":              repo.Topics,
		"hasIssues":           repo.HasIssues,
		"hasProjects":         repo.HasProjects,
		"hasWiki":             repo.HasWiki,
		"hasDiscussions":      repoHasDiscussions(repo),
		"parentID":            repo.ParentID,
		"templateRepoID":      repo.TemplateRepoID,
		"allowSquashMerge":    repo.AllowSquashMerge,
		"allowMergeCommit":    repo.AllowMergeCommit,
		"allowRebaseMerge":    repo.AllowRebaseMerge,
		"deleteBranchOnMerge": repo.DeleteBranchOnMerge,
		"isTemplate":          repo.IsTemplate,
		"owner":               ownerMap,
		"createdAt":           repo.CreatedAt.Format(time.RFC3339),
		"updatedAt":           repo.UpdatedAt.Format(time.RFC3339),
		"pushedAt":            nullableTimestamp(repo.PushedAt),
		"archivedAt":          nullableTimePtr(repo.ArchivedAt),
	}
}

func graphQLInputBool(input map[string]interface{}, key string) (bool, bool) {
	v, ok := input[key]
	if !ok || v == nil {
		return false, false
	}
	switch b := v.(type) {
	case bool:
		return b, true
	case *bool:
		if b == nil {
			return false, false
		}
		return *b, true
	default:
		panic(fmt.Sprintf("GraphQL input %s decoded as %T, want bool", key, v))
	}
}

// repoOwnerGraphQLLocked returns a User-shaped map for the owner of repo.
// For org-owned repositories it resolves the organization from the repo's
// full name and converts it with the same field names userToGraphQL uses, so
// callers that embed the owner under REST/GraphQL repo shapes get a
// consistent User-type object. Callers already hold st.mu; it never acquires
// the lock itself.
func repoOwnerGraphQLLocked(repo *Repo, st *Store) map[string]interface{} {
	if repo == nil {
		return nil
	}
	ownerLogin, _, _ := strings.Cut(repo.FullName, "/")
	org := st.OrgsByLogin[ownerLogin]
	if org != nil {
		return map[string]interface{}{
			"nodeID":     org.NodeID,
			"databaseId": org.ID,
			"login":      org.Login,
			"name":       org.Name,
			"email":      org.Email,
			"avatarUrl":  org.AvatarURL,
			"bio":        org.Description,
			"url":        "/" + org.Login,
			"createdAt":  org.CreatedAt.Format(time.RFC3339),
			"updatedAt":  org.UpdatedAt.Format(time.RFC3339),
		}
	}
	if repo.Owner != nil {
		return userToGraphQL(repo.Owner)
	}
	return nil
}

// repoOwnerREST returns a simple-user-shaped map for the owner of repo,
// using snake_case keys. For org-owned repos it resolves the organization
// from the repo's full name rather than the creating user.
func repoOwnerREST(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	if repo == nil {
		return nil
	}
	ownerLogin, _, _ := strings.Cut(repo.FullName, "/")
	st.mu.RLock()
	org := st.OrgsByLogin[ownerLogin]
	st.mu.RUnlock()
	if org != nil {
		api := baseURL + "/api/v3/orgs/" + org.Login
		return map[string]interface{}{
			"login":               org.Login,
			"id":                  org.ID,
			"node_id":             org.NodeID,
			"avatar_url":          org.AvatarURL,
			"gravatar_id":         "",
			"url":                 api,
			"html_url":            baseURL + "/" + org.Login,
			"followers_url":       api + "/followers",
			"following_url":       api + "/following{/other_user}",
			"gists_url":           api + "/gists{/gist_id}",
			"starred_url":         api + "/starred{/owner}{/repo}",
			"subscriptions_url":   api + "/subscriptions",
			"organizations_url":   api + "/orgs",
			"repos_url":           api + "/repos",
			"events_url":          api + "/events{/privacy}",
			"received_events_url": api + "/received_events",
			"type":                org.Type,
			"site_admin":          false,
			"name":                org.Name,
			"email":               org.Email,
			"user_view_type":      "public",
		}
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	if repo.Owner != nil {
		return userToJSON(repo.Owner)
	}
	return nil
}

// releaseToGQL renders a stored Release as the GraphQL source map for the
// Release type. latestID is the id of the repo's latest published release
// (0 when none) so isLatest reflects the same derivation REST uses.
func releaseToGQL(rel *Release, latestID int, repoFullName string, immutable bool) map[string]interface{} {
	var publishedAt interface{}
	if rel.PublishedAt != nil {
		publishedAt = rel.PublishedAt.Format(time.RFC3339)
	}
	var name interface{}
	if rel.Name != "" {
		name = rel.Name
	}
	return map[string]interface{}{
		"nodeID":       rel.NodeID,
		"databaseId":   rel.ID,
		"name":         name,
		"tagName":      rel.TagName,
		"isDraft":      rel.Draft,
		"immutable":    immutable,
		"isLatest":     latestID != 0 && rel.ID == latestID,
		"isPrerelease": rel.Prerelease,
		"createdAt":    rel.CreatedAt.Format(time.RFC3339),
		"publishedAt":  publishedAt,
		"url":          "/" + repoFullName + "/releases/tag/" + rel.TagName,
		"description":  nilStr(rel.Body),
	}
}

func (s *Server) repoImmutableReleasesEnabled(repoID int) bool {
	repo := s.store.GetRepoByID(repoID)
	if repo == nil {
		return false
	}
	enabled, _ := s.store.RepoImmutableReleasesState(repo)
	return enabled
}

// repoHasNoCommits reports whether the repo's git storage lacks a resolvable
// HEAD commit — GitHub's "empty repository" condition.
func (s *Server) repoHasNoCommits(owner, name string) bool {
	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		return true
	}
	headRef, err := stor.Reference(plumbing.HEAD)
	if err != nil {
		return true
	}
	if headRef.Type() == plumbing.SymbolicReference {
		targetRef, err := stor.Reference(headRef.Target())
		if err != nil {
			return true
		}
		return targetRef.Hash().IsZero()
	}
	return headRef.Hash().IsZero()
}

// paginateRepos implements Relay-style cursor pagination. Each page element is
// rendered from a private snapshot so repoToGraphQL never reads a shared repo
// pointer off the store lock.
func paginateRepos(st *Store, repos []*Repo, first int, after string) map[string]interface{} {
	return paginateGQL(repos, first, after, func(r *Repo) map[string]interface{} {
		return repoToGraphQL(st, st.snapRepo(r))
	})
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
