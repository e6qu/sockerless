package azure_sdk_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeyVault_State_FullVersionChain locks the BUG-1149 versioning
// invariant: PUT appends a new version per call; ListSecretProperties
// returns the canonical paged SecretListResult with one item per
// version (NOT a single SecretBundle envelope); a version-specific
// GET reads the exact bytes that PUT wrote.
//
// Pre-fix the sim stored one row per name (PUT overwrote); ListVersions
// returned the wrong envelope shape; SDK paged iterators returned zero
// items.
func TestKeyVault_State_FullVersionChain(t *testing.T) {
	rg := "kv-state-versions-rg"
	vault := "kv-state-versions"
	createKVViaARM(t, rg, vault)

	c, err := azsecrets.NewClient(kvVaultURL(vault), &fakeCredential{},
		&azsecrets.ClientOptions{
			ClientOptions:                        kvClientOptions(),
			DisableChallengeResourceVerification: true,
		})
	require.NoError(t, err)

	// PUT 3 versions. Each call should append, not overwrite.
	values := []string{"v1", "v2", "v3"}
	versionIDs := make([]string, 0, len(values))
	for _, v := range values {
		out, err := c.SetSecret(ctx, "chain", azsecrets.SetSecretParameters{Value: stringPtr(v)}, nil)
		require.NoError(t, err)
		require.NotNil(t, out.ID)
		versionIDs = append(versionIDs, out.ID.Version())
	}

	// Versions must be distinct UUIDs.
	require.Len(t, versionIDs, 3)
	assert.NotEqual(t, versionIDs[0], versionIDs[1])
	assert.NotEqual(t, versionIDs[1], versionIDs[2])

	// Latest GET returns the last-written value.
	latest, err := c.GetSecret(ctx, "chain", "", nil)
	require.NoError(t, err)
	require.NotNil(t, latest.Value)
	assert.Equal(t, "v3", *latest.Value, "GET without version must return latest")

	// Version-specific GET returns the exact bytes.
	for i, version := range versionIDs {
		got, err := c.GetSecret(ctx, "chain", version, nil)
		require.NoError(t, err, "version-specific GET must succeed for %s", version)
		require.NotNil(t, got.Value)
		assert.Equal(t, values[i], *got.Value,
			"version %s must return original PUT value %q", version, values[i])
	}

	// Paged iterator over versions returns all 3 (load-bearing test:
	// uses NewListSecretPropertiesVersionsPager, not auto-unwrap).
	pager := c.NewListSecretPropertiesVersionsPager("chain", nil)
	var seen []string
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err, "paged iterator must succeed (BUG-1149 regression guard)")
		for _, item := range page.Value {
			require.NotNil(t, item.ID)
			seen = append(seen, item.ID.Version())
		}
	}
	assert.ElementsMatch(t, versionIDs, seen,
		"paged List must surface all PUT versions")
}

// TestKeyVault_State_SoftDeleteRoundTrip locks the BUG-1151 soft-
// delete state machine: active → deleted → recovered → deleted →
// purged. Each transition must be observable via the SDK's canonical
// methods (DeleteSecret / GetDeletedSecret / RecoverDeletedSecret /
// PurgeDeletedSecret) and the response shape must include the
// RecoveryId / ScheduledPurgeDate fields the state-machine-completeness
// skill mandates.
func TestKeyVault_State_SoftDeleteRoundTrip(t *testing.T) {
	rg := "kv-state-softdel-rg"
	vault := "kv-state-softdel"
	createKVViaARM(t, rg, vault)

	c, err := azsecrets.NewClient(kvVaultURL(vault), &fakeCredential{},
		&azsecrets.ClientOptions{
			ClientOptions:                        kvClientOptions(),
			DisableChallengeResourceVerification: true,
		})
	require.NoError(t, err)

	// active.
	_, err = c.SetSecret(ctx, "doomed", azsecrets.SetSecretParameters{Value: stringPtr("alive")}, nil)
	require.NoError(t, err)

	// active → deleted. DeleteSecret returns DeletedSecretBundle with
	// RecoveryId + ScheduledPurgeDate set.
	delResp, err := c.DeleteSecret(ctx, "doomed", nil)
	require.NoError(t, err)
	require.NotNil(t, delResp.RecoveryID, "RecoveryID must be set on soft-delete")
	require.NotNil(t, delResp.ScheduledPurgeDate, "ScheduledPurgeDate must be set")

	// GET via /secrets/{name} now 404s.
	_, err = c.GetSecret(ctx, "doomed", "", nil)
	require.Error(t, err, "GetSecret on soft-deleted secret must 404")

	// GET via /deletedsecrets/{name} reads the deleted bundle.
	delGot, err := c.GetDeletedSecret(ctx, "doomed", nil)
	require.NoError(t, err)
	require.NotNil(t, delGot.RecoveryID)
	assert.Equal(t, "alive", *delGot.Value,
		"DeletedSecret retains the latest version's value while in soft-delete")

	// deleted → active.
	recResp, err := c.RecoverDeletedSecret(ctx, "doomed", nil)
	require.NoError(t, err)
	require.NotNil(t, recResp.Value)
	assert.Equal(t, "alive", *recResp.Value)

	// GET via /secrets/{name} succeeds again.
	got, err := c.GetSecret(ctx, "doomed", "", nil)
	require.NoError(t, err)
	require.NotNil(t, got.Value)
	assert.Equal(t, "alive", *got.Value)

	// Delete again then PURGE (final transition; row removed).
	_, err = c.DeleteSecret(ctx, "doomed", nil)
	require.NoError(t, err)
	_, err = c.PurgeDeletedSecret(ctx, "doomed", nil)
	require.NoError(t, err)

	// GET via /deletedsecrets/{name} now 404s — purged.
	_, err = c.GetDeletedSecret(ctx, "doomed", nil)
	require.Error(t, err, "GetDeletedSecret after Purge must 404")
}
