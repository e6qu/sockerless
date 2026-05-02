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
type Label struct {
	Name           string `toml:"name"`
	Project        string `toml:"gcp_project"`
	Region         string `toml:"gcp_region"`
	Image          string `toml:"image"`
	ServiceAccount string `toml:"service_account"`
	BuildBucket    string `toml:"build_bucket"` // GCS bucket for Cloud Build context (sockerless backend uses this for `docker build`)
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
		if l.BuildBucket == "" {
			return Config{}, fmt.Errorf("label %q: build_bucket is required (GCS bucket for sockerless backend's docker build context uploads)", l.Name)
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
