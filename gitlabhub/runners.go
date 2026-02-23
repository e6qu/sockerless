package gitlabhub

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerRunnerRoutes() {
	s.mux.HandleFunc("POST /api/v4/runners", s.handleRegisterRunner)
	s.mux.HandleFunc("POST /api/v4/runners/verify", s.handleVerifyRunner)
	s.mux.HandleFunc("DELETE /api/v4/runners", s.handleUnregisterRunner)
}

// handleRegisterRunner handles POST /api/v4/runners.
// GitLab Runner sends a registration token and gets back a runner-specific token.
func (s *Server) handleRegisterRunner(w http.ResponseWriter, r *http.Request) {
	var req RunnerRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Accept any registration token (we're a lightweight coordinator)
	if req.Token == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}

	runner := s.store.RegisterRunner(req.Description, req.TagList)

	s.logger.Info().
		Int("runner_id", runner.ID).
		Str("description", runner.Description).
		Msg("runner registered")

	if s.metrics != nil {
		s.metrics.RecordRunnerRegister()
	}

	writeJSON(w, http.StatusCreated, RunnerRegistrationResponse{
		ID:    runner.ID,
		Token: runner.Token,
	})
}

// handleVerifyRunner handles POST /api/v4/runners/verify.
func (s *Server) handleVerifyRunner(w http.ResponseWriter, r *http.Request) {
	var req RunnerVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	runner := s.store.LookupRunnerByToken(req.Token)
	if runner == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleUnregisterRunner handles DELETE /api/v4/runners.
func (s *Server) handleUnregisterRunner(w http.ResponseWriter, r *http.Request) {
	var req RunnerVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if !s.store.UnregisterRunner(req.Token) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	s.logger.Info().Str("token_prefix", req.Token[:10]+"...").Msg("runner unregistered")
	w.WriteHeader(http.StatusNoContent)
}
