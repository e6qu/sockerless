package bleephub

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SecurityAdvisory is a repository-scoped security advisory.
type SecurityAdvisory struct {
	ID                     int                             `json:"id"`
	NodeID                 string                          `json:"node_id"`
	GHSAID                 string                          `json:"ghsa_id"`
	RepoID                 int                             `json:"repo_id"`
	AuthorID               int                             `json:"author_id"`
	Title                  string                          `json:"title,omitempty"`
	Summary                string                          `json:"summary"`
	Description            string                          `json:"description"`
	Severity               string                          `json:"severity"`
	CVSSScore              float64                         `json:"cvss_score"`
	CVSSVector             string                          `json:"cvss_vector"`
	CWEs                   []string                        `json:"cwes"`
	State                  string                          `json:"state"`
	CreatedAt              time.Time                       `json:"created_at"`
	UpdatedAt              time.Time                       `json:"updated_at"`
	PublishedAt            *time.Time                      `json:"published_at,omitempty"`
	CVEID                  string                          `json:"cve_id"`
	HTMLURL                string                          `json:"html_url"`
	URL                    string                          `json:"url"`
	SubmissionAccepted     bool                            `json:"submission_accepted"`
	PrivateForkID          int                             `json:"private_fork_id"`
	VulnerableVersionRange string                          `json:"vulnerable_version_range"`
	Vulnerabilities        []SecurityAdvisoryVulnerability `json:"vulnerabilities,omitempty"`
}

type SecurityAdvisoryVulnerability struct {
	PackageName            string `json:"package_name"`
	PackageEcosystem       string `json:"package_ecosystem"`
	VulnerableVersionRange string `json:"vulnerable_version_range"`
	FirstPatchedVersion    string `json:"first_patched_version,omitempty"`
}

// SecurityAdvisoryReport records a vulnerability report that spawned an advisory.
type SecurityAdvisoryReport struct {
	ID                     int       `json:"id"`
	AdvisoryID             int       `json:"advisory_id"`
	ReporterID             int       `json:"reporter_id"`
	Summary                string    `json:"summary"`
	Description            string    `json:"description"`
	Severity               string    `json:"severity"`
	CVSSScore              float64   `json:"cvss_score"`
	CVSSVector             string    `json:"cvss_vector"`
	CWEs                   []string  `json:"cwes"`
	VulnerableVersionRange string    `json:"vulnerable_version_range"`
	CreatedAt              time.Time `json:"created_at"`
}

// CreateAdvisoryReq is the request body for creating a security advisory.
type CreateAdvisoryReq struct {
	Summary                string   `json:"summary"`
	Description            string   `json:"description"`
	Severity               string   `json:"severity"`
	CVSSScore              float64  `json:"cvss_score"`
	CVSSVector             string   `json:"cvss_vector"`
	CWEs                   []string `json:"cwe_ids"`
	State                  string   `json:"state"`
	VulnerableVersionRange string   `json:"vulnerable_version_range"`
	Vulnerabilities        []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		VulnerableVersionRange string `json:"vulnerable_version_range"`
		FirstPatchedVersion    string `json:"first_patched_version"`
		PatchedVersions        string `json:"patched_versions"`
	} `json:"vulnerabilities"`
}

func validAdvisorySeverity(s string) bool {
	switch s {
	case "critical", "high", "medium", "low":
		return true
	}
	return false
}

func validAdvisoryState(s string) bool {
	switch s {
	case "draft", "triage", "published", "closed", "withdrawn":
		return true
	}
	return false
}

func generateGHSAID() (string, error) {
	h, err := randomHex(6)
	if err != nil {
		return "", fmt.Errorf("generate GitHub Security Advisory id: %w", err)
	}
	return fmt.Sprintf("GHSA-%s-%s-%s", h[0:4], h[4:8], h[8:12]), nil
}

func generateCVEID() (string, error) {
	b, err := randomBytes(4)
	if err != nil {
		return "", fmt.Errorf("generate CVE id: %w", err)
	}
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 {
		n = -n
	}
	return fmt.Sprintf("CVE-%d-%04d", time.Now().UTC().Year(), n%10000), nil
}

// CreateSecurityAdvisory creates a new security advisory in the given repo.
func (st *Store) CreateSecurityAdvisory(repoID, authorID int, req CreateAdvisoryReq) *SecurityAdvisory {
	adv, err := st.CreateSecurityAdvisoryE(repoID, authorID, req)
	if err != nil {
		panic(err)
	}
	return adv
}

func (st *Store) CreateSecurityAdvisoryE(repoID, authorID int, req CreateAdvisoryReq) (*SecurityAdvisory, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil, nil
	}
	if req.Severity == "" {
		req.Severity = "medium"
	}
	if !validAdvisorySeverity(req.Severity) {
		return nil, nil
	}
	state := req.State
	if state == "" {
		state = "draft"
	}
	if !validAdvisoryState(state) {
		return nil, nil
	}
	ghsaID, err := generateGHSAID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	adv := &SecurityAdvisory{
		ID:                     st.NextSecurityAdvisoryID,
		NodeID:                 fmt.Sprintf("GSA_kwCN%07d", st.NextSecurityAdvisoryID),
		GHSAID:                 ghsaID,
		RepoID:                 repoID,
		AuthorID:               authorID,
		Summary:                req.Summary,
		Description:            req.Description,
		Severity:               req.Severity,
		CVSSScore:              req.CVSSScore,
		CVSSVector:             req.CVSSVector,
		CWEs:                   req.CWEs,
		State:                  state,
		CreatedAt:              now,
		UpdatedAt:              now,
		VulnerableVersionRange: req.VulnerableVersionRange,
	}
	if state == "published" {
		adv.PublishedAt = &now
	}
	for _, v := range req.Vulnerabilities {
		if v.Package.Name == "" || v.Package.Ecosystem == "" || v.VulnerableVersionRange == "" {
			continue
		}
		patched := v.FirstPatchedVersion
		if patched == "" {
			patched = v.PatchedVersions
		}
		adv.Vulnerabilities = append(adv.Vulnerabilities, SecurityAdvisoryVulnerability{
			PackageName:            v.Package.Name,
			PackageEcosystem:       v.Package.Ecosystem,
			VulnerableVersionRange: v.VulnerableVersionRange,
			FirstPatchedVersion:    patched,
		})
	}
	if adv.CWEs == nil {
		adv.CWEs = []string{}
	}
	st.NextSecurityAdvisoryID++

	if st.SecurityAdvisoriesByRepo[repo.FullName] == nil {
		st.SecurityAdvisoriesByRepo[repo.FullName] = map[string]*SecurityAdvisory{}
	}
	st.SecurityAdvisoriesByRepo[repo.FullName][adv.GHSAID] = adv
	st.SecurityAdvisories[adv.ID] = adv

	st.persistSecurityAdvisory(adv)
	return adv, nil
}

// ListSecurityAdvisories returns all security advisories for a repo, newest first.
func (st *Store) ListSecurityAdvisories(repoID int) []*SecurityAdvisory {
	st.mu.RLock()
	defer st.mu.RUnlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}
	out := make([]*SecurityAdvisory, 0, len(st.SecurityAdvisoriesByRepo[repo.FullName]))
	for _, a := range st.SecurityAdvisoriesByRepo[repo.FullName] {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// GetSecurityAdvisoryByGHSA returns an advisory by repo and GHSA ID.
func (st *Store) GetSecurityAdvisoryByGHSA(repoID int, ghsaID string) *SecurityAdvisory {
	st.mu.RLock()
	defer st.mu.RUnlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}
	return st.SecurityAdvisoriesByRepo[repo.FullName][ghsaID]
}

// UpdateSecurityAdvisory applies fn to the advisory and persists it.
func (st *Store) UpdateSecurityAdvisory(id int, fn func(*SecurityAdvisory)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	adv := st.SecurityAdvisories[id]
	if adv == nil {
		return false
	}
	fn(adv)
	if adv.CWEs == nil {
		adv.CWEs = []string{}
	}
	adv.UpdatedAt = time.Now().UTC()
	st.persistSecurityAdvisory(adv)
	return true
}

// RequestCVE assigns a CVE ID to the advisory.
func (st *Store) RequestCVE(id int) bool {
	ok, err := st.RequestCVEE(id)
	if err != nil {
		panic(err)
	}
	return ok
}

func (st *Store) RequestCVEE(id int) (bool, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	adv := st.SecurityAdvisories[id]
	if adv == nil || adv.CVEID != "" {
		return false, nil
	}
	cveID, err := generateCVEID()
	if err != nil {
		return false, err
	}
	adv.CVEID = cveID
	adv.UpdatedAt = time.Now().UTC()
	st.persistSecurityAdvisory(adv)
	return true, nil
}

// CreateTemporaryFork creates a private fork of the advisory's repo for collaboration.
func (st *Store) CreateTemporaryFork(repoID int, ghsaID string) *Repo {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}
	byRepo := st.SecurityAdvisoriesByRepo[repo.FullName]
	if byRepo == nil {
		return nil
	}
	adv := byRepo[ghsaID]
	if adv == nil {
		return nil
	}
	author := st.Users[adv.AuthorID]
	if author == nil {
		return nil
	}

	owner := author
	baseName := fmt.Sprintf("%s-%s-%s", owner.Login, repo.Name, strings.ToLower(ghsaID))
	name := baseName
	fullName := owner.Login + "/" + name
	for i := 2; ; i++ {
		if _, exists := st.ReposByName[fullName]; !exists {
			break
		}
		name = fmt.Sprintf("%s-%d", baseName, i)
		fullName = owner.Login + "/" + name
	}

	sourceID := repo.ID
	if repo.SourceID != 0 {
		sourceID = repo.SourceID
	}

	fork := &Repo{
		ID:                        st.NextRepo,
		NodeID:                    fmt.Sprintf("R_kgDO%08d", st.NextRepo),
		Name:                      name,
		FullName:                  fullName,
		Description:               repo.Description,
		Homepage:                  repo.Homepage,
		DefaultBranch:             repo.DefaultBranch,
		Visibility:                "private",
		Language:                  repo.Language,
		Owner:                     owner,
		OwnerID:                   owner.ID,
		OwnerType:                 "User",
		Private:                   true,
		Fork:                      true,
		ParentID:                  repo.ID,
		SourceID:                  sourceID,
		HasIssues:                 repo.HasIssues,
		HasProjects:               repo.HasProjects,
		HasWiki:                   repo.HasWiki,
		HasDiscussions:            boolPointer(repoHasDiscussions(repo)),
		HasPullRequests:           repo.HasPullRequests,
		AllowSquashMerge:          repo.AllowSquashMerge,
		AllowMergeCommit:          repo.AllowMergeCommit,
		AllowRebaseMerge:          repo.AllowRebaseMerge,
		AllowAutoMerge:            repo.AllowAutoMerge,
		AllowUpdateBranch:         repo.AllowUpdateBranch,
		DeleteBranchOnMerge:       repo.DeleteBranchOnMerge,
		UseSquashPRTitleAsDefault: repo.UseSquashPRTitleAsDefault,
		SquashMergeCommitTitle:    repo.SquashMergeCommitTitle,
		SquashMergeCommitMessage:  repo.SquashMergeCommitMessage,
		MergeCommitTitle:          repo.MergeCommitTitle,
		MergeCommitMessage:        repo.MergeCommitMessage,
		PullRequestCreationPolicy: repo.PullRequestCreationPolicy,
		LicenseKey:                repo.LicenseKey,
		LicenseName:               repo.LicenseName,
		LicenseSPDX:               repo.LicenseSPDX,
		Topics:                    append([]string(nil), repo.Topics...),
		Stargazers:                map[int]bool{},
		NextIssueNumber:           1,
		NextMilestoneNumber:       1,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
		PushedAt:                  time.Now().UTC(),
	}
	st.NextRepo++

	srcStor := st.GitStorages[repo.FullName]
	if srcStor == nil {
		return nil
	}
	stor, err := openOrInitGitStorage(context.Background(), fullName)
	if err != nil {
		log.Printf("bleephub: security advisory fork %s: open git storage: %v", fullName, err)
		return nil
	}
	if err := copyGitStorage(srcStor, stor); err != nil {
		log.Printf("bleephub: security advisory fork %s: copy git storage: %v", fullName, err)
		return nil
	}

	st.Repos[fork.ID] = fork
	st.ReposByName[fullName] = fork
	st.GitStorages[fullName] = stor

	st.ensureDefaultDiscussionCategoriesLocked(fork.ID)

	adv.PrivateForkID = fork.ID
	st.persistSecurityAdvisory(adv)

	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(fork.ID), fork)
	}
	return fork
}

// CreateSecurityAdvisoryReport persists a report record.
func (st *Store) CreateSecurityAdvisoryReport(report SecurityAdvisoryReport) *SecurityAdvisoryReport {
	st.mu.Lock()
	defer st.mu.Unlock()

	report.ID = st.NextSecurityAdvisoryReportID
	st.NextSecurityAdvisoryReportID++
	st.SecurityAdvisoryReports[report.ID] = &report
	if st.persist != nil {
		st.persist.MustPut("security_advisory_reports", strconv.Itoa(report.ID), &report)
	}
	return &report
}

func (st *Store) persistSecurityAdvisory(a *SecurityAdvisory) {
	if st.persist != nil {
		st.persist.MustPut("security_advisories", strconv.Itoa(a.ID), a)
	}
}
