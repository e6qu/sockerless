package bleephub

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

func (s *Server) registerGHPackagesRoutes() {
	// GitHub Container Registry-compatible OCI/Docker Registry HTTP API v2.
	s.route("GET /v2/{rest...}", s.handleContainerRegistry)
	s.route("HEAD /v2/{rest...}", s.handleContainerRegistry)
	s.route("POST /v2/{rest...}", s.handleContainerRegistry)
	s.route("PATCH /v2/{rest...}", s.handleContainerRegistry)
	s.route("PUT /v2/{rest...}", s.handleContainerRegistry)

	// Authenticated-user scoped
	s.route("GET /api/v3/user/packages", s.handleListAuthUserPackages)
	s.route("GET /api/v3/user/packages/{package_type}/{package_name}", s.handleGetAuthUserPackage)
	s.route("DELETE /api/v3/user/packages/{package_type}/{package_name}", s.handleDeleteAuthUserPackage)
	s.route("POST /api/v3/user/packages/{package_type}/{package_name}/restore", s.handleRestoreAuthUserPackage)
	s.route("GET /api/v3/user/packages/{package_type}/{package_name}/versions", s.handleListAuthUserPackageVersions)
	s.route("GET /api/v3/user/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleGetAuthUserPackageVersion)
	s.route("DELETE /api/v3/user/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleDeleteAuthUserPackageVersion)
	s.route("POST /api/v3/user/packages/{package_type}/{package_name}/versions/{package_version_id}/restore", s.handleRestoreAuthUserPackageVersion)

	// Docker registry migration conflicts
	s.route("GET /api/v3/user/docker/conflicts", s.handleListAuthUserDockerConflicts)
	s.route("GET /api/v3/users/{username}/docker/conflicts", s.handleListUserDockerConflicts)

	// User-scoped
	s.route("GET /api/v3/users/{username}/packages", s.handleListUserPackages)
	s.route("POST /api/v3/users/{username}/packages/{package_type}/{package_name}/restore", s.handleRestoreUserPackage)
	s.route("GET /api/v3/users/{username}/packages/{package_type}/{package_name}", s.handleGetUserPackage)
	s.route("DELETE /api/v3/users/{username}/packages/{package_type}/{package_name}", s.handleDeleteUserPackage)
	s.route("GET /api/v3/users/{username}/packages/{package_type}/{package_name}/versions", s.handleListUserPackageVersions)
	s.route("GET /api/v3/users/{username}/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleGetUserPackageVersion)
	s.route("DELETE /api/v3/users/{username}/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleDeleteUserPackageVersion)
	s.route("POST /api/v3/users/{username}/packages/{package_type}/{package_name}/versions/{package_version_id}/restore", s.handleRestoreUserPackageVersion)
	s.route("GET /api/v3/users/{username}/packages/{package_type}/{package_name}/versions/{package_version_id}/files", s.handleListUserPackageFiles)
	s.route("GET /api/v3/users/{username}/packages/{package_type}/{package_name}/versions/{package_version_id}/files/{file_id}", s.handleDownloadUserPackageFile)

	// Org-scoped
	s.route("GET /api/v3/orgs/{org}/packages", s.handleListOrgPackages)
	s.route("POST /api/v3/orgs/{org}/packages/{package_type}/{package_name}/restore", s.handleRestoreOrgPackage)
	s.route("GET /api/v3/orgs/{org}/packages/{package_type}/{package_name}", s.handleGetOrgPackage)
	s.route("DELETE /api/v3/orgs/{org}/packages/{package_type}/{package_name}", s.handleDeleteOrgPackage)
	s.route("GET /api/v3/orgs/{org}/docker/conflicts", s.handleListOrgDockerConflicts)
	s.route("GET /api/v3/orgs/{org}/packages/{package_type}/{package_name}/versions", s.handleListOrgPackageVersions)
	s.route("GET /api/v3/orgs/{org}/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleGetOrgPackageVersion)
	s.route("DELETE /api/v3/orgs/{org}/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleDeleteOrgPackageVersion)
	s.route("POST /api/v3/orgs/{org}/packages/{package_type}/{package_name}/versions/{package_version_id}/restore", s.handleRestoreOrgPackageVersion)
	s.route("GET /api/v3/orgs/{org}/packages/{package_type}/{package_name}/versions/{package_version_id}/files", s.handleListOrgPackageFiles)
	s.route("GET /api/v3/orgs/{org}/packages/{package_type}/{package_name}/versions/{package_version_id}/files/{file_id}", s.handleDownloadOrgPackageFile)

	// Repo-scoped
	s.route("GET /api/v3/repos/{owner}/{repo}/packages", s.handleListRepoPackages)
	s.route("GET /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}", s.handleGetRepoPackage)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}", s.handleDeleteRepoPackage)
	s.route("GET /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}/versions", s.handleListRepoPackageVersions)
	s.route("GET /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleGetRepoPackageVersion)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}/versions/{package_version_id}", s.handleDeleteRepoPackageVersion)
	s.route("GET /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}/versions/{package_version_id}/files", s.handleListRepoPackageFiles)
	s.route("GET /api/v3/repos/{owner}/{repo}/packages/{package_type}/{package_name}/versions/{package_version_id}/files/{file_id}", s.handleDownloadRepoPackageFile)
}

// ─── Internal upload endpoint ───────────────────────────────────────────

type packageVersionCreateBody struct {
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
	Files       []PackageFileInput     `json:"files"`
}

func (s *Server) handleInternalCreatePackageVersion(w http.ResponseWriter, r *http.Request) {
	ownerType := r.PathValue("owner_type")
	owner := r.PathValue("owner")
	if repo := r.PathValue("repo"); repo != "" {
		owner = owner + "/" + repo
	}
	pkgType := r.PathValue("package_type")
	pkgName := r.PathValue("package_name")

	if !isPackageType(pkgType) {
		writeGHValidationError(w, "Package", "package_type", "invalid")
		return
	}
	if pkgType == "container" {
		writeGHValidationError(w, "Package", "package_type", "use_registry_data_plane")
		return
	}

	var body packageVersionCreateBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Version == "" {
		writeGHValidationError(w, "PackageVersion", "version", "missing_field")
		return
	}

	var resolvedOwnerKey string
	switch ownerType {
	case "user":
		u := s.store.LookupUserByLogin(owner)
		if u == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		resolvedOwnerKey = u.Login
	case "org":
		org := s.store.GetOrg(owner)
		if org == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		resolvedOwnerKey = org.Login
	case "repository":
		repo := s.store.ReposByName[owner]
		if repo == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		resolvedOwnerKey = repo.FullName
	default:
		writeGHValidationError(w, "Package", "owner_type", "invalid")
		return
	}

	// Internal uploads are public by default.
	p, _ := s.store.CreatePackage(ownerTypeFromInternal(ownerType), resolvedOwnerKey, pkgType, pkgName, "public")
	v, err := s.store.CreatePackageVersion(ownerTypeFromInternal(ownerType), resolvedOwnerKey, pkgType, pkgName, body.Version, body.Description, body.Metadata, body.Files)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	baseURL := s.baseURL(r)
	scopePath := packageScopePath(ownerTypeFromInternal(ownerType), resolvedOwnerKey)
	out := s.packageVersionToJSON(v, p, baseURL, scopePath)
	writeJSON(w, http.StatusCreated, out)
}

func ownerTypeFromInternal(t string) string {
	switch t {
	case "user":
		return "User"
	case "org":
		return "Organization"
	case "repository":
		return "Repository"
	default:
		return t
	}
}

func packageScopePath(ownerType, ownerKey string) string {
	switch ownerType {
	case "User":
		return "/users/" + ownerKey
	case "Organization":
		return "/orgs/" + ownerKey
	case "Repository":
		return "/repos/" + ownerKey
	default:
		return "/users/" + ownerKey
	}
}

// ─── User-scoped handlers ───────────────────────────────────────────────

func (s *Server) handleListUserPackages(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	owner := r.PathValue("username")
	u := s.store.LookupUserByLogin(owner)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	pkgType, visibility, ok := packageListFilters(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, s.listOwnerPackagesJSON(r, user, u.Login, pkgType, visibility)))
}

func (s *Server) handleGetUserPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveUserPackage(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.packageToJSON(p, s.baseURL(r)))
}

func (s *Server) handleDeleteUserPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveUserPackage(w, r)
	if !ok {
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	if !s.store.DeletePackage(p.OwnerKey, p.PackageType, p.Name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListUserPackageVersions(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveUserPackage(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.listVersionsJSON(p, r))
}

func (s *Server) handleGetUserPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveUserPackageVersion(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.packageVersionToJSON(v, p, s.baseURL(r), packageScopePath(p.OwnerType, p.OwnerKey)))
}

func (s *Server) handleDeleteUserPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, ok := s.resolveUserPackageVersion(w, r)
	if !ok {
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("package_version_id"))
	if !s.store.DeletePackageVersion(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreUserPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, ok := s.resolveUserPackageVersion(w, r)
	if !ok {
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("package_version_id"))
	if !s.store.RestorePackageVersion(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListUserPackageFiles(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveUserPackageVersion(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.listFilesJSON(v, p, r))
}

func (s *Server) handleDownloadUserPackageFile(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, f, ok := s.resolveUserPackageFile(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	s.servePackageFile(w, r, f)
}

// ─── Authenticated-user scoped handlers ─────────────────────────────────

// packageListFilters parses the package_type (required by GitHub's list
// endpoints) and visibility query parameters.
func packageListFilters(w http.ResponseWriter, r *http.Request) (pkgType, visibility string, ok bool) {
	pkgType = r.URL.Query().Get("package_type")
	if pkgType == "" {
		writeGHError(w, http.StatusBadRequest, "Invalid request.\n\n\"package_type\" wasn't supplied.")
		return "", "", false
	}
	if !isPackageType(pkgType) {
		writeGHError(w, http.StatusBadRequest, "Invalid request.\n\n\"package_type\" isn't a valid package type.")
		return "", "", false
	}
	return pkgType, r.URL.Query().Get("visibility"), true
}

// listOwnerPackagesJSON renders the packages of one owner scope filtered
// by type, visibility, and viewer access.
func (s *Server) listOwnerPackagesJSON(r *http.Request, user *User, ownerKey, pkgType, visibility string) []map[string]interface{} {
	pkgs := s.store.ListPackages(ownerKey)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(pkgs))
	for _, p := range pkgs {
		if p.PackageType != pkgType {
			continue
		}
		if visibility != "" && p.Visibility != visibility {
			continue
		}
		if !s.canViewPackage(user, p) {
			continue
		}
		out = append(out, s.packageToJSON(p, baseURL))
	}
	return out
}

func (s *Server) handleListAuthUserPackages(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	pkgType, visibility, ok := packageListFilters(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, s.listOwnerPackagesJSON(r, user, user.Login, pkgType, visibility)))
}

// resolveAuthUserPackage resolves {package_type}/{package_name} within
// the authenticated user's own package namespace.
func (s *Server) resolveAuthUserPackage(w http.ResponseWriter, r *http.Request, user *User) (*Package, bool) {
	pkgType := r.PathValue("package_type")
	pkgName := r.PathValue("package_name")
	if !isPackageType(pkgType) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	p := s.store.GetPackage(user.Login, pkgType, pkgName)
	if p == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return p, true
}

func (s *Server) handleGetAuthUserPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveAuthUserPackage(w, r, user)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.packageToJSON(p, s.baseURL(r)))
}

func (s *Server) handleDeleteAuthUserPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveAuthUserPackage(w, r, user)
	if !ok {
		return
	}
	if !s.store.DeletePackage(p.OwnerKey, p.PackageType, p.Name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreAuthUserPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	s.restorePackageForOwner(w, r, user, user.Login)
}

func (s *Server) handleListAuthUserPackageVersions(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveAuthUserPackage(w, r, user)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.listVersionsJSON(p, r))
}

func (s *Server) resolveAuthUserPackageVersion(w http.ResponseWriter, r *http.Request, user *User) (*Package, *PackageVersion, bool) {
	p, ok := s.resolveAuthUserPackage(w, r, user)
	if !ok {
		return nil, nil, false
	}
	v, ok := s.resolveVersion(w, r)
	if !ok || v.PackageID != p.ID {
		return nil, nil, false
	}
	return p, v, true
}

func (s *Server) handleGetAuthUserPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveAuthUserPackageVersion(w, r, user)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.packageVersionToJSON(v, p, s.baseURL(r), packageScopePath(p.OwnerType, p.OwnerKey)))
}

func (s *Server) handleDeleteAuthUserPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	_, v, ok := s.resolveAuthUserPackageVersion(w, r, user)
	if !ok {
		return
	}
	if !s.store.DeletePackageVersion(v.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreAuthUserPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	_, v, ok := s.resolveAuthUserPackageVersion(w, r, user)
	if !ok {
		return
	}
	if !s.store.RestorePackageVersion(v.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Package restore (owner-scoped) ─────────────────────────────────────

// restorePackageForOwner restores a soft-deleted package in an owner
// namespace after checking the caller may administer it.
func (s *Server) restorePackageForOwner(w http.ResponseWriter, r *http.Request, user *User, ownerKey string) {
	pkgType := r.PathValue("package_type")
	pkgName := r.PathValue("package_name")
	if !isPackageType(pkgType) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	p := s.store.GetDeletedPackage(ownerKey, pkgType, pkgName)
	if p == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	if !s.store.RestorePackage(ownerKey, pkgType, pkgName) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreUserPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	u := s.store.LookupUserByLogin(r.PathValue("username"))
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.restorePackageForOwner(w, r, user, u.Login)
}

func (s *Server) handleRestoreOrgPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.restorePackageForOwner(w, r, user, org.Login)
}

// ─── Docker registry migration conflicts ────────────────────────────────

// listDockerConflicts returns Docker-registry packages whose names
// collide with a GitHub Container registry package in the same owner
// namespace — the packages that cannot migrate off the legacy Docker
// registry. Empty when there are no conflicts.
func (s *Server) listDockerConflicts(r *http.Request, ownerKey string) []map[string]interface{} {
	pkgs := s.store.ListPackages(ownerKey)
	baseURL := s.baseURL(r)
	out := []map[string]interface{}{}
	for _, p := range pkgs {
		if p.PackageType != "docker" {
			continue
		}
		if s.store.GetPackage(ownerKey, "container", p.Name) != nil {
			out = append(out, s.packageToJSON(p, baseURL))
		}
	}
	return out
}

func (s *Server) handleListAuthUserDockerConflicts(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.listDockerConflicts(r, user.Login))
}

func (s *Server) handleListUserDockerConflicts(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	u := s.store.LookupUserByLogin(r.PathValue("username"))
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.listDockerConflicts(r, u.Login))
}

// ─── Org-scoped handlers ────────────────────────────────────────────────

func (s *Server) handleListOrgPackages(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	pkgType, visibility, ok := packageListFilters(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, s.listOwnerPackagesJSON(r, user, org.Login, pkgType, visibility)))
}

func (s *Server) handleGetOrgPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveOrgPackage(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.packageToJSON(p, s.baseURL(r)))
}

func (s *Server) handleDeleteOrgPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveOrgPackage(w, r)
	if !ok {
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	if !s.store.DeletePackage(p.OwnerKey, p.PackageType, p.Name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListOrgDockerConflicts implements GET /orgs/{org}/docker/conflicts:
// the org's docker-type packages whose names collide with a container-type
// package in the same namespace — the packages a Docker registry migration
// to the GitHub Container registry cannot move. Computed from the real
// package store; empty when nothing conflicts.
func (s *Server) handleListOrgDockerConflicts(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	pkgs := s.store.ListPackages(org.Login)
	containerNames := map[string]bool{}
	for _, p := range pkgs {
		if p.PackageType == "container" {
			containerNames[p.Name] = true
		}
	}
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0)
	for _, p := range pkgs {
		if p.PackageType != "docker" || !containerNames[p.Name] {
			continue
		}
		if !s.canViewPackage(user, p) {
			continue
		}
		row := s.packageToJSON(p, baseURL)
		// The vendored OpenAPI description's package schema does not
		// declare node_id, and this endpoint emits exactly the documented
		// members.
		delete(row, "node_id")
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListOrgPackageVersions(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveOrgPackage(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.listVersionsJSON(p, r))
}

func (s *Server) handleGetOrgPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveOrgPackageVersion(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.packageVersionToJSON(v, p, s.baseURL(r), packageScopePath(p.OwnerType, p.OwnerKey)))
}

func (s *Server) handleDeleteOrgPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, ok := s.resolveOrgPackageVersion(w, r)
	if !ok {
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("package_version_id"))
	if !s.store.DeletePackageVersion(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreOrgPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, ok := s.resolveOrgPackageVersion(w, r)
	if !ok {
		return
	}
	if !s.canAdminPackage(user, p) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("package_version_id"))
	if !s.store.RestorePackageVersion(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgPackageFiles(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveOrgPackageVersion(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	writeJSON(w, http.StatusOK, s.listFilesJSON(v, p, r))
}

func (s *Server) handleDownloadOrgPackageFile(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, f, ok := s.resolveOrgPackageFile(w, r)
	if !ok || !s.canViewPackage(user, p) {
		return
	}
	s.servePackageFile(w, r, f)
}

// ─── Repo-scoped handlers ───────────────────────────────────────────────

func (s *Server) handleListRepoPackages(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	repo, ok := s.resolveRepoForPackages(w, r)
	if !ok || !canReadRepo(s.store, user, repo) {
		return
	}
	pkgs := s.store.ListPackages(repo.FullName)
	out := make([]map[string]interface{}, 0, len(pkgs))
	baseURL := s.baseURL(r)
	for _, p := range pkgs {
		out = append(out, s.packageToJSON(p, baseURL))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetRepoPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveRepoPackage(w, r)
	if !ok {
		return
	}
	if !canReadRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.packageToJSON(p, s.baseURL(r)))
}

func (s *Server) handleDeleteRepoPackage(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveRepoPackage(w, r)
	if !ok {
		return
	}
	if !canAdminRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	if !s.store.DeletePackage(p.OwnerKey, p.PackageType, p.Name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRepoPackageVersions(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, ok := s.resolveRepoPackage(w, r)
	if !ok {
		return
	}
	if !canReadRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.listVersionsJSON(p, r))
}

func (s *Server) handleGetRepoPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveRepoPackageVersion(w, r)
	if !ok {
		return
	}
	if !canReadRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.packageVersionToJSON(v, p, s.baseURL(r), packageScopePath(p.OwnerType, p.OwnerKey)))
}

func (s *Server) handleDeleteRepoPackageVersion(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, ok := s.resolveRepoPackageVersion(w, r)
	if !ok {
		return
	}
	if !canAdminRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusForbidden, "Forbidden")
		return
	}
	id, _ := strconv.Atoi(r.PathValue("package_version_id"))
	if !s.store.DeletePackageVersion(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRepoPackageFiles(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, v, ok := s.resolveRepoPackageVersion(w, r)
	if !ok {
		return
	}
	if !canReadRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.listFilesJSON(v, p, r))
}

func (s *Server) handleDownloadRepoPackageFile(w http.ResponseWriter, r *http.Request) {
	user := s.requireUser(w, r)
	if user == nil {
		return
	}
	p, _, f, ok := s.resolveRepoPackageFile(w, r)
	if !ok {
		return
	}
	if !canReadRepo(s.store, user, s.store.ReposByName[p.OwnerKey]) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.servePackageFile(w, r, f)
}

// ─── Resolvers ──────────────────────────────────────────────────────────

func (s *Server) resolveUserPackage(w http.ResponseWriter, r *http.Request) (*Package, bool) {
	owner := r.PathValue("username")
	pkgType := r.PathValue("package_type")
	pkgName := r.PathValue("package_name")
	if !isPackageType(pkgType) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	u := s.store.LookupUserByLogin(owner)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	p := s.store.GetPackage(u.Login, pkgType, pkgName)
	if p == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return p, true
}

func (s *Server) resolveUserPackageVersion(w http.ResponseWriter, r *http.Request) (*Package, *PackageVersion, bool) {
	p, ok := s.resolveUserPackage(w, r)
	if !ok {
		return nil, nil, false
	}
	v, ok := s.resolveVersion(w, r)
	if !ok || v.PackageID != p.ID {
		return nil, nil, false
	}
	return p, v, true
}

func (s *Server) resolveUserPackageFile(w http.ResponseWriter, r *http.Request) (*Package, *PackageVersion, *PackageFile, bool) {
	p, v, ok := s.resolveUserPackageVersion(w, r)
	if !ok {
		return nil, nil, nil, false
	}
	f, ok := s.resolveFile(w, r)
	if !ok || f.VersionID != v.ID {
		return nil, nil, nil, false
	}
	return p, v, f, true
}

func (s *Server) resolveOrgPackage(w http.ResponseWriter, r *http.Request) (*Package, bool) {
	orgLogin := r.PathValue("org")
	pkgType := r.PathValue("package_type")
	pkgName := r.PathValue("package_name")
	if !isPackageType(pkgType) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	p := s.store.GetPackage(org.Login, pkgType, pkgName)
	if p == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return p, true
}

func (s *Server) resolveOrgPackageVersion(w http.ResponseWriter, r *http.Request) (*Package, *PackageVersion, bool) {
	p, ok := s.resolveOrgPackage(w, r)
	if !ok {
		return nil, nil, false
	}
	v, ok := s.resolveVersion(w, r)
	if !ok || v.PackageID != p.ID {
		return nil, nil, false
	}
	return p, v, true
}

func (s *Server) resolveOrgPackageFile(w http.ResponseWriter, r *http.Request) (*Package, *PackageVersion, *PackageFile, bool) {
	p, v, ok := s.resolveOrgPackageVersion(w, r)
	if !ok {
		return nil, nil, nil, false
	}
	f, ok := s.resolveFile(w, r)
	if !ok || f.VersionID != v.ID {
		return nil, nil, nil, false
	}
	return p, v, f, true
}

func (s *Server) resolveRepoForPackages(w http.ResponseWriter, r *http.Request) (*Repo, bool) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return repo, true
}

func (s *Server) resolveRepoPackage(w http.ResponseWriter, r *http.Request) (*Package, bool) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	pkgType := r.PathValue("package_type")
	pkgName := r.PathValue("package_name")
	if !isPackageType(pkgType) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	repoKey := owner + "/" + name
	if s.store.ReposByName[repoKey] == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	p := s.store.GetPackage(repoKey, pkgType, pkgName)
	if p == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return p, true
}

func (s *Server) resolveRepoPackageVersion(w http.ResponseWriter, r *http.Request) (*Package, *PackageVersion, bool) {
	p, ok := s.resolveRepoPackage(w, r)
	if !ok {
		return nil, nil, false
	}
	v, ok := s.resolveVersion(w, r)
	if !ok || v.PackageID != p.ID {
		return nil, nil, false
	}
	return p, v, true
}

func (s *Server) resolveRepoPackageFile(w http.ResponseWriter, r *http.Request) (*Package, *PackageVersion, *PackageFile, bool) {
	p, v, ok := s.resolveRepoPackageVersion(w, r)
	if !ok {
		return nil, nil, nil, false
	}
	f, ok := s.resolveFile(w, r)
	if !ok || f.VersionID != v.ID {
		return nil, nil, nil, false
	}
	return p, v, f, true
}

func (s *Server) resolveVersion(w http.ResponseWriter, r *http.Request) (*PackageVersion, bool) {
	id, err := strconv.Atoi(r.PathValue("package_version_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	v := s.store.GetPackageVersion(id)
	if v == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return v, true
}

func (s *Server) resolveFile(w http.ResponseWriter, r *http.Request) (*PackageFile, bool) {
	id, err := strconv.Atoi(r.PathValue("file_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	f := s.store.GetPackageFile(id)
	if f == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return f, true
}

// ─── Authorization helpers ──────────────────────────────────────────────

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *User {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return nil
	}
	return user
}

func (s *Server) canViewPackage(user *User, p *Package) bool {
	if p.Visibility == "public" {
		return true
	}
	return s.canAdminPackage(user, p)
}

func (s *Server) canAdminPackage(user *User, p *Package) bool {
	if user == nil {
		return false
	}
	switch p.OwnerType {
	case "User":
		return p.OwnerKey == user.Login
	case "Organization":
		if org := s.store.GetOrg(p.OwnerKey); org != nil {
			return canAdminOrg(s.store, user, org)
		}
		return false
	case "Repository":
		if repo := s.store.ReposByName[p.OwnerKey]; repo != nil {
			return canAdminRepo(s.store, user, repo)
		}
		return false
	}
	return false
}

// ─── JSON serialization ─────────────────────────────────────────────────

func (s *Server) packageToJSON(p *Package, baseURL string) map[string]interface{} {
	out := map[string]interface{}{
		"id":            p.ID,
		"node_id":       p.NodeID,
		"name":          p.Name,
		"package_type":  p.PackageType,
		"visibility":    p.Visibility,
		"url":           s.packageURL(baseURL, packageScopePath(p.OwnerType, p.OwnerKey), p),
		"html_url":      s.packageHTMLURL(baseURL, p),
		"version_count": p.VersionCount,
		"created_at":    p.CreatedAt.Format(time.RFC3339),
		"updated_at":    p.UpdatedAt.Format(time.RFC3339),
	}
	switch p.OwnerType {
	case "User":
		if u := s.store.LookupUserByLogin(p.OwnerKey); u != nil {
			out["owner"] = userToJSON(u)
		} else {
			out["owner"] = nil
		}
		out["repository"] = nil
	case "Organization":
		if org := s.store.GetOrg(p.OwnerKey); org != nil {
			out["owner"] = orgAsSimpleUserJSON(org)
		} else {
			out["owner"] = nil
		}
		out["repository"] = nil
	case "Repository":
		out["owner"] = nil
		if repo := s.store.ReposByName[p.OwnerKey]; repo != nil {
			out["repository"] = minimalRepoJSON(repo, s.store, baseURL)
		} else {
			out["repository"] = nil
		}
	}
	return out
}

func (s *Server) packageVersionToJSON(v *PackageVersion, p *Package, baseURL, scopePath string) map[string]interface{} {
	out := map[string]interface{}{
		"id":               v.ID,
		"node_id":          v.NodeID,
		"name":             v.Version,
		"url":              s.packageVersionURL(baseURL, scopePath, p, v),
		"package_html_url": s.packageHTMLURL(baseURL, p),
		"html_url":         s.packageVersionHTMLURL(baseURL, p, v),
		"license":          nil,
		"description":      v.Description,
		"created_at":       v.CreatedAt.Format(time.RFC3339),
		"updated_at":       v.UpdatedAt.Format(time.RFC3339),
		"metadata":         v.Metadata,
	}
	if v.Deleted && v.DeletedAt != nil {
		out["deleted_at"] = v.DeletedAt.Format(time.RFC3339)
	}
	return out
}

func (s *Server) packageFileToJSON(f *PackageFile, p *Package, v *PackageVersion, baseURL, scopePath string) map[string]interface{} {
	return map[string]interface{}{
		"id":           f.ID,
		"node_id":      f.NodeID,
		"name":         f.Name,
		"content_type": f.ContentType,
		"size":         f.Size,
		"url":          baseURL + "/api/v3" + scopePath + "/" + url.PathEscape(p.PackageType) + "/" + url.PathEscape(p.Name) + "/versions/" + strconv.Itoa(v.ID) + "/files/" + strconv.Itoa(f.ID),
		"html_url":     baseURL + "/" + p.OwnerKey + "/packages/" + p.PackageType + "/" + p.Name + "/files/" + strconv.Itoa(f.ID),
		"download_url": baseURL + "/api/v3" + scopePath + "/" + url.PathEscape(p.PackageType) + "/" + url.PathEscape(p.Name) + "/versions/" + strconv.Itoa(v.ID) + "/files/" + strconv.Itoa(f.ID),
	}
}

func (s *Server) packageHTMLURL(baseURL string, p *Package) string {
	var path string
	switch p.OwnerType {
	case "User":
		path = "/users/" + p.OwnerKey + "/packages/" + p.PackageType + "/" + p.Name
	case "Organization":
		path = "/orgs/" + p.OwnerKey + "/packages/" + p.PackageType + "/" + p.Name
	case "Repository":
		path = "/repos/" + p.OwnerKey + "/packages/" + p.PackageType + "/" + p.Name
	}
	return baseURL + path
}

func (s *Server) packageVersionHTMLURL(baseURL string, p *Package, v *PackageVersion) string {
	return s.packageHTMLURL(baseURL, p) + "/" + strconv.Itoa(v.ID)
}

func (s *Server) listVersionsJSON(p *Package, r *http.Request) []map[string]interface{} {
	baseURL := s.baseURL(r)
	scopePath := packageScopePath(p.OwnerType, p.OwnerKey)
	versions := s.store.ListPackageVersions(p.ID, false)
	out := make([]map[string]interface{}, len(versions))
	for i, v := range versions {
		out[i] = s.packageVersionToJSON(v, p, baseURL, scopePath)
	}
	return out
}

func (s *Server) listFilesJSON(v *PackageVersion, p *Package, r *http.Request) []map[string]interface{} {
	baseURL := s.baseURL(r)
	scopePath := packageScopePath(p.OwnerType, p.OwnerKey)
	files := s.store.ListPackageFiles(v.ID)
	out := make([]map[string]interface{}, len(files))
	for i, f := range files {
		out[i] = s.packageFileToJSON(f, p, v, baseURL, scopePath)
	}
	return out
}

// minimalRepoJSON returns the subset of repository fields that matches
// GitHub's minimal-repository schema, used for the package.repository block.
func minimalRepoJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	full := repoToJSON(repo, st, baseURL)
	allowed := map[string]bool{
		"id": true, "node_id": true, "name": true, "full_name": true, "owner": true,
		"private": true, "html_url": true, "description": true, "fork": true, "url": true,
		"archive_url": true, "assignees_url": true, "blobs_url": true, "branches_url": true,
		"collaborators_url": true, "comments_url": true, "commits_url": true, "compare_url": true,
		"contents_url": true, "contributors_url": true, "deployments_url": true, "downloads_url": true,
		"events_url": true, "forks_url": true, "git_commits_url": true, "git_refs_url": true,
		"git_tags_url": true, "hooks_url": true, "issue_comment_url": true, "issue_events_url": true,
		"issues_url": true, "keys_url": true, "labels_url": true, "languages_url": true,
		"merges_url": true, "milestones_url": true, "notifications_url": true, "pulls_url": true,
		"releases_url": true, "stargazers_url": true, "statuses_url": true, "subscribers_url": true,
		"subscription_url": true, "tags_url": true, "teams_url": true, "trees_url": true,
	}
	out := map[string]interface{}{}
	for k, v := range full {
		if allowed[k] {
			out[k] = v
		}
	}
	return out
}

// ─── File serving ───────────────────────────────────────────────────────

func (s *Server) servePackageFile(w http.ResponseWriter, r *http.Request, f *PackageFile) {
	var (
		data []byte
		err  error
	)
	if s.store.ObjectByteStore != nil {
		data, err = s.store.ObjectByteStore.Get(r.Context(), f.StoragePath)
	} else {
		data, err = os.ReadFile(f.StoragePath)
	}
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+f.Name+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(data))
}
