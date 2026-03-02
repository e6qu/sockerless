//go:build !noui

package gitlabhub

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var uiAssets embed.FS

func (s *Server) registerUI() {
	sub, err := fs.Sub(uiAssets, "dist")
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to load embedded UI assets")
		return
	}
	s.mux.Handle("/ui/", spaHandler(sub, "/ui/"))
	s.logger.Info().Msg("UI registered at /ui/")
}

func spaHandler(fsys fs.FS, pathPrefix string) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	stripped := http.StripPrefix(pathPrefix, fileServer)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := r.URL.Path
		if len(reqPath) > len(pathPrefix) {
			reqPath = reqPath[len(pathPrefix):]
		} else {
			reqPath = "."
		}

		f, err := fsys.Open(reqPath)
		if err == nil {
			stat, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !stat.IsDir() {
				stripped.ServeHTTP(w, r)
				return
			}
		}

		indexFile, err := fsys.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = indexFile.Close() }()

		stat, err := indexFile.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		type readSeeker interface {
			Read(p []byte) (n int, err error)
			Seek(offset int64, whence int) (int64, error)
		}
		http.ServeContent(w, r, "index.html", stat.ModTime(), indexFile.(readSeeker))
	})
}
