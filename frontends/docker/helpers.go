package frontend

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/sockerless/api"
)

// flushingCopy copies from src to w, flushing after each read chunk.
// This ensures streaming responses (pull progress, build output, events)
// are delivered incrementally instead of buffering until stream close.
func flushingCopy(w http.ResponseWriter, src io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if sc, ok := err.(api.StatusCoder); ok {
		status = sc.StatusCode()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(api.ErrorResponse{Message: err.Error()})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

// proxyPassthrough copies the backend response directly to the client.
func proxyPassthrough(w http.ResponseWriter, resp *http.Response) {
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// proxyErrorResponse reads an error from the backend and writes it to the client.
func proxyErrorResponse(w http.ResponseWriter, resp *http.Response) {
	var errResp api.ErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	json.NewEncoder(w).Encode(errResp)
}
