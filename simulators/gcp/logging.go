package main

import (
	"net/http"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud Logging types

// LogEntry represents a Cloud Logging log entry.
type LogEntry struct {
	LogName      string             `json:"logName"`
	Resource     *MonitoredResource `json:"resource,omitempty"`
	Timestamp    string             `json:"timestamp,omitempty"`
	Severity     string             `json:"severity,omitempty"`
	TextPayload  string             `json:"textPayload,omitempty"`
	JsonPayload  map[string]any     `json:"jsonPayload,omitempty"`
	InsertID     string             `json:"insertId,omitempty"`
	Labels       map[string]string  `json:"labels,omitempty"`
}

// MonitoredResource represents the monitored resource that produced a log entry.
type MonitoredResource struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
}

// ListLogEntriesRequest is the request body for listing log entries.
type ListLogEntriesRequest struct {
	ResourceNames []string `json:"resourceNames"`
	Filter        string   `json:"filter"`
	OrderBy       string   `json:"orderBy"`
	PageSize      int      `json:"pageSize"`
	PageToken     string   `json:"pageToken"`
}

// ListLogEntriesResponse is the response body for listing log entries.
type ListLogEntriesResponse struct {
	Entries       []LogEntry `json:"entries"`
	NextPageToken string     `json:"nextPageToken,omitempty"`
}

// WriteLogEntriesRequest is the request body for writing log entries.
type WriteLogEntriesRequest struct {
	LogName  string             `json:"logName"`
	Resource *MonitoredResource `json:"resource,omitempty"`
	Labels   map[string]string  `json:"labels,omitempty"`
	Entries  []LogEntry         `json:"entries"`
}

func registerCloudLogging(srv *sim.Server) {
	logEntries := sim.NewStateStore[[]LogEntry]()

	// List log entries
	srv.HandleFunc("POST /v2/entries:list", func(w http.ResponseWriter, r *http.Request) {
		var req ListLogEntriesRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		var allEntries []LogEntry
		all := logEntries.List()
		for _, entries := range all {
			allEntries = append(allEntries, entries...)
		}

		// Apply basic filter matching if a filter is provided
		if req.Filter != "" {
			var filtered []LogEntry
			for _, entry := range allEntries {
				if matchesFilter(entry, req.Filter) {
					filtered = append(filtered, entry)
				}
			}
			allEntries = filtered
		}

		// Filter by resource names
		if len(req.ResourceNames) > 0 {
			var filtered []LogEntry
			for _, entry := range allEntries {
				for _, rn := range req.ResourceNames {
					if strings.HasPrefix(entry.LogName, rn) || strings.Contains(entry.LogName, rn) {
						filtered = append(filtered, entry)
						break
					}
				}
			}
			allEntries = filtered
		}

		// Apply page size
		if req.PageSize > 0 && len(allEntries) > req.PageSize {
			allEntries = allEntries[:req.PageSize]
		}

		if allEntries == nil {
			allEntries = []LogEntry{}
		}

		sim.WriteJSON(w, http.StatusOK, ListLogEntriesResponse{
			Entries: allEntries,
		})
	})

	// Write log entries
	srv.HandleFunc("POST /v2/entries:write", func(w http.ResponseWriter, r *http.Request) {
		var req WriteLogEntriesRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		for i := range req.Entries {
			entry := &req.Entries[i]
			// Fill in defaults from the request-level fields
			if entry.LogName == "" {
				entry.LogName = req.LogName
			}
			if entry.Resource == nil {
				entry.Resource = req.Resource
			}
			if entry.Timestamp == "" {
				entry.Timestamp = nowTimestamp()
			}
			if entry.InsertID == "" {
				entry.InsertID = generateUUID()
			}
			// Merge request-level labels
			if len(req.Labels) > 0 && entry.Labels == nil {
				entry.Labels = make(map[string]string)
			}
			for k, v := range req.Labels {
				if _, exists := entry.Labels[k]; !exists {
					entry.Labels[k] = v
				}
			}
		}

		// Store entries keyed by logName
		for _, entry := range req.Entries {
			logName := entry.LogName
			existing, ok := logEntries.Get(logName)
			if !ok {
				existing = []LogEntry{}
			}
			existing = append(existing, entry)
			logEntries.Put(logName, existing)
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})
}

// matchesFilter does a simple substring match on the entry's fields.
// Real GCP uses a rich filter language; this is a simplification for the simulator.
func matchesFilter(entry LogEntry, filter string) bool {
	// Simple: check if filter substring appears in logName, severity, or textPayload
	filter = strings.ToLower(filter)
	if strings.Contains(strings.ToLower(entry.LogName), filter) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Severity), filter) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.TextPayload), filter) {
		return true
	}
	return false
}
