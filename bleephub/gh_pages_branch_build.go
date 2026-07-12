package bleephub

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (s *Server) buildPagesBranch(ctx context.Context, repo *Repo, branch, sourcePath string) (string, bool, error) {
	stor := s.store.GetGitStorage(repo.Owner.Login, repo.Name)
	if stor == nil {
		return "", false, fmt.Errorf("Pages source git storage is unavailable")
	}
	ref, err := stor.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		return "", false, fmt.Errorf("resolve Pages source branch %q: %w", branch, err)
	}
	commitSHA := ref.Hash().String()
	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		return commitSHA, false, fmt.Errorf("read Pages source commit %s: %w", ref.Hash(), err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return commitSHA, false, fmt.Errorf("read Pages source tree: %w", err)
	}
	entries, err := collectArchiveEntries(stor, tree)
	if err != nil {
		return commitSHA, false, fmt.Errorf("read Pages source files: %w", err)
	}
	artifact, custom404, err := buildStaticPagesArtifact(entries, sourcePath, commit.Committer.When.UTC())
	if err != nil {
		return commitSHA, false, err
	}
	if _, err := s.publishPagesArtifact(ctx, repo.ID, "github-pages", commitSHA, artifact); err != nil {
		return commitSHA, false, err
	}
	return commitSHA, custom404, nil
}

func pagesLegacySource(site *PagesSite) (string, string, error) {
	if site.BuildType == nil || *site.BuildType != "legacy" {
		return "", "", fmt.Errorf("manual Pages builds require build_type legacy")
	}
	branch, _ := site.Source["branch"].(string)
	if branch == "" {
		return "", "", fmt.Errorf("Pages source branch is required")
	}
	sourcePath, _ := site.Source["path"].(string)
	if sourcePath == "" {
		sourcePath = "/"
	}
	if sourcePath != "/" && sourcePath != "/docs" {
		return "", "", fmt.Errorf("Pages source path must be / or /docs")
	}
	return branch, sourcePath, nil
}

func buildStaticPagesArtifact(entries []archiveEntry, sourcePath string, when time.Time) ([]byte, bool, error) {
	prefix := strings.TrimPrefix(sourcePath, "/")
	if prefix != "" {
		prefix += "/"
	}
	selected := make([]archiveEntry, 0, len(entries))
	var total int64
	hasNoJekyll := false
	custom404 := false
	for _, entry := range entries {
		if !strings.HasPrefix(entry.path, prefix) {
			continue
		}
		name := strings.TrimPrefix(entry.path, prefix)
		if name == "" || strings.HasPrefix(name, ".git/") {
			continue
		}
		if entry.mode == filemode.Symlink || entry.mode == filemode.Submodule {
			return nil, false, fmt.Errorf("Pages source contains unsupported link %q", entry.path)
		}
		if cleanPagesArchivePath(name) != path.Clean(name) {
			return nil, false, fmt.Errorf("Pages source contains unsafe path %q", entry.path)
		}
		hasNoJekyll = hasNoJekyll || name == ".nojekyll"
		custom404 = custom404 || name == "404.html"
		total += int64(len(entry.content))
		if total > maxPagesArtifactSize {
			return nil, false, fmt.Errorf("Pages source exceeds 10 GB")
		}
		selected = append(selected, archiveEntry{path: name, mode: entry.mode, content: entry.content})
	}
	if len(selected) == 0 {
		return nil, false, fmt.Errorf("Pages source path %s contains no files", sourcePath)
	}
	if !hasNoJekyll {
		return nil, false, fmt.Errorf("Pages source requires the GitHub Pages Jekyll build runtime because .nojekyll is absent")
	}
	var artifact bytes.Buffer
	tw := tar.NewWriter(&artifact)
	for _, entry := range selected {
		mode := int64(0o644)
		if entry.mode == filemode.Executable {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: entry.path, Mode: mode, Size: int64(len(entry.content)), ModTime: when, Typeflag: tar.TypeReg}); err != nil {
			return nil, false, fmt.Errorf("write Pages artifact header: %w", err)
		}
		if _, err := tw.Write(entry.content); err != nil {
			return nil, false, fmt.Errorf("write Pages artifact content: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, false, fmt.Errorf("close Pages artifact: %w", err)
	}
	return artifact.Bytes(), custom404, nil
}

func (s *Server) publishPagesArtifact(ctx context.Context, repoID int, environment, buildVersion string, artifact []byte) (*PagesDeploymentRecord, error) {
	if s.store.ObjectByteStore == nil {
		return nil, fmt.Errorf("Pages publication requires configured object storage")
	}
	digest := sha256.Sum256(artifact)
	key := pagesArtifactDataKey(repoID, digest)
	if err := s.store.ObjectByteStore.Put(ctx, key, artifact); err != nil {
		return nil, fmt.Errorf("publish Pages artifact: %w", err)
	}
	previous := s.store.latestPublishedPagesDeployment(repoID)
	if previous != nil && previous.ArtifactKey != "" && previous.ArtifactKey != key {
		if err := s.store.ObjectByteStore.Delete(ctx, previous.ArtifactKey); err != nil {
			rollbackErr := s.store.ObjectByteStore.Delete(ctx, key)
			if rollbackErr != nil {
				return nil, fmt.Errorf("replace Pages artifact: delete previous object: %v; roll back new object: %v", err, rollbackErr)
			}
			return nil, fmt.Errorf("replace Pages artifact: delete previous object: %w", err)
		}
	}
	deployment := s.store.CreatePagesDeployment(repoID, environment, buildVersion, "succeed", int64(len(artifact)), fmt.Sprintf("sha256:%x", digest), key)
	return deployment, nil
}
