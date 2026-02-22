package bleephub

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
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

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return nil, transport.ErrRepositoryNotFound
	}

	s := l.store.GetGitStorage(parts[0], parts[1])
	if s == nil {
		return nil, transport.ErrRepositoryNotFound
	}
	return s, nil
}

// tryHandleGitRequest checks if the request is a git smart HTTP request and handles it.
// Returns true if handled, false otherwise.
// Git URLs look like: /{owner}/{repo}.git/info/refs, /{owner}/{repo}.git/git-upload-pack, etc.
func (s *Server) tryHandleGitRequest(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path

	// Match /{owner}/{repo}/info/refs or /{owner}/{repo}.git/info/refs
	if strings.HasSuffix(path, "/info/refs") && r.Method == "GET" {
		repoPath := strings.TrimSuffix(path, "/info/refs")
		owner, repo := splitRepoPath(repoPath)
		if owner != "" && repo != "" {
			s.handleGitInfoRefs(w, r, owner, repo)
			return true
		}
	}

	// Match /{owner}/{repo}/git-upload-pack
	if strings.HasSuffix(path, "/git-upload-pack") && r.Method == "POST" {
		repoPath := strings.TrimSuffix(path, "/git-upload-pack")
		owner, repo := splitRepoPath(repoPath)
		if owner != "" && repo != "" {
			s.handleGitUploadPack(w, r, owner, repo)
			return true
		}
	}

	// Match /{owner}/{repo}/git-receive-pack
	if strings.HasSuffix(path, "/git-receive-pack") && r.Method == "POST" {
		repoPath := strings.TrimSuffix(path, "/git-receive-pack")
		owner, repo := splitRepoPath(repoPath)
		if owner != "" && repo != "" {
			s.handleGitReceivePack(w, r, owner, repo)
			return true
		}
	}

	return false
}

// splitRepoPath splits "/owner/repo.git" or "/owner/repo" into (owner, repo).
func splitRepoPath(path string) (string, string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	return parts[0], repo
}

func (s *Server) resolveGitRepo(owner, repoName string) *memory.Storage {
	return s.store.GetGitStorage(owner, repoName)
}

func (s *Server) handleGitInfoRefs(w http.ResponseWriter, r *http.Request, owner, repoName string) {
	stor := s.resolveGitRepo(owner, repoName)
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

	ep, err := transport.NewEndpoint(fmt.Sprintf("/%s/%s", owner, repoName))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	// Write pkt-line header
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

	case "git-receive-pack":
		sess, err := server.NewReceivePackSession(ep, nil)
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

func (s *Server) handleGitUploadPack(w http.ResponseWriter, r *http.Request, owner, repoName string) {
	stor := s.resolveGitRepo(owner, repoName)
	if stor == nil {
		http.NotFound(w, r)
		return
	}

	loader := &storeLoader{store: s.store}
	server := gitserver.NewServer(loader)

	ep, err := transport.NewEndpoint(fmt.Sprintf("/%s/%s", owner, repoName))
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

func (s *Server) handleGitReceivePack(w http.ResponseWriter, r *http.Request, owner, repoName string) {
	stor := s.resolveGitRepo(owner, repoName)
	if stor == nil {
		http.NotFound(w, r)
		return
	}

	loader := &storeLoader{store: s.store}
	server := gitserver.NewServer(loader)

	ep, err := transport.NewEndpoint(fmt.Sprintf("/%s/%s", owner, repoName))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sess, err := server.NewReceivePackSession(ep, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := sess.ReceivePack(r.Context(), req)
	if err != nil {
		if !strings.Contains(err.Error(), "EOF") {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update pushed_at timestamp
	s.store.UpdateRepo(owner, repoName, func(repo *Repo) {
		repo.PushedAt = repo.UpdatedAt
	})

	// Update HEAD to point to a valid branch if the current target doesn't exist
	repo := s.store.GetRepo(owner, repoName)
	if repo != nil {
		needsUpdate := false
		headRef, headErr := stor.Reference(plumbing.HEAD)
		if headErr != nil {
			needsUpdate = true
		} else if headRef.Type() == plumbing.SymbolicReference {
			// HEAD exists but check if its target branch exists
			if _, err := stor.Reference(headRef.Target()); err != nil {
				needsUpdate = true
			}
		}

		if needsUpdate {
			for _, branch := range []string{"main", "master"} {
				ref := plumbing.NewBranchReferenceName(branch)
				if _, err := stor.Reference(ref); err == nil {
					symRef := plumbing.NewSymbolicReference(plumbing.HEAD, ref)
					_ = stor.SetReference(symRef)
					s.store.UpdateRepo(owner, repoName, func(r *Repo) {
						r.DefaultBranch = branch
					})
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	if result != nil {
		if err := result.Encode(w); err != nil {
			s.logger.Error().Err(err).Msg("failed to encode receive-pack response")
		}
	}
}
