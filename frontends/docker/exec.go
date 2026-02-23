package frontend

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/sockerless/api"
)

func (s *Server) handleExecCreate(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	var req api.ExecCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.backend.post(r.Context(), "/containers/"+containerID+"/exec", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		proxyErrorResponse(w, resp)
		return
	}

	var result api.ExecCreateResponse
	json.NewDecoder(resp.Body).Decode(&result)
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleExecInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.get(r.Context(), "/exec/"+id)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleExecStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req api.ExecStartRequest
	_ = readJSON(r, &req)

	// Dial the backend with connection upgrade for bidirectional streaming.
	// The backend handles agent communication internally if needed.
	backendConn, backendBuf, err := s.backend.dialUpgrade("POST", "/exec/"+id+"/start", &req)
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
	// First drain any buffered data from the backend reader
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

func (s *Server) handleExecResize(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
