package gitlabhub

import (
	"io"
	"net/http"
)

func (s *Server) registerCacheRoutes() {
	s.mux.HandleFunc("PUT /cache/{key...}", s.handleCachePut)
	s.mux.HandleFunc("GET /cache/{key...}", s.handleCacheGet)
	s.mux.HandleFunc("HEAD /cache/{key...}", s.handleCacheHead)
}

// handleCachePut handles PUT /cache/:key.
func (s *Server) handleCachePut(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "cache key required", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	s.store.SetCache(key, data)

	s.logger.Debug().
		Str("key", key).
		Int("size", len(data)).
		Msg("cache stored")

	w.WriteHeader(http.StatusOK)
}

// handleCacheGet handles GET /cache/:key.
func (s *Server) handleCacheGet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "cache key required", http.StatusBadRequest)
		return
	}

	data := s.store.GetCache(key)
	if data == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handleCacheHead handles HEAD /cache/:key.
func (s *Server) handleCacheHead(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		http.Error(w, "cache key required", http.StatusBadRequest)
		return
	}

	data := s.store.GetCache(key)
	if data == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}
