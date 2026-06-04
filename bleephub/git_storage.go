package bleephub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gitStorage "github.com/go-git/go-git/v5/storage"
	gitFilesystem "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitDataDir returns the configured root directory for persistent git storage.
// Set BLEEPHUB_GIT_DIR to a directory path (e.g. "/data/git") to enable
// filesystem-backed repos that survive restarts and can be mounted as a
// Docker volume. When empty, repos use in-memory storage (ephemeral).
func GitDataDir() string {
	return os.Getenv("BLEEPHUB_GIT_DIR")
}

// newGitStorage allocates storage for a repo identified by fullName ("owner/repo").
// Returns in-memory storage when gitDir is empty, filesystem storage otherwise.
// Does NOT initialise the git repo itself (no git.Init call).
func newGitStorage(gitDir, fullName string) (gitStorage.Storer, error) {
	if gitDir == "" {
		return memory.NewStorage(), nil
	}
	repoDir := filepath.Join(gitDir, filepath.FromSlash(fullName))
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", repoDir, err)
	}
	fs := osfs.New(repoDir)
	return gitFilesystem.NewStorage(fs, cache.NewObjectLRUDefault()), nil
}

// openOrInitGitStorage returns storage for a repo, initialising the git
// repository structure if it does not yet exist.
func openOrInitGitStorage(gitDir, fullName string) (gitStorage.Storer, error) {
	stor, err := newGitStorage(gitDir, fullName)
	if err != nil {
		return nil, err
	}
	_, err = git.Init(stor, nil)
	if err != nil && !errors.Is(err, git.ErrRepositoryAlreadyExists) {
		return nil, fmt.Errorf("git init %s: %w", fullName, err)
	}
	return stor, nil
}
