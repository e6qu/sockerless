// Package config loads the Azure-dispatcher TOML config from
// `~/.sockerless/dispatcher-azure/config.toml`. Schema:
//
//	[[label]]
//	name             = "sockerless-aca"
//	subscription_id  = "00000000-0000-0000-0000-000000000000"
//	resource_group   = "sockerless-runners-rg"
//	environment      = "/subscriptions/.../managedEnvironments/sockerless-runners-env"
//	location         = "eastus2"
//	image            = "myacr.azurecr.io/runners/runner:latest"
//	managed_identity = "/subscriptions/.../userAssignedIdentities/runner-id"
//
// Mirror of the GCP config schema; replaces (project, region, service
// account) with (subscription, resource group, environment, location,
// managed identity).
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Label maps a runs-on label to an ACA Container Apps Environment +
// runner image.
type Label struct {
	Name            string `toml:"name"`
	SubscriptionID  string `toml:"subscription_id"`
	ResourceGroup   string `toml:"resource_group"`
	Environment     string `toml:"environment"`
	Location        string `toml:"location"`
	Image           string `toml:"image"`
	ManagedIdentity string `toml:"managed_identity"`
}

// Config is the on-disk dispatcher config.
type Config struct {
	Labels []Label `toml:"label"`
}

// DefaultPath returns the standard config path under
// `~/.sockerless/dispatcher-azure/config.toml`.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".sockerless", "dispatcher-azure", "config.toml"), nil
}

// Load reads the config from `path`. Returns an empty Config (not an
// error) if the file does not exist.
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
		if l.SubscriptionID == "" {
			return Config{}, fmt.Errorf("label %q: subscription_id is required", l.Name)
		}
		if l.ResourceGroup == "" {
			return Config{}, fmt.Errorf("label %q: resource_group is required", l.Name)
		}
		if l.Environment == "" {
			return Config{}, fmt.Errorf("label %q: environment is required", l.Name)
		}
		if l.Location == "" {
			return Config{}, fmt.Errorf("label %q: location is required", l.Name)
		}
		if l.Image == "" {
			return Config{}, fmt.Errorf("label %q: image is required", l.Name)
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
