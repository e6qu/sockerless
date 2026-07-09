package bleephub

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// --- shared fixtures for the secrets/variables suites ---

// sealForServer produces the {encrypted_value, key_id} pair a real client
// (libsodium sealed box against the public-key endpoint) would PUT.
func sealForServer(t *testing.T, plain string) (enc, keyID string) {
	t.Helper()
	enc, keyID, err := testServer.store.SealSecretValue(plain)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	return enc, keyID
}

// putSealedSecret PUTs a secret the way real clients do.
func putSealedSecret(t *testing.T, path, plain string) *http.Response {
	t.Helper()
	enc, keyID := sealForServer(t, plain)
	return ghPut(t, path, defaultToken, map[string]interface{}{
		"encrypted_value": enc,
		"key_id":          keyID,
	})
}

// seedTestRepo creates an admin-owned repo (idempotent across tests).
func seedTestRepo(t *testing.T, name string, private bool) *Repo {
	t.Helper()
	admin := testServer.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("default admin user missing")
	}
	if repo := testServer.store.GetRepo("admin", name); repo != nil {
		return repo
	}
	repo := testServer.store.CreateRepo(admin, name, "", private)
	if repo == nil {
		t.Fatalf("CreateRepo %s failed", name)
	}
	return repo
}

// seedTestOrg creates an org owned by admin (idempotent across tests).
func seedTestOrg(t *testing.T, login string) *Org {
	t.Helper()
	if org := testServer.store.GetOrg(login); org != nil {
		return org
	}
	admin := testServer.store.LookupUserByLogin("admin")
	org := testServer.store.CreateOrg(admin, login, login, "")
	if org == nil {
		t.Fatalf("CreateOrg %s failed", login)
	}
	return org
}

// seedOrgRepo creates an org-owned repo (idempotent across tests).
func seedOrgRepo(t *testing.T, org *Org, name string, private bool) *Repo {
	t.Helper()
	if repo := testServer.store.GetRepo(org.Login, name); repo != nil {
		return repo
	}
	admin := testServer.store.LookupUserByLogin("admin")
	repo := testServer.store.CreateOrgRepo(org, admin, name, "", private)
	if repo == nil {
		t.Fatalf("CreateOrgRepo %s/%s failed", org.Login, name)
	}
	return repo
}

func mustStatus(t *testing.T, resp *http.Response, want int, what string) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: status %d, want %d (body: %s)", what, resp.StatusCode, want, body)
	}
}

// --- repository secrets ---

func TestSecretsPublicKey(t *testing.T) {
	resp := ghGet(t, "/api/v3/repos/owner/repo/actions/secrets/public-key", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	body := decodeJSON(t, resp)
	kp, err := testServer.store.ActionsKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if body["key_id"] != kp.KeyID {
		t.Errorf("key_id = %v, want %s", body["key_id"], kp.KeyID)
	}
	if body["key"] != kp.PublicKey {
		t.Errorf("key = %v, want %s", body["key"], kp.PublicKey)
	}
}

func TestSecretsSealedRoundTrip(t *testing.T) {
	repo := seedTestRepo(t, "sealed-rt", false)
	path := "/api/v3/repos/" + repo.FullName + "/actions/secrets/ROUND_TRIP"

	mustStatus(t, putSealedSecret(t, path, "v1-plain"), 201, "create")

	secrets, _, err := testServer.CollectJobSecretsAndVars(repo.FullName, "")
	if err != nil {
		t.Fatal(err)
	}
	if secrets["ROUND_TRIP"] != "v1-plain" {
		t.Fatalf("injected = %q, want v1-plain", secrets["ROUND_TRIP"])
	}

	mustStatus(t, putSealedSecret(t, path, "v2-plain"), 204, "update")

	secrets, _, err = testServer.CollectJobSecretsAndVars(repo.FullName, "")
	if err != nil {
		t.Fatal(err)
	}
	if secrets["ROUND_TRIP"] != "v2-plain" {
		t.Fatalf("after update injected = %q, want v2-plain", secrets["ROUND_TRIP"])
	}
}

func TestSecretsPutWrongKeyID422(t *testing.T) {
	enc, keyID := sealForServer(t, "doesnt-matter")
	resp := ghPut(t, "/api/v3/repos/owner/repo/actions/secrets/WRONG_KID", defaultToken,
		map[string]interface{}{"encrypted_value": enc, "key_id": keyID + "0"})
	mustStatus(t, resp, 422, "wrong key_id")
}

func TestSecretsPutBadCiphertext422(t *testing.T) {
	_, keyID := sealForServer(t, "x")
	cases := []struct {
		name string
		enc  string
	}{
		{"valid base64, not a sealed box", "Z2FyYmFnZS1ub3QtYS1zZWFsZWQtYm94LWF0LWFsbC1ub3BlCg=="},
		{"not base64", "!!!not-base64!!!"},
		{"empty encrypted_value", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := ghPut(t, "/api/v3/repos/owner/repo/actions/secrets/BAD_CT", defaultToken,
				map[string]interface{}{"encrypted_value": tc.enc, "key_id": keyID})
			mustStatus(t, resp, 422, tc.name)
		})
	}
}

func TestSecretsPutMissingKeyID422(t *testing.T) {
	enc, _ := sealForServer(t, "x")
	resp := ghPut(t, "/api/v3/repos/owner/repo/actions/secrets/NO_KID", defaultToken,
		map[string]interface{}{"encrypted_value": enc})
	mustStatus(t, resp, 422, "missing key_id")
}

func TestSecretsBadNames422(t *testing.T) {
	enc, keyID := sealForServer(t, "x")
	body := map[string]interface{}{"encrypted_value": enc, "key_id": keyID}
	for _, name := range []string{"1STARTS_WITH_DIGIT", "HAS-DASH", "GITHUB_RESERVED", "github_reserved"} {
		resp := ghPut(t, "/api/v3/repos/owner/repo/actions/secrets/"+name, defaultToken, body)
		mustStatus(t, resp, 422, "bad name "+name)
	}
}

func TestSecretsListAndCaseInsensitiveGet(t *testing.T) {
	repo := seedTestRepo(t, "sec-list", false)
	base := "/api/v3/repos/" + repo.FullName + "/actions/secrets"
	mustStatus(t, putSealedSecret(t, base+"/lower_cased", "v"), 201, "create")

	list := decodeJSON(t, ghGet(t, base, defaultToken))
	if int(list["total_count"].(float64)) != 1 {
		t.Fatalf("total_count = %v, want 1", list["total_count"])
	}
	item := list["secrets"].([]interface{})[0].(map[string]interface{})
	if item["name"] != "LOWER_CASED" {
		t.Errorf("name = %v, want LOWER_CASED (uppercased)", item["name"])
	}
	if _, leaked := item["value"]; leaked {
		t.Error("list response exposes value member")
	}

	// Real GitHub treats secret names case-insensitively.
	one := decodeJSON(t, ghGet(t, base+"/LOWER_CASED", defaultToken))
	if one["name"] != "LOWER_CASED" {
		t.Errorf("get name = %v", one["name"])
	}
}

func TestSecretsValueNotExposed(t *testing.T) {
	repo := seedTestRepo(t, "sec-hidden", false)
	path := "/api/v3/repos/" + repo.FullName + "/actions/secrets/HIDDEN_VAL"
	mustStatus(t, putSealedSecret(t, path, "top-secret-plaintext"), 201, "create")

	resp := ghGet(t, path, defaultToken)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if bytes.Contains(body, []byte("top-secret-plaintext")) {
		t.Error("GET response exposes secret value")
	}
}

func TestSecretsDelete(t *testing.T) {
	repo := seedTestRepo(t, "sec-del", false)
	path := "/api/v3/repos/" + repo.FullName + "/actions/secrets/DELETE_ME"
	mustStatus(t, putSealedSecret(t, path, "val"), 201, "create")
	mustStatus(t, ghDelete(t, path, defaultToken), 204, "delete")
	mustStatus(t, ghGet(t, path, defaultToken), 404, "get after delete")
}

func TestSecretsNoAuth401(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/api/v3/repos/owner/repo/actions/secrets")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestSecretsMissingSecret404(t *testing.T) {
	mustStatus(t, ghGet(t, "/api/v3/repos/nonexist/repo/actions/secrets/NOPE", defaultToken), 404, "missing secret")
}

// --- organization secrets ---

func TestOrgSecretsMissingOrg404(t *testing.T) {
	mustStatus(t, ghGet(t, "/api/v3/orgs/no-such-org/actions/secrets", defaultToken), 404, "list")
	mustStatus(t, ghGet(t, "/api/v3/orgs/no-such-org/actions/secrets/public-key", defaultToken), 404, "public-key")
	mustStatus(t, putSealedSecret(t, "/api/v3/orgs/no-such-org/actions/secrets/X", "v"), 404, "put")
}

func TestOrgSecretsVisibilityRequired422(t *testing.T) {
	org := seedTestOrg(t, "secorg-vis")
	enc, keyID := sealForServer(t, "v")
	resp := ghPut(t, "/api/v3/orgs/"+org.Login+"/actions/secrets/NEEDS_VIS", defaultToken,
		map[string]interface{}{"encrypted_value": enc, "key_id": keyID})
	mustStatus(t, resp, 422, "missing visibility")

	resp = ghPut(t, "/api/v3/orgs/"+org.Login+"/actions/secrets/NEEDS_VIS", defaultToken,
		map[string]interface{}{"encrypted_value": enc, "key_id": keyID, "visibility": "everyone"})
	mustStatus(t, resp, 422, "invalid visibility")
}

func TestOrgSecretsLifecycle(t *testing.T) {
	org := seedTestOrg(t, "secorg-life")
	selRepo := seedOrgRepo(t, org, "sel-target", false)
	base := "/api/v3/orgs/" + org.Login + "/actions/secrets"

	// visibility=all secret.
	enc, keyID := sealForServer(t, "all-value")
	mustStatus(t, ghPut(t, base+"/ORG_ALL", defaultToken, map[string]interface{}{
		"encrypted_value": enc, "key_id": keyID, "visibility": "all",
	}), 201, "create all")

	// visibility=selected secret.
	enc, keyID = sealForServer(t, "sel-value")
	mustStatus(t, ghPut(t, base+"/ORG_SEL", defaultToken, map[string]interface{}{
		"encrypted_value": enc, "key_id": keyID, "visibility": "selected",
		"selected_repository_ids": []int{selRepo.ID},
	}), 201, "create selected")

	// List carries visibility; selected_repositories_url only on selected.
	list := decodeJSON(t, ghGet(t, base, defaultToken))
	if int(list["total_count"].(float64)) != 2 {
		t.Fatalf("total_count = %v, want 2", list["total_count"])
	}
	for _, raw := range list["secrets"].([]interface{}) {
		item := raw.(map[string]interface{})
		_, hasURL := item["selected_repositories_url"]
		switch item["name"] {
		case "ORG_ALL":
			if item["visibility"] != "all" || hasURL {
				t.Errorf("ORG_ALL: visibility=%v hasURL=%v", item["visibility"], hasURL)
			}
		case "ORG_SEL":
			if item["visibility"] != "selected" || !hasURL {
				t.Errorf("ORG_SEL: visibility=%v hasURL=%v", item["visibility"], hasURL)
			}
		}
	}

	// Selected-repositories list.
	repos := decodeJSON(t, ghGet(t, base+"/ORG_SEL/repositories", defaultToken))
	if int(repos["total_count"].(float64)) != 1 {
		t.Fatalf("repositories total_count = %v, want 1", repos["total_count"])
	}
	first := repos["repositories"].([]interface{})[0].(map[string]interface{})
	if int(first["id"].(float64)) != selRepo.ID {
		t.Errorf("repository id = %v, want %d", first["id"], selRepo.ID)
	}

	// Per-repo add/remove on a non-selected secret conflicts.
	idPath := fmt.Sprintf("%s/ORG_ALL/repositories/%d", base, selRepo.ID)
	mustStatus(t, ghPut(t, idPath, defaultToken, nil), 409, "add to visibility=all")
	mustStatus(t, ghDelete(t, idPath, defaultToken), 409, "remove from visibility=all")

	// Add + remove on the selected secret.
	other := seedOrgRepo(t, org, "sel-other", true)
	addPath := fmt.Sprintf("%s/ORG_SEL/repositories/%d", base, other.ID)
	mustStatus(t, ghPut(t, addPath, defaultToken, nil), 204, "add repo")
	repos = decodeJSON(t, ghGet(t, base+"/ORG_SEL/repositories", defaultToken))
	if int(repos["total_count"].(float64)) != 2 {
		t.Fatalf("after add total_count = %v, want 2", repos["total_count"])
	}
	mustStatus(t, ghDelete(t, addPath, defaultToken), 204, "remove repo")

	// Replace the selected set wholesale.
	mustStatus(t, ghPut(t, base+"/ORG_SEL/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{other.ID}}), 204, "set repositories")
	repos = decodeJSON(t, ghGet(t, base+"/ORG_SEL/repositories", defaultToken))
	if int(repos["total_count"].(float64)) != 1 {
		t.Fatalf("after set total_count = %v, want 1", repos["total_count"])
	}

	// Unknown repository id in the set → 404.
	mustStatus(t, ghPut(t, base+"/ORG_SEL/repositories", defaultToken,
		map[string]interface{}{"selected_repository_ids": []int{999999}}), 404, "set unknown repo id")

	// Delete; then 404.
	mustStatus(t, ghDelete(t, base+"/ORG_ALL", defaultToken), 204, "delete")
	mustStatus(t, ghGet(t, base+"/ORG_ALL", defaultToken), 404, "get after delete")
	mustStatus(t, ghDelete(t, base+"/ORG_ALL", defaultToken), 404, "delete again")
}

// --- environment secrets ---

func TestEnvSecretsMissingEnv404(t *testing.T) {
	repo := seedTestRepo(t, "env-missing", false)
	base := "/api/v3/repos/" + repo.FullName + "/environments/ghost/secrets"
	mustStatus(t, ghGet(t, base, defaultToken), 404, "list")
	mustStatus(t, ghGet(t, base+"/public-key", defaultToken), 404, "public-key")
	// Real GitHub does NOT auto-create the environment on secret PUT.
	mustStatus(t, putSealedSecret(t, base+"/NEW_SECRET", "v"), 404, "put")
	if env := testServer.store.Deployments.GetEnvironment(repo.ID, "ghost"); env != nil {
		t.Error("PUT must not auto-create the environment")
	}
}

func TestEnvSecretsLifecycle(t *testing.T) {
	repo := seedTestRepo(t, "env-life", false)
	testServer.store.Deployments.UpsertEnvironment(repo.ID, "production")
	base := "/api/v3/repos/" + repo.FullName + "/environments/production/secrets"

	pk := decodeJSON(t, ghGet(t, base+"/public-key", defaultToken))
	if pk["key_id"] == "" || pk["key"] == "" {
		t.Fatalf("public-key incomplete: %v", pk)
	}

	mustStatus(t, putSealedSecret(t, base+"/ENV_ONLY", "env-plain"), 201, "create")
	mustStatus(t, putSealedSecret(t, base+"/ENV_ONLY", "env-plain-2"), 204, "update")

	list := decodeJSON(t, ghGet(t, base, defaultToken))
	if int(list["total_count"].(float64)) != 1 {
		t.Fatalf("total_count = %v, want 1", list["total_count"])
	}

	one := decodeJSON(t, ghGet(t, base+"/ENV_ONLY", defaultToken))
	if one["name"] != "ENV_ONLY" {
		t.Errorf("name = %v", one["name"])
	}

	secrets, _, err := testServer.CollectJobSecretsAndVars(repo.FullName, "production")
	if err != nil {
		t.Fatal(err)
	}
	if secrets["ENV_ONLY"] != "env-plain-2" {
		t.Errorf("injected env secret = %q, want env-plain-2", secrets["ENV_ONLY"])
	}

	mustStatus(t, ghDelete(t, base+"/ENV_ONLY", defaultToken), 204, "delete")
	mustStatus(t, ghDelete(t, base+"/ENV_ONLY", defaultToken), 404, "delete again")
}

// --- repo-visible organization secrets ---

func TestRepoOrganizationSecretsList(t *testing.T) {
	org := seedTestOrg(t, "secorg-repovis")
	pubRepo := seedOrgRepo(t, org, "vis-pub", false)
	privRepo := seedOrgRepo(t, org, "vis-priv", true)
	base := "/api/v3/orgs/" + org.Login + "/actions/secrets"

	enc, keyID := sealForServer(t, "v")
	mustStatus(t, ghPut(t, base+"/VIS_ALL", defaultToken, map[string]interface{}{
		"encrypted_value": enc, "key_id": keyID, "visibility": "all",
	}), 201, "create all")
	enc, keyID = sealForServer(t, "v")
	mustStatus(t, ghPut(t, base+"/VIS_PRIV", defaultToken, map[string]interface{}{
		"encrypted_value": enc, "key_id": keyID, "visibility": "private",
	}), 201, "create private")

	names := func(repo *Repo) map[string]bool {
		body := decodeJSON(t, ghGet(t, "/api/v3/repos/"+repo.FullName+"/actions/organization-secrets", defaultToken))
		out := map[string]bool{}
		for _, raw := range body["secrets"].([]interface{}) {
			item := raw.(map[string]interface{})
			out[item["name"].(string)] = true
			// The documented repo-level item shape is the plain
			// actions-secret: no visibility member.
			if _, has := item["visibility"]; has {
				t.Errorf("repo-level org secret item leaks visibility: %v", item)
			}
		}
		return out
	}

	pub := names(pubRepo)
	if !pub["VIS_ALL"] || pub["VIS_PRIV"] {
		t.Errorf("public repo sees %v, want VIS_ALL only", pub)
	}
	priv := names(privRepo)
	if !priv["VIS_ALL"] || !priv["VIS_PRIV"] {
		t.Errorf("private repo sees %v, want VIS_ALL and VIS_PRIV", priv)
	}

	mustStatus(t, ghGet(t, "/api/v3/repos/admin/no-such-repo/actions/organization-secrets", defaultToken), 404, "missing repo")
}
