package bleephub

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
)

func (s *Server) authenticateGitRequest(r *http.Request) *User {
	ctx := s.authenticateRequest(r)
	return ghUserFromContext(ctx)
}

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

func (s *Server) resolveGitRepo(owner, repoName string) storer.Storer { //nolint:ireturn
	return s.store.GetGitStorage(owner, repoName)
}

func (s *Server) handleGitInfoRefs(w http.ResponseWriter, r *http.Request, owner, repoName string) {
	stor := s.resolveGitRepo(owner, repoName)
	if stor == nil {
		http.NotFound(w, r)
		return
	}

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		http.NotFound(w, r)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		http.Error(w, "service parameter required", http.StatusBadRequest)
		return
	}

	user := s.authenticateGitRequest(r)

	switch service {
	case "git-upload-pack":
		if !canReadRepo(s.store, user, repo) {
			w.Header().Set("WWW-Authenticate", `Basic realm="GitHub"`)
			http.Error(w, "401 Authorization Required", http.StatusUnauthorized)
			return
		}
	case "git-receive-pack":
		if !canPushRepo(s.store, user, repo) {
			w.Header().Set("WWW-Authenticate", `Basic realm="GitHub"`)
			if user == nil {
				http.Error(w, "401 Authorization Required", http.StatusUnauthorized)
			} else {
				http.Error(w, "403 Forbidden", http.StatusForbidden)
			}
			return
		}
	default:
		http.Error(w, "unsupported service", http.StatusBadRequest)
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

	// Write pkt-line header. Encode/Flush errors at this point mean the
	// response connection dropped before we sent the advertisement — log
	// at Debug (the client is gone, nothing further to do).
	enc := pktline.NewEncoder(w)
	if err := enc.Encodef("# service=%s\n", service); err != nil {
		s.logger.Debug().Err(err).Str("service", service).Msg("git-http: pkt-line advertisement encode failed (client disconnected?)")
		return
	}
	if err := enc.Flush(); err != nil {
		s.logger.Debug().Err(err).Str("service", service).Msg("git-http: pkt-line advertisement flush failed (client disconnected?)")
		return
	}

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
				if flushErr := enc.Flush(); flushErr != nil {
					s.logger.Debug().Err(flushErr).Str("service", service).Msg("git-http: empty-repo flush failed")
				}
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
				if flushErr := enc.Flush(); flushErr != nil {
					s.logger.Debug().Err(flushErr).Str("service", service).Msg("git-http: empty-repo flush failed")
				}
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := info.Encode(w); err != nil {
			s.logger.Error().Err(err).Msg("failed to encode advertised refs")
		}
	}
}

func (s *Server) handleGitUploadPack(w http.ResponseWriter, r *http.Request, owner, repoName string) {
	stor := s.resolveGitRepo(owner, repoName)
	if stor == nil {
		http.NotFound(w, r)
		return
	}

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		http.NotFound(w, r)
		return
	}

	user := s.authenticateGitRequest(r)
	if !canReadRepo(s.store, user, repo) {
		w.Header().Set("WWW-Authenticate", `Basic realm="GitHub"`)
		http.Error(w, "401 Authorization Required", http.StatusUnauthorized)
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

	requestReader := bufio.NewReader(r.Body)
	empty, err := flushOnlyGitRequest(requestReader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if empty {
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		if err := pktline.NewEncoder(w).Flush(); err != nil {
			s.logger.Error().Err(err).Msg("failed to encode empty upload-pack response")
		}
		return
	}

	upreq := packp.NewUploadPackRequest()
	if err := upreq.Decode(requestReader); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := sess.UploadPack(r.Context(), upreq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// A fetch with no haves is a full clone; count it for the traffic API.
	// The actor identity is the authenticated login, or the remote host for
	// anonymous clones of public repos.
	if len(upreq.Haves) == 0 {
		actor := r.RemoteAddr
		if host, _, splitErr := net.SplitHostPort(r.RemoteAddr); splitErr == nil {
			actor = host
		}
		if user != nil {
			actor = user.Login
		}
		s.store.RecordRepoClone(repo.ID, actor)
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

	repo := s.store.GetRepo(owner, repoName)
	if repo == nil {
		http.NotFound(w, r)
		return
	}

	user := s.authenticateGitRequest(r)
	if !canPushRepo(s.store, user, repo) {
		w.Header().Set("WWW-Authenticate", `Basic realm="GitHub"`)
		if user == nil {
			http.Error(w, "401 Authorization Required", http.StatusUnauthorized)
		} else {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
		}
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
			s.logger.Error().Err(err).Str("repo", owner+"/"+repoName).Msg("git HTTP receive-pack failed")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.afterGitReceivePack(repo, user, req, s.baseURL(r))

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	if result != nil {
		if err := result.Encode(w); err != nil {
			s.logger.Error().Err(err).Msg("failed to encode receive-pack response")
		}
	}
}

func (s *Server) afterGitReceivePack(repo *Repo, user *User, request *packp.ReferenceUpdateRequest, baseURL string) {
	owner, _ := splitRepoPath("/" + repo.FullName)
	stor := s.resolveGitRepo(owner, repo.Name)
	s.store.UpdateRepo(owner, repo.Name, func(updated *Repo) {
		updated.PushedAt = updated.UpdatedAt
	})
	if stor != nil {
		needsUpdate := false
		headRef, headErr := stor.Reference(plumbing.HEAD)
		if headErr != nil {
			needsUpdate = true
		} else if headRef.Type() == plumbing.SymbolicReference {
			_, targetErr := stor.Reference(headRef.Target())
			needsUpdate = targetErr != nil
		}
		if needsUpdate {
			for _, branch := range []string{"main", "master"} {
				ref := plumbing.NewBranchReferenceName(branch)
				if _, err := stor.Reference(ref); err == nil {
					_ = stor.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, ref))
					s.store.UpdateRepo(owner, repo.Name, func(updated *Repo) { updated.DefaultBranch = branch })
					break
				}
			}
		}
	}
	for _, command := range request.Commands {
		s.afterCommittedRefUpdate(repo, user, command.Name.String(), command.Old.String(), command.New.String(), baseURL)
	}
}
