package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestAgentRefreshMessageDelivery verifies that the sim-control endpoint
// POST /internal/agents/{id}/refresh-message delivers a real
// AgentRefreshMessage to every open session for the target agent.
func TestAgentRefreshMessageDelivery(t *testing.T) {
	agentID := 4242
	testServer.store.mu.Lock()
	testServer.store.Agents[agentID] = &Agent{
		ID:      agentID,
		Name:    "refresh-runner",
		Version: "2.319.0",
		Status:  "online",
		Labels:  []Label{{Name: "self-hosted"}},
	}
	testServer.store.mu.Unlock()
	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Agents, agentID)
		delete(testServer.store.Sessions, "refresh-sess")
		testServer.store.mu.Unlock()
	}()

	sess := &Session{
		SessionID: "refresh-sess",
		Agent:     &Agent{ID: agentID, Labels: []Label{{Name: "self-hosted"}}},
		MsgCh:     make(chan *TaskAgentMessage, 10),
	}
	testServer.store.mu.Lock()
	testServer.store.Sessions["refresh-sess"] = sess
	testServer.store.mu.Unlock()

	targetVersion := "2.320.0"
	resp := ghPost(t, fmt.Sprintf("/internal/agents/%d/refresh-message", agentID), defaultToken, map[string]interface{}{
		"targetVersion": targetVersion,
		"timeout":       "10m",
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	resp.Body.Close()

	var msg *TaskAgentMessage
	select {
	case msg = <-sess.MsgCh:
	case <-time.After(2 * time.Second):
		t.Fatal("no AgentRefreshMessage delivered to runner session")
	}

	if msg.MessageType != "AgentRefreshMessage" {
		t.Fatalf("message type = %q, want AgentRefreshMessage", msg.MessageType)
	}
	var body struct {
		AgentID       int    `json:"agentId"`
		TargetVersion string `json:"targetVersion"`
		Timeout       string `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(msg.Body), &body); err != nil {
		t.Fatalf("body unmarshal: %v (body=%s)", err, msg.Body)
	}
	if body.AgentID != agentID {
		t.Errorf("agentId = %d, want %d", body.AgentID, agentID)
	}
	if body.TargetVersion != targetVersion {
		t.Errorf("targetVersion = %q, want %q", body.TargetVersion, targetVersion)
	}
	if body.Timeout != "10m0s" {
		t.Errorf("timeout = %q, want 10m0s", body.Timeout)
	}
}

// TestAgentRefreshMessageAdminOnly verifies the refresh endpoint rejects
// non-admin callers.
func TestAgentRefreshMessageAdminOnly(t *testing.T) {
	agentID := 4243
	testServer.store.mu.Lock()
	testServer.store.Agents[agentID] = &Agent{ID: agentID, Name: "r"}
	testServer.store.mu.Unlock()
	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Agents, agentID)
		testServer.store.mu.Unlock()
	}()

	// Create a non-admin user + token.
	nonAdmin := &User{ID: 9001, Login: "nobody", Type: "User"}
	testServer.store.mu.Lock()
	testServer.store.Users[nonAdmin.ID] = nonAdmin
	tok := &Token{Value: "ghp_nonadmin", UserID: nonAdmin.ID, Scopes: "repo"}
	testServer.store.Tokens[tok.Value] = tok
	testServer.store.mu.Unlock()
	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Users, nonAdmin.ID)
		delete(testServer.store.Tokens, tok.Value)
		testServer.store.mu.Unlock()
	}()

	resp := ghPost(t, fmt.Sprintf("/internal/agents/%d/refresh-message", agentID), tok.Value, map[string]interface{}{
		"targetVersion": "2.320.0",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}

// TestAgentRefreshMessageAgentNotFound verifies 404 for unknown agents.
func TestAgentRefreshMessageAgentNotFound(t *testing.T) {
	resp := ghPost(t, "/internal/agents/99999/refresh-message", defaultToken, map[string]interface{}{
		"targetVersion": "2.320.0",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
