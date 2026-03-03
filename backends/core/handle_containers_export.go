package core

import (
	"archive/tar"
	"net/http"

	"github.com/sockerless/api"
)

// handleContainerExport returns a tar archive of the container's filesystem.
// For synthetic containers (no real filesystem), returns an empty tar.
func (s *BaseServer) handleContainerExport(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)

	rootPath, err := s.Drivers.Filesystem.RootPath(id)
	if err != nil || rootPath == "" {
		// Synthetic container — return empty tar
		tw := tar.NewWriter(w)
		_ = tw.Close()
		return
	}

	if err := createTar(w, rootPath, "."); err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("failed to create export tar")
	}
}
