package aws_sdk_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func s3Client() *s3.Client {
	return s3.NewFromConfig(sdkConfig(), func(o *s3.Options) {
		o.BaseEndpoint = aws.String(baseURL + "/s3")
		o.UsePathStyle = true
	})
}

func TestS3_CreateBucket(t *testing.T) {
	client := s3Client()
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("test-bucket"),
	})
	require.NoError(t, err)

	out, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String("test-bucket"),
	})
	require.NoError(t, err)
	assert.NotNil(t, out)
}

func TestS3_PutAndGetObject(t *testing.T) {
	client := s3Client()

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("obj-bucket"),
	})
	require.NoError(t, err)

	body := []byte("hello world")
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("obj-bucket"),
		Key:    aws.String("greeting.txt"),
		Body:   bytes.NewReader(body),
	})
	require.NoError(t, err)

	getOut, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String("obj-bucket"),
		Key:    aws.String("greeting.txt"),
	})
	require.NoError(t, err)
	defer getOut.Body.Close()

	data, err := io.ReadAll(getOut.Body)
	require.NoError(t, err)
	assert.Equal(t, body, data)
}

func TestS3_ListObjects(t *testing.T) {
	client := s3Client()

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("list-bucket"),
	})
	require.NoError(t, err)

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("list-bucket"),
		Key:    aws.String("file1.txt"),
		Body:   bytes.NewReader([]byte("one")),
	})
	require.NoError(t, err)

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("list-bucket"),
		Key:    aws.String("file2.txt"),
		Body:   bytes.NewReader([]byte("two")),
	})
	require.NoError(t, err)

	out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String("list-bucket"),
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(out.Contents), 2)
}

func TestS3_DeleteObject(t *testing.T) {
	client := s3Client()

	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String("del-bucket"),
	})
	require.NoError(t, err)

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("del-bucket"),
		Key:    aws.String("to-delete.txt"),
		Body:   bytes.NewReader([]byte("bye")),
	})
	require.NoError(t, err)

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String("del-bucket"),
		Key:    aws.String("to-delete.txt"),
	})
	require.NoError(t, err)
}
