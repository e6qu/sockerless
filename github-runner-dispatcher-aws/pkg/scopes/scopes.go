// Package scopes verifies that the supplied GitHub PAT carries the
// scopes the dispatcher needs at startup. Fail-loud-with-instructions
// is the rule — silent degradation when the token is missing scopes
// would mean the poller silently fails on every iteration.
package scopes

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Required is the minimum set of OAuth scopes the dispatcher needs.
//
//   - `repo` covers reading queued runs / jobs on private repos and
//     creating ephemeral registration tokens.
//   - `workflow` is required by GitHub for the runner-token endpoint
//     even when the repo is public.
var Required = []string{"repo", "workflow"}

// Verify hits `GET /user` with the supplied token; reads the
// `X-OAuth-Scopes` response header and asserts every entry in Required
// is present. Returns a guidance error otherwise.
//
// The HTTP client is taken as a parameter so smoke tests can swap it
// out with a roundtripper that returns a deterministic header.
func Verify(ctx context.Context, client *http.Client, token string) error {
	if client == nil {
		client = http.DefaultClient
	}
	if token == "" {
		return fmt.Errorf("github token is empty — set $GITHUB_TOKEN, run `gh auth token | …`, or pass --token=…")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return fmt.Errorf("build /user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET https://api.github.com/user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("github token rejected (401). Re-issue with `gh auth login` or a new PAT carrying the %v scopes", Required)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /user returned %d %s — fix the token before retrying", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	scopes := parseScopesHeader(resp.Header.Get("X-OAuth-Scopes"))
	missing := missingScopes(Required, scopes)
	if len(missing) > 0 {
		return fmt.Errorf(
			"github token is missing required scopes: %v. Granted: %v. Reissue via `gh auth refresh -s %s` or a new fine-grained PAT including: %s",
			missing, scopes, strings.Join(missing, " -s "), strings.Join(Required, ", "),
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
