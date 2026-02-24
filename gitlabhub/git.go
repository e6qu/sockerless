package gitlabhub

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/storage/memory"
)

// storeLoader implements transport.Loader to look up go-git storages from the Store.
type storeLoader struct {
	store *Store
}

func (l *storeLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	path := strings.TrimPrefix(ep.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	s := l.store.GetGitStorage(path)
	if s == nil {
		return nil, transport.ErrRepositoryNotFound
	}
	return s, nil
}

// createProjectRepo creates an in-memory bare git repo with an initial commit
// containing the given files. Returns the commit SHA.
func (s *Server) createProjectRepo(name string, files map[string]string) (string, error) {
	stor := memory.NewStorage()

	repo, err := git.Init(stor, nil)
	if err != nil {
		return "", fmt.Errorf("git init: %w", err)
	}

	// Create tree entries from files
	tree := &object.Tree{}
	for filename, content := range files {
		blob := &plumbing.MemoryObject{}
		blob.SetType(plumbing.BlobObject)
		blob.SetSize(int64(len(content)))
		w, _ := blob.Writer()
		_, _ = w.Write([]byte(content))
		w.Close()
		blobHash, err := stor.SetEncodedObject(blob)
		if err != nil {
			return "", fmt.Errorf("store blob %s: %w", filename, err)
		}
		tree.Entries = append(tree.Entries, object.TreeEntry{
			Name: filename,
			Mode: 0100644,
			Hash: blobHash,
		})
	}

	// Sort tree entries (git requires sorted entries)
	sort.Slice(tree.Entries, func(i, j int) bool {
		return tree.Entries[i].Name < tree.Entries[j].Name
	})

	// Store tree
	treeObj := &plumbing.MemoryObject{}
	treeObj.SetType(plumbing.TreeObject)
	err = tree.Encode(treeObj)
	if err != nil {
		return "", fmt.Errorf("encode tree: %w", err)
	}
	treeHash, err := stor.SetEncodedObject(treeObj)
	if err != nil {
		return "", fmt.Errorf("store tree: %w", err)
	}

	// Create commit
	now := time.Now()
	commit := &object.Commit{
		Author: object.Signature{
			Name:  "gitlabhub",
			Email: "ci@gitlabhub.local",
			When:  now,
		},
		Committer: object.Signature{
			Name:  "gitlabhub",
			Email: "ci@gitlabhub.local",
			When:  now,
		},
		Message:  "Initial commit",
		TreeHash: treeHash,
	}
	commitObj := &plumbing.MemoryObject{}
	commitObj.SetType(plumbing.CommitObject)
	if err := commit.Encode(commitObj); err != nil {
		return "", fmt.Errorf("encode commit: %w", err)
	}
	commitHash, err := stor.SetEncodedObject(commitObj)
	if err != nil {
		return "", fmt.Errorf("store commit: %w", err)
	}

	// Set refs/heads/main → commit
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commitHash)
	if err := stor.SetReference(ref); err != nil {
		return "", fmt.Errorf("set ref: %w", err)
	}

	// Set HEAD → refs/heads/main
	symRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
	if err := stor.SetReference(symRef); err != nil {
		return "", fmt.Errorf("set HEAD: %w", err)
	}

	// Verify commit is reachable
	if _, err := repo.CommitObject(commitHash); err != nil {
		return "", fmt.Errorf("verify commit: %w", err)
	}

	s.store.SetGitStorage(name, stor)
	return commitHash.String(), nil
}

// tryHandleGitRequest checks if the request is a git smart HTTP request and handles it.
func (s *Server) tryHandleGitRequest(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path

	// Match /<project>.git/info/refs or /<project>/info/refs
	if strings.HasSuffix(path, "/info/refs") && r.Method == "GET" {
		repoPath := strings.TrimSuffix(path, "/info/refs")
		repoPath = strings.TrimPrefix(repoPath, "/")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		if repoPath != "" {
			s.handleGitInfoRefs(w, r, repoPath)
			return true
		}
	}

	// Match /<project>.git/git-upload-pack
	if strings.HasSuffix(path, "/git-upload-pack") && r.Method == "POST" {
		repoPath := strings.TrimSuffix(path, "/git-upload-pack")
		repoPath = strings.TrimPrefix(repoPath, "/")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		if repoPath != "" {
			s.handleGitUploadPack(w, r, repoPath)
			return true
		}
	}

	return false
}

func (s *Server) handleGitInfoRefs(w http.ResponseWriter, r *http.Request, repoName string) {
	stor := s.store.GetGitStorage(repoName)
	if stor == nil {
		http.NotFound(w, r)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		http.Error(w, "service parameter required", http.StatusBadRequest)
		return
	}

	loader := &storeLoader{store: s.store}
	server := gitserver.NewServer(loader)

	ep, err := transport.NewEndpoint(fmt.Sprintf("/%s", repoName))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	enc := pktline.NewEncoder(w)
	_ = enc.Encodef("# service=%s\n", service)
	_ = enc.Flush()

	switch service {
	case "git-upload-pack":
		sess, err := server.NewUploadPackSession(ep, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		info, err := sess.AdvertisedReferencesContext(r.Context())
		if err != nil {
			if err == transport.ErrEmptyRemoteRepository {
				_ = enc.Flush()
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := info.Encode(w); err != nil {
			s.logger.Error().Err(err).Msg("failed to encode advertised refs")
		}

	default:
		http.Error(w, "unsupported service", http.StatusBadRequest)
	}
}

func (s *Server) handleGitUploadPack(w http.ResponseWriter, r *http.Request, repoName string) {
	stor := s.store.GetGitStorage(repoName)
	if stor == nil {
		http.NotFound(w, r)
		return
	}

	loader := &storeLoader{store: s.store}
	server := gitserver.NewServer(loader)

	ep, err := transport.NewEndpoint(fmt.Sprintf("/%s", repoName))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sess, err := server.NewUploadPackSession(ep, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	upreq := packp.NewUploadPackRequest()
	if err := upreq.Decode(r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := sess.UploadPack(r.Context(), upreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	if err := resp.Encode(w); err != nil {
		s.logger.Error().Err(err).Msg("failed to encode upload-pack response")
	}
}
