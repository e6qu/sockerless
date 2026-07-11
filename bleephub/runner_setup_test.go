package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// TestRegistrationTokenRandom verifies the repo registration token is a
// random per-request opaque value (not the old hardcoded constant) with a
// near-term expiry, and that an authenticated caller gets 201.
func TestRegistrationTokenRandom(t *testing.T) {
	mint := func() (string, string) {
		resp := ghPost(t, "/api/v3/repos/admin/regtok/actions/runners/registration-token", defaultToken, map[string]interface{}{})
		if resp.StatusCode != 201 {
			resp.Body.Close()
			t.Fatalf("registration-token = %d, want 201", resp.StatusCode)
		}
		var body struct {
			Token     string `json:"token"`
			ExpiresAt string `json:"expires_at"`
		}
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		return body.Token, body.ExpiresAt
	}

	t1, exp1 := mint()
	t2, _ := mint()
	if t1 == "" || t2 == "" {
		t.Fatal("registration token must be non-empty")
	}
	if t1 == "BLEEPHUB_REG_TOKEN" {
		t.Error("registration token must not be the hardcoded constant")
	}
	if t1 == t2 {
		t.Error("registration token must be random per request, got identical values")
	}
	if exp1 == "" || exp1 == "2099-01-01T00:00:00Z" {
		t.Errorf("expires_at must be a near-term TTL, got %q", exp1)
	}
}

func TestAgentRSAPublicKeyRequiresProtocolStandardBase64(t *testing.T) {
	pub, err := agentRSAPublicKey(&AgentPublicKey{
		Modulus:  base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}),
		Exponent: base64.StdEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
	})
	if err != nil {
		t.Fatalf("standard-base64 public key rejected: %v", err)
	}
	if pub.E != 65537 {
		t.Fatalf("exponent = %d, want 65537", pub.E)
	}

	for name, pk := range map[string]*AgentPublicKey{
		"url-safe modulus": {
			Modulus:  base64.URLEncoding.EncodeToString([]byte{0xff, 0xff}),
			Exponent: base64.StdEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
		},
		"raw standard modulus": {
			Modulus:  base64.RawStdEncoding.EncodeToString([]byte{0xff, 0xff}),
			Exponent: base64.StdEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
		},
		"raw url-safe exponent": {
			Modulus:  base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}),
			Exponent: base64.RawURLEncoding.EncodeToString([]byte{0xff, 0xff}),
		},
	} {
		if _, err := agentRSAPublicKey(pk); err == nil {
			t.Fatalf("%s was accepted; runner public keys must use protocol-standard base64", name)
		}
	}
}

// TestRemoveToken verifies the repo removal token endpoint returns the
// {token, expires_at} shape with 201 for an authenticated caller.
func TestRemoveToken(t *testing.T) {
	resp := ghPost(t, "/api/v3/repos/admin/rmtok/actions/runners/remove-token", defaultToken, map[string]interface{}{})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("remove-token = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if body.Token == "" {
		t.Error("remove token must be non-empty")
	}
	if body.ExpiresAt == "" {
		t.Error("remove token must carry expires_at")
	}
}

// TestGenerateJITConfig verifies the repo generate-jitconfig endpoint mints
// a runner + a decodable base64 JIT config, validates required fields, and
// registers the runner so it appears in the runners list.
func TestGenerateJITConfig(t *testing.T) {
	// Missing required fields → 422.
	bad := ghPost(t, "/api/v3/repos/admin/jit/actions/runners/generate-jitconfig", defaultToken,
		map[string]interface{}{"name": "jit-runner"})
	if bad.StatusCode != 422 {
		bad.Body.Close()
		t.Fatalf("jitconfig missing fields = %d, want 422", bad.StatusCode)
	}
	bad.Body.Close()

	rgid := 1
	resp := ghPost(t, "/api/v3/repos/admin/jit/actions/runners/generate-jitconfig", defaultToken,
		map[string]interface{}{
			"name":            "jit-runner",
			"runner_group_id": rgid,
			"labels":          []string{"self-hosted", "linux"},
		})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("jitconfig = %d, want 201", resp.StatusCode)
	}
	var body struct {
		Runner struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"runner"`
		EncodedJITConfig string `json:"encoded_jit_config"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()

	if body.Runner.ID == 0 || body.Runner.Name != "jit-runner" {
		t.Errorf("runner = %+v, want id>0 name=jit-runner", body.Runner)
	}
	if len(body.Runner.Labels) != 2 {
		t.Errorf("runner labels = %d, want 2", len(body.Runner.Labels))
	}
	if body.EncodedJITConfig == "" {
		t.Fatal("encoded_jit_config must be non-empty")
	}
	// The runner must be able to consume the JIT config: it's base64 of a
	// JSON blob carrying the agent identity + server URL.
	raw, err := base64.StdEncoding.DecodeString(body.EncodedJITConfig)
	if err != nil {
		t.Fatalf("encoded_jit_config is not valid base64: %v", err)
	}
	var blob map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blob); err != nil {
		t.Fatalf("decoded JIT config is not valid JSON: %v", err)
	}
	if _, ok := blob[".runner"]; !ok {
		t.Errorf("JIT config missing .runner section: %s", raw)
	}
	if _, ok := blob[".credentials"]; !ok {
		t.Errorf("JIT config missing .credentials section: %s", raw)
	}

	// The minted runner must be registered (visible in the runners list).
	listResp := ghGet(t, "/api/v3/repos/admin/jit/actions/runners", defaultToken)
	if listResp.StatusCode != 200 {
		listResp.Body.Close()
		t.Fatalf("list runners = %d, want 200", listResp.StatusCode)
	}
	var list struct {
		Runners []struct {
			ID int64 `json:"id"`
		} `json:"runners"`
	}
	json.NewDecoder(listResp.Body).Decode(&list)
	listResp.Body.Close()
	found := false
	for _, rr := range list.Runners {
		if rr.ID == body.Runner.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("JIT-minted runner not present in runners list")
	}
}
