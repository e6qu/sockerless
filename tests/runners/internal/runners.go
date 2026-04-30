package runnersinternal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// MintGitHubRegistrationToken returns a short-lived (~1 h) registration
// token for self-hosted runner setup against the given owner/repo. Uses
// `gh api` so auth flows through the keychain; the caller's PAT must
// have the `workflow` scope.
func MintGitHubRegistrationToken(repo string) (string, error) {
	out, err := exec.Command("gh", "api", "-X", "POST",
		fmt.Sprintf("/repos/%s/actions/runners/registration-token", repo),
		"--jq", ".token").Output()
	if err != nil {
		return "", fmt.Errorf("mint GitHub registration token (need 'workflow' scope on the PAT — run `gh auth refresh -s workflow`): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MintGitHubRemoveToken returns a short-lived removal token used by
// `config.sh remove --token <REMOVE_TOKEN>` so the harness can
// unregister cleanly even if it doesn't know the runner ID.
func MintGitHubRemoveToken(repo string) (string, error) {
	out, err := exec.Command("gh", "api", "-X", "POST",
		fmt.Sprintf("/repos/%s/actions/runners/remove-token", repo),
		"--jq", ".token").Output()
	if err != nil {
		return "", fmt.Errorf("mint GitHub remove token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CleanupOldGitHubRunners deletes any self-hosted runner registered
// against the repo whose name starts with the given prefix. Used as a
// self-healing step at the start of each harness run so a previous
// crash doesn't leave dangling registrations.
func CleanupOldGitHubRunners(repo, namePrefix string) error {
	out, err := exec.Command("gh", "api", "--paginate",
		fmt.Sprintf("/repos/%s/actions/runners", repo)).Output()
	if err != nil {
		return fmt.Errorf("list GitHub runners: %w", err)
	}
	var resp struct {
		Runners []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"runners"`
	}
	if jerr := json.Unmarshal(out, &resp); jerr != nil {
		return fmt.Errorf("parse GitHub runners list: %w", jerr)
	}
	for _, r := range resp.Runners {
		if !strings.HasPrefix(r.Name, namePrefix) {
			continue
		}
		_ = exec.Command("gh", "api", "-X", "DELETE",
			fmt.Sprintf("/repos/%s/actions/runners/%d", repo, r.ID)).Run()
	}
	return nil
}

// GitLabRunner is the response shape from POST /api/v4/user/runners.
type GitLabRunner struct {
	ID    int64  `json:"id"`
	Token string `json:"token"`
}

// CreateGitLabRunner mints an authentication token for a project-scoped
// runner via the `/user/runners` endpoint (the modern API; the legacy
// project registration token is deprecated). Returns the runner ID
// (used for cleanup) and the auth token (passed to `gitlab-runner
// register --token`).
func CreateGitLabRunner(pat []byte, projectID int64, description string, tags []string) (*GitLabRunner, error) {
	form := url.Values{}
	form.Set("runner_type", "project_type")
	form.Set("project_id", fmt.Sprintf("%d", projectID))
	form.Set("description", description)
	for _, t := range tags {
		form.Add("tag_list[]", t)
	}
	req, err := http.NewRequest(http.MethodPost,
		"https://gitlab.com/api/v4/user/runners",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create GitLab runner: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("create GitLab runner: HTTP %d %s", resp.StatusCode, buf.String())
	}
	var r GitLabRunner
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode runner response: %w", err)
	}
	return &r, nil
}

// DeleteGitLabRunner removes a runner by ID. Used in the harness
// cleanup path. Returns nil even on 404 — idempotent cleanup.
func DeleteGitLabRunner(pat []byte, runnerID int64) error {
	req, _ := http.NewRequest(http.MethodDelete,
		fmt.Sprintf("https://gitlab.com/api/v4/runners/%d", runnerID), nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete GitLab runner: %w", err)
	}
	resp.Body.Close()
	return nil
}

// ResolveGitLabProjectID looks up the numeric project ID for a path
// like "e6qu/sockerless". The runner-create API takes the numeric ID,
// not the path.
func ResolveGitLabProjectID(pat []byte, projectPath string) (int64, error) {
	req, _ := http.NewRequest(http.MethodGet,
		"https://gitlab.com/api/v4/projects/"+url.PathEscape(projectPath), nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("resolve GitLab project %q: HTTP %d", projectPath, resp.StatusCode)
	}
	var p struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return 0, err
	}
	return p.ID, nil
}

// CleanupOldGitLabRunners deletes any project-scoped runner whose
// description starts with the given prefix (e.g. "sockerless-").
// Mirror of CleanupOldGitHubRunners.
func CleanupOldGitLabRunners(pat []byte, projectID int64, descriptionPrefix string) error {
	endpoint := fmt.Sprintf("https://gitlab.com/api/v4/projects/%d/runners", projectID)
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var runners []struct {
		ID          int64  `json:"id"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runners); err != nil {
		return err
	}
	for _, r := range runners {
		if !strings.HasPrefix(r.Description, descriptionPrefix) {
			continue
		}
		_ = DeleteGitLabRunner(pat, r.ID)
	}
	return nil
}

// Timestamp returns a stable per-run UTC timestamp suitable for runner
// names ("sockerless-ecs-20260427-153012").
func Timestamp() string {
	return time.Now().UTC().Format("20060102-150405")
}
