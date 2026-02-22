package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerAuthRoutes() {
	// Runner registration (GHES-style)
	s.mux.HandleFunc("POST /api/v3/actions/runner-registration", s.handleRunnerRegistration)

	// Connection data (service discovery)
	s.mux.HandleFunc("GET /_apis/connectionData", s.handleConnectionData)

	// OAuth token exchange
	s.mux.HandleFunc("POST /_apis/v1/auth/", s.handleOAuthToken)
	s.mux.HandleFunc("POST /_apis/v1/auth", s.handleOAuthToken)
}

// handleRunnerRegistration returns the tenant URL and a management token.
// The runner calls this during `config.sh --url <url> --token <token>`.
func (s *Server) handleRunnerRegistration(w http.ResponseWriter, r *http.Request) {
	s.logger.Info().Msg("runner registration request")

	// Parse request body — runner sends the original --url value
	var req struct {
		URL         string `json:"url"`
		RunnerEvent string `json:"runner_event"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Build server URL from request host
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	serverURL := scheme + "://" + r.Host

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

// serviceDefinition matches the Azure DevOps ServiceDefinition format
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

// handleConnectionData returns service location data.
// The runner uses this to discover API endpoint paths via GUIDs.
// Format matches ChristopherHX/runner.server exactly.
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

	// Minimal ConnectionData — match runner.server format (no authenticatedUser/authorizedUser)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"instanceId": uuid.New().String(),
		"locationServiceData": map[string]interface{}{
			"serviceDefinitions": defs,
		},
	})
}

// handleOAuthToken accepts a JWT assertion and returns an access token.
// The runner exchanges its RSA-signed JWT for an OAuth bearer token.
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("oauth token exchange")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": makeJWT("bleephub", "bleephub"),
		"expires_in":   604800,
		"scope":        "/",
		"token_type":   "access_token",
	})
}

// makeJWT creates a minimal unsigned JWT that the runner can parse.
// The runner's JsonWebToken deserializer needs a valid 3-part JWT structure.
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
