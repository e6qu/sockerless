package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerAgentRoutes() {
	// Registration token (for config.sh)
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/actions/runners/registration-token", s.handleRegistrationToken)

	// Agent pools
	s.mux.HandleFunc("GET /_apis/v1/AgentPools", s.handleListPools)

	// Agent CRUD — order matters: more specific patterns first
	s.mux.HandleFunc("POST /_apis/v1/Agent/{poolId}", s.handleRegisterAgent)
	s.mux.HandleFunc("GET /_apis/v1/Agent/{poolId}/{agentId}", s.handleGetAgent)
	s.mux.HandleFunc("PUT /_apis/v1/Agent/{poolId}/{agentId}", s.handleUpdateAgent)
	s.mux.HandleFunc("DELETE /_apis/v1/Agent/{poolId}/{agentId}", s.handleDeleteAgent)
	s.mux.HandleFunc("GET /_apis/v1/Agent/{poolId}", s.handleListAgents)
}

func (s *Server) handleRegistrationToken(w http.ResponseWriter, r *http.Request) {
	s.logger.Info().Msg("registration token requested")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      "BLEEPHUB_REG_TOKEN",
		"expires_at": "2099-01-01T00:00:00Z",
	})
}

func (s *Server) handleListPools(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("list agent pools")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": 1,
		"value": []map[string]interface{}{
			{"id": 1, "name": "Default", "size": 0, "isHosted": false, "poolType": "automation"},
		},
	})
}

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	// Parse as generic JSON since the runner sends fields not in our Agent struct
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		s.logger.Error().Err(err).Msg("failed to parse agent registration")
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Build agent from raw data
	var agent Agent
	if name, ok := raw["name"].(string); ok {
		agent.Name = name
	}
	if ver, ok := raw["version"].(string); ok {
		agent.Version = ver
	}
	if desc, ok := raw["osDescription"].(string); ok {
		agent.OSDescription = desc
	}
	// Parse labels
	if labelsRaw, ok := raw["labels"].([]interface{}); ok {
		for _, l := range labelsRaw {
			if lm, ok := l.(map[string]interface{}); ok {
				label := Label{}
				if n, ok := lm["name"].(string); ok {
					label.Name = n
				}
				if t, ok := lm["type"].(string); ok {
					label.Type = t
				}
				agent.Labels = append(agent.Labels, label)
			}
		}
	}
	// Preserve authorization (RSA public key) from the runner
	if authRaw, ok := raw["authorization"].(map[string]interface{}); ok {
		agent.Authorization = &AgentAuthorization{}
		if pk, ok := authRaw["publicKey"].(map[string]interface{}); ok {
			agent.Authorization.PublicKey = &AgentPublicKey{}
			if exp, ok := pk["exponent"].(string); ok {
				agent.Authorization.PublicKey.Exponent = exp
			}
			if mod, ok := pk["modulus"].(string); ok {
				agent.Authorization.PublicKey.Modulus = mod
			}
		}
	}

	s.store.mu.Lock()
	agent.ID = s.store.NextAgent
	s.store.NextAgent++
	agent.Enabled = true
	agent.Status = "online"
	agent.CreatedOn = time.Now()

	// Set authorization URL and client ID for OAuth token exchange
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if agent.Authorization == nil {
		agent.Authorization = &AgentAuthorization{}
	}
	agent.Authorization.AuthorizationURL = scheme + "://" + r.Host + "/_apis/v1/auth/"
	agent.Authorization.ClientID = uuid.New().String()

	s.store.Agents[agent.ID] = &agent
	s.store.mu.Unlock()

	s.logger.Info().Int("id", agent.ID).Str("name", agent.Name).Msg("agent registered")
	writeJSON(w, http.StatusOK, &agent)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	// Filter by agentName query param if present
	nameFilter := r.URL.Query().Get("agentName")

	s.store.mu.RLock()
	agents := make([]*Agent, 0)
	for _, a := range s.store.Agents {
		if nameFilter != "" && !strings.EqualFold(a.Name, nameFilter) {
			continue
		}
		agents = append(agents, a)
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(agents),
		"value": agents,
	})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := strconv.Atoi(r.PathValue("agentId"))
	if err != nil {
		http.Error(w, "invalid agent ID", http.StatusBadRequest)
		return
	}

	s.store.mu.RLock()
	agent, ok := s.store.Agents[agentID]
	s.store.mu.RUnlock()

	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := strconv.Atoi(r.PathValue("agentId"))
	if err != nil {
		http.Error(w, "invalid agent ID", http.StatusBadRequest)
		return
	}

	var update Agent
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.store.mu.Lock()
	agent, ok := s.store.Agents[agentID]
	if !ok {
		s.store.mu.Unlock()
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	// Preserve ID and authorization, update other fields
	update.ID = agent.ID
	if update.Authorization == nil {
		update.Authorization = agent.Authorization
	}
	update.CreatedOn = agent.CreatedOn
	s.store.Agents[agentID] = &update
	s.store.mu.Unlock()

	writeJSON(w, http.StatusOK, &update)
}

func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID, err := strconv.Atoi(r.PathValue("agentId"))
	if err != nil {
		http.Error(w, "invalid agent ID", http.StatusBadRequest)
		return
	}

	s.store.mu.Lock()
	_, ok := s.store.Agents[agentID]
	if ok {
		delete(s.store.Agents, agentID)
	}
	s.store.mu.Unlock()

	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	s.logger.Info().Int("id", agentID).Msg("agent unregistered")
	w.WriteHeader(http.StatusOK)
}
