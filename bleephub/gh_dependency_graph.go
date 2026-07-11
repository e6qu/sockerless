package bleephub

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Dependency graph: dependency submission (snapshots), the SPDX SBOM export,
// and the dependency diff between two commits.
// Endpoints:
//
//	POST /repos/{o}/{r}/dependency-graph/snapshots
//	GET  /repos/{o}/{r}/dependency-graph/sbom
//	GET  /repos/{o}/{r}/dependency-graph/sbom/generate-report
//	GET  /repos/{o}/{r}/dependency-graph/sbom/fetch-report/{sbom_uuid}
//	GET  /repos/{o}/{r}/dependency-graph/compare/{basehead}
//
// Snapshots are the source of truth: the SBOM and the compare diff are both
// computed from the submitted snapshot data, never invented. A repo with no
// snapshots gets an SBOM describing just the repository itself.

// DependencySnapshot is a submitted dependency snapshot. The field names
// mirror the dependency submission API's snapshot object.
type DependencySnapshot struct {
	ID        int                          `json:"id"`
	RepoID    int                          `json:"repo_id"`
	Version   int                          `json:"version"`
	Ref       string                       `json:"ref"`
	Sha       string                       `json:"sha"`
	Job       SnapshotJob                  `json:"job"`
	Detector  SnapshotDetector             `json:"detector"`
	Scanned   string                       `json:"scanned"`
	Manifests map[string]*SnapshotManifest `json:"manifests,omitempty"`
	// Result records the submission outcome (SUCCESS, ACCEPTED, or INVALID).
	// A malformed (INVALID) snapshot is stored for the submission response
	// but never contributes to the repository's dependency set.
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

type SnapshotJob struct {
	ID         string `json:"id"`
	Correlator string `json:"correlator"`
	HTMLURL    string `json:"html_url,omitempty"`
}

type SnapshotDetector struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

type SnapshotManifest struct {
	Name string `json:"name"`
	File *struct {
		SourceLocation string `json:"source_location"`
	} `json:"file,omitempty"`
	Resolved map[string]*SnapshotDependency `json:"resolved,omitempty"`
}

type SnapshotDependency struct {
	PackageURL   string   `json:"package_url"`
	Relationship string   `json:"relationship,omitempty"`
	Scope        string   `json:"scope,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// SBOMExport is a generated SBOM report export addressed by UUID.
type SBOMExport struct {
	UUID      string    `json:"uuid"`
	RepoID    int       `json:"repo_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) registerGHDependencyGraphRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/dependency-graph/snapshots",
		s.requirePerm(scopeContents, permWrite, s.handleCreateDependencySnapshot))
	s.route("GET /api/v3/repos/{owner}/{repo}/dependency-graph/sbom", s.handleGetDependencySBOM)
	s.route("GET /api/v3/repos/{owner}/{repo}/dependency-graph/sbom/generate-report", s.handleGenerateSBOMReport)
	s.route("GET /api/v3/repos/{owner}/{repo}/dependency-graph/sbom/fetch-report/{sbom_uuid}", s.handleFetchSBOMReport)
	s.route("GET /api/v3/repos/{owner}/{repo}/dependency-graph/compare/{basehead}", s.handleDependencyGraphCompare)
}

// --- Store ---

// AddDependencySnapshot appends a snapshot for the repository.
func (st *Store) AddDependencySnapshot(snap *DependencySnapshot) *DependencySnapshot {
	st.mu.Lock()
	defer st.mu.Unlock()
	snap.ID = st.NextDependencySnapshotID
	st.NextDependencySnapshotID++
	snap.CreatedAt = time.Now().UTC()
	st.DependencySnapshots[snap.RepoID] = append(st.DependencySnapshots[snap.RepoID], snap)
	if st.persist != nil {
		st.persist.MustPut("dependency_snapshots", strconv.Itoa(snap.RepoID), st.DependencySnapshots[snap.RepoID])
	}
	return snap
}

// ListDependencySnapshots returns the repo's snapshots, oldest first.
func (st *Store) ListDependencySnapshots(repoID int) []*DependencySnapshot {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*DependencySnapshot, len(st.DependencySnapshots[repoID]))
	copy(out, st.DependencySnapshots[repoID])
	return out
}

// AddSBOMExport records a generated SBOM export.
func (st *Store) AddSBOMExport(repoID int) *SBOMExport {
	st.mu.Lock()
	defer st.mu.Unlock()
	exp := &SBOMExport{
		UUID:      uuid.New().String(),
		RepoID:    repoID,
		CreatedAt: time.Now().UTC(),
	}
	st.SBOMExports[exp.UUID] = exp
	if st.persist != nil {
		st.persist.MustPut("sbom_exports", exp.UUID, exp)
	}
	return exp
}

// GetSBOMExport returns an export by UUID, or nil.
func (st *Store) GetSBOMExport(uuid string) *SBOMExport {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.SBOMExports[uuid]
}

// --- Handlers ---

func (s *Server) handleCreateDependencySnapshot(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var snap DependencySnapshot
	if !decodeJSONBody(w, r, &snap) {
		return
	}
	snap.RepoID = repo.ID

	created := func(result, message string) {
		snap.Result = result
		stored := s.store.AddDependencySnapshot(&snap)
		if result == "SUCCESS" {
			s.deriveDependabotAlertsForRepository(repo)
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"id":         stored.ID,
			"created_at": stored.CreatedAt.Format(time.RFC3339),
			"result":     result,
			"message":    message,
		})
	}

	if msg := validateDependencySnapshot(&snap); msg != "" {
		created("INVALID", msg)
		return
	}
	if snap.Ref == "refs/heads/"+repo.DefaultBranch {
		created("SUCCESS", "Dependency results for the repo have been successfully updated.")
		return
	}
	created("ACCEPTED", "Snapshot accepted, but the repo's dependencies were not updated because the ref is not the default branch.")
}

// validateDependencySnapshot checks the snapshot's required members and
// returns a message describing the first problem, or "" when well-formed.
func validateDependencySnapshot(snap *DependencySnapshot) string {
	switch {
	case snap.Version == 0 && snap.Detector.Name == "" && snap.Job.ID == "":
		return "snapshot is empty"
	case snap.Detector.Name == "" || snap.Detector.Version == "" || snap.Detector.URL == "":
		return "detector name, version, and url are required"
	case snap.Job.ID == "" || snap.Job.Correlator == "":
		return "job id and correlator are required"
	case snap.Ref == "" || !strings.HasPrefix(snap.Ref, "refs/"):
		return "ref must be a fully qualified git ref (refs/...)"
	case len(snap.Sha) != 40:
		return "sha must be a 40-character commit SHA"
	case snap.Scanned == "":
		return "scanned timestamp is required"
	}
	return ""
}

// currentDependencies returns the dependency set recorded for a ref+sha:
// per (job.correlator, detector.name) only the latest matching snapshot
// counts — the dependency submission API's replacement semantics. Matching
// is by exact sha when given, else by ref.
func (s *Server) currentDependencies(repoID int, ref, sha string) map[string]*dependencyEntry {
	latest := map[string]*DependencySnapshot{}
	for _, snap := range s.store.ListDependencySnapshots(repoID) {
		if snap.Result == "INVALID" {
			continue
		}
		if sha != "" && snap.Sha != sha {
			continue
		}
		if sha == "" && snap.Ref != ref {
			continue
		}
		key := snap.Job.Correlator + "\x1f" + snap.Detector.Name
		if cur, ok := latest[key]; !ok || snap.ID > cur.ID {
			latest[key] = snap
		}
	}
	deps := map[string]*dependencyEntry{}
	for _, snap := range latest {
		for _, manifest := range snap.Manifests {
			for _, dep := range manifest.Resolved {
				if dep.PackageURL == "" {
					continue
				}
				deps[dep.PackageURL] = &dependencyEntry{
					PackageURL: dep.PackageURL,
					Manifest:   manifest.Name,
					Scope:      dep.Scope,
				}
			}
		}
	}
	return deps
}

type dependencyEntry struct {
	PackageURL string
	Manifest   string
	Scope      string
}

// parsePurl splits a package-url into ecosystem, name, and version.
func parsePurl(purl string) (ecosystem, name, version string) {
	rest := strings.TrimPrefix(purl, "pkg:")
	rest = strings.TrimPrefix(rest, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		ecosystem = rest[:i]
		rest = rest[i+1:]
	} else {
		ecosystem = rest
		rest = ""
	}
	if i := strings.LastIndexByte(rest, '@'); i >= 0 {
		version = rest[i+1:]
		rest = rest[:i]
	}
	if decoded, err := url.PathUnescape(rest); err == nil {
		rest = decoded
	}
	name = rest
	return ecosystem, name, version
}

func (s *Server) handleGetDependencySBOM(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sbom": s.buildSPDXSBOM(repo, s.baseURL(r))})
}

// buildSPDXSBOM produces a real SPDX 2.3 document from the repository's
// recorded default-branch dependencies. With no snapshots the document
// honestly describes only the repository package itself.
func (s *Server) buildSPDXSBOM(repo *Repo, baseURL string) map[string]interface{} {
	docName := "com.github." + repo.FullName
	repoSPDXID := "SPDXRef-com.github." + strings.ReplaceAll(repo.FullName, "/", "-")

	owner, name, _ := splitRepoFullName(repo.FullName)
	headSha := resolveBranchSha(s.store.GetGitStorage(owner, name), repo.DefaultBranch)
	versionInfo := repo.DefaultBranch
	if headSha != "" {
		versionInfo = headSha
	}

	packages := []map[string]interface{}{{
		"SPDXID":           repoSPDXID,
		"name":             docName,
		"versionInfo":      versionInfo,
		"downloadLocation": "git+" + baseURL + "/" + repo.FullName,
		"filesAnalyzed":    false,
	}}
	relationships := []map[string]interface{}{{
		"relationshipType":   "DESCRIBES",
		"spdxElementId":      "SPDXRef-DOCUMENT",
		"relatedSpdxElement": repoSPDXID,
	}}

	deps := s.currentDependencies(repo.ID, "refs/heads/"+repo.DefaultBranch, "")
	purls := make([]string, 0, len(deps))
	for purl := range deps {
		purls = append(purls, purl)
	}
	sort.Strings(purls)
	for _, purl := range purls {
		ecosystem, depName, version := parsePurl(purl)
		spdxID := "SPDXRef-" + sanitizeSPDXIDPart(ecosystem+"-"+depName+"-"+version)
		packages = append(packages, map[string]interface{}{
			"SPDXID":           spdxID,
			"name":             ecosystem + ":" + depName,
			"versionInfo":      version,
			"downloadLocation": "NOASSERTION",
			"filesAnalyzed":    false,
			"externalRefs": []map[string]interface{}{{
				"referenceCategory": "PACKAGE-MANAGER",
				"referenceLocator":  purl,
				"referenceType":     "purl",
			}},
		})
		relationships = append(relationships, map[string]interface{}{
			"relationshipType":   "DEPENDS_ON",
			"spdxElementId":      repoSPDXID,
			"relatedSpdxElement": spdxID,
		})
	}

	return map[string]interface{}{
		"SPDXID":      "SPDXRef-DOCUMENT",
		"spdxVersion": "SPDX-2.3",
		"creationInfo": map[string]interface{}{
			"created":  time.Now().UTC().Format(time.RFC3339),
			"creators": []string{"Tool: bleephub-dependency-graph"},
		},
		"name":              docName,
		"dataLicense":       "CC0-1.0",
		"documentNamespace": baseURL + "/" + repo.FullName + "/dependency_graph/sbom",
		"packages":          packages,
		"relationships":     relationships,
	}
}

// sanitizeSPDXIDPart keeps only the characters SPDX allows in an SPDXID
// idstring (letters, digits, '.', '-').
func sanitizeSPDXIDPart(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-':
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

func (s *Server) handleGenerateSBOMReport(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	exp := s.store.AddSBOMExport(repo.ID)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"sbom_url": s.baseURL(r) + "/api/v3/repos/" + repo.FullName + "/dependency-graph/sbom/fetch-report/" + exp.UUID,
	})
}

func (s *Server) handleFetchSBOMReport(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	exp := s.store.GetSBOMExport(r.PathValue("sbom_uuid"))
	if exp == nil || exp.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// The export is served by the SBOM endpoint; the fetch step is a
	// redirect to the download location, as on real GitHub.
	http.Redirect(w, r, s.baseURL(r)+"/api/v3/repos/"+repo.FullName+"/dependency-graph/sbom", http.StatusFound)
}

func (s *Server) handleDependencyGraphCompare(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	basehead := r.PathValue("basehead")
	base, head, found := strings.Cut(basehead, "...")
	if !found || base == "" || head == "" {
		writeGHError(w, http.StatusBadRequest, "basehead must be in the form BASE...HEAD")
		return
	}

	baseDeps, baseOK := s.dependenciesForRevision(repo, base)
	headDeps, headOK := s.dependenciesForRevision(repo, head)
	if !baseOK || !headOK {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	diff := []map[string]interface{}{}
	appendChange := func(changeType string, dep *dependencyEntry) {
		ecosystem, depName, version := parsePurl(dep.PackageURL)
		scope := dep.Scope
		if scope == "" {
			scope = "unknown"
		}
		diff = append(diff, map[string]interface{}{
			"change_type":           changeType,
			"manifest":              dep.Manifest,
			"ecosystem":             ecosystem,
			"name":                  depName,
			"version":               version,
			"package_url":           dep.PackageURL,
			"license":               nil,
			"source_repository_url": nil,
			"vulnerabilities":       []map[string]interface{}{},
			"scope":                 scope,
		})
	}
	var addedPurls, removedPurls []string
	for purl := range headDeps {
		if _, ok := baseDeps[purl]; !ok {
			addedPurls = append(addedPurls, purl)
		}
	}
	for purl := range baseDeps {
		if _, ok := headDeps[purl]; !ok {
			removedPurls = append(removedPurls, purl)
		}
	}
	sort.Strings(addedPurls)
	sort.Strings(removedPurls)
	for _, purl := range addedPurls {
		appendChange("added", headDeps[purl])
	}
	for _, purl := range removedPurls {
		appendChange("removed", baseDeps[purl])
	}
	writeJSON(w, http.StatusOK, diff)
}

// dependenciesForRevision resolves one side of a basehead expression — a
// commit SHA, a branch name, or a fully qualified ref — to its recorded
// dependency set. ok is false when the revision matches neither the git
// storage nor any snapshot.
func (s *Server) dependenciesForRevision(repo *Repo, rev string) (map[string]*dependencyEntry, bool) {
	owner, name, _ := splitRepoFullName(repo.FullName)
	gitStor := s.store.GetGitStorage(owner, name)

	branch := strings.TrimPrefix(rev, "refs/heads/")
	var sha string
	if len(rev) == 40 && !strings.Contains(rev, "/") {
		sha = rev
	} else {
		sha = resolveBranchSha(gitStor, branch)
	}

	if sha != "" {
		if deps := s.currentDependencies(repo.ID, "", sha); len(deps) > 0 {
			return deps, true
		}
	}
	if deps := s.currentDependencies(repo.ID, "refs/heads/"+branch, ""); len(deps) > 0 {
		return deps, true
	}
	// A revision that resolves in git but has no snapshots legitimately has
	// an empty dependency set; an unresolvable revision is a 404.
	if sha != "" {
		return map[string]*dependencyEntry{}, true
	}
	return nil, false
}
