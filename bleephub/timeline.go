package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

const (
	// logFileCap bounds a single runner-uploaded log file. Content past
	// the cap is dropped and the truncation is marked in the stored log
	// so readers see it happened.
	logFileCap = 4 << 20

	// consoleLineCap bounds the live console capture per job. When the
	// cap trims lines, consoleTruncationMarker is appended once.
	consoleLineCap = 10000
)

var (
	logTruncationMarker     = []byte("\n[bleephub] log truncated at 4 MiB\n")
	consoleTruncationMarker = fmt.Sprintf("[bleephub] console log truncated at %d lines", consoleLineCap)
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
	planID := r.PathValue("planId")
	timelineID := r.PathValue("timelineId")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read timeline records body: "+err.Error(), http.StatusBadRequest)
		return
	}
	records, err := decodeTimelineRecords(body)
	if err != nil {
		http.Error(w, "invalid timeline records body: "+err.Error(), http.StatusBadRequest)
		return
	}

	merged := s.upsertTimelineRecords(planID, records)
	for _, rec := range merged {
		s.logger.Debug().
			Str("planId", planID).
			Str("timelineId", timelineID).
			Str("name", rec.Name).
			Str("state", rec.State).
			Str("result", rec.Result).
			Msg("timeline record update")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(merged),
		"value": merged,
	})
}

// decodeTimelineRecords decodes a timeline-record PATCH body. The official
// actions/runner wraps the records in a VssJsonCollectionWrapper
// ({"count": N, "value": [...]}); a bare array is accepted too so direct
// callers don't have to build the wrapper.
func decodeTimelineRecords(body []byte) ([]*TimelineRecord, error) {
	var wrapper struct {
		Value []*TimelineRecord `json:"value"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Value != nil {
		return wrapper.Value, nil
	}
	var bare []*TimelineRecord
	if err := json.Unmarshal(body, &bare); err != nil {
		return nil, err
	}
	return bare, nil
}

// upsertTimelineRecords folds the PATCHed records into the plan's stored
// set, keyed by record ID. Returns copies of the post-merge records for
// the response body.
func (s *Server) upsertTimelineRecords(planID string, records []*TimelineRecord) []*TimelineRecord {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	out := make([]*TimelineRecord, 0, len(records))
	for _, rec := range records {
		if rec == nil || rec.ID == "" {
			continue
		}
		var stored *TimelineRecord
		for _, existing := range s.store.TimelineRecords[planID] {
			if existing.ID == rec.ID {
				stored = existing
				break
			}
		}
		if stored == nil {
			stored = &TimelineRecord{ID: rec.ID}
			s.store.TimelineRecords[planID] = append(s.store.TimelineRecords[planID], stored)
		}
		mergeTimelineRecord(stored, rec)
		cp := *stored
		out = append(out, &cp)
	}
	return out
}

// mergeTimelineRecord folds a newer runner update into the stored record.
// The runner PATCHes the same record repeatedly as state advances and a
// later update may omit fields it isn't changing, so a present field never
// regresses to empty.
func mergeTimelineRecord(stored, incoming *TimelineRecord) {
	if incoming.ParentID != "" {
		stored.ParentID = incoming.ParentID
	}
	if incoming.Type != "" {
		stored.Type = incoming.Type
	}
	if incoming.Name != "" {
		stored.Name = incoming.Name
	}
	if incoming.RefName != "" {
		stored.RefName = incoming.RefName
	}
	if incoming.Order != 0 {
		stored.Order = incoming.Order
	}
	if incoming.State != "" {
		stored.State = incoming.State
	}
	if incoming.Result != "" {
		stored.Result = incoming.Result
	}
	if incoming.StartTime != "" {
		stored.StartTime = incoming.StartTime
	}
	if incoming.FinishTime != "" {
		stored.FinishTime = incoming.FinishTime
	}
	if incoming.Log != nil {
		stored.Log = incoming.Log
	}
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
	logID, err := strconv.Atoi(r.PathValue("logId"))
	if err != nil {
		http.Error(w, "invalid log ID", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read log content: "+err.Error(), http.StatusBadRequest)
		return
	}

	// The runner may upload a log in multiple blocks — append. Bound the
	// stored size at logFileCap, keeping the head and marking the cut.
	s.store.mu.Lock()
	existing := s.store.LogFiles[logID]
	next := append([]byte(nil), existing...)
	switch {
	case bytes.HasSuffix(existing, logTruncationMarker):
		// Already capped; later blocks are dropped past the marker.
	case len(existing)+len(body) <= logFileCap:
		next = append(next, body...)
	default:
		if keep := logFileCap - len(existing); keep > 0 {
			next = append(next, body[:keep]...)
		}
		next = append(next, logTruncationMarker...)
	}
	storedData := append([]byte(nil), next...)
	stored := len(next)

	if err := s.artifactStore.writeLogData(r.Context(), logID, storedData); err != nil {
		s.store.mu.Unlock()
		http.Error(w, "log byte-store write: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.store.LogFiles[logID] = next
	s.store.mu.Unlock()

	s.logger.Debug().Int("logId", logID).Int("uploadBytes", len(body)).Int("storedBytes", stored).Msg("log upload")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        logID,
		"path":      fmt.Sprintf("logs/%d", logID),
		"createdOn": "2026-01-01T00:00:00Z",
		"lineCount": bytes.Count(body, []byte{'\n'}),
	})
}

func (s *Server) handleWebConsoleLog(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")
	recordID := r.PathValue("recordId")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read console log body: "+err.Error(), http.StatusBadRequest)
		return
	}
	lines, err := decodeConsoleLines(body)
	if err != nil {
		http.Error(w, "invalid console log body: "+err.Error(), http.StatusBadRequest)
		return
	}

	for _, line := range lines {
		s.logger.Info().Str("recordId", recordID).Str("line", line).Msg("console")
	}

	// Capture log lines keyed by jobID for the management dashboard.
	// Capped at consoleLineCap; trimming appends the marker line once.
	if planID != "" && len(lines) > 0 {
		job := s.lookupJobByPlanID(planID)
		if job != nil {
			s.store.mu.Lock()
			existing := s.store.LogLines[job.ID]
			switch {
			case len(existing) > 0 && existing[len(existing)-1] == consoleTruncationMarker:
				// Already capped; later lines are dropped past the marker.
			case len(existing)+len(lines) <= consoleLineCap:
				s.store.LogLines[job.ID] = append(existing, lines...)
			default:
				if keep := consoleLineCap - len(existing); keep > 0 {
					existing = append(existing, lines[:keep]...)
				}
				s.store.LogLines[job.ID] = append(existing, consoleTruncationMarker)
			}
			s.store.mu.Unlock()
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"count": len(lines)})
}

// decodeConsoleLines decodes a web-console-log POST body. The official
// actions/runner sends a TimelineRecordFeedLinesWrapper
// ({"count": N, "value": [...], "stepId": ...}); a bare line array is
// accepted too so direct callers don't have to build the wrapper.
func decodeConsoleLines(body []byte) ([]string, error) {
	var wrapper struct {
		Value []string `json:"value"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Value != nil {
		return wrapper.Value, nil
	}
	var bare []string
	if err := json.Unmarshal(body, &bare); err != nil {
		return nil, err
	}
	return bare, nil
}

func (s *Server) handleTimelineAttachment(w http.ResponseWriter, r *http.Request) {
	attachType := r.PathValue("attachType")
	name := r.PathValue("name")
	s.logger.Debug().Str("type", attachType).Str("name", name).Msg("timeline attachment")

	if _, err := io.ReadAll(r.Body); err != nil {
		s.logger.Error().Err(err).Str("type", attachType).Str("name", name).Msg("timeline attachment: read body")
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"status": "error", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}
