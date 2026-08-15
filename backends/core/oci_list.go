package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sockerless/api"
)

// OCIListOptions configures an OCI registry catalog+tags listing.
type OCIListOptions struct {
	Registry string // e.g. "my-registry.azurecr.io" or "us-docker.pkg.dev/proj/repo"
	// Endpoint is the network coordinate the registry is reached at
	// ("scheme://host") when an operator has relocated it away from the host
	// its references name. Empty dials https://<Registry>.
	Endpoint string
	// TokenFor returns the credential that authorizes reading `repo` — the
	// registry's catalog when it is empty. A registry that issues per-resource
	// tokens needs one per repository, so the listing asks for each as it
	// reaches it.
	TokenFor func(repo string) (string, error)
	// Catalog limits how many repositories to enumerate per page from
	// GET /v2/_catalog. Many registries cap at 100; zero = default 100.
	CatalogPageSize int
	// Deadline caps total time across catalog + per-repo tag listing.
	// Zero = default 60s.
	Deadline time.Duration
}

// ociListClient is an HTTP client with timeouts for OCI list requests.
var ociListClient = &http.Client{Timeout: 60 * time.Second}

// OCIListImages enumerates every repository via GET /v2/_catalog, then
// every tag per repository via GET /v2/<repo>/tags/list, projecting the
// result into `api.ImageSummary` with fully-qualified RepoTags. Used by
// backends that expose their cloud container registry through the OCI
// distribution v2 protocol (GCP Artifact Registry + Azure Container
// Registry + anything else in the family).
//
// A failing catalog or tag request surfaces as an error: a listing that
// reported only the repositories it happened to be able to read would show a
// short image list as if it were the registry's whole contents.
func OCIListImages(ctx context.Context, opts OCIListOptions) ([]*api.ImageSummary, error) {
	if opts.TokenFor == nil {
		return nil, fmt.Errorf("OCIListImages: TokenFor is required to authenticate against %s", opts.Registry)
	}
	if opts.CatalogPageSize == 0 {
		opts.CatalogPageSize = 100
	}
	if opts.Deadline == 0 {
		opts.Deadline = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Deadline)
	defer cancel()

	repos, err := ociCatalog(ctx, opts)
	if err != nil {
		return nil, err
	}

	var result []*api.ImageSummary
	for _, repo := range repos {
		tags, tErr := ociTags(ctx, opts, repo)
		if tErr != nil {
			return nil, tErr
		}
		for _, tag := range tags {
			ref := opts.Registry + "/" + repo + ":" + tag
			result = append(result, &api.ImageSummary{
				ID:          ref, // no digest available from list endpoints; ref serves as stable identity
				RepoTags:    []string{ref},
				RepoDigests: []string{opts.Registry + "/" + repo},
			})
		}
	}
	return result, nil
}

// ociCatalog returns every repository under opts.Registry via
// GET /v2/_catalog (pagination via the `Link` header or `last` query).
func ociCatalog(ctx context.Context, opts OCIListOptions) ([]string, error) {
	var repos []string
	var last string
	token, err := opts.TokenFor("")
	if err != nil {
		return repos, fmt.Errorf("catalog %s: auth: %w", opts.Registry, err)
	}
	for {
		url := fmt.Sprintf("%s/v2/_catalog?n=%d", OCIRegistryBaseURL(opts.Endpoint, opts.Registry), opts.CatalogPageSize)
		if last != "" {
			url += "&last=" + last
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return repos, err
		}
		SetOCIHost(req, opts.Registry)
		SetOCIAuth(req, token)
		resp, err := ociListClient.Do(req)
		if err != nil {
			return repos, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return repos, fmt.Errorf("catalog %s: %d %s", opts.Registry, resp.StatusCode, string(body))
		}
		var page struct {
			Repositories []string `json:"repositories"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return repos, fmt.Errorf("catalog %s: decode: %w", opts.Registry, err)
		}
		repos = append(repos, page.Repositories...)
		if len(page.Repositories) < opts.CatalogPageSize {
			return repos, nil
		}
		last = page.Repositories[len(page.Repositories)-1]
	}
}

// ociTags returns every tag for a single repository under opts.Registry
// via GET /v2/<repo>/tags/list.
func ociTags(ctx context.Context, opts OCIListOptions, repo string) ([]string, error) {
	token, err := opts.TokenFor(repo)
	if err != nil {
		return nil, fmt.Errorf("tags %s/%s: auth: %w", opts.Registry, repo, err)
	}
	url := fmt.Sprintf("%s/v2/%s/tags/list", OCIRegistryBaseURL(opts.Endpoint, opts.Registry), repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	SetOCIHost(req, opts.Registry)
	SetOCIAuth(req, token)
	resp, err := ociListClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tags %s/%s: %d", opts.Registry, repo, resp.StatusCode)
	}
	var page struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	return page.Tags, nil
}
