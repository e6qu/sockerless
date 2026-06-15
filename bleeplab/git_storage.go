package bleeplab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-billy/v5/helper/polyfill"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gitStorage "github.com/go-git/go-git/v5/storage"
	gitFilesystem "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Git repositories are stored exactly like bleephub's: a go-git Storer whose
// backend is chosen by env — an S3-compatible object store, a filesystem
// directory, or in-memory. The same project repo a real gitlab-runner clones
// is served over smart-HTTP from this storage (see git.go), so bleeplab's git
// is "based on object store, just like bleephub".

var (
	s3FSMu     sync.Mutex
	s3FSCache  *s3FS
	s3FSErr    error
	s3FSInited bool
)

func getS3FS(ctx context.Context) (*s3FS, error) {
	s3FSMu.Lock()
	defer s3FSMu.Unlock()
	if s3FSInited {
		return s3FSCache, s3FSErr
	}
	s3FSInited = true

	endpoint := os.Getenv("BLEEPLAB_S3_ENDPOINT")
	bucket := os.Getenv("BLEEPLAB_S3_BUCKET")
	if bucket == "" {
		return nil, nil
	}

	prefix := os.Getenv("BLEEPLAB_S3_PREFIX")
	fs, err := newS3FS(ctx, endpoint, bucket, prefix)
	if err != nil {
		s3FSErr = err
		return nil, err
	}
	s3FSCache = fs
	return fs, nil
}

// GitDataDir is the filesystem directory for git repos (BLEEPLAB_GIT_DIR).
func GitDataDir() string { return os.Getenv("BLEEPLAB_GIT_DIR") }

// IsS3GitStorage reports whether git repos live in an S3 object store.
func IsS3GitStorage() bool { return os.Getenv("BLEEPLAB_S3_BUCKET") != "" }

// newGitStorage builds the go-git Storer for a project path, preferring an S3
// object store, then a filesystem dir, then in-memory — the same precedence as
// bleephub.
func newGitStorage(ctx context.Context, fullName string) (gitStorage.Storer, error) {
	s3fs, err := getS3FS(ctx)
	if err != nil {
		return nil, err
	}
	if s3fs != nil {
		chrooted, err := s3fs.Chroot(fullName)
		if err != nil {
			return nil, fmt.Errorf("s3 chroot %s: %w", fullName, err)
		}
		return gitFilesystem.NewStorage(polyfill.New(chrooted), cache.NewObjectLRUDefault()), nil
	}

	gitDir := GitDataDir()
	if gitDir == "" {
		return memory.NewStorage(), nil
	}
	repoDir := filepath.Join(gitDir, filepath.FromSlash(fullName))
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", repoDir, err)
	}
	return gitFilesystem.NewStorage(osfs.New(repoDir), cache.NewObjectLRUDefault()), nil
}
