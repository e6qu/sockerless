//go:build !noui

package docker

import (
	"embed"
	"io/fs"
	"net/http"

	core "github.com/sockerless/backend-core"
)

//go:embed all:dist
var uiAssets embed.FS

func registerUI(s *Server) {
	sub, err := fs.Sub(uiAssets, "dist")
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to load embedded UI assets")
		return
	}
	s.mux.Handle("/ui/", core.SPAHandler(sub, "/ui/"))
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	s.logger.Info().Msg("UI registered at /ui/")
}
