package gcp_sdk_test

import (
	"encoding/base64"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/cloudkms/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

var kmsCRCTable = crc32.MakeTable(crc32.Castagnoli)

func kmsService(t *testing.T) *cloudkms.Service {
	t.Helper()
	svc, err := cloudkms.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func requireKMSErrorCode(t *testing.T, err error, code int) {
	t.Helper()
	require.Error(t, err)
	var gerr *googleapi.Error
	require.ErrorAs(t, err, &gerr)
	require.Equal(t, code, gerr.Code)
}

func TestCloudKMSKeyRingLifecycleSDK(t *testing.T) {
	svc := kmsService(t)
	parent := "projects/sdk-kms-project/locations/global"
	ringID := "sdk-ring-lifecycle"
	ringName := parent + "/keyRings/" + ringID

	created, err := svc.Projects.Locations.KeyRings.Create(parent, &cloudkms.KeyRing{}).KeyRingId(ringID).Do()
	require.NoError(t, err)
	require.Equal(t, ringName, created.Name)
	require.NotEmpty(t, created.CreateTime)

	// Re-create the same ring → ALREADY_EXISTS.
	_, err = svc.Projects.Locations.KeyRings.Create(parent, &cloudkms.KeyRing{}).KeyRingId(ringID).Do()
	requireKMSErrorCode(t, err, 409)

	got, err := svc.Projects.Locations.KeyRings.Get(ringName).Do()
	require.NoError(t, err)
	require.Equal(t, ringName, got.Name)

	// GET on an uncreated ring must 404, not 200.
	_, err = svc.Projects.Locations.KeyRings.Get(parent + "/keyRings/never-created").Do()
	requireKMSErrorCode(t, err, 404)

	list, err := svc.Projects.Locations.KeyRings.List(parent).Do()
	require.NoError(t, err)
	found := false
	for _, kr := range list.KeyRings {
		if kr.Name == ringName {
			found = true
		}
	}
	require.True(t, found)
}

func TestCloudKMSEncryptDecryptSDK(t *testing.T) {
	svc := kmsService(t)
	parent := "projects/sdk-kms-project/locations/global"
	ringID := "sdk-ring-crypto"
	ringName := parent + "/keyRings/" + ringID
	_, err := svc.Projects.Locations.KeyRings.Create(parent, &cloudkms.KeyRing{}).KeyRingId(ringID).Do()
	require.NoError(t, err)

	keyID := "sdk-key"
	keyName := ringName + "/cryptoKeys/" + keyID
	key, err := svc.Projects.Locations.KeyRings.CryptoKeys.Create(ringName, &cloudkms.CryptoKey{
		Purpose: "ENCRYPT_DECRYPT",
	}).CryptoKeyId(keyID).Do()
	require.NoError(t, err)
	require.Equal(t, keyName, key.Name)
	require.NotNil(t, key.Primary, "ENCRYPT_DECRYPT key must auto-create a primary version")
	require.Equal(t, "ENABLED", key.Primary.State)
	require.Equal(t, "GOOGLE_SYMMETRIC_ENCRYPTION", key.Primary.Algorithm)

	// Creating a key in a non-existent ring must 404.
	_, err = svc.Projects.Locations.KeyRings.CryptoKeys.Create(parent+"/keyRings/missing", &cloudkms.CryptoKey{
		Purpose: "ENCRYPT_DECRYPT",
	}).CryptoKeyId("x").Do()
	requireKMSErrorCode(t, err, 404)

	plaintext := []byte("super-secret-payload-\x00\x01\x02")
	ptB64 := base64.StdEncoding.EncodeToString(plaintext)
	encResp, err := svc.Projects.Locations.KeyRings.CryptoKeys.Encrypt(keyName, &cloudkms.EncryptRequest{
		Plaintext:       ptB64,
		PlaintextCrc32c: int64(crc32.Checksum(plaintext, kmsCRCTable)),
	}).Do()
	require.NoError(t, err)
	require.True(t, encResp.VerifiedPlaintextCrc32c, "server must verify the supplied plaintext CRC32C")
	require.NotEmpty(t, encResp.Ciphertext)
	require.NotZero(t, encResp.CiphertextCrc32c, "encrypt response must carry ciphertextCrc32c")
	require.Equal(t, keyName+"/cryptoKeyVersions/1", encResp.Name)

	// The ciphertext must be opaque (not contain the plaintext).
	require.NotContains(t, encResp.Ciphertext, ptB64)

	decResp, err := svc.Projects.Locations.KeyRings.CryptoKeys.Decrypt(keyName, &cloudkms.DecryptRequest{
		Ciphertext: encResp.Ciphertext,
	}).Do()
	require.NoError(t, err)
	require.NotZero(t, decResp.PlaintextCrc32c, "decrypt response must carry plaintextCrc32c")
	gotPlain, err := base64.StdEncoding.DecodeString(decResp.Plaintext)
	require.NoError(t, err)
	require.Equal(t, plaintext, gotPlain)

	// Additional authenticated data must round-trip and be enforced.
	aad := []byte("context-binding")
	aadB64 := base64.StdEncoding.EncodeToString(aad)
	encAAD, err := svc.Projects.Locations.KeyRings.CryptoKeys.Encrypt(keyName, &cloudkms.EncryptRequest{
		Plaintext:                   ptB64,
		AdditionalAuthenticatedData: aadB64,
	}).Do()
	require.NoError(t, err)
	// Decrypt with matching AAD succeeds.
	_, err = svc.Projects.Locations.KeyRings.CryptoKeys.Decrypt(keyName, &cloudkms.DecryptRequest{
		Ciphertext:                  encAAD.Ciphertext,
		AdditionalAuthenticatedData: aadB64,
	}).Do()
	require.NoError(t, err)
	// Decrypt with the wrong AAD fails the integrity check.
	_, err = svc.Projects.Locations.KeyRings.CryptoKeys.Decrypt(keyName, &cloudkms.DecryptRequest{
		Ciphertext:                  encAAD.Ciphertext,
		AdditionalAuthenticatedData: base64.StdEncoding.EncodeToString([]byte("wrong-context")),
	}).Do()
	require.Error(t, err)

	// A malformed ciphertext is rejected.
	_, err = svc.Projects.Locations.KeyRings.CryptoKeys.Decrypt(keyName, &cloudkms.DecryptRequest{
		Ciphertext: base64.StdEncoding.EncodeToString([]byte("garbage")),
	}).Do()
	require.Error(t, err)
}

func TestCloudKMSCryptoKeyManagementSDK(t *testing.T) {
	svc := kmsService(t)
	parent := "projects/sdk-kms-project/locations/global"
	ringID := "sdk-ring-mgmt"
	ringName := parent + "/keyRings/" + ringID
	_, err := svc.Projects.Locations.KeyRings.Create(parent, &cloudkms.KeyRing{}).KeyRingId(ringID).Do()
	require.NoError(t, err)

	keyName := ringName + "/cryptoKeys/mgmt-key"
	_, err = svc.Projects.Locations.KeyRings.CryptoKeys.Create(ringName, &cloudkms.CryptoKey{
		Purpose: "ENCRYPT_DECRYPT",
	}).CryptoKeyId("mgmt-key").Do()
	require.NoError(t, err)

	keys, err := svc.Projects.Locations.KeyRings.CryptoKeys.List(ringName).Do()
	require.NoError(t, err)
	require.Len(t, keys.CryptoKeys, 1)
	require.Equal(t, keyName, keys.CryptoKeys[0].Name)

	versions, err := svc.Projects.Locations.KeyRings.CryptoKeys.CryptoKeyVersions.List(keyName).Do()
	require.NoError(t, err)
	require.Len(t, versions.CryptoKeyVersions, 1)
	v1Name := keyName + "/cryptoKeyVersions/1"
	require.Equal(t, v1Name, versions.CryptoKeyVersions[0].Name)

	gotVer, err := svc.Projects.Locations.KeyRings.CryptoKeys.CryptoKeyVersions.Get(v1Name).Do()
	require.NoError(t, err)
	require.Equal(t, "ENABLED", gotVer.State)

	// Patch rotation period via update mask.
	patched, err := svc.Projects.Locations.KeyRings.CryptoKeys.Patch(keyName, &cloudkms.CryptoKey{
		RotationPeriod: "604800s",
	}).UpdateMask("rotationPeriod").Do()
	require.NoError(t, err)
	require.Equal(t, "604800s", patched.RotationPeriod)

	// Destroy the version → DESTROY_SCHEDULED with a real destroyTime.
	destroyed, err := svc.Projects.Locations.KeyRings.CryptoKeys.CryptoKeyVersions.Destroy(v1Name, &cloudkms.DestroyCryptoKeyVersionRequest{}).Do()
	require.NoError(t, err)
	require.Equal(t, "DESTROY_SCHEDULED", destroyed.State)
	require.NotEmpty(t, destroyed.DestroyTime)

	// Encrypt now fails because the primary version is no longer enabled.
	_, err = svc.Projects.Locations.KeyRings.CryptoKeys.Encrypt(keyName, &cloudkms.EncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString([]byte("x")),
	}).Do()
	require.Error(t, err)
}
