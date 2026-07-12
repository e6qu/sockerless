package bleephub

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const maxPagesJekyllOutput = 1 << 20

type pagesJekyllOutput struct {
	bytes.Buffer
	truncated bool
}

func (output *pagesJekyllOutput) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := maxPagesJekyllOutput - output.Len()
	if remaining <= 0 {
		output.truncated = true
		return originalLength, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		output.truncated = true
	}
	_, _ = output.Buffer.Write(data)
	return originalLength, nil
}

func (output *pagesJekyllOutput) message() string {
	message := strings.TrimSpace(output.String())
	if output.truncated {
		message += "\n[GitHub Pages Jekyll output truncated at 1 MiB]"
	}
	return message
}

func (s *Server) buildPagesBranch(ctx context.Context, repo *Repo, branch, sourcePath string) (string, bool, error) {
	stor, _ := s.store.GitStorageForRepoID(repo.ID)
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
	selected, hasNoJekyll, err := selectPagesSourceEntries(entries, sourcePath)
	if err != nil {
		return commitSHA, false, err
	}
	var artifact []byte
	var custom404 bool
	if hasNoJekyll {
		artifact, custom404, err = archivePagesEntries(selected, commit.Committer.When.UTC())
	} else {
		artifact, custom404, err = s.buildJekyllPagesArtifact(ctx, repo, selected, commit.Committer.When.UTC())
	}
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

func selectPagesSourceEntries(entries []archiveEntry, sourcePath string) ([]archiveEntry, bool, error) {
	prefix := strings.TrimPrefix(sourcePath, "/")
	if prefix != "" {
		prefix += "/"
	}
	selected := make([]archiveEntry, 0, len(entries))
	var total int64
	hasNoJekyll := false
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
		total += int64(len(entry.content))
		if total > maxPagesArtifactSize {
			return nil, false, fmt.Errorf("Pages source exceeds 10 GB")
		}
		selected = append(selected, archiveEntry{path: name, mode: entry.mode, content: entry.content})
	}
	if len(selected) == 0 {
		return nil, false, fmt.Errorf("Pages source path %s contains no files", sourcePath)
	}
	return selected, hasNoJekyll, nil
}

func archivePagesEntries(entries []archiveEntry, when time.Time) ([]byte, bool, error) {
	var artifact bytes.Buffer
	tw := tar.NewWriter(&artifact)
	custom404 := false
	for _, entry := range entries {
		custom404 = custom404 || entry.path == "404.html"
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

func (s *Server) buildJekyllPagesArtifact(ctx context.Context, repo *Repo, entries []archiveEntry, when time.Time) ([]byte, bool, error) {
	workspace, err := os.MkdirTemp("", "bleephub-pages-jekyll-*")
	if err != nil {
		return nil, false, fmt.Errorf("create GitHub Pages Jekyll workspace: %w", err)
	}
	defer os.RemoveAll(workspace)
	sourceDir := filepath.Join(workspace, "source")
	destinationDir := filepath.Join(workspace, "site")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("create GitHub Pages Jekyll source: %w", err)
	}
	for _, entry := range entries {
		target := filepath.Join(sourceDir, filepath.FromSlash(entry.path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, false, fmt.Errorf("create GitHub Pages source directory: %w", err)
		}
		mode := fs.FileMode(0o644)
		if entry.mode == filemode.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(target, entry.content, mode); err != nil {
			return nil, false, fmt.Errorf("write GitHub Pages source %q: %w", entry.path, err)
		}
	}
	cmd := exec.CommandContext(ctx, s.pagesJekyllExecutable, "build", "--safe", "--source", sourceDir, "--destination", destinationDir, "--trace")
	cmd.Env = append(os.Environ(), "JEKYLL_ENV=production", "PAGES_REPO_NWO="+repo.FullName)
	output := &pagesJekyllOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	if err != nil {
		return nil, false, fmt.Errorf("GitHub Pages Jekyll build failed: %w: %s", err, output.message())
	}
	generated, err := collectPagesOutput(destinationDir)
	if err != nil {
		return nil, false, err
	}
	return archivePagesEntries(generated, when)
}

func collectPagesOutput(root string) ([]archiveEntry, error) {
	entries := []archiveEntry{}
	var total int64
	err := filepath.WalkDir(root, func(filePath string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root || dirEntry.IsDir() {
			return nil
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("GitHub Pages Jekyll output %q is not a regular file", filePath)
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		total += int64(len(content))
		if total > maxPagesArtifactSize {
			return fmt.Errorf("GitHub Pages Jekyll output exceeds 10 GB")
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		entries = append(entries, archiveEntry{path: filepath.ToSlash(relative), mode: filemode.Regular, content: content})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read GitHub Pages Jekyll output: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("GitHub Pages Jekyll build produced no files")
	}
	return entries, nil
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
