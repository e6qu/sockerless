package aws_sdk_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3_Multipart_ListParts locks the BUG-1148 (issue #196 reopen)
// regression guard: the `GET /{bucket}/{key}?uploadId={id}` route
// must return a ListPartsResult, not fall through to GetObject and
// 404 NoSuchKey. aws-sdk-go-v2's `manager.Uploader` calls ListParts
// on every retry path; without this handler, the upload retry loop
// either re-sends every part (wasted work) or stalls.
func TestS3_Multipart_ListParts(t *testing.T) {
	c := s3Client()
	ctx := context.Background()
	bucket := "lp-bucket"
	key := "lp-key"
	s3CreateBucket(t, c, bucket)

	initOut, err := c.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	require.NoError(t, err)
	uploadID := aws.ToString(initOut.UploadId)

	// Upload 3 parts.
	parts := []string{"alpha ", "beta ", "gamma"}
	for i, body := range parts {
		_, err := c.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(bucket),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadID),
			PartNumber: aws.Int32(int32(i + 1)),
			Body:       bytes.NewReader([]byte(body)),
		})
		require.NoError(t, err)
	}

	// ListParts must return all 3 parts in order with non-empty ETags.
	listOut, err := c.ListParts(ctx, &s3.ListPartsInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	require.NoError(t, err, "ListParts must succeed (BUG-1148 regression guard)")
	require.Len(t, listOut.Parts, 3,
		"ListParts must return one entry per uploaded part")
	for i, p := range listOut.Parts {
		assert.Equal(t, int32(i+1), aws.ToInt32(p.PartNumber))
		etag := aws.ToString(p.ETag)
		assert.True(t, strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`),
			"ETag must be canonical quoted form; got %q", etag)
		assert.Equal(t, int64(len(parts[i])), aws.ToInt64(p.Size))
	}

	_, err = c.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	require.NoError(t, err)
}
