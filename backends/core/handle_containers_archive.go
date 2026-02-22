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
	}
	w.WriteHeader(http.StatusOK)
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
	_, _ = s.Drivers.Filesystem.GetArchive(id, path, w)
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

		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			io.Copy(f, tr)
			_ = f.Close()
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
			_ = os.MkdirAll(dest, info.Mode())
			return nil
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0755)
		src, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = src.Close() }()
		dst, err := os.Create(dest)
		if err != nil {
			return nil
		}
		defer func() { _ = dst.Close() }()
		_, _ = io.Copy(dst, src)
		_ = dst.Chmod(info.Mode())
		return nil
	})
}

// createTar creates a tar archive from a path and writes it to w.
func createTar(w io.Writer, srcPath string, baseName string) {
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()

	info, err := os.Stat(srcPath)
	if err != nil {
		return
	}

	if !info.IsDir() {
		tw.WriteHeader(&tar.Header{
			Name: baseName,
			Size: info.Size(),
			Mode: int64(info.Mode().Perm()),
		})
		f, err := os.Open(srcPath)
		if err != nil {
			return
		}
		io.Copy(tw, f)
		_ = f.Close()
		return
	}

	_ = filepath.Walk(srcPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(srcPath, path)
		name := filepath.Join(baseName, rel)

		if fi.IsDir() {
			tw.WriteHeader(&tar.Header{
				Name:     name + "/",
				Typeflag: tar.TypeDir,
				Mode:     int64(fi.Mode().Perm()),
			})
			return nil
		}

		tw.WriteHeader(&tar.Header{
			Name: name,
			Size: fi.Size(),
			Mode: int64(fi.Mode().Perm()),
		})
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		io.Copy(tw, f)
		_ = f.Close()
		return nil
	})
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
		Image:  config.Image,
		Config: config,
		HostConfig: hostConfig,
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts:   make([]api.MountPoint, 0),
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
// IPAM from the store and adding the container to the Network.Containers map.
func (s *BaseServer) buildEndpointForNetwork(netRef, containerID, containerName string, reqEndpoint *api.EndpointSettings) *api.EndpointSettings {
	endpoint := &api.EndpointSettings{
		EndpointID:  GenerateID()[:16],
		IPPrefixLen: 16,
		MacAddress:  "02:42:ac:11:00:02",
	}

	net, found := s.Store.ResolveNetwork(netRef)
	if found {
		endpoint.NetworkID = net.ID
		// Use IPAM from the actual network
		if len(net.IPAM.Config) > 0 {
			endpoint.Gateway = net.IPAM.Config[0].Gateway
			endpoint.IPAddress = fmt.Sprintf("%s%d",
				net.IPAM.Config[0].Gateway[:strings.LastIndex(net.IPAM.Config[0].Gateway, ".")+1],
				len(net.Containers)+2)
		}
		// Add container to network's Containers map
		s.Store.Networks.Update(net.ID, func(n *api.Network) {
			n.Containers[containerID] = api.EndpointResource{
				Name:        strings.TrimPrefix(containerName, "/"),
				EndpointID:  endpoint.EndpointID,
				MacAddress:  endpoint.MacAddress,
				IPv4Address: endpoint.IPAddress + "/16",
			}
		})
	} else {
		// Fallback for unknown networks
		endpoint.NetworkID = netRef
		endpoint.Gateway = "172.17.0.1"
		endpoint.IPAddress = fmt.Sprintf("172.17.0.%d", s.Store.Containers.Len()+2)
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

	return endpoint
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
					result[m.Target] = volDir
				}
			}
		}
	}
	return result
}
