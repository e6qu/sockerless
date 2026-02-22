package bleephub

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (s *Server) registerGHRepoObjectRoutes() {
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/commits", s.handleListCommits)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/git/trees/{sha}", s.handleGetTree)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/git/blobs/{sha}", s.handleGetBlob)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/readme", s.handleGetReadme)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/contents/{path...}", s.handleGetContents)
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	// Resolve default branch
	branchRef := plumbing.NewBranchReferenceName(repo.DefaultBranch)
	ref, err := stor.Reference(branchRef)
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	// Walk commits
	var commits []map[string]interface{}
	hash := ref.Hash()
	for i := 0; i < 30; i++ {
		commit, err := object.GetCommit(stor, hash)
		if err != nil {
			break
		}

		commits = append(commits, map[string]interface{}{
			"sha": commit.Hash.String(),
			"commit": map[string]interface{}{
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
			"html_url": "/" + repo.FullName + "/commit/" + commit.Hash.String(),
		})

		if commit.NumParents() == 0 {
			break
		}
		hash = commit.ParentHashes[0]
	}

	if commits == nil {
		commits = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, commits))
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

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":     name,
			"path":     name,
			"sha":      entry.Hash.String(),
			"size":     blob.Size,
			"type":     "file",
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString(content),
		})
		return
	}

	writeGHError(w, http.StatusNotFound, "Not Found")
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

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"name":     entry.Name,
			"path":     path,
			"sha":      entry.Hash.String(),
			"size":     blob.Size,
			"type":     "file",
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString(content),
		})
		return
	}

	// It's a directory (tree entry)
	subTree, err := object.GetTree(stor, entry.Hash)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var items []map[string]interface{}
	for _, e := range subTree.Entries {
		entryType := "file"
		if !e.Mode.IsFile() {
			entryType = "dir"
		}
		items = append(items, map[string]interface{}{
			"name": e.Name,
			"path": path + "/" + e.Name,
			"sha":  e.Hash.String(),
			"type": entryType,
		})
	}

	writeJSON(w, http.StatusOK, items)
}
