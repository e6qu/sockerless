package aca

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// shareToVolume converts a sockerless-managed Azure Files share into
// a Docker-API Volume entry.
func shareToVolume(dockerName, storageAccount, environment string, sh *armstorage.FileShareItem) *api.Volume {
	shareName := ""
	if sh.Name != nil {
		shareName = *sh.Name
	}
	created := ""
	if sh.Properties != nil && sh.Properties.LastModifiedTime != nil {
		created = sh.Properties.LastModifiedTime.UTC().Format(time.RFC3339Nano)
	}
	return &api.Volume{
		Name:       dockerName,
		Driver:     "azurefile",
		Mountpoint: fmt.Sprintf("//%s.file.core.windows.net/%s", storageAccount, shareName),
		Scope:      "local",
		Options: map[string]string{
			"storageAccount": storageAccount,
			"shareName":      shareName,
			"environment":    environment,
		},
		CreatedAt: created,
	}
}

// Container methods with resolution.

func (s *Server) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return s.BaseServer.ContainerChanges(id)
}

// ContainerGetArchive runs tar via the reverse-agent. Phase 98.
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

func (s *Server) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	return s.BaseServer.ContainerList(opts)
}

// ContainerPutArchive extracts the incoming tar body via the
// reverse-agent. Phase 98.
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

// ContainerStatPath runs `stat` inside the ACA job via the reverse-
// agent. Phase 98 (BUG-751).
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
// and parses the output. Phase 98 (BUG-752). Requires a bootstrap
// inside the container (SOCKERLESS_CALLBACK_URL).
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

// Exec methods (pass-through, exec IDs not container IDs).

func (s *Server) ExecInspect(id string) (*api.ExecInstance, error) {
	return s.BaseServer.ExecInspect(id)
}

func (s *Server) ExecResize(id string, h int, w int) error {
	return s.BaseServer.ExecResize(id, h, w)
}

// Image methods (delegate to ImageManager).

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

// Pod methods (pass-through to BaseServer).

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

// System methods (pass-through to BaseServer).

func (s *Server) SystemDf() (*api.DiskUsageResponse, error) {
	return s.BaseServer.SystemDf()
}

func (s *Server) SystemEvents(opts api.EventsOptions) (io.ReadCloser, error) {
	return s.BaseServer.SystemEvents(opts)
}

// Named-volume operations map to Azure Files shares inside the
// operator-configured storage account (Phase 93). Each volume gets
// a dedicated share, and a matching `ManagedEnvironmentsStorages`
// entry links the share to the env so Apps/Jobs can reference it by
// storage name at container-start time.
func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	if req == nil || req.Name == "" {
		return nil, &api.InvalidParameterError{Message: "volume name is required"}
	}
	share, err := s.shareForVolume(s.ctx(), req.Name)
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("provision Azure Files share for %q: %v", req.Name, err)}
	}
	return &api.Volume{
		Name:       req.Name,
		Driver:     "azurefile",
		Mountpoint: fmt.Sprintf("//%s.file.core.windows.net/%s", s.config.StorageAccount, share),
		Labels:     req.Labels,
		Scope:      "local",
		Options: map[string]string{
			"storageAccount": s.config.StorageAccount,
			"shareName":      share,
			"environment":    s.config.Environment,
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Server) VolumeInspect(name string) (*api.Volume, error) {
	shares, err := s.listManagedShares(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list managed Azure Files shares: %v", err)}
	}
	for _, sh := range shares {
		if azurecommon.ShareVolumeName(sh) == azurecommon.SanitiseMetaValue(name) {
			return shareToVolume(name, s.config.StorageAccount, s.config.Environment, sh), nil
		}
	}
	return nil, &api.NotFoundError{Resource: "volume", ID: name}
}

func (s *Server) VolumeList(filters map[string][]string) (*api.VolumeListResponse, error) {
	shares, err := s.listManagedShares(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list managed Azure Files shares: %v", err)}
	}
	vols := make([]*api.Volume, 0, len(shares))
	for _, sh := range shares {
		vols = append(vols, shareToVolume(azurecommon.ShareVolumeName(sh), s.config.StorageAccount, s.config.Environment, sh))
	}
	return &api.VolumeListResponse{Volumes: vols}, nil
}
