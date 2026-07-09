package bleephub

import (
	"testing"
	"time"
)

func TestRepoCodeQualitySetup_DefaultAndRoundTrip(t *testing.T) {
	admin := testServer.store.LookupUserByLogin("admin")
	repo := testServer.store.CreateRepo(admin, "code-quality-repo", "", false)
	path := "/api/v3/repos/" + repo.FullName + "/code-quality/setup"

	// Unconfigured default.
	setup := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if setup["state"] != "not-configured" {
		t.Fatalf("default state = %v, want not-configured", setup["state"])
	}
	if langs := setup["languages"].([]interface{}); len(langs) != 0 {
		t.Fatalf("default languages = %v, want empty", langs)
	}
	for _, field := range []string{"runner_type", "runner_label", "updated_at", "schedule"} {
		if setup[field] != nil {
			t.Fatalf("default %s = %v, want null", field, setup[field])
		}
	}

	// Configure and read back.
	body := decodeJSONWithStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{
		"state": "configured", "languages": []string{"go", "python"}, "runner_type": "standard"}), 200)
	if len(body) != 0 {
		t.Fatalf("update response = %v, want empty object", body)
	}
	setup = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if setup["state"] != "configured" || setup["schedule"] != "weekly" || setup["runner_type"] != "standard" {
		t.Fatalf("configured setup = %v", setup)
	}
	if langs := setup["languages"].([]interface{}); len(langs) != 2 || langs[0] != "go" {
		t.Fatalf("configured languages = %v", langs)
	}
	if setup["updated_at"] == nil {
		t.Fatalf("updated_at still null after update: %v", setup)
	}

	// A labeled runner keeps its label.
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{
		"runner_type": "labeled", "runner_label": "code-scanning"}), 200)
	setup = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if setup["runner_type"] != "labeled" || setup["runner_label"] != "code-scanning" {
		t.Fatalf("labeled setup = %v", setup)
	}

	// Disabling clears the schedule.
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{"state": "not-configured"}), 200)
	setup = decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if setup["state"] != "not-configured" || setup["schedule"] != nil {
		t.Fatalf("disabled setup = %v", setup)
	}
}

func TestRepoCodeQualitySetup_Validation(t *testing.T) {
	admin := testServer.store.LookupUserByLogin("admin")
	repo := testServer.store.CreateRepo(admin, "code-quality-validation-repo", "", false)
	path := "/api/v3/repos/" + repo.FullName + "/code-quality/setup"

	// At least one updatable member is required.
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{}), 422)

	// Unknown members are rejected (additionalProperties: false).
	requireStatus(t, ghPatch(t, path, defaultToken,
		map[string]interface{}{"state": "configured", "query_suite": "default"}), 422)

	// Enum violations.
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{"state": "enabled"}), 422)
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{"languages": []string{"cobol"}}), 422)

	// A labeled runner without a label is unusable.
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{"runner_type": "labeled"}), 422)

	// Rejected updates must not leak partial mutations into store state.
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{
		"state": "configured", "languages": []string{"go"}, "runner_type": "standard",
	}), 200)
	requireStatus(t, ghPatch(t, path, defaultToken, map[string]interface{}{"runner_type": "labeled"}), 422)
	setup := decodeJSONWithStatus(t, ghGet(t, path, defaultToken), 200)
	if setup["runner_type"] != "standard" || setup["runner_label"] != nil || setup["state"] != "configured" {
		t.Fatalf("rejected update mutated setup = %v", setup)
	}

	// Only repository admins may update; other users read a public
	// repository but get 403 on writes.
	outsider := seedTestUser(testServer, "code-quality-outsider")
	outsiderTok := testServer.store.CreateToken(outsider.ID, "repo").Value
	requireStatus(t, ghGet(t, path, outsiderTok), 200)
	requireStatus(t, ghPatch(t, path, outsiderTok, map[string]interface{}{"state": "configured"}), 403)

	// Unknown repository → 404.
	requireStatus(t, ghGet(t, "/api/v3/repos/admin/no-such-repo/code-quality/setup", defaultToken), 404)
}

func TestStoreCodeQualitySetup_SnapshotsStoreBoundary(t *testing.T) {
	st := NewStore()
	now := time.Now().UTC()
	wantUpdatedAt := now
	input := &CodeQualitySetup{
		RepoFullName: "admin/code-quality-boundary",
		State:        "configured",
		Languages:    []string{"go"},
		RunnerType:   "standard",
		UpdatedAt:    &now,
	}
	st.SetCodeQualitySetup(input)

	input.Languages[0] = "python"
	*input.UpdatedAt = input.UpdatedAt.Add(time.Hour)
	got := st.GetCodeQualitySetup(input.RepoFullName)
	if got.Languages[0] != "go" || !got.UpdatedAt.Equal(wantUpdatedAt) {
		t.Fatalf("SetCodeQualitySetup stored caller-owned references: %+v", got)
	}

	got.Languages[0] = "ruby"
	*got.UpdatedAt = got.UpdatedAt.Add(time.Hour)
	again := st.GetCodeQualitySetup(input.RepoFullName)
	if again.Languages[0] != "go" || !again.UpdatedAt.Equal(wantUpdatedAt) {
		t.Fatalf("GetCodeQualitySetup returned store-owned references: %+v", again)
	}
}
