package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	dockerclient "github.com/docker/docker/client"
	sim "github.com/sockerless/simulator"
)

// Artifact Registry types

// Repository represents an Artifact Registry repository.
type Repository struct {
	Name                   string            `json:"name"`
	Format                 string            `json:"format"`
	Mode                   string            `json:"mode,omitempty"`
	Description            string            `json:"description,omitempty"`
	Labels                 map[string]string `json:"labels,omitempty"`
	KmsKeyName             string            `json:"kmsKeyName,omitempty"`
	CleanupPolicyDryRun    *bool             `json:"cleanupPolicyDryRun,omitempty"`
	RemoteRepositoryConfig map[string]any    `json:"remoteRepositoryConfig,omitempty"`
	// Nested writable configs the sim persists verbatim so the
	// terraform-provider-google read path round-trips without drift.
	CleanupPolicies json.RawMessage `json:"cleanupPolicies,omitempty"`
	DockerConfig    json.RawMessage `json:"dockerConfig,omitempty"`
	RegistryURI     string          `json:"registryUri,omitempty"` // external: canonical `<location>-docker.pkg.dev/<project>/<repo>` URI; sim serves OCI at the configured endpoint, not pkg.dev
	CreateTime      string          `json:"createTime"`
	UpdateTime      string          `json:"updateTime"`
}

// DockerImage represents a Docker image in Artifact Registry.
type DockerImage struct {
	Name       string   `json:"name"`
	URI        string   `json:"uri"` // external: canonical `<location>-docker.pkg.dev/<project>/<repo>@<digest>` URI; sim serves OCI at the configured endpoint
	Tags       []string `json:"tags,omitempty"`
	UploadTime string   `json:"uploadTime"`
	MediaType  string   `json:"mediaType,omitempty"`
	BuildTime  string   `json:"buildTime,omitempty"`
}

// Package-level store for dashboard access.
var arRepos sim.Store[Repository]

func registerArtifactRegistry(srv *sim.Server) {
	repos := sim.MakeStore[Repository](srv.DB(), "ar_repos")
	arRepos = repos
	dockerImages := sim.MakeStore[DockerImage](srv.DB(), "ar_docker_images")

	// OCI Distribution data plane (shared registry library). Cloud-specifics:
	// AR serves its control-plane API under /v2/projects/ (SkipPath), registers
	// a DockerImage row on manifest push (OnManifestPut), and hydrates docker-hub
	// remote repos from the local Docker daemon on a pull miss (HydrateManifest).
	dockerImagesForHooks := dockerImages
	reg := &sim.OCIRegistry{
		Manifests: sim.MakeStore[sim.OCIManifest](srv.DB(), "ar_manifests"),
		Blobs:     sim.MakeStore[sim.OCIBlob](srv.DB(), "ar_blobs"),
		Uploads:   sim.MakeStore[sim.OCIUpload](srv.DB(), "ar_uploads"),
		SkipPath:  func(path string) bool { return strings.HasPrefix(path, "/v2/projects/") },
		OnManifestPut: func(repo, ref, contentType string, data []byte) {
			registerDockerImageFromManifest(dockerImagesForHooks, repo, ref, contentType, data)
		},
		HydrateManifest: func(reg *sim.OCIRegistry, repo, ref string) bool {
			if err := hydrateOCIImageFromLocalDocker(reg, dockerImagesForHooks, repo, ref); err != nil {
				fmt.Fprintf(os.Stderr, "[sim-gcp-ar] local docker cache miss for %s:%s: %v\n", repo, ref, err)
				return false
			}
			return true
		},
	}

	// Create repository
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/repositories", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		// The terraform google provider sends repository_id (snake_case),
		// while the SDK sends repositoryId (camelCase). Accept both.
		repoID := r.URL.Query().Get("repositoryId")
		if repoID == "" {
			repoID = r.URL.Query().Get("repository_id")
		}
		if repoID == "" {
			sim.GCPError(w, http.StatusBadRequest, "repositoryId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var repo Repository
		if err := sim.ReadJSON(r, &repo); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)
		if _, exists := repos.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "repository %q already exists", name)
			return
		}

		now := nowTimestamp()
		repo.Name = name
		if repo.Format == "" {
			repo.Format = "DOCKER"
		}
		if repo.Mode == "" {
			repo.Mode = "STANDARD_REPOSITORY"
		}
		repo.RegistryURI = fmt.Sprintf("%s-docker.pkg.dev/%s/%s", location, project, repoID)
		repo.CreateTime = now
		repo.UpdateTime = now

		repos.Put(name, repo)

		lro := newLRO(project, location, repo, "type.googleapis.com/google.devtools.artifactregistry.v1.Repository")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Get repository (also handles :getIamPolicy/:setIamPolicy suffixes from terraform)
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/repositories/{repo}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		repoID := sim.PathParam(r, "repo")

		// Don't match if path continues with /dockerImages
		if strings.Contains(r.URL.Path, "/dockerImages") {
			return
		}

		// Handle IAM operations — terraform google provider uses GET for these
		if base, action, ok := strings.Cut(repoID, ":"); ok {
			resource := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, base)
			handleResourceIAM(w, r, gcpResourcePolicies, resource, action)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)
		repo, ok := repos.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "repository %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, repo)
	})

	// Artifact registry repository IAM (POST variant)
	srv.HandleFunc("POST /v1/projects/{project}/locations/{location}/repositories/{repoAction}", func(w http.ResponseWriter, r *http.Request) {
		repoAction := sim.PathParam(r, "repoAction")
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")

		repo, action, ok := strings.Cut(repoAction, ":")
		if !ok {
			http.NotFound(w, r)
			return
		}

		resource := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repo)
		handleResourceIAM(w, r, gcpResourcePolicies, resource, action)
	})

	// List repositories
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/repositories", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/repositories/", project, location)

		result := repos.Filter(func(repo Repository) bool {
			return strings.HasPrefix(repo.Name, prefix)
		})
		if result == nil {
			result = []Repository{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"repositories": result,
		})
	})

	// Delete repository
	srv.HandleFunc("DELETE /v1/projects/{project}/locations/{location}/repositories/{repo}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		repoID := sim.PathParam(r, "repo")
		name := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)

		repo, ok := repos.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "repository %q not found", name)
			return
		}
		repos.Delete(name)

		// Clean up docker images for this repo
		images := dockerImages.Filter(func(img DockerImage) bool {
			return strings.HasPrefix(img.Name, name+"/")
		})
		for _, img := range images {
			dockerImages.Delete(img.Name)
		}

		lro := newLRO(project, location, repo, "type.googleapis.com/google.devtools.artifactregistry.v1.Repository")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// List docker images
	srv.HandleFunc("GET /v1/projects/{project}/locations/{location}/repositories/{repo}/dockerImages", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		repoID := sim.PathParam(r, "repo")
		repoName := fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoID)

		if _, ok := repos.Get(repoName); !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "repository %q not found", repoName)
			return
		}

		prefix := repoName + "/dockerImages/"
		result := dockerImages.Filter(func(img DockerImage) bool {
			return strings.HasPrefix(img.Name, prefix)
		})
		if result == nil {
			result = []DockerImage{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"dockerImages": result,
		})
	})

	// OCI Distribution data plane — mounted from the shared registry library.
	reg.Register(srv)
}

// hydrateOCIImageFromLocalDocker is the AR pull-through cache: on a manifest
// miss for a docker-hub remote repo it saves the image from the local Docker
// daemon and populates the shared registry's blobs + manifest.
func hydrateOCIImageFromLocalDocker(reg *sim.OCIRegistry, dockerImages sim.Store[DockerImage], imageName, reference string) error {
	if !strings.Contains(imageName, "/docker-hub/") {
		return fmt.Errorf("repository is not a docker-hub remote repository")
	}

	localRef := imageName + ":" + reference
	if idx := strings.Index(imageName, "/docker-hub/"); idx >= 0 {
		localRef = strings.TrimPrefix(imageName[idx+len("/docker-hub/"):], "library/") + ":" + reference
	}
	ctx := context.Background()
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	rc, err := cli.ImageSave(ctx, []string{localRef})
	if err != nil {
		return fmt.Errorf("docker image save %q: %w", localRef, err)
	}
	defer rc.Close()

	manifestData, files, err := readDockerImageSave(rc)
	if err != nil {
		return err
	}
	var saved []struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags"`
		Layers   []string `json:"Layers"`
	}
	if err := json.Unmarshal(manifestData, &saved); err != nil {
		return fmt.Errorf("decode docker save manifest: %w", err)
	}
	if len(saved) == 0 {
		return fmt.Errorf("docker save manifest is empty")
	}
	image := saved[0]
	configData, ok := files[image.Config]
	if !ok {
		return fmt.Errorf("docker save config %q missing", image.Config)
	}

	configDigest := digestBytes(configData)
	reg.PutBlob(imageName, configDigest, "application/vnd.docker.container.image.v1+json", configData)

	type descriptor struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	}
	layerDescriptors := make([]descriptor, 0, len(image.Layers))
	for _, layerPath := range image.Layers {
		layerData, ok := files[layerPath]
		if !ok {
			return fmt.Errorf("docker save layer %q missing", layerPath)
		}
		layerDigest := digestBytes(layerData)
		reg.PutBlob(imageName, layerDigest, "application/vnd.oci.image.layer.v1.tar", layerData)
		layerDescriptors = append(layerDescriptors, descriptor{
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Size:      int64(len(layerData)),
			Digest:    layerDigest,
		})
	}

	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": descriptor{
			MediaType: "application/vnd.docker.container.image.v1+json",
			Size:      int64(len(configData)),
			Digest:    configDigest,
		},
		"layers": layerDescriptors,
	}
	ociManifest, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode OCI manifest: %w", err)
	}
	reg.PutManifest(imageName, reference, "application/vnd.oci.image.manifest.v1+json", ociManifest)
	registerDockerImageFromManifest(dockerImages, imageName, reference, "application/vnd.oci.image.manifest.v1+json", ociManifest)
	return nil
}

func readDockerImageSave(r io.Reader) ([]byte, map[string][]byte, error) {
	tr := tar.NewReader(r)
	files := make(map[string][]byte)
	var manifest []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read docker save tar: %w", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, tr); err != nil {
			return nil, nil, fmt.Errorf("read docker save entry %q: %w", hdr.Name, err)
		}
		data := buf.Bytes()
		if hdr.Name == "manifest.json" {
			manifest = data
		}
		files[hdr.Name] = data
	}
	if len(manifest) == 0 {
		return nil, nil, fmt.Errorf("docker save manifest.json missing")
	}
	return manifest, files, nil
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum)
}

func registerDockerImageFromManifest(dockerImages sim.Store[DockerImage], imageName, reference, contentType string, data []byte) {
	var manifest struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		fmt.Fprintf(os.Stderr, "[sim-gcp-ar] mediaType extraction from manifest failed (image=%s ref=%s): %v\n",
			imageName, reference, err)
		manifest.MediaType = contentType
	}

	project, location, repoID, imagePath, ok := artifactRegistryImageParts(imageName)
	if !ok {
		fmt.Fprintf(os.Stderr, "[sim-gcp-ar] docker image registration skipped for malformed Artifact Registry image name %q\n", imageName)
		return
	}
	manifestDigest := digestBytes(data)
	now := nowTimestamp()
	imgName := fmt.Sprintf("projects/%s/locations/%s/repositories/%s/dockerImages/%s@%s", project, location, repoID, imagePath, manifestDigest)
	tags := []string{}
	if !strings.HasPrefix(reference, "sha256:") {
		tags = append(tags, reference)
	}

	img := DockerImage{
		Name:       imgName,
		URI:        fmt.Sprintf("%s-docker.pkg.dev/%s/%s/%s@%s", location, project, repoID, imagePath, manifestDigest),
		Tags:       tags,
		UploadTime: now,
		MediaType:  contentType,
	}
	dockerImages.Put(imgName, img)
}

func artifactRegistryImageParts(imageName string) (project, location, repoID, imagePath string, ok bool) {
	location = "us-central1"
	parts := strings.SplitN(imageName, "/", 3)
	if len(parts) < 3 {
		return "", "", "", "", false
	}
	project, repoID, imagePath = parts[0], parts[1], parts[2]
	if arRepos != nil {
		prefix := fmt.Sprintf("projects/%s/locations/", project)
		suffix := fmt.Sprintf("/repositories/%s", repoID)
		matches := arRepos.Filter(func(repo Repository) bool {
			return strings.HasPrefix(repo.Name, prefix) && strings.HasSuffix(repo.Name, suffix)
		})
		if len(matches) > 0 {
			segments := strings.Split(matches[0].Name, "/")
			if len(segments) >= 4 {
				location = segments[3]
			}
		}
	}
	return project, location, repoID, imagePath, true
}
