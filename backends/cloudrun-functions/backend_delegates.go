package gcf

import (
	"context"
	"io"

	"github.com/sockerless/api"
)

// Auth methods (pass-through)

func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	return s.BaseServer.AuthLogin(req)
}

// Container methods with resolution

func (s *Server) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerChanges(id)
}

func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerGetArchive(id, path)
}

func (s *Server) ContainerInspect(id string) (*api.Container, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerInspect(id)
}

func (s *Server) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	return s.BaseServer.ContainerList(opts)
}

func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerPutArchive(id, path, noOverwriteDirNonDir, body)
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
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerStatPath(id, path)
}

func (s *Server) ContainerStats(id string, stream bool) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerStats(id, stream)
}

func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerTop(id, psArgs)
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

// Exec methods

func (s *Server) ExecCreate(containerID string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), containerID); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: containerID}
	}
	return s.BaseServer.ExecCreate(containerID, req)
}

func (s *Server) ExecInspect(id string) (*api.ExecInstance, error) {
	return s.BaseServer.ExecInspect(id)
}

func (s *Server) ExecResize(id string, h int, w int) error {
	return s.BaseServer.ExecResize(id, h, w)
}

func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	return s.BaseServer.ExecStart(id, opts)
}

// Image methods (pass-through via ImageManager)

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

// Network methods (pass-through)

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

// Pod methods (pass-through)

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

func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	return s.BaseServer.PodStop(name, timeout)
}

// System methods (pass-through)

func (s *Server) SystemDf() (*api.DiskUsageResponse, error) {
	return s.BaseServer.SystemDf()
}

func (s *Server) SystemEvents(opts api.EventsOptions) (io.ReadCloser, error) {
	return s.BaseServer.SystemEvents(opts)
}

// Volume methods (pass-through)

func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	return s.BaseServer.VolumeCreate(req)
}

func (s *Server) VolumeInspect(name string) (*api.Volume, error) {
	return s.BaseServer.VolumeInspect(name)
}

func (s *Server) VolumeList(filters map[string][]string) (*api.VolumeListResponse, error) {
	return s.BaseServer.VolumeList(filters)
}

func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	return s.BaseServer.VolumePrune(filters)
}

func (s *Server) VolumeRemove(name string, force bool) error {
	return s.BaseServer.VolumeRemove(name, force)
}
