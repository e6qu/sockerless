package azf

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// shareToVolume converts a sockerless-managed Azure Files share into a
// Docker-API Volume entry. Mirrors ACA's helper.
func shareToVolume(dockerName, storageAccount string, sh *armstorage.FileShareItem) *api.Volume {
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
			"share":          shareName,
		},
		CreatedAt: created,
	}
}

// Container methods with resolution.

// ContainerChanges lists files modified since container boot via the
// reverse-agent. Phase 98 (BUG-753).
func (s *Server) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	items, err := core.RunContainerChangesViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker diff requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("changes via reverse-agent: %v", err)}
	}
	return items, nil
}

// ContainerGetArchive runs tar via the reverse-agent. Phase 98.
func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	resp, err := core.RunContainerGetArchiveViaAgent(s.reverseAgents, cid, path)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker cp requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
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
		return &api.NotImplementedError{Message: "docker cp requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
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

// ContainerStatPath runs `stat` inside the function container via the
// reverse-agent. Phase 98 (BUG-751).
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	stat, err := core.RunContainerStatPathViaAgent(s.reverseAgents, cid, path)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker container stat requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
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

// ContainerTop runs `ps` inside the function app container via the
// reverse-agent. Phase 98 (BUG-752).
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	resp, err := core.RunContainerTopViaAgent(s.reverseAgents, cid, psArgs)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker top requires a reverse-agent bootstrap inside the function container (SOCKERLESS_CALLBACK_URL); no session registered"}
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

// Exec methods.

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

// Network methods (pass-through to BaseServer).

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

// Phase 94: named-volume operations provision sockerless-managed Azure
// Files shares via azurecommon.FileShareManager (shared with ACA).
// Shares are attached to the function site at ContainerStart time via
// WebApps.UpdateAzureStorageAccounts (see volumes.go).
//
// Host-path bind specs (/h:/c) stay rejected — AZF containers have no
// host filesystem.
func (s *Server) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	if req == nil || req.Name == "" {
		return nil, &api.InvalidParameterError{Message: "volume name is required"}
	}
	shareName, err := s.shareForVolume(s.ctx(), req.Name)
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("provision Azure Files share for %q: %v", req.Name, err)}
	}
	return &api.Volume{
		Name:       req.Name,
		Driver:     "azurefile",
		Mountpoint: fmt.Sprintf("//%s.file.core.windows.net/%s", s.config.StorageAccount, shareName),
		Labels:     req.Labels,
		Scope:      "local",
		Options: map[string]string{
			"storageAccount": s.config.StorageAccount,
			"share":          shareName,
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
			return shareToVolume(name, s.config.StorageAccount, sh), nil
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
		vols = append(vols, shareToVolume(azurecommon.ShareVolumeName(sh), s.config.StorageAccount, sh))
	}
	return &api.VolumeListResponse{Volumes: vols}, nil
}

// VolumeRemove deletes the Azure Files share backing a Docker volume.
// Any AZF site attachments must be removed first — the site's
// azurestorageaccounts dictionary is not rewritten here; operators
// remove the function app (or edit the dictionary) before removing the
// volume.
func (s *Server) VolumeRemove(name string, force bool) error {
	if err := s.deleteShareForVolume(s.ctx(), name); err != nil {
		return &api.ServerError{Message: fmt.Sprintf("delete Azure Files share for %q: %v", name, err)}
	}
	return nil
}

// VolumePrune deletes every sockerless-managed Azure Files share that
// isn't currently referenced by a pending container's binds.
func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	shares, err := s.listManagedShares(s.ctx())
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("list managed Azure Files shares: %v", err)}
	}
	in := s.inUseVolumeNames()
	resp := &api.VolumePruneResponse{}
	for _, sh := range shares {
		name := azurecommon.ShareVolumeName(sh)
		if _, busy := in[name]; busy {
			continue
		}
		if err := s.deleteShareForVolume(s.ctx(), name); err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("delete Azure Files share for %q: %v", name, err)}
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
