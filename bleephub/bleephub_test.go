package bleephub

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

var (
	testBaseURL string
	testServer  *Server
)

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
	testServer = srv
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
	// Register a runner with an RSA public key, then exchange a signed
	// client_assertion JWT for an access token — the real Azure DevOps
	// agent OAuth2 jwt-bearer flow the actions/runner uses.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	mod := base64.StdEncoding.EncodeToString(key.N.Bytes())
	exp := base64.StdEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	regBody := fmt.Sprintf(`{"name":"oauth-test","version":"2.0","authorization":{"publicKey":{"modulus":%q,"exponent":%q}}}`, mod, exp)
	regResp, err := http.Post(testBaseURL+"/_apis/v1/Agent/1", "application/json", bytes.NewBufferString(regBody))
	if err != nil {
		t.Fatalf("register agent: %v", err)
	}
	defer regResp.Body.Close()
	if regResp.StatusCode != 200 {
		t.Fatalf("agent register: expected 200, got %d", regResp.StatusCode)
	}
	var agent struct {
		ID            int `json:"id"`
		Authorization struct {
			ClientID string `json:"clientId"`
		} `json:"authorization"`
	}
	if err := json.NewDecoder(regResp.Body).Decode(&agent); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	if agent.Authorization.ClientID == "" {
		t.Fatal("missing clientId on registered agent")
	}

	assertion := signTestAssertion(t, key, agent.Authorization.ClientID)
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	form.Set("client_assertion", assertion)

	resp, err := http.Post(testBaseURL+"/_apis/v1/auth/", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if data["access_token"] == nil {
		t.Fatal("missing access_token")
	}
}

func TestOAuthTokenRejectsMissingAssertion(t *testing.T) {
	resp, err := http.Post(testBaseURL+"/_apis/v1/auth/", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

func TestOAuthTokenRejectsUnknownClient(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	assertion := signTestAssertion(t, key, "00000000-0000-0000-0000-000000000000")
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	form.Set("client_assertion", assertion)
	resp, err := http.Post(testBaseURL+"/_apis/v1/auth/", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401 for unregistered clientId, got %d", resp.StatusCode)
	}
}

func signTestAssertion(t *testing.T, key *rsa.PrivateKey, clientID string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	now := time.Now().Unix()
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"iss":%q,"iat":%d,"exp":%d}`, clientID, now, now+300,
	)))
	signInput := header + "." + payload
	hash := sha256.Sum256([]byte(signInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signInput + "." + base64.RawURLEncoding.EncodeToString(sig)
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
