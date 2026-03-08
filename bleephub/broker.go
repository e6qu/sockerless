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
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		s.logger.Error().Err(err).Msg("failed to parse session request")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	ownerName, _ := raw["ownerName"].(string)

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

	if s.metrics != nil {
		s.metrics.SetActiveSessions(int64(s.sessionCount()))
	}

	s.drainPendingMessages()

	s.logger.Info().Str("sessionId", sessionID).Msg("session created")

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

// handleGetMessage long-polls for a job message (30s timeout).
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
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	msgID := r.PathValue("messageId")
	s.logger.Debug().Str("messageId", msgID).Msg("message acknowledged")
	w.WriteHeader(http.StatusOK)
}

// sendMessageToAgent sends a message to the next available session (round-robin).
func (s *Server) sendMessageToAgent(msg *TaskAgentMessage) bool {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	if len(s.store.Sessions) == 0 {
		return false
	}

	ids := make([]string, 0, len(s.store.Sessions))
	for id := range s.store.Sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)

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
		}
	}
	return false
}

func (s *Server) requeuePendingMessage(msg *TaskAgentMessage) {
	s.store.mu.Lock()
	s.store.PendingMessages = append(s.store.PendingMessages, msg)
	s.store.mu.Unlock()
}

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

func (s *Server) nextMessageID() int64 {
	s.store.mu.Lock()
	id := s.store.NextMsg
	s.store.NextMsg++
	s.store.mu.Unlock()
	return id
}

func (s *Server) nextRequestID() int64 {
	s.store.mu.Lock()
	id := s.store.NextReqID
	s.store.NextReqID++
	s.store.mu.Unlock()
	return id
}

func (s *Server) nextLogID() int {
	s.store.mu.Lock()
	id := s.store.NextLog
	s.store.NextLog++
	s.store.mu.Unlock()
	return id
}

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

func (s *Server) sessionCount() int {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	return len(s.store.Sessions)
}

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
