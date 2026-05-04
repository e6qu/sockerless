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
}

// New builds a Client. `repo` is required ("owner/repo"); `token` is
// the GitHub PAT.
func New(httpc *http.Client, token, repo string) *Client {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	return &Client{
		HTTP:     httpc,
		Token:    token,
		APIBase:  "https://api.github.com",
		Repo:     repo,
		Now:      time.Now,
		Seen:     newSeenSet(5 * time.Minute),
		pollWait: 15 * time.Second,
	}
}

// PollOnce fetches the queued runs + jobs and returns every queued job
// not already in the seen-set. Filling the seen-set is the caller's
// responsibility (call Mark(jobID) after a successful spawn) so a
// failed spawn lets the next poll retry.
func (c *Client) PollOnce(ctx context.Context) ([]Job, error) {
	runs, err := c.listQueuedRuns(ctx)
	if err != nil {
		return nil, err
	}
	var queuedJobs []Job
	for _, run := range runs {
		jobs, err := c.listRunJobs(ctx, run.ID)
		if err != nil {
			return nil, fmt.Errorf("list jobs for run %d: %w", run.ID, err)
		}
		for _, j := range jobs {
			if j.Status != "queued" {
				continue
			}
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
	}
	return queuedJobs, nil
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
