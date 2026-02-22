package core

import (
	"strings"

	"github.com/sockerless/api"
)

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
