package azf

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func (s *Server) handleImagePull(w http.ResponseWriter, r *http.Request) {
	var req api.ImagePullRequest
	if err := core.ReadJSON(r, &req); err != nil {
		core.WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	ref := req.Reference
	if ref == "" {
		core.WriteError(w, &api.InvalidParameterError{Message: "image reference is required"})
		return
	}

	// Add :latest if no tag or digest
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref += ":latest"
	}

	// Generate image ID
	hash := sha256.Sum256([]byte(ref))
	imageID := fmt.Sprintf("sha256:%x", hash)

	image := api.Image{
		ID:           imageID,
		RepoTags:     []string{ref},
		RepoDigests:  []string{},
		Created:      time.Now().UTC().Format(time.RFC3339Nano),
		Size:         0,
		VirtualSize:  0,
		Architecture: "amd64",
		Os:           "linux",
		RootFS:       api.RootFS{Type: "layers"},
		Config: api.ContainerConfig{
			Image: ref,
		},
	}

	core.StoreImageWithAliases(s.Store, ref, image)

	// Stream progress
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	progress := []map[string]string{
		{"status": "Pulling from " + ref},
		{"status": "Digest: " + imageID[:19]},
		{"status": "Status: Downloaded newer image for " + ref},
	}
	for _, p := range progress {
		json.NewEncoder(w).Encode(p)
		if canFlush {
			flusher.Flush()
		}
	}
}

func (s *Server) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	core.WriteError(w, &api.NotImplementedError{Message: "image load is not supported by Azure Functions backend"})
}
