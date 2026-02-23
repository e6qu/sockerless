package gitlabhub

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerJobUpdateRoutes() {
	s.mux.HandleFunc("PUT /api/v4/jobs/{id}", s.handleUpdateJob)
	s.mux.HandleFunc("PATCH /api/v4/jobs/{id}/trace", s.handlePatchTrace)
}

// handleUpdateJob handles PUT /api/v4/jobs/:id.
// The runner calls this to update job status (running/success/failed).
func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid job ID", http.StatusBadRequest)
		return
	}

	// Authenticate via JOB-TOKEN header
	jobToken := r.Header.Get("JOB-TOKEN")
	if jobToken == "" {
		// Also check Authorization header
		jobToken = extractToken(r)
	}

	job := s.store.GetJob(jobID)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	if jobToken != "" && job.Token != jobToken {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Parse the state from the request
	state := r.FormValue("state")
	if state == "" {
		// Try JSON body
		var req JobUpdateRequest
		if err := decodeJSON(r, &req); err == nil && req.State != "" {
			state = req.State
		}
	}

	if state == "" {
		http.Error(w, "state is required", http.StatusBadRequest)
		return
	}

	s.logger.Info().
		Int("job_id", jobID).
		Str("state", state).
		Msg("job update")

	s.store.mu.Lock()
	switch state {
	case "running":
		job.Status = "running"
	case "success":
		job.Status = "success"
		job.Result = "success"
	case "failed":
		job.Status = "failed"
		job.Result = "failed"
	case "canceled":
		job.Status = "canceled"
		job.Result = "canceled"
	}
	s.store.mu.Unlock()

	// Trigger engine callback for terminal states
	if state == "success" || state == "failed" || state == "canceled" {
		if s.metrics != nil {
			s.metrics.RecordJobCompletion(state)
		}
		s.onJobCompleted(r.Context(), jobID, state)
	}

	w.WriteHeader(http.StatusOK)
}

// handlePatchTrace handles PATCH /api/v4/jobs/:id/trace.
// The runner sends incremental log output via this endpoint.
func (s *Server) handlePatchTrace(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid job ID", http.StatusBadRequest)
		return
	}

	job := s.store.GetJob(jobID)
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	// Read trace body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	// Append to job's trace data
	s.store.mu.Lock()
	job.TraceData = append(job.TraceData, data...)
	traceLen := len(job.TraceData)
	s.store.mu.Unlock()

	// Return required headers
	w.Header().Set("Job-Status", job.Status)
	w.Header().Set("Range", fmt.Sprintf("0-%d", traceLen-1))
	w.WriteHeader(http.StatusAccepted)
}

// extractToken gets a token from Authorization header.
func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return auth
}

// decodeJSON decodes a JSON request body (non-destructive for FormValue fallback).
func decodeJSON(r *http.Request, v interface{}) error {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "json") {
		return parseJSON(r.Body, v)
	}
	return fmt.Errorf("not JSON content type")
}
