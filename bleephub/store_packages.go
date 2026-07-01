package bleephub

import (
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
type Package struct {
	ID           int       `json:"id"`
	NodeID       string    `json:"node_id"`
	Name         string    `json:"name"`
	PackageType  string    `json:"package_type"`
	OwnerType    string    `json:"-"` // "User", "Organization", "Repository"
	OwnerKey     string    `json:"-"` // username, org login, or owner/repo
	Visibility   string    `json:"visibility"`
	URL          string    `json:"url"`
	HTMLURL      string    `json:"html_url"`
	VersionCount int       `json:"version_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PackageVersion is a version of a package.
type PackageVersion struct {
	ID          int                    `json:"id"`
	NodeID      string                 `json:"node_id"`
	PackageID   int                    `json:"-"`
	Version     string                 `json:"name"` // GitHub calls the version "name"
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
	URL         string                 `json:"url"`
	HTMLURL     string                 `json:"html_url"`
	PackageURL  string                 `json:"package_html_url"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Deleted     bool                   `json:"-"`
	DeletedAt   *time.Time             `json:"deleted_at,omitempty"`
}

// PackageFile is a single file attached to a package version.
type PackageFile struct {
	ID          int    `json:"id"`
	NodeID      string `json:"node_id"`
	VersionID   int    `json:"-"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
	StoragePath string `json:"-"`
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
	return filepath.Join(st.packageStorageBase(ownerType, ownerKey, pkgType, name), sanitizePackagePathSegment(version))
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

// DeletePackage removes a package, all its versions, and all files.
func (st *Store) DeletePackage(ownerKey, pkgType, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := packageKey(pkgType, name)
	p := st.PackagesByOwnerKey[ownerKey][key]
	if p == nil {
		return false
	}
	for _, v := range st.PackageVersionsByPackage[p.ID] {
		for _, f := range st.PackageFilesByVersion[v.ID] {
			delete(st.PackageFiles, f.ID)
			if f.StoragePath != "" {
				_ = os.Remove(f.StoragePath)
			}
		}
		delete(st.PackageVersions, v.ID)
	}
	delete(st.Packages, p.ID)
	delete(st.PackagesByOwnerKey[ownerKey], key)
	if base := st.packageStorageBase(p.OwnerType, p.OwnerKey, p.PackageType, p.Name); base != "" {
		_ = os.RemoveAll(base)
	}
	if st.persist != nil {
		st.persist.MustDelete("packages", strconv.Itoa(p.ID))
	}
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
	st.PackageVersions[id] = v
	if st.PackageVersionsByPackage[p.ID] == nil {
		st.PackageVersionsByPackage[p.ID] = map[int]*PackageVersion{}
	}
	st.PackageVersionsByPackage[p.ID][id] = v
	st.NextPackageVersionID++

	vdir := st.versionStorageDir(ownerType, ownerKey, pkgType, pkgName, version)
	for _, fin := range files {
		data, err := base64.StdEncoding.DecodeString(fin.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("decode file %q: %w", fin.Name, err)
		}
		fid := st.NextPackageFileID
		pf := &PackageFile{
			ID:          fid,
			NodeID:      packageFileNodeID(fid),
			VersionID:   v.ID,
			Name:        fin.Name,
			ContentType: fin.ContentType,
			Size:        int64(len(data)),
			StoragePath: filepath.Join(vdir, sanitizePackagePathSegment(fin.Name)),
		}
		if vdir != "" {
			if err := os.MkdirAll(vdir, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir %s: %w", vdir, err)
			}
			if err := os.WriteFile(pf.StoragePath, data, 0o644); err != nil {
				return nil, fmt.Errorf("write file %s: %w", pf.StoragePath, err)
			}
		}
		st.PackageFiles[fid] = pf
		if st.PackageFilesByVersion[v.ID] == nil {
			st.PackageFilesByVersion[v.ID] = map[int]*PackageFile{}
		}
		st.PackageFilesByVersion[v.ID][fid] = pf
		st.NextPackageFileID++
	}

	st.recomputeVersionCountLocked(p)
	st.persistPackageVersion(v)
	st.persistPackage(p)
	return v, nil
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

// PackageVersionURL returns the public API URL for a package version.
func (s *Server) packageVersionURL(baseURL, scopePath string, p *Package, v *PackageVersion) string {
	return baseURL + "/api/v3" + scopePath + "/" + url.PathEscape(p.PackageType) + "/" + url.PathEscape(p.Name) + "/versions/" + strconv.Itoa(v.ID)
}

// packageURL returns the public API URL for a package.
func (s *Server) packageURL(baseURL, scopePath string, p *Package) string {
	return baseURL + "/api/v3" + scopePath + "/" + url.PathEscape(p.PackageType) + "/" + url.PathEscape(p.Name)
}
