package docker

import (
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	"github.com/sockerless/api"
)

// goverter:converter
// goverter:output:file ./converter_gen.go
// goverter:output:package github.com/sockerless/backend-docker
// goverter:extend TimeToRFC3339Nano
// goverter:extend DurationToInt64
// goverter:extend NetworkModeToString
// goverter:extend IsolationToString
// goverter:extend PidModeToString
// goverter:extend IpcModeToString
// goverter:extend UTSModeToString
// goverter:extend UsernsToString
// goverter:extend CgroupnsModeToString
// goverter:extend RestartPolicyModeToString
// goverter:extend MountTypeToString
// goverter:extend PropagationToString
// goverter:extend ConsistencyToString
// goverter:extend PortSetToMap
// goverter:extend PortMapToBindings
// goverter:extend FileModeToUint32
// goverter:extend StrSliceToStrings
// goverter:extend ChangeTypeToInt
// goverter:extend InterfaceToAny
// goverter:extend EventTypeToString
// goverter:extend EventActionToString
// goverter:extend EventActorToAPI
// goverter:extend HealthResultToLog
// goverter:extend HealthResultsToLogs
// goverter:extend HealthToState
// goverter:extend MountPointToAPI
// goverter:extend MountPointsToAPI
// goverter:extend DockerMountToAPI
// goverter:extend DockerMountsToAPI
// goverter:extend LogConfigToAPI
// goverter:extend EndpointSettingsToAPI
// goverter:extend EndpointSettingsMapToAPI
// goverter:extend EndpointIPAMToAPI
type Converter interface {
	// goverter:ignore Config HostConfig NetworkSettings Mounts State AgentAddress AgentToken
	// goverter:map . State | MapContainerState
	ConvertContainerBase(source types.ContainerJSONBase) api.Container

	ConvertContainerState(source types.ContainerState) api.ContainerState

	// goverter:ignore Healthcheck ExposedPorts
	ConvertContainerConfig(source container.Config) api.ContainerConfig

	ConvertHealthcheckConfig(source container.HealthConfig) api.HealthcheckConfig

	// HostConfig conversion is manual due to embedded Resources struct + complex sub-types

	ConvertRestartPolicy(source container.RestartPolicy) api.RestartPolicy

	ConvertImageSummary(source image.Summary) api.ImageSummary

	// goverter:ignore Config RootFS Metadata
	ConvertImageBase(source types.ImageInspect) api.Image

	ConvertVolume(source volume.Volume) api.Volume

	ConvertEndpointResource(source network.EndpointResource) api.EndpointResource

	// goverter:ignore AuxiliaryAddresses
	ConvertIPAMConfig(source network.IPAMConfig) api.IPAMConfig

	ConvertEventMessage(source events.Message) api.Event

	ConvertContainerChange(source container.FilesystemChange) api.ContainerChangeItem

	ConvertPort(source types.Port) api.Port

	ConvertImageDeleteResponseItem(source image.DeleteResponse) api.ImageDeleteResponse

	ConvertImageHistoryResponseItem(source image.HistoryResponseItem) api.ImageHistoryEntry

	ConvertAuthResponse(source registry.AuthenticateOKBody) api.AuthResponse
}

// MapContainerState extracts ContainerState from ContainerJSONBase.
// This function exists to handle the pointer-to-value conversion.
func MapContainerState(source types.ContainerJSONBase) api.ContainerState {
	if source.State == nil {
		return api.ContainerState{}
	}
	return api.ContainerState{
		Status:     source.State.Status,
		Running:    source.State.Running,
		Paused:     source.State.Paused,
		Restarting: source.State.Restarting,
		OOMKilled:  source.State.OOMKilled,
		Dead:       source.State.Dead,
		Pid:        source.State.Pid,
		ExitCode:   source.State.ExitCode,
		Error:      source.State.Error,
		StartedAt:  source.State.StartedAt,
		FinishedAt: source.State.FinishedAt,
		Health:     HealthToState(source.State.Health),
	}
}

// --- Extend functions: type alias conversions ---

func TimeToRFC3339Nano(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

func DurationToInt64(d time.Duration) int64 {
	return int64(d)
}

func NetworkModeToString(m container.NetworkMode) string {
	return string(m)
}

func IsolationToString(i container.Isolation) string {
	return string(i)
}

func PidModeToString(m container.PidMode) string {
	return string(m)
}

func IpcModeToString(m container.IpcMode) string {
	return string(m)
}

func UTSModeToString(m container.UTSMode) string {
	return string(m)
}

func UsernsToString(m container.UsernsMode) string {
	return string(m)
}

func CgroupnsModeToString(m container.CgroupnsMode) string {
	return string(m)
}

func RestartPolicyModeToString(m container.RestartPolicyMode) string {
	return string(m)
}

func MountTypeToString(t mount.Type) string {
	return string(t)
}

func PropagationToString(p mount.Propagation) string {
	return string(p)
}

func ConsistencyToString(c mount.Consistency) string {
	return string(c)
}

func FileModeToUint32(m os.FileMode) uint32 {
	return uint32(m)
}

func StrSliceToStrings(s []string) []string {
	return s
}

func ChangeTypeToInt(c container.ChangeType) int {
	return int(c)
}

func EventTypeToString(t events.Type) string {
	return string(t)
}

func EventActionToString(a events.Action) string {
	return string(a)
}

func InterfaceToAny(v interface{}) any {
	return v
}

func EventActorToAPI(a events.Actor) api.EventActor {
	return api.EventActor{
		ID:         a.ID,
		Attributes: a.Attributes,
	}
}

// --- Extend functions: complex type conversions ---

func PortSetToMap(ports nat.PortSet) map[string]struct{} {
	if len(ports) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(ports))
	for port := range ports {
		result[string(port)] = struct{}{}
	}
	return result
}

func PortMapToBindings(pm nat.PortMap) map[string][]api.PortBinding {
	if len(pm) == 0 {
		return nil
	}
	result := make(map[string][]api.PortBinding, len(pm))
	for port, bindings := range pm {
		var mapped []api.PortBinding
		for _, b := range bindings {
			mapped = append(mapped, api.PortBinding{
				HostIP:   b.HostIP,
				HostPort: b.HostPort,
			})
		}
		result[string(port)] = mapped
	}
	return result
}

func HealthToState(h *types.Health) *api.HealthState {
	if h == nil {
		return nil
	}
	return &api.HealthState{
		Status:        h.Status,
		FailingStreak: h.FailingStreak,
		Log:           HealthResultsToLogs(h.Log),
	}
}

func HealthResultsToLogs(results []*types.HealthcheckResult) []api.HealthLog {
	logs := make([]api.HealthLog, 0, len(results))
	for _, r := range results {
		if r != nil {
			logs = append(logs, HealthResultToLog(*r))
		}
	}
	return logs
}

func HealthResultToLog(r types.HealthcheckResult) api.HealthLog {
	return api.HealthLog{
		Start:    r.Start.Format(time.RFC3339Nano),
		End:      r.End.Format(time.RFC3339Nano),
		ExitCode: r.ExitCode,
		Output:   r.Output,
	}
}

func MountPointToAPI(m types.MountPoint) api.MountPoint {
	return api.MountPoint{
		Type:        string(m.Type),
		Name:        m.Name,
		Source:      m.Source,
		Destination: m.Destination,
		Driver:      m.Driver,
		Mode:        m.Mode,
		RW:          m.RW,
		Propagation: string(m.Propagation),
	}
}

func MountPointsToAPI(mounts []types.MountPoint) []api.MountPoint {
	result := make([]api.MountPoint, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, MountPointToAPI(m))
	}
	return result
}

func DockerMountToAPI(m mount.Mount) api.Mount {
	am := api.Mount{
		Type:        string(m.Type),
		Source:      m.Source,
		Target:      m.Target,
		ReadOnly:    m.ReadOnly,
		Consistency: string(m.Consistency),
	}
	if m.BindOptions != nil {
		am.BindOptions = &api.BindOptions{
			Propagation: string(m.BindOptions.Propagation),
		}
	}
	if m.VolumeOptions != nil {
		am.VolumeOptions = &api.VolumeOptions{
			NoCopy: m.VolumeOptions.NoCopy,
			Labels: m.VolumeOptions.Labels,
		}
		if m.VolumeOptions.DriverConfig != nil {
			am.VolumeOptions.DriverConfig = &api.VolumeDriverConfig{
				Name:    m.VolumeOptions.DriverConfig.Name,
				Options: m.VolumeOptions.DriverConfig.Options,
			}
		}
	}
	if m.TmpfsOptions != nil {
		am.TmpfsOptions = &api.TmpfsOptions{
			SizeBytes: m.TmpfsOptions.SizeBytes,
			Mode:      uint32(m.TmpfsOptions.Mode),
		}
	}
	return am
}

func DockerMountsToAPI(mounts []mount.Mount) []api.Mount {
	if len(mounts) == 0 {
		return nil
	}
	result := make([]api.Mount, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, DockerMountToAPI(m))
	}
	return result
}

func LogConfigToAPI(lc container.LogConfig) api.LogConfig {
	return api.LogConfig{
		Type:   lc.Type,
		Config: lc.Config,
	}
}

func EndpointIPAMToAPI(c *network.EndpointIPAMConfig) *api.EndpointIPAMConfig {
	if c == nil {
		return nil
	}
	return &api.EndpointIPAMConfig{
		IPv4Address:  c.IPv4Address,
		IPv6Address:  c.IPv6Address,
		LinkLocalIPs: c.LinkLocalIPs,
	}
}

func EndpointSettingsToAPI(ep *network.EndpointSettings) *api.EndpointSettings {
	if ep == nil {
		return nil
	}
	return &api.EndpointSettings{
		IPAMConfig:          EndpointIPAMToAPI(ep.IPAMConfig),
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
}

func EndpointSettingsMapToAPI(m map[string]*network.EndpointSettings) map[string]*api.EndpointSettings {
	if m == nil {
		return nil
	}
	result := make(map[string]*api.EndpointSettings, len(m))
	for k, v := range m {
		result[k] = EndpointSettingsToAPI(v)
	}
	return result
}

// Prevent unused import errors
var (
	_ = events.Message{}
	_ = image.Summary{}
	_ = volume.Volume{}
)
