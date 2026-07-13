package bleephub

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerAuthRoutes() {
	s.registerExternalIdentityRoutes()
	// Runner registration (GHES-style)
	s.route("POST /api/v3/actions/runner-registration", s.handleRunnerRegistration)

	// Connection data (service discovery)
	s.route("GET /_apis/connectionData", s.handleConnectionData)

	// OAuth token exchange
	s.route("POST /_apis/v1/auth/", s.handleOAuthToken)
	s.route("POST /_apis/v1/auth", s.handleOAuthToken)
}

// handleRunnerRegistration returns the tenant URL and a management token.
// The runner calls this during `config.sh --url <url> --token <token>`.
func (s *Server) handleRunnerRegistration(w http.ResponseWriter, r *http.Request) {
	s.logger.Info().Msg("runner registration request")

	var req struct {
		URL         string `json:"url"`
		RunnerEvent string `json:"runner_event"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	serverURL := s.baseURL(r)

	// The runner extracts org/repo from the tenant URL for display purposes.
	// If the original --url had a path (e.g. /owner/repo), preserve it.
	if req.URL != "" {
		if parsed, err := url.Parse(req.URL); err == nil && parsed.Path != "" {
			serverURL += parsed.Path
		}
	}

	s.logger.Info().Str("tenantUrl", serverURL).Msg("returning tenant URL")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":          serverURL,
		"token_schema": "OAuthAccessToken",
		"token":        "bleephub-mgmt-" + uuid.New().String(),
	})
}

// serviceDefinition matches the internal ServiceDefinition format
// that the runner SDK expects in ConnectionData.
type serviceDefinition struct {
	ServiceType       string        `json:"serviceType"`
	Identifier        string        `json:"identifier"`
	DisplayName       string        `json:"displayName"`
	RelativeToSetting string        `json:"relativeToSetting"`
	RelativePath      string        `json:"relativePath"`
	Description       string        `json:"description"`
	ServiceOwner      string        `json:"serviceOwner"`
	LocationMappings  []interface{} `json:"locationMappings"`
	ToolID            string        `json:"toolId"`
	Status            string        `json:"status"`
	Properties        interface{}   `json:"properties"`
	ResourceVersion   int           `json:"resourceVersion"`
	MinVersion        string        `json:"minVersion"`
	MaxVersion        string        `json:"maxVersion"`
}

func newServiceDef(name, guid, path string) serviceDefinition {
	return serviceDefinition{
		ServiceType:       name,
		Identifier:        guid,
		DisplayName:       name,
		RelativeToSetting: "fullyQualified",
		RelativePath:      path,
		Description:       name,
		ServiceOwner:      "00000000-0000-0000-0000-000000000000",
		LocationMappings:  []interface{}{},
		ToolID:            name,
		Status:            "active",
		Properties:        map[string]interface{}{},
		ResourceVersion:   1,
		MinVersion:        "1.0",
		MaxVersion:        "12.0",
	}
}

// handleConnectionData returns service location data (GUIDs → API paths).
func (s *Server) handleConnectionData(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("connection data request")

	defs := []serviceDefinition{
		newServiceDef("AgentPools", "a8c47e17-4d56-4a56-92bb-de7ea7dc65be", "/_apis/v1/AgentPools"),
		newServiceDef("Agent", "e298ef32-5878-4cab-993c-043836571f42", "/_apis/v1/Agent/{poolId}/{agentId}"),
		newServiceDef("AgentSession", "134e239e-2df3-4794-a6f6-24f1f19ec8dc", "/_apis/v1/AgentSession/{poolId}/{sessionId}"),
		newServiceDef("Message", "c3a054f6-7a8a-49c0-944e-3a8e5d7adfd7", "/_apis/v1/Message/{poolId}/{messageId}"),
		newServiceDef("AgentRequest", "fc825784-c92a-4299-9221-998a02d1b54f", "/_apis/v1/AgentRequest/{poolId}/{requestId}"),
		newServiceDef("FinishJob", "557624af-b29e-4c20-8ab0-0399d2204f3f", "/_apis/v1/FinishJob/{scopeIdentifier}/{hubName}/{planId}"),
		newServiceDef("Timeline", "83597576-cc2c-453c-bea6-2882ae6a1653", "/_apis/v1/Timeline/{scopeIdentifier}/{hubName}/{planId}/timeline/{timelineId}"),
		newServiceDef("TimelineRecords", "8893bc5b-35b2-4be7-83cb-99e683551db4", "/_apis/v1/Timeline/{scopeIdentifier}/{hubName}/{planId}/{timelineId}"),
		newServiceDef("Logfiles", "46f5667d-263a-4684-91b1-dff7fdcf64e2", "/_apis/v1/Logfiles/{scopeIdentifier}/{hubName}/{planId}/{logId}"),
		newServiceDef("TimeLineWebConsoleLog", "858983e4-19bd-4c5e-864c-507b59b58b12", "/_apis/v1/TimeLineWebConsoleLog/{scopeIdentifier}/{hubName}/{planId}/{timelineId}/{recordId}"),
		newServiceDef("ActionDownloadInfo", "27d7f831-88c1-4719-8ca1-6a061dad90eb", "/_apis/v1/ActionDownloadInfo/{scopeIdentifier}/{hubName}/{planId}"),
		newServiceDef("TimelineAttachments", "7898f959-9cdf-4096-b29e-7f293031629e", "/_apis/v1/Timeline/{scopeIdentifier}/{hubName}/{planId}/{timelineId}/attachments/{recordId}/{type}/{name}"),
		newServiceDef("CustomerIntelligence", "b5cc35c2-ff2b-491d-a085-24b6e9f396fd", "/_apis/v1/tasks"),
		newServiceDef("Tasks", "60aac929-f0cd-4bc8-9ce4-6b30e8f1b1bd", "/_apis/v1/tasks/{taskId}/{versionString}"),
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"instanceId": uuid.New().String(),
		"locationServiceData": map[string]interface{}{
			"serviceDefinitions": defs,
		},
	})
}

// handleOAuthToken exchanges a runner-signed client_assertion JWT for a
// session access token. The runner registered its RSA public key on
// /_apis/v1/Agent/{poolId}; the assertion's iss claim names the agent's
// ClientID and must verify against that stored public key.
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "failed to parse form body: "+err.Error())
		return
	}
	if grant := r.PostFormValue("grant_type"); grant != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", fmt.Sprintf("grant_type %q is not supported", grant))
		return
	}
	if at := r.PostFormValue("client_assertion_type"); at != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("client_assertion_type %q is not supported", at))
		return
	}
	assertion := r.PostFormValue("client_assertion")
	if assertion == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_assertion is required")
		return
	}

	agent, err := s.verifyAgentClientAssertion(assertion)
	if err != nil {
		s.logger.Warn().Err(err).Msg("agent client_assertion validation failed")
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", err.Error())
		return
	}

	s.logger.Debug().Int("agentId", agent.ID).Str("clientId", agent.Authorization.ClientID).Msg("oauth token issued")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": makeJWT(agent.Authorization.ClientID, "bleephub"),
		"expires_in":   604800,
		"scope":        "/",
		"token_type":   "access_token",
	})
}

func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"error": code, "error_description": desc})
	_, _ = w.Write(body)
}

// verifyAgentClientAssertion validates an RS256 JWT signed by the agent's
// registered RSA private key. The JWT's iss claim must match a known agent
// ClientID; the signature is verified against that agent's public key.
func (s *Server) verifyAgentClientAssertion(token string) (*Agent, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts")
	}

	headerBytes, err := base64urlDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode JWT header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("parse JWT header: %w", err)
	}
	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported JWT algorithm %q (expected RS256)", header.Alg)
	}

	payloadBytes, err := base64urlDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}
	var payload struct {
		Iss string  `json:"iss"`
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("parse JWT payload: %w", err)
	}
	if payload.Iss == "" {
		return nil, fmt.Errorf("missing iss claim")
	}
	if exp := int64(payload.Exp); exp > 0 && time.Now().Unix() > exp {
		return nil, fmt.Errorf("JWT expired")
	}

	agent := s.store.LookupAgentByClientID(payload.Iss)
	if agent == nil {
		return nil, fmt.Errorf("no agent registered with clientId %q", payload.Iss)
	}
	if agent.Authorization == nil || agent.Authorization.PublicKey == nil {
		return nil, fmt.Errorf("agent %d has no registered public key", agent.ID)
	}
	pubKey, err := agentRSAPublicKey(agent.Authorization.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("build agent public key: %w", err)
	}

	sigBytes, err := base64urlDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode JWT signature: %w", err)
	}
	signInput := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(signInput))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sigBytes); err != nil {
		return nil, fmt.Errorf("invalid JWT signature: %w", err)
	}
	return agent, nil
}

// agentRSAPublicKey reconstructs an *rsa.PublicKey from the modulus+exponent
// pair the runner sent during agent registration. The runner encodes both as
// standard base64, per the Azure DevOps agent protocol.
func agentRSAPublicKey(pk *AgentPublicKey) (*rsa.PublicKey, error) {
	modBytes, err := base64.StdEncoding.DecodeString(pk.Modulus)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	expBytes, err := base64.StdEncoding.DecodeString(pk.Exponent)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	if len(modBytes) == 0 || len(expBytes) == 0 {
		return nil, fmt.Errorf("empty modulus or exponent")
	}
	e := 0
	for _, b := range expBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("invalid public exponent (zero)")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(modBytes), E: e}, nil
}

// makeJWT creates a minimal unsigned JWT (alg:none) the runner can parse.
func makeJWT(sub, aud string) string {
	header := base64url([]byte(`{"alg":"none","typ":"JWT"}`))

	now := time.Now().Unix()
	exp := now + 86400*365 // 1 year

	payload := fmt.Sprintf(
		`{"sub":"%s","iss":"bleephub","aud":"%s","nbf":%d,"exp":%d,"scp":"Actions.Results:write Actions.Pipelines:read"}`,
		sub, aud, now, exp,
	)
	payloadEnc := base64url([]byte(payload))

	// "none" algorithm: empty signature
	return header + "." + payloadEnc + "."
}

func base64url(data []byte) string {
	s := base64.RawURLEncoding.EncodeToString(data)
	return strings.TrimRight(s, "=")
}
