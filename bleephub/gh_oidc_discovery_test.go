package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// seedOIDCRepoOwner creates a user with the given login so a repo can be
// created under it (the OIDC mint requires the repo to actually exist).
func seedOIDCRepoOwner(s *Server, login string) *User {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	u := &User{ID: s.store.NextUser, Login: login, Type: "User", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	s.store.NextUser++
	s.store.Users[u.ID] = u
	s.store.UsersByLogin[login] = u
	return u
}

// fullOIDCQuery builds a complete, real run context — the OIDC mint refuses to
// fabricate any of these, so a faithful caller must supply them all.
func fullOIDCQuery(repo string, extra ...string) string {
	q := "repo=" + repo +
		"&ref=refs/heads/main" +
		"&sha=0123456789abcdef0123456789abcdef01234567" +
		"&run_id=42&run_number=7&run_attempt=1" +
		"&workflow=CI&workflow_file=ci.yml&event_name=push"
	for _, e := range extra {
		q += "&" + e
	}
	return q
}

// decodeOIDCToken mints a token via GET /token with the given query and returns
// the decoded JWT payload claims.
func decodeOIDCToken(t *testing.T, s *Server, query string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("GET", "/token?"+query, nil)
	req.Header.Set("Authorization", "token "+adminPAT)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/token status = %d, body = %s", w.Code, w.Body.String())
	}
	var env struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("token envelope not JSON: %v", err)
	}
	parts := strings.Split(env.Value, ".")
	if len(parts) != 3 {
		t.Fatalf("token is not a 3-part JWT: %q", env.Value)
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(pb, &claims); err != nil {
		t.Fatalf("JWT payload not JSON: %v", err)
	}
	return claims
}

// repository_owner must be the owner segment of the repository claim, not a
// hardcoded value — OIDC trust policies key on it.
func TestOIDCToken_RepositoryOwnerDerived(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()

	owner := seedOIDCRepoOwner(s, "octo-org")
	s.store.CreateRepo(owner, "octo-repo", "", false)

	claims := decodeOIDCToken(t, s, fullOIDCQuery("octo-org/octo-repo"))
	if claims["repository"] != "octo-org/octo-repo" {
		t.Fatalf("repository = %v, want octo-org/octo-repo", claims["repository"])
	}
	if claims["repository_owner"] != "octo-org" {
		t.Fatalf("repository_owner = %v, want octo-org (owner segment)", claims["repository_owner"])
	}
	if claims["repository_owner"] == "admin" {
		t.Fatal("repository_owner must not be hardcoded admin")
	}
}

// sub reflects the environment when supplied, else the ref form.
func TestOIDCToken_SubReflectsEnvironment(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()

	owner := seedOIDCRepoOwner(s, "acme")
	s.store.CreateRepo(owner, "app", "", false)

	noEnv := decodeOIDCToken(t, s, fullOIDCQuery("acme/app"))
	if noEnv["sub"] != "repo:acme/app:ref:refs/heads/main" {
		t.Fatalf("sub (no env) = %v, want repo:acme/app:ref:refs/heads/main", noEnv["sub"])
	}

	withEnv := decodeOIDCToken(t, s, fullOIDCQuery("acme/app", "environment=production"))
	if withEnv["sub"] != "repo:acme/app:environment:production" {
		t.Fatalf("sub (env) = %v, want repo:acme/app:environment:production", withEnv["sub"])
	}
}

// The standard Actions-OIDC claim set must be present and populated.
func TestOIDCToken_StandardClaimsPresent(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "claims-repo", "", false)

	claims := decodeOIDCToken(t, s, "repo="+repo.FullName+
		"&ref=refs/heads/main&sha=0123456789abcdef0123456789abcdef01234567"+
		"&run_id=42&run_number=7&run_attempt=2&workflow=Deploy&workflow_file=deploy.yml&event_name=push")

	for _, k := range []string{
		"repository_id", "repository_owner_id", "actor_id", "run_attempt",
		"workflow", "workflow_ref", "workflow_sha", "job_workflow_ref",
		"job_workflow_sha", "head_ref", "base_ref", "event_name", "ref_type",
		"repository_visibility", "runner_environment",
	} {
		if _, ok := claims[k]; !ok {
			t.Errorf("missing standard claim %q", k)
		}
	}

	// ids resolved from the store, emitted as strings (matching real GitHub).
	if claims["repository_id"] != strconv.Itoa(repo.ID) {
		t.Errorf("repository_id = %v, want %q", claims["repository_id"], strconv.Itoa(repo.ID))
	}
	if claims["repository_owner_id"] != strconv.Itoa(admin.ID) {
		t.Errorf("repository_owner_id = %v, want %q", claims["repository_owner_id"], strconv.Itoa(admin.ID))
	}
	if claims["run_id"] != "42" || claims["run_number"] != "7" || claims["run_attempt"] != "2" {
		t.Errorf("run claims = id:%v number:%v attempt:%v", claims["run_id"], claims["run_number"], claims["run_attempt"])
	}
	if claims["ref_type"] != "branch" {
		t.Errorf("ref_type = %v, want branch", claims["ref_type"])
	}
	wantJWRef := repo.FullName + "/.github/workflows/deploy.yml@refs/heads/main"
	if claims["job_workflow_ref"] != wantJWRef {
		t.Errorf("job_workflow_ref = %v, want %q", claims["job_workflow_ref"], wantJWRef)
	}
	if claims["repository_visibility"] != "public" {
		t.Errorf("repository_visibility = %v, want public", claims["repository_visibility"])
	}
}

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
