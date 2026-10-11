package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	pinned = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	moved  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var now = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func TestScanFindsPinsAndRefusesDockerHub(t *testing.T) {
	files := map[string]string{
		"Dockerfile": "# syntax=mirror.gcr.io/docker/dockerfile:1@" + pinned + "\n" +
			"FROM public.ecr.aws/docker/library/golang:1.27.2-alpine@" + pinned + " AS build\n" +
			"FROM build AS test\n" +
			"FROM gcr.io/distroless/static-debian12:nonroot\n" +
			"FROM scratch\n",
		".github/workflows/ci.yml": "    container:\n      image: ${{ matrix.image }}\n" +
			"    services:\n      db:\n        image: postgres:16-alpine\n" +
			"    strategy:\n      matrix:\n        image:\n          - { name: a }\n",
		"deploy/compose.yaml": "    image: sockerless-deploy/shauth:local\n    image: ghcr.io/e6qu/shauth:${TAG}\n",
		"scripts/pull.sh":     "docker pull public.ecr.aws/docker/library/alpine:latest\nname=public.ecr.aws/docker/library/${image}\n",
		"Other.Dockerfile":    "# syntax=docker/dockerfile:1\nFROM docker.io/library/alpine:3.24\n",
	}
	pins, where, problems := collect(files)
	if len(pins) != 2 || len(where[pins[0]]) != 1 {
		t.Fatalf("pins = %v", pins)
	}
	if got := pins[0].Hub(); got != "docker/dockerfile" {
		t.Errorf("mirror.gcr.io pin copies %q", got)
	}
	if got := pins[1].Hub(); got != "library/golang" {
		t.Errorf("ECR Public pin copies %q", got)
	}
	want := []string{
		".github/workflows/ci.yml: image postgres:16-alpine",
		"Other.Dockerfile: FROM docker.io/library/alpine:3.24",
		"Other.Dockerfile: syntax=docker/dockerfile:1",
		"scripts/pull.sh: public.ecr.aws/docker/library/alpine:latest has no tag and digest",
	}
	if len(problems) != len(want) {
		t.Fatalf("problems = %q", problems)
	}
	for i, w := range want {
		if !strings.HasPrefix(problems[i], w) {
			t.Errorf("problem %d = %q, want prefix %q", i, problems[i], w)
		}
	}
}

func TestScannedFilesLeaveGoSourcesOut(t *testing.T) {
	for file, want := range map[string]bool{
		"smoke-tests/Dockerfile.ecs":             true,
		".github/workflows/ci.yml":               true,
		"deploy/compose.yaml":                    true,
		".gitlab-ci.yml":                         true,
		"tests/terraform-integration/run.sh":     true,
		"Makefile":                               true,
		"make/go-app.mk":                         true,
		"backends/core/registry_test.go":         false,
		"backends/ecs/image_resolve.go":          false,
		"specs/CLOUD_RESOURCE_MAPPING.md":        false,
		"terraform/modules/ecs/test/main.tf":     false,
		"ui/packages/admin/src/images.json":      false,
		".github/required-status-checks.txt":     false,
		"deploy/README.md":                       false,
		"tests/runners/github/dockerfile/README": false,
	} {
		if got := scanned(file); got != want {
			t.Errorf("scanned(%q) = %v, want %v", file, got, want)
		}
	}
}

func release(tag string, age time.Duration) Release { return Release{tag, now.Add(-age)} }

func TestNewerReleasesKeepVariantAndMajorVersion(t *testing.T) {
	day := 24 * time.Hour
	candidates := []Release{
		release("1.27.3-bookworm", day), release("1.27.3-alpine", day), release("1.28.0-bookworm", day),
		release("2.0.0-bookworm", day), release("1.27-bookworm", day), release("1.27.1-bookworm", day),
		release("1.29.0-bookworm", 91*day),
	}
	var got []string
	for _, r := range newerReleases("1.27.2-bookworm", candidates, now) {
		got = append(got, r.Tag)
	}
	if fmt.Sprint(got) != "[1.28.0-bookworm 1.27.3-bookworm]" {
		t.Errorf("newer = %v", got)
	}
	if r := newerReleases("latest", candidates, now); r != nil {
		t.Errorf("latest has newer releases %v", r)
	}
	if r := newerReleases("v0.33.0", []Release{release("v0.34.0", day), release("0.35.0", day)}, now); len(r) != 1 || r[0].Tag != "v0.34.0" {
		t.Errorf("v-prefixed newer = %v", r)
	}
}

func TestJudgeHoldsForTwentyFourHours(t *testing.T) {
	old := now.Add(-quarantine)
	young := old.Add(time.Nanosecond)
	cases := []struct {
		name        string
		hub, mirror string
		pushed      time.Time
		newer       []Release
		want        string
	}{
		{"current", pinned, pinned, old, nil, "ok"},
		{"moved tag inside quarantine", moved, pinned, young, nil, "HELD"},
		{"moved tag after quarantine", moved, pinned, old, nil, "DRIFT"},
		{"newer release inside quarantine", pinned, pinned, old, []Release{{"1.27.3", young}}, "HELD"},
		{"newer release after quarantine", pinned, pinned, old, []Release{{"1.27.3", old}}, "DRIFT"},
		{"mirror serves another digest", pinned, moved, old, nil, "DRIFT"},
	}
	for _, c := range cases {
		if got := judge(pinned, c.hub, c.pushed, c.mirror, c.newer, now).State; got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestRegistriesReadHubAndSpaceECRPublicRequests(t *testing.T) {
	var ecrTimes []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/repositories/library/postgres/tags/17.11-alpine":
			fmt.Fprintf(w, `{"name":"17.11-alpine","digest":%q,"tag_last_pushed":"2026-10-10T10:23:34.98928Z"}`, pinned)
		case "/v2/repositories/library/postgres/tags":
			if r.URL.Query().Get("name") != "17." {
				t.Errorf("candidate filter %q", r.URL.Query().Get("name"))
			}
			fmt.Fprint(w, `{"results":[{"name":"17.12-alpine","tag_last_pushed":"2026-10-11T11:00:00Z"}]}`)
		case "/token/":
			ecrTimes = append(ecrTimes, time.Now())
			fmt.Fprint(w, `{"token":"t"}`)
		case "/v2/docker/library/postgres/manifests/17.11-alpine":
			ecrTimes = append(ecrTimes, time.Now())
			if r.Header.Get("Authorization") != "Bearer t" || r.Method != http.MethodHead {
				t.Errorf("manifest request %s %q", r.Method, r.Header.Get("Authorization"))
			}
			w.Header().Set("Docker-Content-Digest", pinned)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	reg := &Registries{Client: srv.Client(), Hub: srv.URL, ECR: srv.URL, GCR: srv.URL}
	pin := Pin{"public.ecr.aws/docker/library", "postgres", "17.11-alpine", pinned}
	v, err := check(reg, pin, now)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "HELD" {
		t.Errorf("verdict %+v", v)
	}
	if len(ecrTimes) != 2 || ecrTimes[1].Sub(ecrTimes[0]) < ecrSpacing {
		t.Errorf("ECR Public requests at %v, want %v apart", ecrTimes, ecrSpacing)
	}
	if _, _, err := reg.HubTag("library/missing", "1"); err == nil {
		t.Error("a missing Docker Hub tag did not fail")
	}
}
