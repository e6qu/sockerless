package main

import (
	"net/http"
	"strconv"
	"strings"
)

// registerProcessAPI registers process management API routes.
func registerProcessAPI(mux *http.ServeMux, procMgr *ProcessManager) {
	mux.HandleFunc("GET /api/v1/processes", handleProcessList(procMgr))
	mux.HandleFunc("POST /api/v1/processes/{name}/start", handleProcessStart(procMgr))
	mux.HandleFunc("POST /api/v1/processes/{name}/stop", handleProcessStop(procMgr))
	mux.HandleFunc("GET /api/v1/processes/{name}/logs", handleProcessLogs(procMgr))
}

// handleProcessList returns all managed processes.
func handleProcessList(procMgr *ProcessManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, procMgr.List())
	}
}

// handleProcessStart starts a managed process.
func handleProcessStart(procMgr *ProcessManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := procMgr.Start(name); err != nil {
			writeJSON(w, processErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		info, _ := procMgr.Get(name)
		writeJSON(w, http.StatusOK, info)
	}
}

// handleProcessStop stops a managed process.
func handleProcessStop(procMgr *ProcessManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := procMgr.Stop(name); err != nil {
			writeJSON(w, processErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		info, _ := procMgr.Get(name)
		writeJSON(w, http.StatusOK, info)
	}
}

// handleProcessLogs returns the last N log lines for a process.
func handleProcessLogs(procMgr *ProcessManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		lines := 100
		if q := r.URL.Query().Get("lines"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				lines = n
			}
		}

		logLines, err := procMgr.GetLogs(name, lines)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if logLines == nil {
			logLines = []string{}
		}
		writeJSON(w, http.StatusOK, logLines)
	}
}

// processErrorStatus maps process manager errors to HTTP status codes.
func processErrorStatus(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(msg, "already") || strings.Contains(msg, "is not running") {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
