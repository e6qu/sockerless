package bleephub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-git/go-billy/v5/helper/polyfill"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	gitStorage "github.com/go-git/go-git/v5/storage"
	gitFilesystem "github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/memory"
)

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

	endpoint := os.Getenv("BLEEPHUB_S3_ENDPOINT")
	bucket := os.Getenv("BLEEPHUB_S3_BUCKET")
	if bucket == "" {
		return nil, nil
	}

	prefix := os.Getenv("BLEEPHUB_S3_PREFIX")
	fs, err := newS3FS(ctx, endpoint, bucket, prefix)
	if err != nil {
		s3FSErr = err
		return nil, err
	}
	s3FSCache = fs
	return fs, nil
}

func GitDataDir() string {
	return os.Getenv("BLEEPHUB_GIT_DIR")
}

func IsS3GitStorage() bool {
	return os.Getenv("BLEEPHUB_S3_BUCKET") != ""
}

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
	fs := osfs.New(repoDir)
	return gitFilesystem.NewStorage(fs, cache.NewObjectLRUDefault()), nil
}

func openOrInitGitStorage(ctx context.Context, fullName string) (gitStorage.Storer, error) {
	stor, err := newGitStorage(ctx, fullName)
	if err != nil {
		return nil, err
	}
	_, err = git.Init(stor, nil)
	if err != nil && !errors.Is(err, git.ErrRepositoryAlreadyExists) {
		return nil, fmt.Errorf("git init %s: %w", fullName, err)
	}
	return stor, nil
}
