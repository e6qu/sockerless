package main

import (
	"context"
	"net/http"
	"strings"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	sim "github.com/sockerless/simulator"
	monitoredres "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Cloud Logging types (internal representation)

// LogEntry represents a Cloud Logging log entry.
type LogEntry struct {
	LogName     string             `json:"logName"`
	Resource    *MonitoredResource `json:"resource,omitempty"`
	Timestamp   string             `json:"timestamp,omitempty"`
	Severity    string             `json:"severity,omitempty"`
	TextPayload string            `json:"textPayload,omitempty"`
	JsonPayload map[string]any     `json:"jsonPayload,omitempty"`
	InsertID    string             `json:"insertId,omitempty"`
	Labels      map[string]string  `json:"labels,omitempty"`
}

// MonitoredResource represents the monitored resource that produced a log entry.
type MonitoredResource struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Package-level state store shared between HTTP and gRPC handlers.
var logEntries = sim.NewStateStore[[]LogEntry]()

// listLogEntries is the shared implementation for listing log entries,
// used by both the REST handler and the gRPC server.
func listLogEntries(filter string, resourceNames []string, pageSize int) []LogEntry {
	var allEntries []LogEntry
	all := logEntries.List()
	for _, entries := range all {
		allEntries = append(allEntries, entries...)
	}

	// Apply structured filter
	if filter != "" {
		var filtered []LogEntry
		for _, entry := range allEntries {
			if matchesFilter(entry, filter) {
				filtered = append(filtered, entry)
			}
		}
		allEntries = filtered
	}

	// Filter by resource names
	if len(resourceNames) > 0 {
		var filtered []LogEntry
		for _, entry := range allEntries {
			for _, rn := range resourceNames {
				if strings.HasPrefix(entry.LogName, rn) || strings.Contains(entry.LogName, rn) {
					filtered = append(filtered, entry)
					break
				}
			}
		}
		allEntries = filtered
	}

	// Apply page size
	if pageSize > 0 && len(allEntries) > pageSize {
		allEntries = allEntries[:pageSize]
	}

	if allEntries == nil {
		allEntries = []LogEntry{}
	}
	return allEntries
}

// writeLogEntries is the shared implementation for writing log entries,
// used by both the REST handler and the gRPC server.
func writeLogEntries(logName string, resource *MonitoredResource, labels map[string]string, entries []LogEntry) {
	for i := range entries {
		entry := &entries[i]
		if entry.LogName == "" {
			entry.LogName = logName
		}
		if entry.Resource == nil {
			entry.Resource = resource
		}
		if entry.Timestamp == "" {
			entry.Timestamp = nowTimestamp()
		}
		if entry.InsertID == "" {
			entry.InsertID = generateUUID()
		}
		if len(labels) > 0 && entry.Labels == nil {
			entry.Labels = make(map[string]string)
		}
		for k, v := range labels {
			if _, exists := entry.Labels[k]; !exists {
				entry.Labels[k] = v
			}
		}
	}

	for _, entry := range entries {
		ln := entry.LogName
		existing, ok := logEntries.Get(ln)
		if !ok {
			existing = []LogEntry{}
		}
		existing = append(existing, entry)
		logEntries.Put(ln, existing)
	}
}

// REST request/response types

// ListLogEntriesRESTRequest is the request body for listing log entries via REST.
type ListLogEntriesRESTRequest struct {
	ResourceNames []string `json:"resourceNames"`
	Filter        string   `json:"filter"`
	OrderBy       string   `json:"orderBy"`
	PageSize      int      `json:"pageSize"`
	PageToken     string   `json:"pageToken"`
}

// ListLogEntriesRESTResponse is the response body for listing log entries via REST.
type ListLogEntriesRESTResponse struct {
	Entries       []LogEntry `json:"entries"`
	NextPageToken string     `json:"nextPageToken,omitempty"`
}

// WriteLogEntriesRESTRequest is the request body for writing log entries via REST.
type WriteLogEntriesRESTRequest struct {
	LogName  string             `json:"logName"`
	Resource *MonitoredResource `json:"resource,omitempty"`
	Labels   map[string]string  `json:"labels,omitempty"`
	Entries  []LogEntry         `json:"entries"`
}

func registerCloudLogging(srv *sim.Server) {
	// List log entries (REST)
	srv.HandleFunc("POST /v2/entries:list", func(w http.ResponseWriter, r *http.Request) {
		var req ListLogEntriesRESTRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		entries := listLogEntries(req.Filter, req.ResourceNames, req.PageSize)
		sim.WriteJSON(w, http.StatusOK, ListLogEntriesRESTResponse{
			Entries: entries,
		})
	})

	// Write log entries (REST)
	srv.HandleFunc("POST /v2/entries:write", func(w http.ResponseWriter, r *http.Request) {
		var req WriteLogEntriesRESTRequest
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		writeLogEntries(req.LogName, req.Resource, req.Labels, req.Entries)
		sim.WriteJSON(w, http.StatusOK, map[string]any{})
	})
}

// gRPC Cloud Logging server

type loggingServer struct {
	loggingpb.UnimplementedLoggingServiceV2Server
}

func (s *loggingServer) WriteLogEntries(_ context.Context, req *loggingpb.WriteLogEntriesRequest) (*loggingpb.WriteLogEntriesResponse, error) {
	var resource *MonitoredResource
	if req.Resource != nil {
		resource = &MonitoredResource{
			Type:   req.Resource.Type,
			Labels: req.Resource.Labels,
		}
	}

	var entries []LogEntry
	for _, pe := range req.Entries {
		entry := protoToLogEntry(pe)
		entries = append(entries, entry)
	}

	writeLogEntries(req.LogName, resource, req.Labels, entries)
	return &loggingpb.WriteLogEntriesResponse{}, nil
}

func (s *loggingServer) ListLogEntries(_ context.Context, req *loggingpb.ListLogEntriesRequest) (*loggingpb.ListLogEntriesResponse, error) {
	entries := listLogEntries(req.Filter, req.ResourceNames, int(req.PageSize))

	var pbEntries []*loggingpb.LogEntry
	for _, e := range entries {
		pbEntries = append(pbEntries, logEntryToProto(e))
	}

	return &loggingpb.ListLogEntriesResponse{
		Entries: pbEntries,
	}, nil
}

// registerCloudLoggingGRPC registers the gRPC Cloud Logging service on a grpc.Server.
func registerCloudLoggingGRPC(gs *grpc.Server) {
	loggingpb.RegisterLoggingServiceV2Server(gs, &loggingServer{})
}

// Conversion helpers

func protoToLogEntry(pe *loggingpb.LogEntry) LogEntry {
	entry := LogEntry{
		LogName:  pe.LogName,
		InsertID: pe.InsertId,
		Labels:   pe.Labels,
	}

	if pe.Resource != nil {
		entry.Resource = &MonitoredResource{
			Type:   pe.Resource.Type,
			Labels: pe.Resource.Labels,
		}
	}

	if pe.Timestamp != nil {
		entry.Timestamp = pe.Timestamp.AsTime().Format("2006-01-02T15:04:05.999999999Z07:00")
	}

	if pe.Severity != 0 {
		entry.Severity = pe.Severity.String()
	}

	switch p := pe.Payload.(type) {
	case *loggingpb.LogEntry_TextPayload:
		entry.TextPayload = p.TextPayload
	case *loggingpb.LogEntry_JsonPayload:
		if p.JsonPayload != nil {
			entry.JsonPayload = p.JsonPayload.AsMap()
		}
	}

	return entry
}

func logEntryToProto(e LogEntry) *loggingpb.LogEntry {
	pe := &loggingpb.LogEntry{
		LogName:  e.LogName,
		InsertId: e.InsertID,
		Labels:   e.Labels,
	}

	if e.Resource != nil {
		pe.Resource = &monitoredres.MonitoredResource{
			Type:   e.Resource.Type,
			Labels: e.Resource.Labels,
		}
	}

	if e.Timestamp != "" {
		t, err := parseTimestamp(e.Timestamp)
		if err == nil {
			pe.Timestamp = timestamppb.New(t)
		}
	}

	if e.TextPayload != "" {
		pe.Payload = &loggingpb.LogEntry_TextPayload{TextPayload: e.TextPayload}
	} else if len(e.JsonPayload) > 0 {
		s, err := structpb.NewStruct(e.JsonPayload)
		if err == nil {
			pe.Payload = &loggingpb.LogEntry_JsonPayload{JsonPayload: s}
		}
	}

	return pe
}
