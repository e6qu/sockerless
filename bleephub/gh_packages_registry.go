package bleephub

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const containerRegistryAPIVersion = "registry/2.0"

type containerRegistryUpload struct {
	Name string
	Data []byte
}

type containerRegistryDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type containerRegistryManifest struct {
	SchemaVersion int                           `json:"schemaVersion"`
	MediaType     string                        `json:"mediaType"`
	Config        containerRegistryDescriptor   `json:"config"`
	Layers        []containerRegistryDescriptor `json:"layers"`
}

func (s *Server) handleContainerRegistry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-Api-Version", containerRegistryAPIVersion)
	rest := strings.Trim(r.PathValue("rest"), "/")
	if rest == "" {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		s.writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository name not known")
		return
	}
	segments := strings.Split(rest, "/")
	switch {
	case r.Method == http.MethodPost && len(segments) >= 3 && segments[len(segments)-2] == "blobs" && segments[len(segments)-1] == "uploads":
		s.handleContainerRegistryStartBlobUpload(w, r, strings.Join(segments[:len(segments)-2], "/"))
	case (r.Method == http.MethodPatch || r.Method == http.MethodPut) && len(segments) >= 4 && segments[len(segments)-3] == "blobs" && segments[len(segments)-2] == "uploads":
		name := strings.Join(segments[:len(segments)-3], "/")
		uploadID := segments[len(segments)-1]
		if r.Method == http.MethodPatch {
			s.handleContainerRegistryPatchBlobUpload(w, r, name, uploadID)
		} else {
			s.handleContainerRegistryPutBlobUpload(w, r, name, uploadID)
		}
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && len(segments) >= 3 && segments[len(segments)-2] == "blobs":
		s.handleContainerRegistryGetBlob(w, r, strings.Join(segments[:len(segments)-2], "/"), segments[len(segments)-1])
	case r.Method == http.MethodPut && len(segments) >= 3 && segments[len(segments)-2] == "manifests":
		s.handleContainerRegistryPutManifest(w, r, strings.Join(segments[:len(segments)-2], "/"), segments[len(segments)-1])
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && len(segments) >= 3 && segments[len(segments)-2] == "manifests":
		s.handleContainerRegistryGetManifest(w, r, strings.Join(segments[:len(segments)-2], "/"), segments[len(segments)-1])
	default:
		s.writeRegistryError(w, http.StatusNotFound, "UNSUPPORTED", "registry operation is not supported")
	}
}

func (s *Server) handleContainerRegistryStartBlobUpload(w http.ResponseWriter, r *http.Request, name string) {
	if s.requireRegistryUser(w, r) == nil {
		return
	}
	if _, _, ok := s.resolveContainerRegistryOwner(w, name); !ok {
		return
	}
	uploadID := uuid.NewString()
	s.registryUploadsMu.Lock()
	s.registryUploads[uploadID] = &containerRegistryUpload{Name: name}
	s.registryUploadsMu.Unlock()
	w.Header().Set("Docker-Upload-UUID", uploadID)
	w.Header().Set("Location", s.containerRegistryUploadLocation(name, uploadID))
	w.Header().Set("Range", "0-0")
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleContainerRegistryPatchBlobUpload(w http.ResponseWriter, r *http.Request, name, uploadID string) {
	if s.requireRegistryUser(w, r) == nil {
		return
	}
	chunk, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", "read upload body: "+err.Error())
		return
	}
	s.registryUploadsMu.Lock()
	upload := s.registryUploads[uploadID]
	if upload != nil && upload.Name == name {
		upload.Data = append(upload.Data, chunk...)
	}
	size := 0
	if upload != nil {
		size = len(upload.Data)
	}
	s.registryUploadsMu.Unlock()
	if upload == nil || upload.Name != name {
		s.writeRegistryError(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "blob upload unknown")
		return
	}
	w.Header().Set("Docker-Upload-UUID", uploadID)
	w.Header().Set("Location", s.containerRegistryUploadLocation(name, uploadID))
	if size > 0 {
		w.Header().Set("Range", fmt.Sprintf("0-%d", size-1))
	} else {
		w.Header().Set("Range", "0-0")
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleContainerRegistryPutBlobUpload(w http.ResponseWriter, r *http.Request, name, uploadID string) {
	if s.requireRegistryUser(w, r) == nil {
		return
	}
	digest := r.URL.Query().Get("digest")
	if digest == "" {
		s.writeRegistryError(w, http.StatusBadRequest, "DIGEST_INVALID", "digest query parameter is required")
		return
	}
	tail, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UPLOAD_INVALID", "read upload body: "+err.Error())
		return
	}
	s.registryUploadsMu.Lock()
	upload := s.registryUploads[uploadID]
	if upload != nil && upload.Name == name {
		upload.Data = append(upload.Data, tail...)
		delete(s.registryUploads, uploadID)
	}
	s.registryUploadsMu.Unlock()
	if upload == nil || upload.Name != name {
		s.writeRegistryError(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "blob upload unknown")
		return
	}
	if got := digestSHA256(upload.Data); got != digest {
		s.writeRegistryError(w, http.StatusBadRequest, "DIGEST_INVALID", "digest mismatch")
		return
	}
	if err := s.writeRegistryBlob(digest, upload.Data); err != nil {
		s.writeRegistryError(w, http.StatusInternalServerError, "BLOB_UPLOAD_INVALID", err.Error())
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Location", "/v2/"+name+"/blobs/"+digest)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleContainerRegistryGetBlob(w http.ResponseWriter, r *http.Request, _ string, digest string) {
	if s.requireRegistryUser(w, r) == nil {
		return
	}
	data, err := s.readRegistryBlob(digest)
	if err != nil {
		s.writeRegistryError(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob unknown to registry")
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleContainerRegistryPutManifest(w http.ResponseWriter, r *http.Request, name, reference string) {
	user := s.requireRegistryUser(w, r)
	if user == nil {
		return
	}
	ownerType, ownerKey, ok := s.resolveContainerRegistryOwner(w, name)
	if !ok {
		return
	}
	if !s.canPublishContainerRegistryPackage(user, ownerType, ownerKey) {
		s.writeRegistryError(w, http.StatusForbidden, "DENIED", "requested access to the resource is denied")
		return
	}
	manifestBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "read manifest body: "+err.Error())
		return
	}
	manifestDigest := digestSHA256(manifestBytes)
	var manifest containerRegistryManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "manifest is not valid JSON")
		return
	}
	if manifest.SchemaVersion != 2 {
		s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "schemaVersion must be 2")
		return
	}
	pkgName := containerRegistryPackageName(name)
	p, _ := s.store.CreatePackage(ownerType, ownerKey, "container", pkgName, "public")
	files := []PackageFileInput{{
		Name:          "manifest.json",
		ContentType:   firstNonEmpty(manifest.MediaType, r.Header.Get("Content-Type"), "application/vnd.oci.image.manifest.v1+json"),
		ContentBase64: base64.StdEncoding.EncodeToString(manifestBytes),
	}}
	descriptors := append([]containerRegistryDescriptor{}, manifest.Config)
	descriptors = append(descriptors, manifest.Layers...)
	for _, desc := range descriptors {
		if desc.Digest == "" {
			continue
		}
		blob, err := s.readRegistryBlob(desc.Digest)
		if err != nil {
			s.writeRegistryError(w, http.StatusBadRequest, "BLOB_UNKNOWN", "manifest references unknown blob "+desc.Digest)
			return
		}
		if desc.Size != 0 && int64(len(blob)) != desc.Size {
			s.writeRegistryError(w, http.StatusBadRequest, "MANIFEST_INVALID", "manifest blob size mismatch for "+desc.Digest)
			return
		}
		files = append(files, PackageFileInput{
			Name:          registryBlobFileName(desc.Digest),
			ContentType:   firstNonEmpty(desc.MediaType, "application/octet-stream"),
			ContentBase64: base64.StdEncoding.EncodeToString(blob),
		})
	}
	metadata := map[string]interface{}{
		"package_type": "container",
		"container": map[string]interface{}{
			"tags": registryReferenceTags(reference),
		},
	}
	v, err := s.store.CreatePackageVersion(ownerType, ownerKey, "container", pkgName, reference, "Published through the OCI registry data plane.", metadata, files)
	if err != nil {
		s.writeRegistryError(w, http.StatusConflict, "MANIFEST_INVALID", err.Error())
		return
	}
	s.store.SetPackageVersionRegistryManifestDigest(v.ID, manifestDigest)
	w.Header().Set("Docker-Content-Digest", manifestDigest)
	w.Header().Set("Location", "/v2/"+name+"/manifests/"+reference)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(s.packageVersionToJSON(v, p, s.baseURL(r), packageScopePath(ownerType, ownerKey)))
}

func (s *Server) handleContainerRegistryGetManifest(w http.ResponseWriter, r *http.Request, name, reference string) {
	if s.requireRegistryUser(w, r) == nil {
		return
	}
	ownerType, ownerKey, ok := s.resolveContainerRegistryOwner(w, name)
	if !ok {
		return
	}
	p := s.store.GetPackage(ownerKey, "container", containerRegistryPackageName(name))
	if p == nil || p.OwnerType != ownerType {
		s.writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository name not known")
		return
	}
	var version *PackageVersion
	for _, cand := range s.store.ListPackageVersions(p.ID, false) {
		if cand.Version == reference || packageVersionManifestDigest(cand) == reference {
			version = cand
			break
		}
	}
	if version == nil {
		s.writeRegistryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest unknown")
		return
	}
	data, contentType, ok := s.packageVersionFileData(version.ID, "manifest.json")
	if !ok {
		s.writeRegistryError(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest bytes are missing")
		return
	}
	digest := packageVersionManifestDigest(version)
	if digest == "" {
		digest = digestSHA256(data)
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.Header().Set("Content-Type", firstNonEmpty(contentType, "application/vnd.oci.image.manifest.v1+json"))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) requireRegistryUser(w http.ResponseWriter, r *http.Request) *User {
	ctx := s.authenticateRequest(r)
	user := ghUserFromContext(ctx)
	if user == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+s.baseURL(r)+`/v2/"`)
		s.writeRegistryError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return nil
	}
	return user
}

func (s *Server) resolveContainerRegistryOwner(w http.ResponseWriter, name string) (string, string, bool) {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		s.writeRegistryError(w, http.StatusNotFound, "NAME_INVALID", "repository name must be owner/package")
		return "", "", false
	}
	owner := parts[0]
	if u := s.store.LookupUserByLogin(owner); u != nil {
		return "User", u.Login, true
	}
	if org := s.store.GetOrg(owner); org != nil {
		return "Organization", org.Login, true
	}
	s.writeRegistryError(w, http.StatusNotFound, "NAME_UNKNOWN", "repository owner not found")
	return "", "", false
}

func (s *Server) canPublishContainerRegistryPackage(user *User, ownerType, ownerKey string) bool {
	switch ownerType {
	case "User":
		return user.Login == ownerKey
	case "Organization":
		if org := s.store.GetOrg(ownerKey); org != nil {
			return canAdminOrg(s.store, user, org)
		}
	}
	return false
}

func (s *Server) containerRegistryUploadLocation(name, uploadID string) string {
	return "/v2/" + name + "/blobs/uploads/" + url.PathEscape(uploadID)
}

func containerRegistryPackageName(name string) string {
	parts := strings.Split(strings.Trim(name, "/"), "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[1:], "/")
}

func registryReferenceTags(reference string) []string {
	if strings.HasPrefix(reference, "sha256:") {
		return []string{}
	}
	return []string{reference}
}

func registryBlobFileName(digest string) string {
	algo, hexPart, ok := strings.Cut(digest, ":")
	if !ok {
		return "blobs/" + digest
	}
	return "blobs/" + algo + "/" + hexPart
}

func packageVersionManifestDigest(v *PackageVersion) string {
	return v.RegistryManifestDigest
}

func (s *Server) writeRegistryBlob(digest string, data []byte) error {
	if s.store.ObjectByteStore != nil {
		return s.store.ObjectByteStore.Put(context.Background(), packageRegistryBlobDataKey(digest), data)
	}
	if s.store.PackageDataDir == "" {
		return fmt.Errorf("package file storage is not configured")
	}
	path, err := s.registryBlobPath(digest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *Server) readRegistryBlob(digest string) ([]byte, error) {
	if s.store.ObjectByteStore != nil {
		return s.store.ObjectByteStore.Get(context.Background(), packageRegistryBlobDataKey(digest))
	}
	path, err := s.registryBlobPath(digest)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *Server) registryBlobPath(digest string) (string, error) {
	if s.store.PackageDataDir == "" {
		return "", fmt.Errorf("package file storage is not configured")
	}
	algo, value, ok := strings.Cut(digest, ":")
	if !ok || algo == "" || value == "" {
		return "", fmt.Errorf("invalid digest")
	}
	if algo != "sha256" {
		return "", fmt.Errorf("unsupported digest algorithm %q", algo)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("invalid digest")
	}
	return filepath.Join(s.store.PackageDataDir, "registry-blobs", algo, value), nil
}

func (s *Server) packageVersionFileData(versionID int, name string) ([]byte, string, bool) {
	for _, file := range s.store.ListPackageFiles(versionID) {
		if file.Name != name {
			continue
		}
		var (
			data []byte
			err  error
		)
		if s.store.ObjectByteStore != nil {
			data, err = s.store.ObjectByteStore.Get(context.Background(), file.StoragePath)
		} else {
			data, err = os.ReadFile(file.StoragePath)
		}
		if err != nil {
			return nil, "", false
		}
		return data, file.ContentType, true
	}
	return nil, "", false
}

func digestSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) writeRegistryError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []map[string]string{{
			"code":    code,
			"message": message,
		}},
	})
}
