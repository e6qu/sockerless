package lambda

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
)

// tagCacheTTL bounds how long a cached ListTags result is reused. It is
// deliberately short so the cache only coalesces a burst of poll ticks /
// concurrent docker ps|inspect|wait calls that all need the same
// function's tags within the same fraction of a second. After the TTL a
// fresh ListTags is issued, so the cache never becomes authoritative —
// the cloud stays the source of truth and a dropped cache changes
// nothing but call volume.
const tagCacheTTL = 2 * time.Second

// lambdaTagCache is a short-TTL read-through cache of Lambda ListTags
// results keyed by function ARN. queryFunctions and resolveFunctionARN
// both walk every function and previously issued one ListTags call per
// function per tick (N+1 against ListFunctions); under a poll loop or a
// burst of concurrent docker ps calls that is ~N calls each, repeated
// every PollInterval. The cache lets all callers within tagCacheTTL
// share a single ListTags per function.
//
// It is a pure optimization: every entry is re-derivable from the cloud,
// nothing reads through it as state, and dropping it only restores the
// uncached N+1 behavior. Entries past their TTL are refetched, never
// served stale.
type lambdaTagCache struct {
	mu      sync.Mutex
	entries map[string]tagCacheEntry
}

type tagCacheEntry struct {
	tags    map[string]string
	fetched time.Time
}

func newLambdaTagCache() *lambdaTagCache {
	return &lambdaTagCache{entries: make(map[string]tagCacheEntry)}
}

// listTagsCached returns the sockerless tags for a Lambda function ARN,
// serving a cached copy when one was fetched within tagCacheTTL and
// issuing a real ListTags otherwise. The returned map is a defensive
// copy so callers can read it without holding the lock; an error from
// ListTags is surfaced (never cached) so transient throttles don't pin a
// stale or empty tag set.
func (c *lambdaTagCache) listTagsCached(ctx context.Context, client lambdaTagLister, arn *string) (map[string]string, error) {
	key := aws.ToString(arn)
	now := time.Now()

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && now.Sub(e.fetched) < tagCacheTTL {
		out := copyTags(e.tags)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	res, err := client.ListTags(ctx, &awslambda.ListTagsInput{Resource: arn})
	if err != nil {
		return nil, err
	}
	tags := res.Tags
	if tags == nil {
		tags = map[string]string{}
	}

	c.mu.Lock()
	c.entries[key] = tagCacheEntry{tags: copyTags(tags), fetched: now}
	c.mu.Unlock()

	return copyTags(tags), nil
}

func copyTags(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// lambdaTagLister is the subset of the Lambda client the tag cache needs.
type lambdaTagLister interface {
	ListTags(ctx context.Context, in *awslambda.ListTagsInput, opts ...func(*awslambda.Options)) (*awslambda.ListTagsOutput, error)
}
