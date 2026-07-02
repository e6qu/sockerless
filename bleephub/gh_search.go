package bleephub

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (s *Server) registerGHSearchRoutes() {
	s.route("GET /api/v3/search/issues", s.handleSearchIssues)
	s.route("GET /api/v3/search/repositories", s.handleSearchRepositories)
	s.route("GET /api/v3/search/code", s.handleSearchCode)
	s.route("GET /api/v3/search/users", s.handleSearchUsers)
}

// searchQuery holds the parsed pieces of a GitHub search query.
type searchQuery struct {
	Terms     []string
	Repo      string
	User      string
	Org       string
	Language  string
	Label     string
	State     string
	IsIssue   *bool
	IsPR      *bool
	IsPrivate *bool
	IsPublic  *bool
	InTitle   bool
	InBody    bool
	Sort      string
	Order     string
	PerPage   int
	Page      int
	Path      string
	Extension string
	Filename  string
	Type      string // user search type: user/org
}

func parseSearchQuery(r *http.Request) searchQuery {
	q := searchQuery{
		Terms:   []string{},
		Sort:    r.URL.Query().Get("sort"),
		Order:   r.URL.Query().Get("order"),
		PerPage: 30,
		Page:    1,
	}
	pp := parsePagination(r)
	q.PerPage = pp.PerPage
	q.Page = pp.Page

	raw := strings.TrimSpace(r.URL.Query().Get("q"))
	for len(raw) > 0 {
		var token string
		if raw[0] == '"' {
			idx := strings.Index(raw[1:], "\"")
			if idx < 0 {
				token = raw[1:]
				raw = ""
			} else {
				token = raw[1 : idx+1]
				raw = strings.TrimSpace(raw[idx+2:])
			}
		} else {
			idx := strings.IndexAny(raw, " \t")
			if idx < 0 {
				token = raw
				raw = ""
			} else {
				token = raw[:idx]
				raw = strings.TrimSpace(raw[idx:])
			}
		}
		token = strings.Trim(token, "\"")
		if token == "" {
			continue
		}
		if strings.Contains(token, ":") {
			parts := strings.SplitN(token, ":", 2)
			key, val := parts[0], parts[1]
			switch strings.ToLower(key) {
			case "repo":
				q.Repo = val
			case "user":
				q.User = val
			case "org":
				q.Org = val
			case "language":
				q.Language = val
			case "label":
				q.Label = val
			case "state":
				q.State = strings.ToLower(val)
			case "is":
				switch strings.ToLower(val) {
				case "issue":
					v := true
					q.IsIssue = &v
				case "pr", "pull-request":
					v := true
					q.IsPR = &v
				case "private":
					v := true
					q.IsPrivate = &v
				case "public":
					v := true
					q.IsPublic = &v
				case "open", "closed":
					q.State = strings.ToLower(val)
				}
			case "in":
				switch strings.ToLower(val) {
				case "title":
					q.InTitle = true
				case "body":
					q.InBody = true
				}
			case "path":
				q.Path = val
			case "extension", "ext":
				q.Extension = val
			case "filename", "file":
				q.Filename = val
			case "type":
				q.Type = strings.ToLower(val)
			}
			continue
		}
		q.Terms = append(q.Terms, strings.ToLower(token))
	}
	return q
}

func (q searchQuery) matchesText(text string) bool {
	if len(q.Terms) == 0 {
		return true
	}
	text = strings.ToLower(text)
	for _, term := range q.Terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

func (s *Server) handleSearchIssues(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	q := parseSearchQuery(r)

	s.store.mu.RLock()
	defer s.store.mu.RUnlock()

	var results []map[string]interface{}
	base := s.baseURL(r)

	for _, issue := range s.store.Issues {
		repo := s.store.Repos[issue.RepoID]
		if repo == nil {
			continue
		}
		if !canReadRepo(s.store, user, repo) {
			continue
		}
		if q.Repo != "" && !strings.EqualFold(repo.FullName, q.Repo) {
			continue
		}
		if q.User != "" && !strings.EqualFold(repo.Owner.Login, q.User) {
			continue
		}
		if q.Org != "" && repo.OwnerType != "Organization" {
			continue
		}
		if q.Org != "" {
			parts := strings.SplitN(repo.FullName, "/", 2)
			if len(parts) == 0 || !strings.EqualFold(parts[0], q.Org) {
				continue
			}
		}
		if q.Label != "" {
			if !issueHasLabelNames(s.store, issue, []string{q.Label}) {
				continue
			}
		}
		if q.State != "" && !strings.EqualFold(issue.State, q.State) {
			continue
		}
		if q.IsIssue != nil && !*q.IsIssue {
			continue
		}
		if q.IsPR != nil && *q.IsPR {
			continue
		}
		text := issue.Title + " " + issue.Body
		if q.InTitle && !q.InBody {
			text = issue.Title
		} else if q.InBody && !q.InTitle {
			text = issue.Body
		}
		if !q.matchesText(text) {
			continue
		}

		item := issueToJSON(issue, s.store, base, repo.FullName)
		item["score"] = 1.0
		item["author_association"] = authorAssociationForIssue(s.store, issue, repo)
		item["draft"] = false
		item["pull_request"] = nil
		item["repository"] = repoToJSON(repo, s.store, base)
		results = append(results, item)
	}

	for _, pr := range s.store.PullRequests {
		repo := s.store.Repos[pr.RepoID]
		if repo == nil {
			continue
		}
		if !canReadRepo(s.store, user, repo) {
			continue
		}
		if q.Repo != "" && !strings.EqualFold(repo.FullName, q.Repo) {
			continue
		}
		if q.User != "" && !strings.EqualFold(repo.Owner.Login, q.User) {
			continue
		}
		if q.Org != "" {
			parts := strings.SplitN(repo.FullName, "/", 2)
			if len(parts) == 0 || !strings.EqualFold(parts[0], q.Org) {
				continue
			}
		}
		if q.Label != "" {
			if !prHasLabelNames(s.store, pr, []string{q.Label}) {
				continue
			}
		}
		if q.State != "" {
			if q.State == "open" && pr.State != "OPEN" {
				continue
			}
			if q.State == "closed" && pr.State != "CLOSED" && pr.State != "MERGED" {
				continue
			}
		}
		if q.IsIssue != nil && *q.IsIssue {
			continue
		}
		if q.IsPR != nil && !*q.IsPR {
			continue
		}
		text := pr.Title + " " + pr.Body
		if q.InTitle && !q.InBody {
			text = pr.Title
		} else if q.InBody && !q.InTitle {
			text = pr.Body
		}
		if !q.matchesText(text) {
			continue
		}

		item := issueToJSONForPR(pr, s.store, base, repo.FullName)
		item["score"] = 1.0
		item["author_association"] = authorAssociationForPR(s.store, pr, repo)
		item["repository"] = repoToJSON(repo, s.store, base)
		results = append(results, item)
	}

	results = sortSearchResults(results, q.Sort, q.Order)
	writeJSON(w, http.StatusOK, searchEnvelope("issues", results, q.Page, q.PerPage))
}

func issueToJSONForPR(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	out := issueToJSONForPullRequest(pr, st, baseURL, repoFullName)
	out["pull_request"] = map[string]interface{}{
		"url":       baseURL + "/api/v3/repos/" + repoFullName + "/pulls/" + strconv.Itoa(pr.Number),
		"html_url":  baseURL + "/" + repoFullName + "/pull/" + strconv.Itoa(pr.Number),
		"diff_url":  baseURL + "/" + repoFullName + "/pull/" + strconv.Itoa(pr.Number) + ".diff",
		"patch_url": baseURL + "/" + repoFullName + "/pull/" + strconv.Itoa(pr.Number) + ".patch",
		"merged_at": nil,
	}
	return out
}

func issueToJSONForPullRequest(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	authorJSON := userToJSON(st.Users[pr.AuthorID])

	labels := make([]map[string]interface{}, 0)
	for _, lid := range pr.LabelIDs {
		if l := st.Labels[lid]; l != nil {
			labels = append(labels, issueLabelToJSON(l, baseURL, repoFullName))
		}
	}
	assignees := make([]map[string]interface{}, 0)
	for _, aid := range pr.AssigneeIDs {
		if u := st.Users[aid]; u != nil {
			assignees = append(assignees, userToJSON(u))
		}
	}
	var assignee interface{}
	if len(assignees) > 0 {
		assignee = assignees[0]
	}
	var milestoneJSON interface{}
	if pr.MilestoneID > 0 {
		milestoneJSON = milestoneToJSON(st.Milestones[pr.MilestoneID], st, baseURL, repoFullName)
	}
	var closedAt interface{}
	if pr.ClosedAt != nil {
		closedAt = pr.ClosedAt.Format(time.RFC3339)
	}
	var activeLockReason interface{}
	if pr.Locked {
		activeLockReason = pr.ActiveLockReason
	}
	commentCount := 0
	for _, c := range st.Comments {
		if c.ParentType == "pull_request" && c.IssueID == pr.ID {
			commentCount++
		}
	}
	numStr := strconv.Itoa(pr.Number)
	api := baseURL + "/api/v3/repos/" + repoFullName + "/issues/" + numStr
	return map[string]interface{}{
		"id":                 pr.ID,
		"node_id":            pr.NodeID,
		"url":                api,
		"html_url":           baseURL + "/" + repoFullName + "/issues/" + numStr,
		"repository_url":     baseURL + "/api/v3/repos/" + repoFullName,
		"comments_url":       api + "/comments",
		"events_url":         api + "/events",
		"labels_url":         api + "/labels{/name}",
		"number":             pr.Number,
		"title":              pr.Title,
		"body":               pr.Body,
		"state":              strings.ToLower(pr.State),
		"state_reason":       "",
		"user":               authorJSON,
		"labels":             labels,
		"assignee":           assignee,
		"assignees":          assignees,
		"milestone":          milestoneJSON,
		"locked":             pr.Locked,
		"active_lock_reason": activeLockReason,
		"comments":           commentCount,
		"created_at":         pr.CreatedAt.Format(time.RFC3339),
		"updated_at":         pr.UpdatedAt.Format(time.RFC3339),
		"closed_at":          closedAt,
		"draft":              pr.IsDraft,
	}
}

func authorAssociationForIssue(st *Store, issue *Issue, repo *Repo) string {
	author := st.Users[issue.AuthorID]
	if author == nil {
		return "NONE"
	}
	if repo.Owner != nil && repo.Owner.ID == author.ID {
		return "OWNER"
	}
	return "CONTRIBUTOR"
}

func authorAssociationForPR(st *Store, pr *PullRequest, repo *Repo) string {
	author := st.Users[pr.AuthorID]
	if author == nil {
		return "NONE"
	}
	if repo.Owner != nil && repo.Owner.ID == author.ID {
		return "OWNER"
	}
	return "CONTRIBUTOR"
}

func authorAssociationForComment(st *Store, comment *Comment, repo *Repo) string {
	author := st.Users[comment.AuthorID]
	if author == nil {
		return "NONE"
	}
	if repo.Owner != nil && repo.Owner.ID == author.ID {
		return "OWNER"
	}
	return "CONTRIBUTOR"
}

func issueHasLabelNames(st *Store, issue *Issue, names []string) bool {
	for _, name := range names {
		found := false
		for _, lid := range issue.LabelIDs {
			if l := st.Labels[lid]; l != nil && strings.EqualFold(l.Name, name) {
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

func prHasLabelNames(st *Store, pr *PullRequest, names []string) bool {
	for _, name := range names {
		found := false
		for _, lid := range pr.LabelIDs {
			if l := st.Labels[lid]; l != nil && strings.EqualFold(l.Name, name) {
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

func (s *Server) handleSearchRepositories(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	q := parseSearchQuery(r)

	s.store.mu.RLock()
	defer s.store.mu.RUnlock()

	base := s.baseURL(r)
	var results []map[string]interface{}

	for _, repo := range s.store.Repos {
		if !canReadRepo(s.store, user, repo) {
			continue
		}
		if q.Repo != "" && !strings.EqualFold(repo.FullName, q.Repo) {
			continue
		}
		if q.User != "" {
			if repo.OwnerType == "Organization" {
				parts := strings.SplitN(repo.FullName, "/", 2)
				if len(parts) == 0 || !strings.EqualFold(parts[0], q.User) {
					continue
				}
			} else if repo.Owner == nil || !strings.EqualFold(repo.Owner.Login, q.User) {
				continue
			}
		}
		if q.Org != "" {
			if repo.OwnerType != "Organization" {
				continue
			}
			parts := strings.SplitN(repo.FullName, "/", 2)
			if len(parts) == 0 || !strings.EqualFold(parts[0], q.Org) {
				continue
			}
		}
		if q.IsPrivate != nil && *q.IsPrivate != repo.Private {
			continue
		}
		if q.IsPublic != nil && *q.IsPublic == repo.Private {
			continue
		}
		if q.Language != "" && !strings.EqualFold(repo.Language, q.Language) {
			continue
		}
		text := repo.Name + " " + repo.Description + " " + strings.Join(repo.Topics, " ")
		if !q.matchesText(text) {
			continue
		}
		item := repoToJSON(repo, s.store, base)
		item["score"] = 1.0
		results = append(results, item)
	}

	results = sortRepoSearchResults(results, q.Sort, q.Order)
	writeJSON(w, http.StatusOK, searchEnvelope("", results, q.Page, q.PerPage))
}

func (s *Server) handleSearchCode(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	q := parseSearchQuery(r)

	if len(q.Terms) == 0 && q.Filename == "" && q.Extension == "" && q.Path == "" {
		writeGHValidationError(w, "Search", "q", "missing_field")
		return
	}

	s.store.mu.RLock()
	defer s.store.mu.RUnlock()

	base := s.baseURL(r)
	var results []map[string]interface{}

	for _, repo := range s.store.Repos {
		if !canReadRepo(s.store, user, repo) {
			continue
		}
		if q.Repo != "" && !strings.EqualFold(repo.FullName, q.Repo) {
			continue
		}
		if q.User != "" || q.Org != "" {
			parts := strings.SplitN(repo.FullName, "/", 2)
			if len(parts) == 0 {
				continue
			}
			owner := parts[0]
			if q.User != "" && !strings.EqualFold(owner, q.User) {
				continue
			}
			if q.Org != "" && (repo.OwnerType != "Organization" || !strings.EqualFold(owner, q.Org)) {
				continue
			}
		}
		if q.Language != "" && !strings.EqualFold(repo.Language, q.Language) {
			continue
		}

		stor, ok := s.store.GitStorages[repo.FullName]
		if !ok {
			continue
		}
		gr, err := git.Open(stor, nil)
		if err != nil {
			continue
		}
		head, err := gr.Head()
		if err != nil {
			continue
		}
		commit, err := gr.CommitObject(head.Hash())
		if err != nil {
			continue
		}
		tree, err := gr.TreeObject(commit.TreeHash)
		if err != nil {
			continue
		}

		err = tree.Files().ForEach(func(f *object.File) error {
			path := f.Name
			name := filepath.Base(path)
			ext := strings.TrimPrefix(filepath.Ext(name), ".")

			if q.Filename != "" && !strings.EqualFold(name, q.Filename) {
				return nil
			}
			if q.Extension != "" && !strings.EqualFold(ext, q.Extension) {
				return nil
			}
			if q.Path != "" && !strings.Contains(path, q.Path) {
				return nil
			}

			matched := false
			if len(q.Terms) == 0 {
				matched = true
			} else {
				blob, err := gr.BlobObject(plumbing.NewHash(f.Hash.String()))
				if err == nil {
					reader, err := blob.Reader()
					if err == nil {
						data, err := io.ReadAll(reader)
						_ = reader.Close()
						if err == nil && len(data) < 384*1024 {
							content := strings.ToLower(string(data))
							if pathMatches(content, q.Terms) || pathMatches(strings.ToLower(path), q.Terms) {
								matched = true
							}
						}
					}
				}
			}
			if !matched {
				return nil
			}

			api := base + "/api/v3/repos/" + repo.FullName
			item := map[string]interface{}{
				"name":       name,
				"path":       path,
				"sha":        f.Hash.String(),
				"url":        api + "/contents/" + path,
				"git_url":    api + "/git/blobs/" + f.Hash.String(),
				"html_url":   base + "/" + repo.FullName + "/blob/" + repo.DefaultBranch + "/" + path,
				"repository": repoToJSON(repo, s.store, base),
				"score":      1.0,
				"language":   detectLanguage(name),
			}
			results = append(results, item)
			if len(results) >= 1000 {
				return fmt.Errorf("result limit")
			}
			return nil
		})
		if err != nil && err.Error() != "result limit" {
			s.logger.Debug().Err(err).Str("repo", repo.FullName).Msg("code search tree walk")
		}
	}

	writeJSON(w, http.StatusOK, searchEnvelope("", results, q.Page, q.PerPage))
}

func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	_ = ghUserFromContext(r.Context())
	q := parseSearchQuery(r)

	s.store.mu.RLock()
	defer s.store.mu.RUnlock()

	var results []map[string]interface{}

	for _, u := range s.store.Users {
		if q.Type == "org" {
			continue
		}
		text := u.Login + " " + u.Name + " " + u.Bio
		if !q.matchesText(text) {
			continue
		}
		item := s.fullUserJSON(u)
		item["score"] = 1.0
		results = append(results, item)
	}
	for _, org := range s.store.Orgs {
		if q.Type == "user" {
			continue
		}
		text := org.Login + " " + org.Name + " " + org.Description
		if !q.matchesText(text) {
			continue
		}
		item := orgAsSimpleUserJSON(org)
		item["score"] = 1.0
		results = append(results, item)
	}

	results = sortUserSearchResults(results, q.Sort, q.Order)
	writeJSON(w, http.StatusOK, searchEnvelope("", results, q.Page, q.PerPage))
}

func pathMatches(text string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

func detectLanguage(filename string) interface{} {
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	switch strings.ToLower(ext) {
	case "go":
		return "Go"
	case "js", "jsx":
		return "JavaScript"
	case "ts", "tsx":
		return "TypeScript"
	case "py":
		return "Python"
	case "md":
		return "Markdown"
	case "yml", "yaml":
		return "YAML"
	case "json":
		return "JSON"
	case "sh":
		return "Shell"
	case "dockerfile":
		return "Dockerfile"
	case "":
		return nil
	}
	return ext
}

func searchEnvelope(searchType string, items []map[string]interface{}, page, perPage int) map[string]interface{} {
	total := len(items)
	start := (page - 1) * perPage
	if start < 0 || start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	m := map[string]interface{}{
		"total_count":        total,
		"incomplete_results": false,
		"items":              items[start:end],
	}
	if searchType != "" {
		m["search_type"] = searchType
	}
	return m
}

func sortSearchResults(items []map[string]interface{}, sortKey, order string) []map[string]interface{} {
	switch sortKey {
	case "created":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["created_at"].(string)
			b, _ := items[j]["created_at"].(string)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	case "updated":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["updated_at"].(string)
			b, _ := items[j]["updated_at"].(string)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	case "comments":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["comments"].(int)
			b, _ := items[j]["comments"].(int)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	}
	return items
}

func sortRepoSearchResults(items []map[string]interface{}, sortKey, order string) []map[string]interface{} {
	switch sortKey {
	case "stars", "stargazers":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["stargazers_count"].(int)
			b, _ := items[j]["stargazers_count"].(int)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	case "created":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["created_at"].(string)
			b, _ := items[j]["created_at"].(string)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	case "updated":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["updated_at"].(string)
			b, _ := items[j]["updated_at"].(string)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	}
	return items
}

func sortUserSearchResults(items []map[string]interface{}, sortKey, order string) []map[string]interface{} {
	switch sortKey {
	case "followers":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["followers"].(int)
			b, _ := items[j]["followers"].(int)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	case "created":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["created_at"].(string)
			b, _ := items[j]["created_at"].(string)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	case "updated":
		sort.Slice(items, func(i, j int) bool {
			a, _ := items[i]["updated_at"].(string)
			b, _ := items[j]["updated_at"].(string)
			if order == "asc" {
				return a < b
			}
			return a > b
		})
	}
	return items
}
