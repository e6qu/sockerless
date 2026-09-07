package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/moby/moby/client"
	"io"
	"net"
	"net/netip"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
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
		if len(cc.ExposedPorts) > 0 {
			config.ExposedPorts = make(network.PortSet, len(cc.ExposedPorts))
			for p := range cc.ExposedPorts {
				port, err := network.ParsePort(p)
				if err != nil {
					return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid exposed port %q: %v", p, err)}
				}
				config.ExposedPorts[port] = struct{}{}
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

	hostConfig, err := mapHostConfigToDocker(req.HostConfig)
	if err != nil {
		return nil, err
	}
	networkingConfig, err := mapNetworkingConfigToDocker(req.NetworkingConfig)
	if err != nil {
		return nil, err
	}
	if req.ContainerConfig != nil && req.MacAddress != "" {
		// The Docker API carries a container's MAC address on its network
		// endpoints; a container-wide address applies to every endpoint it
		// joins, the default one when it names none.
		mac, err := net.ParseMAC(req.MacAddress)
		if err != nil {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid MAC address %q: %v", req.MacAddress, err)}
		}
		if networkingConfig == nil {
			networkingConfig = &network.NetworkingConfig{}
		}
		if len(networkingConfig.EndpointsConfig) == 0 {
			networkingConfig.EndpointsConfig = map[string]*network.EndpointSettings{"default": {}}
		}
		for _, endpoint := range networkingConfig.EndpointsConfig {
			endpoint.MacAddress = network.HardwareAddr(mac)
		}
	}

	// Auto-pull image if needed
	if _, err := s.docker.ImageInspect(ctx, config.Image); err != nil {
		rc, pullErr := s.docker.ImagePull(ctx, config.Image, client.ImagePullOptions{})
		if pullErr != nil {
			// Preserve the real status (401 auth, registry 5xx, network)
			// instead of masking every failure as a 404.
			return nil, mapDockerError(pullErr)
		}
		// The pull stream reports mid-stream failures (e.g. a layer that
		// 401s after the manifest fetched) only as an error from the body
		// reader; draining without checking would let a failed pull proceed
		// to ContainerCreate.
		if _, copyErr := io.Copy(io.Discard, rc); copyErr != nil {
			rc.Close()
			return nil, mapDockerError(copyErr)
		}
		rc.Close()
	}

	resp, err := s.docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkingConfig,
		Name:             req.Name,
	})
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
	info, err := s.docker.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}
	c := ConvertContainerJSON(info.Container)
	return &c, nil
}

// ContainerList lists containers.
func (s *Server) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	listOpts := client.ContainerListOptions{
		All:   opts.All,
		Limit: opts.Limit,
	}
	if len(opts.Filters) > 0 {
		listOpts.Filters = client.Filters{}
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

	result := make([]*api.ContainerSummary, 0, len(containers.Items))
	for _, c := range containers.Items {
		result = append(result, ConvertContainerSummary(c))
	}
	return result, nil
}

// ContainerStart starts a container.
func (s *Server) ContainerStart(id string) error {
	_, err := s.docker.ContainerStart(context.Background(), id, client.ContainerStartOptions{})
	return mapDockerError(err)
}

// ContainerStop stops a container.
func (s *Server) ContainerStop(id string, timeout *int) error {
	_, err := s.docker.ContainerStop(context.Background(), id, client.ContainerStopOptions{Timeout: timeout})
	return mapDockerError(err)
}

// ContainerKill sends a signal to a container.
func (s *Server) ContainerKill(id string, signal string) error {
	if signal == "" {
		signal = "SIGKILL"
	}
	_, err := s.docker.ContainerKill(context.Background(), id, client.ContainerKillOptions{Signal: signal})
	return mapDockerError(err)
}

// ContainerRemove removes a container.
func (s *Server) ContainerRemove(id string, force bool) error {
	_, err := s.docker.ContainerRemove(context.Background(), id, client.ContainerRemoveOptions{Force: force})
	return mapDockerError(err)
}

// ContainerLogs returns container logs as a stream.
func (s *Server) ContainerLogs(id string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	rc, err := s.docker.ContainerLogs(context.Background(), id, client.ContainerLogsOptions{
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
	return s.ContainerWaitCtx(context.Background(), id, condition)
}

// ContainerWaitCtx is the context-aware variant the wait handler prefers so
// that a client which disconnects after issuing `docker wait` cancels the
// upstream daemon wait instead of leaking this goroutine until the container
// exits. It overrides the promoted *core.BaseServer.ContainerWaitCtx so the
// real Docker SDK path (not the in-memory channel path) handles passthrough.
func (s *Server) ContainerWaitCtx(ctx context.Context, id string, condition string) (*api.ContainerWaitResponse, error) {
	if condition == "" {
		condition = "not-running"
	}
	wait := s.docker.ContainerWait(ctx, id, client.ContainerWaitOptions{Condition: container.WaitCondition(condition)})
	select {
	case result := <-wait.Result:
		resp := &api.ContainerWaitResponse{StatusCode: int(result.StatusCode)}
		if result.Error != nil {
			resp.Error = &api.WaitError{Message: result.Error.Message}
		}
		return resp, nil
	case err := <-wait.Error:
		return nil, mapDockerError(err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ContainerAttach attaches to a container's stdio.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	resp, err := s.docker.ContainerAttach(context.Background(), id, client.ContainerAttachOptions{
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
	return &hijackedRWC{resp.HijackedResponse}, nil
}

// ContainerRestart restarts a container.
func (s *Server) ContainerRestart(id string, timeout *int) error {
	_, err := s.docker.ContainerRestart(context.Background(), id, client.ContainerRestartOptions{Timeout: timeout})
	return mapDockerError(err)
}

// ContainerTop returns the running processes inside a container.
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	if psArgs == "" {
		psArgs = "-ef"
	}
	top, err := s.docker.ContainerTop(context.Background(), id, client.ContainerTopOptions{Arguments: []string{psArgs}})
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
	pruned, err := s.docker.ContainerPrune(context.Background(), client.ContainerPruneOptions{Filters: filtersFromMap(f)})
	if err != nil {
		return nil, mapDockerError(err)
	}
	report := pruned.Report
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
	stats, err := s.docker.ContainerStats(context.Background(), id, client.ContainerStatsOptions{Stream: stream})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return stats.Body, nil
}

// ContainerRename renames a container.
func (s *Server) ContainerRename(id string, newName string) error {
	_, err := s.docker.ContainerRename(context.Background(), id, client.ContainerRenameOptions{NewName: newName})
	return mapDockerError(err)
}

// ContainerPause pauses a container.
func (s *Server) ContainerPause(id string) error {
	_, err := s.docker.ContainerPause(context.Background(), id, client.ContainerPauseOptions{})
	return mapDockerError(err)
}

// ContainerUnpause unpauses a container.
func (s *Server) ContainerUnpause(id string) error {
	_, err := s.docker.ContainerUnpause(context.Background(), id, client.ContainerUnpauseOptions{})
	return mapDockerError(err)
}

// ExecCreate creates an exec instance in a container.
func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	resp, err := s.docker.ExecCreate(context.Background(), containerID, client.ExecCreateOptions{
		AttachStdin:  req.AttachStdin,
		AttachStdout: req.AttachStdout,
		AttachStderr: req.AttachStderr,
		TTY:          req.Tty,
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
		_, err := s.docker.ExecStart(context.Background(), id, client.ExecStartOptions{
			Detach: true,
			TTY:    opts.Tty,
		})
		if err != nil {
			return nil, mapDockerError(err)
		}
		return &nopRWC{}, nil
	}

	resp, err := s.docker.ExecAttach(context.Background(), id, client.ExecAttachOptions{
		TTY: opts.Tty,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return &hijackedRWC{resp.HijackedResponse}, nil
}

// ExecInspect returns info about an exec instance.
func (s *Server) ExecInspect(id string) (*api.ExecInstance, error) {
	ctx := context.Background()
	resp, err := s.docker.ExecInspect(ctx, id, client.ExecInspectOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}

	exec := &api.ExecInstance{
		ID:          resp.ID,
		ContainerID: resp.ContainerID,
		Running:     resp.Running,
		ExitCode:    resp.ExitCode,
		Pid:         resp.PID,
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
	rc, err := s.docker.ImagePull(context.Background(), ref, client.ImagePullOptions{RegistryAuth: auth})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return rc, nil
}

// ImageInspect returns detailed info about an image.
func (s *Server) ImageInspect(name string) (*api.Image, error) {
	info, err := s.docker.ImageInspect(context.Background(), name)
	if err != nil {
		return nil, mapDockerError(err)
	}
	img := ConvertImageInspect(info.InspectResponse)
	return &img, nil
}

// ImageLoad loads an image from a tar archive.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	resp, err := s.docker.ImageLoad(context.Background(), r)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return resp, nil
}

// ImageTag tags an image.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	ref := repo
	if tag != "" {
		ref = repo + ":" + tag
	}
	_, err := s.docker.ImageTag(context.Background(), client.ImageTagOptions{Source: source, Target: ref})
	return mapDockerError(err)
}

// ImageList lists images.
func (s *Server) ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
	listOpts := client.ImageListOptions{All: opts.All}
	if len(opts.Filters) > 0 {
		listOpts.Filters = filtersFromMap(opts.Filters)
	}
	images, err := s.docker.ImageList(context.Background(), listOpts)
	if err != nil {
		return nil, mapDockerError(err)
	}
	result := make([]*api.ImageSummary, 0, len(images.Items))
	for _, img := range images.Items {
		s := conv.ConvertImageSummary(img)
		result = append(result, &s)
	}
	return result, nil
}

// ImageRemove removes an image.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	removed, err := s.docker.ImageRemove(context.Background(), name, client.ImageRemoveOptions{
		Force:         force,
		PruneChildren: prune,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	result := make([]*api.ImageDeleteResponse, 0, len(removed.Items))
	for _, item := range removed.Items {
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
	result := make([]*api.ImageHistoryEntry, 0, len(history.Items))
	for _, h := range history.Items {
		entry := conv.ConvertImageHistoryResponseItem(h)
		result = append(result, &entry)
	}
	return result, nil
}

// ImagePrune removes unused images.
func (s *Server) ImagePrune(f map[string][]string) (*api.ImagePruneResponse, error) {
	pruned, err := s.docker.ImagePrune(context.Background(), client.ImagePruneOptions{Filters: filtersFromMap(f)})
	if err != nil {
		return nil, mapDockerError(err)
	}
	report := pruned.Report
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
	// The registry login carries a username and password (the email a
	// Docker client may still send is not part of the credential).
	resp, err := s.docker.RegistryLogin(context.Background(), client.RegistryLoginOptions{
		Username:      req.Username,
		Password:      req.Password,
		ServerAddress: req.ServerAddress,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	r := conv.ConvertAuthResponse(resp.Auth)
	return &r, nil
}

// NetworkCreate creates a network.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	opts := client.NetworkCreateOptions{
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
			cfg, err := ipamConfigToDocker(c)
			if err != nil {
				return nil, err
			}
			ipamConfigs[i] = cfg
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
		Warning: strings.Join(resp.Warning, "\n"),
	}, nil
}

// NetworkList lists networks.
func (s *Server) NetworkList(f map[string][]string) ([]*api.Network, error) {
	opts := client.NetworkListOptions{}
	if len(f) > 0 {
		opts.Filters = filtersFromMap(f)
	}
	networks, err := s.docker.NetworkList(context.Background(), opts)
	if err != nil {
		return nil, mapDockerError(err)
	}

	result := make([]*api.Network, 0, len(networks.Items))
	for _, n := range networks.Items {
		net := ConvertNetworkSummary(n)
		result = append(result, &net)
	}
	return result, nil
}

// NetworkInspect returns details about a network.
func (s *Server) NetworkInspect(id string) (*api.Network, error) {
	n, err := s.docker.NetworkInspect(context.Background(), id, client.NetworkInspectOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}
	net := ConvertNetworkResource(n.Network)
	return &net, nil
}

// NetworkConnect connects a container to a network.
func (s *Server) NetworkConnect(id string, req *api.NetworkConnectRequest) error {
	epConfig, err := APIEndpointToDocker(req.EndpointConfig)
	if err != nil {
		return err
	}
	_, err = s.docker.NetworkConnect(context.Background(), id, client.NetworkConnectOptions{Container: req.Container, EndpointConfig: epConfig})
	return mapDockerError(err)
}

// NetworkDisconnect disconnects a container from a network.
func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	_, err := s.docker.NetworkDisconnect(context.Background(), id, client.NetworkDisconnectOptions{Container: req.Container, Force: req.Force})
	return mapDockerError(err)
}

// NetworkRemove removes a network.
func (s *Server) NetworkRemove(id string) error {
	_, err := s.docker.NetworkRemove(context.Background(), id, client.NetworkRemoveOptions{})
	return mapDockerError(err)
}

// NetworkPrune removes unused networks.
func (s *Server) NetworkPrune(f map[string][]string) (*api.NetworkPruneResponse, error) {
	pruned, err := s.docker.NetworkPrune(context.Background(), client.NetworkPruneOptions{Filters: filtersFromMap(f)})
	if err != nil {
		return nil, mapDockerError(err)
	}
	deleted := pruned.Report.NetworksDeleted
	if deleted == nil {
		deleted = []string{}
	}
	return &api.NetworkPruneResponse{
		NetworksDeleted: deleted,
	}, nil
}

// VolumeCreate creates a volume.
func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	vol, err := s.docker.VolumeCreate(context.Background(), client.VolumeCreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}
	v := conv.ConvertVolume(vol.Volume)
	return &v, nil
}

// VolumeList lists volumes.
func (s *Server) VolumeList(f map[string][]string) (*api.VolumeListResponse, error) {
	opts := client.VolumeListOptions{}
	if len(f) > 0 {
		opts.Filters = filtersFromMap(f)
	}
	vols, err := s.docker.VolumeList(context.Background(), opts)
	if err != nil {
		return nil, mapDockerError(err)
	}

	result := make([]*api.Volume, 0)
	for _, v := range vols.Items {
		vol := conv.ConvertVolume(v)
		result = append(result, &vol)
	}
	warnings := vols.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return &api.VolumeListResponse{
		Volumes:  result,
		Warnings: warnings,
	}, nil
}

// VolumeInspect returns details about a volume.
func (s *Server) VolumeInspect(name string) (*api.Volume, error) {
	vol, err := s.docker.VolumeInspect(context.Background(), name, client.VolumeInspectOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}
	v := conv.ConvertVolume(vol.Volume)
	return &v, nil
}

// VolumeRemove removes a volume.
func (s *Server) VolumeRemove(name string, force bool) error {
	_, err := s.docker.VolumeRemove(context.Background(), name, client.VolumeRemoveOptions{Force: force})
	return mapDockerError(err)
}

// VolumePrune removes unused volumes.
func (s *Server) VolumePrune(f map[string][]string) (*api.VolumePruneResponse, error) {
	pruned, err := s.docker.VolumePrune(context.Background(), client.VolumePruneOptions{Filters: filtersFromMap(f)})
	if err != nil {
		return nil, mapDockerError(err)
	}
	report := pruned.Report
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
	return s.SystemEventsCtx(context.Background(), opts)
}

// SystemEventsCtx is the context-aware variant the events handler prefers so
// that a client which disconnects from `docker events` cancels the upstream
// daemon event stream instead of leaking this goroutine (and its daemon
// connection) until the process exits. It mirrors ContainerWaitCtx: the
// api.Backend.SystemEvents signature is unchanged, the handler reaches the
// cancellable path through the optional SystemEventsCtx interface.
func (s *Server) SystemEventsCtx(ctx context.Context, opts api.EventsOptions) (io.ReadCloser, error) {
	listOpts := client.EventsListOptions{
		Since: opts.Since,
		Until: opts.Until,
	}
	if len(opts.Filters) > 0 {
		listOpts.Filters = filtersFromMap(opts.Filters)
	}

	stream := s.docker.Events(ctx, listOpts)
	eventsCh, errCh := stream.Messages, stream.Err

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
			case <-ctx.Done():
				pw.CloseWithError(ctx.Err())
				return
			}
		}
	}()

	return pr, nil
}

// SystemDf returns disk usage information.
func (s *Server) SystemDf() (*api.DiskUsageResponse, error) {
	du, err := s.docker.DiskUsage(context.Background(), client.DiskUsageOptions{
		Containers: true,
		Images:     true,
		Volumes:    true,
		BuildCache: true,
		Verbose:    true,
	})
	if err != nil {
		return nil, mapDockerError(err)
	}

	containers := make([]*api.ContainerSummary, 0, len(du.Containers.Items))
	for _, c := range du.Containers.Items {
		containers = append(containers, ConvertContainerSummary(c))
	}
	images := make([]*api.ImageSummary, 0, len(du.Images.Items))
	for _, img := range du.Images.Items {
		s := conv.ConvertImageSummary(img)
		images = append(images, &s)
	}
	volumes := make([]*api.Volume, 0, len(du.Volumes.Items))
	for _, v := range du.Volumes.Items {
		vol := conv.ConvertVolume(v)
		volumes = append(volumes, &vol)
	}
	buildCache := make([]*api.BuildCache, 0, len(du.BuildCache.Items))
	for _, bc := range du.BuildCache.Items {
		entry := ConvertBuildCache(bc)
		buildCache = append(buildCache, &entry)
	}

	// The Docker API's LayersSize is the images' total size.
	return &api.DiskUsageResponse{
		LayersSize: du.Images.TotalSize,
		Images:     images,
		Containers: containers,
		Volumes:    volumes,
		BuildCache: buildCache,
	}, nil
}

// --- Helper types and functions ---

// hijackedRWC wraps a Docker HijackedResponse as an io.ReadWriteCloser.
type hijackedRWC struct {
	resp client.HijackedResponse
}

func (h *hijackedRWC) Read(p []byte) (int, error)  { return h.resp.Reader.Read(p) }
func (h *hijackedRWC) Write(p []byte) (int, error) { return h.resp.Conn.Write(p) }
func (h *hijackedRWC) Close() error                { h.resp.Close(); return nil }

// nopRWC is a no-op ReadWriteCloser for detached exec.
type nopRWC struct{}

func (n *nopRWC) Read([]byte) (int, error)  { return 0, io.EOF }
func (n *nopRWC) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (n *nopRWC) Close() error              { return nil }

// filtersFromMap converts a map[string][]string to the client's Filters.
func filtersFromMap(f map[string][]string) client.Filters {
	args := client.Filters{}
	for k, vals := range f {
		for _, v := range vals {
			args.Add(k, v)
		}
	}
	return args
}

// mapHostConfigToDocker converts api.HostConfig to Docker SDK container.HostConfig.
func mapHostConfigToDocker(hc *api.HostConfig) (*container.HostConfig, error) {
	if hc == nil {
		return nil, nil
	}
	dns, err := parseAddrs("DNS server", hc.DNS)
	if err != nil {
		return nil, err
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
		DNS:        dns,
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
	}
	if hc.ConsoleSize != nil {
		hostConfig.ConsoleSize = *hc.ConsoleSize
	}
	if len(hc.PortBindings) > 0 {
		hostConfig.PortBindings = make(network.PortMap, len(hc.PortBindings))
		for port, bindings := range hc.PortBindings {
			parsed, err := network.ParsePort(port)
			if err != nil {
				return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid port binding %q: %v", port, err)}
			}
			var nb []network.PortBinding
			for _, b := range bindings {
				binding := network.PortBinding{HostPort: b.HostPort}
				if b.HostIP != "" {
					hostIP, err := netip.ParseAddr(b.HostIP)
					if err != nil {
						return nil, &api.InvalidParameterError{Message: fmt.Sprintf("invalid host address %q for port %s: %v", b.HostIP, port, err)}
					}
					binding.HostIP = hostIP
				}
				nb = append(nb, binding)
			}
			hostConfig.PortBindings[parsed] = nb
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
	return hostConfig, nil
}

// mapNetworkingConfigToDocker converts api.NetworkingConfig to Docker SDK network.NetworkingConfig.
func mapNetworkingConfigToDocker(nc *api.NetworkingConfig) (*network.NetworkingConfig, error) {
	if nc == nil || len(nc.EndpointsConfig) == 0 {
		return nil, nil
	}
	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: make(map[string]*network.EndpointSettings, len(nc.EndpointsConfig)),
	}
	for name, ep := range nc.EndpointsConfig {
		es, err := APIEndpointToDocker(ep)
		if err != nil {
			return nil, err
		}
		networkingConfig.EndpointsConfig[name] = es
	}
	return networkingConfig, nil
}

// parseAddr parses one address a client sent as text; an empty string is
// the zero address.
func parseAddr(what, value string) (netip.Addr, error) {
	if value == "" {
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, &api.InvalidParameterError{Message: fmt.Sprintf("invalid %s %q: %v", what, value, err)}
	}
	return addr, nil
}

// parseAddrs parses the addresses a client sent as text.
func parseAddrs(what string, values []string) ([]netip.Addr, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]netip.Addr, 0, len(values))
	for _, v := range values {
		addr, err := parseAddr(what, v)
		if err != nil {
			return nil, err
		}
		out = append(out, addr)
	}
	return out, nil
}

// ---methods ---

// ContainerResize resizes the TTY of a container.
func (s *Server) ContainerResize(id string, h, w int) error {
	_, err := s.docker.ContainerResize(context.Background(), id, client.ContainerResizeOptions{
		Height: uint(h),
		Width:  uint(w),
	})
	return mapDockerError(err)
}

// ExecResize resizes the TTY of an exec instance.
func (s *Server) ExecResize(id string, h, w int) error {
	_, err := s.docker.ExecResize(context.Background(), id, client.ExecResizeOptions{
		Height: uint(h),
		Width:  uint(w),
	})
	return mapDockerError(err)
}

// ContainerPutArchive uploads a tar archive to a container path.
func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	_, err := s.docker.CopyToContainer(context.Background(), id, client.CopyToContainerOptions{
		DestinationPath:           path,
		Content:                   body,
		AllowOverwriteDirWithFile: !noOverwriteDirNonDir,
	})
	return mapDockerError(err)
}

// ContainerStatPath returns stat info for a path in a container.
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	result, err := s.docker.ContainerStatPath(context.Background(), id, client.ContainerStatPathOptions{Path: path})
	if err != nil {
		return nil, mapDockerError(err)
	}
	stat := result.Stat
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
	copied, err := s.docker.CopyFromContainer(context.Background(), id, client.CopyFromContainerOptions{SourcePath: path})
	if err != nil {
		return nil, mapDockerError(err)
	}
	stat, rc := copied.Stat, copied.Content
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
	updateConfig := client.ContainerUpdateOptions{
		Resources: &container.Resources{
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
		RestartPolicy: &container.RestartPolicy{
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
	diff, err := s.docker.ContainerDiff(context.Background(), id, client.ContainerDiffOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}
	var result []api.ContainerChangeItem
	for _, c := range diff.Changes {
		result = append(result, conv.ConvertContainerChange(c))
	}
	if result == nil {
		result = []api.ContainerChangeItem{}
	}
	return result, nil
}

// ContainerExport exports a container's filesystem as a tar stream.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	rc, err := s.docker.ContainerExport(context.Background(), id, client.ContainerExportOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}
	return rc, nil
}

// ImageBuild builds an image from a Dockerfile and build context.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	dockerOpts := client.ImageBuildOptions{
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

	resp, err := s.docker.ImagePush(context.Background(), ref, client.ImagePushOptions{
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
	results, err := s.docker.ImageSearch(context.Background(), term, client.ImageSearchOptions{
		Limit:   limit,
		Filters: filtersFromMap(searchFilters),
	})
	if err != nil {
		return nil, mapDockerError(err)
	}

	mapped := make([]*api.ImageSearchResult, 0, len(results.Items))
	for _, r := range results.Items {
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
	commitOpts := client.ContainerCommitOptions{
		Comment: req.Comment,
		Author:  req.Author,
		NoPause: !req.Pause,
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

// Docker backend synthesises pods via the shared `sockerless-pod`
// label convention. Local Docker daemon has no
// native pod primitive, but sockerless already tracks pods in
// Store.Pods and labels cloud-managed containers with `sockerless-pod`
// for cross-backend consistency. The Docker backend follows the same
// pattern:
//  - Store.Pods (via BaseServer's PodCreate/Inspect/Exists) holds the
//    pod metadata within a backend run.
//  - PodList merges Store.Pods with live Docker containers grouped by
//    the `sockerless-pod` label so `docker pod ls` after a backend
//    restart still reflects containers that survived.
//  - PodStart/Stop/Kill/Remove fan out to the Docker daemon over the
//    SDK — not the in-memory Store.Containers which the BaseServer
//    defaults operate on.

// PodCreate delegates to BaseServer (in-memory registry).
func (s *Server) PodCreate(req *api.PodCreateRequest) (*api.PodCreateResponse, error) {
	return s.BaseServer.PodCreate(req)
}

// PodInspect delegates to BaseServer for metadata, augmenting with live
// docker container membership derived from the sockerless-pod label so
// inspect output reflects containers that survived a restart.
func (s *Server) PodInspect(name string) (*api.PodInspectResponse, error) {
	if pod, ok := s.Store.Pods.GetPod(name); ok {
		return s.BaseServer.PodInspect(pod.Name)
	}
	// Reconstruct from docker containers labelled with the pod name.
	containers, err := s.dockerContainersByPodLabel(context.Background(), name)
	if err != nil {
		return nil, mapDockerError(err)
	}
	if len(containers) == 0 {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}
	infos := make([]api.PodContainerInfo, 0, len(containers))
	state := "stopped"
	for _, c := range containers {
		if c.State == "running" {
			state = "running"
		}
		infos = append(infos, api.PodContainerInfo{ID: c.ID, Name: nameFromDocker(c), State: string(c.State)})
	}
	created := ""
	if len(containers) > 0 {
		created = time.Unix(containers[0].Created, 0).UTC().Format(time.RFC3339Nano)
	}
	return &api.PodInspectResponse{
		ID:            name,
		Name:          name,
		Created:       created,
		State:         state,
		Containers:    infos,
		NumContainers: len(infos),
	}, nil
}

// PodExists reports presence in Store.Pods OR by label lookup against
// the Docker daemon.
func (s *Server) PodExists(name string) (bool, error) {
	if s.Store.Pods.Exists(name) {
		return true, nil
	}
	containers, err := s.dockerContainersByPodLabel(context.Background(), name)
	if err != nil {
		return false, mapDockerError(err)
	}
	return len(containers) > 0, nil
}

// PodList merges in-memory Store.Pods entries with pods reconstructed
// from local docker containers carrying the `sockerless-pod` label.
func (s *Server) PodList(opts api.PodListOptions) ([]*api.PodListEntry, error) {
	result := []*api.PodListEntry{}
	seen := make(map[string]bool)
	// In-memory pods first (these have richer metadata).
	if base, err := s.BaseServer.PodList(opts); err == nil {
		for _, p := range base {
			result = append(result, p)
			seen[p.Name] = true
		}
	}
	// Pods synthesised from container labels for anything Store.Pods
	// doesn't know about (post-restart reconstruction).
	containers, err := s.docker.ContainerList(context.Background(), client.ContainerListOptions{
		All:     true,
		Filters: client.Filters{}.Add("label", "sockerless-pod"),
	})
	if err != nil {
		return result, nil
	}
	groups := make(map[string][]container.Summary)
	for _, c := range containers.Items {
		podName := c.Labels["sockerless-pod"]
		if podName == "" || seen[podName] {
			continue
		}
		groups[podName] = append(groups[podName], c)
	}
	for podName, members := range groups {
		state := "stopped"
		infos := make([]api.PodContainerInfo, 0, len(members))
		for _, c := range members {
			if c.State == "running" {
				state = "running"
			}
			infos = append(infos, api.PodContainerInfo{ID: c.ID, Name: nameFromDocker(c), State: string(c.State)})
		}
		result = append(result, &api.PodListEntry{
			ID:         podName,
			Name:       podName,
			Status:     state,
			Containers: infos,
		})
	}
	return result, nil
}

// PodStart starts all containers in a pod via the Docker daemon.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	ids, err := s.podContainerIDs(context.Background(), name)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	var errs []string
	for _, id := range ids {
		if _, err := s.docker.ContainerStart(ctx, id, client.ContainerStartOptions{}); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if pod, ok := s.Store.Pods.GetPod(name); ok {
		s.Store.Pods.SetStatus(pod.ID, "running")
	}
	return &api.PodActionResponse{ID: name, Errs: errs}, nil
}

// PodStop stops all containers in a pod via the Docker daemon.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	ids, err := s.podContainerIDs(context.Background(), name)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	var errs []string
	for _, id := range ids {
		if _, err := s.docker.ContainerStop(ctx, id, client.ContainerStopOptions{Timeout: timeout}); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if pod, ok := s.Store.Pods.GetPod(name); ok {
		s.Store.Pods.SetStatus(pod.ID, "stopped")
	}
	return &api.PodActionResponse{ID: name, Errs: errs}, nil
}

// PodKill sends a signal to all containers in a pod.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	ids, err := s.podContainerIDs(context.Background(), name)
	if err != nil {
		return nil, err
	}
	if signal == "" {
		signal = "SIGKILL"
	}
	ctx := context.Background()
	var errs []string
	for _, id := range ids {
		if _, err := s.docker.ContainerKill(ctx, id, client.ContainerKillOptions{Signal: signal}); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if pod, ok := s.Store.Pods.GetPod(name); ok {
		s.Store.Pods.SetStatus(pod.ID, "exited")
	}
	return &api.PodActionResponse{ID: name, Errs: errs}, nil
}

// PodRemove removes all containers in a pod and deletes the pod
// metadata from Store.Pods.
func (s *Server) PodRemove(name string, force bool) error {
	ids, err := s.podContainerIDs(context.Background(), name)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, id := range ids {
		if _, err := s.docker.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: force}); err != nil {
			return mapDockerError(err)
		}
	}
	if pod, ok := s.Store.Pods.GetPod(name); ok {
		s.Store.Pods.DeletePod(pod.ID)
	}
	return nil
}

// podContainerIDs returns the container IDs belonging to a pod. Prefers
// Store.Pods, falls back to the sockerless-pod label scan on the Docker
// daemon.
func (s *Server) podContainerIDs(ctx context.Context, name string) ([]string, error) {
	if pod, ok := s.Store.Pods.GetPod(name); ok {
		return pod.ContainerIDs, nil
	}
	containers, err := s.dockerContainersByPodLabel(ctx, name)
	if err != nil {
		return nil, mapDockerError(err)
	}
	if len(containers) == 0 {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}
	ids := make([]string, 0, len(containers))
	for _, c := range containers {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

// dockerContainersByPodLabel queries the Docker daemon for containers
// tagged with `sockerless-pod=<name>`.
func (s *Server) dockerContainersByPodLabel(ctx context.Context, name string) ([]container.Summary, error) {
	listed, err := s.docker.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: client.Filters{}.Add("label", "sockerless-pod="+name),
	})
	if err != nil {
		return nil, err
	}
	return listed.Items, nil
}

// nameFromDocker returns the first Docker name (trimmed of leading "/").
func nameFromDocker(c container.Summary) string {
	if len(c.Names) == 0 {
		return c.ID[:12]
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// Prevent unused import errors
var (
	_ = bytes.NewReader
	_ = sort.Slice
	_ = strings.Contains
)

// ipamConfigToDocker parses an IPAM configuration's addresses into the
// typed prefixes and address the Docker API carries.
func ipamConfigToDocker(c api.IPAMConfig) (network.IPAMConfig, error) {
	var cfg network.IPAMConfig
	var err error
	if c.Subnet != "" {
		if cfg.Subnet, err = netip.ParsePrefix(c.Subnet); err != nil {
			return cfg, &api.InvalidParameterError{Message: fmt.Sprintf("invalid IPAM subnet %q: %v", c.Subnet, err)}
		}
	}
	if c.IPRange != "" {
		if cfg.IPRange, err = netip.ParsePrefix(c.IPRange); err != nil {
			return cfg, &api.InvalidParameterError{Message: fmt.Sprintf("invalid IPAM range %q: %v", c.IPRange, err)}
		}
	}
	if c.Gateway != "" {
		if cfg.Gateway, err = netip.ParseAddr(c.Gateway); err != nil {
			return cfg, &api.InvalidParameterError{Message: fmt.Sprintf("invalid IPAM gateway %q: %v", c.Gateway, err)}
		}
	}
	return cfg, nil
}
