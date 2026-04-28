package lambda

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	core "github.com/sockerless/backend-core"
)

// Config holds Lambda backend configuration.
type Config struct {
	Region           string
	RoleARN          string
	LogGroup         string
	MemorySize       int
	Timeout          int
	SubnetIDs        []string
	SecurityGroupIDs []string
	// AgentEFSID (optional) lets operators reuse an existing EFS filesystem
	// for Lambda volumes instead of sockerless provisioning a fresh one.
	// Mirrors SOCKERLESS_ECS_AGENT_EFS_ID on the ECS backend.
	AgentEFSID       string
	CodeBuildProject string        // AWS CodeBuild project for docker build
	BuildBucket      string        // S3 bucket for build context upload
	EndpointURL      string        // Custom endpoint URL
	PollInterval     time.Duration // Cloud API poll interval (default 2s)
	CallbackURL      string        // Reverse-agent callback URL injected into Lambda functions; must be reachable from Lambda (public or VPC endpoint). Empty => exec unsupported.

	// Overlay image build. Used when CallbackURL is set, to layer the
	// agent + bootstrap binaries on top of the user's requested image
	// so `docker exec` can reach a running invocation. Paths are
	// resolved against the running backend's binary environment
	// (typically a container image that bundles the binaries alongside
	// the backend).
	AgentBinaryPath     string // path to sockerless-agent; defaults to SOCKERLESS_AGENT_BINARY or /opt/sockerless/sockerless-agent
	BootstrapBinaryPath string // path to sockerless-lambda-bootstrap; defaults to SOCKERLESS_LAMBDA_BOOTSTRAP or /opt/sockerless/sockerless-lambda-bootstrap

	// PrebuiltOverlayImage, when non-empty, bypasses the
	// BuildAndPushOverlayImage call and uses this image URI directly.
	// Used by operators who pre-bake their own overlay images (e.g.
	// cached in ECR at deploy time, or built through a CI pipeline
	// rather than at container-create time). Also used by the
	// end-to-end test to exercise the reverse-agent path without
	// requiring insecure-registry config on the docker daemon.
	PrebuiltOverlayImage string

	// EnableCommit opts into the agent-driven `docker commit` path
	// (backends/core.CommitContainerViaAgent). Off by default because
	// the result isn't a traditional diff-against-base-image commit —
	// sockerless can't read the base image's rootfs from the backend
	// host, so the whole container filesystem becomes a single
	// new layer. Users who understand that tradeoff set
	// SOCKERLESS_ENABLE_COMMIT=1 and accept the larger image.
	EnableCommit bool

	// Architecture is the Lambda function architecture: "x86_64"
	// (default) or "arm64". The sockerless backend reports this value
	// (Docker-style: amd64 / arm64) via `docker info` so clients pull
	// single-arch images that actually run on the cloud workload —
	// sockerless's own host arch is irrelevant (client/server model:
	// Docker clients on any host arch report the *server* arch, and our
	// server is the cloud workload). Set via SOCKERLESS_LAMBDA_ARCHITECTURE.
	Architecture string

	// SharedVolumes mirrors the ECS backend's same-named field. When
	// sockerless runs inside a Lambda invocation that already has EFS
	// access points mounted at known paths (via FileSystemConfigs on
	// the Lambda function), and the runner inside the invocation does
	// `docker create -v /home/runner/_work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference
	// whose EFS access point is shared with the runner-Lambda. Sub-
	// tasks (spawned via the ECS backend, since Lambda can't easily
	// dispatch to itself recursively) mount the same access point.
	// Format identical to ECS: SOCKERLESS_LAMBDA_SHARED_VOLUMES=name=path=fsap-XXX[=fs-YYY],...
	SharedVolumes []SharedVolume
}

// SharedVolume describes a workspace volume mounted via EFS that the
// caller (the runner-Lambda) shares with sockerless. When docker
// create sees a bind mount whose source matches ContainerPath, the
// bind is rewritten to a named volume named Name backed by the EFS
// access point AccessPointID. Mirror of `ecs.SharedVolume`.
type SharedVolume struct {
	Name          string // logical volume name used in spawned sub-tasks
	ContainerPath string // path inside the calling container (= the bind-mount source)
	AccessPointID string // EFS access point ID (fsap-...)
	FileSystemID  string // EFS filesystem ID (fs-...); defaults to Config.AgentEFSID
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Region:               envOrDefault("AWS_REGION", "us-east-1"),
		RoleARN:              os.Getenv("SOCKERLESS_LAMBDA_ROLE_ARN"),
		LogGroup:             envOrDefault("SOCKERLESS_LAMBDA_LOG_GROUP", "/sockerless/lambda"),
		MemorySize:           envOrDefaultInt("SOCKERLESS_LAMBDA_MEMORY_SIZE", 1024),
		Timeout:              envOrDefaultInt("SOCKERLESS_LAMBDA_TIMEOUT", 900),
		SubnetIDs:            splitCSV(os.Getenv("SOCKERLESS_LAMBDA_SUBNETS")),
		SecurityGroupIDs:     splitCSV(os.Getenv("SOCKERLESS_LAMBDA_SECURITY_GROUPS")),
		AgentEFSID:           firstNonEmpty(os.Getenv("SOCKERLESS_LAMBDA_AGENT_EFS_ID"), os.Getenv("SOCKERLESS_AGENT_EFS_ID")),
		CodeBuildProject:     os.Getenv("SOCKERLESS_AWS_CODEBUILD_PROJECT"),
		BuildBucket:          os.Getenv("SOCKERLESS_AWS_BUILD_BUCKET"),
		EndpointURL:          os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:         parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		CallbackURL:          os.Getenv("SOCKERLESS_CALLBACK_URL"),
		AgentBinaryPath:      envOrDefault("SOCKERLESS_AGENT_BINARY", "/opt/sockerless/sockerless-agent"),
		BootstrapBinaryPath:  envOrDefault("SOCKERLESS_LAMBDA_BOOTSTRAP", "/opt/sockerless/sockerless-lambda-bootstrap"),
		PrebuiltOverlayImage: os.Getenv("SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE"),
		EnableCommit:         os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		Architecture:         os.Getenv("SOCKERLESS_LAMBDA_ARCHITECTURE"),
		SharedVolumes:        parseSharedVolumes(os.Getenv("SOCKERLESS_LAMBDA_SHARED_VOLUMES")),
	}
}

// parseSharedVolumes parses the SOCKERLESS_LAMBDA_SHARED_VOLUMES
// env-var (`name=containerPath=fsap-XXXX[=fs-YYYY],name2=...`).
// Mirror of `ecs.parseSharedVolumes`.
func parseSharedVolumes(s string) []SharedVolume {
	if s == "" {
		return nil
	}
	var out []SharedVolume
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "=")
		if len(parts) < 3 || len(parts) > 4 {
			continue
		}
		sv := SharedVolume{
			Name:          strings.TrimSpace(parts[0]),
			ContainerPath: strings.TrimSpace(parts[1]),
			AccessPointID: strings.TrimSpace(parts[2]),
		}
		if len(parts) == 4 {
			sv.FileSystemID = strings.TrimSpace(parts[3])
		}
		if sv.Name == "" || sv.ContainerPath == "" || sv.AccessPointID == "" {
			continue
		}
		out = append(out, sv)
	}
	return out
}

// LookupSharedVolumeBySourcePath returns the SharedVolume entry whose
// ContainerPath equals the given path, or nil if none matches.
func (c Config) LookupSharedVolumeBySourcePath(path string) *SharedVolume {
	for i := range c.SharedVolumes {
		if c.SharedVolumes[i].ContainerPath == path {
			return &c.SharedVolumes[i]
		}
	}
	return nil
}

// LookupSharedVolumeByName returns the SharedVolume entry whose Name
// equals the given volume name, or nil if none matches.
func (c Config) LookupSharedVolumeByName(name string) *SharedVolume {
	for i := range c.SharedVolumes {
		if c.SharedVolumes[i].Name == name {
			return &c.SharedVolumes[i]
		}
	}
	return nil
}

// isSubPathOfSharedVolume reports whether path is a strict sub-path
// (descendant) of any SharedVolume's ContainerPath.
func isSubPathOfSharedVolume(path string, vols []SharedVolume) bool {
	for i := range vols {
		base := vols[i].ContainerPath
		if base == "" {
			continue
		}
		if strings.HasPrefix(path, base+"/") {
			return true
		}
	}
	return false
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-east-1",
		LogGroup:     "/sockerless/lambda",
		MemorySize:   1024,
		Timeout:      900,
		PollInterval: 2 * time.Second,
	}
	if env.AWS != nil {
		if env.AWS.Region != "" {
			c.Region = env.AWS.Region
		}
		c.CodeBuildProject = env.AWS.CodeBuildProject
		c.BuildBucket = env.AWS.BuildBucket
		if l := env.AWS.Lambda; l != nil {
			c.RoleARN = l.RoleARN
			if l.LogGroup != "" {
				c.LogGroup = l.LogGroup
			}
			if l.MemorySize > 0 {
				c.MemorySize = l.MemorySize
			}
			if l.Timeout > 0 {
				c.Timeout = l.Timeout
			}
			c.SubnetIDs = l.Subnets
			c.SecurityGroupIDs = l.SecurityGroups
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	if env.Common.PollInterval != "" {
		c.PollInterval = parseDuration(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.RoleARN == "" {
		return fmt.Errorf("SOCKERLESS_LAMBDA_ROLE_ARN is required")
	}
	switch strings.ToLower(c.Architecture) {
	case "x86_64", "arm64":
		// ok
	default:
		return fmt.Errorf("SOCKERLESS_LAMBDA_ARCHITECTURE must be set to x86_64 or arm64 (no default — sockerless reports the cloud workload's architecture, not its own host arch); got %q", c.Architecture)
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func envOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
