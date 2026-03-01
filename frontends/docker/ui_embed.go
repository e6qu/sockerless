//go:build !noui

package frontend

import (
	"embed"
	"io/fs"
	"net/http"

	core "github.com/sockerless/backend-core"
)

//go:embed all:dist
var uiAssets embed.FS

func registerUI(m *MgmtServer) {
	sub, err := fs.Sub(uiAssets, "dist")
	if err != nil {
		m.logger.Warn().Err(err).Msg("failed to load embedded UI assets")
		return
	}
	m.mux.Handle("/ui/", core.SPAHandler(sub, "/ui/"))
	m.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	m.logger.Info().Msg("UI registered at /ui/")
}
