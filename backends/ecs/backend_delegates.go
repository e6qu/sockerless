package ecs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
)

// accessPointToVolume converts an EFS AccessPointDescription into the
// Docker-API volume shape clients expect from `docker volume inspect`
// and `docker volume ls`.
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
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// --- Container methods requiring resolution ---

// ContainerAttach is implemented in attach.go — overrides the BaseServer
// delegation so cloud backend streams CloudWatch logs instead of
// returning an immediately-EOF pipe.

func (s *Server) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	return s.ContainerChangesViaSSM(id)
}

func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	return s.ContainerGetArchiveViaSSM(id, path)
}

func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	resolvedID, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}

	// If the container has a running ECS task and we're running
	// against the simulator (EndpointURL set), prefer the sim's
	// helper endpoint that pipes the tar straight into the Docker
	// container — faster than round-tripping through SSM frames.
	if ecsState, ok := s.ECS.Get(resolvedID); ok && ecsState.TaskARN != "" && s.config.EndpointURL != "" {
		taskID := extractTaskIDFromARN(ecsState.TaskARN)
		archiveURL := fmt.Sprintf("%s/sockerless/tasks/%s/archive?path=%s",
			s.config.EndpointURL, taskID, url.QueryEscape(path))
		req, err := http.NewRequest("PUT", archiveURL, body)
		if err != nil {
			return fmt.Errorf("failed to create archive request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-tar")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to forward archive to simulator: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("simulator archive upload failed (%d): %s", resp.StatusCode, string(respBody))
		}
		return nil
	}

	// Real ECS path: pipe the tar body through SSM ExecuteCommand
	// running `tar -xf - -C <path>` inside the task.
	return s.ContainerPutArchiveViaSSM(resolvedID, path, body)
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

func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	return s.ContainerStatPathViaSSM(id, path)
}

func (s *Server) ContainerStats(id string, stream bool) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerStats(id, stream)
}

func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	return s.ContainerTopViaSSM(id, psArgs)
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

// --- Exec methods (exec ID, not container ID) ---

func (s *Server) ExecInspect(id string) (*api.ExecInstance, error) {
	return s.BaseServer.ExecInspect(id)
}

func (s *Server) ExecResize(id string, h int, w int) error {
	return s.BaseServer.ExecResize(id, h, w)
}

// --- Non-container pass-through methods ---

func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	return s.BaseServer.AuthLogin(req)
}

func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return s.images.Build(opts, buildContext)
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

func (s *Server) PodCreate(req *api.PodCreateRequest) (*api.PodCreateResponse, error) {
	return s.BaseServer.PodCreate(req)
}

func (s *Server) PodExists(name string) (bool, error) {
	return s.BaseServer.PodExists(name)
}

func (s *Server) PodInspect(name string) (*api.PodInspectResponse, error) {
	return s.BaseServer.PodInspect(name)
}

func (s *Server) PodList(opts api.PodListOptions) ([]*api.PodListEntry, error) {
	return s.BaseServer.PodList(opts)
}

func (s *Server) SystemDf() (*api.DiskUsageResponse, error) {
	return s.BaseServer.SystemDf()
}

func (s *Server) SystemEvents(opts api.EventsOptions) (io.ReadCloser, error) {
	return s.BaseServer.SystemEvents(opts)
}

// Named-volume operations map to EFS access points on a sockerless-owned
// EFS filesystem (Phase 91). Each volume is backed by its own access
// point + subdirectory so containers see real, persistent state. If the
// operator provides `SOCKERLESS_ECS_AGENT_EFS_ID` the filesystem is
// reused; otherwise one is created lazily on first use.
func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	if req == nil || req.Name == "" {
		return nil, &api.InvalidParameterError{Message: "volume name is required"}
	}
	apID, err := s.accessPointForVolume(s.ctx(), req.Name)
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("provision EFS access point for %q: %v", req.Name, err)}
	}
	fsID, err := s.ensureEFSFilesystem(s.ctx())
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
