// Package config loads dispatcher TOML config from
// `~/.sockerless/dispatcher/config.toml`. Schema:
//
//	[[label]]
//	name        = "sockerless-ecs"
//	docker_host = "tcp://localhost:3375"
//	image       = "729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64"
//
//	[[label]]
//	name        = "sockerless-lambda"
//	docker_host = "tcp://localhost:3376"
//	image       = "729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64"
//
// `name` is matched against the `runs-on:` label on each queued
// workflow_job. CLI flags override individual entries; config file is
// optional (empty config means "no labels mapped — every job is
// skipped with a warning").
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Label maps a runs-on label to a docker daemon + runner image.
type Label struct {
	Name       string `toml:"name"`
	DockerHost string `toml:"docker_host"`
	Image      string `toml:"image"`
	// RunnerJobTimeout bounds the runner-task (seconds; default
	// 3600 per the dispatcher timeout-knob contract in
	// specs/CLOUD_RESOURCE_MAPPING.md). The docker primitive has no
	// native task-timeout field, so the dispatcher's GC sweep enforces
	// it: running dispatcher containers older than the bound are
	// stopped and removed.
	RunnerJobTimeout int `toml:"runner_job_timeout"`
	// MaxConcurrent caps live (running/created) runner containers on
	// this label's docker_host. 0 = unbounded. Queued jobs beyond the
	// cap stay queued and are retried on the next poll. The
	// ARC-equivalent knob is `maxRunners`.
	MaxConcurrent int `toml:"max_concurrent"`
}

// DefaultRunnerJobTimeout is the spec'd default bound (seconds) for a
// runner-task when the label doesn't set `runner_job_timeout`.
const DefaultRunnerJobTimeout = 3600

// Config is the on-disk dispatcher config.
type Config struct {
	Labels []Label `toml:"label"`
}

// DefaultPath returns the standard config path under
// `~/.sockerless/dispatcher/config.toml`. Errors when HOME is unset.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".sockerless", "dispatcher", "config.toml"), nil
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
		if l.DockerHost == "" {
			return Config{}, fmt.Errorf("label %q: docker_host is required", l.Name)
		}
		if l.Image == "" {
			return Config{}, fmt.Errorf("label %q: image is required", l.Name)
		}
		if l.RunnerJobTimeout < 0 {
			return Config{}, fmt.Errorf("label %q: runner_job_timeout must be positive", l.Name)
		}
		if l.RunnerJobTimeout == 0 {
			cfg.Labels[i].RunnerJobTimeout = DefaultRunnerJobTimeout
		}
		if l.MaxConcurrent < 0 {
			return Config{}, fmt.Errorf("label %q: max_concurrent must be >= 0 (0 = unbounded)", l.Name)
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
