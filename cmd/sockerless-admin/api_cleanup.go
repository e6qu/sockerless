package main

import (
	"net/http"
	"time"
)

// registerCleanupAPI registers cleanup API routes.
func registerCleanupAPI(mux *http.ServeMux, reg *Registry, client *http.Client) {
	mux.HandleFunc("GET /api/v1/cleanup/scan", handleCleanupScan(reg, client))
	mux.HandleFunc("POST /api/v1/cleanup/processes", handleCleanProcesses())
	mux.HandleFunc("POST /api/v1/cleanup/tmp", handleCleanTmp())
	mux.HandleFunc("POST /api/v1/cleanup/containers", handleCleanContainers(reg, client))
}

// handleCleanupScan runs all scan functions and returns combined results.
func handleCleanupScan(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var items []CleanupItem

		items = append(items, ScanOrphanedProcesses()...)
		items = append(items, ScanStaleTmpFiles()...)
		items = append(items, ScanStoppedContainers(reg, client)...)
		items = append(items, ScanStaleResources(reg, client)...)

		if items == nil {
			items = []CleanupItem{}
		}

		writeJSON(w, http.StatusOK, CleanupScanResult{
			Items:     items,
			ScannedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// handleCleanProcesses cleans orphaned PID files.
func handleCleanProcesses() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cleaned := CleanOrphanedProcesses()
		writeJSON(w, http.StatusOK, map[string]int{"cleaned": cleaned})
	}
}

// handleCleanTmp cleans stale temp directories.
func handleCleanTmp() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cleaned := CleanTmpFiles()
		writeJSON(w, http.StatusOK, map[string]int{"cleaned": cleaned})
	}
}

// handleCleanContainers prunes stopped containers across backends.
func handleCleanContainers(reg *Registry, client *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cleaned := CleanStoppedContainers(reg, client)
		writeJSON(w, http.StatusOK, map[string]int{"cleaned": cleaned})
	}
}
