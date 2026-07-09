package bleephub

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// Repository read surfaces: watchers/teams/assignees, license and community
// health, single-commit and contributor/statistics aggregations over the real
// git history, git ref reads, source archives, activity/events, and traffic.
func (s *Server) registerGHRepoReadsRoutes() {
	// People, watching, access.
	s.route("GET /api/v3/repos/{owner}/{repo}/subscribers", s.handleListRepoSubscribers)
	s.route("GET /api/v3/repos/{owner}/{repo}/subscription", s.requirePerm(scopeContents, permRead, s.handleGetRepoSubscription))
	s.route("GET /api/v3/repos/{owner}/{repo}/teams", s.handleListRepoTeams)
	s.route("GET /api/v3/repos/{owner}/{repo}/assignees", s.handleListRepoAssignees)
	s.route("GET /api/v3/repos/{owner}/{repo}/assignees/{assignee}", s.handleCheckRepoAssignee)
	s.route("GET /api/v3/repos/{owner}/{repo}/collaborators/{username}", s.requirePerm(scopeContents, permRead, s.handleCheckRepoCollaborator))

	// Repository metadata reads.
	s.route("GET /api/v3/repos/{owner}/{repo}/hash-algorithm", s.handleGetRepoHashAlgorithm)
	s.route("GET /api/v3/repos/{owner}/{repo}/license", s.handleGetRepoLicense)
	s.route("GET /api/v3/repos/{owner}/{repo}/readme/{dir...}", s.handleGetReadmeInDirectory)
	s.route("GET /api/v3/repos/{owner}/{repo}/community/profile", s.handleGetCommunityProfile)
	s.route("GET /api/v3/repos/{owner}/{repo}/codeowners/errors", s.handleListCodeownersErrors)
	s.route("GET /api/v3/repositories", s.handleListPublicRepositories)

	// Commits, contributors, statistics.
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{ref...}", s.handleGetSingleCommit)
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{commit_sha}/pulls", s.handleListPullsForCommit)
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{commit_sha}/branches-where-head", s.handleListBranchesWhereHead)
	s.route("GET /api/v3/repos/{owner}/{repo}/contributors", s.handleListRepoContributors)
	s.route("GET /api/v3/repos/{owner}/{repo}/stats/contributors", s.handleStatsContributors)
	s.route("GET /api/v3/repos/{owner}/{repo}/stats/code_frequency", s.handleStatsCodeFrequency)
	s.route("GET /api/v3/repos/{owner}/{repo}/stats/commit_activity", s.handleStatsCommitActivity)
	s.route("GET /api/v3/repos/{owner}/{repo}/stats/participation", s.handleStatsParticipation)
	s.route("GET /api/v3/repos/{owner}/{repo}/stats/punch_card", s.handleStatsPunchCard)

	// Git refs: exact single-ref lookup + prefix listing.
	s.route("GET /api/v3/repos/{owner}/{repo}/git/ref/{ref...}", s.requirePerm(scopeContents, permRead, s.handleGetSingleRef))
	s.route("GET /api/v3/repos/{owner}/{repo}/git/matching-refs/{ref...}", s.requirePerm(scopeContents, permRead, s.handleListMatchingRefs))

	// Source archives: the API endpoints answer 302 to the codeload-style
	// legacy URLs (the same URLs the tags API advertises), which stream real
	// tar.gz/zip archives built from the git tree. The legacy URLs are served
	// from the catch-all (tryHandleArchiveRequest) beside the git smart HTTP
	// protocol — a top-level /{owner}/{repo}/… mux pattern would conflict
	// with the /api/v3/ subtree pattern.
	s.route("GET /api/v3/repos/{owner}/{repo}/tarball/{ref...}", s.handleGetTarball)
	s.route("GET /api/v3/repos/{owner}/{repo}/zipball/{ref...}", s.handleGetZipball)

	// Activity, events, traffic.
	s.route("GET /api/v3/repos/{owner}/{repo}/activity", s.handleListRepoActivity)
	s.route("GET /api/v3/repos/{owner}/{repo}/events", s.handleListRepoEvents)
	s.route("GET /api/v3/networks/{owner}/{repo}/events", s.handleListNetworkEvents)
	s.route("GET /api/v3/repos/{owner}/{repo}/traffic/views", s.handleTrafficViews)
	s.route("GET /api/v3/repos/{owner}/{repo}/traffic/clones", s.handleTrafficClones)
	s.route("GET /api/v3/repos/{owner}/{repo}/traffic/popular/paths", s.handleTrafficPopularPaths)
	s.route("GET /api/v3/repos/{owner}/{repo}/traffic/popular/referrers", s.handleTrafficPopularReferrers)
}

// --- People / watching / access ---

func (s *Server) handleListRepoSubscribers(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	users := s.store.ListRepoSubscribers(repo.ID)
	out := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		out = append(out, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleGetRepoSubscription(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	sub := s.store.GetRepoSubscription(user.ID, repo.ID)
	if sub == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, subscriptionToJSON(sub, repo, s.baseURL(r)))
}

func (s *Server) handleListRepoTeams(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	teams := s.store.ListTeamsForRepo(repo.FullName)
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(teams))
	for _, team := range teams {
		org := s.store.GetOrgByID(team.OrgID)
		if org == nil {
			continue
		}
		out = append(out, teamSimpleJSON(team, org, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleListRepoAssignees(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	users := s.store.ListAssignableUsers(repo)
	out := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		out = append(out, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, out))
}

func (s *Server) handleCheckRepoAssignee(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	assignee := r.PathValue("assignee")
	for _, u := range s.store.ListAssignableUsers(repo) {
		if strings.EqualFold(u.Login, assignee) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleCheckRepoCollaborator(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	username := r.PathValue("username")
	if repo.Owner != nil && strings.EqualFold(repo.Owner.Login, username) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for login := range s.store.ListRepoCollaborators(owner, name) {
		if strings.EqualFold(login, username) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// --- Repository metadata reads ---

func (s *Server) handleGetRepoHashAlgorithm(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	// go-git object storage is SHA-1; bleephub has no SHA-256 repositories.
	writeJSON(w, http.StatusOK, map[string]interface{}{"hash_algorithm": "sha1"})
}

// licenseFileCandidates are the root-level filenames GitHub's license
// detection inspects.
var licenseFileCandidates = []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "COPYING", "COPYING.md"}

func (s *Server) handleGetRepoLicense(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	tree, _, err := s.repoTreeAtRef(repo, r.URL.Query().Get("ref"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	stor := s.gitStorageForRepo(repo)
	name, entry, content, ok := findFirstFile(stor, tree, "", licenseFileCandidates)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	refName := r.URL.Query().Get("ref")
	if refName == "" {
		refName = repo.DefaultBranch
	}
	out := contentFileJSON(s.baseURL(r), repo, refName, name, entry.Hash.String(), int64(len(content)))
	out["encoding"] = "base64"
	out["content"] = base64.StdEncoding.EncodeToString(content)
	out["license"] = s.licenseSimpleJSON(detectLicenseKey(string(content)), s.baseURL(r))
	writeJSON(w, http.StatusOK, out)
}

// detectLicenseKey identifies the license of a file's content by normalized
// token overlap against the license catalog served at /licenses. Content
// matching no catalog entry is "other", exactly as real GitHub reports
// licenses licensee cannot identify.
func detectLicenseKey(content string) string {
	contentTokens := licenseTokenSet(content)
	bestKey := ""
	bestScore := 0.0
	for key, tmpl := range licenseTemplates {
		tmplTokens := licenseTokenSet(tmpl.body)
		if len(tmplTokens) == 0 {
			continue
		}
		matched := 0
		for tok := range tmplTokens {
			if contentTokens[tok] {
				matched++
			}
		}
		score := float64(matched) / float64(len(tmplTokens))
		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}
	if bestScore >= 0.9 {
		return bestKey
	}
	return "other"
}

// licenseTokenSet lowercases and tokenizes license text, dropping the
// placeholder tokens the templates substitute per-repo.
func licenseTokenSet(text string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		switch tok {
		case "year", "fullname":
			continue
		}
		out[tok] = true
	}
	return out
}

// licenseSimpleJSON renders the GitHub license-simple shape for a catalog key
// or the "other" pseudo-license GitHub reports for unidentified licenses.
func (s *Server) licenseSimpleJSON(key, base string) map[string]interface{} {
	if tmpl, ok := licenseTemplates[key]; ok {
		return map[string]interface{}{
			"key":     key,
			"name":    tmpl.name,
			"spdx_id": tmpl.spdxID,
			"url":     base + "/api/v3/licenses/" + key,
			"node_id": "MDc6TGljZW5zZ" + key,
		}
	}
	return map[string]interface{}{
		"key":     "other",
		"name":    "Other",
		"spdx_id": "NOASSERTION",
		"url":     nil,
		"node_id": "MDc6TGljZW5zZTA=",
	}
}

// readmeFileCandidates mirrors the variants handleGetReadme accepts.
var readmeFileCandidates = []string{"README.md", "README", "README.txt", "readme.md"}

func (s *Server) handleGetReadmeInDirectory(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	dir := strings.Trim(r.PathValue("dir"), "/")
	tree, _, err := s.repoTreeAtRef(repo, r.URL.Query().Get("ref"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if dir != "" {
		tree, err = tree.Tree(dir)
		if err != nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	stor := s.gitStorageForRepo(repo)
	name, entry, content, ok := findFirstFile(stor, tree, "", readmeFileCandidates)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	refName := r.URL.Query().Get("ref")
	if refName == "" {
		refName = repo.DefaultBranch
	}
	path := name
	if dir != "" {
		path = dir + "/" + name
	}
	out := contentFileJSON(s.baseURL(r), repo, refName, path, entry.Hash.String(), int64(len(content)))
	out["encoding"] = "base64"
	out["content"] = base64.StdEncoding.EncodeToString(content)
	writeJSON(w, http.StatusOK, out)
}

// communityFileLocations are the directories GitHub scans for community
// health files, in precedence order.
var communityFileLocations = []string{".github", "", "docs"}

func (s *Server) handleGetCommunityProfile(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	base := s.baseURL(r)
	var tree *object.Tree
	if t, _, err := s.repoTreeAtRef(repo, ""); err == nil {
		tree = t
	}
	stor := s.gitStorageForRepo(repo)

	healthFile := func(candidates []string) (map[string]interface{}, bool) {
		if tree == nil {
			return nil, false
		}
		for _, loc := range communityFileLocations {
			scope := tree
			if loc != "" {
				sub, err := tree.Tree(loc)
				if err != nil {
					continue
				}
				scope = sub
			}
			name, _, _, ok := findFirstFile(stor, scope, "", candidates)
			if !ok {
				continue
			}
			path := name
			if loc != "" {
				path = loc + "/" + name
			}
			return map[string]interface{}{
				"url":      base + "/api/v3/repos/" + repo.FullName + "/contents/" + path,
				"html_url": base + "/" + repo.FullName + "/blob/" + repo.DefaultBranch + "/" + path,
			}, true
		}
		return nil, false
	}

	readme, hasReadme := healthFile(readmeFileCandidates)
	contributing, hasContributing := healthFile([]string{"CONTRIBUTING.md", "CONTRIBUTING"})
	codeOfConductFile, hasCoC := healthFile([]string{"CODE_OF_CONDUCT.md", "CODE_OF_CONDUCT"})
	issueTemplate, hasIssueTemplate := healthFile([]string{"ISSUE_TEMPLATE.md", "ISSUE_TEMPLATE"})
	prTemplate, hasPRTemplate := healthFile([]string{"PULL_REQUEST_TEMPLATE.md", "PULL_REQUEST_TEMPLATE"})

	var license map[string]interface{}
	hasLicense := false
	if tree != nil {
		if _, _, content, ok := findFirstFile(stor, tree, "", licenseFileCandidates); ok {
			license = s.licenseSimpleJSON(detectLicenseKey(string(content)), base)
			hasLicense = true
		}
	}

	checks := []bool{repo.Description != "", hasReadme, hasContributing, hasCoC, hasLicense, hasIssueTemplate, hasPRTemplate}
	present := 0
	for _, c := range checks {
		if c {
			present++
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"health_percentage": present * 100 / len(checks),
		"description":       nilOrString(repo.Description),
		"documentation":     nil,
		"files": map[string]interface{}{
			"code_of_conduct":       nil,
			"code_of_conduct_file":  orNil(codeOfConductFile),
			"license":               orNil(license),
			"contributing":          orNil(contributing),
			"readme":                orNil(readme),
			"issue_template":        orNil(issueTemplate),
			"pull_request_template": orNil(prTemplate),
		},
		"updated_at": repo.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// orNil converts a nil map into a typed-nil-free JSON null.
func orNil(m map[string]interface{}) interface{} {
	if m == nil {
		return nil
	}
	return m
}

// codeownersLocations are the paths GitHub searches for a CODEOWNERS file, in
// precedence order; the first found is the operative file.
var codeownersLocations = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

func (s *Server) handleListCodeownersErrors(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	errorsOut := []map[string]interface{}{}
	tree, _, err := s.repoTreeAtRef(repo, r.URL.Query().Get("ref"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"errors": errorsOut})
		return
	}
	stor := s.gitStorageForRepo(repo)
	for _, path := range codeownersLocations {
		entry, err := tree.FindEntry(path)
		if err != nil || !entry.Mode.IsFile() {
			continue
		}
		content, err := readBlob(stor, entry.Hash)
		if err != nil {
			continue
		}
		errorsOut = s.validateCodeowners(string(content), path)
		break
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"errors": errorsOut})
}

// validateCodeowners parses CODEOWNERS content and reports the errors real
// GitHub reports: malformed owner tokens ("Invalid owner") and owners that
// reference no existing user, organization, or team ("Unknown owner").
func (s *Server) validateCodeowners(content, path string) []map[string]interface{} {
	out := []map[string]interface{}{}
	for lineNo, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		for _, owner := range fields[1:] {
			kind := s.classifyCodeowner(owner)
			if kind == "" {
				continue
			}
			column := strings.Index(line, owner) + 1
			pointer := strings.Repeat(" ", column-1) + "^"
			out = append(out, map[string]interface{}{
				"line":       lineNo + 1,
				"column":     column,
				"source":     line,
				"kind":       kind,
				"suggestion": nil,
				"message":    fmt.Sprintf("%s on line %d:\n\n  %s\n  %s", kind, lineNo+1, line, pointer),
				"path":       path,
			})
		}
	}
	return out
}

// classifyCodeowner returns the CODEOWNERS error kind for an owner token, or
// "" when the token is a valid, resolvable owner.
func (s *Server) classifyCodeowner(owner string) string {
	if strings.HasPrefix(owner, "@") {
		name := strings.TrimPrefix(owner, "@")
		if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Count(name, "/") > 1 {
			return "Invalid owner"
		}
		if orgName, teamSlug, isTeam := strings.Cut(name, "/"); isTeam {
			org := s.store.GetOrg(orgName)
			if org == nil {
				return "Unknown owner"
			}
			if team := s.store.GetTeam(orgName, teamSlug); team == nil {
				return "Unknown owner"
			}
			return ""
		}
		if s.store.LookupUserByLogin(name) == nil && s.store.GetOrg(name) == nil {
			return "Unknown owner"
		}
		return ""
	}
	// Email owner: must look like an address.
	if at := strings.Index(owner, "@"); at > 0 && at < len(owner)-1 {
		return ""
	}
	return "Invalid owner"
}

func (s *Server) handleListPublicRepositories(w http.ResponseWriter, r *http.Request) {
	since := queryInt(r.URL.Query(), "since", 0)
	repos := s.store.ListPublicRepos(since)
	// GET /repositories pages by `since` (repo ID), not page/per_page.
	const pageSize = 100
	more := len(repos) > pageSize
	if more {
		repos = repos[:pageSize]
	}
	base := s.baseURL(r)
	viewer := ghUserFromContext(r.Context())
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repoToJSONForViewer(repo, s.store, base, viewer))
	}
	if more {
		last := repos[len(repos)-1].ID
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/repositories?since=%d>; rel="next"`, base, last))
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Git refs ---

func (s *Server) handleGetSingleRef(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil || stor == nil {
		return
	}
	refPath := strings.Trim(r.PathValue("ref"), "/")
	if refPath == "" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	ref, err := stor.Reference(plumbing.ReferenceName("refs/" + refPath))
	if err != nil || ref.Type() != plumbing.HashReference {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, refToJSON(s.baseURL(r), repo.FullName, ref))
}

func (s *Server) handleListMatchingRefs(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil || stor == nil {
		return
	}
	prefix := "refs/" + strings.Trim(r.PathValue("ref"), "/")
	base := s.baseURL(r)
	items := []map[string]interface{}{}
	refs, err := stor.IterReferences()
	if err == nil {
		_ = refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Type() != plumbing.HashReference {
				return nil
			}
			if !strings.HasPrefix(string(ref.Name()), prefix) {
				return nil
			}
			items = append(items, refToJSON(base, repo.FullName, ref))
			return nil
		})
	}
	sort.Slice(items, func(i, j int) bool {
		ri, _ := items[i]["ref"].(string)
		rj, _ := items[j]["ref"].(string)
		return ri < rj
	})
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, items))
}

// --- shared git helpers ---

// gitStorageForRepo returns the repo's git storage, or nil.
func (s *Server) gitStorageForRepo(repo *Repo) gitStorage.Storer {
	owner, name, ok := splitRepoFullName(repo.FullName)
	if !ok {
		return nil
	}
	return s.store.GetGitStorage(owner, name)
}

// repoTreeAtRef resolves a ref (or the default branch when empty) to its
// commit and root tree.
func (s *Server) repoTreeAtRef(repo *Repo, refName string) (*object.Tree, *object.Commit, error) {
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		return nil, nil, fmt.Errorf("no git storage for %s", repo.FullName)
	}
	if refName == "" {
		refName = repo.DefaultBranch
	}
	hash, err := resolveGitRef(stor, refName)
	if err != nil {
		return nil, nil, err
	}
	commit, err := object.GetCommit(stor, hash)
	if err != nil {
		return nil, nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, nil, err
	}
	return tree, commit, nil
}

// findFirstFile locates the first candidate filename present in the tree and
// returns its name, entry, and content.
func findFirstFile(stor gitStorage.Storer, tree *object.Tree, prefix string, candidates []string) (string, *object.TreeEntry, []byte, bool) {
	for _, name := range candidates {
		lookup := name
		if prefix != "" {
			lookup = prefix + "/" + name
		}
		entry, err := tree.FindEntry(lookup)
		if err != nil || !entry.Mode.IsFile() {
			continue
		}
		content, err := readBlob(stor, entry.Hash)
		if err != nil {
			continue
		}
		return name, entry, content, true
	}
	return "", nil, nil, false
}

// readBlob returns a blob's full content.
func readBlob(stor gitStorage.Storer, hash plumbing.Hash) ([]byte, error) {
	blob, err := object.GetBlob(stor, hash)
	if err != nil {
		return nil, err
	}
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// shortSHA returns the 7-character abbreviation GitHub uses in archive
// directory names.
func shortSHA(h plumbing.Hash) string {
	full := h.String()
	if len(full) < 7 {
		return full
	}
	return full[:7]
}
