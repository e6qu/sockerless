// Package poller queries the GitHub Actions REST API for queued
// workflow_jobs and emits one Job event per newly-seen queued job.
//
// API path: `GET /repos/{owner}/{repo}/actions/runs?status=queued`
// followed by `GET /repos/{owner}/{repo}/actions/runs/{run_id}/jobs`
// per run. The two-call shape mirrors the production webhook payload —
// 110b switches to the webhook ingress without the consumers having
// to change.
//
// Stateless: dedup is a 5-min TTL `seen` set keyed by job ID. A
// caller restart loses the set and may double-spawn; the runner image
// entrypoint's 60-s idle timeout cleans up duplicates.
package poller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sockerless/github-runner-dispatcher-aws/pkg/scopes"
)

// Job is the dispatcher's view of a queued workflow_job.
type Job struct {
	JobID    int64    // GitHub workflow_job ID — used as dedup key
	RunID    int64    // parent workflow run
	Name     string   // job display name (logging only)
	Labels   []string // requested runs-on labels
	Repo     string   // owner/repo
	JobURL   string   // GitHub URL to the job (logging only)
	QueuedAt time.Time
}

// Client is a thin wrapper over net/http with a token + API base URL.
type Client struct {
	HTTP     *http.Client
	Token    string
	APIBase  string // override for tests; defaults to https://api.github.com
	Repo     string // "owner/repo"
	Now      func() time.Time
	Seen     *seenSet
	pollWait time.Duration

	// runSeen tracks runIDs whose every job has been Mark()-ed or is no
	// longer queued — re-fetching their jobs costs API quota but yields
	// no new work. Entries expire on the same TTL as the per-job seen-set.
	runSeen *seenSet

	// rlMu/rl* track the latest GitHub rate-limit headers. PollOnce reads
	// these before issuing new calls; if remaining drops below 5% of the
	// limit, we back off until reset to avoid blowing the bucket. Updated
	// on every successful HTTP response by get(). Per
	// `feedback_strict_rate_limit.md`: honor upstream hints, don't burn
	// the bucket on speculative polls.
	rlMu        sync.Mutex
	rlRemaining int
	rlReset     time.Time
	rlLimit     int
}

// New builds a Client. `repo` is required ("owner/repo"); `token` is
// the GitHub PAT.
func New(httpc *http.Client, token, repo string) *Client {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	return &Client{
		HTTP:    httpc,
		Token:   token,
		APIBase: "https://api.github.com",
		Repo:    repo,
		Now:     time.Now,
		Seen:    newSeenSet(5 * time.Minute),
		runSeen: newSeenSet(5 * time.Minute),
		// Default 60s. 15s is too aggressive when many queued runs exist
		// (each poll costs 1+N GitHub calls; 4 polls/min × 30 runs ≈
		// 7440 calls/h, exceeds GitHub's 5000/h). PollOnce's proactive
		// back-off catches the depletion case; this just paces nominal
		// load.
		pollWait: 60 * time.Second,
	}
}

// PollOnce fetches the queued runs + jobs and returns every queued job
// not already in the seen-set. Filling the seen-set is the caller's
// responsibility (call Mark(jobID) after a successful spawn) so a
// failed spawn lets the next poll retry.
//
// Returns a RateLimitError immediately (no API call) when the cached
// X-RateLimit-Remaining indicates the bucket is depleted — the caller
// (dispatch loop) sleeps the wait and retries. Skips per-run job
// fetches for runs already in runSeen (TTL-bounded) to avoid burning
// quota on dormant queued runs that no dispatcher serves.
func (c *Client) PollOnce(ctx context.Context) ([]Job, error) {
	if wait, ok := c.shouldBackOff(); ok {
		return nil, fmt.Errorf("poller proactive back-off: %w", &scopes.RateLimitError{
			Status: 429, Body: "client-side: rate-limit headers near depletion",
			Wait:      wait,
			Remaining: fmt.Sprintf("%d", c.rlRemaining),
			Reset:     fmt.Sprintf("%d", c.rlReset.Unix()),
		})
	}
	runs, err := c.listQueuedRuns(ctx)
	if err != nil {
		return nil, err
	}
	var queuedJobs []Job
	for _, run := range runs {
		if c.runSeen.Has(run.ID) {
			continue
		}
		jobs, err := c.listRunJobs(ctx, run.ID)
		if err != nil {
			return nil, fmt.Errorf("list jobs for run %d: %w", run.ID, err)
		}
		anyQueued := false
		for _, j := range jobs {
			if j.Status != "queued" {
				continue
			}
			anyQueued = true
			if c.Seen.Has(j.ID) {
				continue
			}
			queuedJobs = append(queuedJobs, Job{
				JobID:    j.ID,
				RunID:    run.ID,
				Name:     j.Name,
				Labels:   j.Labels,
				Repo:     c.Repo,
				JobURL:   j.HTMLURL,
				QueuedAt: c.Now(),
			})
		}
		// If the run has zero queued jobs (all done or in-flight) OR
		// every queued job is in the seen-set, mark the run itself so
		// subsequent polls skip the listRunJobs call until TTL expiry.
		// New jobs added to the run within TTL are missed; that's an
		// accepted trade for not blowing the API quota on dormant runs
		// (e.g., hello-ecs jobs queued without an ECS dispatcher).
		if !anyQueued {
			c.runSeen.Add(run.ID, c.Now())
		}
	}
	return queuedJobs, nil
}

// shouldBackOff returns (wait, true) when the cached rate-limit headers
// suggest the next API call would exhaust the bucket. Threshold: skip
// polls when remaining < max(50, 5% of limit). Wait is reset-time + the
// standard +10% +1s buffer.
func (c *Client) shouldBackOff() (time.Duration, bool) {
	c.rlMu.Lock()
	defer c.rlMu.Unlock()
	if c.rlLimit <= 0 || c.rlReset.IsZero() {
		return 0, false
	}
	threshold := c.rlLimit / 20
	if threshold < 50 {
		threshold = 50
	}
	if c.rlRemaining > threshold {
		return 0, false
	}
	until := time.Until(c.rlReset)
	if until <= 0 {
		return 0, false
	}
	return time.Duration(float64(until)*1.10) + time.Second, true
}

// Mark records `jobID` so subsequent polls skip it for the seen-set's
// TTL. Caller is expected to call this after a successful spawn.
func (c *Client) Mark(jobID int64) {
	c.Seen.Add(jobID, c.Now())
}

// PollInterval is the cadence the dispatcher's main loop should call
// PollOnce at. Mirrors the spec (15 s).
func (c *Client) PollInterval() time.Duration { return c.pollWait }

// listQueuedRuns fetches `/actions/runs?status=queued` and returns the
// (possibly empty) run list.
func (c *Client) listQueuedRuns(ctx context.Context) ([]apiRun, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/runs?status=queued&per_page=100", c.APIBase, c.Repo)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		WorkflowRuns []apiRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode runs: %w", err)
	}
	return resp.WorkflowRuns, nil
}

// listRunJobs fetches `/actions/runs/{run_id}/jobs`. GitHub returns
// `queued`, `in_progress`, `completed` jobs all in one call; caller
// filters by status.
func (c *Client) listRunJobs(ctx context.Context, runID int64) ([]apiJob, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/runs/%d/jobs?per_page=100", c.APIBase, c.Repo, runID)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Jobs []apiJob `json:"jobs"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode jobs: %w", err)
	}
	return resp.Jobs, nil
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.absorbRateHeaders(resp.Header)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		// Surface a typed RateLimitError when the response carries rate
		// hints — the caller's dispatch loop reads Wait to honor GitHub's
		// reset window strictly (per `feedback_strict_rate_limit.md`).
		wait := scopes.WaitFromRateHeaders(resp.Header, time.Now())
		err := &scopes.RateLimitError{
			Status:    resp.StatusCode,
			Body:      strings.TrimSpace(string(body)),
			Wait:      wait,
			Remaining: resp.Header.Get("X-RateLimit-Remaining"),
			Reset:     resp.Header.Get("X-RateLimit-Reset"),
			Retry:     resp.Header.Get("Retry-After"),
		}
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return readAll(resp.Body), nil
}

// asScopesRateLimit unwraps a scopes.RateLimitError if present in err's
// chain. Re-exported to avoid import cycles in the dispatcher main.
func asScopesRateLimit(err error) (*scopes.RateLimitError, bool) {
	var rle *scopes.RateLimitError
	if errors.As(err, &rle) {
		return rle, true
	}
	return nil, false
}

// AsRateLimit reports whether err contains a GitHub rate-limit signal
// and returns the wait derived from upstream headers (already buffered).
// Caller (dispatch loop) sleeps this wait before the next poll.
func AsRateLimit(err error) (time.Duration, bool) {
	if rle, ok := asScopesRateLimit(err); ok && rle.Wait > 0 {
		return rle.Wait, true
	}
	return 0, false
}

type apiRun struct {
	ID int64 `json:"id"`
}

type apiJob struct {
	ID      int64    `json:"id"`
	RunID   int64    `json:"run_id"`
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Labels  []string `json:"labels"`
	HTMLURL string   `json:"html_url"`
}

// seenSet is a TTL-bounded dedup set. Entries older than `ttl` are
// purged on lookup so the set self-prunes without a background loop.
type seenSet struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[int64]time.Time
}

func newSeenSet(ttl time.Duration) *seenSet {
	return &seenSet{ttl: ttl, m: make(map[int64]time.Time)}
}

func (s *seenSet) Add(id int64, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = now
}

func (s *seenSet) Has(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.m {
		if now.Sub(t) > s.ttl {
			delete(s.m, k)
		}
	}
	_, ok := s.m[id]
	return ok
}

// absorbRateHeaders updates the cached rate-limit state from the latest
// HTTP response headers. Called from get() on every request (success or
// failure) so PollOnce's pre-flight back-off has fresh data.
func (c *Client) absorbRateHeaders(h http.Header) {
	c.rlMu.Lock()
	defer c.rlMu.Unlock()
	if v := h.Get("X-RateLimit-Limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.rlLimit = n
		}
	}
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.rlRemaining = n
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if epoch, err := strconv.ParseInt(v, 10, 64); err == nil && epoch > 0 {
			c.rlReset = time.Unix(epoch, 0)
		}
	}
}
