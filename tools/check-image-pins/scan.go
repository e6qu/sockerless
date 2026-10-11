package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// The files whose images a build, CI job, deployment or test harness pulls.
// Go sources are left out on purpose: the backends name public.ecr.aws in the
// code that rewrites workload images, and their tests feed the reference
// parsers made-up digests such as sha256:abc. Those are inputs, not pulls.
func scanned(file string) bool {
	base := path.Base(file)
	switch {
	case strings.Contains(base, "Dockerfile"):
		return true
	case file == ".gitlab-ci.yml":
		return true
	case strings.HasPrefix(file, ".github/workflows/") && strings.HasSuffix(file, ".yml"):
		return true
	case strings.HasPrefix(file, "deploy/") && (strings.HasSuffix(file, ".yaml") || strings.HasSuffix(file, ".yml")):
		return true
	case strings.HasSuffix(file, ".sh"), base == "Makefile", strings.HasSuffix(file, ".mk"):
		return true
	}
	return false
}

// pinPattern matches an image copied from Docker Hub onto ECR Public or
// Google's mirror, with or without a tag and digest.
var pinPattern = regexp.MustCompile(
	`(public\.ecr\.aws/docker/library|mirror\.gcr\.io)/([a-z0-9._/-]+)(?::([A-Za-z0-9_.-]+))?(@sha256:[0-9a-f]{64})?`)

// Pin is one Docker Hub copy pinned by tag and digest.
type Pin struct {
	Registry string // public.ecr.aws/docker/library or mirror.gcr.io
	Path     string // repository path on that registry
	Tag      string
	Digest   string
}

// Hub is the Docker Hub repository the pin copies.
func (p Pin) Hub() string {
	if p.Registry == "mirror.gcr.io" && strings.Contains(p.Path, "/") {
		return p.Path
	}
	return "library/" + p.Path
}

func (p Pin) String() string { return p.Registry + "/" + p.Path + ":" + p.Tag }

var (
	fromLine   = regexp.MustCompile(`(?mi)^FROM\s+(?:--platform=\S+\s+)?(\S+)(?:\s+AS\s+(\S+))?`)
	syntaxLine = regexp.MustCompile(`(?m)^#\s*syntax=(\S+)`)
	imageLine  = regexp.MustCompile(`(?m)^[ \t]*-?[ \t]*image:[ \t]*["']?([^\s"'#]+)`)
)

// Images built from this tree under a local name, never pulled.
var localImage = regexp.MustCompile(`^sockerless-deploy/[a-z0-9-]+:local$`)

// scanFile returns every pin in one file and every problem with how the file
// names its images: an image from Docker Hub, or a Docker Hub copy with no
// tag and digest.
func scanFile(file, text string) (pins []Pin, problems []string) {
	for _, m := range pinPattern.FindAllStringSubmatchIndex(text, -1) {
		ref := text[m[0]:m[1]]
		// A path cut short by a shell or template expression is a prefix the
		// script completes, such as public.ecr.aws/docker/library/${name}.
		if m[1] < len(text) && strings.ContainsRune("$<{", rune(text[m[1]])) {
			continue
		}
		if m[6] < 0 || m[8] < 0 {
			problems = append(problems, fmt.Sprintf("%s: %s has no tag and digest", file, ref))
			continue
		}
		pins = append(pins, Pin{
			Registry: text[m[2]:m[3]],
			Path:     text[m[4]:m[5]],
			Tag:      text[m[6]:m[7]],
			Digest:   text[m[8]+1 : m[9]],
		})
	}
	stages := map[string]bool{}
	for _, m := range fromLine.FindAllStringSubmatch(text, -1) {
		if p := dockerHub(m[1], stages); p != "" {
			problems = append(problems, fmt.Sprintf("%s: FROM %s %s", file, m[1], p))
		}
		if m[2] != "" {
			stages[strings.ToLower(m[2])] = true
		}
	}
	for _, m := range syntaxLine.FindAllStringSubmatch(text, -1) {
		if p := dockerHub(m[1], nil); p != "" {
			problems = append(problems, fmt.Sprintf("%s: syntax=%s %s", file, m[1], p))
		}
	}
	if strings.HasSuffix(file, ".yml") || strings.HasSuffix(file, ".yaml") {
		for _, m := range imageLine.FindAllStringSubmatch(text, -1) {
			if localImage.MatchString(m[1]) {
				continue
			}
			if p := dockerHub(m[1], nil); p != "" {
				problems = append(problems, fmt.Sprintf("%s: image %s %s", file, m[1], p))
			}
		}
	}
	return pins, problems
}

// dockerHub explains why an image reference is pulled from Docker Hub, or
// returns "" when it is not: a registry host, an earlier build stage, scratch,
// or a reference assembled from a build argument or workflow expression.
func dockerHub(ref string, stages map[string]bool) string {
	if ref == "scratch" || stages[strings.ToLower(ref)] || strings.ContainsAny(ref, "${") {
		return ""
	}
	host, _, found := strings.Cut(ref, "/")
	if found && (strings.ContainsAny(host, ".:") || host == "localhost") && host != "docker.io" && host != "index.docker.io" {
		return ""
	}
	return "is pulled from Docker Hub; use public.ecr.aws/docker/library/<name> or mirror.gcr.io/<repository>, pinned by tag and digest"
}

// collect scans every file and returns the distinct pins and all problems.
func collect(files map[string]string) ([]Pin, map[Pin][]string, []string) {
	where := map[Pin][]string{}
	var problems []string
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pins, p := scanFile(name, files[name])
		problems = append(problems, p...)
		for _, pin := range pins {
			if !contains(where[pin], name) {
				where[pin] = append(where[pin], name)
			}
		}
	}
	pins := make([]Pin, 0, len(where))
	for pin := range where {
		pins = append(pins, pin)
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].String()+pins[i].Digest < pins[j].String()+pins[j].Digest })
	return pins, where, problems
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
