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
		log.Printf("warning: no label entries in %s; every queued job will be skipped", cfgPath)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := scopes.Verify(ctx, http.DefaultClient, *token); err != nil {
		return err
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
		runnerName := fmt.Sprintf("dispatcher-azure-%d-%d", job.JobID, time.Now().Unix())
		jobARMID, err := spawner.Spawn(ctx, spawner.Request{
			SubscriptionID:  label.SubscriptionID,
			ResourceGroup:   label.ResourceGroup,
			Environment:     label.Environment,
			Location:        label.Location,
			Image:           label.Image,
			ManagedIdentity: label.ManagedIdentity,
			RegToken:        regToken,
			Repo:            job.Repo,
			RunnerName:      runnerName,
			Labels:          job.Labels,
			JobID:           job.JobID,
		})
		if err != nil {
			log.Printf("skip job %d: spawn (%s/%s): %v", job.JobID, label.SubscriptionID, label.ResourceGroup, err)
			continue
		}
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

func (d *dispatchLoop) Cleanup(ctx context.Context) error {
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
			if !isTerminalProvisioningState(m.State) {
				continue
			}
			if err := spawner.Delete(ctx, label.SubscriptionID, label.ResourceGroup, m.JobName); err != nil {
				log.Printf("cleanup: delete %s failed: %v", m.JobName, err)
				continue
			}
			log.Printf("cleanup: deleted terminated ACA Job %s", m.JobName)
		}
	}
	return nil
}

// isTerminalProvisioningState returns true for ACA Job
// `ProvisioningState` values that indicate the Job's last execution
// has ended (Job exists but isn't running anything). The Job
// resource itself is preserved by ACA after every execution; without
// sweep, the resource group accumulates one Job per workflow_job.
func isTerminalProvisioningState(state string) bool {
	switch strings.ToLower(state) {
	case "succeeded", "failed", "canceled":
		return true
	}
	return false
}
