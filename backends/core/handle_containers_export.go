package core

import (
	"io"
	"net/http"
)

// handleContainerExport returns a tar archive of the container's filesystem.
func (s *BaseServer) handleContainerExport(w http.ResponseWriter, r *http.Request) {
	rc, err := s.self.ContainerExport(r.PathValue("id"))
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}
