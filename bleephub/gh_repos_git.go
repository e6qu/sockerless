package bleephub

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
