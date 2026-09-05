package core

import (
	"embed"
	"io/fs"
)

// RegisterEmbeddedUI serves the backend's embedded console from the given
// subdirectory of assets. The embed directive itself has to live in the
// backend package, next to the `dist/` it embeds; everything after that
// is the same for every backend.
func RegisterEmbeddedUI(s *BaseServer, assets embed.FS, dir string) {
	sub, err := fs.Sub(assets, dir)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("failed to load embedded UI assets")
		return
	}
	s.RegisterUI(sub)
}
