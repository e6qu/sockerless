package bleephub

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
)

const messagePollTimeout = 30 * time.Second

func (s *Server) registerBrokerRoutes() {
	// Sessions
	s.mux.HandleFunc("POST /_apis/v1/AgentSession/{poolId}", s.handleCreateSession)
	s.mux.HandleFunc("DELETE /_apis/v1/AgentSession/{poolId}/{sessionId}", s.handleDeleteSession)

	// Message polling
	s.mux.HandleFunc("GET /_apis/v1/Message/{poolId}", s.handleGetMessage)
	s.mux.HandleFunc("DELETE /_apis/v1/Message/{poolId}/{messageId}", s.handleDeleteMessage)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	// Parse as raw JSON to avoid type mismatches (e.g., createdOn format)
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		s.logger.Error().Err(err).Msg("failed to parse session request")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	ownerName, _ := raw["ownerName"].(string)

	// Extract agent info for session tracking
	var agent *Agent
	if agentRaw, ok := raw["agent"].(map[string]interface{}); ok {
		agent = &Agent{
			Enabled: true,
			Status:  "online",
		}
		if id, ok := agentRaw["id"].(float64); ok {
			agent.ID = int(id)
		}
		if name, ok := agentRaw["name"].(string); ok {
			agent.Name = name
		}
		if version, ok := agentRaw["version"].(string); ok {
			agent.Version = version
		}
	}

	sessionID := uuid.New().String()
	session := &Session{
		SessionID: sessionID,
		OwnerName: ownerName,
		Agent:     agent,
		MsgCh:     make(chan *TaskAgentMessage, 10),
	}

	s.store.mu.Lock()
	s.store.Sessions[sessionID] = session
	s.store.mu.Unlock()

	// Update session count metric
	if s.metrics != nil {
		s.metrics.SetActiveSessions(int64(s.sessionCount()))
	}

	// Drain any pending messages to the new session
	s.drainPendingMessages()

	s.logger.Info().Str("sessionId", sessionID).Msg("session created")

	// Return session WITHOUT encryption key — the runner will use plaintext messages
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessionId":     sessionID,
		"ownerName":     ownerName,
		"agent":         agent,
		"encryptionKey": nil,
	})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")

	s.store.mu.Lock()
	session, ok := s.store.Sessions[sessionID]
	if ok {
		close(session.MsgCh)
		delete(s.store.Sessions, sessionID)
	}
	s.store.mu.Unlock()

	if s.metrics != nil {
		s.metrics.SetActiveSessions(int64(s.sessionCount()))
	}

	s.logger.Info().Str("sessionId", sessionID).Bool("found", ok).Msg("session deleted")
	w.WriteHeader(http.StatusOK)
}

// handleGetMessage long-polls for a job message for the runner.
// Returns 200 with a message if one is available, or 200 with empty body after timeout.
func (s *Server) handleGetMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")

	s.store.mu.RLock()
	session, ok := s.store.Sessions[sessionID]
	s.store.mu.RUnlock()

	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), messagePollTimeout)
	defer cancel()

	select {
	case msg, open := <-session.MsgCh:
		if !open || msg == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		s.logger.Info().Int64("messageId", msg.MessageID).Msg("delivering message to runner")
		writeJSON(w, http.StatusOK, msg)
	case <-ctx.Done():
		// Timeout — no message available, return empty 200
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	msgID := r.PathValue("messageId")
	s.logger.Debug().Str("messageId", msgID).Msg("message acknowledged")
	w.WriteHeader(http.StatusOK)
}

// sendMessageToAgent sends a TaskAgentMessage to the next available session
// using round-robin distribution for fair load balancing.
func (s *Server) sendMessageToAgent(msg *TaskAgentMessage) bool {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	if len(s.store.Sessions) == 0 {
		return false
	}

	// Sort session IDs for deterministic ordering
	ids := make([]string, 0, len(s.store.Sessions))
	for id := range s.store.Sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Round-robin from lastSessionIdx
	n := len(ids)
	for i := 0; i < n; i++ {
		idx := (s.lastSessionIdx + i) % n
		session := s.store.Sessions[ids[idx]]
		select {
		case session.MsgCh <- msg:
			s.lastSessionIdx = (idx + 1) % n
			s.logger.Info().
				Int64("messageId", msg.MessageID).
				Str("sessionId", session.SessionID).
				Msg("message queued for runner")
			return true
		default:
			// Channel full, try next session
		}
	}
	return false
}

// requeuePendingMessage stores a message for later delivery when no session is available.
func (s *Server) requeuePendingMessage(msg *TaskAgentMessage) {
	s.store.mu.Lock()
	s.store.PendingMessages = append(s.store.PendingMessages, msg)
	s.store.mu.Unlock()
}

// drainPendingMessages sends any queued messages to available sessions.
func (s *Server) drainPendingMessages() {
	s.store.mu.Lock()
	pending := s.store.PendingMessages
	s.store.PendingMessages = nil
	s.store.mu.Unlock()

	var remaining []*TaskAgentMessage
	for _, msg := range pending {
		if !s.sendMessageToAgent(msg) {
			remaining = append(remaining, msg)
		}
	}

	if len(remaining) > 0 {
		s.store.mu.Lock()
		s.store.PendingMessages = append(remaining, s.store.PendingMessages...)
		s.store.mu.Unlock()
	}
}

// nextMessageID returns the next message ID.
func (s *Server) nextMessageID() int64 {
	s.store.mu.Lock()
	id := s.store.NextMsg
	s.store.NextMsg++
	s.store.mu.Unlock()
	return id
}

// nextRequestID returns the next request ID.
func (s *Server) nextRequestID() int64 {
	s.store.mu.Lock()
	id := s.store.NextReqID
	s.store.NextReqID++
	s.store.mu.Unlock()
	return id
}

// nextLogID returns the next log container ID.
func (s *Server) nextLogID() int {
	s.store.mu.Lock()
	id := s.store.NextLog
	s.store.NextLog++
	s.store.mu.Unlock()
	return id
}

// lookupJobByRequestID finds a job by its request ID.
func (s *Server) lookupJobByRequestID(reqID int64) *Job {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, j := range s.store.Jobs {
		if j.RequestID == reqID {
			return j
		}
	}
	return nil
}

// sessionCount returns the current number of active sessions.
func (s *Server) sessionCount() int {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	return len(s.store.Sessions)
}

// lookupJobByPlanID finds a job by its plan ID.
func (s *Server) lookupJobByPlanID(planID string) *Job {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, j := range s.store.Jobs {
		if j.PlanID == planID {
			return j
		}
	}
	return nil
}
