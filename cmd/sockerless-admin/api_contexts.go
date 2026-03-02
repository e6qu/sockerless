package main

import "net/http"

// handleContexts returns all CLI contexts from ~/.sockerless/.
func handleContexts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contexts := listContexts()
		if contexts == nil {
			contexts = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, contexts)
	}
}
