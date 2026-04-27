// Package runnersinternal exposes shared helpers for the GitHub + GitLab
// runner harnesses. It deliberately uses no build tags so the helpers
// compile on every `go test ./...`; the live harnesses themselves stay
// tag-gated.
//
// Token discipline: PATs are only ever read via these helpers, returned
// as []byte (caller responsibility to zero them on cleanup), and never
// echoed to logs. Runner registration tokens are returned as strings
// because they're short-lived and tied to one harness run.
package runnersinternal

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitHubPAT returns the GitHub personal access token from the gh CLI's
// keychain-backed auth store. The caller must already have run
// `gh auth login` and `gh auth refresh -s workflow` (the workflow scope
// is required for the registration-token endpoint).
//
// Returns the token bytes — caller must zero on cleanup. Errors include
// a hint when the workflow scope is missing.
func GitHubPAT() ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not installed: brew install gh; gh auth login; gh auth refresh -s workflow")
	}
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		return nil, fmt.Errorf("gh not authenticated: gh auth login (then gh auth refresh -s workflow)")
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return nil, fmt.Errorf("gh auth token failed: %w", err)
	}
	return []byte(strings.TrimSpace(string(out))), nil
}

// GitLabPAT returns the GitLab personal access token from the macOS
// Keychain. The keychain entry is created one-time by the operator via:
//
//	security add-generic-password -U -s sockerless-gl-pat -a "$USER" -w
//
// Returns the token bytes — caller must zero on cleanup.
func GitLabPAT() ([]byte, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return nil, fmt.Errorf("security(1) not available — macOS only")
	}
	out, err := exec.Command("security", "find-generic-password",
		"-s", "sockerless-gl-pat", "-a", currentUser(), "-w").Output()
	if err != nil {
		return nil, fmt.Errorf("GitLab PAT not in keychain. One-time setup: " +
			`security add-generic-password -U -s sockerless-gl-pat -a "$USER" -w`)
	}
	return []byte(strings.TrimSpace(string(out))), nil
}

func currentUser() string {
	out, _ := exec.Command("id", "-un").Output()
	return strings.TrimSpace(string(out))
}
