// Package config loads the GCP-dispatcher TOML config from
// `~/.sockerless/dispatcher-gcp/config.toml`. Schema:
//
//	[[label]]
//	name             = "sockerless-cloudrun"
//	gcp_project      = "my-project"
//	gcp_region       = "us-central1"
//	image            = "us-central1-docker.pkg.dev/my-project/runners/runner:latest"
//	service_account  = "github-runners@my-project.iam.gserviceaccount.com"
//
//	[[label]]
//	name             = "sockerless-gcf"
//	gcp_project      = "my-project"
//	gcp_region       = "us-central1"
//	image            = "us-central1-docker.pkg.dev/my-project/runners/runner-gcf:latest"
//	service_account  = "github-runners@my-project.iam.gserviceaccount.com"
//
// Same shape as the AWS dispatcher's config but with GCP-side
// addressing (project + region + service account) replacing the
// docker-host indirection. CLI flags override individual entries;
// config file is optional (empty config means "no labels mapped —
// every job is skipped with a warning").
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Label maps a runs-on label to a GCP project + region + runner image.
//
// All fields required (no optional vars, no fallbacks per project
// rule) — fail-loudly at config-load time if any are missing.
// Label maps a GitHub Actions runs-on label to a GCP project + region
// + runner image. Per CLOUD_RESOURCE_MAPPING.md "Adjustment to
// dispatcher scope", these are the ONLY config items the dispatcher
// needs — sockerless-shaped config (build bucket, workspace bucket,
// shared volumes) lives inside the runner image itself.
type Label struct {
	Name           string `toml:"name"`
	Project        string `toml:"gcp_project"`
	Region         string `toml:"gcp_region"`
	Image          string `toml:"image"`
	ServiceAccount string `toml:"service_account"`
	// RunnerWorkspaceBucket — when set, the dispatcher attaches
	// a Cloud Run native `Volume{Gcs{Bucket}}` mount at /tmp/runner-work
	// on the spawned runner-task. Required for GH actions/runner pattern
	// where step scripts written by the runner agent on the runner-task
	// need to be visible inside the JOB container's pod-Service via
	// sockerless's bind-mount → GCS-volume translation. Operator-side
	// infrastructure config (which bucket); the dispatcher itself stays
	// sockerless-unaware (it just provisions the mount).
	RunnerWorkspaceBucket string `toml:"runner_workspace_bucket"`
	// RunnerWorkspaceBacking — chooses how the runner-task shares its
	// workspace with the JOB pod-Service:
	//   - "gcs-fuse": legacy. Mount the GCS bucket directly on the
	//     runner-task via Cloud Run native Volume{Gcs}. Fast for
	//     whole-tar uploads (legacy tar-pack persist) but breaks GH
	//     actions/runner per-step rewrites of event.json — FUSE
	//     invalidates open handles when the object is rewritten.
	//   - "gcs-sync": pure GCS SDK, no FUSE. Runner-task keeps tmpfs at
	//     /tmp/runner-work; sockerless-backend tars + uploads per-exec;
	//     pod-Service bootstrap restores from GCS pre-subprocess + saves
	//     post-subprocess. Required for the per-step shared-workspace
	//     pattern.
	// Required when RunnerWorkspaceBucket is set — no automatic fallback
	// per the storage-backing no-fallbacks directive.
	RunnerWorkspaceBacking string `toml:"runner_workspace_backing"`
}

// Config is the on-disk dispatcher config.
type Config struct {
	Labels []Label `toml:"label"`
}

// DefaultPath returns the standard config path under
// `~/.sockerless/dispatcher-gcp/config.toml`. Errors when HOME is unset.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".sockerless", "dispatcher-gcp", "config.toml"), nil
}

// Load reads the config from `path`. Returns an empty Config (not an
// error) if the file does not exist — empty config is a valid state.
func Load(path string) (Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Config{}, nil
	} else if err != nil {
		return Config{}, fmt.Errorf("stat %s: %w", path, err)
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", path, err)
	}
	for i, l := range cfg.Labels {
		if l.Name == "" {
			return Config{}, fmt.Errorf("label entry #%d: name is required", i+1)
		}
		if l.Project == "" {
			return Config{}, fmt.Errorf("label %q: gcp_project is required", l.Name)
		}
		if l.Region == "" {
			return Config{}, fmt.Errorf("label %q: gcp_region is required", l.Name)
		}
		if l.Image == "" {
			return Config{}, fmt.Errorf("label %q: image is required", l.Name)
		}
		if l.ServiceAccount == "" {
			return Config{}, fmt.Errorf("label %q: service_account is required", l.Name)
		}
		if l.RunnerWorkspaceBucket != "" {
			switch l.RunnerWorkspaceBacking {
			case "gcs-fuse", "gcs-sync":
				// ok
			case "":
				return Config{}, fmt.Errorf("label %q: runner_workspace_backing is required when runner_workspace_bucket is set (no fallback — choose %q or %q)", l.Name, "gcs-fuse", "gcs-sync")
			default:
				return Config{}, fmt.Errorf("label %q: runner_workspace_backing %q invalid (must be %q or %q)", l.Name, l.RunnerWorkspaceBacking, "gcs-fuse", "gcs-sync")
			}
		}
	}
	return cfg, nil
}

// LookupLabel returns the Label entry matching `name`, or nil.
func (c Config) LookupLabel(name string) *Label {
	for i := range c.Labels {
		if c.Labels[i].Name == name {
			return &c.Labels[i]
		}
	}
	return nil
}
