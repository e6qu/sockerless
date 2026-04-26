package lambda

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// accessPointToVolume converts an EFS AccessPointDescription into the
// Docker-API volume shape. Mirrors ECS's helper.
func accessPointToVolume(ap efstypes.AccessPointDescription) *api.Volume {
	name := awscommon.APVolumeName(ap)
	root := ""
	if ap.RootDirectory != nil {
		root = aws.ToString(ap.RootDirectory.Path)
	}
	return &api.Volume{
		Name:       name,
		Driver:     "efs",
		Mountpoint: root,
		Scope:      "local",
		Options: map[string]string{
			"accessPointId": aws.ToString(ap.AccessPointId),
			"fileSystemId":  aws.ToString(ap.FileSystemId),
		},
		CreatedAt: "",
	}
}

// --- Container methods requiring resolution ---

// ContainerChanges lists files modified since container boot via the
// reverse-agent (find + /proc/1 baseline).
func (s *Server) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	items, err := core.RunContainerChangesViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker diff requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("changes via reverse-agent: %v", err)}
	}
	return items, nil
}

// ContainerGetArchive runs `tar -cf - -C <parent> <name>` inside the
// container via the reverse-agent.
func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	resp, err := core.RunContainerGetArchiveViaAgent(s.reverseAgents, cid, path)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker cp requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("archive via reverse-agent: %v", err)}
	}
	return resp, nil
}

func (s *Server) ContainerInspect(id string) (*api.Container, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerInspect(id)
}

// ContainerPutArchive extracts the incoming tar body into <path> via
// the reverse-agent.
func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	err := core.RunContainerPutArchiveViaAgent(s.reverseAgents, cid, path, body)
	if err == core.ErrNoReverseAgent {
		return &api.NotImplementedError{Message: "docker cp requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return &api.ServerError{Message: fmt.Sprintf("put-archive via reverse-agent: %v", err)}
	}
	return nil
}

func (s *Server) ContainerRename(id string, newName string) error {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerRename(id, newName)
}

func (s *Server) ContainerResize(id string, h int, w int) error {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerResize(id, h, w)
}

// ContainerStatPath runs `stat` inside the container via the
// reverse-agent and parses the output.
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	stat, err := core.RunContainerStatPathViaAgent(s.reverseAgents, cid, path)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker container stat requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("stat via reverse-agent: %v", err)}
	}
	return stat, nil
}

func (s *Server) ContainerStats(id string, stream bool) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerStats(id, stream)
}

// ContainerTop runs `ps` inside the container via the reverse-agent
// and parses the output. Requires a bootstrap / agent running inside
// the container; returns ErrNoReverseAgent when no session is
// registered.
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	resp, err := core.RunContainerTopViaAgent(s.reverseAgents, cid, psArgs)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker top requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("top via reverse-agent: %v", err)}
	}
	return resp, nil
}

func (s *Server) ContainerUpdate(id string, req *api.ContainerUpdateRequest) (*api.ContainerUpdateResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerUpdate(id, req)
}

func (s *Server) ContainerWait(id string, condition string) (*api.ContainerWaitResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerWait(id, condition)
}

func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), containerID); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}
	return s.BaseServer.ExecCreate(containerID, req)
}

// --- Exec methods (exec ID, not container ID) ---

func (s *Server) ExecInspect(id string) (*api.ExecInstance, error) {
	return s.BaseServer.ExecInspect(id)
}

func (s *Server) ExecResize(id string, h int, w int) error {
	return s.BaseServer.ExecResize(id, h, w)
}

// ExecStart runs the exec inside the container via the reverse-agent
// WebSocket. Lambda exposes no native exec API; the bootstrap is the
// only path. If no session is registered, return NotImplementedError
// with the specific reason rather than letting the driver silently
// return exit code 126.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}
	c, ok := s.ResolveContainerAuto(context.Background(), exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID)}
	}
	if _, hasAgent := s.reverseAgents.Resolve(c.ID); !hasAgent {
		return nil, &api.NotImplementedError{Message: "docker exec requires a reverse-agent bootstrap inside the Lambda container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	return s.BaseServer.ExecStart(id, opts)
}

// --- Non-container pass-through methods ---

func (s *Server) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	return s.BaseServer.ContainerList(opts)
}

func (s *Server) ImageHistory(name string) ([]*api.ImageHistoryEntry, error) {
	return s.images.History(name)
}

func (s *Server) ImageInspect(name string) (*api.Image, error) {
	return s.images.Inspect(name)
}

func (s *Server) ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
	return s.images.List(opts)
}

func (s *Server) ImagePrune(filters map[string][]string) (*api.ImagePruneResponse, error) {
	return s.images.Prune(filters)
}

func (s *Server) ImageSave(names []string) (io.ReadCloser, error) {
	return s.images.Save(names)
}

func (s *Server) ImageSearch(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error) {
	return s.images.Search(term, limit, filters)
}

func (s *Server) NetworkConnect(id string, req *api.NetworkConnectRequest) error {
	return s.BaseServer.NetworkConnect(id, req)
}

func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	return s.BaseServer.NetworkCreate(req)
}

func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	return s.BaseServer.NetworkDisconnect(id, req)
}

func (s *Server) NetworkInspect(id string) (*api.Network, error) {
	return s.BaseServer.NetworkInspect(id)
}

func (s *Server) NetworkList(filters map[string][]string) ([]*api.Network, error) {
	return s.BaseServer.NetworkList(filters)
}

func (s *Server) NetworkPrune(filters map[string][]string) (*api.NetworkPruneResponse, error) {
	return s.BaseServer.NetworkPrune(filters)
}

func (s *Server) NetworkRemove(id string) error {
	return s.BaseServer.NetworkRemove(id)
}

func (s *Server) PodCreate(req *api.PodCreateRequest) (*api.PodCreateResponse, error) {
	return s.BaseServer.PodCreate(req)
}

func (s *Server) PodExists(name string) (bool, error) {
	return s.BaseServer.PodExists(name)
}

func (s *Server) PodInspect(name string) (*api.PodInspectResponse, error) {
	return s.BaseServer.PodInspect(name)
}

func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	return s.BaseServer.PodKill(name, signal)
}

func (s *Server) PodList(opts api.PodListOptions) ([]*api.PodListEntry, error) {
	return s.BaseServer.PodList(opts)
}

func (s *Server) PodRemove(name string, force bool) error {
	return s.BaseServer.PodRemove(name, force)
}

func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	return s.BaseServer.PodStart(name)
}

func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	return s.BaseServer.PodStop(name, timeout)
}

func (s *Server) SystemDf() (*api.DiskUsageResponse, error) {
	return s.BaseServer.SystemDf()
}

func (s *Server) SystemEvents(opts api.EventsOptions) (io.ReadCloser, error) {
	return s.BaseServer.SystemEvents(opts)
}

// Named-volume operations provision sockerless-managed EFS access
// points via awscommon.EFSManager (shared with ECS). Lambda
// attaches them at CreateFunction time via Function.FileSystemConfigs[].
// Named volumes require the function to run in a VPC with mount targets
// in matching subnets — `SOCKERLESS_LAMBDA_SUBNETS` must be set.
//
// Host-path bind specs (`/h:/c`) stay rejected — Lambda containers have
// no host filesystem.
func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	if req == nil || req.Name == "" {
		return nil, &api.InvalidParameterError{Message: "volume name is required"}
	}
	apID, err := s.accessPointForVolume(s.ctx(), req.Name)
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("provision EFS access point for %q: %v", req.Name, err)}
	}
	fsID, err := s.efs.EnsureFilesystem(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("resolve EFS filesystem for %q: %v", req.Name, err)}
	}
	return &api.Volume{
		Name:       req.Name,
		Driver:     "efs",
		Mountpoint: "/sockerless/volumes/" + awscommon.SanitiseVolumePath(req.Name),
		Labels:     req.Labels,
		Scope:      "local",
		Options:    map[string]string{"accessPointId": apID, "fileSystemId": fsID},
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Server) VolumeInspect(name string) (*api.Volume, error) {
	aps, err := s.listManagedAccessPoints(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list EFS access points: %v", err)}
	}
	for _, ap := range aps {
		if awscommon.APVolumeName(ap) == name {
			return accessPointToVolume(ap), nil
		}
	}
	return nil, &api.NotFoundError{Resource: "volume", ID: name}
}

func (s *Server) VolumeList(filters map[string][]string) (*api.VolumeListResponse, error) {
	aps, err := s.listManagedAccessPoints(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list EFS access points: %v", err)}
	}
	vols := make([]*api.Volume, 0, len(aps))
	for _, ap := range aps {
		vols = append(vols, accessPointToVolume(ap))
	}
	return &api.VolumeListResponse{Volumes: vols}, nil
}

// VolumeRemove deletes the EFS access point backing a volume. The
// underlying filesystem is left in place so other volumes keep working.
func (s *Server) VolumeRemove(name string, force bool) error {
	if err := s.deleteAccessPointForVolume(s.ctx(), name); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("delete EFS access point for %q: %v", name, err)}
	}
	return nil
}

// VolumePrune deletes every sockerless-managed access point not
// referenced by a pending container's binds.
func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	aps, err := s.listManagedAccessPoints(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list EFS access points: %v", err)}
	}
	in := s.inUseVolumeNames()
	resp := &api.VolumePruneResponse{}
	for _, ap := range aps {
		name := awscommon.APVolumeName(ap)
		if _, busy := in[name]; busy {
			continue
		}
		if err := s.deleteAccessPointForVolume(s.ctx(), name); err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("delete EFS access point for %q: %v", name, err)}
		}
		resp.VolumesDeleted = append(resp.VolumesDeleted, name)
	}
	return resp, nil
}

// inUseVolumeNames returns Docker volume names currently referenced by
// pending container binds.
func (s *Server) inUseVolumeNames() map[string]struct{} {
	in := make(map[string]struct{})
	for _, c := range s.PendingCreates.List() {
		for _, b := range c.HostConfig.Binds {
			parts := strings.SplitN(b, ":", 3)
			if len(parts) >= 2 && !strings.HasPrefix(parts[0], "/") {
				in[parts[0]] = struct{}{}
			}
		}
	}
	return in
}
