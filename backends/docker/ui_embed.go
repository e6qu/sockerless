//go:build !noui

package docker

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
		return
	}
	s.RegisterUI(sub)
}
