package frontend

import (
	"io"
	"net/http"
	"net/url"
)

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	for _, key := range []string{"stdout", "stderr", "follow", "timestamps", "tail", "since", "until"} {
		if v := r.URL.Query().Get(key); v != "" {
			query.Set(key, v)
		}
	}
	resp, err := s.backend.getWithQuery("/containers/"+id+"/logs", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *Server) handleContainerAttach(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Forward attach request to backend via connection upgrade.
	// The backend handles agent communication internally if needed.
	query := url.Values{}
	for _, key := range []string{"stream", "stdin", "stdout", "stderr", "detachKeys", "logs"} {
		if v := r.URL.Query().Get(key); v != "" {
			query.Set(key, v)
		}
	}

	backendConn, backendBuf, err := s.backend.dialUpgrade("POST", "/containers/"+id+"/attach", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer backendConn.Close()

	// Hijack the client connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	// Write upgrade response to Docker client
	clientBuf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	clientBuf.WriteString("Content-Type: application/vnd.docker.multiplexed-stream\r\n")
	clientBuf.WriteString("Connection: Upgrade\r\n")
	clientBuf.WriteString("Upgrade: tcp\r\n")
	clientBuf.WriteString("\r\n")
	clientBuf.Flush()

	// Bridge bidirectionally: client ↔ backend
	done := make(chan struct{})

	// Client → Backend (stdin)
	go func() {
		defer func() { close(done) }()
		io.Copy(backendConn, clientConn)
	}()

	// Backend → Client (stdout/stderr)
	if backendBuf.Buffered() > 0 {
		buffered := make([]byte, backendBuf.Buffered())
		n, _ := backendBuf.Read(buffered)
		if n > 0 {
			_, _ = clientConn.Write(buffered[:n])
		}
	}
	io.Copy(clientConn, backendConn)

	// Backend is done sending. Close client write side so client gets EOF.
	if cw, ok := clientConn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	} else {
		clientConn.Close()
	}

	// Close read side to promptly unblock the stdin goroutine.
	// The backend has closed, so no more stdin forwarding is useful.
	if cr, ok := clientConn.(interface{ CloseRead() error }); ok {
		_ = cr.CloseRead()
	}

	<-done
}

func (s *Server) handleContainerResize(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleContainerTop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if ps := r.URL.Query().Get("ps_args"); ps != "" {
		query.Set("ps_args", ps)
	}
	resp, err := s.backend.getWithQuery("/containers/"+id+"/top", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if stream := r.URL.Query().Get("stream"); stream != "" {
		query.Set("stream", stream)
	}
	resp, err := s.backend.getWithQuery("/containers/"+id+"/stats", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Use flushing copy for streaming stats
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

func (s *Server) handleContainerPutArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if p := r.URL.Query().Get("path"); p != "" {
		query.Set("path", p)
	}
	resp, err := s.backend.putWithQuery("/containers/"+id+"/archive", query, r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerHeadArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if p := r.URL.Query().Get("path"); p != "" {
		query.Set("path", p)
	}
	resp, err := s.backend.headWithQuery("/containers/"+id+"/archive", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
}

func (s *Server) handleContainerGetArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if p := r.URL.Query().Get("path"); p != "" {
		query.Set("path", p)
	}
	resp, err := s.backend.getWithQuery("/containers/"+id+"/archive", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
