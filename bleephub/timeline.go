package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (s *Server) registerTimelineRoutes() {
	// Timeline CRUD
	s.mux.HandleFunc("POST /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/timeline", s.handleCreateTimeline)
	s.mux.HandleFunc("POST /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/timeline/{timelineId}", s.handleCreateTimeline)
	s.mux.HandleFunc("PUT /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/timeline/{timelineId}", s.handleCreateTimeline)

	// Timeline records
	s.mux.HandleFunc("PATCH /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/{timelineId}", s.handleUpdateRecords)

	// Log files
	s.mux.HandleFunc("POST /_apis/v1/Logfiles/{scopeId}/{hubName}/{planId}", s.handleCreateLog)
	s.mux.HandleFunc("POST /_apis/v1/Logfiles/{scopeId}/{hubName}/{planId}/{logId}", s.handleUploadLog)

	// Web console log (live output)
	s.mux.HandleFunc("POST /_apis/v1/TimeLineWebConsoleLog/{scopeId}/{hubName}/{planId}/{timelineId}/{recordId}", s.handleWebConsoleLog)

	// Timeline attachments
	s.mux.HandleFunc("PUT /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/{timelineId}/attachments/{recordId}/{attachType}/{name}", s.handleTimelineAttachment)
}

func (s *Server) handleCreateTimeline(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("timelineId")
	s.logger.Debug().Str("timelineId", timelineID).Msg("create/update timeline")

	// Read and discard body
	body, _ := io.ReadAll(r.Body)
	if len(body) > 0 {
		var data interface{}
		json.Unmarshal(body, &data)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       timelineID,
		"changeId": 1,
	})
}

func (s *Server) handleUpdateRecords(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("timelineId")

	var records []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&records); err != nil {
		// Try as wrapper object
		var wrapper map[string]interface{}
		r.Body.Close()
		writeJSON(w, http.StatusOK, map[string]interface{}{"count": 0, "value": []interface{}{}})
		_ = wrapper
		return
	}

	for _, rec := range records {
		name, _ := rec["name"].(string)
		state, _ := rec["state"].(string)
		result, _ := rec["result"].(string)
		s.logger.Info().
			Str("timelineId", timelineID).
			Str("name", name).
			Str("state", state).
			Str("result", result).
			Msg("timeline record update")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(records),
		"value": records,
	})
}

func (s *Server) handleCreateLog(w http.ResponseWriter, r *http.Request) {
	logID := s.nextLogID()
	s.logger.Debug().Int("logId", logID).Msg("create log container")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        logID,
		"path":      fmt.Sprintf("logs/%d", logID),
		"createdOn": "2026-01-01T00:00:00Z",
		"lineCount": 0,
	})
}

func (s *Server) handleUploadLog(w http.ResponseWriter, r *http.Request) {
	logID := r.PathValue("logId")

	body, err := io.ReadAll(r.Body)
	if err == nil && len(body) > 0 {
		s.logger.Info().Str("logId", logID).Str("content", string(body)).Msg("log upload")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        logID,
		"path":      fmt.Sprintf("logs/%s", logID),
		"createdOn": "2026-01-01T00:00:00Z",
		"lineCount": len(body),
	})
}

func (s *Server) handleWebConsoleLog(w http.ResponseWriter, r *http.Request) {
	recordID := r.PathValue("recordId")

	var lines []string
	if err := json.NewDecoder(r.Body).Decode(&lines); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	for _, line := range lines {
		s.logger.Info().Str("recordId", recordID).Str("line", line).Msg("console")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"count": len(lines)})
}

func (s *Server) handleTimelineAttachment(w http.ResponseWriter, r *http.Request) {
	attachType := r.PathValue("attachType")
	name := r.PathValue("name")
	s.logger.Debug().Str("type", attachType).Str("name", name).Msg("timeline attachment")

	io.ReadAll(r.Body) // consume body
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}
