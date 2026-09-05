//go:build !noui

package aca

import (
	"embed"

	core "github.com/sockerless/backend-core"
)

//go:embed all:dist
var uiAssets embed.FS

func registerUI(s *core.BaseServer) {
	core.RegisterEmbeddedUI(s, uiAssets, "dist")
}
