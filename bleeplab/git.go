package bleeplab

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// git.go serves each project as a real git repository over smart-HTTP, exactly
// as gitlab.com does — a real gitlab-runner clones `git_info.repo_url` to
// materialize CI_PROJECT_DIR. The repository objects live in the same
// object-store-backed go-git Storer bleephub uses (see git_storage.go), so
// bleeplab's git is "based on object store, just like bleephub". bleeplab is a
// faithful GitLab slice: it accepts the runner's `gitlab-ci-token` exactly as
// GitLab does and serves the standard `/info/refs` + `git-upload-pack` /
// `git-receive-pack` endpoints — no sockerless-aware special-casing.

// ensureGitStorageLocked returns the cached go-git Storer for a project path,
// creating it on first use. Caller holds s.mu.
func (s *Server) ensureGitStorageLocked(ctx context.Context, repoPath string) (gitStorage.Storer, error) { //nolint:ireturn
	if st, ok := s.gitStorages[repoPath]; ok {
		return st, nil
	}
	st, err := newGitStorage(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	s.gitStorages[repoPath] = st
	return st, nil
}

// gitStorageFor returns the Storer for a project path, or nil if unknown.
func (s *Server) gitStorageFor(repoPath string) gitStorage.Storer { //nolint:ireturn
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gitStorages[repoPath]
}

// seedCommit applies a set of file actions to a project's git repo as a single
// commit on `branch`, mirroring GitLab's commits API (create/update). It
// returns the new commit SHA. Existing branch content is preserved (the
// actions are applied on top of HEAD).
func (s *Server) seedCommit(ctx context.Context, repoPath, branch string, files map[string]string) (string, error) {
	s.mu.Lock()
	stor, err := s.ensureGitStorageLocked(ctx, repoPath)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	repo, err := git.InitWithOptions(stor, memfs.New(), git.InitOptions{DefaultBranch: branchRef})
	if errors.Is(err, git.ErrRepositoryAlreadyExists) {
		repo, err = git.Open(stor, memfs.New())
	}
	if err != nil {
		return "", fmt.Errorf("init repo %s: %w", repoPath, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("worktree %s: %w", repoPath, err)
	}

	// Preserve existing branch content so each commit is additive (GitLab's
	// per-action create/update semantics), populating the fresh worktree from
	// HEAD when the branch already exists.
	if ref, err := repo.Reference(branchRef, true); err == nil {
		if err := wt.Reset(&git.ResetOptions{Commit: ref.Hash(), Mode: git.HardReset}); err != nil {
			return "", fmt.Errorf("reset worktree %s: %w", repoPath, err)
		}
	}

	for fp, content := range files {
		if dir := path.Dir(fp); dir != "." && dir != "/" {
			if err := wt.Filesystem.MkdirAll(dir, 0o755); err != nil {
				return "", fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
		f, err := wt.Filesystem.Create(fp)
		if err != nil {
			return "", fmt.Errorf("create %s: %w", fp, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("write %s: %w", fp, err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("close %s: %w", fp, err)
		}
		if _, err := wt.Add(fp); err != nil {
			return "", fmt.Errorf("add %s: %w", fp, err)
		}
	}

	sig := &object.Signature{Name: "bleeplab", Email: "bleeplab@sockerless.local", When: time.Now()}
	h, err := wt.Commit("Update "+joinSorted(files), &git.CommitOptions{Author: sig, Committer: sig, AllowEmptyCommits: true})
	if err != nil {
		return "", fmt.Errorf("commit %s: %w", repoPath, err)
	}
	return h.String(), nil
}

func joinSorted(files map[string]string) string {
	names := make([]string, 0, len(files))
	for k := range files {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// storeLoader resolves a transport.Endpoint path to its go-git Storer.
type storeLoader struct{ s *Server }

func (l *storeLoader) Load(ep *transport.Endpoint) (storer.Storer, error) { //nolint:ireturn
	st := l.s.gitStorageFor(normalizeRepoPath(ep.Path))
	if st == nil {
		return nil, transport.ErrRepositoryNotFound
	}
	return st, nil
}

// normalizeRepoPath turns "/root/demo.git" (or "root/demo") into "root/demo".
func normalizeRepoPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	return strings.TrimSuffix(p, ".git")
}

// tryHandleGitRequest serves a git smart-HTTP request if the path matches one.
// GitLab URLs look like /{namespace}/{project}.git/info/refs,
// /{namespace}/{project}.git/git-upload-pack, etc. Returns true if handled.
func (s *Server) tryHandleGitRequest(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path
	switch {
	case strings.HasSuffix(p, "/info/refs") && r.Method == http.MethodGet:
		s.handleGitInfoRefs(w, r, normalizeRepoPath(strings.TrimSuffix(p, "/info/refs")))
		return true
	case strings.HasSuffix(p, "/git-upload-pack") && r.Method == http.MethodPost:
		s.handleGitUploadPack(w, r, normalizeRepoPath(strings.TrimSuffix(p, "/git-upload-pack")))
		return true
	case strings.HasSuffix(p, "/git-receive-pack") && r.Method == http.MethodPost:
		s.handleGitReceivePack(w, r, normalizeRepoPath(strings.TrimSuffix(p, "/git-receive-pack")))
		return true
	}
	return false
}

func (s *Server) gitEndpoint(repoPath string) (*transport.Endpoint, error) {
	return transport.NewEndpoint("/" + repoPath)
}

func (s *Server) handleGitInfoRefs(w http.ResponseWriter, r *http.Request, repoPath string) {
	if s.gitStorageFor(repoPath) == nil {
		http.NotFound(w, r)
		return
	}
	service := r.URL.Query().Get("service")
	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "service parameter required", http.StatusBadRequest)
		return
	}

	ep, err := s.gitEndpoint(repoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	server := gitserver.NewServer(&storeLoader{s: s})

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	enc := pktline.NewEncoder(w)
	if err := enc.Encodef("# service=%s\n", service); err != nil {
		s.logger.Debug().Err(err).Msg("git-http: advertisement encode failed")
		return
	}
	if err := enc.Flush(); err != nil {
		s.logger.Debug().Err(err).Msg("git-http: advertisement flush failed")
		return
	}

	var sess transport.Session
	if service == "git-upload-pack" {
		sess, err = server.NewUploadPackSession(ep, nil)
	} else {
		sess, err = server.NewReceivePackSession(ep, nil)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	info, err := sess.AdvertisedReferencesContext(r.Context())
	if err != nil {
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			_ = enc.Flush()
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := info.Encode(w); err != nil {
		s.logger.Error().Err(err).Msg("git-http: failed to encode advertised refs")
	}
}

func (s *Server) handleGitUploadPack(w http.ResponseWriter, r *http.Request, repoPath string) {
	if s.gitStorageFor(repoPath) == nil {
		http.NotFound(w, r)
		return
	}
	ep, err := s.gitEndpoint(repoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	server := gitserver.NewServer(&storeLoader{s: s})
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
		s.logger.Error().Err(err).Msg("git-http: failed to encode upload-pack response")
	}
}

func (s *Server) handleGitReceivePack(w http.ResponseWriter, r *http.Request, repoPath string) {
	stor := s.gitStorageFor(repoPath)
	if stor == nil {
		http.NotFound(w, r)
		return
	}
	ep, err := s.gitEndpoint(repoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	server := gitserver.NewServer(&storeLoader{s: s})
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
	// A clean/short EOF is the benign end-of-stream for an empty or
	// flush-only push; any OTHER ReceivePack error is a real failure and
	// must not be swallowed by a substring match on "EOF".
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Keep HEAD pointing at a branch that exists after the push.
	if head, herr := stor.Reference(plumbing.HEAD); herr != nil || (head.Type() == plumbing.SymbolicReference && refMissing(stor, head.Target())) {
		for _, b := range []string{"main", "master"} {
			ref := plumbing.NewBranchReferenceName(b)
			if _, e := stor.Reference(ref); e == nil {
				_ = stor.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, ref))
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	if result != nil {
		if err := result.Encode(w); err != nil {
			s.logger.Error().Err(err).Msg("git-http: failed to encode receive-pack response")
		}
	}
}

func refMissing(stor gitStorage.Storer, name plumbing.ReferenceName) bool {
	_, err := stor.Reference(name)
	return err != nil
}
