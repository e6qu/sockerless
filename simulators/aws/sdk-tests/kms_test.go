package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kmsClient() *kms.Client {
	return kms.NewFromConfig(sdkConfig(), func(o *kms.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestKMS_CreateEncryptDecryptCycle pins the canonical envelope flow:
// CreateKey → Encrypt → Decrypt round-trips plaintext bytes through the
// sim's structured ciphertext envelope. Real KMS uses HSM-protected key
// material; the sim's envelope is opaque to SDK callers but reversible
// — same wire shape.
func TestKMS_CreateEncryptDecryptCycle(t *testing.T) {
	c := kmsClient()

	createOut, err := c.CreateKey(ctx, &kms.CreateKeyInput{
		Description: aws.String("runner-secret-key"),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.KeyMetadata)
	keyId := *createOut.KeyMetadata.KeyId
	assert.Contains(t, *createOut.KeyMetadata.Arn, "arn:aws:kms:")
	assert.Equal(t, "Enabled", string(createOut.KeyMetadata.KeyState))

	plaintext := []byte("hunter2")
	encOut, err := c.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(keyId),
		Plaintext: plaintext,
	})
	require.NoError(t, err)
	require.NotEmpty(t, encOut.CiphertextBlob, "Encrypt must return a non-empty ciphertext blob")

	decOut, err := c.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: encOut.CiphertextBlob,
	})
	require.NoError(t, err)
	assert.Equal(t, plaintext, decOut.Plaintext, "Decrypted plaintext must round-trip exactly")
	assert.Contains(t, *decOut.KeyId, keyId, "Decrypt KeyId must reference the encrypting key")
}

// TestKMS_AliasResolution covers the alias path callers commonly use:
// CreateAlias → Encrypt(KeyId=alias/...) → ListAliases → DeleteAlias.
func TestKMS_AliasResolution(t *testing.T) {
	c := kmsClient()

	createOut, err := c.CreateKey(ctx, &kms.CreateKeyInput{Description: aws.String("alias-target")})
	require.NoError(t, err)
	keyId := *createOut.KeyMetadata.KeyId

	_, err = c.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   aws.String("alias/runner-cache"),
		TargetKeyId: aws.String(keyId),
	})
	require.NoError(t, err)
	defer c.DeleteAlias(ctx, &kms.DeleteAliasInput{AliasName: aws.String("alias/runner-cache")})

	// Encrypt addressed by alias should resolve to the target key.
	encOut, err := c.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String("alias/runner-cache"),
		Plaintext: []byte("test"),
	})
	require.NoError(t, err)
	assert.Contains(t, *encOut.KeyId, keyId)

	// Duplicate alias must 400.
	_, err = c.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   aws.String("alias/runner-cache"),
		TargetKeyId: aws.String(keyId),
	})
	require.Error(t, err, "duplicate alias must fail like real KMS (AlreadyExistsException)")

	listOut, err := c.ListAliases(ctx, &kms.ListAliasesInput{})
	require.NoError(t, err)
	found := false
	for _, a := range listOut.Aliases {
		if a.AliasName != nil && *a.AliasName == "alias/runner-cache" {
			found = true
			break
		}
	}
	assert.True(t, found, "ListAliases must include the just-created alias")
}

// TestKMS_GenerateDataKey covers envelope encryption — caller gets both
// the plaintext data key (to use locally) and a wrapped ciphertext (to
// store alongside the encrypted payload).
func TestKMS_GenerateDataKey(t *testing.T) {
	c := kmsClient()

	createOut, err := c.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyId := *createOut.KeyMetadata.KeyId

	out, err := c.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:         aws.String(keyId),
		NumberOfBytes: aws.Int32(32),
	})
	require.NoError(t, err)
	assert.Len(t, out.Plaintext, 32, "Plaintext must match requested NumberOfBytes")
	assert.NotEmpty(t, out.CiphertextBlob, "CiphertextBlob must be present")

	// Round-trip the wrapped data key.
	decOut, err := c.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: out.CiphertextBlob,
	})
	require.NoError(t, err)
	assert.Equal(t, out.Plaintext, decOut.Plaintext)
}

func TestKMS_DescribeAndScheduleDeletion(t *testing.T) {
	c := kmsClient()
	createOut, err := c.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyId := *createOut.KeyMetadata.KeyId

	desc, err := c.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(keyId)})
	require.NoError(t, err)
	assert.Equal(t, keyId, *desc.KeyMetadata.KeyId)

	listOut, err := c.ListKeys(ctx, &kms.ListKeysInput{})
	require.NoError(t, err)
	found := false
	for _, k := range listOut.Keys {
		if k.KeyId != nil && *k.KeyId == keyId {
			found = true
			break
		}
	}
	assert.True(t, found, "ListKeys must include the new key")

	delOut, err := c.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(keyId),
		PendingWindowInDays: aws.Int32(7),
	})
	require.NoError(t, err)
	assert.Equal(t, "PendingDeletion", string(delOut.KeyState))
}
