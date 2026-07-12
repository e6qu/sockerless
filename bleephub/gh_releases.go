package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
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
	mu           sync.RWMutex
	byID         map[int]*Release
	byRepo       map[int][]*Release
	assetByID    map[int]*ReleaseAsset
	assetData    map[int][]byte
	assetDataDir string
	byteStore    actionsByteStore
	nextID       int
	nextAsset    int
	persist      *Persistence
}

func newReleaseStore(p *Persistence) *ReleaseStore {
	rs := &ReleaseStore{
		byID:      map[int]*Release{},
		byRepo:    map[int][]*Release{},
		assetByID: map[int]*ReleaseAsset{},
		assetData: map[int][]byte{},
		nextID:    1,
		nextAsset: 1,
		persist:   p,
	}
	if d := os.Getenv("BLEEPHUB_DATA_DIR"); d != "" {
		rs.assetDataDir = filepath.Join(d, "release_assets")
	}
	return rs
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
func (rs *ReleaseStore) DeleteAllForRepo(repoID int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.byRepo[repoID] {
		if err := rs.deleteReleaseLocked(r); err != nil {
			return err
		}
	}
	delete(rs.byRepo, repoID)
	return nil
}

func (rs *ReleaseStore) IDsForRepo(repoID int) map[int]bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	ids := make(map[int]bool, len(rs.byRepo[repoID]))
	for _, r := range rs.byRepo[repoID] {
		ids[r.ID] = true
	}
	return ids
}

func (rs *ReleaseStore) Delete(id int) (bool, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.byID[id]
	if r == nil {
		return false, nil
	}
	if err := rs.deleteReleaseLocked(r); err != nil {
		return true, err
	}
	src := rs.byRepo[r.RepoID]
	for i, x := range src {
		if x.ID == id {
			rs.byRepo[r.RepoID] = append(src[:i], src[i+1:]...)
			break
		}
	}
	return true, nil
}

// --- Asset methods ---

func (rs *ReleaseStore) assetFilePath(id int) string {
	if rs.assetDataDir == "" {
		return ""
	}
	return filepath.Join(rs.assetDataDir, strconv.Itoa(id))
}

func (rs *ReleaseStore) saveAssetDataLocked(id int, data []byte) error {
	if rs.byteStore != nil {
		return rs.byteStore.Put(context.Background(), releaseAssetDataKey(id), data)
	}
	if rs.assetDataDir != "" {
		if err := os.MkdirAll(rs.assetDataDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(rs.assetFilePath(id), data, 0o644)
	}
	rs.assetData[id] = data
	return nil
}

func (rs *ReleaseStore) loadAssetDataLocked(id int) ([]byte, bool) {
	if rs.byteStore != nil {
		data, err := rs.byteStore.Get(context.Background(), releaseAssetDataKey(id))
		if err != nil {
			return nil, false
		}
		return data, true
	}
	if rs.assetDataDir != "" {
		path := rs.assetFilePath(id)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false
		}
		return data, true
	}
	data, ok := rs.assetData[id]
	return data, ok
}

func (rs *ReleaseStore) removeAssetDataLocked(id int) error {
	if rs.byteStore != nil {
		return rs.byteStore.Delete(context.Background(), releaseAssetDataKey(id))
	}
	if rs.assetDataDir != "" {
		if err := os.Remove(rs.assetFilePath(id)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	delete(rs.assetData, id)
	return nil
}

func (rs *ReleaseStore) deleteAssetLocked(a *ReleaseAsset) error {
	if err := rs.removeAssetDataLocked(a.ID); err != nil {
		return err
	}
	delete(rs.assetByID, a.ID)
	if rel := rs.byID[a.ReleaseID]; rel != nil {
		src := rel.Assets
		for i, x := range src {
			if x.ID == a.ID {
				rel.Assets = append(src[:i], src[i+1:]...)
				break
			}
		}
	}
	if rs.persist != nil {
		rs.persist.MustDelete("release_assets", strconv.Itoa(a.ID))
	}
	return nil
}

func (rs *ReleaseStore) deleteReleaseLocked(r *Release) error {
	for _, a := range r.Assets {
		if err := rs.deleteAssetLocked(a); err != nil {
			return err
		}
	}
	delete(rs.byID, r.ID)
	if rs.persist != nil {
		rs.persist.MustDelete("releases", strconv.Itoa(r.ID))
	}
	return nil
}

func (rs *ReleaseStore) CreateReleaseAsset(releaseID, uploaderID int, name, label, contentType string, data []byte) (*ReleaseAsset, error) {
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.byID[releaseID] == nil {
		return nil, fmt.Errorf("release not found")
	}
	now := time.Now().UTC()
	id := rs.nextAsset
	rs.nextAsset++
	asset := &ReleaseAsset{
		ID:          id,
		NodeID:      fmt.Sprintf("RA_kgDO%08d", id),
		ReleaseID:   releaseID,
		Name:        name,
		Label:       label,
		State:       "uploaded",
		ContentType: contentType,
		Size:        len(data),
		UploaderID:  uploaderID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	rs.assetByID[id] = asset
	rs.byID[releaseID].Assets = append(rs.byID[releaseID].Assets, asset)
	if err := rs.saveAssetDataLocked(id, data); err != nil {
		// Rollback maps and disk on write failure.
		_ = rs.deleteAssetLocked(asset)
		return nil, err
	}
	if rs.persist != nil {
		rs.persist.MustPut("release_assets", strconv.Itoa(id), asset)
	}
	return asset, nil
}

func (rs *ReleaseStore) GetReleaseAsset(id int) *ReleaseAsset {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.assetByID[id]
}

func (rs *ReleaseStore) GetAssetData(id int) ([]byte, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.loadAssetDataLocked(id)
}

func (rs *ReleaseStore) UpdateReleaseAsset(id int, name, label string) *ReleaseAsset {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	a := rs.assetByID[id]
	if a == nil {
		return nil
	}
	if name != "" {
		a.Name = name
	}
	a.Label = label
	a.UpdatedAt = time.Now().UTC()
	if rs.persist != nil {
		rs.persist.MustPut("release_assets", strconv.Itoa(id), a)
	}
	return a
}

func (rs *ReleaseStore) DeleteReleaseAsset(id int) (bool, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	a := rs.assetByID[id]
	if a == nil {
		return false, nil
	}
	if err := rs.deleteAssetLocked(a); err != nil {
		return true, err
	}
	return true, nil
}

func (rs *ReleaseStore) ListReleaseAssets(releaseID int) []*ReleaseAsset {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	rel := rs.byID[releaseID]
	if rel == nil {
		return nil
	}
	out := make([]*ReleaseAsset, len(rel.Assets))
	copy(out, rel.Assets)
	return out
}

func (rs *ReleaseStore) IncrementAssetDownloads(id int) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	a := rs.assetByID[id]
	if a == nil {
		return false
	}
	a.DownloadCount++
	if rs.persist != nil {
		rs.persist.MustPut("release_assets", strconv.Itoa(id), a)
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
	//   p1=="assets"    → GET/PATCH/DELETE asset by id
	//   p2=="assets"    → POST upload / GET list assets for release {p1}
	//   p2=="reactions" → GET/POST reactions on release {p1}
	// Go 1.22's mux refuses to register the two distinct patterns directly,
	// so a single dispatcher handles all real-GH paths under this prefix.
	s.route("GET /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}",
		s.handleReleaseTwoSegDispatch("GET"))
	s.route("POST /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}",
		s.handleReleaseTwoSegDispatch("POST"))
	s.route("POST /api/uploads/repos/{owner}/{repo}/releases/{release_id}/assets",
		s.requirePerm(scopeContents, permWrite, s.handleUploadReleaseAsset))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}",
		s.handleReleaseTwoSegDispatch("PATCH"))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}",
		s.handleReleaseTwoSegDispatch("DELETE"))

	// `/releases/{p1}/{p2}/{p3}` is only used for release-reaction deletion
	// (reactions have a three-segment path while assets stop at two).
	s.route("DELETE /api/v3/repos/{owner}/{repo}/releases/{p1}/{p2}/{p3}",
		s.handleReleaseThreeSegDispatch("DELETE"))
}

// handleReleaseTwoSegDispatch resolves
//
//	GET /releases/tags/{tag}
//	GET/PATCH/DELETE /releases/assets/{asset_id}
//	GET/POST /releases/{release_id}/assets
//	GET/POST /releases/{release_id}/reactions
func (s *Server) handleReleaseTwoSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		p2 := r.PathValue("p2")
		switch {
		case p1 == "tags" && method == "GET":
			// Stash the tag back into the {tag} slot via r.SetPathValue.
			r.SetPathValue("tag", p2)
			s.handleGetReleaseByTag(w, r)
		case p1 == "assets":
			r.SetPathValue("asset_id", p2)
			switch method {
			case "GET":
				s.handleGetReleaseAsset(w, r)
			case "PATCH":
				s.requirePerm(scopeContents, permWrite, s.handleUpdateReleaseAsset)(w, r)
			case "DELETE":
				s.requirePerm(scopeContents, permWrite, s.handleDeleteReleaseAsset)(w, r)
			default:
				writeGHError(w, http.StatusNotFound, "Not Found")
			}
		case p2 == "assets":
			r.SetPathValue("release_id", p1)
			switch method {
			case "GET":
				s.handleListReleaseAssets(w, r)
			case "POST":
				s.requirePerm(scopeContents, permWrite, s.handleUploadReleaseAsset)(w, r)
			default:
				writeGHError(w, http.StatusNotFound, "Not Found")
			}
		case p2 == "reactions":
			r.SetPathValue("release_id", p1)
			switch method {
			case "GET":
				s.handleListReactions("release", "release_id")(w, r)
			case "POST":
				s.requirePerm(scopeReactions, permWrite, s.handleCreateReaction("release", "release_id"))(w, r)
			default:
				writeGHError(w, http.StatusNotFound, "Not Found")
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
			s.requirePerm(scopeReactions, permWrite, s.handleDeleteReaction("release", "release_id"))(w, r)
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
	if s.store.Releases.GetByTag(repo.ID, req.TagName) != nil {
		writeGHValidationError(w, "Release", "tag_name", "already_exists")
		return
	}
	target := req.TargetCommitish
	if target == "" {
		target = repo.DefaultBranch
	}
	if err := s.ensureReleaseTag(repo, req.TagName, target); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	release := s.store.Releases.Create(repo.ID, user.ID, req.TagName, target, req.Name, req.Body, bool(req.Draft), bool(req.Prerelease))
	s.emitReleaseEvent(repo, release, user, "created", s.baseURL(r))
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
	release := s.store.Releases.Get(id)
	if release == nil || release.RepoID != repo.ID {
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
	wasDraft := release.Draft
	wasPrerelease := release.Prerelease
	if req.TagName != nil && *req.TagName != release.TagName {
		if *req.TagName == "" {
			writeGHValidationError(w, "Release", "tag_name", "missing_field")
			return
		}
		if s.store.Releases.GetByTag(repo.ID, *req.TagName) != nil {
			writeGHValidationError(w, "Release", "tag_name", "already_exists")
			return
		}
		target := release.TargetCommitish
		if req.TargetCommitish != nil {
			target = *req.TargetCommitish
		}
		if err := s.ensureReleaseTag(repo, *req.TagName, target); err != nil {
			writeGHError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	} else if req.TargetCommitish != nil {
		if _, err := s.resolveReleaseTarget(repo, *req.TargetCommitish); err != nil {
			writeGHError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	ok := s.store.Releases.Update(id, func(rel *Release) {
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
	updated := s.store.Releases.Get(id)
	action := releaseUpdateAction(wasDraft, wasPrerelease, updated.Draft, updated.Prerelease)
	s.emitReleaseEvent(repo, updated, ghUserFromContext(r.Context()), action, s.baseURL(r))
	writeJSON(w, http.StatusOK, releaseToJSON(updated, s.store, s.baseURL(r), repo))
}

func (s *Server) resolveReleaseTarget(repo *Repo, target string) (plumbing.Hash, error) {
	stor, _ := s.store.GitStorageForRepoID(repo.ID)
	if stor == nil {
		return plumbing.ZeroHash, fmt.Errorf("release source git storage is unavailable")
	}
	hash, err := resolveGitRef(stor, target)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("target_commitish %q does not resolve to a commit", target)
	}
	return hash, nil
}

func (s *Server) ensureReleaseTag(repo *Repo, tagName, target string) error {
	stor, _ := s.store.GitStorageForRepoID(repo.ID)
	if stor == nil {
		return fmt.Errorf("release source git storage is unavailable")
	}
	tagRef := plumbing.NewTagReferenceName(tagName)
	if existing, err := stor.Reference(tagRef); err == nil {
		_, err = refHash(existing, stor)
		if err != nil {
			return fmt.Errorf("release tag %q does not resolve to a commit", tagName)
		}
		return nil
	}
	targetHash, err := s.resolveReleaseTarget(repo, target)
	if err != nil {
		return err
	}
	if err := stor.SetReference(plumbing.NewHashReference(tagRef, targetHash)); err != nil {
		return fmt.Errorf("create release tag %q: %w", tagName, err)
	}
	return nil
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
	payload := s.buildReleaseEventPayload(repo, rel, user, "deleted", s.baseURL(r))
	deleted, err := s.store.Releases.Delete(id)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "Failed to delete release")
		return
	}
	if !deleted {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.Reactions.DeleteParent("release", id)
	s.emitWebhookEvent(repo.FullName, "release", "deleted", payload)
	if !rel.Draft {
		s.triggerWorkflowsForEvent(repo.FullName, "release", "deleted", plumbing.NewTagReferenceName(rel.TagName).String(), payload)
	}
	s.recordAuditEvent("release.destroy", user.Login, "", map[string]interface{}{"repo": repo.FullName, "release_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func releaseUpdateAction(wasDraft, wasPrerelease, draft, prerelease bool) string {
	switch {
	case !wasDraft && draft:
		return "unpublished"
	case wasDraft && !draft:
		return "published"
	case !draft && !wasPrerelease && prerelease:
		return "prereleased"
	case !draft && wasPrerelease && !prerelease:
		return "released"
	default:
		return "edited"
	}
}

func (s *Server) emitReleaseEvent(repo *Repo, release *Release, sender *User, action, baseURL string) {
	payload := s.buildReleaseEventPayload(repo, release, sender, action, baseURL)
	s.emitWebhookEvent(repo.FullName, "release", action, payload)
	if release.Draft && (action == "created" || action == "edited" || action == "deleted") {
		return
	}
	s.triggerWorkflowsForEvent(repo.FullName, "release", action, plumbing.NewTagReferenceName(release.TagName).String(), payload)
}

func readUploadAssetBody(r *http.Request) (name, label, contentType string, data []byte, ok bool, err error) {
	q := r.URL.Query()
	name = q.Get("name")
	label = q.Get("label")
	contentType = r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if name == "" {
		return "", "", "", nil, false, nil
	}
	data, err = io.ReadAll(r.Body)
	if err != nil {
		return "", "", "", nil, false, err
	}
	return name, label, contentType, data, true, nil
}

func (s *Server) handleUploadReleaseAsset(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	releaseID, err := strconv.Atoi(r.PathValue("release_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(releaseID)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	name, label, contentType, data, ok, err := readUploadAssetBody(r)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Bad Request")
		return
	}
	if !ok {
		writeGHValidationError(w, "ReleaseAsset", "name", "missing_field")
		return
	}
	asset, err := s.store.Releases.CreateReleaseAsset(releaseID, user.ID, name, label, contentType, data)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusCreated, releaseAssetToJSON(asset, s.store, s.baseURL(r), repo, rel))
}

func (s *Server) handleListReleaseAssets(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	releaseID, err := strconv.Atoi(r.PathValue("release_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(releaseID)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	assets := s.store.Releases.ListReleaseAssets(releaseID)
	out := make([]map[string]interface{}, 0, len(assets))
	for _, a := range assets {
		out = append(out, releaseAssetToJSON(a, s.store, s.baseURL(r), repo, rel))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetReleaseAsset(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	assetID, err := strconv.Atoi(r.PathValue("asset_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	asset := s.store.Releases.GetReleaseAsset(assetID)
	if asset == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(asset.ReleaseID)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/octet-stream") {
		data, ok := s.store.Releases.GetAssetData(assetID)
		if !ok {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		s.store.Releases.IncrementAssetDownloads(assetID)
		w.Header().Set("Content-Type", asset.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	writeJSON(w, http.StatusOK, releaseAssetToJSON(asset, s.store, s.baseURL(r), repo, rel))
}

func (s *Server) handleUpdateReleaseAsset(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	assetID, err := strconv.Atoi(r.PathValue("asset_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	asset := s.store.Releases.GetReleaseAsset(assetID)
	if asset == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(asset.ReleaseID)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Name  *string `json:"name"`
		Label *string `json:"label"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	name := asset.Name
	label := asset.Label
	if req.Name != nil {
		name = *req.Name
	}
	if req.Label != nil {
		label = *req.Label
	}
	updated := s.store.Releases.UpdateReleaseAsset(assetID, name, label)
	writeJSON(w, http.StatusOK, releaseAssetToJSON(updated, s.store, s.baseURL(r), repo, rel))
}

func (s *Server) handleDeleteReleaseAsset(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	assetID, err := strconv.Atoi(r.PathValue("asset_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	asset := s.store.Releases.GetReleaseAsset(assetID)
	if asset == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rel := s.store.Releases.Get(asset.ReleaseID)
	if rel == nil || rel.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	deleted, err := s.store.Releases.DeleteReleaseAsset(assetID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "Failed to delete release asset")
		return
	}
	if !deleted {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func releaseAssetToJSON(asset *ReleaseAsset, st *Store, baseURL string, repo *Repo, rel *Release) map[string]interface{} {
	var uploader map[string]interface{}
	if user := st.Users[asset.UploaderID]; user != nil {
		uploader = userToJSON(user)
	}
	downloadURL := fmt.Sprintf("%s/api/v3/repos/%s/releases/assets/%d", baseURL, repo.FullName, asset.ID)
	return map[string]interface{}{
		"id":                   asset.ID,
		"node_id":              asset.NodeID,
		"name":                 asset.Name,
		"label":                asset.Label,
		"content_type":         asset.ContentType,
		"state":                asset.State,
		"size":                 asset.Size,
		"download_count":       asset.DownloadCount,
		"created_at":           asset.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":           asset.UpdatedAt.UTC().Format(time.RFC3339),
		"url":                  downloadURL,
		"browser_download_url": downloadURL,
		"uploader":             uploader,
	}
}

func (s *Server) handleGenerateReleaseNotes(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
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
	notes := s.generatedReleaseNotes(repo, req.TagName, req.TargetCommitish, req.PreviousTagName)
	out := map[string]interface{}{
		"name": req.TagName,
		"body": notes,
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) generatedReleaseNotes(repo *Repo, tagName, targetCommitish, previousTagName string) string {
	var lines []string
	lines = append(lines, "## What's Changed", "")
	for _, pr := range s.mergedPullRequestsInRange(repo, tagName, targetCommitish, previousTagName) {
		author := ""
		if u := s.store.GetUserByID(pr.AuthorID); u != nil {
			author = u.Login
		}
		if author == "" {
			author = "ghost"
		}
		lines = append(lines, fmt.Sprintf("* %s by @%s in %s/pull/%d", pr.Title, author, repo.FullName, pr.Number))
	}
	if len(lines) == 2 {
		lines = append(lines, fmt.Sprintf("Since %s", coalesceStr(previousTagName, "(no previous tag)")))
	}
	lines = append(lines, "", fmt.Sprintf("**Full Changelog**: %s...%s", coalesceStr(previousTagName, ""), tagName))
	return strings.Join(lines, "\n")
}

func (s *Server) mergedPullRequestsInRange(repo *Repo, tagName, targetCommitish, previousTagName string) []*PullRequest {
	owner, name, ok := splitRepoFullName(repo.FullName)
	if !ok {
		return nil
	}
	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		return nil
	}
	targetRef := tagName
	if targetCommitish != "" {
		targetRef = targetCommitish
	}
	targetHash, err := resolveGitRef(stor, targetRef)
	if err != nil && targetCommitish == "" {
		targetHash, err = resolveGitRef(stor, repo.DefaultBranch)
	}
	if err != nil {
		return nil
	}
	var previousHash plumbing.Hash
	if previousTagName != "" {
		if h, err := resolveGitRef(stor, previousTagName); err == nil {
			previousHash = h
		}
	}
	inRange := releaseCommitRangeSet(stor, previousHash, targetHash)
	if len(inRange) == 0 {
		return nil
	}

	var out []*PullRequest
	for _, pr := range s.store.ListPullRequests(repo.ID, "MERGED") {
		snap := s.store.snapPR(pr)
		if snap.MergeCommitSHA != "" && inRange[snap.MergeCommitSHA] {
			out = append(out, snap)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MergedAt != nil && out[j].MergedAt != nil && !out[i].MergedAt.Equal(*out[j].MergedAt) {
			return out[i].MergedAt.Before(*out[j].MergedAt)
		}
		return out[i].Number < out[j].Number
	})
	return out
}

func releaseCommitRangeSet(stor gitStorage.Storer, previousHash, targetHash plumbing.Hash) map[string]bool {
	out := map[string]bool{}
	var commits []*object.Commit
	var err error
	if previousHash.IsZero() {
		head, err := object.GetCommit(stor, targetHash)
		if err != nil {
			return out
		}
		iter := object.NewCommitPreorderIter(head, nil, nil)
		defer iter.Close()
		_ = iter.ForEach(func(c *object.Commit) error {
			commits = append(commits, c)
			return nil
		})
	} else {
		commits, err = commitsBetween(stor, previousHash, targetHash)
		if err != nil {
			return out
		}
	}
	for _, c := range commits {
		out[c.Hash.String()] = true
	}
	return out
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
	assets := make([]interface{}, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		assets = append(assets, releaseAssetToJSON(a, st, baseURL, repo, rel))
	}
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
		"assets":           assets,
		"reactions":        reactions,
	}
}

// buildReleaseEventPayload — `release` webhook event payload.
func (s *Server) buildReleaseEventPayload(repo *Repo, rel *Release, sender *User, action, baseURL string) map[string]interface{} {
	return attachInstallationBlock(map[string]interface{}{
		"action":     action,
		"release":    releaseToJSON(rel, s.store, baseURL, repo),
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
