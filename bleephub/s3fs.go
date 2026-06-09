package bleephub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
)

type s3FS struct {
	client *s3.Client
	bucket string
	prefix string
}

func newS3FS(ctx context.Context, endpoint, bucket, prefix string) (*s3FS, error) {
	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion("us-east-1"))
	if endpoint != "" {
		opts = append(opts, awsconfig.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint}, nil
			}),
		))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("s3 config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}
	client := s3.NewFromConfig(cfg, clientOpts...)

	return &s3FS{client: client, bucket: bucket, prefix: prefix}, nil
}

func (f *s3FS) key(p string) string {
	return path.Join(f.prefix, p)
}

func (f *s3FS) Create(filename string) (billy.File, error) {
	return &s3File{
		fs:     f,
		name:   filename,
		buf:    &bytes.Buffer{},
		closed: false,
	}, nil
}

func (f *s3FS) Open(filename string) (billy.File, error) {
	key := f.key(filename)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := f.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("s3 read %s: %w", key, err)
	}

	return &s3File{
		fs:     f,
		name:   filename,
		buf:    bytes.NewBuffer(data),
		closed: false,
	}, nil
}

func (f *s3FS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	switch {
	case flag&(os.O_CREATE|os.O_WRONLY) != 0 && flag&os.O_RDONLY == 0:
		return f.Create(filename)
	case flag&os.O_TRUNC != 0:
		return f.Create(filename)
	case flag&(os.O_RDWR|os.O_WRONLY|os.O_RDONLY) != 0:
		ff, err := f.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && flag&os.O_CREATE != 0 {
				return f.Create(filename)
			}
			return nil, err
		}
		return ff, nil
	default:
		return f.Open(filename)
	}
}

func (f *s3FS) Stat(filename string) (os.FileInfo, error) {
	key := f.key(filename)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := f.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, os.ErrNotExist
		}
		var nfe *s3types.NotFound
		if errors.As(err, &nfe) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("s3 head %s: %w", key, err)
	}

	return &s3FileInfo{
		name:    path.Base(filename),
		size:    aws.ToInt64(resp.ContentLength),
		mode:    0o644,
		modTime: aws.ToTime(resp.LastModified),
		isDir:   false,
	}, nil
}

func (f *s3FS) Rename(oldpath, newpath string) error {
	srcKey := f.key(oldpath)
	dstKey := f.key(newpath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := f.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(f.bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(f.bucket + "/" + srcKey),
	})
	if err != nil {
		return fmt.Errorf("s3 copy %s -> %s: %w", srcKey, dstKey, err)
	}

	_, err = f.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(srcKey),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s after copy: %w", srcKey, err)
	}

	return nil
}

func (f *s3FS) Remove(filename string) error {
	key := f.key(filename)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := f.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(f.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

func (f *s3FS) Join(elem ...string) string {
	return path.Join(elem...)
}

func (f *s3FS) TempFile(dir, prefix string) (billy.File, error) {
	name := path.Join(dir, prefix+uuid.New().String())
	return f.Create(name)
}

func (f *s3FS) ReadDir(dirname string) ([]os.FileInfo, error) {
	prefix := f.key(dirname)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := f.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(f.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 list %s: %w", prefix, err)
	}

	var entries []os.FileInfo
	baseLen := len(f.prefix)
	if f.prefix != "" {
		baseLen++
	}

	for _, obj := range resp.Contents {
		key := aws.ToString(obj.Key)
		if len(key) <= baseLen {
			continue
		}
		relKey := key[baseLen:]
		entries = append(entries, &s3FileInfo{
			name:    path.Base(relKey),
			size:    aws.ToInt64(obj.Size),
			mode:    0o644,
			modTime: aws.ToTime(obj.LastModified),
			isDir:   false,
		})
	}

	for _, cp := range resp.CommonPrefixes {
		p := aws.ToString(cp.Prefix)
		if len(p) <= baseLen {
			continue
		}
		relKey := p[baseLen:]
		entries = append(entries, &s3FileInfo{
			name:    path.Base(relKey),
			size:    0,
			mode:    0o755 | os.ModeDir,
			modTime: time.Time{},
			isDir:   true,
		})
	}

	slices.SortFunc(entries, func(a, b os.FileInfo) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name(), b.Name())
	})

	return entries, nil
}

func (f *s3FS) MkdirAll(filename string, perm os.FileMode) error {
	return nil
}

func (f *s3FS) Lstat(filename string) (os.FileInfo, error) {
	return f.Stat(filename)
}

func (f *s3FS) Symlink(target, link string) error {
	return billy.ErrNotSupported
}

func (f *s3FS) Readlink(link string) (string, error) {
	return "", billy.ErrNotSupported
}

func (f *s3FS) Chroot(path string) (billy.Filesystem, error) {
	return &s3FS{
		client: f.client,
		bucket: f.bucket,
		prefix: f.key(path),
	}, nil
}

func (f *s3FS) Root() string {
	return f.prefix
}

func (f *s3FS) deleteRepoPrefix(fullName string) {
	prefix := f.key(fullName) + "/"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var continuation *string
	for {
		resp, err := f.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(f.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuation,
		})
		if err != nil {
			return
		}
		for _, obj := range resp.Contents {
			_, _ = f.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(f.bucket),
				Key:    obj.Key,
			})
		}
		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		continuation = resp.NextContinuationToken
	}
}

type s3File struct {
	fs     *s3FS
	name   string
	buf    *bytes.Buffer
	pos    int
	closed bool
	dirty  bool
	mu     sync.Mutex
}

func (sf *s3File) Name() string {
	return sf.name
}

func (sf *s3File) Write(p []byte) (n int, err error) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.dirty = true
	return sf.buf.Write(p)
}

func (sf *s3File) Read(p []byte) (n int, err error) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	if sf.pos >= sf.buf.Len() {
		return 0, io.EOF
	}
	n, err = sf.buf.Read(p[sf.pos:])
	if n > 0 {
		sf.pos += n
	}
	return n, err
}

func (sf *s3File) ReadAt(p []byte, off int64) (n int, err error) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	data := sf.buf.Bytes()
	if off >= int64(len(data)) {
		return 0, io.EOF
	}
	n = copy(p, data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
}

func (sf *s3File) Seek(offset int64, whence int) (int64, error) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	switch whence {
	case io.SeekStart:
		sf.pos = int(offset)
	case io.SeekCurrent:
		sf.pos += int(offset)
	case io.SeekEnd:
		sf.pos = sf.buf.Len() + int(offset)
	default:
		return 0, errors.New("invalid whence")
	}
	return int64(sf.pos), nil
}

func (sf *s3File) Close() error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	if sf.closed {
		return nil
	}
	sf.closed = true
	if !sf.dirty {
		return nil
	}
	return sf.flush()
}

func (sf *s3File) flush() error {
	key := sf.fs.key(sf.name)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := sf.fs.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(sf.fs.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(sf.buf.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (sf *s3File) Lock() error {
	return nil
}

func (sf *s3File) Unlock() error {
	return nil
}

func (sf *s3File) Truncate(size int64) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.dirty = true
	if size >= int64(sf.buf.Len()) {
		return nil
	}
	sf.buf.Truncate(int(size))
	return nil
}

type s3FileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (fi *s3FileInfo) Name() string       { return fi.name }
func (fi *s3FileInfo) Size() int64        { return fi.size }
func (fi *s3FileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *s3FileInfo) ModTime() time.Time { return fi.modTime }
func (fi *s3FileInfo) IsDir() bool        { return fi.isDir }
func (fi *s3FileInfo) Sys() interface{}   { return nil }

var _ billy.Filesystem = (*s3FS)(nil)
var _ billy.File = (*s3File)(nil)
