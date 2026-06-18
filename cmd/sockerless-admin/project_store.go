package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SaveProject persists a project config to disk.
func SaveProject(dir string, cfg *ProjectConfig) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, cfg.Name+".json"), data, 0o644)
}

// LoadProjects reads all project configs from a directory.
func LoadProjects(dir string) ([]*ProjectConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var projects []*ProjectConfig
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			// A project config that can't be read must not silently vanish
			// from the admin's view.
			return nil, fmt.Errorf("read project config %s: %w", e.Name(), err)
		}
		var cfg ProjectConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse project config %s: %w", e.Name(), err)
		}
		projects = append(projects, &cfg)
	}
	return projects, nil
}

// DeleteProjectFile removes a persisted project config.
func DeleteProjectFile(dir, name string) error {
	return os.Remove(filepath.Join(dir, name+".json"))
}

// defaultProjectStoreDir returns the default project storage directory.
func defaultProjectStoreDir() string {
	return filepath.Join(sockerlessDir(), "admin", "projects")
}
