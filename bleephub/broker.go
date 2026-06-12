package bleephub

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const messagePollTimeout = 30 * time.Second

func (s *Server) registerBrokerRoutes() {
	// Sessions
	s.route("POST /_apis/v1/AgentSession/{poolId}", s.handleCreateSession)
	s.route("DELETE /_apis/v1/AgentSession/{poolId}/{sessionId}", s.handleDeleteSession)

	// Message polling
	s.route("GET /_apis/v1/Message/{poolId}", s.handleGetMessage)
	s.route("DELETE /_apis/v1/Message/{poolId}/{messageId}", s.handleDeleteMessage)
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
		// The session request carries a slim agent reference; the
		// REGISTERED agent is the routing source of truth because it
		// holds the labels from config-time registration.
		if id, ok := agentRaw["id"].(float64); ok {
			s.store.mu.RLock()
			agent = s.store.Agents[int(id)]
			s.store.mu.RUnlock()
		}
		if agent == nil {
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

// sendMessageToAgent sends a message to the next eligible session
// (round-robin among sessions whose agent labels satisfy the job's
// runs-on requirements).
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
		if !agentSatisfiesLabels(session.Agent, msg.Labels) {
			continue
		}
		select {
		case session.MsgCh <- msg:
			s.lastSessionIdx = (idx + 1) % n
			// Record which agent took the job: the runners API reports
			// `busy` from running jobs' agent association.
			if msg.JobID != "" && session.Agent != nil {
				if job := s.store.Jobs[msg.JobID]; job != nil {
					job.AgentID = session.Agent.ID
				}
			}
			s.logger.Info().
				Int64("messageId", msg.MessageID).
				Str("sessionId", session.SessionID).
				Strs("labels", msg.Labels).
				Msg("message queued for runner")
			return true
		default:
		}
	}
	return false
}

// agentSatisfiesLabels reports whether an agent's registered labels
// cover every runs-on requirement (case-insensitive). GitHub-hosted
// pool aliases (ubuntu-*, macos-*, windows-*) are satisfiable by ANY
// agent: bleephub has no hosted pool, so a hosted-alias job runs on
// whatever runner connects — the same accommodation act/nektos makes.
// All other labels (self-hosted, custom) match strictly.
func agentSatisfiesLabels(agent *Agent, required []string) bool {
	if len(required) == 0 {
		return true
	}
	var have map[string]bool
	if agent != nil {
		have = make(map[string]bool, len(agent.Labels))
		for _, l := range agent.Labels {
			have[strings.ToLower(l.Name)] = true
		}
	}
	for _, req := range required {
		lower := strings.ToLower(req)
		if isHostedPoolAlias(lower) {
			continue
		}
		if !have[lower] {
			return false
		}
	}
	return true
}

func isHostedPoolAlias(lower string) bool {
	return strings.HasPrefix(lower, "ubuntu-") ||
		strings.HasPrefix(lower, "macos-") ||
		strings.HasPrefix(lower, "windows-")
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
