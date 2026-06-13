// Package scopes verifies that the supplied GitHub PAT carries the
// scopes the dispatcher needs at startup. Fail-loud-with-instructions
// is the rule — silent degradation when the token is missing scopes
// would mean the poller silently fails on every iteration.
package scopes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Required is the minimum set of OAuth scopes the dispatcher needs.
//
//   - `repo` covers reading queued runs / jobs on private repos and
//     creating ephemeral registration tokens.
//   - `workflow` is required by GitHub for the runner-token endpoint
//     even when the repo is public.
var Required = []string{"repo", "workflow"}

// DefaultAPIBase is github.com's REST root. GHES instances use
// `https://<host>/api/v3` (the dispatchers' `--api-base` flag).
const DefaultAPIBase = "https://api.github.com"

// Verify checks the token against `apiBase` (empty → github.com).
// `GET /user` must authenticate; then one of two paths proves the
// token can do the dispatcher's job:
//
//   - Classic PATs return the `X-OAuth-Scopes` header — assert every
//     entry in Required is granted.
//   - Fine-grained PATs, GitHub App installation tokens, and GHES
//     deployments may not send that header at all. Requiring it
//     rejected tokens that work; instead, verify by CAPABILITY — mint
//     a runner registration token for `repo`, the exact permission
//     the dispatcher needs (the mint is discarded; registration
//     tokens are short-lived and single-purpose).
//
// The HTTP client is taken as a parameter so smoke tests can swap it
// out with a roundtripper that returns deterministic responses.
func Verify(ctx context.Context, client *http.Client, apiBase, repo, token string) error {
	if client == nil {
		client = http.DefaultClient
	}
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	apiBase = strings.TrimRight(apiBase, "/")
	if token == "" {
		return fmt.Errorf("github token is empty — set $GITHUB_TOKEN, run `gh auth token | …`, or pass --token=…")
	}
	userURL := apiBase + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return fmt.Errorf("build /user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", userURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("github token rejected (401). Re-issue with `gh auth login` or a new PAT carrying the %v scopes. Body: %s", Required, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		wait := WaitFromRateHeaders(resp.Header, time.Now())
		return &RateLimitError{
			Status:    resp.StatusCode,
			Body:      strings.TrimSpace(string(body)),
			Wait:      wait,
			Remaining: resp.Header.Get("X-RateLimit-Remaining"),
			Reset:     resp.Header.Get("X-RateLimit-Reset"),
			Retry:     resp.Header.Get("Retry-After"),
		}
	}
	if header := resp.Header.Get("X-OAuth-Scopes"); header != "" {
		scopes := parseScopesHeader(header)
		missing := missingScopes(Required, scopes)
		if len(missing) > 0 {
			return fmt.Errorf(
				"github token is missing required scopes: %v. Granted: %v. Reissue via `gh auth refresh -s %s`",
				missing, scopes, strings.Join(missing, " -s "),
			)
		}
		return nil
	}
	return verifyByCapability(ctx, client, apiBase, repo, token)
}

// verifyByCapability proves the token can mint runner registration
// tokens — the dispatcher's one load-bearing permission — for tokens
// that don't advertise scopes via X-OAuth-Scopes.
func verifyByCapability(ctx context.Context, client *http.Client, apiBase, repo, token string) error {
	if repo == "" {
		return fmt.Errorf("token sends no X-OAuth-Scopes header (fine-grained PAT / app token / GHES) and no repo is configured to capability-probe against")
	}
	mintURL := fmt.Sprintf("%s/repos/%s/actions/runners/registration-token", apiBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mintURL, nil)
	if err != nil {
		return fmt.Errorf("build registration-token probe: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", mintURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf(
			"token authenticates but cannot mint runner registration tokens for %s (%d %s) — grant the fine-grained `administration: write` repo permission (classic: %v). Body: %s",
			repo, resp.StatusCode, http.StatusText(resp.StatusCode), Required, strings.TrimSpace(string(body)),
		)
	}
	return nil
}

func parseScopesHeader(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func missingScopes(required, granted []string) []string {
	have := make(map[string]struct{}, len(granted))
	for _, g := range granted {
		have[g] = struct{}{}
	}
	var missing []string
	for _, r := range required {
		if _, ok := have[r]; !ok {
			missing = append(missing, r)
		}
	}
	return missing
}

// RateLimitError is returned by Verify when GitHub responds with a
// non-200 status that carries rate-limit hints. Callers can sleep
// `Wait` (already includes the +10% +1s buffer) before retrying.
type RateLimitError struct {
	Status    int
	Body      string
	Wait      time.Duration
	Remaining string
	Reset     string
	Retry     string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("GET /user returned %d %s (rate-remaining=%q rate-reset=%q retry-after=%q wait=%s). Body: %s",
		e.Status, http.StatusText(e.Status), e.Remaining, e.Reset, e.Retry, e.Wait, e.Body)
}

// AsRateLimit unwraps a RateLimitError from Verify's return — callers
// use it to read Wait before sleeping. Returns (nil, false) for any
// other error.
func AsRateLimit(err error) (*RateLimitError, bool) {
	var rle *RateLimitError
	if errors.As(err, &rle) {
		return rle, true
	}
	return nil, false
}

// WaitFromRateHeaders computes how long to sleep before retrying the
// upstream API based on rate-limit hints in resp.Header. Honors the
// strict-rate-limit feedback rule: take max(Retry-After, X-RateLimit-Reset
// since now) * 1.10 + 1s. If no hint is present, returns 0 — caller
// should fall back to its own exponential backoff.
func WaitFromRateHeaders(h http.Header, now time.Time) time.Duration {
	var raw time.Duration
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			raw = time.Duration(secs) * time.Second
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if epoch, err := strconv.ParseInt(v, 10, 64); err == nil && epoch > 0 {
			d := time.Unix(epoch, 0).Sub(now)
			if d > raw {
				raw = d
			}
		}
	}
	if raw <= 0 {
		return 0
	}
	// Buffer per `feedback_strict_rate_limit.md`: clock skew + bunching.
	buffered := time.Duration(float64(raw)*1.10) + time.Second
	return buffered
}
