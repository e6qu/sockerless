package core

import (
	"context"
	"strings"

	"github.com/sockerless/api"
)

// ResolveContainerAuto resolves a container reference using the cloud provider
// when available, falling back to the local Store.
// Also checks PendingCreates for containers between create and start.
func (s *BaseServer) ResolveContainerAuto(ctx context.Context, ref string) (api.Container, bool) {
	// Check pending creates first (containers not yet in cloud)
	if c, ok := s.PendingCreates.Get(ref); ok {
		return c, true
	}
	// Check by name in pending
	for _, c := range s.PendingCreates.List() {
		if c.Name == ref || c.Name == "/"+ref {
			return c, true
		}
	}

	// Try cloud provider
	if s.CloudState != nil {
		c, ok, err := s.CloudState.GetContainer(ctx, ref)
		if err == nil && ok {
			return c, true
		}
	}

	// Fall back to local Store
	return s.Store.ResolveContainer(ref)
}

// ResolveContainerIDAuto resolves a reference to a container ID using cloud or local Store.
func (s *BaseServer) ResolveContainerIDAuto(ctx context.Context, ref string) (string, bool) {
	c, ok := s.ResolveContainerAuto(ctx, ref)
	if ok {
		return c.ID, true
	}
	return "", false
}

// ResolveContainer finds a container by full ID, short ID, or name.
func (st *Store) ResolveContainer(ref string) (api.Container, bool) {
	// Try full ID
	if c, ok := st.Containers.Get(ref); ok {
		return c, true
	}
	// Try name mapping
	if id, ok := st.ContainerNames.Get(ref); ok {
		if c, ok := st.Containers.Get(id); ok {
			return c, true
		}
	}
	// Try name with leading /
	if len(ref) > 0 && ref[0] != '/' {
		if id, ok := st.ContainerNames.Get("/" + ref); ok {
			if c, ok := st.Containers.Get(id); ok {
				return c, true
			}
		}
	}
	// Try short ID prefix match
	for _, c := range st.Containers.List() {
		if len(ref) >= 3 && len(c.ID) > len(ref) && c.ID[:len(ref)] == ref {
			return c, true
		}
	}
	return api.Container{}, false
}

// ResolveContainerID resolves a reference to a container ID.
func (st *Store) ResolveContainerID(ref string) (string, bool) {
	c, ok := st.ResolveContainer(ref)
	if !ok {
		return "", false
	}
	return c.ID, true
}

// ResolveNetwork finds a network by full ID, name, or short ID.
func (st *Store) ResolveNetwork(ref string) (api.Network, bool) {
	if n, ok := st.Networks.Get(ref); ok {
		return n, true
	}
	// Try by name
	for _, n := range st.Networks.List() {
		if n.Name == ref {
			return n, true
		}
	}
	// Try short ID
	for _, n := range st.Networks.List() {
		if strings.HasPrefix(n.ID, ref) {
			return n, true
		}
	}
	return api.Network{}, false
}

// ResolveImage finds an image by ID, tag, or name.
func (st *Store) ResolveImage(ref string) (api.Image, bool) {
	if img, ok := st.Images.Get(ref); ok {
		return img, true
	}
	// Try with :latest
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		if img, ok := st.Images.Get(ref + ":latest"); ok {
			return img, true
		}
	}
	// Try prefix match on ID
	for _, img := range st.Images.List() {
		if strings.HasPrefix(img.ID, ref) {
			return img, true
		}
	}
	return api.Image{}, false
}
