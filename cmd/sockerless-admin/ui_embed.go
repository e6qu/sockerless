//go:build !noui

package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed all:dist
var uiAssets embed.FS

func registerUI(mux *http.ServeMux) {
	sub, err := fs.Sub(uiAssets, "dist")
	if err != nil {
		log.Printf("warning: failed to load embedded UI assets: %v", err)
		return
	}
	mux.Handle("/ui/", spaHandler(sub, "/ui/"))
}
