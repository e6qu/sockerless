package bleephub

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"sort"
	"strings"
)

const maxPagesArtifactSize = int64(10 << 30)
const pagesValidationPath = "\x00validate-pages-artifact"

func pagesArtifactDataKey(repoID int, digest [sha256.Size]byte) string {
	return fmt.Sprintf("pages/sites/%d/%x/artifact", repoID, digest)
}

func (s *Server) registerGHPagesContentRoutes() {
	s.route("GET /pages/{owner}/{repo}/{path...}", s.handlePagesContent)
	s.route("HEAD /pages/{owner}/{repo}/{path...}", s.handlePagesContent)
}

func validatePagesArtifact(data []byte) error {
	_, _, err := readPagesArtifactFile(data, pagesValidationPath)
	if !errors.Is(err, errPagesFileNotFound) {
		return err
	}
	return nil
}

var errPagesFileNotFound = errors.New("pages file not found")

func readPagesArtifactFile(data []byte, requested string) ([]byte, string, error) {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{'P', 'K', 3, 4}) {
		return readPagesZipFile(data, requested)
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, "", fmt.Errorf("open gzip archive: %w", err)
		}
		content, fileName, readErr := readPagesTarFile(zr, requested)
		closeErr := zr.Close()
		if closeErr != nil && (readErr == nil || errors.Is(readErr, errPagesFileNotFound)) {
			return nil, "", fmt.Errorf("close gzip archive: %w", closeErr)
		}
		if readErr != nil {
			return nil, "", readErr
		}
		return content, fileName, nil
	}
	return readPagesTarFile(bytes.NewReader(data), requested)
}

func readPagesZipFile(data []byte, requested string) ([]byte, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", fmt.Errorf("open ZIP archive: %w", err)
	}
	if len(zr.File) == 1 && cleanPagesArchivePath(zr.File[0].Name) == "artifact.tar" {
		file := zr.File[0]
		if !file.Mode().IsRegular() {
			return nil, "", errors.New("artifact.tar is not a regular file")
		}
		r, err := file.Open()
		if err != nil {
			return nil, "", fmt.Errorf("open artifact.tar: %w", err)
		}
		defer r.Close()
		return readPagesTarFile(r, requested)
	}
	candidates := pagesPathCandidates(requested)
	var total int64
	regularFiles := 0
	for _, file := range zr.File {
		name := cleanPagesArchivePath(file.Name)
		if name == "" && !file.FileInfo().IsDir() {
			return nil, "", fmt.Errorf("unsafe ZIP path %q", file.Name)
		}
		mode := file.Mode()
		if !mode.IsRegular() && !file.FileInfo().IsDir() {
			return nil, "", fmt.Errorf("zip entry %q is not a regular file or directory", file.Name)
		}
		if mode.IsRegular() {
			regularFiles++
		}
		if file.UncompressedSize64 > uint64(maxPagesArtifactSize-total) {
			return nil, "", errors.New("pages artifact exceeds 10 GB")
		}
		total += int64(file.UncompressedSize64)
		if requested == pagesValidationPath || !containsPagesPath(candidates, name) {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return nil, "", fmt.Errorf("open ZIP entry %q: %w", name, err)
		}
		content, err := io.ReadAll(r)
		closeErr := r.Close()
		if err != nil {
			return nil, "", fmt.Errorf("read ZIP entry %q: %w", name, err)
		}
		if closeErr != nil {
			return nil, "", fmt.Errorf("close ZIP entry %q: %w", name, closeErr)
		}
		return content, name, nil
	}
	if requested == pagesValidationPath && regularFiles == 0 {
		return nil, "", errors.New("pages artifact contains no regular files")
	}
	return nil, "", errPagesFileNotFound
}

func readPagesTarFile(r io.Reader, requested string) ([]byte, string, error) {
	tr := tar.NewReader(r)
	candidates := pagesPathCandidates(requested)
	var total int64
	regularFiles := 0
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read TAR archive: %w", err)
		}
		name := cleanPagesArchivePath(header.Name)
		if name == "" && header.Name != "." && header.Name != "./" {
			return nil, "", fmt.Errorf("unsafe TAR path %q", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			regularFiles++
			total += header.Size
			if total > maxPagesArtifactSize {
				return nil, "", errors.New("pages artifact exceeds 10 GB")
			}
		case tar.TypeDir:
			continue
		default:
			return nil, "", fmt.Errorf("tar entry %q is not a regular file or directory", header.Name)
		}
		if requested == pagesValidationPath || !containsPagesPath(candidates, name) {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, "", fmt.Errorf("read TAR entry %q: %w", name, err)
		}
		return content, name, nil
	}
	if requested == pagesValidationPath && regularFiles == 0 {
		return nil, "", errors.New("pages artifact contains no regular files")
	}
	return nil, "", errPagesFileNotFound
}

func cleanPagesArchivePath(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") {
		return ""
	}
	name = strings.TrimPrefix(name, "./")
	clean := path.Clean("/" + name)
	if clean == "/" {
		return ""
	}
	clean = strings.TrimPrefix(clean, "/")
	if name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return ""
	}
	return clean
}

func pagesPathCandidates(requested string) []string {
	requested = strings.TrimPrefix(path.Clean("/"+requested), "/")
	if requested == "." || requested == "" {
		return []string{"index.html"}
	}
	candidates := []string{requested}
	if strings.HasSuffix(requested, "/") {
		candidates = append(candidates, requested+"index.html")
	} else {
		candidates = append(candidates, requested+"/index.html")
		if path.Ext(requested) == "" {
			candidates = append(candidates, requested+".html")
		}
	}
	return candidates
}

func containsPagesPath(candidates []string, name string) bool {
	for _, candidate := range candidates {
		if candidate == name {
			return true
		}
	}
	return false
}

func (s *Server) handlePagesContent(w http.ResponseWriter, r *http.Request) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		http.NotFound(w, r)
		return
	}
	s.store.Misc.mu.RLock()
	site := s.store.Misc.pagesByRepo[repo.ID]
	s.store.Misc.mu.RUnlock()
	if site != nil && !site.Public && !canReadRepo(s.store, ghUserFromContext(r.Context()), repo) {
		http.NotFound(w, r)
		return
	}
	deployment := s.store.latestPublishedPagesDeployment(repo.ID)
	if site == nil || site.Status != "built" || deployment == nil || deployment.ArtifactKey == "" {
		http.NotFound(w, r)
		return
	}
	if s.store.ObjectByteStore == nil {
		http.Error(w, "Pages content requires configured object storage", http.StatusInternalServerError)
		return
	}
	archive, err := s.store.ObjectByteStore.Get(r.Context(), deployment.ArtifactKey)
	if err != nil {
		http.Error(w, "read Pages content: "+err.Error(), http.StatusBadGateway)
		return
	}
	pathSegments := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	requested := ""
	if len(pathSegments) > 3 {
		requested = strings.Join(pathSegments[3:], "/")
	}
	content, fileName, err := readPagesArtifactFile(archive, requested)
	status := http.StatusOK
	if errors.Is(err, errPagesFileNotFound) {
		content, fileName, err = readPagesArtifactFile(archive, "404.html")
		status = http.StatusNotFound
	}
	if err != nil {
		if errors.Is(err, errPagesFileNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "read Pages artifact: "+err.Error(), http.StatusInternalServerError)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(fileName))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", `"`+strings.TrimPrefix(deployment.ArtifactSHA, "sha256:")+`"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(content)
	}
}

func (st *Store) latestPublishedPagesDeployment(repoID int) *PagesDeploymentRecord {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var deployments []*PagesDeploymentRecord
	for _, deployment := range st.PagesDeployments[repoID] {
		if deployment.Status == "succeed" && deployment.ArtifactKey != "" {
			deployments = append(deployments, deployment)
		}
	}
	if len(deployments) == 0 {
		return nil
	}
	sort.Slice(deployments, func(i, j int) bool { return deployments[i].ID > deployments[j].ID })
	copy := *deployments[0]
	return &copy
}

func (st *Store) deletePagesPublicationData(ctx context.Context, repoID int) error {
	st.mu.RLock()
	keys, hasDeployments := st.pagesPublicationKeysLocked(repoID)
	st.mu.RUnlock()
	return st.deletePagesPublicationKeys(ctx, keys, hasDeployments)
}

func (st *Store) deletePagesPublicationDataLocked(ctx context.Context, repoID int) error {
	keys, hasDeployments := st.pagesPublicationKeysLocked(repoID)
	return st.deletePagesPublicationKeys(ctx, keys, hasDeployments)
}

func (st *Store) pagesPublicationKeysLocked(repoID int) (map[string]struct{}, bool) {
	deployments := st.PagesDeployments[repoID]
	keys := map[string]struct{}{}
	for _, deployment := range deployments {
		if deployment.ArtifactKey != "" {
			keys[deployment.ArtifactKey] = struct{}{}
		}
	}
	return keys, len(deployments) > 0
}

func (st *Store) deletePagesPublicationKeys(ctx context.Context, keys map[string]struct{}, hasDeployments bool) error {
	if st.ObjectByteStore == nil {
		if !hasDeployments {
			return nil
		}
		return errors.New("pages publication deletion requires configured object storage")
	}
	for key := range keys {
		if err := st.ObjectByteStore.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete Pages artifact %s: %w", key, err)
		}
	}
	return nil
}
