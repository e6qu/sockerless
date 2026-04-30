// github-runner-dispatcher is a laptop-foreground binary that polls
// GitHub Actions for queued workflow_jobs and spawns one runner
// container per queued job. The dispatcher is sockerless-agnostic —
// it talks to whichever Docker daemon `DOCKER_HOST` points at,
// including local Podman, Docker Desktop, or sockerless.
//
// Usage:
//
//	gh auth token | xargs -I{} \
//	  github-runner-dispatcher --repo owner/repo --token {}
//
// Per the Phase 110a spec:
//   - --repo is mandatory (no default; explicit-only).
//   - --token defaults to $GITHUB_TOKEN; missing → fail with guidance.
//   - Scopes verified at startup; missing → fail with `gh auth refresh`
//     instructions.
//   - Stateless 15-s polling loop; dedup via 5-min seen-set.
//   - Failure handling: log + skip; the next poll retries.
//   - Logs to stdout only.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sockerless/github-runner-dispatcher/internal/config"
	"github.com/sockerless/github-runner-dispatcher/internal/poller"
	"github.com/sockerless/github-runner-dispatcher/internal/scopes"
	"github.com/sockerless/github-runner-dispatcher/internal/spawner"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "github-runner-dispatcher: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	repo := flag.String("repo", "", "owner/repo (mandatory; no default)")
	token := flag.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT; default $GITHUB_TOKEN")
	configPath := flag.String("config", "", "path to dispatcher config.toml; default ~/.sockerless/dispatcher/config.toml")
	once := flag.Bool("once", false, "run a single poll cycle and exit (smoke / debug)")
	cleanupOnly := flag.Bool("cleanup-only", false, "run a single GC sweep (containers + GitHub runners) and exit; no polling")
	flag.Parse()

	if *repo == "" || !strings.Contains(*repo, "/") {
		return fmt.Errorf("--repo owner/repo is required (e.g. --repo e6qu/sockerless)")
	}
	if *token == "" {
		return fmt.Errorf("github token is empty — set $GITHUB_TOKEN, run `gh auth token | …`, or pass --token=…")
	}

	cfgPath := *configPath
	if cfgPath == "" {
		def, err := config.DefaultPath()
		if err != nil {
			return err
		}
		cfgPath = def
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config %s: %w", cfgPath, err)
	}
	if len(cfg.Labels) == 0 {
		log.Printf("warning: no label entries in %s; every queued job will be skipped (write a config.toml — see internal/config doc)", cfgPath)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := scopes.Verify(ctx, http.DefaultClient, *token); err != nil {
		return err
	}
	log.Printf("dispatcher ready: repo=%s labels=%d once=%v cleanup-only=%v",
		*repo, len(cfg.Labels), *once, *cleanupOnly)

	gh := poller.New(http.DefaultClient, *token, *repo)
	loop := newDispatchLoop(gh, cfg)

	// State recovery: rebuild the seen-set from running containers
	// labelled by a previous instance of the dispatcher. The dispatcher
	// is otherwise stateless — Docker is the source of truth for
	// "have I already spawned a runner for this job ID".
	loop.RecoverState(ctx)

	// One-shot GC mode: sweep containers + GitHub runners once and exit.
	// Useful both as a manual cleanup hook and as the cron-style
	// "drain dispatcher state before redeploy" entrypoint.
	if *cleanupOnly {
		return loop.Cleanup(ctx)
	}

	if *once {
		if err := loop.Step(ctx); err != nil {
			return err
		}
		return loop.Cleanup(ctx)
	}

	// Graceful shutdown: when the signal context fires, drain
	// in-flight runners + reap our GitHub runner registrations so
	// next-run state matches reality.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := loop.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	ticker := time.NewTicker(gh.PollInterval())
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(2 * time.Minute) // cheaper than a poll; reaps offline runners + dead containers
	defer cleanupTicker.Stop()
	for {
		if err := loop.Step(ctx); err != nil {
			log.Printf("poll error (continuing): %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cleanupTicker.C:
			if err := loop.Cleanup(ctx); err != nil {
				log.Printf("cleanup error (continuing): %v", err)
			}
		case <-ticker.C:
		}
	}
}

// dispatchLoop pulls one batch of queued jobs, spawns one runner
// per job, and marks each spawn as seen on success.
type dispatchLoop struct {
	gh  *poller.Client
	cfg config.Config
}

func newDispatchLoop(gh *poller.Client, cfg config.Config) *dispatchLoop {
	return &dispatchLoop{gh: gh, cfg: cfg}
}

func (d *dispatchLoop) Step(ctx context.Context) error {
	jobs, err := d.gh.PollOnce(ctx)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	for _, job := range jobs {
		label := pickKnownLabel(job.Labels, d.cfg)
		if label == nil {
			log.Printf("skip job %d (%s): no matching dispatcher label in %v", job.JobID, job.Name, job.Labels)
			d.gh.Mark(job.JobID) // dedup so the same skip doesn't repeat every cycle
			continue
		}
		if err := spawner.Liveness(ctx, label.DockerHost); err != nil {
			log.Printf("skip job %d (%s): docker daemon at %s unreachable: %v", job.JobID, label.Name, label.DockerHost, err)
			continue // do NOT mark — retry next cycle once the daemon is back
		}
		regToken, err := d.gh.MintRegistrationToken(ctx)
		if err != nil {
			log.Printf("skip job %d: mint registration token: %v", job.JobID, err)
			continue
		}
		runnerName := fmt.Sprintf("dispatcher-%d-%d", job.JobID, time.Now().Unix())
		cid, err := spawner.Spawn(ctx, spawner.Request{
			DockerHost: label.DockerHost,
			Image:      label.Image,
			RegToken:   regToken,
			Repo:       job.Repo,
			RunnerName: runnerName,
			Labels:     job.Labels,
			JobID:      job.JobID,
		})
		if err != nil {
			log.Printf("skip job %d: spawn: %v", job.JobID, err)
			continue
		}
		log.Printf("spawned runner for job %d (%s) on %s: container=%s name=%s url=%s",
			job.JobID, label.Name, label.DockerHost, shortID(cid), runnerName, job.JobURL)
		d.gh.Mark(job.JobID)
	}
	return nil
}

func pickKnownLabel(labels []string, cfg config.Config) *config.Label {
	for _, l := range labels {
		if got := cfg.LookupLabel(l); got != nil {
			return got
		}
	}
	return nil
}

func shortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// RecoverState re-populates the seen-set from running dispatcher
// containers across every configured docker_host. Run once at
// startup. Doesn't fail the dispatcher on a per-host error — a
// daemon being down at startup is normal (Liveness will skip it on
// the next poll), so we log and move on.
func (d *dispatchLoop) RecoverState(ctx context.Context) {
	now := d.gh.Now()
	for _, label := range d.cfg.Labels {
		managed, err := spawner.ListManaged(ctx, label.DockerHost)
		if err != nil {
			log.Printf("recover: list managed on %s failed: %v", label.DockerHost, err)
			continue
		}
		for _, m := range managed {
			if m.JobID == 0 {
				continue
			}
			if m.State == "running" || m.State == "created" {
				d.gh.Seen.Add(m.JobID, now)
				log.Printf("recover: seen-set restored for job %d (container %s, state=%s, host=%s)",
					m.JobID, shortID(m.ContainerID), m.State, label.DockerHost)
			}
		}
	}
}

// Cleanup is the dispatcher's GC sweep — runs alongside Step on a
// slower cadence (2 min) and once at startup. Two parts:
//
//  1. **Container reap**: across every configured docker_host, list
//     dispatcher-managed containers in `exited` / `dead` state and
//     `docker rm` them. `--rm` on `docker run` covers the happy path;
//     this catches the edge case where the runner image's entrypoint
//     died before `--rm` could fire (kernel OOM, daemon restart).
//
//  2. **GitHub-runner reap**: list registered runners, delete any
//     dispatcher-prefixed runner whose status is `offline` (the
//     container is gone but the registration lingers in the GitHub
//     UI). Idempotent — DELETE on an already-gone runner returns 404
//     which we treat as success.
//
// Errors are logged and swallowed; the next sweep retries. Don't
// crash the dispatcher because a single API call returned 5xx.
func (d *dispatchLoop) Cleanup(ctx context.Context) error {
	for _, label := range d.cfg.Labels {
		managed, err := spawner.ListManaged(ctx, label.DockerHost)
		if err != nil {
			log.Printf("cleanup: list managed on %s failed: %v", label.DockerHost, err)
			continue
		}
		for _, m := range managed {
			if m.State == "exited" || m.State == "dead" || m.State == "removing" {
				if err := spawner.StopAndRemove(ctx, label.DockerHost, m.ContainerID); err != nil {
					log.Printf("cleanup: rm %s on %s: %v", shortID(m.ContainerID), label.DockerHost, err)
					continue
				}
				log.Printf("cleanup: removed %s container %s (job=%d, runner=%s)",
					m.State, shortID(m.ContainerID), m.JobID, m.RunnerName)
			}
		}
	}

	runners, err := d.gh.ListRunners(ctx)
	if err != nil {
		log.Printf("cleanup: list github runners: %v", err)
		return nil
	}
	for _, r := range runners {
		if !poller.IsDispatcherRunner(r) {
			continue
		}
		// Reap offline dispatcher runners — the container is gone but
		// GitHub still has the registration. Don't reap online/busy
		// ones; those are running real jobs.
		if r.Status == "offline" {
			if err := d.gh.DeleteRunner(ctx, r.ID); err != nil {
				log.Printf("cleanup: delete runner %s (id=%d): %v", r.Name, r.ID, err)
				continue
			}
			log.Printf("cleanup: deleted offline runner %s (id=%d)", r.Name, r.ID)
		}
	}
	return nil
}

// Shutdown is the graceful-stop path. Stops every running
// dispatcher-managed container across every configured docker_host
// and deletes every dispatcher-prefixed GitHub runner. Called once
// when the dispatcher process receives SIGINT/SIGTERM. Best-effort —
// individual failures are logged but don't abort the rest of the
// teardown.
func (d *dispatchLoop) Shutdown(ctx context.Context) error {
	log.Printf("shutdown: draining dispatcher containers + runners")
	for _, label := range d.cfg.Labels {
		managed, err := spawner.ListManaged(ctx, label.DockerHost)
		if err != nil {
			log.Printf("shutdown: list managed on %s: %v", label.DockerHost, err)
			continue
		}
		for _, m := range managed {
			if err := spawner.StopAndRemove(ctx, label.DockerHost, m.ContainerID); err != nil {
				log.Printf("shutdown: stop %s on %s: %v", shortID(m.ContainerID), label.DockerHost, err)
				continue
			}
			log.Printf("shutdown: stopped %s (job=%d, runner=%s)", shortID(m.ContainerID), m.JobID, m.RunnerName)
		}
	}
	runners, err := d.gh.ListRunners(ctx)
	if err != nil {
		log.Printf("shutdown: list github runners: %v", err)
		return nil
	}
	for _, r := range runners {
		if !poller.IsDispatcherRunner(r) {
			continue
		}
		if err := d.gh.DeleteRunner(ctx, r.ID); err != nil {
			log.Printf("shutdown: delete runner %s (id=%d): %v", r.Name, r.ID, err)
			continue
		}
		log.Printf("shutdown: deleted runner %s (id=%d)", r.Name, r.ID)
	}
	return nil
}
