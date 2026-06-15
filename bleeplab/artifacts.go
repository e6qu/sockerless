package bleeplab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
)

// artifacts.go stores CI job artifact archives, object-store-backed exactly
// like bleeplab's git storage (and bleephub): the backend is chosen by env —
// an S3-compatible object store, a filesystem dir, or in-memory. A real
// gitlab-runner uploads a job's artifacts (`POST /api/v4/jobs/:id/artifacts`)
// and downloads a dependency job's artifacts (`GET /api/v4/jobs/:id/artifacts`)
// exactly as against gitlab.com — bleeplab differs only in coordinates.

// artifactStore persists a job's artifact archive by key.
type artifactStore interface {
	Put(key string, data []byte) error
	Get(key string) ([]byte, bool, error)
}

// getArtifactStore lazily builds the artifact store, preferring an S3 object
// store, then a filesystem dir (BLEEPLAB_ARTIFACTS_DIR), then in-memory — the
// same precedence as the git storage.
func (s *Server) getArtifactStore(ctx context.Context) (artifactStore, error) {
	s.artifactsOnce.Do(func() {
		s3fs, err := getS3FS(ctx)
		if err != nil {
			s.artifactsErr = err
			return
		}
		if s3fs != nil {
			chrooted, cerr := s3fs.Chroot("artifacts")
			if cerr != nil {
				s.artifactsErr = fmt.Errorf("s3 chroot artifacts: %w", cerr)
				return
			}
			s.artifactsStore = &billyArtifactStore{fs: chrooted}
			return
		}
		if dir := os.Getenv("BLEEPLAB_ARTIFACTS_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				s.artifactsErr = fmt.Errorf("mkdir %s: %w", dir, err)
				return
			}
			s.artifactsStore = &billyArtifactStore{fs: osfs.New(dir)}
			return
		}
		s.artifactsStore = &memArtifactStore{m: map[string][]byte{}}
	})
	return s.artifactsStore, s.artifactsErr
}

func artifactKey(jobID int) string { return strconv.Itoa(jobID) + ".zip" }

// billyArtifactStore backs artifacts on a go-billy filesystem (S3 or osfs).
type billyArtifactStore struct{ fs billy.Filesystem }

func (b *billyArtifactStore) Put(key string, data []byte) error {
	if dir := path.Dir(key); dir != "." && dir != "/" {
		if err := b.fs.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	f, err := b.fs.Create(key)
	if err != nil {
		return fmt.Errorf("create %s: %w", key, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", key, err)
	}
	return f.Close()
}

func (b *billyArtifactStore) Get(key string) ([]byte, bool, error) {
	f, err := b.fs.Open(key)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("open %s: %w", key, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", key, err)
	}
	return data, true, nil
}

// memArtifactStore backs artifacts in memory.
type memArtifactStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (m *memArtifactStore) Put(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.m[key] = cp
	return nil
}

func (m *memArtifactStore) Get(key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.m[key]
	return data, ok, nil
}

// handleArtifactUpload stores a job's artifact archive
// (`POST /api/v4/jobs/:id/artifacts`). gitlab-runner uploads a multipart form
// whose `file` field is the zip archive.
func (s *Server) handleArtifactUpload(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	if tok := r.Header.Get("JOB-TOKEN"); tok != "" && tok != job.Token {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "403 Forbidden"})
		return
	}

	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, hdr, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing artifact file part: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		if data, err = io.ReadAll(f); err != nil {
			http.Error(w, "read artifact: "+err.Error(), http.StatusBadRequest)
			return
		}
		if hdr.Filename != "" {
			job.mu.Lock()
			job.ArtifactFilename = hdr.Filename
			job.mu.Unlock()
		}
	} else {
		var err error
		if data, err = io.ReadAll(r.Body); err != nil {
			http.Error(w, "read artifact: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	store, err := s.getArtifactStore(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := store.Put(artifactKey(id), data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job.mu.Lock()
	job.ArtifactSize = int64(len(data))
	if job.ArtifactFilename == "" {
		job.ArtifactFilename = "artifacts.zip"
	}
	job.mu.Unlock()
	s.logger.Info().Int("job", id).Int("bytes", len(data)).Msg("artifact uploaded")
	writeJSON(w, http.StatusCreated, map[string]any{"message": "201 Created"})
}

// handleArtifactDownload serves a job's artifact archive
// (`GET /api/v4/jobs/:id/artifacts`). A dependent job downloads it with the
// artifact-owning job's token, advertised in the job response's `dependencies`.
func (s *Server) handleArtifactDownload(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	if tok := r.Header.Get("JOB-TOKEN"); tok != "" && tok != job.Token {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "403 Forbidden"})
		return
	}
	store, err := s.getArtifactStore(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, ok, err := store.Get(artifactKey(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "404 artifact not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
