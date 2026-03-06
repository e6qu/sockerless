package core

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// ContainerResize resizes the TTY of a container.
func (s *BaseServer) ContainerResize(id string, h, w int) error {
	resolvedID, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	if h > 0 || w > 0 {
		s.Store.Containers.Update(resolvedID, func(c *api.Container) {
			c.HostConfig.ConsoleSize = [2]uint{uint(h), uint(w)}
		})
	}
	return nil
}

// ExecResize resizes the TTY of an exec instance.
func (s *BaseServer) ExecResize(id string, h, w int) error {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return &api.NotFoundError{Resource: "exec instance", ID: id}
	}
	if h > 0 || w > 0 {
		s.Store.Containers.Update(exec.ContainerID, func(c *api.Container) {
			c.HostConfig.ConsoleSize = [2]uint{uint(h), uint(w)}
		})
	}
	return nil
}

// ContainerPutArchive extracts a tar archive to the container filesystem.
func (s *BaseServer) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	resolvedID, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}
	if path == "" {
		path = "/"
	}
	if err := s.Drivers.Filesystem.PutArchive(resolvedID, path, body); err != nil {
		s.Logger.Error().Err(err).Str("container", resolvedID).Msg("failed to extract archive")
		return &api.ServerError{Message: "failed to extract archive: " + err.Error()}
	}
	return nil
}

// ContainerStatPath returns stat info for a path in a container.
func (s *BaseServer) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	resolvedID, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if path == "" {
		path = "/"
	}
	info, err := s.Drivers.Filesystem.StatPath(resolvedID, path)
	if err != nil {
		return nil, &api.NotFoundError{Resource: "path", ID: path}
	}
	return &api.ContainerPathStat{
		Name:  info.Name(),
		Size:  info.Size(),
		Mode:  info.Mode().Perm(),
		Mtime: info.ModTime().UTC(),
	}, nil
}

// ContainerGetArchive returns a tar archive of the requested container path.
func (s *BaseServer) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	resolvedID, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if path == "" {
		path = "/"
	}
	info, err := s.Drivers.Filesystem.StatPath(resolvedID, path)
	if err != nil {
		return nil, &api.NotFoundError{Resource: "path", ID: path}
	}

	pr, pw := io.Pipe()
	go func() {
		_, writeErr := s.Drivers.Filesystem.GetArchive(resolvedID, path, pw)
		pw.CloseWithError(writeErr)
	}()

	return &api.ContainerArchiveResponse{
		Stat: api.ContainerPathStat{
			Name:  info.Name(),
			Size:  info.Size(),
			Mode:  info.Mode().Perm(),
			Mtime: info.ModTime().UTC(),
		},
		Reader: pr,
	}, nil
}

// ContainerUpdate updates resource limits on a container.
func (s *BaseServer) ContainerUpdate(id string, req *api.ContainerUpdateRequest) (*api.ContainerUpdateResponse, error) {
	resolvedID, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	s.Store.Containers.Update(resolvedID, func(c *api.Container) {
		if req.RestartPolicy.Name != "" {
			c.HostConfig.RestartPolicy = req.RestartPolicy
		}
		if req.Memory != 0 {
			c.HostConfig.Memory = req.Memory
		}
		if req.MemorySwap != 0 {
			c.HostConfig.MemorySwap = req.MemorySwap
		}
		if req.MemoryReservation != 0 {
			c.HostConfig.MemoryReservation = req.MemoryReservation
		}
		if req.CPUShares != 0 {
			c.HostConfig.CPUShares = req.CPUShares
		}
		if req.CPUQuota != 0 {
			c.HostConfig.CPUQuota = req.CPUQuota
		}
		if req.CPUPeriod != 0 {
			c.HostConfig.CPUPeriod = req.CPUPeriod
		}
		if req.CpusetCpus != "" {
			c.HostConfig.CpusetCpus = req.CpusetCpus
		}
		if req.CpusetMems != "" {
			c.HostConfig.CpusetMems = req.CpusetMems
		}
		if req.BlkioWeight != 0 {
			c.HostConfig.BlkioWeight = req.BlkioWeight
		}
		if req.PidsLimit != nil {
			c.HostConfig.PidsLimit = req.PidsLimit
		}
		if req.OomKillDisable != nil {
			c.HostConfig.OomKillDisable = req.OomKillDisable
		}
	})

	c, _ := s.Store.Containers.Get(resolvedID)
	s.emitEvent("container", "update", resolvedID, map[string]string{
		"name": strings.TrimPrefix(c.Name, "/"),
	})

	return &api.ContainerUpdateResponse{Warnings: []string{}}, nil
}

// ContainerChanges returns filesystem changes in a container.
func (s *BaseServer) ContainerChanges(id string) ([]api.ContainerChangeItem, error) {
	_, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return []api.ContainerChangeItem{}, nil
}

// ContainerExport exports a container's filesystem as a tar stream.
func (s *BaseServer) ContainerExport(id string) (io.ReadCloser, error) {
	resolvedID, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	rootPath, err := s.Drivers.Filesystem.RootPath(resolvedID)
	if err != nil || rootPath == "" {
		// Synthetic container — return empty tar
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		_ = tw.Close()
		return io.NopCloser(&buf), nil
	}

	pr, pw := io.Pipe()
	go func() {
		err := createTar(pw, rootPath, ".")
		pw.CloseWithError(err)
	}()
	return pr, nil
}

// ImageBuild builds an image from a Dockerfile and build context.
func (s *BaseServer) ImageBuild(opts api.ImageBuildOptions, context io.Reader) (io.ReadCloser, error) {
	tag := ""
	if len(opts.Tags) > 0 {
		tag = opts.Tags[0]
	}
	dockerfileName := opts.Dockerfile
	if dockerfileName == "" {
		dockerfileName = "Dockerfile"
	}

	// Convert BuildArgs from map[string]*string to map[string]string
	var buildArgs map[string]string
	if len(opts.BuildArgs) > 0 {
		buildArgs = make(map[string]string)
		for k, v := range opts.BuildArgs {
			if v != nil {
				buildArgs[k] = *v
			}
		}
	}

	// Extract tar body to temp dir
	contextDir, err := os.MkdirTemp("", "docker-build-")
	if err != nil {
		return nil, &api.ServerError{Message: "failed to create temp dir: " + err.Error()}
	}

	if err := extractTar(context, contextDir); err != nil {
		os.RemoveAll(contextDir)
		return nil, &api.ServerError{Message: "failed to extract build context: " + err.Error()}
	}

	// Read the Dockerfile
	dfContent, err := os.ReadFile(contextDir + "/" + dockerfileName)
	if err != nil {
		os.RemoveAll(contextDir)
		return nil, &api.ServerError{Message: "failed to read Dockerfile: " + err.Error()}
	}

	parsed, err := parseDockerfile(string(dfContent), buildArgs)
	if err != nil {
		os.RemoveAll(contextDir)
		return nil, &api.ServerError{Message: "failed to parse Dockerfile: " + err.Error()}
	}

	// Resolve base image config
	baseConfig := api.ContainerConfig{
		Env: []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
	}
	if baseImg, ok := s.Store.ResolveImage(parsed.from); ok {
		baseConfig = baseImg.Config
	}

	// Merge base image config + parsed Dockerfile overrides
	finalConfig := baseConfig
	if len(parsed.config.Env) > 0 {
		finalConfig.Env = append(finalConfig.Env, parsed.config.Env...)
	}
	if len(parsed.config.Cmd) > 0 {
		finalConfig.Cmd = parsed.config.Cmd
	}
	if len(parsed.config.Entrypoint) > 0 {
		finalConfig.Entrypoint = parsed.config.Entrypoint
	}
	if parsed.config.WorkingDir != "" {
		finalConfig.WorkingDir = parsed.config.WorkingDir
	}
	if parsed.config.User != "" {
		finalConfig.User = parsed.config.User
	}
	if finalConfig.Labels == nil {
		finalConfig.Labels = make(map[string]string)
	}
	for k, v := range parsed.config.Labels {
		finalConfig.Labels[k] = v
	}
	if finalConfig.ExposedPorts == nil {
		finalConfig.ExposedPorts = make(map[string]struct{})
	}
	for k, v := range parsed.config.ExposedPorts {
		finalConfig.ExposedPorts[k] = v
	}
	if parsed.config.Healthcheck != nil {
		finalConfig.Healthcheck = parsed.config.Healthcheck
	}
	if len(parsed.config.Shell) > 0 {
		finalConfig.Shell = parsed.config.Shell
	}
	if parsed.config.StopSignal != "" {
		finalConfig.StopSignal = parsed.config.StopSignal
	}
	if len(parsed.config.Volumes) > 0 {
		if finalConfig.Volumes == nil {
			finalConfig.Volumes = make(map[string]struct{})
		}
		for k, v := range parsed.config.Volumes {
			finalConfig.Volumes[k] = v
		}
	}

	// Apply labels from build options
	if len(opts.Labels) > 0 {
		for k, v := range opts.Labels {
			finalConfig.Labels[k] = v
		}
	}

	// Generate image ID
	hash := sha256.Sum256([]byte(tag + time.Now().String()))
	imageID := fmt.Sprintf("sha256:%x", hash)
	shortID := fmt.Sprintf("%x", hash)[:12]

	ref := tag
	if ref == "" {
		ref = imageID
	}
	if !strings.Contains(ref, ":") {
		ref += ":latest"
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	img := api.Image{
		ID:           imageID,
		RepoTags:     []string{ref},
		Created:      nowStr,
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Config:       finalConfig,
		RootFS:   api.RootFS{Type: "layers", Layers: []string{"sha256:" + GenerateID()}},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: nowStr},
	}
	StoreImageWithAliases(s.Store, ref, img)

	// Process COPY instructions
	if len(parsed.copies) > 0 {
		stagingDir, err := prepareBuildContext(contextDir, parsed.copies)
		if err == nil && stagingDir != "" {
			s.Store.BuildContexts.Store(imageID, stagingDir)
		}
	}

	// Clean up context dir
	os.RemoveAll(contextDir)

	// Build JSON progress output
	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		dfLines := strings.Split(strings.ReplaceAll(string(dfContent), "\\\n", " "), "\n")
		step := 0
		totalSteps := 0
		for _, line := range dfLines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				totalSteps++
			}
		}
		for _, line := range dfLines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			step++
			_ = enc.Encode(map[string]any{"stream": fmt.Sprintf("Step %d/%d : %s\n", step, totalSteps, line)})
			_ = enc.Encode(map[string]any{"stream": fmt.Sprintf(" ---> %s\n", shortID)})
		}
		_ = enc.Encode(map[string]any{"aux": map[string]string{"ID": imageID}})
		_ = enc.Encode(map[string]any{"stream": fmt.Sprintf("Successfully built %s\n", shortID)})
		if tag != "" {
			_ = enc.Encode(map[string]any{"stream": fmt.Sprintf("Successfully tagged %s\n", ref)})
		}
		_ = pw.Close()
	}()

	return pr, nil
}

// ImagePush simulates pushing an image to a registry.
func (s *BaseServer) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}

	if tag == "" {
		tag = "latest"
	}

	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		_ = enc.Encode(map[string]string{"status": "The push refers to repository [" + name + "]"})
		_ = enc.Encode(map[string]string{"status": "Preparing", "id": tag})
		_ = enc.Encode(map[string]string{"status": "Pushed", "id": tag})
		digest := strings.TrimPrefix(img.ID, "sha256:")
		_ = enc.Encode(map[string]string{"status": tag + ": digest: sha256:" + digest})
		_ = pw.Close()
	}()

	return pr, nil
}

// ImageSave exports images as a tar archive.
func (s *BaseServer) ImageSave(names []string) (io.ReadCloser, error) {
	// Validate all images exist before starting
	for _, name := range names {
		if _, ok := s.Store.ResolveImage(name); !ok {
			return nil, &api.NotFoundError{Resource: "image", ID: name}
		}
	}

	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		var manifests []map[string]any
		for _, name := range names {
			img, ok := s.Store.ResolveImage(name)
			if !ok {
				continue
			}
			layers := img.RootFS.Layers
			if layers == nil {
				layers = []string{}
			}
			manifests = append(manifests, map[string]any{
				"Config":   img.ID + ".json",
				"RepoTags": img.RepoTags,
				"Layers":   layers,
			})

			configData, _ := json.Marshal(map[string]any{
				"architecture": img.Architecture,
				"os":           img.Os,
				"created":      img.Created,
				"config":       img.Config,
				"rootfs":       img.RootFS,
			})
			_ = tw.WriteHeader(&tar.Header{Name: img.ID + ".json", Size: int64(len(configData))})
			_, _ = tw.Write(configData)
		}
		if manifests == nil {
			manifests = []map[string]any{}
		}
		data, _ := json.Marshal(manifests)
		_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(data))})
		_, _ = tw.Write(data)
		_ = tw.Close()
		_ = pw.Close()
	}()

	return pr, nil
}

// ImageSearch searches local images by term.
func (s *BaseServer) ImageSearch(term string, limit int, filters map[string][]string) ([]*api.ImageSearchResult, error) {
	var results []*api.ImageSearchResult
	seen := make(map[string]bool)
	for _, img := range s.Store.Images.List() {
		if seen[img.ID] {
			continue
		}
		seen[img.ID] = true
		for _, tag := range img.RepoTags {
			if strings.Contains(tag, term) {
				results = append(results, &api.ImageSearchResult{
					Name: tag,
				})
				break
			}
		}
	}
	if results == nil {
		results = []*api.ImageSearchResult{}
	}
	sort.Slice(results, func(i, j int) bool {
		iExact := results[i].Name == term
		jExact := results[j].Name == term
		if iExact != jExact {
			return iExact
		}
		return results[i].Name < results[j].Name
	})
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	return results, nil
}

// ContainerCommit creates a new image from a container's current state.
func (s *BaseServer) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}

	c, ok := s.Store.ResolveContainer(req.Container)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}

	tag := req.Tag
	if tag == "" {
		tag = "latest"
	}

	// Start with the container's config
	imgConfig := c.Config

	// Apply config overrides
	if req.Config != nil {
		if len(req.Config.Cmd) > 0 {
			imgConfig.Cmd = req.Config.Cmd
		}
		if len(req.Config.Entrypoint) > 0 {
			imgConfig.Entrypoint = req.Config.Entrypoint
		}
		if len(req.Config.Env) > 0 {
			imgConfig.Env = req.Config.Env
		}
		if req.Config.WorkingDir != "" {
			imgConfig.WorkingDir = req.Config.WorkingDir
		}
	}

	// Apply changes (Dockerfile instructions)
	for _, change := range req.Changes {
		change = strings.TrimSpace(change)
		if change == "" {
			continue
		}
		parts := strings.SplitN(change, " ", 2)
		if len(parts) < 2 {
			continue
		}
		instruction, value := strings.ToUpper(parts[0]), parts[1]
		switch instruction {
		case "CMD":
			imgConfig.Cmd = parseJSONOrShell(value)
		case "ENTRYPOINT":
			imgConfig.Entrypoint = parseJSONOrShell(value)
		case "ENV":
			k, v, _ := strings.Cut(value, "=")
			imgConfig.Env = append(imgConfig.Env, k+"="+v)
		case "WORKDIR":
			imgConfig.WorkingDir = value
		case "USER":
			imgConfig.User = value
		case "LABEL":
			k, v, _ := strings.Cut(value, "=")
			if imgConfig.Labels == nil {
				imgConfig.Labels = map[string]string{}
			}
			imgConfig.Labels[k] = strings.Trim(v, "\"")
		case "EXPOSE":
			if imgConfig.ExposedPorts == nil {
				imgConfig.ExposedPorts = map[string]struct{}{}
			}
			imgConfig.ExposedPorts[value+"/tcp"] = struct{}{}
		}
	}

	// Generate image ID
	hash := sha256.Sum256([]byte(c.ID + time.Now().UTC().String()))
	imageID := fmt.Sprintf("sha256:%x", hash)

	ref := req.Repo + ":" + tag
	var repoTags []string
	if req.Repo != "" {
		repoTags = []string{ref}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	img := api.Image{
		ID:           imageID,
		RepoTags:     repoTags,
		Created:      nowStr,
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Author:       req.Author,
		Comment:      req.Comment,
		Config:       imgConfig,
		RootFS:   api.RootFS{Type: "layers", Layers: []string{"sha256:" + GenerateID()}},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: nowStr},
	}

	if req.Repo != "" {
		StoreImageWithAliases(s.Store, ref, img)
	} else {
		s.Store.Images.Put(imageID, img)
	}

	s.emitEvent("container", "commit", c.ID, map[string]string{
		"comment": req.Comment, "imageID": imageID, "imageName": req.Repo + ":" + tag,
	})

	return &api.ContainerCommitResponse{ID: imageID}, nil
}
