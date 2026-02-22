package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

var testBaseURL string

func TestMain(m *testing.M) {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().Timestamp().Logger().Level(zerolog.DebugLevel)

	// Find free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find free port: %v\n", err)
		os.Exit(1)
	}
	addr := ln.Addr().String()
	ln.Close()

	testBaseURL = "http://" + addr

	srv := NewServer(addr, logger)
	go srv.ListenAndServe()

	// Wait for server to be ready
	for i := 0; i < 50; i++ {
		resp, err := http.Get(testBaseURL + "/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	os.Exit(m.Run())
}

func TestHealth(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestConnectionData(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/_apis/connectionData")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	instanceID, _ := data["instanceId"].(string)
	if instanceID == "" {
		t.Fatal("missing instanceId")
	}

	locData, _ := data["locationServiceData"].(map[string]interface{})
	defs, _ := locData["serviceDefinitions"].([]interface{})
	if len(defs) == 0 {
		t.Fatal("no service definitions")
	}
}

func TestOAuthToken(t *testing.T) {
	resp, err := http.Post(testBaseURL+"/_apis/v1/auth/", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	if data["access_token"] == nil {
		t.Fatal("missing access_token")
	}
}

func TestRunnerRegistration(t *testing.T) {
	body := `{"url":"http://localhost","runner_event":"register"}`
	resp, err := http.Post(testBaseURL+"/api/v3/actions/runner-registration", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	if data["token"] == nil {
		t.Fatal("missing token")
	}
	if data["token_schema"] != "OAuthAccessToken" {
		t.Fatalf("unexpected token_schema: %v", data["token_schema"])
	}
}

func TestListPools(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/_apis/v1/AgentPools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	count, _ := data["count"].(float64)
	if count != 1 {
		t.Fatalf("expected 1 pool, got %v", data["count"])
	}
}

func TestAgentLifecycle(t *testing.T) {
	// Register agent
	agentBody := `{"name":"test-runner","version":"3.0.0","labels":[{"name":"self-hosted","type":"system"}]}`
	resp, err := http.Post(testBaseURL+"/_apis/v1/Agent/1", "application/json", bytes.NewBufferString(agentBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var agent map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&agent)

	agentID := int(agent["id"].(float64))
	if agentID == 0 {
		t.Fatal("agent ID should be non-zero")
	}
	if agent["name"] != "test-runner" {
		t.Fatalf("unexpected name: %v", agent["name"])
	}

	// List agents
	resp2, err := http.Get(testBaseURL + "/_apis/v1/Agent/1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var list map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&list)

	agents := list["value"].([]interface{})
	if len(agents) == 0 {
		t.Fatal("expected at least 1 agent")
	}

	// Get agent
	resp3, err := http.Get(fmt.Sprintf("%s/_apis/v1/Agent/1/%d", testBaseURL, agentID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Fatalf("get agent: expected 200, got %d", resp3.StatusCode)
	}

	// Delete agent
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/_apis/v1/Agent/1/%d", testBaseURL, agentID), nil)
	resp4, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != 200 {
		t.Fatalf("delete agent: expected 200, got %d", resp4.StatusCode)
	}

	// Verify deleted
	resp5, err := http.Get(fmt.Sprintf("%s/_apis/v1/Agent/1/%d", testBaseURL, agentID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp5.StatusCode)
	}
}

func TestSessionAndMessage(t *testing.T) {
	// Create session
	sessionBody := `{"ownerName":"RUNNER","agent":{"id":99,"name":"test"}}`
	resp, err := http.Post(testBaseURL+"/_apis/v1/AgentSession/1", "application/json", bytes.NewBufferString(sessionBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var session map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&session)

	sessionID, _ := session["sessionId"].(string)
	if sessionID == "" {
		t.Fatal("missing sessionId")
	}

	// Submit a job
	jobBody := `{"image":"alpine:latest","steps":[{"run":"echo hello"}]}`
	resp2, err := http.Post(testBaseURL+"/api/v3/bleephub/submit", "application/json", bytes.NewBufferString(jobBody))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	// Poll for message (should get it immediately since job was just submitted)
	resp3, err := http.Get(testBaseURL + "/_apis/v1/Message/1?sessionId=" + sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()

	body, _ := io.ReadAll(resp3.Body)
	if len(body) == 0 {
		t.Fatal("expected a message, got empty response")
	}

	var msg TaskAgentMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	if msg.MessageType != "PipelineAgentJobRequest" {
		t.Fatalf("unexpected message type: %s", msg.MessageType)
	}

	// Delete session
	req, _ := http.NewRequest("DELETE", testBaseURL+"/_apis/v1/AgentSession/1/"+sessionID, nil)
	resp4, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp4.Body.Close()
}
