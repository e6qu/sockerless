package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// quarantine is how long a release waits before a pin must adopt it.
const quarantine = 24 * time.Hour

// unmaintained is how long Docker Hub must have left a release untouched for
// it to count as out of support rather than newer. Docker rebuilds every
// supported official image tag for base-image and security updates far more
// often than this; Ubuntu's interim releases such as 24.10, for example, stop
// being pushed when they reach end of life, and are not newer than 24.04 LTS
// in any sense a pin should follow.
const unmaintained = 90 * 24 * time.Hour

// ecrSpacing keeps requests to ECR Public under its limit of one
// unauthenticated request per second.
const ecrSpacing = 1100 * time.Millisecond

var version = regexp.MustCompile(`^(v?)(\d+(?:\.\d+)*)(-[\w.-]+)?$`)

// Release is a Docker Hub tag and when it was last pushed.
type Release struct {
	Tag    string
	Pushed time.Time
}

// newerReleases returns the candidates that are newer releases of the same
// major version and variant as tag and still maintained at now, newest first.
// Tags that are not versions, such as latest or bookworm-slim, have no newer
// releases.
func newerReleases(tag string, candidates []Release, now time.Time) []Release {
	m := version.FindStringSubmatch(tag)
	if m == nil {
		return nil
	}
	pinned := numbers(m[2])
	type ranked struct {
		n []int
		r Release
	}
	var out []ranked
	for _, c := range candidates {
		o := version.FindStringSubmatch(c.Tag)
		if o == nil || o[1] != m[1] || o[3] != m[3] || now.Sub(c.Pushed) > unmaintained {
			continue
		}
		n := numbers(o[2])
		if len(n) == len(pinned) && n[0] == pinned[0] && less(pinned, n) {
			out = append(out, ranked{n, c})
		}
	}
	sort.Slice(out, func(i, j int) bool { return less(out[j].n, out[i].n) })
	releases := make([]Release, len(out))
	for i, r := range out {
		releases[i] = r.r
	}
	return releases
}

func numbers(s string) []int {
	parts := strings.Split(s, ".")
	n := make([]int, len(parts))
	for i, p := range parts {
		n[i], _ = strconv.Atoi(p) // the version pattern admits digits only
	}
	return n
}

func less(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Verdict is the outcome for one pin: "ok", "HELD" or "DRIFT".
type Verdict struct {
	State string
	Notes []string
}

// judge classifies one pin against what Docker Hub and the mirror serve.
func judge(pin, hubDigest string, hubPushed time.Time, mirrorDigest string, newer []Release, now time.Time) Verdict {
	var problems, held []string
	switch {
	case hubDigest != pin && now.Sub(hubPushed) >= quarantine:
		problems = append(problems, fmt.Sprintf("Docker Hub has served %s for the tag since %s", hubDigest, hubPushed.Format(time.RFC3339)))
	case hubDigest != pin:
		held = append(held, fmt.Sprintf("Docker Hub moved the tag to %s at %s", hubDigest, hubPushed.Format(time.RFC3339)))
	case mirrorDigest != pin:
		problems = append(problems, fmt.Sprintf("the mirror serves %s for the tag, not the pinned digest", mirrorDigest))
	}
	var eligible []Release
	for _, r := range newer {
		if now.Sub(r.Pushed) >= quarantine {
			eligible = append(eligible, r)
		}
	}
	if len(eligible) > 0 {
		problems = append(problems, fmt.Sprintf("newer release %s has been on Docker Hub since %s", eligible[0].Tag, eligible[0].Pushed.Format(time.RFC3339)))
	} else if len(newer) > 0 {
		held = append(held, fmt.Sprintf("newer release %s is inside the 24-hour quarantine", newer[0].Tag))
	}
	switch {
	case len(problems) > 0:
		return Verdict{"DRIFT", problems}
	case len(held) > 0:
		return Verdict{"HELD", held}
	}
	return Verdict{"ok", nil}
}

// Registries reads Docker Hub and the two mirrors. The base URLs are fields so
// tests can point them at a local server.
type Registries struct {
	Client  *http.Client
	Hub     string // https://hub.docker.com
	ECR     string // https://public.ecr.aws
	GCR     string // https://mirror.gcr.io
	lastECR time.Time
}

func (r *Registries) get(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "sockerless-check-image-pins")
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s %s: HTTP %d", req.Method, req.URL, resp.StatusCode)
	}
	return resp, nil
}

func (r *Registries) getJSON(rawURL string, out any) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := r.get(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return json.NewDecoder(resp.Body).Decode(out)
}

// ecr spaces ECR Public requests a little over a second apart.
func (r *Registries) ecr(req *http.Request) (*http.Response, error) {
	if wait := time.Until(r.lastECR.Add(ecrSpacing)); wait > 0 {
		time.Sleep(wait)
	}
	defer func() { r.lastECR = time.Now() }()
	return r.get(req)
}

type hubTag struct {
	Name          string `json:"name"`
	Digest        string `json:"digest"`
	TagLastPushed string `json:"tag_last_pushed"`
}

// HubTag returns the digest Docker Hub serves for the tag and when it was pushed.
func (r *Registries) HubTag(repo, tag string) (string, time.Time, error) {
	var t hubTag
	if err := r.getJSON(fmt.Sprintf("%s/v2/repositories/%s/tags/%s", r.Hub, repo, url.PathEscape(tag)), &t); err != nil {
		return "", time.Time{}, err
	}
	pushed, err := time.Parse(time.RFC3339Nano, t.TagLastPushed)
	if err != nil || t.Digest == "" {
		return "", time.Time{}, fmt.Errorf("docker hub returned no digest or push time for %s:%s", repo, tag)
	}
	return t.Digest, pushed, nil
}

// Candidates lists the most recently pushed Docker Hub tags that share the
// pinned tag's major version prefix. A tag that is not a version has none.
func (r *Registries) Candidates(repo, tag string) ([]Release, error) {
	m := version.FindStringSubmatch(tag)
	if m == nil {
		return nil, nil
	}
	prefix := m[1] + strings.SplitN(m[2], ".", 2)[0] + "."
	var page struct {
		Results []hubTag `json:"results"`
	}
	q := url.Values{"page_size": {"100"}, "ordering": {"last_updated"}, "name": {prefix}}
	if err := r.getJSON(fmt.Sprintf("%s/v2/repositories/%s/tags?%s", r.Hub, repo, q.Encode()), &page); err != nil {
		return nil, err
	}
	var out []Release
	for _, t := range page.Results {
		pushed, err := time.Parse(time.RFC3339Nano, t.TagLastPushed)
		if err != nil {
			return nil, fmt.Errorf("docker hub returned no push time for %s:%s", repo, t.Name)
		}
		out = append(out, Release{t.Name, pushed})
	}
	return out, nil
}

const indexTypes = "application/vnd.oci.image.index.v1+json, " +
	"application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, " +
	"application/vnd.docker.distribution.manifest.v2+json"

// MirrorDigest returns the digest the pin's mirror serves for its tag.
func (r *Registries) MirrorDigest(pin Pin) (string, error) {
	var req *http.Request
	var err error
	send := r.get
	if pin.Registry == "mirror.gcr.io" {
		req, err = http.NewRequest(http.MethodHead, fmt.Sprintf("%s/v2/%s/manifests/%s", r.GCR, pin.Path, pin.Tag), nil)
	} else {
		send = r.ecr
		q := url.Values{"scope": {"repository:docker/library/" + pin.Path + ":pull"}, "service": {"public.ecr.aws"}}
		tokenReq, terr := http.NewRequest(http.MethodGet, r.ECR+"/token/?"+q.Encode(), nil)
		if terr != nil {
			return "", terr
		}
		resp, terr := r.ecr(tokenReq)
		if terr != nil {
			return "", terr
		}
		var token struct {
			Token string `json:"token"`
		}
		terr = json.NewDecoder(resp.Body).Decode(&token)
		_ = resp.Body.Close()
		if terr != nil || token.Token == "" {
			return "", fmt.Errorf("public.ecr.aws returned no pull token for %s", pin.Path)
		}
		req, err = http.NewRequest(http.MethodHead, fmt.Sprintf("%s/v2/docker/library/%s/manifests/%s", r.ECR, pin.Path, pin.Tag), nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+token.Token)
		}
	}
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", indexTypes)
	resp, err := send(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("%s returned no digest for %s", pin.Registry, pin)
	}
	return digest, nil
}
