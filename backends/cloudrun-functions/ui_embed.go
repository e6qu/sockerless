//go:build !noui

package gcf

import (
	"embed"
	"io/fs"

	core "github.com/sockerless/backend-core"
)

//go:embed all:dist
var uiAssets embed.FS

func registerUI(s *core.BaseServer) {
	sub, err := fs.Sub(uiAssets, "dist")
	if err != nil {
		s.Logger.Warn().Err(err).Msg("failed to load embedded UI assets")
		return
	}
	s.RegisterUI(sub)
}
