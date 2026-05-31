package aws_cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKMSCLI_KeyAliasAndCrypto(t *testing.T) {
	createOut := runCLI(t, awsCLI("kms", "create-key",
		"--description", "cli-kms-key",
		"--output", "json"))
	var createResult struct {
		KeyMetadata struct {
			KeyId    string `json:"KeyId"`
			Arn      string `json:"Arn"`
			KeyState string `json:"KeyState"`
		} `json:"KeyMetadata"`
	}
	parseJSON(t, createOut, &createResult)
	require.NotEmpty(t, createResult.KeyMetadata.KeyId)
	require.Contains(t, createResult.KeyMetadata.Arn, ":key/")
	require.Equal(t, "Enabled", createResult.KeyMetadata.KeyState)

	aliasName := "alias/cli-kms-coverage"
	runCLI(t, awsCLI("kms", "create-alias",
		"--alias-name", aliasName,
		"--target-key-id", createResult.KeyMetadata.KeyId))
	t.Cleanup(func() {
		_ = awsCLI("kms", "delete-alias", "--alias-name", aliasName).Run()
		_ = awsCLI("kms", "schedule-key-deletion",
			"--key-id", createResult.KeyMetadata.KeyId,
			"--pending-window-in-days", "7").Run()
	})

	encryptOut := runCLI(t, awsCLI("kms", "encrypt",
		"--key-id", aliasName,
		"--plaintext", "cli-secret",
		"--cli-binary-format", "raw-in-base64-out",
		"--output", "json"))
	var encryptResult struct {
		CiphertextBlob string `json:"CiphertextBlob"`
		KeyId          string `json:"KeyId"`
	}
	parseJSON(t, encryptOut, &encryptResult)
	require.NotEmpty(t, encryptResult.CiphertextBlob)
	require.Contains(t, encryptResult.KeyId, createResult.KeyMetadata.KeyId)

	decryptOut := runCLI(t, awsCLI("kms", "decrypt",
		"--ciphertext-blob", encryptResult.CiphertextBlob,
		"--output", "json"))
	var decryptResult struct {
		Plaintext string `json:"Plaintext"`
		KeyId     string `json:"KeyId"`
	}
	parseJSON(t, decryptOut, &decryptResult)
	require.Equal(t, "Y2xpLXNlY3JldA==", decryptResult.Plaintext)
	require.Contains(t, decryptResult.KeyId, createResult.KeyMetadata.KeyId)
}
