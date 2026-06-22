package gcp_cli_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIAMCustomRoleLifecycleCLI exercises `gcloud iam roles` against the sim:
// create → describe → list → update → delete → undelete. Custom-role CRUD
// rides the IAM endpoint override (CLOUDSDK_API_ENDPOINT_OVERRIDES_IAM).
func TestIAMCustomRoleLifecycleCLI(t *testing.T) {
	const roleID = "cliCustomRole"

	out := runCLI(t, gcloudCLI("iam", "roles", "create", roleID,
		"--project="+project,
		"--title=CLI Custom Role",
		"--description=created via cli",
		"--permissions=storage.objects.get,storage.objects.list",
		"--stage=GA",
		"--format=json",
	))
	var created struct {
		Name                string   `json:"name"`
		Title               string   `json:"title"`
		IncludedPermissions []string `json:"includedPermissions"`
		Etag                string   `json:"etag"`
	}
	parseJSONObject(t, out, &created)
	require.Equal(t, "projects/"+project+"/roles/"+roleID, created.Name)
	require.Equal(t, "CLI Custom Role", created.Title)
	require.ElementsMatch(t, []string{"storage.objects.get", "storage.objects.list"}, created.IncludedPermissions)

	out = runCLI(t, gcloudCLI("iam", "roles", "describe", roleID,
		"--project="+project, "--format=json"))
	var described struct {
		Name                string   `json:"name"`
		IncludedPermissions []string `json:"includedPermissions"`
	}
	parseJSONObject(t, out, &described)
	require.Equal(t, created.Name, described.Name)
	require.NotEmpty(t, described.IncludedPermissions, "describe returns FULL view")

	out = runCLI(t, gcloudCLI("iam", "roles", "list", "--project="+project, "--format=json"))
	jsonStart := strings.Index(out, "[")
	require.NotEqual(t, -1, jsonStart, "list output not a JSON array: %s", out)
	var listed []struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal([]byte(out[jsonStart:]), &listed), "output: %s", out)
	found := false
	for _, r := range listed {
		if r.Name == created.Name {
			found = true
		}
	}
	require.True(t, found, "created role must appear in list")

	out = runCLI(t, gcloudCLI("iam", "roles", "update", roleID,
		"--project="+project,
		"--title=Renamed CLI Role",
		"--format=json",
	))
	var updated struct {
		Title string `json:"title"`
		Etag  string `json:"etag"`
	}
	parseJSONObject(t, out, &updated)
	require.Equal(t, "Renamed CLI Role", updated.Title)
	require.NotEqual(t, created.Etag, updated.Etag, "etag must rotate on update")

	out = runCLI(t, gcloudCLI("iam", "roles", "delete", roleID,
		"--project="+project, "--format=json"))
	var deleted struct {
		Deleted bool `json:"deleted"`
	}
	parseJSONObject(t, out, &deleted)
	require.True(t, deleted.Deleted, "delete soft-deletes the role")

	out = runCLI(t, gcloudCLI("iam", "roles", "undelete", roleID,
		"--project="+project, "--format=json"))
	var revived struct {
		Deleted bool `json:"deleted"`
	}
	parseJSONObject(t, out, &revived)
	require.False(t, revived.Deleted, "undelete clears the deleted flag")
}

// TestIAMServiceAccountAsResourceIAMCLI exercises get-iam-policy +
// add-iam-policy-binding on a service account (the SA-as-resource IAM triple),
// plus :testIamPermissions directly against the wire path.
func TestIAMServiceAccountAsResourceIAMCLI(t *testing.T) {
	const accountID = "cli-iam-sa"
	email := accountID + "@" + project + ".iam.gserviceaccount.com"

	runCLI(t, gcloudCLI("iam", "service-accounts", "create", accountID,
		"--display-name=CLI IAM SA", "--format=json"))

	// get-iam-policy on a fresh SA → empty bindings, an etag present.
	out := runCLI(t, gcloudCLI("iam", "service-accounts", "get-iam-policy", email, "--format=json"))
	var pol struct {
		Etag string `json:"etag"`
	}
	parseJSONObject(t, out, &pol)
	require.NotEmpty(t, pol.Etag)

	// add-iam-policy-binding read-modify-writes the SA's policy (get+set).
	out = runCLI(t, gcloudCLI("iam", "service-accounts", "add-iam-policy-binding", email,
		"--member=user:dev@example.com",
		"--role=roles/iam.serviceAccountUser",
		"--format=json",
	))
	var bound struct {
		Bindings []struct {
			Role    string   `json:"role"`
			Members []string `json:"members"`
		} `json:"bindings"`
	}
	parseJSONObject(t, out, &bound)
	require.Len(t, bound.Bindings, 1)
	require.Equal(t, "roles/iam.serviceAccountUser", bound.Bindings[0].Role)
	require.Contains(t, bound.Bindings[0].Members, "user:dev@example.com")

	// get-iam-policy reflects the binding.
	out = runCLI(t, gcloudCLI("iam", "service-accounts", "get-iam-policy", email, "--format=json"))
	parseJSONObject(t, out, &bound)
	require.Len(t, bound.Bindings, 1)

	// testIamPermissions has no gcloud surface on service-accounts; hit the
	// wire path directly. The admin caller echoes the requested set.
	saName := fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
	body := `{"permissions":["iam.serviceAccounts.actAs","iam.serviceAccounts.get"]}`
	respBody := httpDoJSON(t, "POST", baseURL+"/v1/"+saName+":testIamPermissions", body)
	var tip struct {
		Permissions []string `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(respBody), &tip))
	require.ElementsMatch(t,
		[]string{"iam.serviceAccounts.actAs", "iam.serviceAccounts.get"}, tip.Permissions)
}

// TestIAMServiceAccountDisableEnableUpdateCLI exercises :disable, :enable, and
// PATCH (update) on a service account.
func TestIAMServiceAccountDisableEnableUpdateCLI(t *testing.T) {
	const accountID = "cli-toggle-sa"
	email := accountID + "@" + project + ".iam.gserviceaccount.com"

	runCLI(t, gcloudCLI("iam", "service-accounts", "create", accountID,
		"--display-name=Toggle SA", "--format=json"))

	runCLI(t, gcloudCLI("iam", "service-accounts", "disable", email))
	out := runCLI(t, gcloudCLI("iam", "service-accounts", "describe", email, "--format=json"))
	var afterDisable struct {
		Disabled bool `json:"disabled"`
	}
	parseJSONObject(t, out, &afterDisable)
	require.True(t, afterDisable.Disabled, "disable sets disabled=true")

	runCLI(t, gcloudCLI("iam", "service-accounts", "enable", email))
	out = runCLI(t, gcloudCLI("iam", "service-accounts", "describe", email, "--format=json"))
	var afterEnable struct {
		Disabled bool `json:"disabled"`
	}
	parseJSONObject(t, out, &afterEnable)
	require.False(t, afterEnable.Disabled, "enable sets disabled=false")

	// PATCH displayName via the wire path (gcloud's `update` shells the same
	// PATCH with an updateMask).
	out = runCLI(t, gcloudCLI("iam", "service-accounts", "update", email,
		"--display-name=Renamed Toggle SA", "--format=json"))
	var updated struct {
		DisplayName string `json:"displayName"`
	}
	parseJSONObject(t, out, &updated)
	assert.Equal(t, "Renamed Toggle SA", updated.DisplayName)
}
