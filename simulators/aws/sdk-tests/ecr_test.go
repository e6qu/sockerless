package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ecrClient() *ecr.Client {
	return ecr.NewFromConfig(sdkConfig(), func(o *ecr.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestECR_CreateRepository(t *testing.T) {
	client := ecrClient()
	out, err := client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String("test-repo"),
	})
	require.NoError(t, err)
	assert.Equal(t, "test-repo", *out.Repository.RepositoryName)
	assert.Contains(t, *out.Repository.RepositoryUri, "test-repo")
}

func TestECR_DescribeRepositories(t *testing.T) {
	client := ecrClient()

	_, err := client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String("describe-repo"),
	})
	require.NoError(t, err)

	out, err := client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{"describe-repo"},
	})
	require.NoError(t, err)
	require.Len(t, out.Repositories, 1)
	assert.Equal(t, "describe-repo", *out.Repositories[0].RepositoryName)
}

func TestECR_GetAuthorizationToken(t *testing.T) {
	client := ecrClient()
	out, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	require.NoError(t, err)
	require.NotEmpty(t, out.AuthorizationData)
	assert.NotEmpty(t, *out.AuthorizationData[0].AuthorizationToken)
}

func TestECR_LifecyclePolicy(t *testing.T) {
	client := ecrClient()

	_, err := client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String("lifecycle-repo"),
	})
	require.NoError(t, err)

	policy := `{"rules":[{"rulePriority":1,"selection":{"tagStatus":"untagged","countType":"imageCountMoreThan","countNumber":5},"action":{"type":"expire"}}]}`
	_, err = client.PutLifecyclePolicy(ctx, &ecr.PutLifecyclePolicyInput{
		RepositoryName:      aws.String("lifecycle-repo"),
		LifecyclePolicyText: aws.String(policy),
	})
	require.NoError(t, err)

	getOut, err := client.GetLifecyclePolicy(ctx, &ecr.GetLifecyclePolicyInput{
		RepositoryName: aws.String("lifecycle-repo"),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, *getOut.LifecyclePolicyText)
}
