package core

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DockerConfig represents the relevant parts of ~/.docker/config.json.
type DockerConfig struct {
	Auths map[string]DockerAuthEntry `json:"auths"`
}

// DockerAuthEntry is a single registry auth entry.
type DockerAuthEntry struct {
	Auth string `json:"auth"` // base64-encoded "user:pass"
}

// LoadDockerConfig parses a Docker config file. Returns an empty config
// (no error) if the file does not exist.
func LoadDockerConfig(path string) (*DockerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &DockerConfig{Auths: map[string]DockerAuthEntry{}}, nil
		}
		return nil, err
	}
	var cfg DockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Auths == nil {
		cfg.Auths = map[string]DockerAuthEntry{}
	}
	return &cfg, nil
}

// DefaultDockerConfigPath returns the default Docker config file path:
// $DOCKER_CONFIG/config.json or ~/.docker/config.json.
func DefaultDockerConfigPath() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".docker", "config.json")
}

// dockerHubAliases are the hostnames that all resolve to Docker Hub.
var dockerHubAliases = []string{
	"docker.io",
	"index.docker.io",
	"registry-1.docker.io",
	"https://index.docker.io/v1/",
}

// GetRegistryAuth returns credentials for a registry. It handles Docker Hub
// alias matching: docker.io, index.docker.io, registry-1.docker.io, and
// the legacy https://index.docker.io/v1/ URL are all treated as equivalent.
func (c *DockerConfig) GetRegistryAuth(registry string) (user, pass string, ok bool) {
	// Exact match first
	if entry, found := c.Auths[registry]; found {
		u, p, err := decodeAuth(entry.Auth)
		if err == nil {
			return u, p, true
		}
	}

	// Docker Hub alias matching
	if isDockerHub(registry) {
		for _, alias := range dockerHubAliases {
			if entry, found := c.Auths[alias]; found {
				u, p, err := decodeAuth(entry.Auth)
				if err == nil {
					return u, p, true
				}
			}
		}
	}

	return "", "", false
}

func isDockerHub(registry string) bool {
	for _, alias := range dockerHubAliases {
		if registry == alias {
			return true
		}
	}
	return false
}

func decodeAuth(encoded string) (string, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", os.ErrInvalid
	}
	return parts[0], parts[1], nil
}
