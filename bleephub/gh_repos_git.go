package bleephub

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// repoSignature returns the default author/committer signature for
// bleephub-generated commits. Matches the GitHub web UI's behavior for
// auto_init and web-based file creation.
func repoSignature(name, email string) *object.Signature {
	return &object.Signature{
		Name:  name,
		Email: email,
		When:  time.Now().UTC(),
	}
}

// initRepoWithFiles creates the first commit on a freshly created repo,
// populating it with the supplied files and pointing the given branch at
// the resulting commit. It is used for auto_init and for the contents
// PUT endpoint when the caller creates the first file in an empty repo.
func initRepoWithFiles(stor gitStorage.Storer, branch, message string, files map[string]string, sig *object.Signature) (plumbing.Hash, error) {
	fs := memfs.New()
	repo, err := git.Init(stor, fs)
	if err != nil {
		if !errors.Is(err, git.ErrRepositoryAlreadyExists) {
			return plumbing.ZeroHash, fmt.Errorf("git init: %w", err)
		}
		repo, err = git.Open(stor, fs)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("git open: %w", err)
		}
	}
	wt, err := repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("worktree: %w", err)
	}
	if err := writeFilesToWorktree(fs, wt, files); err != nil {
		return plumbing.ZeroHash, err
	}
	commitHash, err := wt.Commit(message, &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("commit: %w", err)
	}
	branchRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), commitHash)
	if err := repo.Storer.SetReference(branchRef); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("set ref: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, branchRef.Name())); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("set HEAD: %w", err)
	}
	if branch != "master" {
		masterRef := plumbing.NewBranchReferenceName("master")
		if _, err := repo.Storer.Reference(masterRef); err == nil {
			_ = repo.Storer.RemoveReference(masterRef)
		}
	}
	return commitHash, nil
}

// createFileCommit adds or updates a single file on the given branch and
// returns the new commit hash. It preserves the existing tree, sets the
// commit parent to the current branch HEAD, and updates the branch ref.
func createFileCommit(stor gitStorage.Storer, branch, path, content, message string, sig *object.Signature) (plumbing.Hash, error) {
	fs := memfs.New()
	repo, err := git.Open(stor, fs)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("git open: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("worktree: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, err := repo.Storer.Reference(branchRef)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("resolve branch %s: %w", branch, err)
	}
	parentHash := ref.Hash()

	if err := wt.Checkout(&git.CheckoutOptions{Hash: parentHash, Force: true}); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("checkout: %w", err)
	}

	if err := writeFileToWorktree(fs, wt, path, content); err != nil {
		return plumbing.ZeroHash, err
	}

	commitHash, err := wt.Commit(message, &git.CommitOptions{
		Author:    sig,
		Committer: sig,
		Parents:   []plumbing.Hash{parentHash},
	})
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("commit: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branchRef, commitHash)); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("set ref: %w", err)
	}
	return commitHash, nil
}

// deleteFileCommit removes a single file on the given branch and returns the
// new commit hash. It returns an error if the file does not exist.
func deleteFileCommit(stor gitStorage.Storer, branch, path, message string, sig *object.Signature) (plumbing.Hash, error) {
	fs := memfs.New()
	repo, err := git.Open(stor, fs)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("git open: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("worktree: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, err := repo.Storer.Reference(branchRef)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("resolve branch %s: %w", branch, err)
	}
	parentHash := ref.Hash()

	if err := wt.Checkout(&git.CheckoutOptions{Hash: parentHash, Force: true}); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("checkout: %w", err)
	}

	if _, err := fs.Stat(path); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("path does not exist: %s", path)
	}

	if _, err := wt.Remove(path); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("git remove %s: %w", path, err)
	}

	commitHash, err := wt.Commit(message, &git.CommitOptions{
		Author:    sig,
		Committer: sig,
		Parents:   []plumbing.Hash{parentHash},
	})
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("commit: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branchRef, commitHash)); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("set ref: %w", err)
	}
	return commitHash, nil
}

func writeFilesToWorktree(fs billy.Filesystem, wt *git.Worktree, files map[string]string) error {
	for path, body := range files {
		if err := writeFileToWorktree(fs, wt, path, body); err != nil {
			return err
		}
	}
	return nil
}

func writeFileToWorktree(fs billy.Filesystem, wt *git.Worktree, path, body string) error {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		if err := fs.MkdirAll(path[:idx], 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", path[:idx], err)
		}
	}
	f, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := f.Write([]byte(body)); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if _, err := wt.Add(path); err != nil {
		return fmt.Errorf("git add %s: %w", path, err)
	}
	return nil
}

// ensureRepoInitialized creates git storage for a repo that does not yet
// have any. It is used by org repo creation, which historically registered
// the Repo row before allocating storage.
func (s *Server) initRepoFiles(ctx context.Context, repo *Repo, branch, description, gitignoreTemplate, licenseTemplate string, includeReadme bool) error {
	owner, name, ok := splitRepoFullName(repo.FullName)
	if !ok {
		return fmt.Errorf("invalid full name %q", repo.FullName)
	}
	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		return fmt.Errorf("no git storage for %s", repo.FullName)
	}

	files := make(map[string]string)
	if includeReadme {
		files["README.md"] = renderReadme(repo.Name, description)
	}
	if gitignoreTemplate != "" {
		if name, ok := normalizeGitignoreName(gitignoreTemplate); ok {
			files[".gitignore"] = gitignoreTemplates[name]
		}
	}
	if licenseTemplate != "" {
		if key, ok := normalizeLicenseKey(licenseTemplate); ok {
			files["LICENSE"] = licenseBody(key, owner, repo.Name, time.Now().Year())
		}
	}
	if len(files) == 0 {
		return nil
	}

	sig := repoSignature(userDisplayName(repo), "bleephub@local")
	_, err := initRepoWithFiles(stor, branch, "Initial commit", files, sig)
	if err != nil {
		return err
	}
	s.store.UpdateRepo(owner, name, func(r *Repo) {
		r.PushedAt = time.Now().UTC()
		if licenseTemplate != "" {
			if key, ok := normalizeLicenseKey(licenseTemplate); ok {
				tmpl := licenseTemplates[key]
				r.LicenseKey = key
				r.LicenseName = tmpl.name
				r.LicenseSPDX = tmpl.spdxID
			}
		}
	})
	return nil
}

func userDisplayName(repo *Repo) string {
	if repo.Owner != nil && repo.Owner.Name != "" {
		return repo.Owner.Name
	}
	if repo.Owner != nil {
		return repo.Owner.Login
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return "bleephub"
}

func splitRepoFullName(fullName string) (owner, name string, ok bool) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func renderReadme(repoName, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", repoName)
	if description != "" {
		fmt.Fprintln(&b, description)
	}
	return b.String()
}

func (s *Server) registerGHGitDataRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/git/blobs/{sha}", s.requirePerm(scopeContents, permRead, s.handleGetBlob))
	s.route("POST /api/v3/repos/{owner}/{repo}/git/blobs", s.requirePerm(scopeContents, permWrite, s.handleCreateBlob))
	s.route("GET /api/v3/repos/{owner}/{repo}/git/trees/{sha}", s.requirePerm(scopeContents, permRead, s.handleGetTree))
	s.route("POST /api/v3/repos/{owner}/{repo}/git/trees", s.requirePerm(scopeContents, permWrite, s.handleCreateTree))
	s.route("GET /api/v3/repos/{owner}/{repo}/git/commits/{sha}", s.requirePerm(scopeContents, permRead, s.handleGetCommit))
	s.route("POST /api/v3/repos/{owner}/{repo}/git/commits", s.requirePerm(scopeContents, permWrite, s.handleCreateCommit))
	s.route("GET /api/v3/repos/{owner}/{repo}/git/tags/{sha}", s.requirePerm(scopeContents, permRead, s.handleGetTag))
	s.route("POST /api/v3/repos/{owner}/{repo}/git/tags", s.requirePerm(scopeContents, permWrite, s.handleCreateTag))
	s.route("GET /api/v3/repos/{owner}/{repo}/git/refs", s.requirePerm(scopeContents, permRead, s.handleListRefs))
	s.route("GET /api/v3/repos/{owner}/{repo}/git/refs/{ref...}", s.requirePerm(scopeContents, permRead, s.handleGetRefs))
	s.route("POST /api/v3/repos/{owner}/{repo}/git/refs", s.requirePerm(scopeContents, permWrite, s.handleCreateRef))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/git/refs/{ref...}", s.requirePerm(scopeContents, permWrite, s.handleUpdateRef))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/git/refs/{ref...}", s.requirePerm(scopeContents, permWrite, s.handleDeleteRef))
}

func (s *Server) gitDataContext(w http.ResponseWriter, r *http.Request) (owner, repoName string, repo *Repo, stor gitStorage.Storer) {
	owner = r.PathValue("owner")
	repoName = r.PathValue("repo")
	repo = s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	stor = s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	return
}

func (s *Server) handleCreateBlob(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	data := []byte(req.Content)
	if req.Encoding == "base64" {
		var err error
		data, err = base64.StdEncoding.DecodeString(req.Content)
		if err != nil {
			writeGHError(w, http.StatusBadRequest, "content must be valid base64")
			return
		}
	}

	hash, err := encodeBlob(stor, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	base := s.baseURL(r)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"sha": hash.String(),
		"url": base + "/api/v3/repos/" + repo.FullName + "/git/blobs/" + hash.String(),
	})
}

func (s *Server) handleCreateTree(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil {
		return
	}
	var req struct {
		BaseTree string `json:"base_tree"`
		Tree     []struct {
			Path    string `json:"path"`
			Mode    string `json:"mode"`
			Type    string `json:"type"`
			SHA     string `json:"sha"`
			Content string `json:"content"`
		} `json:"tree"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	var baseTree *object.Tree
	if req.BaseTree != "" {
		var err error
		baseTree, err = object.GetTree(stor, plumbing.NewHash(req.BaseTree))
		if err != nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}

	tree := &object.Tree{}
	if baseTree != nil {
		tree.Entries = append([]object.TreeEntry(nil), baseTree.Entries...)
	}

	modeOverride := map[string]filemode.FileMode{
		"100644": filemode.Regular,
		"100755": filemode.Executable,
		"040000": filemode.Dir,
		"160000": filemode.Submodule,
		"120000": filemode.Symlink,
	}

	for _, ent := range req.Tree {
		mode := filemode.Regular
		if ent.Mode != "" {
			if m, ok := modeOverride[ent.Mode]; ok {
				mode = m
			} else {
				writeGHError(w, http.StatusUnprocessableEntity, "invalid mode: "+ent.Mode)
				return
			}
		}
		if ent.Type == "tree" {
			mode = filemode.Dir
		}

		var hash plumbing.Hash
		if ent.SHA != "" {
			hash = plumbing.NewHash(ent.SHA)
			if _, err := stor.EncodedObject(plumbing.AnyObject, hash); err != nil {
				writeGHError(w, http.StatusNotFound, "Not Found")
				return
			}
		} else if ent.Content != "" {
			hash, _ = encodeBlob(stor, []byte(ent.Content))
		} else {
			writeGHError(w, http.StatusUnprocessableEntity, "sha or content required")
			return
		}

		// Replace existing entry with same path, or append.
		found := false
		for i, e := range tree.Entries {
			if e.Name == ent.Path {
				tree.Entries[i] = object.TreeEntry{Name: ent.Path, Mode: mode, Hash: hash}
				found = true
				break
			}
		}
		if !found {
			tree.Entries = append(tree.Entries, object.TreeEntry{Name: ent.Path, Mode: mode, Hash: hash})
		}
	}

	hash, err := encodeTree(stor, tree)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
			"size": 0,
			"url":  s.baseURL(r) + "/api/v3/repos/" + repo.FullName + "/git/" + entryType + "s/" + e.Hash.String(),
		})
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"sha":       hash.String(),
		"url":       s.baseURL(r) + "/api/v3/repos/" + repo.FullName + "/git/trees/" + hash.String(),
		"tree":      entries,
		"truncated": false,
	})
}

func (s *Server) handleCreateCommit(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Message   string    `json:"message"`
		Tree      string    `json:"tree"`
		Parents   []string  `json:"parents"`
		Author    gitPerson `json:"author"`
		Committer gitPerson `json:"committer"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Message == "" || req.Tree == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "message and tree are required")
		return
	}

	treeHash := plumbing.NewHash(req.Tree)
	if _, err := object.GetTree(stor, treeHash); err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var parentHashes []plumbing.Hash
	for _, p := range req.Parents {
		h := plumbing.NewHash(p)
		if _, err := object.GetCommit(stor, h); err != nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		parentHashes = append(parentHashes, h)
	}

	sig := repoSignature(userDisplayName(repo), "bleephub@local")
	if req.Author.Name != "" {
		sig.Name = req.Author.Name
	}
	if req.Author.Email != "" {
		sig.Email = req.Author.Email
	}
	if req.Author.Date != "" {
		if d, err := time.Parse(time.RFC3339, req.Author.Date); err == nil {
			sig.When = d
		}
	}
	committerSig := *sig
	if req.Committer.Name != "" {
		committerSig.Name = req.Committer.Name
	}
	if req.Committer.Email != "" {
		committerSig.Email = req.Committer.Email
	}
	if req.Committer.Date != "" {
		if d, err := time.Parse(time.RFC3339, req.Committer.Date); err == nil {
			committerSig.When = d
		}
	}

	commit := &object.Commit{
		Author:       *sig,
		Committer:    committerSig,
		Message:      req.Message,
		TreeHash:     treeHash,
		ParentHashes: parentHashes,
	}
	hash, err := encodeCommit(stor, commit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, gitCommitToJSON(s.baseURL(r), repo.FullName, hash.String(), commit))
}

func (s *Server) handleGetCommit(w http.ResponseWriter, r *http.Request) {
	owner, repoName, _, stor := s.gitDataContext(w, r)
	if stor == nil {
		return
	}
	sha := r.PathValue("sha")
	commit, err := object.GetCommit(stor, plumbing.NewHash(sha))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, gitCommitToJSON(s.baseURL(r), owner+"/"+repoName, sha, commit))
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Tag     string    `json:"tag"`
		Message string    `json:"message"`
		Object  string    `json:"object"`
		Type    string    `json:"type"`
		Tagger  gitPerson `json:"tagger"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Tag == "" || req.Object == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "tag and object are required")
		return
	}

	targetHash := plumbing.NewHash(req.Object)
	objType := plumbing.AnyObject
	switch req.Type {
	case "commit":
		objType = plumbing.CommitObject
	case "tree":
		objType = plumbing.TreeObject
	case "blob":
		objType = plumbing.BlobObject
	case "tag":
		objType = plumbing.TagObject
	}
	if _, err := stor.EncodedObject(objType, targetHash); err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	tag := &object.Tag{
		Name:       req.Tag,
		Tagger:     *repoSignature(userDisplayName(repo), "bleephub@local"),
		Message:    req.Message,
		Target:     targetHash,
		TargetType: objType,
	}
	if req.Tagger.Name != "" {
		tag.Tagger.Name = req.Tagger.Name
	}
	if req.Tagger.Email != "" {
		tag.Tagger.Email = req.Tagger.Email
	}
	if req.Tagger.Date != "" {
		if d, err := time.Parse(time.RFC3339, req.Tagger.Date); err == nil {
			tag.Tagger.When = d
		}
	}
	hash, err := encodeTag(stor, tag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, gitTagToJSON(s.baseURL(r), repo.FullName, hash.String(), tag))
}

func (s *Server) handleGetTag(w http.ResponseWriter, r *http.Request) {
	owner, repoName, _, stor := s.gitDataContext(w, r)
	if stor == nil {
		return
	}
	sha := r.PathValue("sha")
	tag, err := object.GetTag(stor, plumbing.NewHash(sha))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, gitTagToJSON(s.baseURL(r), owner+"/"+repoName, sha, tag))
}

func (s *Server) handleCreateRef(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Ref == "" || req.SHA == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "ref and sha are required")
		return
	}
	fullRef := plumbing.ReferenceName(req.Ref)
	if !strings.HasPrefix(string(fullRef), "refs/") {
		fullRef = plumbing.ReferenceName("refs/" + req.Ref)
	}
	target := plumbing.NewHash(req.SHA)
	if _, err := stor.EncodedObject(plumbing.AnyObject, target); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Object does not exist")
		return
	}
	if _, err := stor.Reference(fullRef); err == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Reference already exists")
		return
	}
	if ph, err := s.secretScanningPushProtectionPlaceholderForRef(repo, stor, fullRef, target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if ph != nil {
		writeSecretScanningPushProtectionBlocked(w, ph)
		return
	}
	if err := stor.SetReference(plumbing.NewHashReference(fullRef, target)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.scanRefForSecretScanning(repo, stor, fullRef, target, s.baseURL(r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fullRef.IsBranch() {
		s.afterCommittedRefUpdate(repo, ghUserFromContext(r.Context()), fullRef.String(), plumbing.ZeroHash.String(), target.String(), s.baseURL(r))
	}
	ref, _ := stor.Reference(fullRef)
	writeJSON(w, http.StatusCreated, refToJSON(s.baseURL(r), repo.FullName, ref))
}

func (s *Server) handleUpdateRef(w http.ResponseWriter, r *http.Request) {
	_, _, repo, stor := s.gitDataContext(w, r)
	if repo == nil {
		return
	}
	refPath := r.PathValue("ref")
	var req struct {
		SHA   string `json:"sha"`
		Force bool   `json:"force"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	fullRef := plumbing.ReferenceName("refs/" + refPath)
	oldRef, err := stor.Reference(fullRef)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Reference does not exist")
		return
	}
	newTarget := plumbing.NewHash(req.SHA)
	if _, err := stor.EncodedObject(plumbing.AnyObject, newTarget); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Object does not exist")
		return
	}
	if !req.Force {
		// Reject non-fast-forward updates.
		if oldRef.Type() == plumbing.HashReference {
			oldHash := oldRef.Hash()
			if newTarget != oldHash {
				// A target that isn't a commit (tag/tree/blob) can never be a
				// fast-forward of the old head, so a failed commit load rejects too.
				isFF := false
				if commit, err := object.GetCommit(stor, newTarget); err == nil {
					for _, p := range commit.ParentHashes {
						if p == oldHash {
							isFF = true
							break
						}
					}
				}
				if !isFF {
					writeGHError(w, http.StatusUnprocessableEntity, "Update is not a fast forward")
					return
				}
			}
		}
	}
	if ph, err := s.secretScanningPushProtectionPlaceholderForRef(repo, stor, fullRef, newTarget); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if ph != nil {
		writeSecretScanningPushProtectionBlocked(w, ph)
		return
	}
	if err := stor.SetReference(plumbing.NewHashReference(fullRef, newTarget)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.scanRefForSecretScanning(repo, stor, fullRef, newTarget, s.baseURL(r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fullRef.IsBranch() {
		s.afterCommittedRefUpdate(repo, ghUserFromContext(r.Context()), fullRef.String(), oldRef.Hash().String(), newTarget.String(), s.baseURL(r))
	}
	ref, _ := stor.Reference(fullRef)
	writeJSON(w, http.StatusOK, refToJSON(s.baseURL(r), repo.FullName, ref))
}

func (s *Server) scanRefForSecretScanning(repo *Repo, stor gitStorage.Storer, ref plumbing.ReferenceName, target plumbing.Hash, baseURL string) error {
	if !strings.HasPrefix(string(ref), "refs/heads/") {
		return nil
	}
	if _, err := object.GetCommit(stor, target); err != nil {
		return nil
	}
	return s.scanCommitForSecretScanning(repo, stor, target, baseURL)
}

type gitPerson struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

func gitCommitToJSON(baseURL, fullName, sha string, c *object.Commit) map[string]interface{} {
	author := map[string]interface{}{
		"name":  c.Author.Name,
		"email": c.Author.Email,
		"date":  c.Author.When.Format(time.RFC3339),
	}
	committer := map[string]interface{}{
		"name":  c.Committer.Name,
		"email": c.Committer.Email,
		"date":  c.Committer.When.Format(time.RFC3339),
	}
	parents := make([]map[string]interface{}, 0, len(c.ParentHashes))
	for _, h := range c.ParentHashes {
		parents = append(parents, map[string]interface{}{
			"sha":      h.String(),
			"url":      baseURL + "/api/v3/repos/" + fullName + "/commits/" + h.String(),
			"html_url": baseURL + "/" + fullName + "/commit/" + h.String(),
		})
	}
	return map[string]interface{}{
		"sha":       sha,
		"node_id":   encodeNodeID("Commit", 0, sha),
		"url":       baseURL + "/api/v3/repos/" + fullName + "/git/commits/" + sha,
		"html_url":  baseURL + "/" + fullName + "/commit/" + sha,
		"author":    author,
		"committer": committer,
		"message":   c.Message,
		"tree": map[string]interface{}{
			"sha": c.TreeHash.String(),
			"url": baseURL + "/api/v3/repos/" + fullName + "/git/trees/" + c.TreeHash.String(),
		},
		"parents": parents,
		"verification": map[string]interface{}{
			"verified":    false,
			"reason":      "unsigned",
			"signature":   nil,
			"payload":     nil,
			"verified_at": nil,
		},
	}
}

func gitTagToJSON(baseURL, fullName, sha string, t *object.Tag) map[string]interface{} {
	return map[string]interface{}{
		"sha":     sha,
		"node_id": encodeNodeID("Tag", 0, sha),
		"url":     baseURL + "/api/v3/repos/" + fullName + "/git/tags/" + sha,
		"tag":     t.Name,
		"message": t.Message,
		"tagger": map[string]interface{}{
			"name":  t.Tagger.Name,
			"email": t.Tagger.Email,
			"date":  t.Tagger.When.Format(time.RFC3339),
		},
		"object": map[string]interface{}{
			"sha":  t.Target.String(),
			"type": objectTypeName(t.TargetType),
			"url":  baseURL + "/api/v3/repos/" + fullName + "/git/" + objectTypeName(t.TargetType) + "s/" + t.Target.String(),
		},
	}
}

func objectTypeName(t plumbing.ObjectType) string {
	switch t {
	case plumbing.CommitObject:
		return "commit"
	case plumbing.TreeObject:
		return "tree"
	case plumbing.BlobObject:
		return "blob"
	case plumbing.TagObject:
		return "tag"
	default:
		return "unknown"
	}
}

func encodeBlob(stor gitStorage.Storer, data []byte) (plumbing.Hash, error) {
	obj := stor.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return stor.SetEncodedObject(obj)
}

func encodeTree(stor gitStorage.Storer, t *object.Tree) (plumbing.Hash, error) {
	obj := stor.NewEncodedObject()
	obj.SetType(plumbing.TreeObject)
	if err := t.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return stor.SetEncodedObject(obj)
}

func encodeCommit(stor gitStorage.Storer, c *object.Commit) (plumbing.Hash, error) {
	obj := stor.NewEncodedObject()
	obj.SetType(plumbing.CommitObject)
	if err := c.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return stor.SetEncodedObject(obj)
}

func encodeTag(stor gitStorage.Storer, t *object.Tag) (plumbing.Hash, error) {
	obj := stor.NewEncodedObject()
	obj.SetType(plumbing.TagObject)
	if err := t.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return stor.SetEncodedObject(obj)
}
