package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	types "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func smClient() *secretsmanager.Client {
	return secretsmanager.NewFromConfig(sdkConfig(), func(o *secretsmanager.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

// TestSecretsManager_CreateGetUpdateDelete pins the canonical runner
// secret-fetching flow: workflow creates a secret, the runner fetches
// it, the workflow rotates the value, then deletes it on cleanup.
func TestSecretsManager_CreateGetUpdateDelete(t *testing.T) {
	c := smClient()

	createOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String("runner/db-password"),
		Description:  aws.String("Runner DB credential"),
		SecretString: aws.String("hunter2"),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.ARN)
	assert.Contains(t, *createOut.ARN, "arn:aws:secretsmanager:")
	assert.Contains(t, *createOut.ARN, ":secret:runner/db-password-")

	// Fetch by name (most common in CI workflows).
	getByName, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("runner/db-password"),
	})
	require.NoError(t, err)
	assert.Equal(t, "hunter2", *getByName.SecretString)
	assert.NotEmpty(t, *getByName.VersionId)

	// Real Secrets Manager accepts the full ARN as SecretId; sim must too.
	getByArn, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: createOut.ARN,
	})
	require.NoError(t, err)
	assert.Equal(t, "hunter2", *getByArn.SecretString)

	// Rotate the value.
	updateOut, err := c.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String("runner/db-password"),
		SecretString: aws.String("rotated-secret"),
	})
	require.NoError(t, err)
	require.NotNil(t, updateOut.VersionId)
	assert.NotEqual(t, *createOut.VersionId, *updateOut.VersionId, "VersionId must change on rotation")

	rotated, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("runner/db-password"),
	})
	require.NoError(t, err)
	assert.Equal(t, "rotated-secret", *rotated.SecretString)

	// Describe should round-trip metadata.
	desc, err := c.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String("runner/db-password"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Runner DB credential", *desc.Description)
	require.Contains(t, desc.VersionIdsToStages, *rotated.VersionId)
	assert.Contains(t, desc.VersionIdsToStages[*rotated.VersionId], "AWSCURRENT")

	// Cleanup.
	_, err = c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String("runner/db-password"),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	require.NoError(t, err)

	_, err = c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("runner/db-password"),
	})
	require.Error(t, err, "GetSecretValue after delete must 404")
}

func TestSecretsManager_DuplicateNameRejected(t *testing.T) {
	c := smClient()
	defer c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String("dup-name"),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	_, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String("dup-name"),
		SecretString: aws.String("first"),
	})
	require.NoError(t, err)
	_, err = c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String("dup-name"),
		SecretString: aws.String("second"),
	})
	require.Error(t, err, "second create must fail like real AWS (ResourceExistsException)")
}

func TestSecretsManager_ListAndTag(t *testing.T) {
	c := smClient()

	defer c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String("list-test"),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	_, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String("list-test"),
		SecretString: aws.String("v1"),
	})
	require.NoError(t, err)

	listOut, err := c.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
	require.NoError(t, err)
	found := false
	for _, s := range listOut.SecretList {
		if s.Name != nil && *s.Name == "list-test" {
			found = true
			break
		}
	}
	assert.True(t, found, "ListSecrets must include the just-created secret")

	// PutSecretValue rotates without changing other metadata.
	put, err := c.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String("list-test"),
		SecretString: aws.String("v2"),
	})
	require.NoError(t, err)
	require.NotNil(t, put.VersionId)

	// TagResource + UntagResource round-trip.
	_, err = c.TagResource(ctx, &secretsmanager.TagResourceInput{
		SecretId: aws.String("list-test"),
		Tags: []types.Tag{
			{Key: aws.String("env"), Value: aws.String("prod")},
		},
	})
	require.NoError(t, err)

	desc, err := c.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String("list-test"),
	})
	require.NoError(t, err)
	require.Len(t, desc.Tags, 1)
	assert.Equal(t, "env", *desc.Tags[0].Key)

	_, err = c.UntagResource(ctx, &secretsmanager.UntagResourceInput{
		SecretId: aws.String("list-test"),
		TagKeys:  []string{"env"},
	})
	require.NoError(t, err)
}
