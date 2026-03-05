package core

import (
	"archive/tar"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// handlePutArchive accepts a tar archive and extracts it to the container filesystem.
// Dispatches through the FilesystemDriver chain (WASM → agent → staging).
func (s *BaseServer) handlePutArchive(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	if err := s.Drivers.Filesystem.PutArchive(id, path, r.Body); err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("failed to extract archive")
		WriteError(w, &api.ServerError{Message: "failed to extract archive: " + err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleHeadArchive returns a stat header for the requested path.
// Dispatches through the FilesystemDriver chain.
func (s *BaseServer) handleHeadArchive(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	info, err := s.Drivers.Filesystem.StatPath(id, path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stat := map[string]interface{}{
		"name":  info.Name(),
		"size":  info.Size(),
		"mode":  info.Mode().Perm(),
		"mtime": info.ModTime().UTC().Format(time.RFC3339),
	}
	statJSON, _ := json.Marshal(stat)
	w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(statJSON))
	w.WriteHeader(http.StatusOK)
}

// handleGetArchive returns a tar archive of the requested path.
// Dispatches through the FilesystemDriver chain.
func (s *BaseServer) handleGetArchive(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	// Stat first to check existence and get info for the header
	info, err := s.Drivers.Filesystem.StatPath(id, path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stat := map[string]interface{}{
		"name":  info.Name(),
		"size":  info.Size(),
		"mode":  info.Mode().Perm(),
		"mtime": info.ModTime().UTC().Format(time.RFC3339),
	}
	statJSON, _ := json.Marshal(stat)
	w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(statJSON))
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	if _, err := s.Drivers.Filesystem.GetArchive(id, path, w); err != nil {
		s.Logger.Error().Err(err).Str("container", id).Str("path", path).Msg("failed to write archive")
	}
}

// extractTar extracts a tar archive into destDir.
func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, filepath.Clean(hdr.Name))

		// Prevent path traversal: ensure target stays within destDir
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			_ = f.Close()
		case tar.TypeSymlink:
			// Validate symlink target stays within destDir
			linkTarget := hdr.Linkname
			if !filepath.IsAbs(linkTarget) {
				linkTarget = filepath.Join(filepath.Dir(target), linkTarget)
			}
			cleanLink := filepath.Clean(linkTarget)
			if !strings.HasPrefix(cleanLink+string(os.PathSeparator), filepath.Clean(destDir)+string(os.PathSeparator)) {
				continue // skip symlinks that escape destDir
			}
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			_ = os.Remove(target)
			_ = os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			linkTarget := filepath.Join(destDir, filepath.Clean(hdr.Linkname))
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			_ = os.Remove(target)
			_ = os.Link(linkTarget, target)
		}
	}
}

// mergeStagingDir copies pre-start archive files into the container process root.
func (s *BaseServer) mergeStagingDir(containerID string, rootPath string) {
	sd, ok := s.Store.StagingDirs.Load(containerID)
	if !ok {
		return
	}
	stagingDir := sd.(string)
	defer func() {
		os.RemoveAll(stagingDir)
		s.Store.StagingDirs.Delete(containerID)
	}()
	// Walk staging dir and copy into process root
	_ = filepath.Walk(stagingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == stagingDir {
			return nil
		}
		rel, _ := filepath.Rel(stagingDir, path)
		dest := filepath.Join(rootPath, rel)
		if info.IsDir() {
			if err := os.MkdirAll(dest, info.Mode()); err != nil {
				s.Logger.Warn().Err(err).Str("dir", rel).Msg("staging merge: mkdir failed")
			}
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			s.Logger.Warn().Err(err).Str("file", rel).Msg("staging merge: mkdir parent failed")
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			s.Logger.Warn().Err(err).Str("file", rel).Msg("staging merge: open source failed")
			return nil
		}
		defer func() { _ = src.Close() }()
		dst, err := os.Create(dest)
		if err != nil {
			s.Logger.Warn().Err(err).Str("file", rel).Msg("staging merge: create dest failed")
			return nil
		}
		defer func() { _ = dst.Close() }()
		if _, err := io.Copy(dst, src); err != nil {
			s.Logger.Warn().Err(err).Str("file", rel).Msg("staging merge: copy failed")
		}
		if err := dst.Chmod(info.Mode()); err != nil {
			s.Logger.Warn().Err(err).Str("file", rel).Msg("staging merge: chmod failed")
		}
		return nil
	})
}

// createTar creates a tar archive from a path and writes it to w.
func createTar(w io.Writer, srcPath string, baseName string) error {
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()

	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		if err := tw.WriteHeader(&tar.Header{
			Name: baseName,
			Size: info.Size(),
			Mode: int64(info.Mode().Perm()),
		}); err != nil {
			return err
		}
		f, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		_ = f.Close()
		return copyErr
	}

	return filepath.Walk(srcPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcPath, path)
		name := filepath.Join(baseName, rel)

		if fi.IsDir() {
			return tw.WriteHeader(&tar.Header{
				Name:     name + "/",
				Typeflag: tar.TypeDir,
				Mode:     int64(fi.Mode().Perm()),
			})
		}

		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Size: fi.Size(),
			Mode: int64(fi.Mode().Perm()),
		}); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, f)
		_ = f.Close()
		return copyErr
	})
}

// resolveTmpfsMounts creates temporary directories for each tmpfs mount entry.
// Returns a map of containerPath → hostDir, or nil if there are no tmpfs mounts.
func resolveTmpfsMounts(tmpfs map[string]string) map[string]string {
	if len(tmpfs) == 0 {
		return nil
	}
	result := make(map[string]string)
	for containerPath := range tmpfs {
		dir, err := os.MkdirTemp("", "tmpfs-")
		if err != nil {
			continue
		}
		result[containerPath] = dir
	}
	return result
}

// buildContainerFromConfig creates a new Container from the given config and host config.
func (s *BaseServer) buildContainerFromConfig(id, name string, config api.ContainerConfig, hostConfig api.HostConfig, networkingConfig *api.NetworkingConfig) api.Container {
	path := ""
	var args []string
	if len(config.Entrypoint) > 0 {
		path = config.Entrypoint[0]
		args = append(config.Entrypoint[1:], config.Cmd...)
	} else if len(config.Cmd) > 0 {
		path = config.Cmd[0]
		args = config.Cmd[1:]
	}

	container := api.Container{
		ID:      id,
		Name:    name,
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Path:    path,
		Args:    args,
		State: api.ContainerState{
			Status:     "created",
			FinishedAt: "0001-01-01T00:00:00Z",
			StartedAt:  "0001-01-01T00:00:00Z",
		},
		Image:          config.Image,
		LogPath:        "/var/lib/sockerless/containers/" + id + "/" + id + "-json.log",
		ResolvConfPath: "/var/lib/sockerless/containers/" + id + "/resolv.conf",
		HostnamePath:   "/var/lib/sockerless/containers/" + id + "/hostname",
		HostsPath:      "/var/lib/sockerless/containers/" + id + "/hosts",
		Config:         config,
		HostConfig:     hostConfig,
		NetworkSettings: api.NetworkSettings{
			SandboxID:  id,
			SandboxKey: "/var/run/docker/netns/" + id[:12],
			Networks:   make(map[string]*api.EndpointSettings),
		},
		Mounts:   buildMounts(hostConfig),
		Platform: "linux",
		Driver:   s.Desc.Driver,
	}

	// Set up default network — resolve via store for correct IPAM
	netName := hostConfig.NetworkMode
	if netName == "default" {
		netName = "bridge"
	}
	endpoint := s.buildEndpointForNetwork(netName, id, name, nil)
	container.NetworkSettings.Networks[netName] = endpoint
	if netName == "bridge" {
		container.NetworkSettings.Bridge = "docker0"
	}

	// Process explicit NetworkingConfig (e.g. from service containers)
	if networkingConfig != nil {
		for netRef, reqEndpoint := range networkingConfig.EndpointsConfig {
			ep := s.buildEndpointForNetwork(netRef, id, name, reqEndpoint)
			// Find the display name for this network
			displayName := netRef
			if net, ok := s.Store.ResolveNetwork(netRef); ok {
				displayName = net.Name
			}
			container.NetworkSettings.Networks[displayName] = ep
		}
	}

	// Populate NetworkSettings.Ports from HostConfig.PortBindings
	if len(hostConfig.PortBindings) > 0 {
		container.NetworkSettings.Ports = make(map[string][]api.PortBinding)
		for port, bindings := range hostConfig.PortBindings {
			container.NetworkSettings.Ports[port] = bindings
		}
	}

	return container
}

// buildEndpointForNetwork creates an EndpointSettings for a network, resolving
// IPAM from the IPAllocator and adding the container to the Network.Containers map.
func (s *BaseServer) buildEndpointForNetwork(netRef, containerID, containerName string, reqEndpoint *api.EndpointSettings) *api.EndpointSettings {
	net, found := s.Store.ResolveNetwork(netRef)
	if !found {
		// Fallback for unknown networks
		return &api.EndpointSettings{
			NetworkID:   netRef,
			EndpointID:  GenerateID()[:16],
			Gateway:     "172.17.0.1",
			IPAddress:   fmt.Sprintf("172.17.0.%d", s.Store.Containers.Len()+2),
			IPPrefixLen: 16,
			MacAddress:  "02:42:ac:11:00:02",
		}
	}

	ip, prefixLen, gateway, mac := s.Store.IPAlloc.AllocateIP(net.ID)

	endpoint := &api.EndpointSettings{
		NetworkID:   net.ID,
		EndpointID:  GenerateID()[:16],
		Gateway:     gateway,
		IPAddress:   ip,
		IPPrefixLen: prefixLen,
		MacAddress:  mac,
	}

	// Copy fields from request endpoint config
	if reqEndpoint != nil {
		if reqEndpoint.IPAddress != "" {
			endpoint.IPAddress = reqEndpoint.IPAddress
		}
		if len(reqEndpoint.Aliases) > 0 {
			endpoint.Aliases = reqEndpoint.Aliases
		}
	}

	// Add container to network's Containers map
	s.Store.Networks.Update(net.ID, func(n *api.Network) {
		n.Containers[containerID] = api.EndpointResource{
			Name:        strings.TrimPrefix(containerName, "/"),
			EndpointID:  endpoint.EndpointID,
			MacAddress:  endpoint.MacAddress,
			IPv4Address: endpoint.IPAddress + fmt.Sprintf("/%d", endpoint.IPPrefixLen),
		}
	})

	return endpoint
}

// buildMounts constructs the Container.Mounts slice from HostConfig.Binds,
// HostConfig.Mounts, and HostConfig.Tmpfs so that docker inspect returns mount info.
func buildMounts(hostConfig api.HostConfig) []api.MountPoint {
	var mounts []api.MountPoint

	// Parse Binds: "source:destination[:mode]"
	for _, bind := range hostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		source, dest := parts[0], parts[1]
		rw := true
		mode := ""
		if len(parts) == 3 {
			mode = parts[2]
			if mode == "ro" {
				rw = false
			}
		}
		mountType := "bind"
		name := ""
		if !filepath.IsAbs(source) {
			mountType = "volume"
			name = source
		}
		mounts = append(mounts, api.MountPoint{
			Type:        mountType,
			Name:        name,
			Source:      source,
			Destination: dest,
			Mode:        mode,
			RW:          rw,
		})
	}

	// Parse HostConfig.Mounts
	for _, m := range hostConfig.Mounts {
		rw := !m.ReadOnly
		mounts = append(mounts, api.MountPoint{
			Type:        m.Type,
			Name:        m.Source,
			Source:      m.Source,
			Destination: m.Target,
			RW:          rw,
		})
	}

	// Parse Tmpfs
	for containerPath := range hostConfig.Tmpfs {
		mounts = append(mounts, api.MountPoint{
			Type:        "tmpfs",
			Destination: containerPath,
			RW:          true,
		})
	}

	if mounts == nil {
		mounts = []api.MountPoint{}
	}
	return mounts
}

// resolveBindMounts converts Docker bind specs (e.g. "volName:/container/path")
// and HostConfig.Mounts into a map of containerPath → hostPath for WASM volume symlinks.
func (s *BaseServer) resolveBindMounts(binds []string, mounts []api.Mount) map[string]string {
	if len(binds) == 0 && len(mounts) == 0 {
		return nil
	}
	result := make(map[string]string)
	for _, bind := range binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		source, target := parts[0], parts[1]
		// Check if source is a named volume
		if dir, ok := s.Store.VolumeDirs.Load(source); ok {
			result[target] = dir.(string)
		} else if filepath.IsAbs(source) {
			// Host path bind mount — pass through if the source directory exists
			if info, err := os.Stat(source); err == nil && info.IsDir() {
				result[target] = source
			}
		} else {
			// Named volume that doesn't exist yet — auto-create
			volDir, err := os.MkdirTemp("", "vol-"+source+"-")
			if err == nil {
				s.Store.VolumeDirs.Store(source, volDir)
				s.Store.Volumes.Put(source, api.Volume{
					Name:       source,
					Driver:     "local",
					Mountpoint: fmt.Sprintf("/var/lib/sockerless/volumes/%s/_data", source),
					CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
					Labels:     make(map[string]string),
					Scope:      "local",
					Options:    make(map[string]string),
				})
				result[target] = volDir
			}
		}
	}
	// Also resolve HostConfig.Mounts (used by Docker SDK clients like act)
	for _, m := range mounts {
		if m.Type == "volume" && m.Source != "" && m.Target != "" {
			if dir, ok := s.Store.VolumeDirs.Load(m.Source); ok {
				result[m.Target] = dir.(string)
			} else {
				// Auto-create the volume if it doesn't exist yet
				volDir, err := os.MkdirTemp("", "vol-"+m.Source+"-")
				if err == nil {
					s.Store.VolumeDirs.Store(m.Source, volDir)
					s.Store.Volumes.Put(m.Source, api.Volume{
						Name:       m.Source,
						Driver:     "local",
						Mountpoint: fmt.Sprintf("/var/lib/sockerless/volumes/%s/_data", m.Source),
						CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
						Labels:     make(map[string]string),
						Scope:      "local",
					})
					result[m.Target] = volDir
				}
			}
		}
	}
	return result
}
