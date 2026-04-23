package ecs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// containerInput groups the data needed to build one ECS container definition.
type containerInput struct {
	ID        string
	Container *api.Container
	IsMain    bool // true = primary container in a pod
}

// buildContainerDef builds a single ECS container definition.
// Named-volume mounts are resolved to EFS access points against the
// sockerless-managed filesystem.
func (s *Server) buildContainerDef(ctx context.Context, ci containerInput) (ecstypes.ContainerDefinition, []ecstypes.Volume, error) {
	config := ci.Container.Config

	// Build environment variables
	var envVars []ecstypes.KeyValuePair
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars = append(envVars, ecstypes.KeyValuePair{
				Name:  aws.String(parts[0]),
				Value: aws.String(parts[1]),
			})
		}
	}

	var entrypoint, command []string
	if len(config.Entrypoint) > 0 {
		entrypoint = config.Entrypoint
	}
	if len(config.Cmd) > 0 {
		command = config.Cmd
	}

	// Container name: "main" for the primary, sanitized name for sidecars
	defName := "main"
	if !ci.IsMain {
		defName = sanitizeContainerName(ci.Container.Name)
	}

	// Resolve the user-supplied image reference to an ECR URI so
	// Fargate can pull it. Already-ECR URIs pass through; failures
	// fall back to the raw ref and Fargate surfaces the pull error.
	// Unit tests may run without aws clients wired — skip resolution
	// and pass the ref through verbatim in that case.
	image := config.Image
	if s.aws != nil && s.aws.ECR != nil {
		if resolved, err := s.resolveImageURI(context.Background(), image); err == nil {
			image = resolved
		} else {
			s.Logger.Warn().Err(err).Str("image", config.Image).Msg("image URI resolution failed; Fargate may not be able to pull")
		}
	}

	containerDef := ecstypes.ContainerDefinition{
		Name:        aws.String(defName),
		Image:       aws.String(image),
		Essential:   aws.Bool(ci.IsMain),
		EntryPoint:  entrypoint,
		Command:     command,
		Environment: envVars,
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         s.config.LogGroup,
				"awslogs-region":        s.config.Region,
				"awslogs-stream-prefix": ci.ID[:12],
			},
		},
	}

	if config.Tty {
		containerDef.PseudoTerminal = aws.Bool(true)
	}
	if config.OpenStdin {
		containerDef.Interactive = aws.Bool(true)
	}

	if config.WorkingDir != "" {
		containerDef.WorkingDirectory = aws.String(config.WorkingDir)
	}

	if config.User != "" {
		containerDef.User = aws.String(config.User)
	}

	// ECS rejects ContainerDefinition.DnsSearchDomains for
	// awsvpc mode. Wrap the user's command in a /bin/sh shim that rewrites
	// etc/resolv.conf to add the per-network Cloud Map namespaces as DNS
	// search domains, preserving VPC DNS nameservers, then exec's the
	// original argv. Only applied when the container is on at least one
	// user-defined network and has an explicit entrypoint or command (so we
	// have something to exec — image-default CMD without explicit override
	// is left alone since we can't reconstruct argv without the image
	// manifest, and Fargate would have to re-pull just to read it).
	if domains := s.searchDomainsForContainer(ci.Container); len(domains) > 0 {
		origArgv := append([]string{}, entrypoint...)
		origArgv = append(origArgv, command...)
		if len(origArgv) > 0 {
			searchLine := "search " + strings.Join(domains, " ")
			script := fmt.Sprintf(
				"{ awk '/^nameserver /' /etc/resolv.conf; printf '%s\\n'; } > /tmp/.skls-resolv && cat /tmp/.skls-resolv > /etc/resolv.conf 2>/dev/null; exec %s",
				searchLine,
				shellQuoteArgs(origArgv),
			)
			containerDef.EntryPoint = []string{"/bin/sh", "-c"}
			containerDef.Command = []string{script}
		}
	}

	// Resolve each named-volume bind spec to an EFS access point on
	// the sockerless-managed filesystem. ContainerCreate has already
	// rejected host-path bind specs (`/h:/c`); everything left is
	// `volName:/mnt[:ro]`. Each volume gets one ECS Volume + one
	// MountPoint entry; the task role needs
	// `elasticfilesystem:ClientMount/ClientWrite` to actually mount.
	var volumes []ecstypes.Volume
	var mountPoints []ecstypes.MountPoint
	if len(ci.Container.HostConfig.Binds) > 0 {
		fsID, err := s.ensureEFSFilesystem(ctx)
		if err != nil {
			return containerDef, nil, fmt.Errorf("ensure EFS filesystem: %w", err)
		}
		for _, bind := range ci.Container.HostConfig.Binds {
			parts := strings.SplitN(bind, ":", 3)
			if len(parts) < 2 {
				continue
			}
			volName := parts[0]
			apID, err := s.accessPointForVolume(ctx, volName)
			if err != nil {
				return containerDef, nil, fmt.Errorf("resolve access point for volume %q: %w", volName, err)
			}
			volumes = append(volumes, ecstypes.Volume{
				Name: aws.String(volName),
				EfsVolumeConfiguration: &ecstypes.EFSVolumeConfiguration{
					FileSystemId:      aws.String(fsID),
					TransitEncryption: ecstypes.EFSTransitEncryptionEnabled,
					AuthorizationConfig: &ecstypes.EFSAuthorizationConfig{
						AccessPointId: aws.String(apID),
						Iam:           ecstypes.EFSAuthorizationConfigIAMEnabled,
					},
				},
			})
			readOnly := false
			if len(parts) == 3 && parts[2] == "ro" {
				readOnly = true
			}
			mountPoints = append(mountPoints, ecstypes.MountPoint{
				SourceVolume:  aws.String(volName),
				ContainerPath: aws.String(parts[1]),
				ReadOnly:      aws.Bool(readOnly),
			})
		}
	}

	containerDef.MountPoints = mountPoints

	return containerDef, volumes, nil
}

// registerTaskDefinition creates an ECS task definition from one or more containers.
func (s *Server) registerTaskDefinition(ctx context.Context, containers []containerInput) (string, error) {
	var allDefs []ecstypes.ContainerDefinition
	var allVolumes []ecstypes.Volume

	for _, ci := range containers {
		def, vols, err := s.buildContainerDef(ctx, ci)
		if err != nil {
			return "", err
		}
		allDefs = append(allDefs, def)
		allVolumes = append(allVolumes, vols...)
	}

	// Family name uses the first (main) container ID
	family := fmt.Sprintf("sockerless-%s", containers[0].ID[:12])

	tags := core.TagSet{
		ContainerID: containers[0].ID,
		Backend:     "ecs",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	// Map Docker resource constraints to valid Fargate CPU/memory.
	cpu, mem := fargateResources(containers)

	input := &awsecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(family),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String(cpu),
		Memory:                  aws.String(mem),
		ContainerDefinitions:    allDefs,
		Volumes:                 allVolumes,
		Tags:                    mapToECSTags(tags.AsMap()),
	}

	if s.config.ExecutionRoleARN != "" {
		input.ExecutionRoleArn = aws.String(s.config.ExecutionRoleARN)
	}
	if s.config.TaskRoleARN != "" {
		input.TaskRoleArn = aws.String(s.config.TaskRoleARN)
	}

	result, err := s.aws.ECS.RegisterTaskDefinition(ctx, input)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.TaskDefinition.TaskDefinitionArn), nil
}

// fargateResource is a valid Fargate CPU/memory combination.
type fargateResource struct {
	cpu        int64   // in CPU units (256 = 0.25 vCPU)
	memOptions []int64 // explicit valid memory values in MB (for lower tiers)
	memMin     int64   // only used when memOptions is nil (range-based tiers)
	memMax     int64
	memInc     int64
}

// fargateCombos lists all valid Fargate CPU/memory combinations.
// Lower tiers (256, 512) use explicit options because the valid values
// are not evenly spaced from the minimum. Higher tiers use range-based
// increments which correctly produce valid values.
var fargateCombos = []fargateResource{
	{256, []int64{512, 1024, 2048}, 0, 0, 0},
	{512, []int64{1024, 2048, 3072, 4096}, 0, 0, 0},
	{1024, nil, 2048, 8192, 1024},
	{2048, nil, 4096, 16384, 1024},
	{4096, nil, 8192, 30720, 1024},
	{8192, nil, 16384, 61440, 4096},
	{16384, nil, 32768, 122880, 8192},
}

// fargateResources maps Docker HostConfig resource constraints to the smallest
// valid Fargate CPU/memory combination that satisfies the request.
func fargateResources(containers []containerInput) (cpu, memory string) {
	var totalMemMB, totalCPU int64
	for _, ci := range containers {
		hc := ci.Container.HostConfig
		if hc.Memory > 0 {
			totalMemMB += hc.Memory / (1024 * 1024)
		}
		if hc.MemoryReservation > 0 && hc.Memory == 0 {
			totalMemMB += hc.MemoryReservation / (1024 * 1024)
		}
		if hc.NanoCPUs > 0 {
			totalCPU += hc.NanoCPUs * 1024 / 1e9
		} else if hc.CPUShares > 0 {
			totalCPU += hc.CPUShares
		}
	}

	for _, combo := range fargateCombos {
		if combo.cpu < totalCPU {
			continue
		}

		if totalMemMB <= 0 {
			// No memory specified: use minimum for this CPU tier
			if len(combo.memOptions) > 0 {
				return fmt.Sprintf("%d", combo.cpu), fmt.Sprintf("%d", combo.memOptions[0])
			}
			return fmt.Sprintf("%d", combo.cpu), fmt.Sprintf("%d", combo.memMin)
		}

		// Explicit memory options (lower tiers)
		if len(combo.memOptions) > 0 {
			for _, opt := range combo.memOptions {
				if opt >= totalMemMB {
					return fmt.Sprintf("%d", combo.cpu), fmt.Sprintf("%d", opt)
				}
			}
			continue // requested memory exceeds this CPU tier's max
		}

		// Range-based (higher tiers)
		if totalMemMB <= combo.memMax {
			mem := combo.memMin
			for mem < totalMemMB {
				mem += combo.memInc
			}
			if mem > combo.memMax {
				mem = combo.memMax
			}
			return fmt.Sprintf("%d", combo.cpu), fmt.Sprintf("%d", mem)
		}
	}

	return "256", "512"
}

// sanitizeContainerName converts a container name to a valid ECS container definition name.
// Strips leading "/" and replaces non-alphanumeric characters with "-".
// shellQuoteArgs joins argv with single-quoted POSIX shell escaping so the
// caller can use `exec $(shellQuoteArgs(argv))` from inside an `sh -c` script
// without arg-splitting hazards.
func shellQuoteArgs(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}

func sanitizeContainerName(name string) string {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return "sidecar"
	}
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteByte('-')
		}
	}
	result := b.String()
	if result == "" {
		return "sidecar"
	}
	return result
}
