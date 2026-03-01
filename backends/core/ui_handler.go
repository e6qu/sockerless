package core

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPAHandler returns an http.Handler that serves a single-page application
// from the given filesystem. Files that exist are served directly; all other
// paths fall back to index.html for client-side routing.
func SPAHandler(fsys fs.FS, pathPrefix string) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	stripped := http.StripPrefix(pathPrefix, fileServer)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path after stripping the prefix
		reqPath := strings.TrimPrefix(r.URL.Path, pathPrefix)
		if reqPath == "" {
			reqPath = "."
		}
		reqPath = path.Clean(reqPath)

		// Try to open the requested file
		f, err := fsys.Open(reqPath)
		if err == nil {
			stat, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !stat.IsDir() {
				// File exists — serve it directly
				stripped.ServeHTTP(w, r)
				return
			}
		}

		// File not found or is a directory — serve index.html (SPA fallback)
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

		http.ServeContent(w, r, "index.html", stat.ModTime(), indexFile.(readSeeker))
	})
}

// readSeeker combines io.ReadSeeker for http.ServeContent.
type readSeeker interface {
	Read(p []byte) (n int, err error)
	Seek(offset int64, whence int) (int64, error)
}
