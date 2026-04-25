package core

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sockerless/api"
)

// CommitSpec describes a docker-commit request after the backend has
// resolved the source container.
type CommitSpec struct {
	ContainerID string
	SourceImage api.Image // the image the container was started from
	// Optional overrides applied on top of SourceImage.Config.
	ConfigOverrides *api.ContainerConfig
	// Ref is the target `repo[:tag]` for the new image. Optional;
	// when empty, the image is stored only by ID.
	Ref     string
	Author  string
	Comment string
}

// CommitContainerRequestViaAgent resolves the source container +
// image via BaseServer and then calls CommitContainerViaAgent with
// the request's overrides. Produces the new image and returns the
// Docker-compatible commit response. Shared by every cloud backend
// whose ContainerCommit routes through the reverse-agent.
func CommitContainerRequestViaAgent(s *BaseServer, reg *ReverseAgentRegistry, req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}
	ctx := context.Background()
	c, ok := s.ResolveContainerAuto(ctx, req.Container)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	if _, hasAgent := reg.Resolve(c.ID); !hasAgent {
		return nil, &api.NotImplementedError{Message: "docker commit requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	srcImage, ok := s.Store.ResolveImage(c.Image)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: c.Image}
	}
	spec := CommitSpec{
		ContainerID:     c.ID,
		SourceImage:     srcImage,
		ConfigOverrides: req.Config,
		Author:          req.Author,
		Comment:         req.Comment,
	}
	if req.Repo != "" {
		ref := req.Repo
		if req.Tag != "" {
			ref = req.Repo + ":" + req.Tag
		}
		spec.Ref = ref
	}
	img, err := CommitContainerViaAgent(ctx, s.Store, reg, spec)
	if err != nil {
		if err == ErrNoReverseAgent {
			return nil, &api.NotImplementedError{Message: "docker commit requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
		}
		return nil, &api.ServerError{Message: fmt.Sprintf("agent-driven commit: %v", err)}
	}
	return &api.ContainerCommitResponse{ID: img.ID}, nil
}

// CommitContainerViaAgent builds a new image layer from the files the
// container has added or modified since boot. The reverse-agent runs
// `find / -xdev -newer /proc/1 -printf '%p\0'` (same reference point
// as `docker diff` in core/container_changes.go) to list changed
// paths, then `tar -cf - --null -T -` to package them. The resulting
// layer is stacked on top of the source image's layer list so the
// new image is a proper docker-commit-style diff image — not a full
// rootfs dump.
//
// Limitation: deletions are not captured. `find(1)` can't report
// files that no longer exist, and without host-side access to the
// base image's rootfs we can't compute whiteout entries. Same scope
// as `core.RunContainerChangesViaAgent` (Phase 98 / BUG-753).
//
// Cloud backends wiring this must gate on an explicit opt-in config
// flag (`SOCKERLESS_ENABLE_COMMIT`) and follow with
// ImageManager.Push to sync to their cloud registry.
func CommitContainerViaAgent(ctx context.Context, store *Store, reg *ReverseAgentRegistry, spec CommitSpec) (*api.Image, error) {
	if reg == nil {
		return nil, ErrNoReverseAgent
	}

	// 1. Ask the agent for the list of paths changed since boot.
	//    NUL separator so path names with spaces/newlines round-trip
	//    intact through the shell.
	findCmd := []string{"find", "/", "-xdev", "-newer", "/proc/1",
		"!", "-path", "/proc/*",
		"!", "-path", "/sys/*",
		"!", "-path", "/dev/*",
		"!", "-path", "/tmp/.sockerless-mainpid",
		"-printf", "%p\x00",
	}
	listOut, listErr, listExit, err := reg.RunAndCapture(spec.ContainerID, "commit-list-"+spec.ContainerID, findCmd, nil, "")
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	if listExit != 0 {
		return nil, fmt.Errorf("find failed (exit %d): %s", listExit, string(listErr))
	}
	paths := splitNulList(listOut)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no filesystem changes since container start — nothing to commit")
	}

	// 2. Tar up just those paths. `tar --null -T -` reads NUL-separated
	//    path list from stdin, avoiding argv-length limits.
	tarCmd := []string{"tar", "-cf", "-", "--null", "-T", "-"}
	rootfsTar, tarErr, tarExit, err := reg.RunAndCaptureWithStdin(spec.ContainerID, "commit-tar-"+spec.ContainerID, tarCmd, nil, "", listOut)
	if err != nil {
		return nil, fmt.Errorf("tar changes: %w", err)
	}
	if tarExit != 0 {
		return nil, fmt.Errorf("tar failed (exit %d): %s", tarExit, string(tarErr))
	}

	// diff_id is sha256 of the *uncompressed* layer tar. blobDigest
	// is sha256 of the gzipped blob — that's what goes on the wire.
	diffSum := sha256.Sum256(rootfsTar)
	diffID := "sha256:" + hexOf(diffSum[:])

	var gzBuf bytes.Buffer
	gzw := gzip.NewWriter(&gzBuf)
	if _, err := gzw.Write(rootfsTar); err != nil {
		return nil, fmt.Errorf("gzip rootfs: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	gzBlob := gzBuf.Bytes()

	// Merge source image's config with caller's overrides. Cmd /
	// Entrypoint / Env / WorkingDir are the fields docker commit
	// traditionally lets you override via --change.
	imgConfig := spec.SourceImage.Config
	if spec.ConfigOverrides != nil {
		mergeCommitConfig(&imgConfig, spec.ConfigOverrides)
	}

	nowRFC := time.Now().UTC().Format(time.RFC3339Nano)

	// Stack the new layer on top of the source image's layers so
	// registry clients see a proper parent chain. The diff layer
	// sits at the end; earlier layers come from the source image.
	layerList := make([]string, 0, len(spec.SourceImage.RootFS.Layers)+1)
	layerList = append(layerList, spec.SourceImage.RootFS.Layers...)
	layerList = append(layerList, diffID)

	// The OCI config blob's sha256 is the image ID. OCIPush later
	// rebuilds the same blob from the image fields when pushing, so
	// the ID stays stable across push/pull round-trips.
	configBlob := buildImageConfigBlobJSON(imgConfig, spec.SourceImage.Architecture, spec.SourceImage.Os, layerList, nowRFC, spec.Author, spec.Comment)
	configSum := sha256.Sum256(configBlob)
	imageID := "sha256:" + hexOf(configSum[:])

	// Stash the compressed new layer in the Store so ImagePush /
	// OCIPush can upload it via LayerContent lookup. The store key is
	// the compressed-blob digest (what registries serve by) — same
	// convention ImagePull uses. The parent's prior layers are
	// already cached in store.LayerContent from the initial pull.
	gzSum := sha256.Sum256(gzBlob)
	gzDigest := "sha256:" + hexOf(gzSum[:])
	store.LayerContent.Store(gzDigest, gzBlob)

	newImage := api.Image{
		ID:           imageID,
		Architecture: spec.SourceImage.Architecture,
		Os:           spec.SourceImage.Os,
		Created:      nowRFC,
		Config:       imgConfig,
		Author:       spec.Author,
		Comment:      spec.Comment,
		Parent:       spec.SourceImage.ID,
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: layerList,
		},
		Size:        spec.SourceImage.Size + int64(len(gzBlob)),
		VirtualSize: spec.SourceImage.VirtualSize + int64(len(rootfsTar)),
	}
	if spec.Ref != "" {
		newImage.RepoTags = []string{spec.Ref}
	}

	// Register under all aliases so `docker inspect <id>` /
	// `docker inspect <repo:tag>` both work.
	StoreImageWithAliases(store, spec.Ref, newImage)

	// Inherit the parent image's manifest layers and append the new
	// commit layer. ImagePush uses this to address blobs in the
	// destination registry by the same compressed digest the source
	// registry served them under.
	var parentManifestLayers []ManifestLayerEntry
	if v, ok := store.ImageManifestLayers.Load(spec.SourceImage.ID); ok {
		parentManifestLayers = v.([]ManifestLayerEntry)
	}
	combined := make([]ManifestLayerEntry, 0, len(parentManifestLayers)+1)
	combined = append(combined, parentManifestLayers...)
	combined = append(combined, ManifestLayerEntry{
		Digest:    gzDigest,
		Size:      int64(len(gzBlob)),
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
	})
	store.ImageManifestLayers.Store(imageID, combined)

	return &newImage, nil
}

// mergeCommitConfig applies non-empty fields from override onto base.
// Matches the semantics of `docker commit --change`: overrides replace
// whole slices/strings rather than merging element-wise.
func mergeCommitConfig(base, override *api.ContainerConfig) {
	if len(override.Cmd) > 0 {
		base.Cmd = override.Cmd
	}
	if len(override.Entrypoint) > 0 {
		base.Entrypoint = override.Entrypoint
	}
	if len(override.Env) > 0 {
		base.Env = override.Env
	}
	if override.WorkingDir != "" {
		base.WorkingDir = override.WorkingDir
	}
	if override.User != "" {
		base.User = override.User
	}
	if len(override.Labels) > 0 {
		if base.Labels == nil {
			base.Labels = map[string]string{}
		}
		for k, v := range override.Labels {
			base.Labels[k] = v
		}
	}
}

// splitNulList splits a NUL-terminated path list (what `find -printf
// '%p\0'` produces) into a slice of strings. Empty trailing entries
// are dropped.
func splitNulList(b []byte) []string {
	parts := bytes.Split(b, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
	}
	return out
}

// hexOf is sha256 hex without importing encoding/hex in this file's
// tight callsites.
func hexOf(b []byte) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, x := range b {
		out[i*2] = hexChars[x>>4]
		out[i*2+1] = hexChars[x&0x0f]
	}
	return string(out)
}

// buildImageConfigBlobJSON renders the OCI config JSON for the commit
// result. Mirrors the shape OCIPush produces at push time so the
// image ID matches whatever the registry later hands back.
func buildImageConfigBlobJSON(cfg api.ContainerConfig, arch, osName string, diffIDs []string, created, author, comment string) []byte {
	if arch == "" {
		arch = "amd64"
	}
	if osName == "" {
		osName = "linux"
	}
	cfgBlob := imageConfigFromAPI(cfg)
	var configField any
	if cfgBlob != nil {
		configField = cfgBlob
	} else {
		configField = map[string]any{}
	}
	payload := map[string]any{
		"architecture": arch,
		"os":           osName,
		"created":      created,
		"config":       configField,
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": diffIDs,
		},
	}
	if author != "" {
		payload["author"] = author
	}
	if comment != "" {
		payload["comment"] = comment
	}
	b, _ := json.Marshal(payload)
	return b
}
