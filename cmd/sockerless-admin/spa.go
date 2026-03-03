//go:build !noui

package main

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler returns an http.Handler that serves a single-page application.
// Files that exist are served directly; all other paths fall back to index.html.
func spaHandler(fsys fs.FS, pathPrefix string) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	stripped := http.StripPrefix(pathPrefix, fileServer)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, pathPrefix)
		if reqPath == "" {
			reqPath = "."
		}
		reqPath = path.Clean(reqPath)

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

		rs, ok := indexFile.(readSeeker)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
	})
}
