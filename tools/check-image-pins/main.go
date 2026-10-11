// Command check-image-pins keeps the images this repository pulls off Docker
// Hub and current.
//
// Docker Hub limits anonymous pulls per address and the shared CI runners
// exhaust that limit, so every image a Dockerfile, workflow, compose file or
// script pulls is a copy of a Docker Hub image, pinned by tag and digest:
// public.ecr.aws/docker/library/<name> for official images and
// mirror.gcr.io/<repository> for the rest.
//
// Usage:
//
//	go run .              Fail on an image pulled from Docker Hub, or a Docker
//	                      Hub copy without a tag and digest. No network access.
//	go run . --freshness  Also compare every pin with Docker Hub:
//	                        ok     the pin is the digest Docker Hub serves for
//	                               its tag, and the mirror serves it too
//	                        HELD   Docker Hub moved the tag, or published a newer
//	                               release with the same variant and major
//	                               version, less than 24 hours ago (a release
//	                               Docker Hub has not pushed for 90 days is out
//	                               of support and does not count)
//	                        DRIFT  that change is at least 24 hours old, or the
//	                               mirror serves another digest: fails
//
// Run it from the repository root, or anywhere inside the repository.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	freshness := flag.Bool("freshness", false, "compare every pin with Docker Hub and its mirror")
	flag.Parse()
	if err := run(*freshness); err != nil {
		fmt.Fprintln(os.Stderr, "check-image-pins:", err)
		os.Exit(1)
	}
}

// git runs a git command on the working tree around the current directory.
// A hook inherits GIT_DIR and GIT_INDEX_FILE from the git process that runs
// it, which would point the command at another tree, so those are dropped.
func git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); name != "GIT_DIR" && name != "GIT_WORK_TREE" && name != "GIT_INDEX_FILE" {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	return cmd.Output()
}

func run(freshness bool) error {
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("find the repository root: %w", err)
	}
	root := strings.TrimSpace(string(top))
	listed, err := git("-C", root, "ls-files")
	if err != nil {
		return fmt.Errorf("list tracked files: %w", err)
	}
	files := map[string]string{}
	for _, name := range strings.Split(strings.TrimSpace(string(listed)), "\n") {
		if !scanned(name) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		files[name] = string(text)
	}
	pins, where, problems := collect(files)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, "  ", p)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d image reference(s) pull from Docker Hub or carry no digest", len(problems))
	}
	if len(pins) == 0 {
		return fmt.Errorf("found no pinned images; the check is not looking at anything")
	}
	if !freshness {
		fmt.Printf("check-image-pins: %d pinned images, none pulled from Docker Hub\n", len(pins))
		return nil
	}

	reg := &Registries{
		Client: &http.Client{Timeout: 20 * time.Second},
		Hub:    "https://hub.docker.com",
		ECR:    "https://public.ecr.aws",
		GCR:    "https://mirror.gcr.io",
	}
	now := time.Now()
	failed := 0
	for _, pin := range pins {
		label := fmt.Sprintf("%s (%s)", pin, strings.Join(where[pin], ", "))
		v, err := check(reg, pin, now)
		if err != nil {
			fmt.Printf("ERROR  %s: could not be checked: %v\n", label, err)
			failed++
			continue
		}
		fmt.Printf("%-6s %s\n", v.State, label)
		for _, note := range v.Notes {
			fmt.Printf("         %s\n", note)
		}
		if v.State == "DRIFT" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d image pin(s) are behind Docker Hub or could not be checked; bump each to the digest Docker Hub serves once the mirror serves the same one", failed)
	}
	fmt.Printf("All %d pinned images are current or held.\n", len(pins))
	return nil
}

// followed names the Docker Hub repositories whose release line this
// repository chooses rather than follows. A moved tag still fails; a newer
// release line is reported, not failed.
var followed = map[string]string{
	"library/golang": "Go stays on the 1.26 line the workspace declares (STATUS.md, Go toolchain): " +
		"CI's golangci-lint 2.12.2 is built with Go 1.26 and cannot load Go 1.27 packages",
}

func check(reg *Registries, pin Pin, now time.Time) (Verdict, error) {
	hubDigest, pushed, err := reg.HubTag(pin.Hub(), pin.Tag)
	if err != nil {
		return Verdict{}, err
	}
	mirror, err := reg.MirrorDigest(pin)
	if err != nil {
		return Verdict{}, err
	}
	candidates, err := reg.Candidates(pin.Hub(), pin.Tag)
	if err != nil {
		return Verdict{}, err
	}
	newer := newerReleases(pin.Tag, candidates, now)
	reason, chosen := followed[pin.Hub()]
	if !chosen || len(newer) == 0 {
		return judge(pin.Digest, hubDigest, pushed, mirror, newer, now), nil
	}
	v := judge(pin.Digest, hubDigest, pushed, mirror, nil, now)
	v.Notes = append(v.Notes, fmt.Sprintf("newer release %s not adopted: %s", newer[0].Tag, reason))
	return v, nil
}
