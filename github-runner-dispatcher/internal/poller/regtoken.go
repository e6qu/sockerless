package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MintRegistrationToken hits
// `POST /repos/{owner}/{repo}/actions/runners/registration-token` to
// obtain a one-shot ephemeral registration token. The token is valid
// for 1 h; the runner uses it once during `./config.sh` and discards
// it. See:
// https://docs.github.com/en/rest/actions/self-hosted-runners?apiVersion=2022-11-28#create-a-registration-token-for-a-repository
func (c *Client) MintRegistrationToken(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/runners/registration-token", c.APIBase, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("POST %s: %d %s", url, resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode registration token: %w", err)
	}
	if body.Token == "" {
		return "", fmt.Errorf("github returned empty registration token")
	}
	return body.Token, nil
}
