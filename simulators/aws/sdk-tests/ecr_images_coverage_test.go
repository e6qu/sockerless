package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ecrManifest = `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":7,"digest":"sha256:covcfg"},"layers":[]}`

// TestECR_ListAndDescribeAndDeleteImages covers ListImages + DescribeImages
// (both were unregistered) and the BatchDeleteImage alias-delete fix (deleting
// by tag must drop the image's digest alias too).
func TestECR_ListAndDescribeAndDeleteImages(t *testing.T) {
	c := ecrClient()
	_, err := c.CreateRepository(ctx, &ecr.CreateRepositoryInput{RepositoryName: aws.String("cov-images")})
	require.NoError(t, err)
	_, err = c.PutImage(ctx, &ecr.PutImageInput{
		RepositoryName: aws.String("cov-images"), ImageTag: aws.String("v1"), ImageManifest: aws.String(ecrManifest),
	})
	require.NoError(t, err)

	list, err := c.ListImages(ctx, &ecr.ListImagesInput{RepositoryName: aws.String("cov-images")})
	require.NoError(t, err)
	require.Len(t, list.ImageIds, 1)
	assert.Equal(t, "v1", aws.ToString(list.ImageIds[0].ImageTag))
	assert.NotEmpty(t, aws.ToString(list.ImageIds[0].ImageDigest))

	desc, err := c.DescribeImages(ctx, &ecr.DescribeImagesInput{RepositoryName: aws.String("cov-images")})
	require.NoError(t, err)
	require.Len(t, desc.ImageDetails, 1)
	assert.Contains(t, desc.ImageDetails[0].ImageTags, "v1")

	del, err := c.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
		RepositoryName: aws.String("cov-images"),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: aws.String("v1")}},
	})
	require.NoError(t, err)
	// Deleted entries are bare ImageIdentifier objects (imageDigest /
	// imageTag) the SDK reads back directly — the digest is resolved even
	// though the delete was by tag.
	require.Len(t, del.ImageIds, 1)
	assert.Equal(t, "v1", aws.ToString(del.ImageIds[0].ImageTag))
	assert.NotEmpty(t, aws.ToString(del.ImageIds[0].ImageDigest))
	assert.Empty(t, del.Failures)

	// The image (and its digest alias) is gone from both reads.
	descAfter, err := c.DescribeImages(ctx, &ecr.DescribeImagesInput{RepositoryName: aws.String("cov-images")})
	require.NoError(t, err)
	assert.Empty(t, descAfter.ImageDetails, "BatchDeleteImage removes every alias")
	listAfter, err := c.ListImages(ctx, &ecr.ListImagesInput{RepositoryName: aws.String("cov-images")})
	require.NoError(t, err)
	assert.Empty(t, listAfter.ImageIds)
}

// TestECR_LifecyclePolicyLifecycle covers Put/Get/DeleteLifecyclePolicy.
func TestECR_LifecyclePolicyLifecycle(t *testing.T) {
	c := ecrClient()
	_, err := c.CreateRepository(ctx, &ecr.CreateRepositoryInput{RepositoryName: aws.String("cov-lifecycle")})
	require.NoError(t, err)
	const policy = `{"rules":[{"rulePriority":1,"description":"expire untagged","selection":{"tagStatus":"untagged","countType":"imageCountMoreThan","countNumber":5},"action":{"type":"expire"}}]}`
	_, err = c.PutLifecyclePolicy(ctx, &ecr.PutLifecyclePolicyInput{RepositoryName: aws.String("cov-lifecycle"), LifecyclePolicyText: aws.String(policy)})
	require.NoError(t, err)

	got, err := c.GetLifecyclePolicy(ctx, &ecr.GetLifecyclePolicyInput{RepositoryName: aws.String("cov-lifecycle")})
	require.NoError(t, err)
	assert.Equal(t, policy, aws.ToString(got.LifecyclePolicyText))

	_, err = c.DeleteLifecyclePolicy(ctx, &ecr.DeleteLifecyclePolicyInput{RepositoryName: aws.String("cov-lifecycle")})
	require.NoError(t, err)
	_, err = c.GetLifecyclePolicy(ctx, &ecr.GetLifecyclePolicyInput{RepositoryName: aws.String("cov-lifecycle")})
	assert.Error(t, err, "lifecycle policy gone after delete")
}
