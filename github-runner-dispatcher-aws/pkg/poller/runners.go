package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Runner is the dispatcher's view of a self-hosted runner registered
// against the repo. Used by the GC sweep to reap offline
// dispatcher-spawned runners that GitHub still has on file.
type Runner struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	OS     string `json:"os"`
	Status string `json:"status"` // "online" / "offline"
	Busy   bool   `json:"busy"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ListRunners hits `GET /repos/{repo}/actions/runners` and returns
// every registered runner. Paginates implicitly via per_page=100; if
// the dispatcher's runner pool grows past 100 GitHub will silently
// truncate (in which case the caller should add cursor pagination).
func (c *Client) ListRunners(ctx context.Context) ([]Runner, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/runners?per_page=100", c.APIBase, c.Repo)
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Runners []Runner `json:"runners"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode runners: %w", err)
	}
	return resp.Runners, nil
}

// DeleteRunner removes a registered runner from the repo. Used by the
// GC sweep — offline dispatcher-spawned runners whose containers are
// already gone get reaped here so the GitHub UI doesn't accumulate
// `dispatcher-…` entries indefinitely.
func (c *Client) DeleteRunner(ctx context.Context, runnerID int64) error {
	url := fmt.Sprintf("%s/repos/%s/actions/runners/%d", c.APIBase, c.Repo, runnerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 204 = deleted, 404 = already gone (treat as success — idempotent).
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("DELETE %s: %d %s", url, resp.StatusCode, http.StatusText(resp.StatusCode))
}

// DispatcherRunnerPrefix is the name prefix Spawn assigns to every
// runner the dispatcher creates. Lets the GC sweep target only its
// own runners and ignore unrelated self-hosted runners on the same
// repo.
const DispatcherRunnerPrefix = "dispatcher-"

// IsDispatcherRunner reports whether the runner's Name was assigned
// by this dispatcher (matches the `dispatcher-<jobID>-<unix>`
// pattern Spawn writes).
func IsDispatcherRunner(r Runner) bool {
	return strings.HasPrefix(r.Name, DispatcherRunnerPrefix)
}
