package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sockerless/api"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// Info returns backend system information.
func (s *Server) Info() (*api.BackendInfo, error) {
	return s.getInfo(context.Background())
}

// ContainerCreate creates a container via the Docker SDK.
func (s *Server) ContainerCreate(req *api.ContainerCreateRequest) (*api.ContainerCreateResponse, error) {
	ctx := context.Background()

	config := &container.Config{}
	if req.ContainerConfig != nil {
		cc := req.ContainerConfig
		config.Image = cc.Image
		config.Cmd = cc.Cmd
		config.Env = cc.Env
		config.Labels = cc.Labels
		config.Tty = cc.Tty
		config.OpenStdin = cc.OpenStdin
		config.StdinOnce = cc.StdinOnce
		config.AttachStdin = cc.AttachStdin
		config.AttachStdout = cc.AttachStdout
		config.AttachStderr = cc.AttachStderr
		config.WorkingDir = cc.WorkingDir
		config.Entrypoint = cc.Entrypoint
		config.User = cc.User
		config.Hostname = cc.Hostname
		config.Domainname = cc.Domainname
		config.StopSignal = cc.StopSignal
		config.StopTimeout = cc.StopTimeout
		config.Shell = cc.Shell
		config.Volumes = cc.Volumes
		config.ArgsEscaped = cc.ArgsEscaped
		config.NetworkDisabled = cc.NetworkDisabled
		config.OnBuild = cc.OnBuild
		config.MacAddress = cc.MacAddress
		if len(cc.ExposedPorts) > 0 {
			config.ExposedPorts = make(nat.PortSet, len(cc.ExposedPorts))
			for p := range cc.ExposedPorts {
				config.ExposedPorts[nat.Port(p)] = struct{}{}
			}
		}
		if cc.Healthcheck != nil {
			config.Healthcheck = &container.HealthConfig{
				Test:          cc.Healthcheck.Test,
				Interval:      time.Duration(cc.Healthcheck.Interval),
				Timeout:       time.Duration(cc.Healthcheck.Timeout),
				StartPeriod:   time.Duration(cc.Healthcheck.StartPeriod),
				StartInterval: time.Duration(cc.Healthcheck.StartInterval),
				Retries:       cc.Healthcheck.Retries,
			}
		}
	}

	hostConfig := mapHostConfigToDocker(req.HostConfig)
	networkingConfig := mapNetworkingConfigToDocker(req.NetworkingConfig)

	// Auto-pull image if needed
	_, _, err := s.docker.ImageInspectWithRaw(ctx, config.Image)
	if err != nil {
		rc, pullErr := s.docker.ImagePull(ctx, config.Image, image.PullOptions{})
		if pullErr != nil {
			return nil, &api.NotFoundError{Resource: "image", ID: config.Image}
		}
		io.Copy(io.Discard, rc)
		rc.Close()
	}

	name := req.Name
	resp, err := s.docker.ContainerCreate(ctx, config, hostConfig, networkingConfig, (*ocispec.Platform)(nil), name)
	if err != nil {
		return nil, mapDockerError(err)
	}

	warnings := resp.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	return &api.ContainerCreateResponse{
		ID:       resp.ID,
		Warnings: warnings,
	}, nil
}

// ContainerInspect returns container details.
func (s *Server) ContainerInspect(id string) (*api.Container, error) {
	info, err := s.docker.ContainerInspect(context.Background(), id)
	if err != nil {
		return nil, mapDockerError(err)
	}
	c := ConvertContainerJSON(info)
	return &c, nil
}

// ContainerList lists containers.
func (s *Server) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	listOpts := container.ListOptions{
		All:   opts.All,
		Limit: opts.Limit,
	}
	if len(opts.Filters) > 0 {
		listOpts.Filters = filters.NewArgs()
		for k, vals := range opts.Filters {
			for _, v := range vals {
				listOpts.Filters.Add(k, v)
			}
		}
	}

	containers, err := s.docker.ContainerList(context.Background(), listOpts)
	if err != nil {
		return nil, mapDockerError(err)
	}

	result := make([]*api.ContainerSummary, 0, len(containers))
	for _, c := range containers {
		result = append(result, ConvertContainerSummary(c))
	}
	return result, nil
}

// ContainerStart starts a container.
func (s *Server) ContainerStart(id string) error {
	return mapDockerError(s.docker.ContainerStart(context.Background(), id, container.StartOptions{}))
}

// ContainerStop stops a container.
func (s *Server) ContainerStop(id string, timeout *int) error {
	return mapDockerError(s.docker.ContainerStop(context.Background(), id, container.StopOptions{Timeout: timeout}))
}

// ContainerKill sends a signal to a container.
func (s *Server) ContainerKill(id string, signal string) error {
	if signal == "" {
		signal = "SIGKILL"
	}
	return mapDockerError(s.docker.ContainerKill(context.Background(), id, signal))
}

// ContainerRemove removes a container.
func (s *Server) ContainerRemove(id string, force bool) error {
	return mapDockerError(s.docker.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: force}))
}

// ContainerLogs returns container logs as a stream.
func (s *Server) ContainerLogs(id string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	rc, err := s.docker.ContainerLogs(context.Background(), id, container.LogsOptions{
		ShowStdout: opts.ShowStdout,
		ShowStderr: opts.ShowStderr,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
		Tail:       opts.Tail,
		Since:      opts.Since,
		Until:      opts.Until,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return rc, nil
}

// ContainerWait blocks until a container stops and returns its exit code.
func (s *Server) ContainerWait(id string, condition string) (*api.ContainerWaitResponse, error) {
	if condition == "" {
		condition = "not-running"
	}
	ctx := context.Background()
	waitCh, errCh := s.docker.ContainerWait(ctx, id, container.WaitCondition(condition))
	select {
	case result := <-waitCh:
		resp := &api.ContainerWaitResponse{StatusCode: int(result.StatusCode)}
		if result.Error != nil {
			resp.Error = &api.WaitError{Message: result.Error.Message}
		}
		return resp, nil
	case err := <-errCh:
		return nil, mapDockerError(err)
	}
}

// ContainerAttach attaches to a container's stdio.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	resp, err := s.docker.ContainerAttach(context.Background(), id, container.AttachOptions{
		Stream:     opts.Stream,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
		Logs:       opts.Logs,
		DetachKeys: opts.DetachKeys,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &hijackedRWC{resp}, nil
}

// ContainerRestart restarts a container.
func (s *Server) ContainerRestart(id string, timeout *int) error {
	return mapDockerError(s.docker.ContainerRestart(context.Background(), id, container.StopOptions{Timeout: timeout}))
}

// ContainerTop returns the running processes inside a container.
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	if psArgs == "" {
		psArgs = "-ef"
	}
	top, err := s.docker.ContainerTop(context.Background(), id, []string{psArgs})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &api.ContainerTopResponse{
		Titles:    top.Titles,
		Processes: top.Processes,
	}, nil
}

// ContainerPrune removes stopped containers.
func (s *Server) ContainerPrune(f map[string][]string) (*api.ContainerPruneResponse, error) {
	args := filtersFromMap(f)
	report, err := s.docker.ContainersPrune(context.Background(), args)
	if err != nil {
		return nil, mapDockerError(err)
	}
	deleted := report.ContainersDeleted
	if deleted == nil {
		deleted = []string{}
	}
	return &api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    report.SpaceReclaimed,
	}, nil
}

// ContainerStats returns resource usage stats for a container.
func (s *Server) ContainerStats(id string, stream bool) (io.ReadCloser, error) {
	stats, err := s.docker.ContainerStats(context.Background(), id, stream)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return stats.Body, nil
}

// ContainerRename renames a container.
func (s *Server) ContainerRename(id string, newName string) error {
	return mapDockerError(s.docker.ContainerRename(context.Background(), id, newName))
}

// ContainerPause pauses a container.
func (s *Server) ContainerPause(id string) error {
	return mapDockerError(s.docker.ContainerPause(context.Background(), id))
}

// ContainerUnpause unpauses a container.
func (s *Server) ContainerUnpause(id string) error {
	return mapDockerError(s.docker.ContainerUnpause(context.Background(), id))
}

// ExecCreate creates an exec instance in a container.
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	resp, err := s.docker.ContainerExecCreate(context.Background(), containerID, container.ExecOptions{
		AttachStdin:  req.AttachStdin,
		AttachStdout: req.AttachStdout,
		AttachStderr: req.AttachStderr,
		Tty:          req.Tty,
		Cmd:          req.Cmd,
		Env:          req.Env,
		WorkingDir:   req.WorkingDir,
		User:         req.User,
		Privileged:   req.Privileged,
		DetachKeys:   req.DetachKeys,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &api.ExecCreateResponse{ID: resp.ID}, nil
}

// ExecStart starts an exec instance and returns a read-write stream.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	if opts.Detach {
		err := s.docker.ContainerExecStart(context.Background(), id, container.ExecStartOptions{
			Detach: true,
			Tty:    opts.Tty,
		})
		if err != nil {
			return nil, mapDockerError(err)
		}
		return &nopRWC{}, nil
	}

	resp, err := s.docker.ContainerExecAttach(context.Background(), id, container.ExecAttachOptions{
		Detach: opts.Detach,
		Tty:    opts.Tty,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &hijackedRWC{resp}, nil
}

// ExecInspect returns info about an exec instance.
func (s *Server) ExecInspect(id string) (*api.ExecInstance, error) {
	ctx := context.Background()
	resp, err := s.docker.ContainerExecInspect(ctx, id)
	if err != nil {
		return nil, mapDockerError(err)
	}

	exec := &api.ExecInstance{
		ID:          resp.ExecID,
		ContainerID: resp.ContainerID,
		Running:     resp.Running,
		ExitCode:    resp.ExitCode,
		Pid:         resp.Pid,
		CanRemove:   !resp.Running,
	}

	// Fetch raw JSON via HTTP to get ProcessConfig (not exposed by SDK).
	rawResp, rawErr := s.httpGet(ctx, "/exec/"+id+"/json")
	if rawErr == nil {
		defer rawResp.Body.Close()
		var raw struct {
			ProcessConfig *struct {
				Entrypoint string   `json:"entrypoint"`
				Arguments  []string `json:"arguments"`
				Tty        bool     `json:"tty"`
				User       string   `json:"user"`
				Privileged *bool    `json:"privileged,omitempty"`
			} `json:"ProcessConfig"`
		}
		if json.NewDecoder(rawResp.Body).Decode(&raw) == nil && raw.ProcessConfig != nil {
			exec.ProcessConfig = api.ExecProcessConfig{
				Entrypoint: raw.ProcessConfig.Entrypoint,
				Arguments:  raw.ProcessConfig.Arguments,
				Tty:        raw.ProcessConfig.Tty,
				User:       raw.ProcessConfig.User,
				Privileged: raw.ProcessConfig.Privileged,
			}
		}
	}

	return exec, nil
}

// ImagePull pulls an image and returns a progress stream.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	rc, err := s.docker.ImagePull(context.Background(), ref, image.PullOptions{RegistryAuth: auth})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return rc, nil
}

// ImageInspect returns detailed info about an image.
func (s *Server) ImageInspect(name string) (*api.Image, error) {
	info, _, err := s.docker.ImageInspectWithRaw(context.Background(), name)
	if err != nil {
		return nil, mapDockerError(err)
	}
	img := ConvertImageInspect(info)
	return &img, nil
}

// ImageLoad loads an image from a tar archive.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	resp, err := s.docker.ImageLoad(context.Background(), r, false)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return resp.Body, nil
}

// ImageTag tags an image.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	ref := repo
	if tag != "" {
		ref = repo + ":" + tag
	}
	return mapDockerError(s.docker.ImageTag(context.Background(), source, ref))
}

// ImageList lists images.
func (s *Server) ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
	listOpts := image.ListOptions{All: opts.All}
	if len(opts.Filters) > 0 {
		listOpts.Filters = filtersFromMap(opts.Filters)
	}
	images, err := s.docker.ImageList(context.Background(), listOpts)
	if err != nil {
		return nil, mapDockerError(err)
	}
	result := make([]*api.ImageSummary, 0, len(images))
	for _, img := range images {
		s := conv.ConvertImageSummary(img)
		result = append(result, &s)
	}
	return result, nil
}

// ImageRemove removes an image.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	items, err := s.docker.ImageRemove(context.Background(), name, image.RemoveOptions{
		Force:         force,
		PruneChildren: prune,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	result := make([]*api.ImageDeleteResponse, 0, len(items))
	for _, item := range items {
		r := conv.ConvertImageDeleteResponseItem(item)
		result = append(result, &r)
	}
	return result, nil
}

// ImageHistory returns the history of an image.
func (s *Server) ImageHistory(name string) ([]*api.ImageHistoryEntry, error) {
	history, err := s.docker.ImageHistory(context.Background(), name)
	if err != nil {
		return nil, mapDockerError(err)
	}
	result := make([]*api.ImageHistoryEntry, 0, len(history))
	for _, h := range history {
		entry := conv.ConvertImageHistoryResponseItem(h)
		result = append(result, &entry)
	}
	return result, nil
}

// ImagePrune removes unused images.
func (s *Server) ImagePrune(f map[string][]string) (*api.ImagePruneResponse, error) {
	args := filtersFromMap(f)
	report, err := s.docker.ImagesPrune(context.Background(), args)
	if err != nil {
		return nil, mapDockerError(err)
	}
	var deleted []*api.ImageDeleteResponse
	for _, img := range report.ImagesDeleted {
		r := conv.ConvertImageDeleteResponseItem(img)
		deleted = append(deleted, &r)
	}
	if deleted == nil {
		deleted = []*api.ImageDeleteResponse{}
	}
	return &api.ImagePruneResponse{
		ImagesDeleted:  deleted,
		SpaceReclaimed: report.SpaceReclaimed,
	}, nil
}

// AuthLogin authenticates with a Docker registry.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	resp, err := s.docker.RegistryLogin(context.Background(), registry.AuthConfig{
		Username:      req.Username,
		Password:      req.Password,
		Email:         req.Email,
		ServerAddress: req.ServerAddress,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	r := conv.ConvertAuthResponse(resp)
	return &r, nil
}

// NetworkCreate creates a network.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	opts := network.CreateOptions{
		Driver:     req.Driver,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Ingress:    req.Ingress,
		EnableIPv6: &req.EnableIPv6,
		Options:    req.Options,
		Labels:     req.Labels,
	}

	if req.IPAM != nil {
		ipamConfigs := make([]network.IPAMConfig, len(req.IPAM.Config))
		for i, c := range req.IPAM.Config {
			ipamConfigs[i] = network.IPAMConfig{
				Subnet:  c.Subnet,
				IPRange: c.IPRange,
				Gateway: c.Gateway,
			}
		}
		opts.IPAM = &network.IPAM{
			Driver:  req.IPAM.Driver,
			Config:  ipamConfigs,
			Options: req.IPAM.Options,
		}
	}

	resp, err := s.docker.NetworkCreate(context.Background(), req.Name, opts)
	if err != nil {
		return nil, mapDockerError(err)
	}

	return &api.NetworkCreateResponse{
		ID:      resp.ID,
		Warning: resp.Warning,
	}, nil
}

// NetworkList lists networks.
func (s *Server) NetworkList(f map[string][]string) ([]*api.Network, error) {
	opts := network.ListOptions{}
	if len(f) > 0 {
		opts.Filters = filtersFromMap(f)
	}
	networks, err := s.docker.NetworkList(context.Background(), opts)
	if err != nil {
		return nil, mapDockerError(err)
	}

	result := make([]*api.Network, 0, len(networks))
	for _, n := range networks {
		net := ConvertNetworkSummary(n)
		result = append(result, &net)
	}
	return result, nil
}

// NetworkInspect returns details about a network.
func (s *Server) NetworkInspect(id string) (*api.Network, error) {
	n, err := s.docker.NetworkInspect(context.Background(), id, network.InspectOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}
	net := ConvertNetworkResource(n)
	return &net, nil
}

// NetworkConnect connects a container to a network.
func (s *Server) NetworkConnect(id string, req *api.NetworkConnectRequest) error {
	var epConfig *network.EndpointSettings
	if req.EndpointConfig != nil {
		epConfig = APIEndpointToDocker(req.EndpointConfig)
	}
	return mapDockerError(s.docker.NetworkConnect(context.Background(), id, req.Container, epConfig))
}

// NetworkDisconnect disconnects a container from a network.
func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	return mapDockerError(s.docker.NetworkDisconnect(context.Background(), id, req.Container, req.Force))
}

// NetworkRemove removes a network.
func (s *Server) NetworkRemove(id string) error {
	return mapDockerError(s.docker.NetworkRemove(context.Background(), id))
}

// NetworkPrune removes unused networks.
func (s *Server) NetworkPrune(f map[string][]string) (*api.NetworkPruneResponse, error) {
	args := filtersFromMap(f)
	report, err := s.docker.NetworksPrune(context.Background(), args)
	if err != nil {
		return nil, mapDockerError(err)
	}
	deleted := report.NetworksDeleted
	if deleted == nil {
		deleted = []string{}
	}
	return &api.NetworkPruneResponse{
		NetworksDeleted: deleted,
	}, nil
}

// VolumeCreate creates a volume.
func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	vol, err := s.docker.VolumeCreate(context.Background(), volume.CreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	v := conv.ConvertVolume(vol)
	return &v, nil
}

// VolumeList lists volumes.
func (s *Server) VolumeList(f map[string][]string) (*api.VolumeListResponse, error) {
	opts := volume.ListOptions{}
	if len(f) > 0 {
		opts.Filters = filtersFromMap(f)
	}
	vols, err := s.docker.VolumeList(context.Background(), opts)
	if err != nil {
		return nil, mapDockerError(err)
	}

	result := make([]*api.Volume, 0)
	for _, v := range vols.Volumes {
		vol := conv.ConvertVolume(*v)
		result = append(result, &vol)
	}
	return &api.VolumeListResponse{
		Volumes:  result,
		Warnings: []string{},
	}, nil
}

// VolumeInspect returns details about a volume.
func (s *Server) VolumeInspect(name string) (*api.Volume, error) {
	vol, err := s.docker.VolumeInspect(context.Background(), name)
	if err != nil {
		return nil, mapDockerError(err)
	}
	v := conv.ConvertVolume(vol)
	return &v, nil
}

// VolumeRemove removes a volume.
func (s *Server) VolumeRemove(name string, force bool) error {
	return mapDockerError(s.docker.VolumeRemove(context.Background(), name, force))
}

// VolumePrune removes unused volumes.
func (s *Server) VolumePrune(f map[string][]string) (*api.VolumePruneResponse, error) {
	args := filtersFromMap(f)
	report, err := s.docker.VolumesPrune(context.Background(), args)
	if err != nil {
		return nil, mapDockerError(err)
	}
	deleted := report.VolumesDeleted
	if deleted == nil {
		deleted = []string{}
	}
	return &api.VolumePruneResponse{
		VolumesDeleted: deleted,
		SpaceReclaimed: report.SpaceReclaimed,
	}, nil
}

// SystemEvents returns a stream of Docker events.
func (s *Server) SystemEvents(opts api.EventsOptions) (io.ReadCloser, error) {
	listOpts := events.ListOptions{
		Since: opts.Since,
		Until: opts.Until,
	}
	if len(opts.Filters) > 0 {
		listOpts.Filters = filtersFromMap(opts.Filters)
	}

	eventsCh, errCh := s.docker.Events(context.Background(), listOpts)

	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		for {
			select {
			case event, ok := <-eventsCh:
				if !ok {
					_ = pw.Close()
					return
				}
				mapped := conv.ConvertEventMessage(event)
				if err := enc.Encode(mapped); err != nil {
					pw.CloseWithError(err)
					return
				}
			case err, ok := <-errCh:
				if ok && err != nil {
					pw.CloseWithError(err)
					return
				}
				_ = pw.Close()
				return
			}
		}
	}()

	return pr, nil
}

// SystemDf returns disk usage information.
func (s *Server) SystemDf() (*api.DiskUsageResponse, error) {
	du, err := s.docker.DiskUsage(context.Background(), types.DiskUsageOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}

	var containers []*api.ContainerSummary
	for _, c := range du.Containers {
		containers = append(containers, ConvertContainerSummary(*c))
	}
	if containers == nil {
		containers = []*api.ContainerSummary{}
	}

	var images []*api.ImageSummary
	for _, img := range du.Images {
		s := conv.ConvertImageSummary(*img)
		images = append(images, &s)
	}
	if images == nil {
		images = []*api.ImageSummary{}
	}

	var volumes []*api.Volume
	if du.Volumes != nil {
		for _, v := range du.Volumes {
			vol := conv.ConvertVolume(*v)
			volumes = append(volumes, &vol)
		}
	}
	if volumes == nil {
		volumes = []*api.Volume{}
	}

	var buildCache []*api.BuildCache
	for _, bc := range du.BuildCache {
		entry := ConvertBuildCache(*bc)
		buildCache = append(buildCache, &entry)
	}
	if buildCache == nil {
		buildCache = []*api.BuildCache{}
	}

	return &api.DiskUsageResponse{
		LayersSize: du.LayersSize,
		Images:     images,
		Containers: containers,
		Volumes:    volumes,
		BuildCache: buildCache,
	}, nil
}

// --- Helper types and functions ---

// hijackedRWC wraps a Docker HijackedResponse as an io.ReadWriteCloser.
type hijackedRWC struct {
	resp types.HijackedResponse
}

func (h *hijackedRWC) Read(p []byte) (int, error)  { return h.resp.Reader.Read(p) }
func (h *hijackedRWC) Write(p []byte) (int, error) { return h.resp.Conn.Write(p) }
func (h *hijackedRWC) Close() error                { h.resp.Close(); return nil }

// nopRWC is a no-op ReadWriteCloser for detached exec.
type nopRWC struct{}

func (n *nopRWC) Read([]byte) (int, error)  { return 0, io.EOF }
func (n *nopRWC) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (n *nopRWC) Close() error              { return nil }

// filtersFromMap converts a map[string][]string to filters.Args.
func filtersFromMap(f map[string][]string) filters.Args {
	args := filters.NewArgs()
	for k, vals := range f {
		for _, v := range vals {
			args.Add(k, v)
		}
	}
	return args
}

// mapHostConfigToDocker converts api.HostConfig to Docker SDK container.HostConfig.
func mapHostConfigToDocker(hc *api.HostConfig) *container.HostConfig {
	if hc == nil {
		return nil
	}
	hostConfig := &container.HostConfig{
		NetworkMode: container.NetworkMode(hc.NetworkMode),
		Binds:       hc.Binds,
		AutoRemove:  hc.AutoRemove,
		Privileged:  hc.Privileged,
		CapAdd:      hc.CapAdd,
		CapDrop:     hc.CapDrop,
		Init:        hc.Init,
		UsernsMode:  container.UsernsMode(hc.UsernsMode),
		ShmSize:     hc.ShmSize,
		Tmpfs:       hc.Tmpfs,
		SecurityOpt: hc.SecurityOpt,
		ExtraHosts:  hc.ExtraHosts,
		Isolation:   container.Isolation(hc.Isolation),
		RestartPolicy: container.RestartPolicy{
			Name:              container.RestartPolicyMode(hc.RestartPolicy.Name),
			MaximumRetryCount: hc.RestartPolicy.MaximumRetryCount,
		},
		DNS:        hc.DNS,
		DNSSearch:  hc.DNSSearch,
		DNSOptions: hc.DNSOptions,
		Resources: container.Resources{
			Memory:            hc.Memory,
			MemorySwap:        hc.MemorySwap,
			MemoryReservation: hc.MemoryReservation,
			CPUShares:         hc.CPUShares,
			CPUQuota:          hc.CPUQuota,
			CPUPeriod:         hc.CPUPeriod,
			CpusetCpus:        hc.CpusetCpus,
			CpusetMems:        hc.CpusetMems,
			BlkioWeight:       hc.BlkioWeight,
			NanoCPUs:          hc.NanoCPUs,
			PidsLimit:         hc.PidsLimit,
			OomKillDisable:    hc.OomKillDisable,
		},
		PidMode:         container.PidMode(hc.PidMode),
		IpcMode:         container.IpcMode(hc.IpcMode),
		UTSMode:         container.UTSMode(hc.UTSMode),
		VolumesFrom:     hc.VolumesFrom,
		GroupAdd:        hc.GroupAdd,
		ReadonlyRootfs:  hc.ReadonlyRootfs,
		Sysctls:         hc.Sysctls,
		Runtime:         hc.Runtime,
		Links:           hc.Links,
		PublishAllPorts: hc.PublishAllPorts,
		CgroupnsMode:    container.CgroupnsMode(hc.CgroupnsMode),
		ConsoleSize:     hc.ConsoleSize,
	}
	if len(hc.PortBindings) > 0 {
		hostConfig.PortBindings = make(nat.PortMap, len(hc.PortBindings))
		for port, bindings := range hc.PortBindings {
			var nb []nat.PortBinding
			for _, b := range bindings {
				nb = append(nb, nat.PortBinding{HostIP: b.HostIP, HostPort: b.HostPort})
			}
			hostConfig.PortBindings[nat.Port(port)] = nb
		}
	}
	if hc.LogConfig.Type != "" {
		hostConfig.LogConfig = container.LogConfig{
			Type:   hc.LogConfig.Type,
			Config: hc.LogConfig.Config,
		}
	}
	for _, m := range hc.Mounts {
		dm := mount.Mount{
			Type:        mount.Type(m.Type),
			Source:      m.Source,
			Target:      m.Target,
			ReadOnly:    m.ReadOnly,
			Consistency: mount.Consistency(m.Consistency),
		}
		if m.BindOptions != nil {
			dm.BindOptions = &mount.BindOptions{
				Propagation: mount.Propagation(m.BindOptions.Propagation),
			}
		}
		if m.VolumeOptions != nil {
			dm.VolumeOptions = &mount.VolumeOptions{
				NoCopy: m.VolumeOptions.NoCopy,
				Labels: m.VolumeOptions.Labels,
			}
			if m.VolumeOptions.DriverConfig != nil {
				dm.VolumeOptions.DriverConfig = &mount.Driver{
					Name:    m.VolumeOptions.DriverConfig.Name,
					Options: m.VolumeOptions.DriverConfig.Options,
				}
			}
		}
		if m.TmpfsOptions != nil {
			dm.TmpfsOptions = &mount.TmpfsOptions{
				SizeBytes: m.TmpfsOptions.SizeBytes,
				Mode:      os.FileMode(m.TmpfsOptions.Mode),
			}
		}
		hostConfig.Mounts = append(hostConfig.Mounts, dm)
	}
	return hostConfig
}

// mapNetworkingConfigToDocker converts api.NetworkingConfig to Docker SDK network.NetworkingConfig.
func mapNetworkingConfigToDocker(nc *api.NetworkingConfig) *network.NetworkingConfig {
	if nc == nil || len(nc.EndpointsConfig) == 0 {
		return nil
	}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: make(map[string]*network.EndpointSettings, len(nc.EndpointsConfig)),
	}
	for name, ep := range nc.EndpointsConfig {
		es := &network.EndpointSettings{
			NetworkID:           ep.NetworkID,
			EndpointID:          ep.EndpointID,
			Gateway:             ep.Gateway,
			IPAddress:           ep.IPAddress,
			IPPrefixLen:         ep.IPPrefixLen,
			IPv6Gateway:         ep.IPv6Gateway,
			GlobalIPv6Address:   ep.GlobalIPv6Address,
			GlobalIPv6PrefixLen: ep.GlobalIPv6PrefixLen,
			MacAddress:          ep.MacAddress,
			Aliases:             ep.Aliases,
			DriverOpts:          ep.DriverOpts,
		}
		if ep.IPAMConfig != nil {
			es.IPAMConfig = &network.EndpointIPAMConfig{
				IPv4Address:  ep.IPAMConfig.IPv4Address,
				IPv6Address:  ep.IPAMConfig.IPv6Address,
				LinkLocalIPs: ep.IPAMConfig.LinkLocalIPs,
			}
		}
		networkingConfig.EndpointsConfig[name] = es
	}
	return networkingConfig
}

// --- Phase 85 methods ---

// ContainerResize resizes the TTY of a container.
func (s *Server) ContainerResize(id string, h, w int) error {
	return mapDockerError(s.docker.ContainerResize(context.Background(), id, container.ResizeOptions{
		Height: uint(h),
		Width:  uint(w),
	}))
}

// ExecResize resizes the TTY of an exec instance.
func (s *Server) ExecResize(id string, h, w int) error {
	return mapDockerError(s.docker.ContainerExecResize(context.Background(), id, container.ResizeOptions{
		Height: uint(h),
		Width:  uint(w),
	}))
}

// ContainerPutArchive uploads a tar archive to a container path.
func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	return mapDockerError(s.docker.CopyToContainer(context.Background(), id, path, body, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: !noOverwriteDirNonDir,
	}))
}

// ContainerStatPath returns stat info for a path in a container.
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	stat, err := s.docker.ContainerStatPath(context.Background(), id, path)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &api.ContainerPathStat{
		Name:       stat.Name,
		Size:       stat.Size,
		Mode:       stat.Mode,
		Mtime:      stat.Mtime,
		LinkTarget: stat.LinkTarget,
	}, nil
}

// ContainerGetArchive downloads a tar archive from a container path.
func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	rc, stat, err := s.docker.CopyFromContainer(context.Background(), id, path)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &api.ContainerArchiveResponse{
		Stat: api.ContainerPathStat{
			Name:       stat.Name,
			Size:       stat.Size,
			Mode:       stat.Mode,
			Mtime:      stat.Mtime,
			LinkTarget: stat.LinkTarget,
		},
		Reader: rc,
	}, nil
}

// ContainerUpdate updates resource limits on a container.
func (s *Server) ContainerUpdate(id string, req *api.ContainerUpdateRequest) (*api.ContainerUpdateResponse, error) {
	updateConfig := container.UpdateConfig{
		Resources: container.Resources{
			Memory:            req.Memory,
			MemorySwap:        req.MemorySwap,
			MemoryReservation: req.MemoryReservation,
			CPUShares:         req.CPUShares,
			CPUQuota:          req.CPUQuota,
			CPUPeriod:         req.CPUPeriod,
			CpusetCpus:        req.CpusetCpus,
			CpusetMems:        req.CpusetMems,
			BlkioWeight:       req.BlkioWeight,
			PidsLimit:         req.PidsLimit,
			OomKillDisable:    req.OomKillDisable,
		},
		RestartPolicy: container.RestartPolicy{
			Name:              container.RestartPolicyMode(req.RestartPolicy.Name),
			MaximumRetryCount: req.RestartPolicy.MaximumRetryCount,
		},
	}
	resp, err := s.docker.ContainerUpdate(context.Background(), id, updateConfig)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &api.ContainerUpdateResponse{Warnings: resp.Warnings}, nil
}

// ContainerChanges returns filesystem changes in a container.
func (s *Server) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	changes, err := s.docker.ContainerDiff(context.Background(), id)
	if err != nil {
		return nil, mapDockerError(err)
	}
	var result []api.ContainerChangeItem
	for _, c := range changes {
		result = append(result, conv.ConvertContainerChange(c))
	}
	if result == nil {
		result = []api.ContainerChangeItem{}
	}
	return result, nil
}

// ContainerExport exports a container's filesystem as a tar stream.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	rc, err := s.docker.ContainerExport(context.Background(), id)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return rc, nil
}

// ImageBuild builds an image from a Dockerfile and build context.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	dockerOpts := types.ImageBuildOptions{
		Tags:       opts.Tags,
		Dockerfile: opts.Dockerfile,
		BuildArgs:  opts.BuildArgs,
		NoCache:    opts.NoCache,
		Remove:     opts.Remove,
		Labels:     opts.Labels,
	}
	if dockerOpts.Dockerfile == "" {
		dockerOpts.Dockerfile = "Dockerfile"
	}

	resp, err := s.docker.ImageBuild(context.Background(), buildContext, dockerOpts)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return resp.Body, nil
}

// ImagePush pushes an image to a registry.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	if tag == "" {
		tag = "latest"
	}
	ref := name + ":" + tag

	resp, err := s.docker.ImagePush(context.Background(), ref, image.PushOptions{
		RegistryAuth: auth,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return resp, nil
}

// ImageSave exports images as a tar archive.
func (s *Server) ImageSave(names []string) (io.ReadCloser, error) {
	resp, err := s.docker.ImageSave(context.Background(), names)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return resp, nil
}

// ImageSearch searches Docker Hub for images.
func (s *Server) ImageSearch(term string, limit int, searchFilters map[string][]string) ([]*api.ImageSearchResult, error) {
	results, err := s.docker.ImageSearch(context.Background(), term, registry.SearchOptions{
		Limit: limit,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}

	mapped := make([]*api.ImageSearchResult, 0, len(results))
	for _, r := range results {
		mapped = append(mapped, &api.ImageSearchResult{
			Name:        r.Name,
			Description: r.Description,
			StarCount:   r.StarCount,
			IsOfficial:  r.IsOfficial,
			IsAutomated: r.IsAutomated,
		})
	}
	return mapped, nil
}

// ContainerCommit creates a new image from a container's changes.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	pause := req.Pause
	commitOpts := container.CommitOptions{
		Comment: req.Comment,
		Author:  req.Author,
		Pause:   pause,
		Changes: req.Changes,
	}
	if req.Tag != "" {
		commitOpts.Reference = req.Repo + ":" + req.Tag
	} else {
		commitOpts.Reference = req.Repo
	}

	resp, err := s.docker.ContainerCommit(context.Background(), req.Container, commitOpts)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &api.ContainerCommitResponse{ID: resp.ID}, nil
}

// PodCreate is not supported by Docker backend.
func (s *Server) PodCreate(req *api.PodCreateRequest) (*api.PodCreateResponse, error) {
	return nil, &api.NotImplementedError{Message: "pods are not supported by Docker backend"}
}

// PodList is not supported by Docker backend.
func (s *Server) PodList(opts api.PodListOptions) ([]*api.PodListEntry, error) {
	return []*api.PodListEntry{}, nil
}

// PodInspect is not supported by Docker backend.
func (s *Server) PodInspect(name string) (*api.PodInspectResponse, error) {
	return nil, &api.NotFoundError{Resource: "pod", ID: name}
}

// PodExists is not supported by Docker backend.
func (s *Server) PodExists(name string) (bool, error) {
	return false, nil
}

// PodStart is not supported by Docker backend.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	return nil, &api.NotFoundError{Resource: "pod", ID: name}
}

// PodStop is not supported by Docker backend.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	return nil, &api.NotFoundError{Resource: "pod", ID: name}
}

// PodKill is not supported by Docker backend.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	return nil, &api.NotFoundError{Resource: "pod", ID: name}
}

// PodRemove is not supported by Docker backend.
func (s *Server) PodRemove(name string, force bool) error {
	return &api.NotFoundError{Resource: "pod", ID: name}
}

// Prevent unused import errors
var (
	_ = bytes.NewReader
	_ = sort.Slice
	_ = strings.Contains
)
