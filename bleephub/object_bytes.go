package bleephub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type actionsByteStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

type s3ActionsByteStore struct {
	fs *s3FS
}

func newActionsByteStoreFromEnv(ctx context.Context) (actionsByteStore, error) {
	bucket := os.Getenv("BLEEPHUB_OBJECT_S3_BUCKET")
	if bucket == "" {
		return nil, nil
	}
	endpoint := os.Getenv("BLEEPHUB_OBJECT_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("BLEEPHUB_S3_ENDPOINT")
	}
	prefix := os.Getenv("BLEEPHUB_OBJECT_S3_PREFIX")
	if prefix == "" {
		prefix = "objects"
	}
	fs, err := newS3FS(ctx, endpoint, bucket, prefix)
	if err != nil {
		return nil, err
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := fs.client.HeadBucket(verifyCtx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
		return nil, fmt.Errorf("s3 head bucket %s: %w", bucket, err)
	}
	return &s3ActionsByteStore{fs: fs}, nil
}

func (s *s3ActionsByteStore) Put(ctx context.Context, key string, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := s.fs.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.fs.bucket),
		Key:    aws.String(s.key(key)),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", s.key(key), err)
	}
	return nil
}

func (s *s3ActionsByteStore) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := s.fs.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.fs.bucket),
		Key:    aws.String(s.key(key)),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", s.key(key), err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read %s: %w", s.key(key), err)
	}
	return data, nil
}

func (s *s3ActionsByteStore) Delete(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := s.fs.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.fs.bucket),
		Key:    aws.String(s.key(key)),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", s.key(key), err)
	}
	return nil
}

func (s *s3ActionsByteStore) key(key string) string {
	return path.Join(s.fs.prefix, strings.TrimPrefix(key, "/"))
}

func artifactDataKey(id int64) string {
	return fmt.Sprintf("actions/artifacts/%d/data", id)
}

func cacheDataKey(id int64) string {
	return fmt.Sprintf("actions/caches/%d/data", id)
}

func logDataKey(id int) string {
	return fmt.Sprintf("actions/logs/%d/data", id)
}

func releaseAssetDataKey(id int) string {
	return fmt.Sprintf("releases/assets/%d/data", id)
}

func packageFileDataKey(fileID int) string {
	return fmt.Sprintf("packages/files/%d/data", fileID)
}

func packageRegistryBlobDataKey(digest string) string {
	algo, hexPart, ok := strings.Cut(digest, ":")
	if !ok {
		return path.Join("packages/registry/blobs", digest)
	}
	return path.Join("packages/registry/blobs", algo, hexPart)
}

func codeQLDatabaseDataKey(id int, content []byte) string {
	digest := sha256.Sum256(content)
	return fmt.Sprintf("code-scanning/codeql/databases/%d/%x.zip", id, digest)
}

func codeQLVariantAnalysisQueryPackDataKey(id int) string {
	return fmt.Sprintf("code-scanning/codeql/variant-analyses/%d/query-pack.tar.gz", id)
}

func attestationBundleDataKey(id int) string {
	return fmt.Sprintf("attestations/%d/bundle.json", id)
}
