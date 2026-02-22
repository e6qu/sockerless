package bleephub

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

func (s *Server) registerGHRepoRefRoutes() {
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/branches", s.handleListBranches)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/branches/{branch}", s.handleGetBranch)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}/git/refs/{ref...}", s.handleDeleteRef)
}

func (s *Server) handleListBranches(w http.ResponseWriter, r *http.Request) {
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

	refs, err := stor.IterReferences()
	if err != nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	var branches []map[string]interface{}
	_ = refs.ForEach(func(ref *plumbing.Reference) error {
		if !ref.Name().IsBranch() {
			return nil
		}
		branchName := ref.Name().Short()
		entry := map[string]interface{}{
			"name":      branchName,
			"protected": false,
			"commit": map[string]interface{}{
				"sha": ref.Hash().String(),
			},
		}

		// Resolve commit details
		if commit := resolveCommit(stor, ref.Hash()); commit != nil {
			entry["commit"] = commitSummary(commit)
		}

		branches = append(branches, entry)
		return nil
	})

	if branches == nil {
		branches = []map[string]interface{}{}
	}
	writeJSON(w, http.StatusOK, branches)
}

func (s *Server) handleGetBranch(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	branch := r.PathValue("branch")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Branch not found")
		return
	}

	ref, err := stor.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Branch not found")
		return
	}

	result := map[string]interface{}{
		"name":      branch,
		"protected": false,
		"commit": map[string]interface{}{
			"sha": ref.Hash().String(),
		},
	}

	if commit := resolveCommit(stor, ref.Hash()); commit != nil {
		result["commit"] = commitSummary(commit)
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDeleteRef(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	repoName := r.PathValue("repo")
	refPath := r.PathValue("ref")

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if repo.Owner.ID != user.ID {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}

	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// refPath is like "heads/branch-name" or "tags/v1.0"
	fullRef := plumbing.ReferenceName("refs/" + refPath)
	if _, err := stor.Reference(fullRef); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Reference does not exist")
		return
	}

	if err := stor.RemoveReference(fullRef); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// resolveCommit looks up a commit object from storage by hash.
func resolveCommit(stor storer.EncodedObjectStorer, hash plumbing.Hash) *object.Commit {
	obj, err := object.GetCommit(stor, hash)
	if err != nil {
		return nil
	}
	return obj
}

// commitSummary converts a commit to a JSON map.
func commitSummary(c *object.Commit) map[string]interface{} {
	return map[string]interface{}{
		"sha": c.Hash.String(),
		"commit": map[string]interface{}{
			"message": strings.TrimSpace(c.Message),
			"author": map[string]interface{}{
				"name":  c.Author.Name,
				"email": c.Author.Email,
				"date":  c.Author.When.Format(time.RFC3339),
			},
			"committer": map[string]interface{}{
				"name":  c.Committer.Name,
				"email": c.Committer.Email,
				"date":  c.Committer.When.Format(time.RFC3339),
			},
			"tree": map[string]interface{}{
				"sha": c.TreeHash.String(),
			},
		},
	}
}
