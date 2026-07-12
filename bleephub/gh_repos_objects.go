package bleephub

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

var (
	errRepoGitStorageUnavailable = errors.New("repository git storage unavailable")
	errRepoGitRepositoryEmpty    = errors.New("repository git repository empty")
	errRepoGitObjectUnavailable  = errors.New("repository git object unavailable")
)

func (s *Server) registerGHRepoObjectRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/commits", s.handleListCommits)
	s.route("GET /api/v3/repos/{owner}/{repo}/readme", s.handleGetReadme)
	s.route("GET /api/v3/repos/{owner}/{repo}/contents/{path...}", s.handleGetContents)
	s.route("PUT /api/v3/repos/{owner}/{repo}/contents/{path...}", s.requirePerm(scopeContents, permWrite, s.handlePutContents))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/contents/{path...}", s.requirePerm(scopeContents, permWrite, s.handleDeleteContents))
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	commits, err := s.listRepoCommits(repo, owner, repoName, s.baseURL(r))
	if err != nil {
		switch {
		case errors.Is(err, errRepoGitRepositoryEmpty):
			writeGHError(w, http.StatusConflict, "Git Repository is empty.")
		case errors.Is(err, errRepoGitObjectUnavailable):
			writeGHError(w, http.StatusInternalServerError, "Git object unavailable")
		default:
			writeGHError(w, http.StatusInternalServerError, "Git storage unavailable")
		}
		return
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, commits))
}

func (s *Server) listRepoCommits(repo *Repo, owner, repoName, baseURL string) ([]map[string]interface{}, error) {
	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		return nil, errRepoGitStorageUnavailable
	}

	// Resolve default branch
	branchRef := plumbing.NewBranchReferenceName(repo.DefaultBranch)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, errRepoGitRepositoryEmpty
		}
		return nil, errRepoGitStorageUnavailable
	}

	// Walk commits
	var commits []map[string]interface{}
	hash := ref.Hash()
	for i := 0; i < 30; i++ {
		commit, err := object.GetCommit(stor, hash)
		if err != nil {
			return nil, errRepoGitObjectUnavailable
		}

		commits = append(commits, commitToJSON(commit, repo, baseURL))

		if commit.NumParents() == 0 {
			break
		}
		hash = commit.ParentHashes[0]
	}

	if commits == nil {
		commits = []map[string]interface{}{}
	}
	return commits, nil
}

func (s *Server) handleGetTree(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	sha := r.PathValue("sha")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	tree, err := object.GetTree(stor, plumbing.NewHash(sha))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	entries := make([]map[string]interface{}, 0, len(tree.Entries))
	for _, e := range tree.Entries {
		entryType := "tree"
		if e.Mode.IsFile() {
			entryType = "blob"
		}

		entries = append(entries, map[string]interface{}{
			"path": e.Name,
			"mode": e.Mode.String(),
			"type": entryType,
			"sha":  e.Hash.String(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sha":       sha,
		"tree":      entries,
		"truncated": false,
	})
}

func (s *Server) handleGetBlob(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	sha := r.PathValue("sha")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	blob, err := object.GetBlob(stor, plumbing.NewHash(sha))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	reader, err := blob.Reader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sha":      sha,
		"size":     blob.Size,
		"encoding": "base64",
		"content":  base64.StdEncoding.EncodeToString(content),
	})
}

func (s *Server) handleGetReadme(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Resolve default branch
	branchRef := plumbing.NewBranchReferenceName(repo.DefaultBranch)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	tree, err := commit.Tree()
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Search for README variants
	for _, name := range []string{"README.md", "README", "README.txt", "readme.md"} {
		entry, err := tree.FindEntry(name)
		if err != nil {
			continue
		}

		blob, err := object.GetBlob(stor, entry.Hash)
		if err != nil {
			continue
		}

		reader, err := blob.Reader()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			continue
		}

		out := contentFileJSON(s.baseURL(r), repo, repo.DefaultBranch, name, entry.Hash.String(), blob.Size)
		out["encoding"] = "base64"
		out["content"] = base64.StdEncoding.EncodeToString(content)
		writeJSON(w, http.StatusOK, out)
		return
	}

	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handlePutContents(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	path := r.PathValue("path")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}

	var req struct {
		Message string `json:"message"`
		Content string `json:"content"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha"`
		Author  *struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Committer *struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"committer"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeGHValidationError(w, "Commit", "message", "missing_field")
		return
	}
	if req.Content == "" {
		writeGHValidationError(w, "Commit", "content", "missing_field")
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		writeGHValidationError(w, "Commit", "content", "invalid")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	sig := repoSignature(user.Login, "bleephub@local")
	if req.Committer != nil {
		sig = repoSignature(req.Committer.Name, req.Committer.Email)
	} else if req.Author != nil {
		sig = repoSignature(req.Author.Name, req.Author.Email)
	}

	// Determine whether this commit initializes an empty repository. A
	// missing branch on a repository that already has branches is a 404 on
	// real GitHub — PUT contents never creates branches (and committing via
	// an unrelated worktree would silently advance the current branch).
	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, refErr := stor.Reference(branchRef)
	var commitHash plumbing.Hash
	var isInitial bool
	beforeHash := plumbing.ZeroHash.String()
	if refErr != nil || ref == nil {
		if repoHasAnyBranch(stor) {
			writeGHError(w, http.StatusNotFound, "Branch not found")
			return
		}
		isInitial = true
	} else {
		beforeHash = ref.Hash().String()
	}

	if isInitial {
		files := map[string]string{path: string(decoded)}
		if ph := s.createSecretScanningPushProtectionPlaceholder(repo, secretScanningContentMatches(string(decoded))); ph != nil {
			writeSecretScanningPushProtectionBlocked(w, ph)
			return
		}
		commitHash, err = initRepoWithFiles(stor, branch, req.Message, files, sig)
	} else {
		if ph := s.createSecretScanningPushProtectionPlaceholder(repo, secretScanningContentMatches(string(decoded))); ph != nil {
			writeSecretScanningPushProtectionBlocked(w, ph)
			return
		}
		commitHash, err = createFileCommit(stor, branch, path, string(decoded), req.Message, sig)
	}
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	commit, err := object.GetCommit(stor, commitHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tree, err := commit.Tree()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry, err := tree.FindEntry(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	blob, err := object.GetBlob(stor, entry.Hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.store.UpdateRepo(owner, repoName, func(r *Repo) {
		r.PushedAt = time.Now().UTC()
	})

	base := s.baseURL(r)
	if err := s.scanCommitForSecretScanning(repo, stor, commitHash, base); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.afterCommittedRefUpdate(repo, user, branchRef.String(), beforeHash, commitHash.String(), base)
	contentOut := contentFileJSON(base, repo, branch, path, entry.Hash.String(), blob.Size)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"content": contentOut,
		"commit": map[string]interface{}{
			"sha":     commitHash.String(),
			"message": strings.TrimSpace(commit.Message),
			"author": map[string]interface{}{
				"name":  commit.Author.Name,
				"email": commit.Author.Email,
				"date":  commit.Author.When.Format(time.RFC3339),
			},
			"committer": map[string]interface{}{
				"name":  commit.Committer.Name,
				"email": commit.Committer.Email,
				"date":  commit.Committer.When.Format(time.RFC3339),
			},
			"tree": map[string]interface{}{
				"sha": commit.TreeHash.String(),
			},
		},
	})
}

func (s *Server) handleDeleteContents(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	path := r.PathValue("path")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}

	var req struct {
		Message string `json:"message"`
		SHA     string `json:"sha"`
		Branch  string `json:"branch"`
		Author  *struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Committer *struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"committer"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeGHValidationError(w, "Commit", "message", "missing_field")
		return
	}
	if req.SHA == "" {
		writeGHValidationError(w, "Commit", "sha", "missing_field")
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Resolve the file to verify the SHA matches.
	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	tree, err := commit.Tree()
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	entry, err := tree.FindEntry(path)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, fmt.Sprintf("path %s does not exist", path))
		return
	}
	if entry.Hash.String() != req.SHA {
		writeGHError(w, http.StatusUnprocessableEntity, fmt.Sprintf("sha mismatch: expected %s, got %s", entry.Hash.String(), req.SHA))
		return
	}

	sig := repoSignature(user.Login, "bleephub@local")
	if req.Committer != nil {
		sig = repoSignature(req.Committer.Name, req.Committer.Email)
	} else if req.Author != nil {
		sig = repoSignature(req.Author.Name, req.Author.Email)
	}

	commitHash, err := deleteFileCommit(stor, branch, path, req.Message, sig)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	commit, err = object.GetCommit(stor, commitHash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.store.UpdateRepo(owner, repoName, func(r *Repo) {
		r.PushedAt = time.Now().UTC()
	})
	s.afterCommittedRefUpdate(repo, user, branchRef.String(), ref.Hash().String(), commitHash.String(), s.baseURL(r))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": nil,
		"commit": map[string]interface{}{
			"sha":     commitHash.String(),
			"message": strings.TrimSpace(commit.Message),
			"author": map[string]interface{}{
				"name":  commit.Author.Name,
				"email": commit.Author.Email,
				"date":  commit.Author.When.Format(time.RFC3339),
			},
			"committer": map[string]interface{}{
				"name":  commit.Committer.Name,
				"email": commit.Committer.Email,
				"date":  commit.Committer.When.Format(time.RFC3339),
			},
			"tree": map[string]interface{}{
				"sha": commit.TreeHash.String(),
			},
		},
	})
}

// contentFileJSON builds the common members of the GitHub content-file
// shape (name/path/sha/size plus the hypermedia URLs and _links the
// schema requires) for a blob at the given path on the given ref.
func contentFileJSON(baseURL string, repo *Repo, ref, path, sha string, size int64) map[string]interface{} {
	selfURL := baseURL + "/api/v3/repos/" + repo.FullName + "/contents/" + path + "?ref=" + ref
	gitURL := baseURL + "/api/v3/repos/" + repo.FullName + "/git/blobs/" + sha
	htmlURL := baseURL + "/" + repo.FullName + "/blob/" + ref + "/" + path
	downloadURL := baseURL + "/" + repo.FullName + "/raw/" + ref + "/" + path
	return map[string]interface{}{
		"name":         path[strings.LastIndex(path, "/")+1:],
		"path":         path,
		"sha":          sha,
		"size":         size,
		"type":         "file",
		"url":          selfURL,
		"git_url":      gitURL,
		"html_url":     htmlURL,
		"download_url": downloadURL,
		"_links": map[string]interface{}{
			"self": selfURL,
			"git":  gitURL,
			"html": htmlURL,
		},
	}
}

func (s *Server) handleGetContents(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	path := r.PathValue("path")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Resolve ref (query param or default branch)
	refName := r.URL.Query().Get("ref")
	if refName == "" {
		refName = repo.DefaultBranch
	}
	branchRef := plumbing.NewBranchReferenceName(refName)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	tree, err := commit.Tree()
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Empty path means list the root tree.
	if path == "" {
		writeTreeListing(w, tree, "")
		return
	}

	// Try as file first
	entry, err := tree.FindEntry(path)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if entry.Mode.IsFile() {
		blob, err := object.GetBlob(stor, entry.Hash)
		if err != nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}

		reader, err := blob.Reader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer reader.Close()

		content, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := contentFileJSON(s.baseURL(r), repo, refName, path, entry.Hash.String(), blob.Size)
		out["encoding"] = "base64"
		out["content"] = base64.StdEncoding.EncodeToString(content)
		writeJSON(w, http.StatusOK, out)
		return
	}

	// It's a directory (tree entry)
	subTree, err := object.GetTree(stor, entry.Hash)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeTreeListing(w, subTree, path)
}

func writeTreeListing(w http.ResponseWriter, tree *object.Tree, prefix string) {
	var items []map[string]interface{}
	for _, e := range tree.Entries {
		entryType := "file"
		if !e.Mode.IsFile() {
			entryType = "dir"
		}
		itemPath := e.Name
		if prefix != "" {
			itemPath = prefix + "/" + e.Name
		}
		items = append(items, map[string]interface{}{
			"name": e.Name,
			"path": itemPath,
			"sha":  e.Hash.String(),
			"type": entryType,
		})
	}

	writeJSON(w, http.StatusOK, items)
}

// repoHasAnyBranch reports whether the repository has at least one branch.
func repoHasAnyBranch(stor gitStorage.Storer) bool {
	refs, err := stor.IterReferences()
	if err != nil {
		return false
	}
	defer refs.Close()
	found := false
	_ = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() && !ref.Hash().IsZero() {
			found = true
		}
		return nil
	})
	return found
}
