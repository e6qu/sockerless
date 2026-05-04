// github-runner-dispatcher-gcp polls GitHub Actions for queued
// workflow_jobs and creates one Cloud Run Job per queued job. The
// runner image's entrypoint registers with GitHub using
// `RUNNER_REG_TOKEN`, runs the job, and exits — Cloud Run terminates
// the execution on subprocess exit (one-shot semantics).
//
// Differs from `github-runner-dispatcher-aws` in only the spawner:
// there is no docker daemon, and dispatch goes directly to the GCP
// Cloud Run control plane via `cloud.google.com/go/run/apiv2`. The
// poller, scopes-check, registration-token mint, and seen-set dedup
// are all reused from the AWS dispatcher's module via the `replace
// github.com/sockerless/github-runner-dispatcher-aws =>
// ../github-runner-dispatcher-aws` in go.mod (sockerless-agnostic,
// GitHub-API-only code paths).
//
// Usage:
//
//	gh auth token | xargs -I{} \
//	  github-runner-dispatcher-gcp --repo owner/repo --token {}
//
// Same flag surface as the AWS dispatcher (`--repo`, `--token`,
// `--config`, `--once`, `--cleanup-only`); config schema documented
// in `internal/config/config.go`.
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
	"github.com/sockerless/github-runner-dispatcher-gcp/internal/config"
	"github.com/sockerless/github-runner-dispatcher-gcp/internal/spawner"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "github-runner-dispatcher-gcp: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	repo := flag.String("repo", "", "owner/repo (mandatory; no default)")
	token := flag.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT; default $GITHUB_TOKEN")
	configPath := flag.String("config", "", "path to dispatcher config.toml; default ~/.sockerless/dispatcher-gcp/config.toml")
	once := flag.Bool("once", false, "run a single poll cycle and exit (smoke / debug)")
	cleanupOnly := flag.Bool("cleanup-only", false, "run a single GC sweep (Cloud Run Jobs + GitHub runners) and exit; no polling")
	flag.Parse()

	// Cloud Run / serverless deployment: --repo and --token can come
	// from env (REPO + GITHUB_TOKEN) so the container starts with no
	// command-line args. The Cloud Run Service revision env variables
	// are how secrets ride in (Secret Manager → env binding).
	if *repo == "" {
		*repo = os.Getenv("REPO")
	}
	if *repo == "" || !strings.Contains(*repo, "/") {
		return fmt.Errorf("--repo owner/repo (or $REPO) is required (e.g. --repo e6qu/sockerless)")
	}
	if *token == "" {
		return fmt.Errorf("github token is empty — set $GITHUB_TOKEN, run `gh auth token | …`, or pass --token=…")
	}

	// $PORT is set by Cloud Run; the container must bind it within
	// the startup probe budget or the revision is killed. Tiny
	// /healthz responder; the polling loop runs in a goroutine.
	if port := os.Getenv("PORT"); port != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		go func() {
			log.Printf("dispatcher-gcp http listening on :%s", port)
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

	// Token verification: retry-with-backoff on 403 instead of exiting.
	// A 403 from GitHub on /user typically means GitHub's secondary
	// rate-limit / abuse protection is throttling the dispatcher's
	// egress IP — exiting and crashlooping makes it WORSE because each
	// container restart re-hits /user. Sleep through the abuse window
	// (5 min increasing to 30 min cap) until /user returns 200.
	verifyBackoff := 30 * time.Second
	for {
		if err := scopes.Verify(ctx, http.DefaultClient, *token); err != nil {
			log.Printf("scope verify failed (sleeping %s before retry): %v", verifyBackoff, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(verifyBackoff):
			}
			if verifyBackoff < 30*time.Minute {
				verifyBackoff *= 2
				if verifyBackoff > 30*time.Minute {
					verifyBackoff = 30 * time.Minute
				}
			}
			continue
		}
		break
	}
	log.Printf("dispatcher-gcp ready: repo=%s labels=%d once=%v cleanup-only=%v",
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

	ticker := time.NewTicker(gh.PollInterval())
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(2 * time.Minute)
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
	for _, job := range jobs {
		label := pickKnownLabel(job.Labels, d.cfg)
		if label == nil {
			log.Printf("skip job %d (%s): no matching dispatcher label in %v", job.JobID, job.Name, job.Labels)
			d.gh.Mark(job.JobID)
			continue
		}
		regToken, err := d.gh.MintRegistrationToken(ctx)
		if err != nil {
			log.Printf("skip job %d: mint registration token: %v", job.JobID, err)
			continue
		}
		runnerName := fmt.Sprintf("dispatcher-gcp-%d-%d", job.JobID, time.Now().Unix())
		fullName, err := spawner.Spawn(ctx, spawner.Request{
			Project:        label.Project,
			Region:         label.Region,
			Image:          label.Image,
			ServiceAccount: label.ServiceAccount,
			RegToken:       regToken,
			Repo:           job.Repo,
			RunnerName:     runnerName,
			Labels:         job.Labels,
			JobID:          job.JobID,
		})
		if err != nil {
			log.Printf("skip job %d: spawn (%s/%s): %v", job.JobID, label.Project, label.Region, err)
			continue
		}
		log.Printf("spawned Cloud Run Job for job %d (%s) at %s/%s: name=%s url=%s",
			job.JobID, label.Name, label.Project, label.Region, fullName, job.JobURL)
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

// RecoverState re-populates the seen-set from active Cloud Run Jobs
// across every (project, region) the config references. Run once at
// startup. A per-(project, region) error is logged and skipped — a
// transient API outage is normal and the next poll will re-check.
func (d *dispatchLoop) RecoverState(ctx context.Context) {
	now := d.gh.Now()
	seen := map[string]bool{}
	for _, label := range d.cfg.Labels {
		key := label.Project + "/" + label.Region
		if seen[key] {
			continue
		}
		seen[key] = true
		managed, err := spawner.ListManaged(ctx, label.Project, label.Region)
		if err != nil {
			log.Printf("recover: list managed on %s failed: %v", key, err)
			continue
		}
		for _, m := range managed {
			if m.JobID == 0 {
				continue
			}
			d.gh.Seen.Add(m.JobID, now)
			log.Printf("recover: seen-set restored for job %d (cloud-run-job %s, state=%s)",
				m.JobID, m.JobName, m.State)
		}
	}
}

// Cleanup deletes Cloud Run Jobs whose execution has terminated and
// whose `LabelManagedBy` matches the dispatcher. Cloud Run preserves
// completed Jobs indefinitely (default retention); without sweep,
// the project accumulates one Job resource per workflow_job. Same
// shape as the docker dispatcher's cleanup, just at the Cloud Run
// resource layer instead of `docker rm`.
func (d *dispatchLoop) Cleanup(ctx context.Context) error {
	seen := map[string]bool{}
	for _, label := range d.cfg.Labels {
		key := label.Project + "/" + label.Region
		if seen[key] {
			continue
		}
		seen[key] = true
		managed, err := spawner.ListManaged(ctx, label.Project, label.Region)
		if err != nil {
			log.Printf("cleanup: list managed on %s failed: %v", key, err)
			continue
		}
		for _, m := range managed {
			if !isTerminalJobState(m.State) {
				continue
			}
			if err := spawner.Delete(ctx, m.JobName); err != nil {
				log.Printf("cleanup: delete %s failed: %v", m.JobName, err)
				continue
			}
			log.Printf("cleanup: deleted terminated Cloud Run Job %s", m.JobName)
		}
	}
	return nil
}

// isTerminalJobState returns true for execution states that indicate
// the runner-task has finished. State strings come from spawner.
// executionStateForJob — values are EXECUTION_SUCCEEDED /
// EXECUTION_FAILED / EXECUTION_RUNNING / NO_EXECUTION. The legacy
// CONDITION_* strings (Cloud Run Job's TerminalCondition.State,
// reflecting Job-DEFINITION reconciliation, not execution outcome)
// are NOT treated as terminal — using them caused BUG-940 (cell 5
// runner-tasks deleted 80s after spawn while still bootstrapping).
func isTerminalJobState(state string) bool {
	switch state {
	case "EXECUTION_SUCCEEDED", "EXECUTION_FAILED":
		return true
	}
	return false
}
