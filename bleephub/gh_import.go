package bleephub

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	git "github.com/go-git/go-git/v5"
	gitConfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitHTTP "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// Source Import API (sunset on github.com, still real on GitHub Enterprise
// Server).
// Endpoints:
//
//	GET/PUT/PATCH/DELETE /repos/{o}/{r}/import
//	PATCH /repos/{o}/{r}/import/lfs
//	GET   /repos/{o}/{r}/import/authors
//	PATCH /repos/{o}/{r}/import/authors/{author_id}
//	GET   /repos/{o}/{r}/import/large_files
//
// The import is a real git fetch: PUT (and PATCH restarts) fetch vcs_url's
// refs into the repository's git storage over any transport go-git speaks,
// synchronously. The status field reflects what actually happened —
// "complete" only after a successful fetch, "auth_failed"/"error" with the
// transport's failure otherwise, and "error" with an explanatory message
// for VCS types bleephub cannot really import (everything but git).
type RepoImport struct {
	RepoID          int             `json:"repo_id"`
	VCS             string          `json:"vcs"` // empty until detected/declared
	VCSURL          string          `json:"vcs_url"`
	VCSUsername     string          `json:"vcs_username,omitempty"`
	VCSPassword     string          `json:"vcs_password,omitempty"`
	TFVCProject     string          `json:"tfvc_project,omitempty"`
	Status          string          `json:"status"`
	StatusText      string          `json:"status_text,omitempty"`
	FailedStep      string          `json:"failed_step,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	ImportPercent   *int            `json:"import_percent"`
	CommitCount     *int            `json:"commit_count"`
	AuthorsCount    *int            `json:"authors_count"`
	UseLFS          bool            `json:"use_lfs"`
	HasLargeFiles   bool            `json:"has_large_files"`
	LargeFilesSize  int             `json:"large_files_size"`
	LargeFilesCount int             `json:"large_files_count"`
	Authors         []*PorterAuthor `json:"authors"`
	NextAuthorID    int             `json:"next_author_id"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// PorterAuthor is one distinct commit author discovered by the import.
type PorterAuthor struct {
	ID         int    `json:"id"`
	RemoteID   string `json:"remote_id"`
	RemoteName string `json:"remote_name"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}

// PorterLargeFile is a blob over the 100 MB large-file threshold found in
// the imported repository.
type PorterLargeFile struct {
	RefName string
	Path    string
	OID     string
	Size    int
}

const porterLargeFileThreshold = 100 * 1024 * 1024

func (s *Server) registerGHImportRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/import", s.handleGetImport)
	s.route("PUT /api/v3/repos/{owner}/{repo}/import",
		s.requirePerm(scopeAdministration, permWrite, s.handleStartImport))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/import",
		s.requirePerm(scopeAdministration, permWrite, s.handleUpdateImport))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/import",
		s.requirePerm(scopeAdministration, permWrite, s.handleCancelImport))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/import/lfs",
		s.requirePerm(scopeAdministration, permWrite, s.handleSetImportLFS))
	s.route("GET /api/v3/repos/{owner}/{repo}/import/authors", s.handleListImportAuthors)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/import/authors/{author_id}",
		s.requirePerm(scopeAdministration, permWrite, s.handleUpdateImportAuthor))
	s.route("GET /api/v3/repos/{owner}/{repo}/import/large_files", s.handleListImportLargeFiles)
}

// --- Store ---

// PutRepoImport stores (creating or replacing) the repo's import record.
func (st *Store) PutRepoImport(imp *RepoImport) {
	st.mu.Lock()
	defer st.mu.Unlock()
	imp.UpdatedAt = time.Now().UTC()
	st.RepoImports[imp.RepoID] = imp
	if st.persist != nil {
		st.persist.MustPut("repo_imports", strconv.Itoa(imp.RepoID), imp)
	}
}

// GetRepoImport returns the repo's import record, or nil.
func (st *Store) GetRepoImport(repoID int) *RepoImport {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.RepoImports[repoID]
}

// DeleteRepoImport removes the repo's import record. Returns true if it existed.
func (st *Store) DeleteRepoImport(repoID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.RepoImports[repoID]; !ok {
		return false
	}
	delete(st.RepoImports, repoID)
	if st.persist != nil {
		st.persist.MustDelete("repo_imports", strconv.Itoa(repoID))
	}
	return true
}

// --- Handlers ---

func (s *Server) importToJSON(imp *RepoImport, repo *Repo, baseURL string) map[string]interface{} {
	apiBase := baseURL + "/api/v3/repos/" + repo.FullName
	var vcs interface{}
	if imp.VCS != "" {
		vcs = imp.VCS
	}
	out := map[string]interface{}{
		"vcs":               vcs,
		"use_lfs":           imp.UseLFS,
		"vcs_url":           imp.VCSURL,
		"status":            imp.Status,
		"url":               apiBase + "/import",
		"html_url":          baseURL + "/" + repo.FullName + "/import",
		"authors_url":       apiBase + "/import/authors",
		"repository_url":    apiBase,
		"status_text":       nullableString(imp.StatusText),
		"failed_step":       nullableString(imp.FailedStep),
		"error_message":     nullableString(imp.ErrorMessage),
		"has_large_files":   imp.HasLargeFiles,
		"large_files_size":  imp.LargeFilesSize,
		"large_files_count": imp.LargeFilesCount,
	}
	if imp.TFVCProject != "" {
		out["tfvc_project"] = imp.TFVCProject
	}
	if imp.ImportPercent != nil {
		out["import_percent"] = *imp.ImportPercent
	} else {
		out["import_percent"] = nil
	}
	if imp.CommitCount != nil {
		out["commit_count"] = *imp.CommitCount
	} else {
		out["commit_count"] = nil
	}
	if imp.AuthorsCount != nil {
		out["authors_count"] = *imp.AuthorsCount
	} else {
		out["authors_count"] = nil
	}
	return out
}

func porterAuthorToJSON(a *PorterAuthor, repo *Repo, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"id":          a.ID,
		"remote_id":   a.RemoteID,
		"remote_name": a.RemoteName,
		"email":       a.Email,
		"name":        a.Name,
		"url":         baseURL + "/api/v3/repos/" + repo.FullName + "/import/authors/" + strconv.Itoa(a.ID),
		"import_url":  baseURL + "/api/v3/repos/" + repo.FullName + "/import",
	}
}

func (s *Server) handleGetImport(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	imp := s.store.GetRepoImport(repo.ID)
	if imp == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.importToJSON(imp, repo, s.baseURL(r)))
}

func (s *Server) handleStartImport(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		VCSURL      string `json:"vcs_url"`
		VCS         string `json:"vcs"`
		VCSUsername string `json:"vcs_username"`
		VCSPassword string `json:"vcs_password"`
		TFVCProject string `json:"tfvc_project"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.VCSURL == "" {
		writeGHValidationError(w, "Import", "vcs_url", "missing_field")
		return
	}
	imp := &RepoImport{
		RepoID:       repo.ID,
		VCS:          req.VCS,
		VCSURL:       req.VCSURL,
		VCSUsername:  req.VCSUsername,
		VCSPassword:  req.VCSPassword,
		TFVCProject:  req.TFVCProject,
		NextAuthorID: 1,
		CreatedAt:    time.Now().UTC(),
	}
	s.runRepoImport(imp, repo)
	s.store.PutRepoImport(imp)
	writeJSON(w, http.StatusCreated, s.importToJSON(imp, repo, s.baseURL(r)))
}

func (s *Server) handleUpdateImport(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	imp := s.store.GetRepoImport(repo.ID)
	if imp == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		VCSUsername string `json:"vcs_username"`
		VCSPassword string `json:"vcs_password"`
		VCS         string `json:"vcs"`
		TFVCProject string `json:"tfvc_project"`
	}
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	if req.VCSUsername != "" {
		imp.VCSUsername = req.VCSUsername
	}
	if req.VCSPassword != "" {
		imp.VCSPassword = req.VCSPassword
	}
	if req.VCS != "" {
		imp.VCS = req.VCS
	}
	if req.TFVCProject != "" {
		imp.TFVCProject = req.TFVCProject
	}
	// PATCH restarts a stalled import with the updated parameters.
	s.runRepoImport(imp, repo)
	s.store.PutRepoImport(imp)
	writeJSON(w, http.StatusOK, s.importToJSON(imp, repo, s.baseURL(r)))
}

func (s *Server) handleCancelImport(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteRepoImport(repo.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetImportLFS(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	imp := s.store.GetRepoImport(repo.ID)
	if imp == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		UseLFS string `json:"use_lfs"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.UseLFS {
	case "opt_in":
		imp.UseLFS = true
	case "opt_out":
		imp.UseLFS = false
	default:
		writeGHValidationError(w, "Import", "use_lfs", "invalid")
		return
	}
	s.store.PutRepoImport(imp)
	writeJSON(w, http.StatusOK, s.importToJSON(imp, repo, s.baseURL(r)))
}

func (s *Server) handleListImportAuthors(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	imp := s.store.GetRepoImport(repo.ID)
	if imp == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(imp.Authors))
	for _, a := range imp.Authors {
		out = append(out, porterAuthorToJSON(a, repo, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateImportAuthor(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	imp := s.store.GetRepoImport(repo.ID)
	if imp == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("author_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	for _, a := range imp.Authors {
		if a.ID == id {
			if req.Email != "" {
				a.Email = req.Email
			}
			if req.Name != "" {
				a.Name = req.Name
			}
			s.store.PutRepoImport(imp)
			writeJSON(w, http.StatusOK, porterAuthorToJSON(a, repo, s.baseURL(r)))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleListImportLargeFiles(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	imp := s.store.GetRepoImport(repo.ID)
	if imp == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	owner, name, _ := splitRepoFullName(repo.FullName)
	files := importLargeFiles(s.store.GetGitStorage(owner, name), repo.DefaultBranch)
	out := make([]map[string]interface{}, 0, len(files))
	for _, f := range files {
		out = append(out, map[string]interface{}{
			"ref_name": f.RefName,
			"path":     f.Path,
			"oid":      f.OID,
			"size":     f.Size,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Import execution ---

// runRepoImport performs the import synchronously and records the honest
// outcome on imp. Only git sources can really be imported; other VCS types
// end in status "error" saying so.
func (s *Server) runRepoImport(imp *RepoImport, repo *Repo) {
	imp.StatusText = ""
	imp.FailedStep = ""
	imp.ErrorMessage = ""

	if imp.VCS != "" && imp.VCS != "git" {
		imp.Status = "error"
		imp.FailedStep = "importing"
		imp.ErrorMessage = fmt.Sprintf("bleephub can only import git repositories; %q imports are not supported", imp.VCS)
		return
	}

	owner, name, ok := splitRepoFullName(repo.FullName)
	if !ok {
		imp.Status = "error"
		imp.ErrorMessage = "invalid repository name"
		return
	}
	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		imp.Status = "error"
		imp.ErrorMessage = "repository git storage unavailable"
		return
	}

	var auth transport.AuthMethod
	if imp.VCSUsername != "" || imp.VCSPassword != "" {
		auth = &gitHTTP.BasicAuth{Username: imp.VCSUsername, Password: imp.VCSPassword}
	}

	remote := git.NewRemote(stor, &gitConfig.RemoteConfig{
		Name: "bleephub-import",
		URLs: []string{imp.VCSURL},
		Fetch: []gitConfig.RefSpec{
			"+refs/heads/*:refs/heads/*",
			"+refs/tags/*:refs/tags/*",
		},
	})
	err := remote.Fetch(&git.FetchOptions{
		Auth:  auth,
		Force: true,
		Tags:  git.AllTags,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		switch {
		case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed):
			imp.Status = "auth_failed"
			imp.FailedStep = "importing"
			imp.ErrorMessage = err.Error()
		case errors.Is(err, transport.ErrRepositoryNotFound), errors.Is(err, transport.ErrEmptyRemoteRepository):
			imp.Status = "detection_found_nothing"
			imp.FailedStep = "detecting"
			imp.ErrorMessage = err.Error()
		default:
			imp.Status = "error"
			imp.FailedStep = "importing"
			imp.ErrorMessage = err.Error()
		}
		return
	}

	imp.VCS = "git"
	pointHEADAtImportedBranch(stor, repo.DefaultBranch)

	commitCount, authors := importedCommitStats(stor, imp)
	imp.CommitCount = &commitCount
	authorsCount := len(authors)
	imp.AuthorsCount = &authorsCount
	imp.Authors = authors

	largeFiles := importLargeFiles(stor, repo.DefaultBranch)
	imp.HasLargeFiles = len(largeFiles) > 0
	imp.LargeFilesCount = len(largeFiles)
	total := 0
	for _, f := range largeFiles {
		total += f.Size
	}
	imp.LargeFilesSize = total

	hundred := 100
	imp.ImportPercent = &hundred
	imp.Status = "complete"

	s.store.UpdateRepo(owner, name, func(rp *Repo) {
		rp.PushedAt = time.Now().UTC()
	})
}

// pointHEADAtImportedBranch makes HEAD resolve to a fetched branch,
// preferring the repo's configured default branch, then main, then master,
// then the alphabetically first branch.
func pointHEADAtImportedBranch(stor gitStorage.Storer, defaultBranch string) {
	var names []string
	iter, err := stor.IterReferences()
	if err != nil {
		return
	}
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() && ref.Type() == plumbing.HashReference {
			names = append(names, ref.Name().Short())
		}
		return nil
	})
	iter.Close()
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	pick := ""
	for _, candidate := range []string{defaultBranch, "main", "master"} {
		for _, n := range names {
			if n == candidate {
				pick = n
				break
			}
		}
		if pick != "" {
			break
		}
	}
	if pick == "" {
		pick = names[0]
	}
	_ = stor.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(pick)))
}

// importedCommitStats counts commits and collects the distinct authors in
// the imported storage.
func importedCommitStats(stor gitStorage.Storer, imp *RepoImport) (int, []*PorterAuthor) {
	count := 0
	seen := map[string]*PorterAuthor{}
	// Preserve author IDs (and any name/email remapping) across restarts.
	for _, a := range imp.Authors {
		seen[a.RemoteID] = a
	}
	if imp.NextAuthorID == 0 {
		imp.NextAuthorID = 1
	}
	authors := append([]*PorterAuthor(nil), imp.Authors...)

	iter, err := stor.IterEncodedObjects(plumbing.CommitObject)
	if err != nil {
		return 0, authors
	}
	defer iter.Close()
	_ = iter.ForEach(func(obj plumbing.EncodedObject) error {
		commit, err := object.DecodeCommit(stor, obj)
		if err != nil {
			return nil
		}
		count++
		remoteID := commit.Author.Name + " <" + commit.Author.Email + ">"
		if _, ok := seen[remoteID]; !ok {
			a := &PorterAuthor{
				ID:         imp.NextAuthorID,
				RemoteID:   remoteID,
				RemoteName: commit.Author.Name,
				Email:      commit.Author.Email,
				Name:       commit.Author.Name,
			}
			imp.NextAuthorID++
			seen[remoteID] = a
			authors = append(authors, a)
		}
		return nil
	})
	sort.Slice(authors, func(i, j int) bool { return authors[i].ID < authors[j].ID })
	return count, authors
}

// importLargeFiles finds blobs over the 100 MB threshold reachable from the
// default branch's tree, reported against their paths.
func importLargeFiles(stor gitStorage.Storer, defaultBranch string) []*PorterLargeFile {
	if stor == nil {
		return nil
	}
	sha := resolveBranchSha(stor, defaultBranch)
	if sha == "" {
		return nil
	}
	commit, err := object.GetCommit(stor, plumbing.NewHash(sha))
	if err != nil {
		return nil
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil
	}
	var out []*PorterLargeFile
	walker := object.NewTreeWalker(tree, true, nil)
	defer walker.Close()
	for {
		path, entry, err := walker.Next()
		if err != nil {
			break
		}
		if !entry.Mode.IsFile() {
			continue
		}
		blob, err := object.GetBlob(stor, entry.Hash)
		if err != nil {
			continue
		}
		if blob.Size >= porterLargeFileThreshold {
			out = append(out, &PorterLargeFile{
				RefName: "refs/heads/" + defaultBranch,
				Path:    path,
				OID:     entry.Hash.String(),
				Size:    int(blob.Size),
			})
		}
	}
	return out
}
