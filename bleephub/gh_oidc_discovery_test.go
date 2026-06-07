package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The OIDC discovery document must carry the metadata OpenID Connect Discovery
// 1.0 § 3 marks REQUIRED for a provider that supports the authorization-code
// flow, so a relying party (Pomerium, Teleport, openid-client, …) can configure
// itself from the document alone. bleephub already implements the
// authorize/token/userinfo endpoints (gh_oauth.go / gh_rest.go); this test pins
// that the discovery document advertises them and that each advertised endpoint
// actually routes.
func TestOIDCDiscovery_AdvertisesOAuthEndpoints(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	s.registerGHOAuthRoutes()
	s.registerGHRestRoutes()

	req := httptest.NewRequest("GET", "/.well-known/openid-configuration", nil)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("discovery status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("discovery body not JSON: %v", err)
	}
	base := "http://" + req.Host

	// OpenID Connect Discovery 1.0 § 3 REQUIRED metadata for an OP supporting
	// the authorization-code flow.
	wantString := map[string]string{
		"issuer":                 base + "/",
		"authorization_endpoint": base + "/login/oauth/authorize",
		"token_endpoint":         base + "/login/oauth/access_token",
		"userinfo_endpoint":      base + "/api/v3/user",
		"jwks_uri":               base + "/.well-known/jwks",
	}
	for field, want := range wantString {
		got, ok := doc[field].(string)
		if !ok {
			t.Errorf("discovery missing required string field %q", field)
			continue
		}
		if got != want {
			t.Errorf("discovery %q = %q, want %q", field, got, want)
		}
	}

	// response_types_supported MUST advertise "code" since the authorize
	// endpoint serves the authorization-code grant.
	if rt := toStringSet(doc["response_types_supported"]); !rt["code"] {
		t.Errorf("response_types_supported = %v, must contain \"code\"", doc["response_types_supported"])
	}
	if gt := toStringSet(doc["grant_types_supported"]); !gt["authorization_code"] {
		t.Errorf("grant_types_supported = %v, must contain \"authorization_code\"", doc["grant_types_supported"])
	}

	// Every advertised endpoint must actually be a registered route (not 404).
	// Drives the real handlers rather than trusting the document.
	endpoints := []struct {
		method, field string
	}{
		{"GET", "authorization_endpoint"},
		{"POST", "token_endpoint"},
		{"GET", "userinfo_endpoint"},
	}
	for _, ep := range endpoints {
		url, _ := doc[ep.field].(string)
		path := strings.TrimPrefix(url, base)
		r := httptest.NewRequest(ep.method, path, nil)
		rec := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(rec, r)
		if rec.Code == http.StatusNotFound {
			t.Errorf("advertised %s (%s %s) is not a registered route (404)", ep.field, ep.method, path)
		}
	}
}

func toStringSet(v any) map[string]bool {
	out := map[string]bool{}
	arr, ok := v.([]any)
	if !ok {
		return out
	}
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out[s] = true
		}
	}
	return out
}
