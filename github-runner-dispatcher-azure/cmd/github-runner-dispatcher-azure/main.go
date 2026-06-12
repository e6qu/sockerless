// github-runner-dispatcher-azure polls GitHub Actions for queued
// workflow_jobs and creates one Azure Container Apps Job execution
// per queued job. Mirror of github-runner-dispatcher-gcp adapted to
// Azure's two-step ACA Jobs shape (Job is the template; JobExecution
// is the running instance).
//
// Same flag surface (`--repo`, `--token`, `--config`, `--once`,
// `--cleanup-only`) and reuses the upstream poller / scopes via the
// `replace github.com/sockerless/github-runner-dispatcher-aws` directive.
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

	"github.com/sockerless/github-runner-dispatcher-aws/pkg/poller"
	"github.com/sockerless/github-runner-dispatcher-aws/pkg/scopes"
	"github.com/sockerless/github-runner-dispatcher-azure/internal/config"
	"github.com/sockerless/github-runner-dispatcher-azure/internal/spawner"
)

// orphanGrace is how long a Job may sit with NO execution before the
// sweep reaps it. Covers the Spawn path where BeginStart failed and
// the Job was deliberately left for the sweep — without an age gate
// the sweep could race a Job between CreateOrUpdate and Start.
const orphanGrace = 15 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "github-runner-dispatcher-azure: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	repo := flag.String("repo", "", "owner/repo (mandatory; no default)")
	token := flag.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT; default $GITHUB_TOKEN")
	configPath := flag.String("config", "", "path to dispatcher config.toml; default ~/.sockerless/dispatcher-azure/config.toml")
	once := flag.Bool("once", false, "run a single poll cycle and exit (smoke / debug)")
	cleanupOnly := flag.Bool("cleanup-only", false, "run a single GC sweep (ACA Jobs + GitHub runners) and exit; no polling")
	flag.Parse()

	// ACA / serverless deployment: --repo and --token can come from env
	// (REPO + GITHUB_TOKEN) so the container starts with no command-line
	// args. ACA secret bindings are how the PAT rides in.
	if *repo == "" {
		*repo = os.Getenv("REPO")
	}
	if *repo == "" || !strings.Contains(*repo, "/") {
		return fmt.Errorf("--repo owner/repo (or $REPO) is required (e.g. --repo e6qu/sockerless)")
	}
	if *token == "" {
		return fmt.Errorf("github token is empty — set $GITHUB_TOKEN, run `gh auth token | …`, or pass --token=…")
	}

	// $PORT (ACA ingress targetPort convention) → tiny /healthz
	// responder so a deployed dispatcher passes container probes; the
	// polling loop runs in the main goroutine.
	if port := os.Getenv("PORT"); port != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		go func() {
			log.Printf("dispatcher-azure http listening on :%s", port)
			if err := http.ListenAndServe(":"+port, mux); err != nil {
				log.Fatalf("http listen: %v", err)
			}
		}()
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
		log.Printf("warning: no label entries in %s; every queued job will be skipped", cfgPath)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Token verification: retry-with-backoff on 403/429, honoring
	// upstream rate hints with the standard +10% +1s buffer. A deployed
	// dispatcher that exits on a transient 429 crashloops straight back
	// into the abuse window — sleeping wins. Same loop as the GCP
	// dispatcher.
	verifyBackoff := 30 * time.Second
	for {
		err := scopes.Verify(ctx, http.DefaultClient, *token)
		if err == nil {
			break
		}
		wait := verifyBackoff
		if rle, ok := scopes.AsRateLimit(err); ok && rle.Wait > 0 {
			wait = rle.Wait
		}
		log.Printf("scope verify failed (sleeping %s before retry): %v", wait, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		if verifyBackoff < 30*time.Minute {
			verifyBackoff *= 2
			if verifyBackoff > 30*time.Minute {
				verifyBackoff = 30 * time.Minute
			}
		}
	}
	log.Printf("dispatcher-azure ready: repo=%s labels=%d once=%v cleanup-only=%v",
		*repo, len(cfg.Labels), *once, *cleanupOnly)

	gh := poller.New(http.DefaultClient, *token, *repo)
	loop := newDispatchLoop(gh, cfg)

	loop.RecoverState(ctx)

	if *cleanupOnly {
		return loop.Cleanup(ctx)
	}
	if *once {
		if err := loop.Step(ctx); err != nil {
			return err
		}
		return loop.Cleanup(ctx)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := loop.Cleanup(shutdownCtx); err != nil {
			log.Printf("shutdown cleanup error: %v", err)
		}
	}()

	cleanupTicker := time.NewTicker(2 * time.Minute)
	defer cleanupTicker.Stop()
	pollEvery := gh.PollInterval()
	// rateLimitedUntil tracks the wall-clock time before which Step()
	// must not be called again — honored across BOTH the poll tick and
	// the cleanup tick (cleanup's GitHub-runner reap also costs quota).
	var rateLimitedUntil time.Time
	for {
		nextPoll := pollEvery
		if !time.Now().Before(rateLimitedUntil) {
			if err := loop.Step(ctx); err != nil {
				if wait, ok := poller.AsRateLimit(err); ok {
					log.Printf("poll error: rate-limited, sleeping %s (upstream reset + 10%% + 1s): %v", wait, err)
					nextPoll = wait
					rateLimitedUntil = time.Now().Add(wait)
				} else {
					log.Printf("poll error (continuing): %v", err)
				}
			}
		} else {
			nextPoll = time.Until(rateLimitedUntil)
			if nextPoll < pollEvery {
				nextPoll = pollEvery
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-cleanupTicker.C:
			if err := loop.Cleanup(ctx); err != nil {
				log.Printf("cleanup error (continuing): %v", err)
			}
		case <-time.After(nextPoll):
		}
	}
}

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
	// Live (non-terminal) Job counts per (subscription, resource group),
	// fetched lazily for labels with a max_concurrent cap and updated as
	// this cycle spawns.
	liveCounts := map[string]int{}
	for _, job := range jobs {
		label := pickKnownLabel(job.Labels, d.cfg)
		if label == nil {
			log.Printf("skip job %d (%s): no matching dispatcher label in %v", job.JobID, job.Name, job.Labels)
			d.gh.Mark(job.JobID)
			continue
		}
		scope := label.SubscriptionID + "/" + label.ResourceGroup
		if label.MaxConcurrent > 0 {
			if _, ok := liveCounts[scope]; !ok {
				managed, err := spawner.ListManaged(ctx, label.SubscriptionID, label.ResourceGroup)
				if err != nil {
					log.Printf("skip job %d: live-count on %s: %v", job.JobID, scope, err)
					continue // do NOT mark — retry next cycle
				}
				n := 0
				for _, m := range managed {
					if !isTerminalState(m.State) {
						n++
					}
				}
				liveCounts[scope] = n
			}
			if liveCounts[scope] >= label.MaxConcurrent {
				log.Printf("defer job %d (%s): %d live runner jobs >= max_concurrent %d on %s",
					job.JobID, label.Name, liveCounts[scope], label.MaxConcurrent, scope)
				continue // job stays queued; next poll retries
			}
		}
		regToken, err := d.gh.MintRegistrationToken(ctx)
		if err != nil {
			log.Printf("skip job %d: mint registration token: %v", job.JobID, err)
			continue
		}
		runnerName := fmt.Sprintf("dispatcher-azure-%d-%d", job.JobID, time.Now().Unix())
		jobARMID, err := spawner.Spawn(ctx, spawner.Request{
			SubscriptionID:    label.SubscriptionID,
			ResourceGroup:     label.ResourceGroup,
			Environment:       label.Environment,
			Location:          label.Location,
			Image:             label.Image,
			ManagedIdentity:   label.ManagedIdentity,
			RegToken:          regToken,
			Repo:              job.Repo,
			RunnerName:        runnerName,
			Labels:            job.Labels,
			JobID:             job.JobID,
			JobTimeoutSeconds: label.RunnerJobTimeout,
		})
		if err != nil {
			log.Printf("skip job %d: spawn (%s/%s): %v", job.JobID, label.SubscriptionID, label.ResourceGroup, err)
			continue
		}
		liveCounts[scope]++
		log.Printf("spawned ACA Job for job %d (%s) in %s/%s: armid=%s url=%s",
			job.JobID, label.Name, label.SubscriptionID, label.ResourceGroup, jobARMID, job.JobURL)
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

func (d *dispatchLoop) RecoverState(ctx context.Context) {
	now := d.gh.Now()
	seen := map[string]bool{}
	for _, label := range d.cfg.Labels {
		key := label.SubscriptionID + "/" + label.ResourceGroup
		if seen[key] {
			continue
		}
		seen[key] = true
		managed, err := spawner.ListManaged(ctx, label.SubscriptionID, label.ResourceGroup)
		if err != nil {
			log.Printf("recover: list managed on %s failed: %v", key, err)
			continue
		}
		for _, m := range managed {
			if m.JobID == 0 {
				continue
			}
			d.gh.Seen.Add(m.JobID, now)
			log.Printf("recover: seen-set restored for job %d (aca-job %s, state=%s)",
				m.JobID, m.JobName, m.State)
		}
	}
}

// Cleanup is the dispatcher's GC sweep. Three parts, mirroring the
// GCP + AWS loops:
//
//  1. Delete ACA Jobs whose latest EXECUTION is terminal. Keyed off
//     execution state, never `ProvisioningState` — the resource's
//     provisioning reads `Succeeded` right after create while the
//     execution is still running, and deleting the Job kills it.
//  2. Delete Jobs that never got an execution (BeginStart failed)
//     once they're older than orphanGrace.
//  3. Deregister offline `dispatcher-*` runners on the GitHub side —
//     ephemeral runners that died without completing leave zombie
//     registrations behind.
func (d *dispatchLoop) Cleanup(ctx context.Context) error {
	now := time.Now()
	seen := map[string]bool{}
	for _, label := range d.cfg.Labels {
		key := label.SubscriptionID + "/" + label.ResourceGroup
		if seen[key] {
			continue
		}
		seen[key] = true
		managed, err := spawner.ListManaged(ctx, label.SubscriptionID, label.ResourceGroup)
		if err != nil {
			log.Printf("cleanup: list managed on %s failed: %v", key, err)
			continue
		}
		for _, m := range managed {
			if !shouldReap(m, now) {
				continue
			}
			if err := spawner.Delete(ctx, label.SubscriptionID, label.ResourceGroup, m.JobName); err != nil {
				log.Printf("cleanup: delete %s failed: %v", m.JobName, err)
				continue
			}
			log.Printf("cleanup: deleted ACA Job %s (state=%s)", m.JobName, m.State)
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

// shouldReap decides whether the sweep may delete a managed Job.
// Terminal executions always reap; execution-less Jobs reap only past
// the orphan grace (and only when their creation time is known —
// missing SystemData means partial information, never delete on
// that).
func shouldReap(m spawner.Managed, now time.Time) bool {
	if isTerminalState(m.State) {
		return true
	}
	if m.State == spawner.StateNoExecution {
		return !m.CreatedAt.IsZero() && now.Sub(m.CreatedAt) > orphanGrace
	}
	return false
}

// isTerminalState reports whether the runner-task's latest execution
// has finished. State strings come from spawner.ClassifyExecutions.
func isTerminalState(state string) bool {
	switch state {
	case spawner.StateExecutionSucceeded, spawner.StateExecutionFailed:
		return true
	}
	return false
}
