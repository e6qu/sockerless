package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleInstanceLogs serves logs for a topology instance from the
// admin-managed `.stack-pids/<name>.log` file.
//
// Query params:
//
//	lines=N    cap on lines returned / streamed-as-seed (default 200)
//	follow=1   open a Server-Sent-Events stream and emit new lines as
//	           they appear, after seeding with the last N lines.
//
// Without `follow=1`, returns the last N lines as JSON
// `{"lines": [...]}`. With `follow=1`, the response is `text/event-stream`
// with one SSE event per log line: `data: <line>\n\n`. Keep-alive
// comments (`: keep-alive\n\n`) are emitted every 250ms when the file
// has not grown so reverse proxies don't time out the connection.
//
// Components stay decoupled: this reads admin's own bookkeeping log
// (written by `make start-component` redirecting stdout/stderr to
// .stack-pids/<NAME>.log) — components do not grow a new endpoint.
func handleInstanceLogs(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instance := r.PathValue("instance")
		ref, ok := mgr.FindInstance(project, instance)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "instance " + project + "/" + instance + " not found",
			})
			return
		}

		lines := parseLinesParam(r.URL.Query().Get("lines"), 200)
		follow := r.URL.Query().Get("follow") == "1"
		path := instanceLogPath(ref.Instance.Name)

		if follow {
			streamInstanceLogs(w, r, path, lines)
			return
		}
		writeTailJSON(w, path, lines)
	}
}

const (
	logTailReadCap   = 4 * 1024 * 1024 // 4 MB cap when reading "last N lines"
	logFollowMaxLine = 64 * 1024       // 64 KB hard cap per emitted line
	logTickInterval  = 250 * time.Millisecond
)

func parseLinesParam(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	if n > 10000 {
		return 10000
	}
	return n
}

// instanceLogPath resolves <cwd>/.stack-pids/<name>.log — the file
// `make start-component` writes to. cwd matches the path used by
// readPidStatus and envFilePath, so the three pieces of bookkeeping
// stay in lockstep.
func instanceLogPath(name string) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, ".stack-pids", name+".log")
}

// readLastLines returns the last n lines of the file at path. Missing
// file is treated as "no logs yet" — returns an empty slice rather
// than an error, since a topology instance can exist without ever
// having been started.
func readLastLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	off := int64(0)
	if size > logTailReadCap {
		off = size - logTailReadCap
	}
	data := make([]byte, size-off)
	if _, err := f.ReadAt(data, off); err != nil && err != io.EOF {
		return nil, err
	}
	if off > 0 {
		// Read window started mid-line. Drop the partial leading line.
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return tailLines(data, n), nil
}

func tailLines(data []byte, n int) []string {
	if len(data) == 0 {
		return []string{}
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	lines := strings.Split(string(data), "\n")
	if n <= 0 || n >= len(lines) {
		return lines
	}
	return lines[len(lines)-n:]
}

func writeTailJSON(w http.ResponseWriter, path string, lines int) {
	tail, err := readLastLines(path, lines)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": tail})
}

// streamInstanceLogs opens path, seeds with the last `seed` lines, and
// emits an SSE event per new line as the file grows. Polls file size
// every 250 ms; truncation re-opens at offset 0. Returns when the
// request context is cancelled (client disconnect).
func streamInstanceLogs(w http.ResponseWriter, r *http.Request, path string, seed int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Open the file (or wait for it) and emit the seed in the same step
	// so we don't duplicate or miss lines if the file appears between a
	// pre-open seed read and the open call.
	var (
		f      *os.File
		offset int64
	)
	for {
		if r.Context().Err() != nil {
			return
		}
		var err error
		f, err = os.Open(path)
		if err == nil {
			st, statErr := f.Stat()
			if statErr != nil {
				_ = f.Close()
				writeSSEError(w, statErr.Error())
				flusher.Flush()
				return
			}
			seedLines, _ := readLastLines(path, seed)
			for _, line := range seedLines {
				writeSSELine(w, line)
			}
			offset = st.Size()
			flusher.Flush()
			break
		}
		if !os.IsNotExist(err) {
			writeSSEError(w, err.Error())
			flusher.Flush()
			return
		}
		_, _ = w.Write([]byte(": waiting-for-log\n\n"))
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(logTickInterval):
		}
	}
	defer f.Close()

	tick := time.NewTicker(logTickInterval)
	defer tick.Stop()
	pending := ""
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			st, err := f.Stat()
			if err != nil {
				return
			}
			if st.Size() < offset {
				_ = f.Close()
				f, err = os.Open(path)
				if err != nil {
					return
				}
				offset = 0
				pending = ""
				continue
			}
			if st.Size() == offset {
				_, _ = w.Write([]byte(": keep-alive\n\n"))
				flusher.Flush()
				continue
			}
			buf := make([]byte, st.Size()-offset)
			n, err := f.ReadAt(buf, offset)
			if err != nil && err != io.EOF {
				return
			}
			combined := pending + string(buf[:n])
			parts := strings.Split(combined, "\n")
			pending = parts[len(parts)-1]
			for _, line := range parts[:len(parts)-1] {
				if len(line) > logFollowMaxLine {
					line = line[:logFollowMaxLine] + "...(truncated)"
				}
				writeSSELine(w, line)
			}
			offset += int64(n)
			flusher.Flush()
		}
	}
}

func writeSSELine(w io.Writer, line string) {
	fmt.Fprintf(w, "data: %s\n\n", escapeSSELine(line))
}

func writeSSEError(w io.Writer, msg string) {
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", escapeSSELine(msg))
}

// escapeSSELine flattens any embedded line breaks. The SSE spec splits
// the data field on \n into separate `data:` lines, which would
// silently merge into one log line on the client; keep one event per
// log line by collapsing newlines to spaces.
func escapeSSELine(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			out = append(out, ' ')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}
