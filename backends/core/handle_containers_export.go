package core

import (
	"net/http"

	"github.com/sockerless/api"
)

// handleContainerExport returns a tar archive of the container's filesystem.
func (s *BaseServer) handleContainerExport(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	c, ok := s.ResolveContainerAuto(r.Context(), ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	dctx := DriverContext{
		Ctx:       r.Context(),
		Container: c,
		Backend:   s.Desc.Driver,
		Logger:    s.Logger,
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	if err := s.Typed.FSExport.Export(dctx, w); err != nil {
		s.Logger.Debug().Err(err).Msg("container export write error after headers sent")
	}
}
