package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (s *Server) registerTimelineRoutes() {
	// Timeline CRUD
	s.route("POST /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/timeline", s.handleCreateTimeline)
	s.route("POST /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/timeline/{timelineId}", s.handleCreateTimeline)
	s.route("PUT /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/timeline/{timelineId}", s.handleCreateTimeline)

	// Timeline records
	s.route("PATCH /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/{timelineId}", s.handleUpdateRecords)

	// Log files
	s.route("POST /_apis/v1/Logfiles/{scopeId}/{hubName}/{planId}", s.handleCreateLog)
	s.route("POST /_apis/v1/Logfiles/{scopeId}/{hubName}/{planId}/{logId}", s.handleUploadLog)

	// Web console log (live output)
	s.route("POST /_apis/v1/TimeLineWebConsoleLog/{scopeId}/{hubName}/{planId}/{timelineId}/{recordId}", s.handleWebConsoleLog)

	// Timeline attachments
	s.route("PUT /_apis/v1/Timeline/{scopeId}/{hubName}/{planId}/{timelineId}/attachments/{recordId}/{attachType}/{name}", s.handleTimelineAttachment)
}

func (s *Server) handleCreateTimeline(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("timelineId")
	s.logger.Debug().Str("timelineId", timelineID).Msg("create/update timeline")

	// The handler ignores the body — the timeline is opaque to bleephub
	// and the body's shape is whatever Azure DevOps' AzurePipelines task
	// happens to send today. Discard explicitly so it's visible in code
	// that there's no decode step. Drain to free the underlying conn.
	_, _ = io.Copy(io.Discard, r.Body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       timelineID,
		"changeId": 1,
	})
}

func (s *Server) handleUpdateRecords(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("timelineId")

	var records []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&records); err != nil {
		r.Body.Close()
		writeJSON(w, http.StatusOK, map[string]interface{}{"count": 0, "value": []interface{}{}})
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
	planID := r.PathValue("planId")
	recordID := r.PathValue("recordId")

	var lines []string
	if !decodeJSONBody(w, r, &lines) {
		return
	}

	for _, line := range lines {
		s.logger.Info().Str("recordId", recordID).Str("line", line).Msg("console")
	}

	// Capture log lines keyed by jobID for the management dashboard
	if planID != "" && len(lines) > 0 {
		job := s.lookupJobByPlanID(planID)
		if job != nil {
			s.store.mu.Lock()
			existing := s.store.LogLines[job.ID]
			remaining := 500 - len(existing)
			if remaining > 0 {
				if len(lines) > remaining {
					lines = lines[:remaining]
				}
				s.store.LogLines[job.ID] = append(existing, lines...)
			}
			s.store.mu.Unlock()
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"count": len(lines)})
}

func (s *Server) handleTimelineAttachment(w http.ResponseWriter, r *http.Request) {
	attachType := r.PathValue("attachType")
	name := r.PathValue("name")
	s.logger.Debug().Str("type", attachType).Str("name", name).Msg("timeline attachment")

	io.ReadAll(r.Body)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}
