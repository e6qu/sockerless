package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Releases API.
// Real GH endpoints:
//   POST   /repos/{o}/{r}/releases                     create
//   GET    /repos/{o}/{r}/releases                     list (paginated)
//   GET    /repos/{o}/{r}/releases/latest              latest non-draft non-prerelease
//   GET    /repos/{o}/{r}/releases/tags/{tag}          by tag
//   GET    /repos/{o}/{r}/releases/{release_id}        by id
//   PATCH  /repos/{o}/{r}/releases/{release_id}        update
//   DELETE /repos/{o}/{r}/releases/{release_id}        delete
//   POST   /repos/{o}/{r}/releases/generate-notes      autogen body from commits
//
// `gh release create` uses POST and PATCH; `gh release list/view` uses GET.

// Release is a tagged release on a repo.
//
// AuthorID/RepoID carry real json names so persistence round-trips the
// linkage (the reload path re-indexes byRepo from RepoID). Client responses
// never marshal this struct — releaseToJSON emits an explicit map.
type Release struct {
	ID              int             `json:"id"`
	NodeID          string          `json:"node_id"`
	TagName         string          `json:"tag_name"`
	TargetCommitish string          `json:"target_commitish"`
	Name            string          `json:"name"`
	Body            string          `json:"body"`
	Draft           bool            `json:"draft"`
	Prerelease      bool            `json:"prerelease"`
	AuthorID        int             `json:"author_id"`
	RepoID          int             `json:"repo_id"`
	Assets          []*ReleaseAsset `json:"-"`
	CreatedAt       time.Time       `json:"created_at"`
	PublishedAt     *time.Time      `json:"published_at"`
}

// ReleaseAsset attaches to a release.
type ReleaseAsset struct {
	ID            int       `json:"id"`
	NodeID        string    `json:"node_id"`
	Name          string    `json:"name"`
	Label         string    `json:"label"`
	State         string    `json:"state"`
	ContentType   string    `json:"content_type"`
	Size          int       `json:"size"`
	DownloadCount int       `json:"download_count"`
	UploaderID    int       `json:"-"`
	ReleaseID     int       `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ReleaseStore wraps release CRUD with a mutex.
type ReleaseStore struct {
	mu        sync.RWMutex
	byID      map[int]*Release
	byRepo    map[int][]*Release
	assetByID map[int]*ReleaseAsset
	nextID    int
	nextAsset int
	persist   *Persistence
}

func newReleaseStore(p *Persistence) *ReleaseStore {
	return &ReleaseStore{
		byID:      map[int]*Release{},
		byRepo:    map[int][]*Release{},
		assetByID: map[int]*ReleaseAsset{},
		nextID:    1,
		nextAsset: 1,
		persist:   p,
	}
}

func (rs *ReleaseStore) Create(repoID, authorID int, tagName, target, name, body string, draft, prerelease bool) *Release {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	now := time.Now().UTC()
	id := rs.nextID
	rs.nextID++
	r := &Release{
		ID:              id,
		NodeID:          fmt.Sprintf("RE_kgDO%08d", id),
		TagName:         tagName,
		TargetCommitish: target,
		Name:            name,
		Body:            body,
		Draft:           draft,
		Prerelease:      prerelease,
		AuthorID:        authorID,
		RepoID:          repoID,
		CreatedAt:       now,
	}
	if !draft {
		r.PublishedAt = &now
	}
	rs.byID[id] = r
	rs.byRepo[repoID] = append(rs.byRepo[repoID], r)
	if rs.persist != nil {
		rs.persist.MustPut("releases", strconv.Itoa(id), r)
	}
	return r
}

func (rs *ReleaseStore) Get(id int) *Release {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.byID[id]
}

func (rs *ReleaseStore) GetByTag(repoID int, tag string) *Release {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for _, r := range rs.byRepo[repoID] {
		if r.TagName == tag {
			return r
		}
	}
	return nil
}

// Latest returns the most-recently-created non-draft non-prerelease release.
func (rs *ReleaseStore) Latest(repoID int) *Release {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	var latest *Release
	for _, r := range rs.byRepo[repoID] {
		if r.Draft || r.Prerelease {
			continue
		}
		if latest == nil || r.CreatedAt.After(latest.CreatedAt) {
			latest = r
		}
	}
	return latest
}

func (rs *ReleaseStore) List(repoID int) []*Release {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	out := make([]*Release, len(rs.byRepo[repoID]))
	copy(out, rs.byRepo[repoID])
	// Real GitHub lists releases newest-first; IDs are monotonic at
	// creation, so id-desc is stable across restarts even though the
	// persistence loader rebuilds byRepo in map-iteration order.
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (rs *ReleaseStore) Update(id int, fn func(*Release)) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.byID[id]
	if r == nil {
		return false
	}
	wasDraft := r.Draft
	fn(r)
	if wasDraft && !r.Draft && r.PublishedAt == nil {
		now := time.Now().UTC()
		r.PublishedAt = &now
	}
	if rs.persist != nil {
		rs.persist.MustPut("releases", strconv.Itoa(id), r)
	}
	return true
}

// DeleteAllForRepo purges every release for a repository, in memory and on
// disk. Used by the delete-repo cascade so a recreated same-name repo can't
// inherit the old repo's releases after a restart.
func (rs *ReleaseStore) DeleteAllForRepo(repoID int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.byRepo[repoID] {
		delete(rs.byID, r.ID)
		if rs.persist != nil {
			rs.persist.MustDelete("releases", strconv.Itoa(r.ID))
		}
	}
	delete(rs.byRepo, repoID)
}

func (rs *ReleaseStore) Delete(id int) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.byID[id]
	if r == nil {
		return false
	}
	delete(rs.byID, id)
	src := rs.byRepo[r.RepoID]
	for i, x := range src {
		if x.ID == id {
			rs.byRepo[r.RepoID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	if rs.persist != nil {
		rs.persist.MustDelete("releases", strconv.Itoa(id))
	}
	return true
}

func (s *Server) registerGHReleasesRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/releases",
		s.requirePerm(scopeContents, permWrite, s.handleCreateRelease))
	s.route("GET /api/v3/repos/{owner}/{repo}/releases",
		s.handleListReleases)
	s.route("GET /api/v3/repos/{owner}/{repo}/releases/latest",
		s.handleGetLatestRelease)
	s.route("POST /api/v3/repos/{owner}/{repo}/releases/generate-notes",
		s.requirePerm(scopeContents, permWrite, s.handleGenerateReleaseNotes))

	// Single-segment after /releases/ is GET-release-by-id. Use {release_id}
	// directly here — these patterns don't conflict with the two-segment ones.
	s.route("GET /api/v3/repos/{owner}/{repo}/releases/{release_id}",
		s.handleGetRelease)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/releases/{release_id}",
		s.requirePerm(scopeContents, permWrite, s.handleUpdateRelease))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/releases/{release_id}",
		s.requirePerm(scopeContents, permWrite, s.handleDeleteRelease))

	// `/releases/{p1}/{p2}` dispatches by segment value:
	//   p1=="tags"      → GET release-by-tag (real GH path: releases/tags/{tag})
	//   p1==numeric     → reactions on release {p1} when p2 == "reactions"
	// Go 1.22's mux refuses to register the two distinct patterns directly,
	// so a single dispatcher handles both real-GH paths.
	s.route("GET /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}",
		s.handleReleaseTwoSegDispatch("GET"))
	s.route("POST /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}",
		s.handleReleaseTwoSegDispatch("POST"))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}/{p3}",
		s.handleReleaseThreeSegDispatch("DELETE"))
}

// handleReleaseTwoSegDispatch resolves either
//
//	GET /releases/tags/{tag}           (real-GH path)
//	GET|POST /releases/{release_id}/reactions
func (s *Server) handleReleaseTwoSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		p2 := r.PathValue("p2")
		switch {
		case p1 == "tags" && method == "GET":
			// Stash the tag back into the {tag} slot via r.SetPathValue.
			r.SetPathValue("tag", p2)
			s.handleGetReleaseByTag(w, r)
		case p2 == "reactions":
			r.SetPathValue("release_id", p1)
			switch method {
			case "GET":
				s.handleListReactions("release", "release_id")(w, r)
			case "POST":
				s.requirePerm(scopeContents, permWrite, s.handleCreateReaction("release", "release_id"))(w, r)
			}
		default:
			writeGHError(w, http.StatusNotFound, "Not Found")
		}
	}
}

// handleReleaseThreeSegDispatch resolves
//
//	DELETE /releases/{release_id}/reactions/{reaction_id}
func (s *Server) handleReleaseThreeSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		p2 := r.PathValue("p2")
		p3 := r.PathValue("p3")
		if p2 == "reactions" && method == "DELETE" {
			r.SetPathValue("release_id", p1)
			r.SetPathValue("reaction_id", p3)
			s.requirePerm(scopeContents, permWrite, s.handleDeleteReaction("release", "release_id"))(w, r)
			return
		}
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

func (s *Server) lookupRepoFromPath(r *http.Request) *Repo {
	return s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
}

// lookupReadableRepoFromPath resolves the {owner}/{repo} path the same as
// lookupRepoFromPath, but additionally enforces private-repo visibility: a
// private repo the caller cannot read returns nil and a 404, matching real
// GitHub (which hides the existence of private repos behind 404, never 403, on
// read paths). Use this on every GET handler that returns repo-scoped content;
// lookupRepoFromPath stays for write handlers already gated by requirePerm.
func (s *Server) lookupReadableRepoFromPath(w http.ResponseWriter, r *http.Request) *Repo {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return repo
}

// enforceRepoReadable applies the private-repo visibility gate without
// requiring that a Repo record exist. It returns false (after writing a 404)
// only when the path resolves to a KNOWN private repo the caller cannot read.
// Paths whose repo has no Store record (e.g. workflow-run state tracked by
// RepoFullName alone) are allowed through unchanged — those handlers carry
// their own not-found semantics. Use this on repo-scoped read handlers that
// must not leak private-repo content but operate on non-Repo-keyed state.
func (s *Server) enforceRepoReadable(w http.ResponseWriter, r *http.Request) bool {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo != nil && repo.Private && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return false
	}
	return true
}

func (s *Server) handleCreateRelease(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		TagName         string   `json:"tag_name"`
		TargetCommitish string   `json:"target_commitish"`
		Name            string   `json:"name"`
		Body            string   `json:"body"`
		Draft           flexBool `json:"draft"`
		Prerelease      flexBool `json:"prerelease"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.TagName == "" {
		writeGHValidationError(w, "Release", "tag_name", "missing_field")
		return
	}
	target := req.TargetCommitish
	if target == "" {
		target = repo.DefaultBranch
	}
	release := s.store.Releases.Create(repo.ID, user.ID, req.TagName, target, req.Name, req.Body, bool(req.Draft), bool(req.Prerelease))
	s.emitWebhookEvent(repo.FullName, "release", "published", buildReleaseEventPayload(repo, release, user, "published"))
	s.recordAuditEvent("release.create", user.Login, "", map[string]interface{}{"repo": repo.FullName, "release_id": release.ID, "tag": release.TagName})
	writeJSON(w, http.StatusCreated, releaseToJSON(release, s.store, s.baseURL(r), repo))
}

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	releases := s.store.Releases.List(repo.ID)
	page := paginateAndLink(w, r, releases)
	out := make([]map[string]interface{}, 0, len(page))
	for _, rel := range page {
		out = append(out, releaseToJSON(rel, s.store, s.baseURL(r), repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetLatestRelease(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	rel := s.store.Releases.Latest(repo.ID)
	if rel == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, releaseToJSON(rel, s.store, s.baseURL(r), repo))
}

func (s *Server) handleGetReleaseByTag(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	rel := s.store.Releases.GetByTag(repo.ID, r.PathValue("tag"))
	if rel == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, releaseToJSON(rel, s.store, s.baseURL(r), repo))
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("release_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(id)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, releaseToJSON(rel, s.store, s.baseURL(r), repo))
}

func (s *Server) handleUpdateRelease(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("release_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		TagName         *string   `json:"tag_name"`
		TargetCommitish *string   `json:"target_commitish"`
		Name            *string   `json:"name"`
		Body            *string   `json:"body"`
		Draft           *flexBool `json:"draft"`
		Prerelease      *flexBool `json:"prerelease"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ok := s.store.Releases.Update(id, func(rel *Release) {
		if rel.RepoID != repo.ID {
			return
		}
		if req.TagName != nil {
			rel.TagName = *req.TagName
		}
		if req.TargetCommitish != nil {
			rel.TargetCommitish = *req.TargetCommitish
		}
		if req.Name != nil {
			rel.Name = *req.Name
		}
		if req.Body != nil {
			rel.Body = *req.Body
		}
		if req.Draft != nil {
			rel.Draft = bool(*req.Draft)
		}
		if req.Prerelease != nil {
			rel.Prerelease = bool(*req.Prerelease)
		}
	})
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, releaseToJSON(s.store.Releases.Get(id), s.store, s.baseURL(r), repo))
}

func (s *Server) handleDeleteRelease(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("release_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(id)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Releases.Delete(id)
	s.recordAuditEvent("release.destroy", user.Login, "", map[string]interface{}{"repo": repo.FullName, "release_id": id})
	w.WriteHeader(http.StatusNoContent)
}

// handleGenerateReleaseNotes — sim returns a deterministic body. Real GH
// summarises merged PRs since the previous tag; bleephub doesn't model
// commit ranges deeply, so the body lists the previous tag + a placeholder.
func (s *Server) handleGenerateReleaseNotes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TagName           string `json:"tag_name"`
		TargetCommitish   string `json:"target_commitish"`
		PreviousTagName   string `json:"previous_tag_name"`
		ConfigurationFile string `json:"configuration_file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TagName == "" {
		writeGHValidationError(w, "Release", "tag_name", "missing_field")
		return
	}
	out := map[string]interface{}{
		"name": req.TagName,
		"body": fmt.Sprintf("## What's Changed\n\nSince %s\n\n**Full Changelog**: %s...%s",
			coalesceStr(req.PreviousTagName, "(no previous tag)"),
			coalesceStr(req.PreviousTagName, ""),
			req.TagName),
	}
	writeJSON(w, http.StatusOK, out)
}

func releaseToJSON(rel *Release, st *Store, baseURL string, repo *Repo) map[string]interface{} {
	if rel == nil {
		return nil
	}
	var author map[string]interface{}
	st.mu.RLock()
	if u := st.Users[rel.AuthorID]; u != nil {
		author = userToJSON(u)
	}
	st.mu.RUnlock()
	publishedAt := interface{}(nil)
	if rel.PublishedAt != nil {
		publishedAt = rel.PublishedAt.UTC().Format(time.RFC3339)
	}
	reactions := st.Reactions.SummarizeReactions("release", rel.ID)
	reactions["url"] = fmt.Sprintf("%s/api/v3/repos/%s/releases/%d/reactions", baseURL, repo.FullName, rel.ID)
	return map[string]interface{}{
		"id":               rel.ID,
		"node_id":          rel.NodeID,
		"tag_name":         rel.TagName,
		"target_commitish": rel.TargetCommitish,
		"name":             rel.Name,
		"body":             rel.Body,
		"draft":            rel.Draft,
		"prerelease":       rel.Prerelease,
		"author":           author,
		"created_at":       rel.CreatedAt.UTC().Format(time.RFC3339),
		"published_at":     publishedAt,
		"url":              fmt.Sprintf("%s/api/v3/repos/%s/releases/%d", baseURL, repo.FullName, rel.ID),
		"html_url":         fmt.Sprintf("%s/%s/releases/tag/%s", baseURL, repo.FullName, rel.TagName),
		"assets_url":       fmt.Sprintf("%s/api/v3/repos/%s/releases/%d/assets", baseURL, repo.FullName, rel.ID),
		"upload_url":       fmt.Sprintf("%s/api/uploads/repos/%s/releases/%d/assets{?name,label}", baseURL, repo.FullName, rel.ID),
		"tarball_url":      fmt.Sprintf("%s/api/v3/repos/%s/tarball/%s", baseURL, repo.FullName, rel.TagName),
		"zipball_url":      fmt.Sprintf("%s/api/v3/repos/%s/zipball/%s", baseURL, repo.FullName, rel.TagName),
		"assets":           []interface{}{},
		"reactions":        reactions,
	}
}

// buildReleaseEventPayload — `release` webhook event payload.
func buildReleaseEventPayload(repo *Repo, rel *Release, sender *User, action string) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"action": action,
		"release": map[string]interface{}{
			"id":         rel.ID,
			"tag_name":   rel.TagName,
			"name":       rel.Name,
			"body":       rel.Body,
			"draft":      rel.Draft,
			"prerelease": rel.Prerelease,
			"created_at": rel.CreatedAt.UTC().Format(time.RFC3339),
		},
		"repository": repoPayload(repo),
		"sender":     senderPayload(sender),
	}, nil)
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
