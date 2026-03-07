package gcf

import (
	"fmt"
	"strings"

	"golang.org/x/oauth2/google"
)

// parseImageRef splits an image reference into registry, repository, and tag.
func parseImageRef(ref string) (registry, repo, tag string) {
	tag = "latest"
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			ref = ref[:idx]
		}
	}

	if strings.Contains(ref, ".") || strings.Contains(ref, ":") {
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) == 2 {
			registry = parts[0]
			repo = parts[1]
			return
		}
	}

	registry = "registry-1.docker.io"
	if !strings.Contains(ref, "/") {
		repo = "library/" + ref
	} else {
		repo = ref
	}
	return
}

// getARToken gets an access token via GCP Application Default Credentials.
func (s *Server) getARToken() (string, error) {
	creds, err := google.FindDefaultCredentials(s.ctx(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("find default credentials: %w", err)
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}
	return "Bearer " + token.AccessToken, nil
}
