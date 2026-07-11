package bleephub

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PackageType values supported by GitHub Packages.
var packageTypes = map[string]bool{
	"npm":       true,
	"maven":     true,
	"rubygems":  true,
	"nuget":     true,
	"docker":    true,
	"container": true,
}

func isPackageType(t string) bool { return packageTypes[t] }

// Package is a GitHub software package (npm, container, maven, ...).
// The struct's JSON tags define the persistence row shape (API responses
// are built by packageToJSON); linkage fields must round-trip through
// persistence so packages survive a restart.
type Package struct {
	ID           int        `json:"id"`
	NodeID       string     `json:"node_id"`
	Name         string     `json:"name"`
	PackageType  string     `json:"package_type"`
	OwnerType    string     `json:"owner_type"` // "User", "Organization", "Repository"
	OwnerKey     string     `json:"owner_key"`  // username, org login, or owner/repo
	Visibility   string     `json:"visibility"`
	URL          string     `json:"url"`
	HTMLURL      string     `json:"html_url"`
	VersionCount int        `json:"version_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Deleted      bool       `json:"deleted,omitempty"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// PackageVersion is a version of a package. JSON tags define the
// persistence row shape.
type PackageVersion struct {
	ID          int                    `json:"id"`
	NodeID      string                 `json:"node_id"`
	PackageID   int                    `json:"package_id"`
	Version     string                 `json:"name"` // GitHub calls the version "name"
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
	// RegistryManifestDigest is internal persisted registry lookup state;
	// GitHub REST package-version responses expose container tags in metadata.
	RegistryManifestDigest string     `json:"registry_manifest_digest,omitempty"`
	URL                    string     `json:"url"`
	HTMLURL                string     `json:"html_url"`
	PackageURL             string     `json:"package_html_url"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	Deleted                bool       `json:"deleted,omitempty"`
	DeletedAt              *time.Time `json:"deleted_at,omitempty"`
}

// PackageFile is a single file attached to a package version. JSON tags
// define the persistence row shape.
type PackageFile struct {
	ID          int    `json:"id"`
	NodeID      string `json:"node_id"`
	VersionID   int    `json:"version_id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
	StoragePath string `json:"storage_path,omitempty"`
}

type decodedPackageFileInput struct {
	Name        string
	ContentType string
	Data        []byte
}

func packageNodeID(id int) string        { return fmt.Sprintf("P_kgDO%08d", id) }
func packageVersionNodeID(id int) string { return fmt.Sprintf("PV_kgDO%08d", id) }
func packageFileNodeID(id int) string    { return fmt.Sprintf("PF_kgDO%08d", id) }

// packageKey is the unique lookup key for a package within an owner scope.
func packageKey(pkgType, name string) string { return pkgType + "/" + name }

// sanitizePackagePathSegment escapes path separators so an arbitrary package
// name or version cannot traverse out of the packages data directory.
func sanitizePackagePathSegment(s string) string {
	s = strings.ReplaceAll(s, "\\", "%5C")
	s = strings.ReplaceAll(s, "/", "%2F")
	s = strings.ReplaceAll(s, "..", "%2E%2E")
	return s
}

// packageStorageBase returns the directory under which a package's files live.
func (st *Store) packageStorageBase(ownerType, ownerKey, pkgType, name string) string {
	if st.PackageDataDir == "" {
		return ""
	}
	var ownerSeg string
	switch ownerType {
	case "User":
		ownerSeg = filepath.Join("users", sanitizePackagePathSegment(ownerKey))
	case "Organization":
		ownerSeg = filepath.Join("orgs", sanitizePackagePathSegment(ownerKey))
	case "Repository":
		parts := strings.SplitN(ownerKey, "/", 2)
		if len(parts) == 2 {
			ownerSeg = filepath.Join("repos", sanitizePackagePathSegment(parts[0]), sanitizePackagePathSegment(parts[1]))
		} else {
			ownerSeg = filepath.Join("repos", sanitizePackagePathSegment(ownerKey))
		}
	default:
		ownerSeg = sanitizePackagePathSegment(ownerKey)
	}
	return filepath.Join(st.PackageDataDir, "packages", ownerSeg, sanitizePackagePathSegment(pkgType), sanitizePackagePathSegment(name))
}

// versionStorageDir returns the directory for a specific version's files.
func (st *Store) versionStorageDir(ownerType, ownerKey, pkgType, name, version string) string {
	base := st.packageStorageBase(ownerType, ownerKey, pkgType, name)
	if base == "" {
		return ""
	}
	return filepath.Join(base, sanitizePackagePathSegment(version))
}

// CreatePackage creates or returns an existing package.
func (st *Store) CreatePackage(ownerType, ownerKey, pkgType, name, visibility string) (*Package, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if visibility == "" {
		visibility = "public"
	}
	key := packageKey(pkgType, name)
	if p := st.PackagesByOwnerKey[ownerKey][key]; p != nil {
		return p, false
	}
	now := time.Now().UTC()
	id := st.NextPackageID
	p := &Package{
		ID:          id,
		NodeID:      packageNodeID(id),
		Name:        name,
		PackageType: pkgType,
		OwnerType:   ownerType,
		OwnerKey:    ownerKey,
		Visibility:  visibility,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	st.Packages[id] = p
	if st.PackagesByOwnerKey[ownerKey] == nil {
		st.PackagesByOwnerKey[ownerKey] = map[string]*Package{}
	}
	st.PackagesByOwnerKey[ownerKey][key] = p
	st.NextPackageID++
	st.persistPackage(p)
	return p, true
}

// GetPackage returns a package by owner/type/name, or nil.
func (st *Store) GetPackage(ownerKey, pkgType, name string) *Package {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.PackagesByOwnerKey[ownerKey][packageKey(pkgType, name)]
}

// ListPackages returns packages for an owner, newest first.
func (st *Store) ListPackages(ownerKey string) []*Package {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Package
	for _, p := range st.PackagesByOwnerKey[ownerKey] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// DeletePackage soft-deletes a package: it leaves the by-owner key map
// (so lists and gets no longer see it, and the name can be reused) while
// keeping the row, its versions, and its files so the package remains
// restorable — GitHub's delete/restore contract for packages.
func (st *Store) DeletePackage(ownerKey, pkgType, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := packageKey(pkgType, name)
	p := st.PackagesByOwnerKey[ownerKey][key]
	if p == nil {
		return false
	}
	now := time.Now().UTC()
	p.Deleted = true
	p.DeletedAt = &now
	p.UpdatedAt = now
	delete(st.PackagesByOwnerKey[ownerKey], key)
	st.persistPackage(p)
	return true
}

// GetDeletedPackage returns a soft-deleted package for an owner scope,
// or nil.
func (st *Store) GetDeletedPackage(ownerKey, pkgType, name string) *Package {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, p := range st.Packages {
		if p.Deleted && p.OwnerKey == ownerKey && p.PackageType == pkgType && p.Name == name {
			return p
		}
	}
	return nil
}

// RestorePackage un-deletes a soft-deleted package together with its
// versions and files. Restore fails when no deleted package exists or
// when a live package has since claimed the same name.
func (st *Store) RestorePackage(ownerKey, pkgType, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := packageKey(pkgType, name)
	if st.PackagesByOwnerKey[ownerKey][key] != nil {
		return false
	}
	var p *Package
	for _, cand := range st.Packages {
		if cand.Deleted && cand.OwnerKey == ownerKey && cand.PackageType == pkgType && cand.Name == name {
			p = cand
			break
		}
	}
	if p == nil {
		return false
	}
	p.Deleted = false
	p.DeletedAt = nil
	p.UpdatedAt = time.Now().UTC()
	if st.PackagesByOwnerKey[ownerKey] == nil {
		st.PackagesByOwnerKey[ownerKey] = map[string]*Package{}
	}
	st.PackagesByOwnerKey[ownerKey][key] = p
	st.persistPackage(p)
	return true
}

// CreatePackageVersion creates a new version of a package and persists files.
func (st *Store) CreatePackageVersion(ownerType, ownerKey, pkgType, pkgName, version, description string, metadata map[string]interface{}, files []PackageFileInput) (*PackageVersion, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	p := st.PackagesByOwnerKey[ownerKey][packageKey(pkgType, pkgName)]
	if p == nil {
		return nil, fmt.Errorf("package not found")
	}
	for _, existing := range st.PackageVersionsByPackage[p.ID] {
		if !existing.Deleted && existing.Version == version {
			return nil, fmt.Errorf("package version %q already exists", version)
		}
	}
	vdir := st.versionStorageDir(ownerType, ownerKey, pkgType, pkgName, version)
	if len(files) > 0 && vdir == "" && st.ObjectByteStore == nil {
		return nil, fmt.Errorf("package file storage is not configured")
	}
	decodedFiles := make([]decodedPackageFileInput, 0, len(files))
	for _, fin := range files {
		data, err := base64.StdEncoding.DecodeString(fin.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("decode file %q: %w", fin.Name, err)
		}
		decodedFiles = append(decodedFiles, decodedPackageFileInput{
			Name:        fin.Name,
			ContentType: fin.ContentType,
			Data:        data,
		})
	}
	now := time.Now().UTC()
	id := st.NextPackageVersionID
	v := &PackageVersion{
		ID:          id,
		NodeID:      packageVersionNodeID(id),
		PackageID:   p.ID,
		Version:     version,
		Description: description,
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	persistedFiles := make([]*PackageFile, 0, len(decodedFiles))
	for i, fin := range decodedFiles {
		fid := st.NextPackageFileID + i
		pf := &PackageFile{
			ID:          fid,
			NodeID:      packageFileNodeID(fid),
			VersionID:   v.ID,
			Name:        fin.Name,
			ContentType: fin.ContentType,
			Size:        int64(len(fin.Data)),
		}
		if st.ObjectByteStore != nil {
			pf.StoragePath = packageFileDataKey(fid)
			if err := st.ObjectByteStore.Put(context.Background(), pf.StoragePath, fin.Data); err != nil {
				return nil, fmt.Errorf("write package file %s: %w", fin.Name, err)
			}
		} else if vdir != "" {
			pf.StoragePath = filepath.Join(vdir, sanitizePackagePathSegment(fin.Name))
			if err := os.MkdirAll(vdir, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir %s: %w", vdir, err)
			}
			if err := os.WriteFile(pf.StoragePath, fin.Data, 0o644); err != nil {
				return nil, fmt.Errorf("write file %s: %w", pf.StoragePath, err)
			}
		}
		persistedFiles = append(persistedFiles, pf)
	}

	st.PackageVersions[id] = v
	if st.PackageVersionsByPackage[p.ID] == nil {
		st.PackageVersionsByPackage[p.ID] = map[int]*PackageVersion{}
	}
	st.PackageVersionsByPackage[p.ID][id] = v
	st.NextPackageVersionID++

	for _, pf := range persistedFiles {
		st.PackageFiles[pf.ID] = pf
		if st.PackageFilesByVersion[v.ID] == nil {
			st.PackageFilesByVersion[v.ID] = map[int]*PackageFile{}
		}
		st.PackageFilesByVersion[v.ID][pf.ID] = pf
		st.NextPackageFileID++
		st.persistPackageFile(pf)
	}

	st.recomputeVersionCountLocked(p)
	st.persistPackageVersion(v)
	st.persistPackage(p)
	return v, nil
}

func (st *Store) deletePackageFilesForOwnerLocked(ownerKey string) error {
	for _, pkg := range st.PackagesByOwnerKey[ownerKey] {
		for versionID := range st.PackageVersionsByPackage[pkg.ID] {
			for _, file := range st.PackageFilesByVersion[versionID] {
				if err := st.deletePackageFileDataLocked(file); err != nil {
					return fmt.Errorf("delete package file %d (%s): %w", file.ID, file.Name, err)
				}
			}
		}
	}
	return nil
}

func (st *Store) deletePackageFileDataLocked(file *PackageFile) error {
	if file == nil || file.StoragePath == "" {
		return nil
	}
	if st.ObjectByteStore != nil {
		return st.ObjectByteStore.Delete(context.Background(), file.StoragePath)
	}
	if err := os.Remove(file.StoragePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PackageFileInput is the wire payload for an uploaded package file.
type PackageFileInput struct {
	Name          string `json:"name"`
	ContentType   string `json:"content_type"`
	ContentBase64 string `json:"content_base64"`
}

// GetPackageVersion returns a package version by ID, or nil.
func (st *Store) GetPackageVersion(id int) *PackageVersion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.PackageVersions[id]
}

// ListPackageVersions returns versions for a package, newest first.
// If includeDeleted is false, deleted versions are omitted.
func (st *Store) ListPackageVersions(pkgID int, includeDeleted bool) []*PackageVersion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*PackageVersion
	for _, v := range st.PackageVersionsByPackage[pkgID] {
		if v.Deleted && !includeDeleted {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// DeletePackageVersion marks a version as deleted.
func (st *Store) DeletePackageVersion(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	v := st.PackageVersions[id]
	if v == nil || v.Deleted {
		return false
	}
	now := time.Now().UTC()
	v.Deleted = true
	v.DeletedAt = &now
	v.UpdatedAt = now
	if p := st.Packages[v.PackageID]; p != nil {
		st.recomputeVersionCountLocked(p)
		st.persistPackage(p)
	}
	st.persistPackageVersion(v)
	return true
}

// RestorePackageVersion unmarks a deleted version.
func (st *Store) RestorePackageVersion(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	v := st.PackageVersions[id]
	if v == nil || !v.Deleted {
		return false
	}
	v.Deleted = false
	v.DeletedAt = nil
	v.UpdatedAt = time.Now().UTC()
	if p := st.Packages[v.PackageID]; p != nil {
		st.recomputeVersionCountLocked(p)
		st.persistPackage(p)
	}
	st.persistPackageVersion(v)
	return true
}

func (st *Store) SetPackageVersionRegistryManifestDigest(id int, digest string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	v := st.PackageVersions[id]
	if v == nil {
		return false
	}
	v.RegistryManifestDigest = digest
	v.UpdatedAt = time.Now().UTC()
	st.persistPackageVersion(v)
	return true
}

// GetPackageFile returns a package file by ID, or nil.
func (st *Store) GetPackageFile(id int) *PackageFile {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.PackageFiles[id]
}

// ListPackageFiles returns files for a version.
func (st *Store) ListPackageFiles(versionID int) []*PackageFile {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*PackageFile
	for _, f := range st.PackageFilesByVersion[versionID] {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (st *Store) recomputeVersionCountLocked(p *Package) {
	count := 0
	for _, v := range st.PackageVersionsByPackage[p.ID] {
		if !v.Deleted {
			count++
		}
	}
	p.VersionCount = count
	p.UpdatedAt = time.Now().UTC()
}

func (st *Store) persistPackage(p *Package) {
	if st.persist != nil {
		st.persist.MustPut("packages", strconv.Itoa(p.ID), p)
	}
}

func (st *Store) persistPackageVersion(v *PackageVersion) {
	if st.persist != nil {
		st.persist.MustPut("package_versions", strconv.Itoa(v.ID), v)
	}
}

func (st *Store) persistPackageFile(f *PackageFile) {
	if st.persist != nil {
		st.persist.MustPut("package_files", strconv.Itoa(f.ID), f)
	}
}

// PackageVersionURL returns the public API URL for a package version.
func (s *Server) packageVersionURL(baseURL, scopePath string, p *Package, v *PackageVersion) string {
	return baseURL + "/api/v3" + scopePath + "/" + url.PathEscape(p.PackageType) + "/" + url.PathEscape(p.Name) + "/versions/" + strconv.Itoa(v.ID)
}

// packageURL returns the public API URL for a package.
func (s *Server) packageURL(baseURL, scopePath string, p *Package) string {
	return baseURL + "/api/v3" + scopePath + "/" + url.PathEscape(p.PackageType) + "/" + url.PathEscape(p.Name)
}
