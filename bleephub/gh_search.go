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
	gitStorage "github.com/go-git/go-git/v5/storage"
)

func (s *Server) registerGHSearchRoutes() {
	s.route("GET /api/v3/search/issues", s.handleSearchIssues)
	s.route("GET /api/v3/search/repositories", s.handleSearchRepositories)
	s.route("GET /api/v3/search/code", s.handleSearchCode)
	s.route("GET /api/v3/search/users", s.handleSearchUsers)
	s.route("GET /api/v3/search/commits", s.handleSearchCommits)
	s.route("GET /api/v3/search/labels", s.handleSearchLabels)
	s.route("GET /api/v3/search/topics", s.handleSearchTopics)
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
	Author    string // commit search: author qualifier
	Hash      string // commit search: hash qualifier
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
			case "author":
				q.Author = val
			case "hash":
				q.Hash = val
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

	// Matching rows are gathered under the read lock; rendering happens
	// after release because the JSON builders (issueToJSON, repoToJSON and
	// friends) take the store lock themselves.
	type issueRow struct {
		issue *Issue
		repo  *Repo
		assoc string
	}
	type prRow struct {
		pr    *PullRequest
		repo  *Repo
		assoc string
	}
	var issueRows []issueRow
	var prRows []prRow

	s.store.mu.RLock()

	for _, issue := range s.store.Issues {
		repo := s.store.Repos[issue.RepoID]
		if repo == nil {
			continue
		}
		if !canReadRepoLocked(s.store, user, repo) {
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

		issueRows = append(issueRows, issueRow{issue, repo, authorAssociationLocked(s.store, issue.AuthorID, repo)})
	}

	for _, pr := range s.store.PullRequests {
		repo := s.store.Repos[pr.RepoID]
		if repo == nil {
			continue
		}
		if !canReadRepoLocked(s.store, user, repo) {
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

		prRows = append(prRows, prRow{pr, repo, authorAssociationLocked(s.store, pr.AuthorID, repo)})
	}

	s.store.mu.RUnlock()

	base := s.baseURL(r)

	// Unify matched issue and PR rows so the whole result set can be sorted and
	// paginated *before* rendering — rendering every matched row (issueToJSON +
	// repoToJSON per row) only to return one page is the dominant per-request
	// cost on a large corpus.
	rows := make([]searchIssueRow, 0, len(issueRows)+len(prRows))
	for _, row := range issueRows {
		rows = append(rows, searchIssueRow{issue: row.issue, repo: row.repo, assoc: row.assoc})
	}
	for _, row := range prRows {
		rows = append(rows, searchIssueRow{pr: row.pr, repo: row.repo, assoc: row.assoc})
	}

	render := func(row searchIssueRow) map[string]interface{} {
		if row.issue != nil {
			item := issueToJSON(row.issue, s.store, base, row.repo.FullName)
			item["score"] = 1.0
			item["author_association"] = row.assoc
			item["draft"] = false
			item["pull_request"] = nil
			item["repository"] = repoToJSON(row.repo, s.store, base)
			return item
		}
		item := issueToJSONForPR(row.pr, s.store, base, row.repo.FullName)
		item["score"] = 1.0
		item["author_association"] = row.assoc
		item["repository"] = repoToJSON(row.repo, s.store, base)
		return item
	}

	total := len(rows)

	// The "comments" sort key needs a rendered comment count per row, so it
	// keeps the render-all path (rare). created/updated/best-match sort only on
	// row timestamps, so those render just the requested page.
	if q.Sort == "comments" {
		results := make([]map[string]interface{}, 0, total)
		for _, row := range rows {
			results = append(results, render(row))
		}
		results = sortSearchResults(results, q.Sort, q.Order)
		writeJSON(w, http.StatusOK, searchEnvelope("issues", results, q.Page, q.PerPage))
		return
	}

	sortSearchRows(rows, q.Sort, q.Order)
	start, end := searchPageBounds(q.Page, q.PerPage, total)
	pageItems := make([]map[string]interface{}, 0, end-start)
	for _, row := range rows[start:end] {
		pageItems = append(pageItems, render(row))
	}
	out := map[string]interface{}{
		"total_count":        total,
		"incomplete_results": false,
		"items":              pageItems,
		"search_type":        "issues",
	}
	writeJSON(w, http.StatusOK, out)
}

// searchIssueRow is a matched issue or PR gathered by the issue-search scan,
// carrying just enough to sort and then render only the requested page.
type searchIssueRow struct {
	issue *Issue
	pr    *PullRequest
	repo  *Repo
	assoc string
}

func (row searchIssueRow) createdAt() time.Time {
	if row.issue != nil {
		return row.issue.CreatedAt
	}
	return row.pr.CreatedAt
}

func (row searchIssueRow) updatedAt() time.Time {
	if row.issue != nil {
		return row.issue.UpdatedAt
	}
	return row.pr.UpdatedAt
}

// sortSearchRows orders unified search rows in place by the same keys as
// sortSearchResults (created/updated), so paginate-before-render yields the
// identical page a render-all-then-sort would.
func sortSearchRows(rows []searchIssueRow, sortKey, order string) {
	switch sortKey {
	case "created":
		sort.SliceStable(rows, func(i, j int) bool {
			if order == "asc" {
				return rows[i].createdAt().Before(rows[j].createdAt())
			}
			return rows[i].createdAt().After(rows[j].createdAt())
		})
	case "updated":
		sort.SliceStable(rows, func(i, j int) bool {
			if order == "asc" {
				return rows[i].updatedAt().Before(rows[j].updatedAt())
			}
			return rows[i].updatedAt().After(rows[j].updatedAt())
		})
	}
}

// searchPageBounds computes the [start,end) slice bounds for the given page.
func searchPageBounds(page, perPage, total int) (int, int) {
	start := (page - 1) * perPage
	if start < 0 || start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return start, end
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

// issueToJSONForPullRequest renders a pull request in the issue shape.
// Must not be called with st.mu held: it takes the read lock itself and
// derives milestone issue counts via milestoneToJSON, which locks too.
func issueToJSONForPullRequest(pr *PullRequest, st *Store, baseURL, repoFullName string) map[string]interface{} {
	st.mu.RLock()
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
	// Grab the milestone pointer; conversion happens after unlock because
	// milestoneToJSON derives issue counts under its own lock.
	var milestone *Milestone
	if pr.MilestoneID > 0 {
		milestone = st.Milestones[pr.MilestoneID]
	}
	commentCount := st.countCommentsForLocked("pull_request", pr.ID)
	st.mu.RUnlock()

	var milestoneJSON interface{}
	if milestone != nil {
		milestoneJSON = milestoneToJSON(milestone, st, baseURL, repoFullName)
	}
	var closedAt interface{}
	if pr.ClosedAt != nil {
		closedAt = pr.ClosedAt.Format(time.RFC3339)
	}
	var activeLockReason interface{}
	if pr.Locked {
		activeLockReason = pr.ActiveLockReason
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

// authorAssociation returns the author_association value for the author of
// an issue, pull request or comment within repo. Must not be called with
// st.mu held; it takes the read lock itself.
func authorAssociation(st *Store, authorID int, repo *Repo) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return authorAssociationLocked(st, authorID, repo)
}

// authorAssociationLocked is authorAssociation for callers that already hold
// st.mu; it never acquires the lock itself.
func authorAssociationLocked(st *Store, authorID int, repo *Repo) string {
	author := st.Users[authorID]
	if author == nil {
		return "NONE"
	}
	if repo.Owner != nil && repo.Owner.ID == author.ID {
		return "OWNER"
	}
	return "CONTRIBUTOR"
}

// issueHasLabelNames reports whether the issue carries every named label.
// Callers hold st.mu (it reads st.Labels directly).
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

// prHasLabelNames reports whether the pull request carries every named
// label. Callers hold st.mu (it reads st.Labels directly).
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

	// Matching repos are gathered under the read lock; rendering happens
	// after release because repoToJSON takes the store lock itself.
	var matched []*Repo
	s.store.mu.RLock()
	for _, repo := range s.store.Repos {
		if !canReadRepoLocked(s.store, user, repo) {
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
		matched = append(matched, repo)
	}
	s.store.mu.RUnlock()

	base := s.baseURL(r)
	var results []map[string]interface{}
	for _, repo := range matched {
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

	// Readable repos and their git storages are gathered under the read
	// lock; the tree walk and rendering happen after release because
	// repoToJSON takes the store lock itself.
	type codeSearchRepo struct {
		repo *Repo
		stor gitStorage.Storer
	}
	var searchRepos []codeSearchRepo
	s.store.mu.RLock()
	for _, repo := range s.store.Repos {
		if !canReadRepoLocked(s.store, user, repo) {
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
		searchRepos = append(searchRepos, codeSearchRepo{repo, stor})
	}
	s.store.mu.RUnlock()

	base := s.baseURL(r)
	var results []map[string]interface{}

	for _, sr := range searchRepos {
		repo := sr.repo
		gr, err := git.Open(sr.stor, nil)
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
	q := parseSearchQuery(r)

	// Matching users and orgs are gathered under the read lock; rendering
	// happens after release because fullUserJSON derives follower and repo
	// counts under the store and misc locks itself.
	var users []*User
	var orgs []*Org
	s.store.mu.RLock()
	for _, u := range s.store.Users {
		if q.Type == "org" {
			continue
		}
		text := u.Login + " " + u.Name + " " + u.Bio
		if !q.matchesText(text) {
			continue
		}
		users = append(users, u)
	}
	for _, org := range s.store.Orgs {
		if q.Type == "user" {
			continue
		}
		text := org.Login + " " + org.Name + " " + org.Description
		if !q.matchesText(text) {
			continue
		}
		orgs = append(orgs, org)
	}
	s.store.mu.RUnlock()

	var results []map[string]interface{}
	for _, u := range users {
		item := s.fullUserJSON(u)
		// user-search-result-item carries no twitter_username member.
		delete(item, "twitter_username")
		item["score"] = 1.0
		results = append(results, item)
	}
	for _, org := range orgs {
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
	// GitHub's search envelope always carries items as an array; an empty
	// result set is [], never null. Slicing a nil source (no matches) yields a
	// nil slice that would marshal to null, so materialize an empty array.
	pageItems := items[start:end]
	if pageItems == nil {
		pageItems = []map[string]interface{}{}
	}
	m := map[string]interface{}{
		"total_count":        total,
		"incomplete_results": false,
		"items":              pageItems,
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

// handleSearchCommits implements GET /search/commits: a real search across
// the git commit history of every repository the caller can read, matching
// query terms against commit messages with repo:/user:/org:/author:/hash:
// qualifiers.
func (s *Server) handleSearchCommits(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	q := parseSearchQuery(r)

	if len(q.Terms) == 0 && q.Repo == "" && q.Author == "" && q.Hash == "" && q.User == "" && q.Org == "" {
		writeGHValidationError(w, "Search", "q", "missing_field")
		return
	}

	// Readable repos and their git storages are gathered under the read
	// lock; the log walk and rendering happen after release because
	// commitSearchItemJSON and commitAuthorMatches take the store lock
	// themselves.
	type commitSearchRepo struct {
		repo *Repo
		stor gitStorage.Storer
	}
	var searchRepos []commitSearchRepo
	s.store.mu.RLock()
	for _, repo := range s.store.Repos {
		if !canReadRepoLocked(s.store, user, repo) {
			continue
		}
		if q.Repo != "" && !strings.EqualFold(repo.FullName, q.Repo) {
			continue
		}
		if q.User != "" || q.Org != "" {
			owner, _, _ := strings.Cut(repo.FullName, "/")
			if q.User != "" && !strings.EqualFold(owner, q.User) {
				continue
			}
			if q.Org != "" && (repo.OwnerType != "Organization" || !strings.EqualFold(owner, q.Org)) {
				continue
			}
		}
		stor, ok := s.store.GitStorages[repo.FullName]
		if !ok {
			continue
		}
		searchRepos = append(searchRepos, commitSearchRepo{repo, stor})
	}
	s.store.mu.RUnlock()

	base := s.baseURL(r)
	var results []map[string]interface{}

	for _, sr := range searchRepos {
		repo := sr.repo
		gr, err := git.Open(sr.stor, nil)
		if err != nil {
			continue
		}
		head, err := gr.Head()
		if err != nil {
			continue
		}
		iter, err := gr.Log(&git.LogOptions{From: head.Hash()})
		if err != nil {
			continue
		}
		err = iter.ForEach(func(commit *object.Commit) error {
			if !q.matchesText(commit.Message) {
				return nil
			}
			sha := commit.Hash.String()
			if q.Hash != "" && !strings.HasPrefix(sha, q.Hash) {
				return nil
			}
			if q.Author != "" && !commitAuthorMatches(s.store, commit, q.Author) {
				return nil
			}
			results = append(results, s.commitSearchItemJSON(commit, repo, base))
			if len(results) >= 1000 {
				return fmt.Errorf("result limit")
			}
			return nil
		})
		if err != nil && err.Error() != "result limit" {
			s.logger.Debug().Err(err).Str("repo", repo.FullName).Msg("commit search log walk")
		}
	}

	sortCommitSearchResults(results, q.Sort, q.Order)
	writeJSON(w, http.StatusOK, searchEnvelope("", results, q.Page, q.PerPage))
}

// commitAuthorMatches matches the author: qualifier against the commit's
// git author name/email and against the login of a store user with the
// commit author's email. Must not be called with st.mu held; it takes the
// read lock itself.
func commitAuthorMatches(st *Store, commit *object.Commit, author string) bool {
	if strings.EqualFold(commit.Author.Name, author) {
		return true
	}
	if strings.EqualFold(commit.Author.Email, author) {
		return true
	}
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, u := range st.Users {
		if strings.EqualFold(u.Login, author) && strings.EqualFold(u.Email, commit.Author.Email) {
			return true
		}
	}
	return false
}

// commitSearchItemJSON renders the spec `commit-search-result-item` shape.
// Must not be called with the store lock held: it resolves the author under
// its own read lock and embeds the repository via repoToJSON, which derives
// counters under the store lock itself.
func (s *Server) commitSearchItemJSON(commit *object.Commit, repo *Repo, base string) map[string]interface{} {
	sha := commit.Hash.String()
	api := base + "/api/v3/repos/" + repo.FullName

	// The top-level author is the GitHub account behind the commit author
	// email (null when the email matches no account).
	var authorJSON interface{}
	s.store.mu.RLock()
	for _, u := range s.store.Users {
		if u.Email != "" && strings.EqualFold(u.Email, commit.Author.Email) {
			authorJSON = userToJSON(u)
			break
		}
	}
	s.store.mu.RUnlock()

	parents := make([]map[string]interface{}, 0, len(commit.ParentHashes))
	for _, p := range commit.ParentHashes {
		parents = append(parents, map[string]interface{}{
			"sha":      p.String(),
			"url":      api + "/commits/" + p.String(),
			"html_url": base + "/" + repo.FullName + "/commit/" + p.String(),
		})
	}

	return map[string]interface{}{
		"sha":          sha,
		"node_id":      "C_" + sha[:16],
		"url":          api + "/commits/" + sha,
		"html_url":     base + "/" + repo.FullName + "/commit/" + sha,
		"comments_url": api + "/commits/" + sha + "/comments",
		"commit": map[string]interface{}{
			"author": map[string]interface{}{
				"name":  commit.Author.Name,
				"email": commit.Author.Email,
				"date":  commit.Author.When.UTC().Format(time.RFC3339),
			},
			"committer": map[string]interface{}{
				"name":  commit.Committer.Name,
				"email": commit.Committer.Email,
				"date":  commit.Committer.When.UTC().Format(time.RFC3339),
			},
			"comment_count": 0,
			"message":       strings.TrimRight(commit.Message, "\n"),
			"tree": map[string]interface{}{
				"sha": commit.TreeHash.String(),
				"url": api + "/git/trees/" + commit.TreeHash.String(),
			},
			"url": api + "/git/commits/" + sha,
		},
		"author": authorJSON,
		"committer": map[string]interface{}{
			"name":  commit.Committer.Name,
			"email": commit.Committer.Email,
			"date":  commit.Committer.When.UTC().Format(time.RFC3339),
		},
		"parents":    parents,
		"repository": repoToJSON(repo, s.store, base),
		"score":      1.0,
	}
}

// sortCommitSearchResults orders commit results for the documented sort
// keys (author-date, committer-date); default is best-match order.
func sortCommitSearchResults(items []map[string]interface{}, sortKey, order string) {
	if sortKey != "author-date" && sortKey != "committer-date" {
		return
	}
	role := "author"
	if sortKey == "committer-date" {
		role = "committer"
	}
	dateOf := func(item map[string]interface{}) string {
		commit, _ := item["commit"].(map[string]interface{})
		gitUser, _ := commit[role].(map[string]interface{})
		d, _ := gitUser["date"].(string)
		return d
	}
	sort.SliceStable(items, func(i, j int) bool {
		if order == "asc" {
			return dateOf(items[i]) < dateOf(items[j])
		}
		return dateOf(items[i]) > dateOf(items[j])
	})
}

// handleSearchLabels implements GET /search/labels: a real search over the
// labels of the repository named by the required repository_id parameter.
func (s *Server) handleSearchLabels(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repoIDStr := r.URL.Query().Get("repository_id")
	if repoIDStr == "" {
		writeGHValidationError(w, "Search", "repository_id", "missing_field")
		return
	}
	if r.URL.Query().Get("q") == "" {
		writeGHValidationError(w, "Search", "q", "missing_field")
		return
	}
	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		writeGHValidationError(w, "Search", "repository_id", "invalid")
		return
	}
	repo := s.store.GetRepoByID(repoID)
	if repo == nil || !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	q := parseSearchQuery(r)

	s.store.mu.RLock()
	labels := make([]*IssueLabel, 0)
	for _, l := range s.store.Labels {
		if l.RepoID != repo.ID {
			continue
		}
		if !q.matchesText(l.Name + " " + l.Description) {
			continue
		}
		labels = append(labels, l)
	}
	s.store.mu.RUnlock()
	sort.Slice(labels, func(i, j int) bool { return labels[i].ID < labels[j].ID })

	base := s.baseURL(r)
	items := make([]map[string]interface{}, 0, len(labels))
	for _, l := range labels {
		items = append(items, map[string]interface{}{
			"id":          l.ID,
			"node_id":     l.NodeID,
			"url":         base + "/api/v3/repos/" + repo.FullName + "/labels/" + l.Name,
			"name":        l.Name,
			"color":       l.Color,
			"default":     l.Default,
			"description": nullOrString(l.Description),
			"score":       1.0,
		})
	}
	writeJSON(w, http.StatusOK, searchEnvelope("", items, q.Page, q.PerPage))
}

// handleSearchTopics implements GET /search/topics: a real search over the
// topics applied to repositories the caller can read.
func (s *Server) handleSearchTopics(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if r.URL.Query().Get("q") == "" {
		writeGHValidationError(w, "Search", "q", "missing_field")
		return
	}
	q := parseSearchQuery(r)

	type topicAgg struct {
		name      string
		count     int
		createdAt time.Time
		updatedAt time.Time
	}
	s.store.mu.RLock()
	agg := map[string]*topicAgg{}
	for _, repo := range s.store.Repos {
		if !canReadRepoLocked(s.store, user, repo) {
			continue
		}
		for _, topic := range repo.Topics {
			if !q.matchesText(topic) {
				continue
			}
			t := agg[topic]
			if t == nil {
				t = &topicAgg{name: topic, createdAt: repo.CreatedAt, updatedAt: repo.UpdatedAt}
				agg[topic] = t
			}
			t.count++
			if repo.CreatedAt.Before(t.createdAt) {
				t.createdAt = repo.CreatedAt
			}
			if repo.UpdatedAt.After(t.updatedAt) {
				t.updatedAt = repo.UpdatedAt
			}
		}
	}
	s.store.mu.RUnlock()

	topics := make([]*topicAgg, 0, len(agg))
	for _, t := range agg {
		topics = append(topics, t)
	}
	sort.Slice(topics, func(i, j int) bool { return topics[i].name < topics[j].name })

	items := make([]map[string]interface{}, 0, len(topics))
	for _, t := range topics {
		items = append(items, map[string]interface{}{
			"name":              t.name,
			"display_name":      nil,
			"short_description": nil,
			"description":       nil,
			"created_by":        nil,
			"released":          nil,
			"created_at":        t.createdAt.UTC().Format(time.RFC3339),
			"updated_at":        t.updatedAt.UTC().Format(time.RFC3339),
			"featured":          false,
			"curated":           false,
			"score":             1.0,
			"repository_count":  t.count,
		})
	}
	writeJSON(w, http.StatusOK, searchEnvelope("", items, q.Page, q.PerPage))
}
