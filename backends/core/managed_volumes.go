package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sockerless/api"
)

// Every cloud backend stores a named Docker volume as one cloud resource
// (an EFS access point, a GCS bucket, an Azure Files share) that it marks
// as its own and labels with the Docker volume name. These are the two
// labels, and the Docker-API shaping over a listing of such resources.
const (
	// ManagedVolumeTagKey marks a cloud resource as a sockerless volume.
	ManagedVolumeTagKey = "sockerless-managed"
	// ManagedVolumeTagValue is the value ManagedVolumeTagKey carries.
	ManagedVolumeTagValue = "true"
	// VolumeNameTagKey carries the Docker volume name on the resource.
	VolumeNameTagKey = "sockerless-volume-name"
)

// InUseVolumeNames returns the named volumes referenced by the binds of
// containers that are created but not yet started. A prune must keep
// those, because the cloud has not seen the container yet.
func InUseVolumeNames(pending []api.Container) map[string]struct{} {
	in := make(map[string]struct{})
	for _, c := range pending {
		for _, b := range c.HostConfig.Binds {
			parts := strings.SplitN(b, ":", 3)
			if len(parts) >= 2 && !IsHostBindSource(parts[0]) {
				in[parts[0]] = struct{}{}
			}
		}
	}
	return in
}

// ListManagedVolumes shapes a listing of cloud resources as the Docker
// volume list.
func ListManagedVolumes[T any](items []T, toVolume func(T) *api.Volume) *api.VolumeListResponse {
	vols := make([]*api.Volume, 0, len(items))
	for _, it := range items {
		vols = append(vols, toVolume(it))
	}
	return &api.VolumeListResponse{Volumes: vols}
}

// InspectManagedVolume finds the resource whose volume name matches and
// shapes it as a Docker volume, or answers the Docker not-found error.
func InspectManagedVolume[T any](items []T, name string, matches func(T) bool, toVolume func(T) *api.Volume) (*api.Volume, error) {
	for _, it := range items {
		if matches(it) {
			return toVolume(it), nil
		}
	}
	return nil, &api.NotFoundError{Resource: "volume", ID: name}
}

// PruneManagedVolumes deletes every listed resource whose volume is not in
// use and reports the names removed. resource names the cloud primitive
// for the error text, e.g. "EFS access point".
func PruneManagedVolumes[T any](items []T, nameOf func(T) string, inUse map[string]struct{}, del func(name string) error, resource string) (*api.VolumePruneResponse, error) {
	resp := &api.VolumePruneResponse{}
	for _, it := range items {
		name := nameOf(it)
		if _, busy := inUse[name]; busy {
			continue
		}
		if err := del(name); err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("delete %s for %q: %v", resource, name, err)}
		}
		resp.VolumesDeleted = append(resp.VolumesDeleted, name)
	}
	return resp, nil
}

// ContainerNameTaken reports whether any listed container carries name,
// with or without Docker's leading slash.
func ContainerNameTaken(containers []api.Container, name string) bool {
	for _, c := range containers {
		if c.Name == name || c.Name == "/"+name {
			return true
		}
	}
	return false
}

// ProvisionCache memoises the cloud resource identifier provisioned for
// each volume name. Ensure serialises callers so two concurrent creates
// of the same volume provision one resource.
type ProvisionCache struct {
	mu    sync.Mutex
	byVol map[string]string
}

// Ensure returns the cached identifier for volName, or finds an existing
// resource and, failing that, creates one. find reports (id, found, err);
// create returns the new id.
func (c *ProvisionCache) Ensure(volName string, find func() (string, bool, error), create func() (string, error)) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byVol == nil {
		c.byVol = make(map[string]string)
	}
	if id, ok := c.byVol[volName]; ok {
		return id, nil
	}
	id, found, err := find()
	if err != nil {
		return "", err
	}
	if !found {
		if id, err = create(); err != nil {
			return "", err
		}
	}
	c.byVol[volName] = id
	return id, nil
}

// Invalidate drops the cached identifier for volName so the next Ensure
// looks the resource up again. Used when a step after provisioning failed
// and both steps must be retried.
func (c *ProvisionCache) Invalidate(volName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byVol, volName)
}

// Forget drops the cached identifier for volName after the resource is
// deleted, and runs del while holding the same lock Ensure takes so a
// concurrent Ensure cannot re-cache a resource being removed.
func (c *ProvisionCache) Forget(volName string, del func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := del(); err != nil {
		return err
	}
	delete(c.byVol, volName)
	return nil
}
