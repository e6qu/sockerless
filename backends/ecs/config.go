package ecs

import (
	"fmt"
	"os"
	"strings"
	"time"

	core "github.com/sockerless/backend-core"
)

// Config holds ECS backend configuration.
type Config struct {
	Region           string
	Cluster          string
	Subnets          []string
	SecurityGroups   []string
	TaskRoleARN      string
	ExecutionRoleARN string
	LogGroup         string
	AgentEFSID       string // EFS filesystem ID for bind mount volumes
	AssignPublicIP   bool
	CodeBuildProject string // AWS CodeBuild project for docker build
	BuildBucket      string // S3 bucket for build context upload
	EndpointURL      string // Custom endpoint URL
	// CpuArchitecture maps to ECS RuntimePlatform.CpuArchitecture.
	// Valid: "X86_64" (Fargate default) or "ARM64" (Graviton). The
	// sockerless backend reports this value (Docker-style) via
	// `docker info` so clients pull single-arch images that actually
	// run on the cloud workload — sockerless's own host architecture
	// is irrelevant (client/server: the Docker client may run on any
	// arch, what matters is the server side).
	CpuArchitecture string
	PollInterval    time.Duration // Cloud API poll interval (default 2s)

	// SharedVolumes maps host bind-mount paths the calling docker
	// client sees (in its own container's filesystem) to EFS access
	// points already mounted in the calling task at the same path.
	// When sockerless runs as a sidecar (or single-container with
	// sockerless baked in) inside an ECS task that has EFS mounts at
	// e.g. `/home/runner/_work`, the runner inside the task does
	// `docker create -v /home/runner/_work:/__w alpine`. Without this
	// config, sockerless rejects the host bind mount because Fargate
	// has no host filesystem. With it, sockerless translates the bind
	// mount to a named volume reference whose EFS access point is
	// shared with the runner-task — both the runner-task and the
	// spawned sub-task see the same workspace via EFS.
	//
	// Format: SOCKERLESS_ECS_SHARED_VOLUMES="name1=containerPath1=fsap-XXXX[=efsFilesystemID],name2=containerPath2=fsap-YYYY[=efsFilesystemID]"
	// The trailing efsFilesystemID is optional — defaults to AgentEFSID.
	SharedVolumes []SharedVolume
}

// SharedVolume describes a workspace volume mounted via EFS that the
// caller (running in another ECS task) shares with sockerless. When
// docker create sees a bind mount whose source matches ContainerPath,
// the bind mount is rewritten to a named volume named Name backed by
// the EFS access point AccessPointID.
type SharedVolume struct {
	Name          string // logical volume name used in spawned sub-tasks
	ContainerPath string // path inside the calling container (= the bind-mount source)
	AccessPointID string // EFS access point ID (fsap-...)
	FileSystemID  string // EFS filesystem ID (fs-...); defaults to Config.AgentEFSID
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Region:           envOrDefault("AWS_REGION", "us-east-1"),
		Cluster:          envOrDefault("SOCKERLESS_ECS_CLUSTER", "sockerless"),
		Subnets:          splitCSV(os.Getenv("SOCKERLESS_ECS_SUBNETS")),
		SecurityGroups:   splitCSV(os.Getenv("SOCKERLESS_ECS_SECURITY_GROUPS")),
		TaskRoleARN:      os.Getenv("SOCKERLESS_ECS_TASK_ROLE_ARN"),
		ExecutionRoleARN: os.Getenv("SOCKERLESS_ECS_EXECUTION_ROLE_ARN"),
		LogGroup:         envOrDefault("SOCKERLESS_ECS_LOG_GROUP", "/sockerless"),
		AgentEFSID:       os.Getenv("SOCKERLESS_AGENT_EFS_ID"),
		AssignPublicIP:   os.Getenv("SOCKERLESS_ECS_PUBLIC_IP") == "true",
		CodeBuildProject: os.Getenv("SOCKERLESS_AWS_CODEBUILD_PROJECT"),
		BuildBucket:      os.Getenv("SOCKERLESS_AWS_BUILD_BUCKET"),
		EndpointURL:      os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		CpuArchitecture:  os.Getenv("SOCKERLESS_ECS_CPU_ARCHITECTURE"),
		PollInterval:     parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		SharedVolumes:    parseSharedVolumes(os.Getenv("SOCKERLESS_ECS_SHARED_VOLUMES")),
	}
}

// parseSharedVolumes parses the SOCKERLESS_ECS_SHARED_VOLUMES env-var
// shape (`name=containerPath=fsap-XXXX[=fs-YYYY],name2=...`) into a
// slice of SharedVolume entries. Returns nil for empty input.
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
// When sim is non-nil, EndpointURL is derived from the simulator port.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-east-1",
		Cluster:      "sockerless",
		LogGroup:     "/sockerless",
		PollInterval: 2 * time.Second,
	}
	if env.AWS != nil {
		if env.AWS.Region != "" {
			c.Region = env.AWS.Region
		}
		c.CodeBuildProject = env.AWS.CodeBuildProject
		c.BuildBucket = env.AWS.BuildBucket
		if ecs := env.AWS.ECS; ecs != nil {
			if ecs.Cluster != "" {
				c.Cluster = ecs.Cluster
			}
			c.Subnets = ecs.Subnets
			c.SecurityGroups = ecs.SecurityGroups
			c.TaskRoleARN = ecs.TaskRoleARN
			c.ExecutionRoleARN = ecs.ExecutionRoleARN
			if ecs.LogGroup != "" {
				c.LogGroup = ecs.LogGroup
			}
			c.AssignPublicIP = ecs.AssignPublicIP
			c.AgentEFSID = ecs.AgentEFSID
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
	if c.Cluster == "" {
		return fmt.Errorf("ECS cluster name is required")
	}
	if len(c.Subnets) == 0 {
		return fmt.Errorf("at least one subnet is required")
	}
	if c.ExecutionRoleARN == "" {
		return fmt.Errorf("execution role ARN is required")
	}
	switch strings.ToUpper(c.CpuArchitecture) {
	case "X86_64", "ARM64":
		// ok
	default:
		return fmt.Errorf("SOCKERLESS_ECS_CPU_ARCHITECTURE must be set to X86_64 or ARM64 (no default — sockerless reports the cloud workload's architecture, not its own host arch); got %q", c.CpuArchitecture)
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
