package aws_sdk_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3_Bucket_Versioning_RoundTrip locks the invariant: PUT
// `/{bucket}?versioning` must route to the bucket-subresource
// dispatcher and persist the configuration, GET must return the
// same configuration back. A regression that re-routes the PUT
// to CreateBucket would surface as 409 BucketAlreadyOwnedByYou
// and fail this test.
func TestS3_Bucket_Versioning_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "versioning-bucket"
	s3CreateBucket(t, c, bucket)

	_, err := c.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: types.BucketVersioningStatusEnabled,
		},
	})
	require.NoError(t, err, "PutBucketVersioning must succeed (subresource dispatcher routes the PUT, not CreateBucket)")

	get, err := c.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	assert.Equal(t, types.BucketVersioningStatusEnabled, get.Status,
		"GetBucketVersioning must return the previously-PUT Enabled status")
}

// TestS3_Bucket_Lifecycle_RoundTrip exercises Put → Get → Delete on
// the bucket lifecycle subresource — the lifecycle config has both
// PUT and DELETE shapes; both must be wired (not just PUT).
func TestS3_Bucket_Lifecycle_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "lifecycle-bucket"
	s3CreateBucket(t, c, bucket)

	// Initial Get → canonical empty-config 404.
	_, err := c.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(bucket)})
	require.Error(t, err, "initial GetBucketLifecycleConfiguration must 404")
	var apiErr smithy.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "NoSuchLifecycleConfiguration", apiErr.ErrorCode())

	// PUT a rule.
	_, err = c.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{
			Rules: []types.LifecycleRule{
				{
					ID:     aws.String("expire-30d"),
					Status: types.ExpirationStatusEnabled,
					Filter: &types.LifecycleRuleFilter{Prefix: aws.String("")},
					Expiration: &types.LifecycleExpiration{
						Days: aws.Int32(30),
					},
				},
			},
		},
	})
	require.NoError(t, err, "PutBucketLifecycleConfiguration must succeed")

	// Get round-trips.
	get, err := c.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(bucket)})
	require.NoError(t, err, "GetBucketLifecycleConfiguration must return the PUT config")
	require.Len(t, get.Rules, 1)
	assert.Equal(t, "expire-30d", aws.ToString(get.Rules[0].ID))

	// Delete clears the config.
	_, err = c.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)

	// Subsequent Get is 404 again.
	_, err = c.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(bucket)})
	require.Error(t, err, "post-delete GetBucketLifecycleConfiguration must 404")
}

// TestS3_Bucket_Cors_RoundTrip covers the bucket CORS subresource.
func TestS3_Bucket_Cors_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "cors-bucket"
	s3CreateBucket(t, c, bucket)

	_, err := c.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(bucket),
		CORSConfiguration: &types.CORSConfiguration{
			CORSRules: []types.CORSRule{
				{
					AllowedOrigins: []string{"https://app.example.com"},
					AllowedMethods: []string{"GET", "PUT"},
					AllowedHeaders: []string{"*"},
				},
			},
		},
	})
	require.NoError(t, err, "PutBucketCors must succeed")

	get, err := c.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	require.Len(t, get.CORSRules, 1)
	assert.Contains(t, get.CORSRules[0].AllowedOrigins, "https://app.example.com")

	_, err = c.DeleteBucketCors(ctx, &s3.DeleteBucketCorsInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}

// TestS3_Bucket_Policy_RoundTrip covers the bucket policy
// subresource — PUT body is a JSON IAM policy document (not XML);
// the dispatcher must accept either content type.
func TestS3_Bucket_Policy_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "policy-bucket"
	s3CreateBucket(t, c, bucket)

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::policy-bucket/*"}]}`
	_, err := c.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(policy),
	})
	require.NoError(t, err, "PutBucketPolicy must succeed")

	get, err := c.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	assert.Equal(t, policy, aws.ToString(get.Policy))

	_, err = c.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}

// TestS3_Bucket_Encryption_RoundTrip covers
// PutBucketEncryption / GetBucketEncryption / DeleteBucketEncryption.
func TestS3_Bucket_Encryption_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "encryption-bucket"
	s3CreateBucket(t, c, bucket)

	_, err := c.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucket),
		ServerSideEncryptionConfiguration: &types.ServerSideEncryptionConfiguration{
			Rules: []types.ServerSideEncryptionRule{
				{
					ApplyServerSideEncryptionByDefault: &types.ServerSideEncryptionByDefault{
						SSEAlgorithm: types.ServerSideEncryptionAes256,
					},
				},
			},
		},
	})
	require.NoError(t, err, "PutBucketEncryption must succeed")

	get, err := c.GetBucketEncryption(ctx, &s3.GetBucketEncryptionInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	require.Len(t, get.ServerSideEncryptionConfiguration.Rules, 1)
	assert.Equal(t, types.ServerSideEncryptionAes256,
		get.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm)

	_, err = c.DeleteBucketEncryption(ctx, &s3.DeleteBucketEncryptionInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}

// TestS3_Bucket_Tagging_RoundTrip covers the bucket-level tagging
// subresource. Distinct from object tagging (which already worked).
func TestS3_Bucket_Tagging_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "btagging-bucket"
	s3CreateBucket(t, c, bucket)

	_, err := c.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucket),
		Tagging: &types.Tagging{
			TagSet: []types.Tag{
				{Key: aws.String("env"), Value: aws.String("prod")},
				{Key: aws.String("team"), Value: aws.String("platform")},
			},
		},
	})
	require.NoError(t, err, "PutBucketTagging must succeed")

	get, err := c.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	require.Len(t, get.TagSet, 2)

	_, err = c.DeleteBucketTagging(ctx, &s3.DeleteBucketTaggingInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}

// TestS3_Bucket_Website_RoundTrip covers the bucket-website
// subresource — locks in the per-subresource success-status fidelity
// (Website is 200 OK, distinct from Policy's 204 No Content).
func TestS3_Bucket_Website_RoundTrip(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "website-bucket"
	s3CreateBucket(t, c, bucket)

	_, err := c.PutBucketWebsite(ctx, &s3.PutBucketWebsiteInput{
		Bucket: aws.String(bucket),
		WebsiteConfiguration: &types.WebsiteConfiguration{
			IndexDocument: &types.IndexDocument{Suffix: aws.String("index.html")},
		},
	})
	require.NoError(t, err, "PutBucketWebsite must succeed")

	get, err := c.GetBucketWebsite(ctx, &s3.GetBucketWebsiteInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	require.NotNil(t, get.IndexDocument)
	assert.Equal(t, "index.html", aws.ToString(get.IndexDocument.Suffix))

	_, err = c.DeleteBucketWebsite(ctx, &s3.DeleteBucketWebsiteInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
}
