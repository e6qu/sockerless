package bleephub

import (
	"encoding/base64"
	"fmt"
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
					r := p.Source.(map[string]interface{})
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

	// --- Repository fields gh CLI selects (clone/create/view --json) ---
	// gh's `GitHubRepo` query (repo clone, pr create) selects hasWikiEnabled
	// and parent{...repo}; `gh repo view --json` exposes the wider static set
	// below. Fields bleephub has no feature for resolve to the honest
	// empty/false/null value real GitHub returns for a repo without that
	// feature — they are not faked.

	repoType.AddFieldConfig("hasWikiEnabled", &graphql.Field{
		// No wiki feature: real value is false.
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return false, nil
		},
	})
	repoType.AddFieldConfig("parent", &graphql.Field{
		// No forks feature: a non-fork repo's parent is null.
		Type: repoType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, nil
		},
	})
	repoType.AddFieldConfig("templateRepository", &graphql.Field{
		// No template-repo feature: null (gh selects {id,name,owner{id,login}}).
		Type: repoType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, nil
		},
	})
	repoType.AddFieldConfig("homepageUrl", &graphql.Field{
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, nil
		},
	})
	repoType.AddFieldConfig("hasProjectsEnabled", &graphql.Field{
		// Classic (v1) projects are not modeled; ProjectsV2 is separate.
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return false, nil
		},
	})
	repoType.AddFieldConfig("hasDiscussionsEnabled", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return false, nil
		},
	})
	repoType.AddFieldConfig("forkCount", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Int),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return 0, nil
		},
	})
	repoType.AddFieldConfig("watchers", &graphql.Field{
		// gh selects watchers{totalCount}; no watch/subscribe feature → 0.
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: "RepoWatcherConnection",
			Fields: graphql.Fields{
				"totalCount": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			},
		}),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return map[string]interface{}{"totalCount": 0}, nil
		},
	})
	repoType.AddFieldConfig("licenseInfo", &graphql.Field{
		// gh selects licenseInfo{key,name,nickname}; no license detection → null.
		Type: graphql.NewObject(graphql.ObjectConfig{
			Name: "RepositoryLicense",
			Fields: graphql.Fields{
				"key":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
				"name":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
				"nickname": &graphql.Field{Type: graphql.String},
			},
		}),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, nil
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
			r := p.Source.(map[string]interface{})
			lang, _ := r["language"].(string)
			if lang == "" {
				return nil, nil
			}
			return map[string]interface{}{"name": lang}, nil
		},
	})
	repoType.AddFieldConfig("languages", &graphql.Field{
		// gh selects languages(first:100){edges{size,node{name}}}; bleephub
		// performs no byte-size language analysis → empty connection.
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
			return map[string]interface{}{"edges": []interface{}{}, "totalCount": 0}, nil
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
			r := p.Source.(map[string]interface{})
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
			return false, nil
		},
	})
	repoType.AddFieldConfig("isTemplate", &graphql.Field{
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return false, nil
		},
	})
	repoType.AddFieldConfig("isEmpty", &graphql.Field{
		// Real value: true until the repo's git storage has a resolvable
		// HEAD commit (matches GitHub's "repository is empty" semantics).
		Type: graphql.NewNonNull(graphql.Boolean),
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			r := p.Source.(map[string]interface{})
			nameWithOwner, _ := r["nameWithOwner"].(string)
			owner, name, ok := strings.Cut(nameWithOwner, "/")
			if !ok {
				return true, nil
			}
			return s.repoHasNoCommits(owner, name), nil
		},
	})
	repoType.AddFieldConfig("archivedAt", &graphql.Field{
		// Archive timestamps aren't recorded; null matches an unarchived repo.
		Type: graphql.String,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return nil, nil
		},
	})

	pageInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PageInfo",
		Fields: graphql.Fields{
			"hasNextPage":     &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"hasPreviousPage": &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
			"startCursor":     &graphql.Field{Type: graphql.String},
			"endCursor":       &graphql.Field{Type: graphql.String},
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
	// release store. Release deliberately does NOT declare `immutable`: gh
	// introspects Release's fields and cleanly falls back to the
	// pre-immutable-releases query when the field is absent.
	releaseType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Release",
		Fields: graphql.Fields{
			"id": &graphql.Field{
				Type: graphql.NewNonNull(graphql.ID),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					rel := p.Source.(map[string]interface{})
					return rel["nodeID"], nil
				},
			},
			"databaseId":   &graphql.Field{Type: graphql.Int},
			"name":         &graphql.Field{Type: graphql.String},
			"tagName":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"isDraft":      &graphql.Field{Type: graphql.NewNonNull(graphql.Boolean)},
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
			repo := p.Source.(map[string]interface{})
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

			first := 30
			if f, ok := p.Args["first"].(int); ok && f > 0 {
				first = f
			}
			after, _ := p.Args["after"].(string)

			return paginateGQL(releases, first, after, func(rel *Release) map[string]interface{} {
				return releaseToGQL(rel, latestID, repoFullName)
			}), nil
		},
	})

	repoType.AddFieldConfig("release", &graphql.Field{
		Type: releaseType,
		Args: graphql.FieldConfigArgument{
			"tagName": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
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
			return releaseToGQL(rel, latestID, repoFullName), nil
		},
	})

	repoType.AddFieldConfig("latestRelease", &graphql.Field{
		// gh repo view --json latestRelease selects {publishedAt,tagName,name,url}.
		Type: releaseType,
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			repo := p.Source.(map[string]interface{})
			repoID, _ := repo["databaseId"].(int)
			repoFullName, _ := repo["nameWithOwner"].(string)
			latest := s.store.Releases.Latest(repoID)
			if latest == nil {
				return nil, nil
			}
			return releaseToGQL(latest, latest.ID, repoFullName), nil
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
				// Real GitHub pairs the null with a typed NOT_FOUND error —
				// gh CLI keys on errors[].type to report "repository not
				// found" instead of decoding an empty object.
				return nil, &ghNotFoundError{
					message: fmt.Sprintf("Could not resolve to a Repository with the name '%s/%s'.", owner, name),
				}
			}
			return repoToGraphQL(repo), nil
		},
	})

	// `repositoryOwner(login)` is the interface real GitHub exposes for "user or
	// organization that owns repos". gh CLI's `gh repo list <login>` queries it.
	// Bleephub treats user and org the same — both have a .repositories field on
	// userType already — so we return the User shape regardless of whether the
	// login resolves to a User or an Org (orgs.login is also indexed in
	// OrgsByLogin; treat their JSON shape as the same as a User for this purpose).
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
			// Org → fake a User-shaped payload so userType.repositories resolves.
			s.store.mu.RLock()
			org := s.store.OrgsByLogin[login]
			s.store.mu.RUnlock()
			if org != nil {
				return map[string]interface{}{
					"login":      org.Login,
					"databaseId": org.ID,
					"name":       org.Name,
					"url":        "/" + org.Login,
				}, nil
			}
			return nil, nil
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

	return repoType, mutationType
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
		"language":       repo.Language,
		"topics":         repo.Topics,
		"owner":          ownerMap,
		"createdAt":      repo.CreatedAt.Format(time.RFC3339),
		"updatedAt":      repo.UpdatedAt.Format(time.RFC3339),
		"pushedAt":       repo.PushedAt.Format(time.RFC3339),
	}
}

// releaseToGQL renders a stored Release as the GraphQL source map for the
// Release type. latestID is the id of the repo's latest published release
// (0 when none) so isLatest reflects the same derivation REST uses.
func releaseToGQL(rel *Release, latestID int, repoFullName string) map[string]interface{} {
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
		"isLatest":     latestID != 0 && rel.ID == latestID,
		"isPrerelease": rel.Prerelease,
		"createdAt":    rel.CreatedAt.Format(time.RFC3339),
		"publishedAt":  publishedAt,
		"url":          "/" + repoFullName + "/releases/tag/" + rel.TagName,
		"description":  nilStr(rel.Body),
	}
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

// paginateRepos implements Relay-style cursor pagination.
func paginateRepos(repos []*Repo, first int, after string) map[string]interface{} {
	return paginateGQL(repos, first, after, repoToGraphQL)
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
