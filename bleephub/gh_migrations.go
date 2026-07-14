package bleephub

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHMigrationsRoutes() {
	// User migrations
	s.route("POST /api/v3/user/migrations", s.handleStartUserMigration)
	s.route("GET /api/v3/user/migrations", s.handleListUserMigrations)
	s.route("GET /api/v3/user/migrations/{migration_id}", s.handleGetUserMigration)
	s.route("GET /api/v3/user/migrations/{migration_id}/archive", s.handleDownloadUserMigrationArchive)
	s.route("DELETE /api/v3/user/migrations/{migration_id}/archive", s.handleDeleteUserMigrationArchive)
	s.route("GET /api/v3/user/migrations/{migration_id}/repositories", s.handleListUserMigrationRepositories)
	s.route("DELETE /api/v3/user/migrations/{migration_id}/repos/{repo_name}/lock", s.handleUnlockUserMigrationRepo)

	// Organization migrations
	s.route("POST /api/v3/orgs/{org}/migrations", s.handleStartOrgMigration)
	s.route("GET /api/v3/orgs/{org}/migrations", s.handleListOrgMigrations)
	s.route("GET /api/v3/orgs/{org}/migrations/{migration_id}", s.handleGetOrgMigration)
	s.route("GET /api/v3/orgs/{org}/migrations/{migration_id}/repositories", s.handleListOrgMigrationRepositories)
	s.route("GET /api/v3/orgs/{org}/migrations/{migration_id}/archive", s.handleDownloadOrgMigrationArchive)
	s.route("DELETE /api/v3/orgs/{org}/migrations/{migration_id}/archive", s.handleDeleteOrgMigrationArchive)
	s.route("GET /api/v3/orgs/{org}/migrations/{migration_id}/repos/{repo_name}/lock", s.handleGetOrgMigrationLock)
	s.route("DELETE /api/v3/orgs/{org}/migrations/{migration_id}/repos/{repo_name}/lock", s.handleUnlockOrgMigrationRepo)
}

type migrationCreateBody struct {
	Repositories         []string `json:"repositories"`
	LockRepositories     bool     `json:"lock_repositories"`
	ExcludeMetadata      bool     `json:"exclude_metadata"`
	ExcludeGitData       bool     `json:"exclude_git_data"`
	ExcludeAttachments   bool     `json:"exclude_attachments"`
	ExcludeReleases      bool     `json:"exclude_releases"`
	ExcludeOwnerProjects bool     `json:"exclude_owner_projects"`
	OrgMetadataOnly      bool     `json:"org_metadata_only"`
	Exclude              []string `json:"exclude"`
}

func decodeMigrationCreateBody(w http.ResponseWriter, r *http.Request) (*migrationCreateBody, bool) {
	var body migrationCreateBody
	if !decodeJSONBody(w, r, &body) {
		return nil, false
	}
	return &body, true
}

func parseExcludeQuery(r *http.Request) []string {
	return r.URL.Query()["exclude"]
}

func shouldExcludeRepos(exclude []string) bool {
	for _, e := range exclude {
		if strings.EqualFold(e, "repositories") {
			return true
		}
	}
	return false
}

// User migrations handlers

func (s *Server) handleStartUserMigration(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	body, ok := decodeMigrationCreateBody(w, r)
	if !ok {
		return
	}
	repos, err := s.validateUserMigrationRepos(user, body.Repositories)
	if err != "" {
		writeGHValidationError(w, "Migration", "repositories", err)
		return
	}
	m := s.store.CreateUserMigration(user.ID, repos, body.LockRepositories, body.ExcludeMetadata, body.ExcludeGitData, body.ExcludeAttachments, body.ExcludeReleases, body.ExcludeOwnerProjects, body.OrgMetadataOnly)
	s.recordAuditEvent("user_migration.start", user.Login, "", map[string]interface{}{"migration_id": m.ID, "repositories": repos})
	writeJSON(w, http.StatusCreated, s.userMigrationToJSON(m, s.baseURL(r), false))
}

func (s *Server) handleListUserMigrations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	exclude := parseExcludeQuery(r)
	excludeRepos := shouldExcludeRepos(exclude)
	migs := s.store.ListUserMigrations(user.ID)
	out := make([]map[string]interface{}, len(migs))
	for i, m := range migs {
		out[i] = s.userMigrationToJSON(m, s.baseURL(r), excludeRepos)
		if len(exclude) > 0 {
			out[i]["exclude"] = exclude
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetUserMigration(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, ok := s.resolveUserMigration(w, r, user.ID)
	if !ok {
		return
	}
	exclude := parseExcludeQuery(r)
	out := s.userMigrationToJSON(m, s.baseURL(r), shouldExcludeRepos(exclude))
	if len(exclude) > 0 {
		out["exclude"] = exclude
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListUserMigrationRepositories lists the repositories captured by
// a user migration in GitHub's minimal-repository shape. Repositories
// deleted since the export are no longer listable.
func (s *Server) handleListUserMigrationRepositories(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, ok := s.resolveUserMigration(w, r, user.ID)
	if !ok {
		return
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(m.Repositories))
	for _, fullName := range m.Repositories {
		if repo := s.store.GetRepoByFullName(fullName); repo != nil {
			out = append(out, minimalRepoJSON(repo, s.store, base))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleDownloadUserMigrationArchive(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, ok := s.resolveUserMigration(w, r, user.ID)
	if !ok {
		return
	}
	s.serveMigrationArchive(w, r, m.MigrationCommon, "user", "")
}

func (s *Server) handleDeleteUserMigrationArchive(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, ok := s.resolveUserMigration(w, r, user.ID)
	if !ok {
		return
	}
	if !s.store.DeleteUserMigrationArchive(m.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("user_migration.archive_deleted", user.Login, "", map[string]interface{}{"migration_id": m.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnlockUserMigrationRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, ok := s.resolveUserMigration(w, r, user.ID)
	if !ok {
		return
	}
	repoName := r.PathValue("repo_name")
	if !s.store.UnlockUserMigrationRepo(m.ID, repoName) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Organization migration handlers

func (s *Server) handleStartOrgMigration(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	body, ok := decodeMigrationCreateBody(w, r)
	if !ok {
		return
	}
	repos, err := s.validateOrgMigrationRepos(org, body.Repositories)
	if err != "" {
		writeGHValidationError(w, "Migration", "repositories", err)
		return
	}
	m := s.store.CreateOrgMigration(org.Login, repos, body.LockRepositories, body.ExcludeMetadata, body.ExcludeGitData, body.ExcludeAttachments, body.ExcludeReleases, body.ExcludeOwnerProjects, body.OrgMetadataOnly)
	s.recordAuditEvent("org_migration.start", user.Login, "", map[string]interface{}{"org": org.Login, "migration_id": m.ID, "repositories": repos})
	writeJSON(w, http.StatusCreated, s.orgMigrationToJSON(m, s.baseURL(r), false))
}

func (s *Server) handleListOrgMigrations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	exclude := parseExcludeQuery(r)
	excludeRepos := shouldExcludeRepos(exclude)
	migs := s.store.ListOrgMigrations(org.Login)
	out := make([]map[string]interface{}, len(migs))
	for i, m := range migs {
		out[i] = s.orgMigrationToJSON(m, s.baseURL(r), excludeRepos)
		if len(exclude) > 0 {
			out[i]["exclude"] = exclude
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetOrgMigration(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, _, ok := s.resolveOrgMigration(w, r, user)
	if !ok {
		return
	}
	exclude := parseExcludeQuery(r)
	out := s.orgMigrationToJSON(m, s.baseURL(r), shouldExcludeRepos(exclude))
	if len(exclude) > 0 {
		out["exclude"] = exclude
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListOrgMigrationRepositories implements
// GET /orgs/{org}/migrations/{migration_id}/repositories: the repositories
// locked into the migration, in the minimal-repository shape.
func (s *Server) handleListOrgMigrationRepositories(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, _, ok := s.resolveOrgMigration(w, r, user)
	if !ok {
		return
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(m.Repositories))
	for _, fullName := range m.Repositories {
		if repo := s.store.GetRepoByFullName(fullName); repo != nil {
			out = append(out, migrationRepoJSON(repo, s.store, base))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleDownloadOrgMigrationArchive(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, _, ok := s.resolveOrgMigration(w, r, user)
	if !ok {
		return
	}
	s.serveMigrationArchive(w, r, m.MigrationCommon, "orgs", m.OrgLogin)
}

func (s *Server) handleDeleteOrgMigrationArchive(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, org, ok := s.resolveOrgMigration(w, r, user)
	if !ok {
		return
	}
	if !s.store.DeleteOrgMigrationArchive(m.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("org_migration.archive_deleted", user.Login, "", map[string]interface{}{"org": org.Login, "migration_id": m.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOrgMigrationLock(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, _, ok := s.resolveOrgMigration(w, r, user)
	if !ok {
		return
	}
	repoName := r.PathValue("repo_name")
	if !s.store.IsOrgMigrationRepoLocked(m.ID, repoName) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"locked": true})
}

func (s *Server) handleUnlockOrgMigrationRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	m, _, ok := s.resolveOrgMigration(w, r, user)
	if !ok {
		return
	}
	repoName := r.PathValue("repo_name")
	if !s.store.UnlockOrgMigrationRepo(m.ID, repoName) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Resolvers

func (s *Server) resolveUserMigration(w http.ResponseWriter, r *http.Request, userID int) (*UserMigration, bool) {
	id, err := strconv.Atoi(r.PathValue("migration_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	m := s.store.GetUserMigration(id)
	if m == nil || m.UserID != userID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return m, true
}

func (s *Server) resolveOrgMigration(w http.ResponseWriter, r *http.Request, user *User) (*OrgMigration, *Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	id, err := strconv.Atoi(r.PathValue("migration_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	m := s.store.GetOrgMigration(id)
	if m == nil || m.OrgLogin != org.Login {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, false
	}
	return m, org, true
}

// Validation

func (s *Server) validateUserMigrationRepos(user *User, names []string) ([]string, string) {
	if len(names) == 0 {
		return nil, "missing_field"
	}
	for _, name := range names {
		repo := s.store.ReposByName[name]
		if repo == nil {
			return nil, "invalid"
		}
		if repo.OwnerType == "Organization" || repo.Owner == nil || repo.Owner.ID != user.ID {
			return nil, "invalid"
		}
	}
	return names, ""
}

func (s *Server) validateOrgMigrationRepos(org *Org, names []string) ([]string, string) {
	if len(names) == 0 {
		return nil, "missing_field"
	}
	prefix := org.Login + "/"
	for _, name := range names {
		repo := s.store.ReposByName[name]
		if repo == nil {
			return nil, "invalid"
		}
		if !strings.HasPrefix(repo.FullName, prefix) {
			return nil, "invalid"
		}
	}
	return names, ""
}

// JSON serialization

func (s *Server) userMigrationToJSON(m *UserMigration, baseURL string, excludeRepos bool) map[string]interface{} {
	owner := map[string]interface{}{}
	if u := s.store.GetUserByID(m.UserID); u != nil {
		owner = userToJSON(u)
	}
	return s.migrationToJSON(m.MigrationCommon, owner, baseURL, "user", "", excludeRepos)
}

func (s *Server) orgMigrationToJSON(m *OrgMigration, baseURL string, excludeRepos bool) map[string]interface{} {
	owner := map[string]interface{}{}
	if org := s.store.GetOrg(m.OrgLogin); org != nil {
		owner = orgAsSimpleUserJSON(org)
	}
	return s.migrationToJSON(m.MigrationCommon, owner, baseURL, "orgs", m.OrgLogin, excludeRepos)
}

func (s *Server) migrationToJSON(m MigrationCommon, owner map[string]interface{}, baseURL, scope, scopeLogin string, excludeRepos bool) map[string]interface{} {
	var url, htmlURL, archiveURL string
	if scope == "user" {
		url = baseURL + "/api/v3/user/migrations/" + strconv.Itoa(m.ID)
		htmlURL = baseURL + "/user/migrations/" + strconv.Itoa(m.ID)
	} else {
		url = baseURL + "/api/v3/orgs/" + scopeLogin + "/migrations/" + strconv.Itoa(m.ID)
		htmlURL = baseURL + "/orgs/" + scopeLogin + "/migrations/" + strconv.Itoa(m.ID)
	}
	archiveURL = url + "/archive"

	out := map[string]interface{}{
		"id":                     m.ID,
		"owner":                  owner,
		"guid":                   m.GUID,
		"state":                  m.State,
		"lock_repositories":      m.LockRepositories,
		"exclude_metadata":       m.ExcludeMetadata,
		"exclude_git_data":       m.ExcludeGitData,
		"exclude_attachments":    m.ExcludeAttachments,
		"exclude_releases":       m.ExcludeReleases,
		"exclude_owner_projects": m.ExcludeOwnerProjects,
		"org_metadata_only":      m.OrgMetadataOnly,
		"repositories":           []map[string]interface{}{},
		"url":                    url,
		"html_url":               htmlURL,
		"archive_url":            archiveURL,
		"created_at":             m.CreatedAt.Format(time.RFC3339),
		"updated_at":             m.UpdatedAt.Format(time.RFC3339),
		"exported_at":            m.ExportedAt.Format(time.RFC3339),
		"node_id":                m.NodeID,
		"exclude":                []string{},
	}
	if !excludeRepos {
		repos := make([]map[string]interface{}, 0, len(m.Repositories))
		for _, name := range m.Repositories {
			repo := s.store.ReposByName[name]
			if repo != nil {
				repos = append(repos, migrationRepoJSON(repo, s.store, baseURL))
			}
		}
		out["repositories"] = repos
	}
	return out
}

func migrationRepoJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	owner := map[string]interface{}{}
	if repo.OwnerType == "Organization" {
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) == 2 {
			if org := st.GetOrg(parts[0]); org != nil {
				owner = orgAsSimpleUserJSON(org)
			}
		}
	} else if repo.Owner != nil {
		owner = userToJSON(repo.Owner)
	}

	api := baseURL + "/api/v3/repos/" + repo.FullName
	htmlURL := baseURL + "/" + repo.FullName
	host := baseURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	openIssues := st.CountOpenIssues(repo.ID)

	return map[string]interface{}{
		"id":                repo.ID,
		"node_id":           repo.NodeID,
		"name":              repo.Name,
		"full_name":         repo.FullName,
		"owner":             owner,
		"private":           repo.Private,
		"html_url":          htmlURL,
		"description":       nilOrString(repo.Description),
		"fork":              repo.Fork,
		"url":               api,
		"archive_url":       api + "/{archive_format}{/ref}",
		"assignees_url":     api + "/assignees{/user}",
		"blobs_url":         api + "/git/blobs{/sha}",
		"branches_url":      api + "/branches{/branch}",
		"collaborators_url": api + "/collaborators{/collaborator}",
		"comments_url":      api + "/comments{/number}",
		"commits_url":       api + "/commits{/sha}",
		"compare_url":       api + "/compare/{base}...{head}",
		"contents_url":      api + "/contents/{+path}",
		"contributors_url":  api + "/contributors",
		"deployments_url":   api + "/deployments",
		"downloads_url":     api + "/downloads",
		"events_url":        api + "/events",
		"forks_url":         api + "/forks",
		"git_commits_url":   api + "/git/commits{/sha}",
		"git_refs_url":      api + "/git/refs{/sha}",
		"git_tags_url":      api + "/git/tags{/sha}",
		"issue_comment_url": api + "/issues/comments{/number}",
		"issue_events_url":  api + "/issues/events{/number}",
		"issues_url":        api + "/issues{/number}",
		"keys_url":          api + "/keys{/key_id}",
		"labels_url":        api + "/labels{/name}",
		"languages_url":     api + "/languages",
		"merges_url":        api + "/merges",
		"milestones_url":    api + "/milestones{/number}",
		"notifications_url": api + "/notifications{?since,all,participating}",
		"pulls_url":         api + "/pulls{/number}",
		"releases_url":      api + "/releases{/id}",
		"stargazers_url":    api + "/stargazers",
		"statuses_url":      api + "/statuses/{sha}",
		"subscribers_url":   api + "/subscribers",
		"subscription_url":  api + "/subscription",
		"tags_url":          api + "/tags",
		"teams_url":         api + "/teams",
		"trees_url":         api + "/git/trees{/sha}",
		"hooks_url":         api + "/hooks",
		"clone_url":         baseURL + "/" + repo.FullName + ".git",
		"git_url":           "git://" + host + "/" + repo.FullName + ".git",
		"ssh_url":           sshGitURL(repo.FullName),
		"svn_url":           baseURL + "/" + repo.FullName,
		"mirror_url":        nil,
		"homepage":          nilOrString(repo.Homepage),
		"license":           licenseJSON(repo),
		"language":          nilOrString(repo.Language),
		"default_branch":    repo.DefaultBranch,
		"visibility":        repo.Visibility,
		"archived":          repo.Archived,
		"disabled":          false,
		"forks":             0,
		"forks_count":       0,
		"stargazers_count":  repo.StargazersCount,
		"watchers":          repo.StargazersCount,
		"watchers_count":    repo.StargazersCount,
		"size":              st.RepoSize(repo.FullName),
		"open_issues":       openIssues,
		"open_issues_count": openIssues,
		"has_issues":        repo.HasIssues,
		"has_projects":      repo.HasProjects,
		"has_wiki":          repo.HasWiki,
		"has_pages":         false,
		"has_downloads":     false,
		"has_discussions":   repoHasDiscussions(repo),
		"has_pull_requests": repo.HasPullRequests,
		"is_template":       repo.IsTemplate,
		"topics":            repo.Topics,
		"permissions": map[string]bool{
			"admin": true,
			"push":  true,
			"pull":  true,
		},
		"created_at": repo.CreatedAt.Format(time.RFC3339),
		"updated_at": repo.UpdatedAt.Format(time.RFC3339),
		"pushed_at":  repo.PushedAt.Format(time.RFC3339),
	}
}

// Archive generation and download

func (s *Server) serveMigrationArchive(w http.ResponseWriter, r *http.Request, m MigrationCommon, scope, scopeLogin string) {
	if m.State != "exported" || m.ArchiveDeleted {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	data, err := s.generateMigrationArchive(m, scope, scopeLogin)
	if err != nil {
		s.logger.Error().Err(err).Int("migration_id", m.ID).Msg("failed to generate migration archive")
		writeGHError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=migration-%d.tar.gz", m.ID))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(data))
}

func (s *Server) generateMigrationArchive(m MigrationCommon, scope, scopeLogin string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	now := time.Now().UTC()

	repoData := map[string]interface{}{}
	for _, name := range m.Repositories {
		repo := s.store.ReposByName[name]
		if repo == nil {
			continue
		}
		repoData[name] = s.store.MigrationRepoExportData(repo.ID)

		readme := fmt.Sprintf("# %s\n\nOwner: %s\nDefault branch: %s\nVisibility: %s\nDescription: %s\n",
			repo.FullName,
			repo.FullName,
			repo.DefaultBranch,
			repo.Visibility,
			repo.Description,
		)
		path := fmt.Sprintf("repositories/%s/README.md", repo.FullName)
		if err := addTarFile(tw, path, []byte(readme), now); err != nil {
			return nil, err
		}
	}

	meta := map[string]interface{}{
		"id":           m.ID,
		"guid":         m.GUID,
		"state":        m.State,
		"scope":        scope,
		"scope_login":  scopeLogin,
		"exported_at":  m.ExportedAt.Format(time.RFC3339),
		"generated_at": now.Format(time.RFC3339),
		"repositories": m.Repositories,
		"repo_data":    repoData,
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := addTarFile(tw, "metadata.json", metaBytes, now); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addTarFile(tw *tar.Writer, name string, data []byte, modTime time.Time) error {
	header := &tar.Header{
		Name:     name,
		Mode:     0o644,
		Size:     int64(len(data)),
		ModTime:  modTime,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// MigrationRepoExportData gathers lightweight metadata for a migration archive.
func (st *Store) MigrationRepoExportData(repoID int) map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var issues []map[string]interface{}
	for _, issue := range st.Issues {
		if issue.RepoID == repoID {
			issues = append(issues, map[string]interface{}{
				"number": issue.Number,
				"title":  issue.Title,
				"state":  issue.State,
			})
		}
	}

	var pulls []map[string]interface{}
	for _, pr := range st.PullRequests {
		if pr.RepoID == repoID {
			pulls = append(pulls, map[string]interface{}{
				"number": pr.Number,
				"title":  pr.Title,
				"state":  pr.State,
			})
		}
	}

	releases := st.Releases.List(repoID)
	relOut := make([]map[string]interface{}, len(releases))
	for i, r := range releases {
		relOut[i] = map[string]interface{}{
			"id":           r.ID,
			"tag_name":     r.TagName,
			"name":         r.Name,
			"draft":        r.Draft,
			"prerelease":   r.Prerelease,
			"created_at":   r.CreatedAt.Format(time.RFC3339),
			"published_at": nil,
		}
		if r.PublishedAt != nil {
			relOut[i]["published_at"] = r.PublishedAt.Format(time.RFC3339)
		}
	}

	return map[string]interface{}{
		"issues":        issues,
		"pull_requests": pulls,
		"releases":      relOut,
	}
}
