package bleephub

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

// Source archives. The /api/v3 tarball/zipball endpoints answer 302 with a
// Location on the codeload-style legacy URL (the same URL shape the tags API
// advertises in tarball_url/zipball_url); the legacy URL streams a real
// archive built from the git tree at that ref, with GitHub's
// "{owner}-{repo}-{shortsha}/" top-level directory.

func (s *Server) handleGetTarball(w http.ResponseWriter, r *http.Request) {
	s.redirectArchive(w, r, "legacy.tar.gz")
}

func (s *Server) handleGetZipball(w http.ResponseWriter, r *http.Request) {
	s.redirectArchive(w, r, "legacy.zip")
}

func (s *Server) redirectArchive(w http.ResponseWriter, r *http.Request, format string) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	ref := strings.Trim(r.PathValue("ref"), "/")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if _, err := resolveGitRef(stor, ref); err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	http.Redirect(w, r, s.baseURL(r)+"/"+repo.FullName+"/"+format+"/"+ref, http.StatusFound)
}

// tryHandleArchiveRequest serves the codeload-style legacy archive URLs
// (/{owner}/{repo}/legacy.tar.gz/{ref} and /{owner}/{repo}/legacy.zip/{ref})
// from the catch-all, beside the git smart HTTP protocol. Returns true when
// the request was an archive download.
func (s *Server) tryHandleArchiveRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 4)
	if len(parts) < 4 {
		return false
	}
	switch parts[2] {
	case "legacy.tar.gz":
		s.serveArchive(w, r, "tar.gz", parts[0], parts[1], parts[3])
		return true
	case "legacy.zip":
		s.serveArchive(w, r, "zip", parts[0], parts[1], parts[3])
		return true
	}
	return false
}

// archiveEntry is one file in an archive, materialized from the git tree.
type archiveEntry struct {
	path    string
	mode    filemode.FileMode
	content []byte
}

func (s *Server) serveArchive(w http.ResponseWriter, r *http.Request, format, owner, name, ref string) {
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		http.NotFound(w, r)
		return
	}
	if repo.Private && !canReadRepo(s.store, s.authenticateGitRequest(r), repo) {
		http.NotFound(w, r)
		return
	}
	stor := s.store.GetGitStorage(owner, name)
	if stor == nil {
		http.NotFound(w, r)
		return
	}
	ref = strings.Trim(ref, "/")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	hash, err := resolveGitRef(stor, ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	commit, err := object.GetCommit(stor, hash)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tree, err := commit.Tree()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entries, err := collectArchiveEntries(stor, tree)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	prefix := fmt.Sprintf("%s-%s-%s/", owner, name, shortSHA(hash))
	filename := strings.TrimSuffix(prefix, "/") + "." + format
	when := commit.Committer.When.UTC()

	switch format {
	case "tar.gz":
		w.Header().Set("Content-Type", "application/x-gzip")
		w.Header().Set("Content-Disposition", `attachment; filename=`+filename)
		if err := writeTarGz(w, prefix, entries, when); err != nil {
			s.logger.Error().Err(err).Str("repo", repo.FullName).Msg("tarball stream failed")
		}
	case "zip":
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename=`+filename)
		if err := writeZip(w, prefix, entries, when); err != nil {
			s.logger.Error().Err(err).Str("repo", repo.FullName).Msg("zipball stream failed")
		}
	}
}

// collectArchiveEntries flattens a git tree into archive entries, sorted by
// path for deterministic archives.
func collectArchiveEntries(stor gitStorage.Storer, tree *object.Tree) ([]archiveEntry, error) {
	flat, err := flattenTree(tree)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(flat))
	for p := range flat {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	entries := make([]archiveEntry, 0, len(paths))
	for _, p := range paths {
		te := flat[p]
		content, err := readBlob(stor, te.Hash)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		entries = append(entries, archiveEntry{path: p, mode: te.Mode, content: content})
	}
	return entries, nil
}

func writeTarGz(w http.ResponseWriter, prefix string, entries []archiveEntry, when time.Time) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     prefix,
		Typeflag: tar.TypeDir,
		Mode:     0o755,
		ModTime:  when,
	}); err != nil {
		return err
	}
	for _, e := range entries {
		switch e.mode {
		case filemode.Symlink:
			if err := tw.WriteHeader(&tar.Header{
				Name:     prefix + e.path,
				Typeflag: tar.TypeSymlink,
				Linkname: string(e.content),
				Mode:     0o777,
				ModTime:  when,
			}); err != nil {
				return err
			}
		default:
			mode := int64(0o644)
			if e.mode == filemode.Executable {
				mode = 0o755
			}
			if err := tw.WriteHeader(&tar.Header{
				Name:     prefix + e.path,
				Typeflag: tar.TypeReg,
				Mode:     mode,
				Size:     int64(len(e.content)),
				ModTime:  when,
			}); err != nil {
				return err
			}
			if _, err := tw.Write(e.content); err != nil {
				return err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeZip(w http.ResponseWriter, prefix string, entries []archiveEntry, when time.Time) error {
	zw := zip.NewWriter(w)
	dirHdr := &zip.FileHeader{Name: prefix, Modified: when}
	dirHdr.SetMode(0o755 | os.ModeDir)
	if _, err := zw.CreateHeader(dirHdr); err != nil {
		return err
	}
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: prefix + e.path, Method: zip.Deflate, Modified: when}
		if e.mode == filemode.Executable {
			hdr.SetMode(0o755)
		} else {
			hdr.SetMode(0o644)
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if _, err := fw.Write(e.content); err != nil {
			return err
		}
	}
	return zw.Close()
}
