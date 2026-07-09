package bleephub

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

type Repo struct {
	ID                                   int          `json:"id"`
	NodeID                               string       `json:"node_id"`
	Name                                 string       `json:"name"`
	FullName                             string       `json:"full_name"`
	Description                          string       `json:"description"`
	Homepage                             string       `json:"homepage"`
	DefaultBranch                        string       `json:"default_branch"`
	Visibility                           string       `json:"visibility"`
	Language                             string       `json:"language"`
	Owner                                *User        `json:"-"`
	OwnerID                              int          `json:"owner_id"`   // serialized so Owner can be relinked on reload
	OwnerType                            string       `json:"owner_type"` // "User" or "Organization"; empty means User for backwards compatibility
	Private                              bool         `json:"private"`
	Fork                                 bool         `json:"fork"`
	Archived                             bool         `json:"archived"`
	ArchivedAt                           *time.Time   `json:"archived_at,omitempty"`
	IsTemplate                           bool         `json:"is_template"`
	WebCommitSignoffRequired             bool         `json:"web_commit_signoff_required"`
	HasIssues                            bool         `json:"has_issues"`
	HasProjects                          bool         `json:"has_projects"`
	HasWiki                              bool         `json:"has_wiki"`
	HasDiscussions                       *bool        `json:"has_discussions"`
	HasPullRequests                      bool         `json:"has_pull_requests"`
	AllowSquashMerge                     bool         `json:"allow_squash_merge"`
	AllowMergeCommit                     bool         `json:"allow_merge_commit"`
	AllowRebaseMerge                     bool         `json:"allow_rebase_merge"`
	AllowAutoMerge                       bool         `json:"allow_auto_merge"`
	AllowUpdateBranch                    bool         `json:"allow_update_branch"`
	DeleteBranchOnMerge                  bool         `json:"delete_branch_on_merge"`
	UseSquashPRTitleAsDefault            bool         `json:"use_squash_pr_title_as_default"`
	SquashMergeCommitTitle               string       `json:"squash_merge_commit_title"`
	SquashMergeCommitMessage             string       `json:"squash_merge_commit_message"`
	MergeCommitTitle                     string       `json:"merge_commit_title"`
	MergeCommitMessage                   string       `json:"merge_commit_message"`
	PullRequestCreationPolicy            string       `json:"pull_request_creation_policy"`
	LicenseKey                           string       `json:"license_key"`
	LicenseName                          string       `json:"license_name"`
	LicenseSPDX                          string       `json:"license_spdx"`
	StargazersCount                      int          `json:"stargazers_count"`
	Topics                               []string     `json:"topics"`
	Stargazers                           map[int]bool `json:"stargazers,omitempty"`
	ParentID                             int          `json:"parent_id"`
	SourceID                             int          `json:"source_id"`
	NextIssueNumber                      int          `json:"-"`
	NextMilestoneNumber                  int          `json:"-"`
	AutomatedSecurityFixesEnabled        bool         `json:"automated_security_fixes_enabled"`
	PrivateVulnerabilityReportingEnabled bool         `json:"private_vulnerability_reporting_enabled"`
	VulnerabilityAlertsEnabled           bool         `json:"vulnerability_alerts_enabled"`
	InteractionLimit                     string       `json:"interaction_limit"`
	InteractionLimitExpiry               *time.Time   `json:"interaction_limit_expiry,omitempty"`
	CreatedAt                            time.Time    `json:"created_at"`
	UpdatedAt                            time.Time    `json:"updated_at"`
	PushedAt                             time.Time    `json:"pushed_at"`
}

func (st *Store) CreateRepo(owner *User, name, description string, private bool) *Repo {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createRepoLocked(owner.Login+"/"+name, name, description, private, owner.ID, "User", owner)
}

// createRepoLocked creates a repo record and its git storage. Caller must hold st.mu.
func (st *Store) createRepoLocked(fullName, name, description string, private bool, ownerID int, ownerType string, owner *User) *Repo {
	if _, exists := st.ReposByName[fullName]; exists {
		return nil
	}

	now := time.Now().UTC()
	visibility := "public"
	if private {
		visibility = "private"
	}

	repo := &Repo{
		ID:                        st.NextRepo,
		NodeID:                    fmt.Sprintf("R_kgDO%08d", st.NextRepo),
		Name:                      name,
		FullName:                  fullName,
		Description:               description,
		DefaultBranch:             "main",
		Visibility:                visibility,
		Owner:                     owner,
		OwnerID:                   ownerID,
		OwnerType:                 ownerType,
		Private:                   private,
		HasIssues:                 true,
		HasProjects:               false,
		HasWiki:                   false,
		HasDiscussions:            boolPointer(true),
		HasPullRequests:           true,
		AllowSquashMerge:          true,
		AllowMergeCommit:          true,
		AllowRebaseMerge:          true,
		PullRequestCreationPolicy: "all",
		Topics:                    []string{},
		Stargazers:                map[int]bool{},
		NextIssueNumber:           1,
		NextMilestoneNumber:       1,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	st.NextRepo++

	// Open git storage before registering the repo so a failure leaves no
	// half-created entry behind, and log the cause — a bare nil return reads
	// as "name taken" at the call sites, hiding storage misconfiguration.
	stor, err := openOrInitGitStorage(context.Background(), fullName)
	if err != nil {
		log.Printf("bleephub: create repo %s: open git storage: %v", fullName, err)
		return nil
	}

	st.Repos[repo.ID] = repo
	st.ReposByName[fullName] = repo
	st.GitStorages[fullName] = stor

	st.ensureDefaultDiscussionCategoriesLocked(repo.ID)

	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}

	return repo
}

func (st *Store) GetRepo(owner, name string) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ReposByName[owner+"/"+name]
}

func (st *Store) GetRepoByID(id int) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Repos[id]
}

func (st *Store) UpdateRepo(owner, name string, fn func(*Repo)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo, ok := st.ReposByName[owner+"/"+name]
	if !ok {
		return false
	}
	fn(repo)
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// ForkRepo creates a fork of sourceRepo owned by owner. It copies the git
// storage and records parent/source linkage. Returns nil if the source repo
// does not exist or the target name is already taken.
func (st *Store) ForkRepo(owner *User, sourceRepo *Repo, name string) *Repo {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner.Login + "/" + name
	if _, exists := st.ReposByName[fullName]; exists {
		return nil
	}

	srcStor, ok := st.GitStorages[sourceRepo.FullName]
	if !ok {
		return nil
	}

	sourceID := sourceRepo.ID
	if sourceRepo.SourceID != 0 {
		sourceID = sourceRepo.SourceID
	}

	repo := &Repo{
		ID:                        st.NextRepo,
		NodeID:                    fmt.Sprintf("R_kgDO%08d", st.NextRepo),
		Name:                      name,
		FullName:                  fullName,
		Description:               sourceRepo.Description,
		Homepage:                  sourceRepo.Homepage,
		DefaultBranch:             sourceRepo.DefaultBranch,
		Visibility:                sourceRepo.Visibility,
		Language:                  sourceRepo.Language,
		Owner:                     owner,
		OwnerID:                   owner.ID,
		OwnerType:                 "User",
		Private:                   sourceRepo.Private,
		Fork:                      true,
		Archived:                  sourceRepo.Archived,
		ArchivedAt:                cloneTimePtr(sourceRepo.ArchivedAt),
		ParentID:                  sourceRepo.ID,
		SourceID:                  sourceID,
		HasIssues:                 sourceRepo.HasIssues,
		HasProjects:               sourceRepo.HasProjects,
		HasWiki:                   sourceRepo.HasWiki,
		HasDiscussions:            boolPointer(repoHasDiscussions(sourceRepo)),
		HasPullRequests:           sourceRepo.HasPullRequests,
		AllowSquashMerge:          sourceRepo.AllowSquashMerge,
		AllowMergeCommit:          sourceRepo.AllowMergeCommit,
		AllowRebaseMerge:          sourceRepo.AllowRebaseMerge,
		AllowAutoMerge:            sourceRepo.AllowAutoMerge,
		AllowUpdateBranch:         sourceRepo.AllowUpdateBranch,
		DeleteBranchOnMerge:       sourceRepo.DeleteBranchOnMerge,
		UseSquashPRTitleAsDefault: sourceRepo.UseSquashPRTitleAsDefault,
		SquashMergeCommitTitle:    sourceRepo.SquashMergeCommitTitle,
		SquashMergeCommitMessage:  sourceRepo.SquashMergeCommitMessage,
		MergeCommitTitle:          sourceRepo.MergeCommitTitle,
		MergeCommitMessage:        sourceRepo.MergeCommitMessage,
		PullRequestCreationPolicy: sourceRepo.PullRequestCreationPolicy,
		LicenseKey:                sourceRepo.LicenseKey,
		LicenseName:               sourceRepo.LicenseName,
		LicenseSPDX:               sourceRepo.LicenseSPDX,
		Topics:                    append([]string(nil), sourceRepo.Topics...),
		Stargazers:                map[int]bool{},
		NextIssueNumber:           1,
		NextMilestoneNumber:       1,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
		PushedAt:                  sourceRepo.PushedAt,
	}
	st.NextRepo++

	stor, err := openOrInitGitStorage(context.Background(), fullName)
	if err != nil {
		log.Printf("bleephub: fork repo %s: open git storage: %v", fullName, err)
		return nil
	}
	if err := copyGitStorage(srcStor, stor); err != nil {
		log.Printf("bleephub: fork repo %s: copy git storage: %v", fullName, err)
		return nil
	}

	st.Repos[repo.ID] = repo
	st.ReposByName[fullName] = repo
	st.GitStorages[fullName] = stor

	st.ensureDefaultDiscussionCategoriesLocked(repo.ID)

	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return repo
}

func boolPointer(v bool) *bool {
	return &v
}

func repoHasDiscussions(repo *Repo) bool {
	if repo == nil || repo.HasDiscussions == nil {
		return true
	}
	return *repo.HasDiscussions
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	clone := t.UTC()
	return &clone
}

// RenameRepo renames owner/name to owner/newName, moving every map keyed by
// the repo full name and updating embedded repo-name strings. It returns true
// on success.
func (st *Store) RenameRepo(owner, name, newName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	oldFull := owner + "/" + name
	newFull := owner + "/" + newName
	if oldFull == newFull {
		return true
	}
	repo, ok := st.ReposByName[oldFull]
	if !ok {
		return false
	}
	if _, exists := st.ReposByName[newFull]; exists {
		return false
	}

	// Rename on-disk / S3 git storage first; if it fails, abort before mutating
	// in-memory indexes.
	if GitDataDir() != "" {
		oldDir := filepath.Join(GitDataDir(), filepath.FromSlash(oldFull))
		newDir := filepath.Join(GitDataDir(), filepath.FromSlash(newFull))
		if err := os.Rename(oldDir, newDir); err != nil && os.IsExist(err) {
			log.Printf("bleephub: rename repo %s -> %s: move git dir: %v", oldFull, newFull, err)
			return false
		}
	}
	if IsS3GitStorage() {
		s3fs, err := getS3FS(context.Background())
		if err != nil {
			log.Printf("bleephub: rename repo %s -> %s: resolve s3 fs: %v", oldFull, newFull, err)
			return false
		}
		if s3fs != nil {
			if err := s3fs.renameRepoPrefix(oldFull, newFull); err != nil {
				log.Printf("bleephub: rename repo %s -> %s: move s3 prefix: %v", oldFull, newFull, err)
				return false
			}
		}
	}

	repo.Name = newName
	repo.FullName = newFull
	repo.UpdatedAt = time.Now().UTC()

	st.ReposByName[newFull] = repo
	delete(st.ReposByName, oldFull)

	if stor := st.GitStorages[oldFull]; stor != nil {
		st.GitStorages[newFull] = stor
		delete(st.GitStorages, oldFull)
	}
	st.moveRepoKeyLocked(oldFull, newFull)

	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}

	return true
}

func (st *Store) DeleteRepo(owner, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner + "/" + name
	repo, ok := st.ReposByName[fullName]
	if !ok {
		return false
	}

	delete(st.Repos, repo.ID)
	delete(st.ReposByName, fullName)
	delete(st.GitStorages, fullName)
	if st.persist != nil {
		st.persist.MustDelete("repos", strconv.Itoa(repo.ID))
	}

	// Cascade: purge everything keyed to this repo from memory AND the DB.
	// Hook IDs, issue numbers and release IDs restart from the surviving
	// maxima after a reload, so leftovers would be inherited by a recreated
	// same-name repo.
	for _, h := range st.Hooks[fullName] {
		delete(st.HookDeliveries, h.ID)
		if st.persist != nil {
			st.persist.MustDelete("hook_deliveries", strconv.Itoa(h.ID))
		}
	}
	delete(st.Hooks, fullName)
	delete(st.RepoSecrets, fullName)
	delete(st.RepoVariables, fullName)
	delete(st.RepoCollaborators, fullName)
	delete(st.RepoAutolinks, fullName)
	delete(st.RepoInvitations, fullName)
	delete(st.RepoDeployKeys, fullName)
	delete(st.CheckSuitePrefs, fullName)
	delete(st.DependabotSecrets, fullName)
	delete(st.CodeScanningDefaultSetups, fullName)
	delete(st.CodeQualitySetups, fullName)
	delete(st.RepoCustomPropertyValues, fullName)
	delete(st.RepoImmutableReleases, fullName)
	delete(st.AgentsRepoSecrets, fullName)
	delete(st.AgentsRepoVariables, fullName)
	delete(st.SecretScanningPushPlaceholders, fullName)
	delete(st.SecretScanningPushBypasses, fullName)
	st.CommitStatuses.deleteRepoKey(fullName)
	st.CommitComments.deleteRepo(repo.ID)
	for id, alert := range st.SecretScanningAlerts {
		if alert.RepoKey == fullName {
			delete(st.SecretScanningAlerts, id)
			if st.persist != nil {
				st.persist.MustDelete("secret_scanning_alerts", strconv.Itoa(id))
			}
		}
	}
	delete(st.SecretScanningAlertsByRepo, fullName)
	delete(st.SecretScanningNextNumber, fullName)
	if st.persist != nil {
		st.persist.MustDelete("hooks", fullName)
		st.persist.MustDelete("repo_secrets", fullName)
		st.persist.MustDelete("repo_variables", fullName)
		st.persist.MustDelete("repo_collaborators", fullName)
		st.persist.MustDelete("repo_autolinks", fullName)
		st.persist.MustDelete("repo_invitations", fullName)
		st.persist.MustDelete("repo_deploy_keys", fullName)
		st.persist.MustDelete("check_suite_prefs", fullName)
		st.persist.MustDelete("secret_scanning_alerts", fullName)
		st.persist.MustDelete("dependabot_secrets", fullName)
		st.persist.MustDelete("code_scanning_default_setups", fullName)
		st.persist.MustDelete("code_quality_setups", fullName)
		st.persist.MustDelete("repo_custom_property_values", fullName)
		st.persist.MustDelete("repo_immutable_releases", fullName)
		st.persist.MustDelete("agents_repo_secrets", fullName)
		st.persist.MustDelete("agents_repo_variables", fullName)
		st.persist.MustDelete("secret_scanning_push_placeholders", fullName)
		st.persist.MustDelete("secret_scanning_push_bypasses", fullName)
	}
	for id, suite := range st.CheckSuites {
		if suite.RepoKey == fullName {
			delete(st.CheckSuites, id)
			if st.persist != nil {
				st.persist.MustDelete("check_suites", strconv.FormatInt(id, 10))
			}
		}
	}
	for id, run := range st.CheckRuns {
		if run.RepoKey == fullName {
			delete(st.CheckRuns, id)
			if st.persist != nil {
				st.persist.MustDelete("check_runs", strconv.FormatInt(id, 10))
			}
		}
	}
	for id, alert := range st.CodeScanningAlerts {
		if alert.RepoKey == fullName {
			delete(st.CodeScanningAlerts, id)
			if st.persist != nil {
				st.persist.MustDelete("code_scanning_alerts", strconv.Itoa(id))
			}
		}
	}
	delete(st.CodeScanningAlertsByRepo, fullName)
	delete(st.CodeScanningNextNumber, fullName)
	for id, analysis := range st.CodeScanningAnalyses {
		if analysis.RepoKey == fullName {
			delete(st.CodeScanningAnalyses, id)
			if st.persist != nil {
				st.persist.MustDelete("code_scanning_analyses", strconv.Itoa(id))
			}
		}
	}
	delete(st.CodeScanningAnalysesByRepo, fullName)
	for key, fix := range st.CodeScanningAutofixes {
		if fix.RepoKey == fullName {
			delete(st.CodeScanningAutofixes, key)
			if st.persist != nil {
				st.persist.MustDelete("code_scanning_autofixes", key)
			}
		}
	}
	for id, up := range st.SARIFUploads {
		if up.RepoKey == fullName {
			delete(st.SARIFUploads, id)
			if st.persist != nil {
				st.persist.MustDelete("sarif_uploads", id)
			}
		}
	}
	for id, db := range st.CodeQLDatabases {
		if db.RepoKey == fullName {
			delete(st.CodeQLDatabases, id)
			if st.persist != nil {
				st.persist.MustDelete("codeql_databases", strconv.Itoa(id))
			}
		}
	}
	delete(st.CodeQLDatabasesByRepo, fullName)
	for id, va := range st.CodeQLVariantAnalyses {
		if va.ControllerRepoKey == fullName {
			delete(st.CodeQLVariantAnalyses, id)
			if st.persist != nil {
				st.persist.MustDelete("codeql_variant_analyses", strconv.Itoa(id))
			}
		}
	}
	for id, alert := range st.DependabotAlerts {
		if alert.RepoKey == fullName {
			delete(st.DependabotAlerts, id)
			if st.persist != nil {
				st.persist.MustDelete("dependabot_alerts", strconv.Itoa(id))
			}
		}
	}
	delete(st.DependabotAlertsByRepo, fullName)
	delete(st.DependabotNextNumber, fullName)
	for id, rs := range st.Rulesets {
		if rs.RepoID == repo.ID {
			delete(st.Rulesets, id)
			if st.persist != nil {
				st.persist.MustDelete("repo_rulesets", strconv.Itoa(id))
			}
		}
	}
	for id, project := range st.ProjectClassic {
		if project.RepoKey == fullName {
			delete(st.ProjectClassic, id)
			if st.persist != nil {
				st.persist.MustDelete("projects_classic", strconv.Itoa(id))
			}
			for columnID, column := range st.ProjectColumns {
				if column.ProjectID == id {
					delete(st.ProjectColumns, columnID)
					if st.persist != nil {
						st.persist.MustDelete("project_columns", strconv.Itoa(columnID))
					}
					for cardID, card := range st.ProjectCards {
						if card.ColumnID == columnID {
							delete(st.ProjectCards, cardID)
							if st.persist != nil {
								st.persist.MustDelete("project_cards", strconv.Itoa(cardID))
							}
						}
					}
				}
			}
		}
	}
	for id, cs := range st.Codespaces {
		if cs.RepoKey == fullName {
			delete(st.Codespaces, id)
			delete(st.CodespacesByName, cs.Name)
			if st.persist != nil {
				st.persist.MustDelete("codespaces", strconv.Itoa(id))
			}
		}
	}
	if pkgs := st.PackagesByOwnerKey[fullName]; len(pkgs) > 0 {
		for _, pkg := range pkgs {
			delete(st.Packages, pkg.ID)
			if st.persist != nil {
				st.persist.MustDelete("packages", strconv.Itoa(pkg.ID))
			}
			for versionID := range st.PackageVersionsByPackage[pkg.ID] {
				delete(st.PackageVersions, versionID)
				if st.persist != nil {
					st.persist.MustDelete("package_versions", strconv.Itoa(versionID))
				}
				for fileID, file := range st.PackageFiles {
					if file.VersionID == versionID {
						delete(st.PackageFiles, fileID)
						if st.persist != nil {
							st.persist.MustDelete("package_files", strconv.Itoa(fileID))
						}
					}
				}
				delete(st.PackageFilesByVersion, versionID)
			}
			delete(st.PackageVersionsByPackage, pkg.ID)
		}
		delete(st.PackagesByOwnerKey, fullName)
	}
	for id, alert := range st.SecurityAdvisories {
		if alert.RepoID == repo.ID {
			delete(st.SecurityAdvisories, id)
			if st.persist != nil {
				st.persist.MustDelete("security_advisories", strconv.Itoa(id))
			}
		}
	}
	delete(st.SecurityAdvisoriesByRepo, fullName)
	if st.persist != nil {
		st.persist.MustDelete("security_advisories", fullName)
	}
	for k := range st.EnvSecrets {
		repoKey, _, found := strings.Cut(k, "\x1f")
		if found && repoKey == fullName {
			delete(st.EnvSecrets, k)
			if st.persist != nil {
				st.persist.MustDelete("env_secrets", k)
			}
		}
	}
	for k := range st.EnvVariables {
		repoKey, _, found := strings.Cut(k, "\x1f")
		if found && repoKey == fullName {
			delete(st.EnvVariables, k)
			if st.persist != nil {
				st.persist.MustDelete("env_variables", k)
			}
		}
	}
	for id, issue := range st.Issues {
		if issue.RepoID == repo.ID {
			delete(st.Issues, id)
			st.unindexIssueLocked(issue)
			if st.persist != nil {
				st.persist.MustDelete("issues", strconv.Itoa(id))
			}
		}
	}
	delete(st.IssuesByRepo, repo.ID)
	for id, pr := range st.PullRequests {
		if pr.RepoID == repo.ID {
			delete(st.PullRequests, id)
			st.unindexPullLocked(pr)
			if st.persist != nil {
				st.persist.MustDelete("pull_requests", strconv.Itoa(id))
			}
		}
	}
	delete(st.PullsByRepo, repo.ID)
	st.Releases.DeleteAllForRepo(repo.ID)

	// Discussion surfaces — comments first because they reference discussions.
	for id, c := range st.DiscussionComments {
		if d := st.Discussions[c.DiscussionID]; d != nil && d.RepoID == repo.ID {
			delete(st.DiscussionComments, id)
			if st.persist != nil {
				st.persist.MustDelete("discussion_comments", strconv.Itoa(id))
			}
		}
	}
	for id, d := range st.Discussions {
		if d.RepoID == repo.ID {
			delete(st.Discussions, id)
			if st.persist != nil {
				st.persist.MustDelete("discussions", strconv.Itoa(id))
			}
		}
	}
	for id, cat := range st.DiscussionCategories {
		if cat.RepoID == repo.ID {
			delete(st.DiscussionCategories, id)
			if st.persist != nil {
				st.persist.MustDelete("discussion_categories", strconv.Itoa(id))
			}
		}
	}

	// Misc surfaces: branch protection is keyed "repoID:branch", pages
	// builds by "owner/name".
	st.Misc.mu.Lock()
	bpPrefix := strconv.Itoa(repo.ID) + ":"
	for key := range st.Misc.branchProtection {
		if strings.HasPrefix(key, bpPrefix) {
			delete(st.Misc.branchProtection, key)
			if st.persist != nil {
				st.persist.MustDelete("branch_protection", key)
			}
		}
	}
	delete(st.Misc.pagesBuilds, fullName)
	if st.persist != nil {
		st.persist.MustDelete("pages_builds", fullName)
	}
	st.Misc.mu.Unlock()

	gitDir := GitDataDir()
	if gitDir != "" {
		repoDir := filepath.Join(gitDir, filepath.FromSlash(fullName))
		_ = os.RemoveAll(repoDir)
	}
	if IsS3GitStorage() {
		s3fs, err := getS3FS(context.Background())
		if err != nil {
			log.Printf("bleephub: delete repo %s: resolve S3 git storage: %v (object prefix left orphaned)", fullName, err)
		} else if s3fs != nil {
			if err := s3fs.deleteRepoPrefix(fullName); err != nil {
				log.Printf("bleephub: delete repo %s: purge S3 object prefix: %v (objects left orphaned)", fullName, err)
			}
		}
	}
	return true
}

// ListForks returns all repositories whose ParentID or SourceID matches
// sourceRepoID, sorted/paged according to opts.
func (st *Store) ListForks(sourceRepoID int, opts RepoListOptions) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var repos []*Repo
	for _, r := range st.Repos {
		if r.Fork && (r.ParentID == sourceRepoID || r.SourceID == sourceRepoID) {
			repos = append(repos, r)
		}
	}
	return filterSortPaginateRepos(repos, opts)
}

// CountForks returns how many repositories were forked from the given
// repository (matched by ParentID or SourceID lineage).
func (st *Store) CountForks(sourceRepoID int) int {
	st.mu.RLock()
	defer st.mu.RUnlock()

	n := 0
	for _, r := range st.Repos {
		if r.Fork && (r.ParentID == sourceRepoID || r.SourceID == sourceRepoID) {
			n++
		}
	}
	return n
}

func (st *Store) ListReposByOwner(login string) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	prefix := login + "/"
	var repos []*Repo
	for k, r := range st.ReposByName {
		if strings.HasPrefix(k, prefix) {
			repos = append(repos, r)
		}
	}
	return repos
}

// RepoListOptions controls filtering, sorting and pagination for repo list
// endpoints. A zero value applies GitHub's defaults. Set NoPaginate when the
// caller will paginate itself (e.g. REST handlers use paginateAndLink).
type RepoListOptions struct {
	Type        string // org: all/public/private/forks/sources/member; user: all/owner/member
	Visibility  string // all/public/private
	Affiliation string // owner,collaborator,organization_member
	Sort        string // created/updated/pushed/full_name
	Direction   string // asc/desc
	PerPage     int
	Page        int
	NoPaginate  bool
}

func (o RepoListOptions) normalize() RepoListOptions {
	if !o.NoPaginate {
		if o.PerPage <= 0 {
			o.PerPage = 30
		}
		if o.PerPage > 100 {
			o.PerPage = 100
		}
		if o.Page <= 0 {
			o.Page = 1
		}
	}
	if o.Sort == "" {
		o.Sort = "created"
	}
	if o.Direction == "" {
		if o.Sort == "full_name" {
			o.Direction = "asc"
		} else {
			o.Direction = "desc"
		}
	}
	return o
}

// ListReposForOrg returns repos owned by an organization, filtered/sorted/paged.
func (st *Store) ListReposForOrg(org string, opts RepoListOptions) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	prefix := org + "/"
	var repos []*Repo
	for k, r := range st.ReposByName {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if r.OwnerType != "Organization" {
			continue
		}
		repos = append(repos, r)
	}
	return filterSortPaginateRepos(repos, opts)
}

// ListReposForUser returns public repos owned by a user, filtered/sorted/paged.
func (st *Store) ListReposForUser(user *User, opts RepoListOptions) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	prefix := user.Login + "/"
	var repos []*Repo
	for k, r := range st.ReposByName {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if r.OwnerType != "User" && r.OwnerType != "" {
			continue
		}
		if r.Private {
			continue
		}
		repos = append(repos, r)
	}
	return filterSortPaginateRepos(repos, opts)
}

// ListReposForAuthUser returns repos the authenticated user can access.
// Affiliation controls owner/collaborator/org-member inclusion.
func (st *Store) ListReposForAuthUser(user *User, opts RepoListOptions) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()

	affiliation := opts.Affiliation
	if affiliation == "" {
		affiliation = "owner,collaborator,organization_member"
	}
	includeOwner := strings.Contains(affiliation, "owner")
	includeCollab := strings.Contains(affiliation, "collaborator")
	includeOrgMember := strings.Contains(affiliation, "organization_member")

	seen := make(map[int]bool)
	var repos []*Repo

	// owner affiliation
	if includeOwner {
		prefix := user.Login + "/"
		for k, r := range st.ReposByName {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			if r.OwnerType != "User" && r.OwnerType != "" {
				continue
			}
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			repos = append(repos, r)
		}
	}

	// collaborator affiliation: repositories where the user has been added as
	// a collaborator.
	if includeCollab {
		for fullName, perms := range st.RepoCollaborators {
			if _, ok := perms[user.Login]; !ok {
				continue
			}
			repo := st.ReposByName[fullName]
			if repo == nil || seen[repo.ID] {
				continue
			}
			seen[repo.ID] = true
			repos = append(repos, repo)
		}
	}

	// organization_member affiliation: every repository owned by an org the
	// user is a member of.
	if includeOrgMember {
		for _, org := range st.Orgs {
			if st.Memberships[membershipKey(org.Login, user.ID)] == nil {
				continue
			}
			prefix := org.Login + "/"
			for k, r := range st.ReposByName {
				if !strings.HasPrefix(k, prefix) {
					continue
				}
				if seen[r.ID] {
					continue
				}
				seen[r.ID] = true
				repos = append(repos, r)
			}
		}
	}

	return filterSortPaginateRepos(repos, opts)
}

func filterSortPaginateRepos(repos []*Repo, opts RepoListOptions) []*Repo {
	repos = filterSortRepos(repos, opts)
	if opts.NoPaginate {
		return repos
	}

	opts = opts.normalize()

	// paginate
	start := (opts.Page - 1) * opts.PerPage
	if start > len(repos) {
		return []*Repo{}
	}
	end := start + opts.PerPage
	if end > len(repos) {
		end = len(repos)
	}
	return repos[start:end]
}

// filterSortRepos applies filtering and sorting without pagination.
func filterSortRepos(repos []*Repo, opts RepoListOptions) []*Repo {
	opts = opts.normalize()

	// visibility filter
	switch opts.Visibility {
	case "public":
		filtered := repos[:0]
		for _, r := range repos {
			if !r.Private {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	case "private":
		filtered := repos[:0]
		for _, r := range repos {
			if r.Private {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	}

	// type filter
	switch opts.Type {
	case "public":
		filtered := repos[:0]
		for _, r := range repos {
			if !r.Private {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	case "private":
		filtered := repos[:0]
		for _, r := range repos {
			if r.Private {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	case "forks":
		filtered := repos[:0]
		for _, r := range repos {
			if r.Fork {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	case "sources":
		filtered := repos[:0]
		for _, r := range repos {
			if !r.Fork {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	case "owner":
		// For user endpoints this means repos owned by the user; auth user
		// endpoints use affiliation instead. Keep all repos already scoped.
	case "member":
		// bleephub does not model team-based repo membership; empty.
		repos = repos[:0]
	}

	// sort
	sort.SliceStable(repos, func(i, j int) bool {
		var less bool
		switch opts.Sort {
		case "updated":
			less = repos[i].UpdatedAt.Before(repos[j].UpdatedAt)
		case "pushed":
			less = repos[i].PushedAt.Before(repos[j].PushedAt)
		case "full_name":
			less = repos[i].FullName < repos[j].FullName
		default: // "created"
			less = repos[i].CreatedAt.Before(repos[j].CreatedAt)
		}
		if opts.Direction == "asc" {
			return less
		}
		return !less
	})

	return repos
}

func (st *Store) GetGitStorage(owner, name string) gitStorage.Storer {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.GitStorages[owner+"/"+name]
}

func (st *Store) GitStorageForRepoID(repoID int) (gitStorage.Storer, string) {
	st.mu.RLock()
	defer st.mu.RUnlock()
	repo := st.Repos[repoID]
	if repo == nil {
		return nil, ""
	}
	return st.GitStorages[repo.FullName], repo.FullName
}

// RepoSize returns the on-disk size of the repository's git storage in
// kilobytes, matching GitHub's `size` field unit. For in-memory storage the
// result is 0; for S3-backed storage the result is also 0 until a real
// list-objects sum is implemented.
func (st *Store) RepoSize(fullName string) int64 {
	if IsS3GitStorage() {
		return 0
	}
	gitDir := GitDataDir()
	if gitDir == "" {
		return 0
	}
	repoDir := filepath.Join(gitDir, filepath.FromSlash(fullName))
	var total int64
	_ = filepath.Walk(repoDir, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total / 1024
}

// RepoPermission is the access level a collaborator has on a repo.
type RepoPermission string

const (
	RepoPermPull  RepoPermission = "pull"
	RepoPermPush  RepoPermission = "push"
	RepoPermAdmin RepoPermission = "admin"
)

// AddRepoCollaborator grants permission to login on the repo. Only pull,
// push, and admin are accepted (matches GitHub). Returns true if the repo
// exists and the user exists.
func (st *Store) AddRepoCollaborator(owner, name, login, permission string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner + "/" + name
	repo, ok := st.ReposByName[fullName]
	if !ok {
		return false
	}
	u := st.UsersByLogin[login]
	if u == nil {
		return false
	}
	perm := normalizeRepoPermission(permission)
	if st.RepoCollaborators[fullName] == nil {
		st.RepoCollaborators[fullName] = map[string]string{}
	}
	st.RepoCollaborators[fullName][login] = perm
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repo_collaborators", fullName, st.RepoCollaborators[fullName])
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// RemoveRepoCollaborator removes a collaborator from the repo. Returns true
// if one was removed.
func (st *Store) RemoveRepoCollaborator(owner, name, login string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner + "/" + name
	repo, ok := st.ReposByName[fullName]
	if !ok {
		return false
	}
	if st.RepoCollaborators[fullName] == nil {
		return false
	}
	if _, ok := st.RepoCollaborators[fullName][login]; !ok {
		return false
	}
	delete(st.RepoCollaborators[fullName], login)
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repo_collaborators", fullName, st.RepoCollaborators[fullName])
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// GetRepoCollaboratorPermission returns the permission string for a
// collaborator, or "" if none.
func (st *Store) GetRepoCollaboratorPermission(owner, name, login string) string {
	st.mu.RLock()
	defer st.mu.RUnlock()

	fullName := owner + "/" + name
	if st.RepoCollaborators[fullName] == nil {
		return ""
	}
	return st.RepoCollaborators[fullName][login]
}

// ListRepoCollaborators returns the collaborators of a repo.
func (st *Store) ListRepoCollaborators(owner, name string) map[string]string {
	st.mu.RLock()
	defer st.mu.RUnlock()

	fullName := owner + "/" + name
	out := make(map[string]string, len(st.RepoCollaborators[fullName]))
	for k, v := range st.RepoCollaborators[fullName] {
		out[k] = v
	}
	return out
}

func normalizeRepoPermission(p string) string {
	switch strings.ToLower(p) {
	case "admin":
		return "admin"
	case "push", "write":
		return "push"
	case "pull", "read", "":
		return "pull"
	default:
		return "pull"
	}
}

// StarRepo adds userID to the repo's stargazers and records the starred repo
// on the user. Idempotent. Returns true if the star was newly added.
func (st *Store) StarRepo(userID int, owner, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner + "/" + name
	repo, ok := st.ReposByName[fullName]
	if !ok {
		return false
	}
	user, ok := st.Users[userID]
	if !ok {
		return false
	}
	if repo.Stargazers == nil {
		repo.Stargazers = map[int]bool{}
	}
	if user.StarredRepos == nil {
		user.StarredRepos = map[string]bool{}
	}
	if repo.Stargazers[userID] {
		return false
	}
	repo.Stargazers[userID] = true
	repo.StargazersCount = len(repo.Stargazers)
	repo.UpdatedAt = time.Now().UTC()
	user.StarredRepos[fullName] = true
	user.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
		st.persist.MustPut("users", strconv.Itoa(user.ID), user)
	}
	return true
}

// UnstarRepo removes userID from the repo's stargazers. Returns true if a
// star was actually removed.
func (st *Store) UnstarRepo(userID int, owner, name string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner + "/" + name
	repo, ok := st.ReposByName[fullName]
	if !ok {
		return false
	}
	user, ok := st.Users[userID]
	if !ok {
		return false
	}
	if repo.Stargazers == nil || !repo.Stargazers[userID] {
		return false
	}
	delete(repo.Stargazers, userID)
	repo.StargazersCount = len(repo.Stargazers)
	repo.UpdatedAt = time.Now().UTC()
	if user.StarredRepos != nil {
		delete(user.StarredRepos, fullName)
	}
	user.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
		st.persist.MustPut("users", strconv.Itoa(user.ID), user)
	}
	return true
}

// IsRepoStarredBy reports whether userID has starred the repo.
func (st *Store) IsRepoStarredBy(userID int, owner, name string) bool {
	st.mu.RLock()
	defer st.mu.RUnlock()

	repo, ok := st.ReposByName[owner+"/"+name]
	if !ok || repo.Stargazers == nil {
		return false
	}
	return repo.Stargazers[userID]
}

// ListRepoStargazers returns the user IDs who starred the repo, sorted
// ascending.
func (st *Store) ListRepoStargazers(owner, name string) []int {
	st.mu.RLock()
	defer st.mu.RUnlock()

	repo, ok := st.ReposByName[owner+"/"+name]
	if !ok || repo.Stargazers == nil {
		return nil
	}
	out := make([]int, 0, len(repo.Stargazers))
	for id := range repo.Stargazers {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// ListStarredRepos returns the full names of repos starred by userID.
func (st *Store) ListStarredRepos(userID int) []string {
	st.mu.RLock()
	defer st.mu.RUnlock()

	user, ok := st.Users[userID]
	if !ok || user.StarredRepos == nil {
		return nil
	}
	out := make([]string, 0, len(user.StarredRepos))
	for name := range user.StarredRepos {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// RepoDeployKey represents a deploy key configured on a repository.
type RepoDeployKey struct {
	ID        int       `json:"id"`
	NodeID    string    `json:"node_id"`
	RepoID    int       `json:"repo_id"`
	Title     string    `json:"title"`
	Key       string    `json:"key"`
	ReadOnly  bool      `json:"read_only"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

// ListRepoDeployKeys returns deploy keys for a repo, sorted by ID.
func (st *Store) ListRepoDeployKeys(repoID int) []*RepoDeployKey {
	st.mu.RLock()
	defer st.mu.RUnlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}
	keys := st.RepoDeployKeys[repo.FullName]
	out := make([]*RepoDeployKey, 0, len(keys))
	for _, k := range keys {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// GetRepoDeployKey returns a deploy key by ID.
func (st *Store) GetRepoDeployKey(id int) *RepoDeployKey {
	st.mu.RLock()
	defer st.mu.RUnlock()

	for _, keys := range st.RepoDeployKeys {
		if k := keys[id]; k != nil {
			return k
		}
	}
	return nil
}

// CreateRepoDeployKey adds a deploy key to a repo.
func (st *Store) CreateRepoDeployKey(repoID int, title, key string, readOnly bool) *RepoDeployKey {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return nil
	}
	if st.RepoDeployKeys[repo.FullName] == nil {
		st.RepoDeployKeys[repo.FullName] = map[int]*RepoDeployKey{}
	}
	k := &RepoDeployKey{
		ID:        st.NextDeployKeyID,
		NodeID:    fmt.Sprintf("RkEAxNNa%07d", st.NextDeployKeyID),
		RepoID:    repoID,
		Title:     title,
		Key:       key,
		ReadOnly:  readOnly,
		Verified:  true,
		CreatedAt: time.Now().UTC(),
	}
	st.RepoDeployKeys[repo.FullName][k.ID] = k
	st.NextDeployKeyID++
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repo_deploy_keys", repo.FullName, st.RepoDeployKeys[repo.FullName])
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return k
}

// DeleteRepoDeployKey removes a deploy key by ID.
func (st *Store) DeleteRepoDeployKey(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	for repoKey, keys := range st.RepoDeployKeys {
		if k := keys[id]; k != nil {
			delete(keys, id)
			repo := st.Repos[k.RepoID]
			if repo != nil {
				repo.UpdatedAt = time.Now().UTC()
				if st.persist != nil {
					st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
				}
			}
			if st.persist != nil {
				st.persist.MustPut("repo_deploy_keys", repoKey, st.RepoDeployKeys[repoKey])
			}
			return true
		}
	}
	return false
}

// RepoSubscription records a user's watch subscription for a repo.
type RepoSubscription struct {
	UserID     int       `json:"user_id"`
	RepoID     int       `json:"repo_id"`
	Subscribed bool      `json:"subscribed"`
	Ignored    bool      `json:"ignored"`
	CreatedAt  time.Time `json:"created_at"`
}

func repoSubscriptionKey(userID, repoID int) string {
	return strconv.Itoa(userID) + ":" + strconv.Itoa(repoID)
}

// SetRepoSubscription creates or updates a subscription.
func (st *Store) SetRepoSubscription(userID int, repoID int, subscribed bool) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.Repos[repoID] == nil {
		return false
	}
	key := repoSubscriptionKey(userID, repoID)
	sub := &RepoSubscription{
		UserID:     userID,
		RepoID:     repoID,
		Subscribed: subscribed,
		Ignored:    false,
		CreatedAt:  time.Now().UTC(),
	}
	if existing := st.RepoSubscriptions[key]; existing != nil {
		sub.CreatedAt = existing.CreatedAt
	}
	st.RepoSubscriptions[key] = sub
	if st.persist != nil {
		st.persist.MustPut("repo_subscriptions", key, sub)
	}
	return true
}

// DeleteRepoSubscription removes a subscription.
func (st *Store) DeleteRepoSubscription(userID int, repoID int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	key := repoSubscriptionKey(userID, repoID)
	if st.RepoSubscriptions[key] == nil {
		return false
	}
	delete(st.RepoSubscriptions, key)
	if st.persist != nil {
		st.persist.MustDelete("repo_subscriptions", key)
	}
	return true
}

// GetRepoSubscription returns a subscription or nil.
func (st *Store) GetRepoSubscription(userID int, repoID int) *RepoSubscription {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return st.RepoSubscriptions[repoSubscriptionKey(userID, repoID)]
}

// ListRepoSubscriptionsForUser returns the repositories subscribed by userID.
func (st *Store) ListRepoSubscriptionsForUser(userID int) []*Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make([]*Repo, 0)
	for key, sub := range st.RepoSubscriptions {
		if sub == nil || sub.UserID != userID || !sub.Subscribed {
			continue
		}
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		repoID, _ := strconv.Atoi(parts[1])
		if repo := st.Repos[repoID]; repo != nil {
			out = append(out, repo)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// TransferRepo transfers ownership of a repository to a new owner account.
// It returns true on success.
func (st *Store) TransferRepo(owner, name, newOwner string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	oldFull := owner + "/" + name
	newFull := newOwner + "/" + name
	if oldFull == newFull {
		return true
	}
	repo, ok := st.ReposByName[oldFull]
	if !ok {
		return false
	}
	if _, exists := st.ReposByName[newFull]; exists {
		return false
	}

	newOwnerUser := st.UsersByLogin[newOwner]
	var newOwnerOrg *Org
	if newOwnerUser == nil {
		newOwnerOrg = st.OrgsByLogin[newOwner]
	}
	if newOwnerUser == nil && newOwnerOrg == nil {
		return false
	}

	if GitDataDir() != "" {
		oldDir := filepath.Join(GitDataDir(), filepath.FromSlash(oldFull))
		newDir := filepath.Join(GitDataDir(), filepath.FromSlash(newFull))
		if err := os.Rename(oldDir, newDir); err != nil && os.IsExist(err) {
			log.Printf("bleephub: transfer repo %s -> %s: move git dir: %v", oldFull, newFull, err)
			return false
		}
	}
	if IsS3GitStorage() {
		s3fs, err := getS3FS(context.Background())
		if err != nil {
			log.Printf("bleephub: transfer repo %s -> %s: resolve s3 fs: %v", oldFull, newFull, err)
			return false
		}
		if s3fs != nil {
			if err := s3fs.renameRepoPrefix(oldFull, newFull); err != nil {
				log.Printf("bleephub: transfer repo %s -> %s: move s3 prefix: %v", oldFull, newFull, err)
				return false
			}
		}
	}

	repo.FullName = newFull
	repo.UpdatedAt = time.Now().UTC()
	if newOwnerOrg != nil {
		repo.OwnerType = "Organization"
		repo.OwnerID = newOwnerOrg.ID
		repo.Owner = nil
	} else if newOwnerUser != nil {
		repo.OwnerType = "User"
		repo.OwnerID = newOwnerUser.ID
		repo.Owner = newOwnerUser
	}

	st.ReposByName[newFull] = repo
	delete(st.ReposByName, oldFull)

	if stor := st.GitStorages[oldFull]; stor != nil {
		st.GitStorages[newFull] = stor
		delete(st.GitStorages, oldFull)
	}

	st.moveRepoKeyLocked(oldFull, newFull)

	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// moveRepoKeyLocked renames all in-memory maps keyed by repo full name from
// oldFull to newFull. Caller must hold st.mu.
func (st *Store) moveRepoKeyLocked(oldFull, newFull string) {
	if v := st.RepoSecrets[oldFull]; v != nil {
		st.RepoSecrets[newFull] = v
		delete(st.RepoSecrets, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_secrets", newFull, v)
			st.persist.MustDelete("repo_secrets", oldFull)
		}
	}
	if v := st.RepoVariables[oldFull]; v != nil {
		st.RepoVariables[newFull] = v
		delete(st.RepoVariables, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_variables", newFull, v)
			st.persist.MustDelete("repo_variables", oldFull)
		}
	}
	if v := st.RepoCollaborators[oldFull]; v != nil {
		st.RepoCollaborators[newFull] = v
		delete(st.RepoCollaborators, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_collaborators", newFull, v)
			st.persist.MustDelete("repo_collaborators", oldFull)
		}
	}
	if v := st.Hooks[oldFull]; v != nil {
		st.Hooks[newFull] = v
		delete(st.Hooks, oldFull)
		if st.persist != nil {
			st.persist.MustPut("hooks", newFull, v)
			st.persist.MustDelete("hooks", oldFull)
		}
	}
	if v := st.CheckSuitePrefs[oldFull]; v != nil {
		st.CheckSuitePrefs[newFull] = v
		delete(st.CheckSuitePrefs, oldFull)
		if st.persist != nil {
			st.persist.MustPut("check_suite_prefs", newFull, v)
			st.persist.MustDelete("check_suite_prefs", oldFull)
		}
	}
	for _, suite := range st.CheckSuites {
		if suite.RepoKey == oldFull {
			suite.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("check_suites", strconv.FormatInt(suite.ID, 10), suite)
			}
		}
	}
	for _, run := range st.CheckRuns {
		if run.RepoKey == oldFull {
			run.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("check_runs", strconv.FormatInt(run.ID, 10), run)
			}
		}
	}
	st.CommitStatuses.moveRepoKey(oldFull, newFull)
	if v := st.RepoAutolinks[oldFull]; v != nil {
		for _, a := range v {
			a.RepoKey = newFull
		}
		st.RepoAutolinks[newFull] = v
		delete(st.RepoAutolinks, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_autolinks", newFull, v)
			st.persist.MustDelete("repo_autolinks", oldFull)
		}
	}
	if v := st.RepoInvitations[oldFull]; v != nil {
		for _, inv := range v {
			inv.RepoKey = newFull
		}
		st.RepoInvitations[newFull] = v
		delete(st.RepoInvitations, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_invitations", newFull, v)
			st.persist.MustDelete("repo_invitations", oldFull)
		}
	}
	if v := st.RepoDeployKeys[oldFull]; v != nil {
		st.RepoDeployKeys[newFull] = v
		delete(st.RepoDeployKeys, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_deploy_keys", newFull, v)
			st.persist.MustDelete("repo_deploy_keys", oldFull)
		}
	}
	if v := st.SecretScanningAlertsByRepo[oldFull]; v != nil {
		st.SecretScanningAlertsByRepo[newFull] = v
		for _, alert := range v {
			alert.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("secret_scanning_alerts", strconv.Itoa(alert.ID), alert)
			}
		}
		delete(st.SecretScanningAlertsByRepo, oldFull)
		if st.persist != nil {
			st.persist.MustDelete("secret_scanning_alerts", oldFull)
		}
	}
	if v := st.SecretScanningNextNumber[oldFull]; v != 0 {
		st.SecretScanningNextNumber[newFull] = v
		delete(st.SecretScanningNextNumber, oldFull)
	}
	if v := st.CodeScanningAlertsByRepo[oldFull]; v != nil {
		st.CodeScanningAlertsByRepo[newFull] = v
		for _, alert := range v {
			alert.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("code_scanning_alerts", strconv.Itoa(alert.ID), alert)
			}
		}
		delete(st.CodeScanningAlertsByRepo, oldFull)
		if st.persist != nil {
			st.persist.MustDelete("code_scanning_alerts", oldFull)
		}
	}
	if v := st.CodeScanningNextNumber[oldFull]; v != 0 {
		st.CodeScanningNextNumber[newFull] = v
		delete(st.CodeScanningNextNumber, oldFull)
	}
	if v := st.CodeScanningAnalysesByRepo[oldFull]; v != nil {
		st.CodeScanningAnalysesByRepo[newFull] = v
		for _, a := range v {
			a.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("code_scanning_analyses", strconv.Itoa(a.ID), a)
			}
		}
		delete(st.CodeScanningAnalysesByRepo, oldFull)
		if st.persist != nil {
			st.persist.MustDelete("code_scanning_analyses", oldFull)
		}
	}
	if v := st.DependabotAlertsByRepo[oldFull]; v != nil {
		st.DependabotAlertsByRepo[newFull] = v
		for _, alert := range v {
			alert.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("dependabot_alerts", strconv.Itoa(alert.ID), alert)
			}
		}
		delete(st.DependabotAlertsByRepo, oldFull)
		if st.persist != nil {
			st.persist.MustDelete("dependabot_alerts", oldFull)
		}
	}
	if v := st.DependabotNextNumber[oldFull]; v != 0 {
		st.DependabotNextNumber[newFull] = v
		delete(st.DependabotNextNumber, oldFull)
	}
	if v := st.DependabotSecrets[oldFull]; v != nil {
		st.DependabotSecrets[newFull] = v
		delete(st.DependabotSecrets, oldFull)
		if st.persist != nil {
			st.persist.MustPut("dependabot_secrets", newFull, v)
			st.persist.MustDelete("dependabot_secrets", oldFull)
		}
	}
	if v := st.CodeScanningDefaultSetups[oldFull]; v != nil {
		v.RepoKey = newFull
		st.CodeScanningDefaultSetups[newFull] = v
		delete(st.CodeScanningDefaultSetups, oldFull)
		if st.persist != nil {
			st.persist.MustPut("code_scanning_default_setups", newFull, v)
			st.persist.MustDelete("code_scanning_default_setups", oldFull)
		}
	}
	if v := st.CodeQualitySetups[oldFull]; v != nil {
		v.RepoFullName = newFull
		st.CodeQualitySetups[newFull] = v
		delete(st.CodeQualitySetups, oldFull)
		if st.persist != nil {
			st.persist.MustPut("code_quality_setups", newFull, v)
			st.persist.MustDelete("code_quality_setups", oldFull)
		}
	}
	if v := st.RepoCustomPropertyValues[oldFull]; v != nil {
		st.RepoCustomPropertyValues[newFull] = v
		delete(st.RepoCustomPropertyValues, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_custom_property_values", newFull, v)
			st.persist.MustDelete("repo_custom_property_values", oldFull)
		}
	}
	if enabled, ok := st.RepoImmutableReleases[oldFull]; ok {
		st.RepoImmutableReleases[newFull] = enabled
		delete(st.RepoImmutableReleases, oldFull)
		if st.persist != nil {
			st.persist.MustPut("repo_immutable_releases", newFull, enabled)
			st.persist.MustDelete("repo_immutable_releases", oldFull)
		}
	}
	if v := st.AgentsRepoSecrets[oldFull]; v != nil {
		st.AgentsRepoSecrets[newFull] = v
		delete(st.AgentsRepoSecrets, oldFull)
		if st.persist != nil {
			st.persist.MustPut("agents_repo_secrets", newFull, v)
			st.persist.MustDelete("agents_repo_secrets", oldFull)
		}
	}
	if v := st.AgentsRepoVariables[oldFull]; v != nil {
		st.AgentsRepoVariables[newFull] = v
		delete(st.AgentsRepoVariables, oldFull)
		if st.persist != nil {
			st.persist.MustPut("agents_repo_variables", newFull, v)
			st.persist.MustDelete("agents_repo_variables", oldFull)
		}
	}
	if v := st.SecretScanningPushPlaceholders[oldFull]; v != nil {
		for _, ph := range v {
			ph.RepoKey = newFull
		}
		st.SecretScanningPushPlaceholders[newFull] = v
		delete(st.SecretScanningPushPlaceholders, oldFull)
		if st.persist != nil {
			st.persist.MustPut("secret_scanning_push_placeholders", newFull, v)
			st.persist.MustDelete("secret_scanning_push_placeholders", oldFull)
		}
	}
	if v := st.SecretScanningPushBypasses[oldFull]; v != nil {
		for _, bypass := range v {
			bypass.RepoKey = newFull
		}
		st.SecretScanningPushBypasses[newFull] = v
		delete(st.SecretScanningPushBypasses, oldFull)
		if st.persist != nil {
			st.persist.MustPut("secret_scanning_push_bypasses", newFull, v)
			st.persist.MustDelete("secret_scanning_push_bypasses", oldFull)
		}
	}

	if v := st.SecurityAdvisoriesByRepo[oldFull]; v != nil {
		st.SecurityAdvisoriesByRepo[newFull] = v
		for _, a := range v {
			if st.persist != nil {
				st.persist.MustPut("security_advisories", strconv.Itoa(a.ID), a)
			}
		}
		delete(st.SecurityAdvisoriesByRepo, oldFull)
		if st.persist != nil {
			st.persist.MustDelete("security_advisories", oldFull)
		}
	}
	for key, fix := range st.CodeScanningAutofixes {
		if fix.RepoKey == oldFull {
			newKey := autofixKey(newFull, fix.AlertNumber)
			fix.RepoKey = newFull
			st.CodeScanningAutofixes[newKey] = fix
			delete(st.CodeScanningAutofixes, key)
			if st.persist != nil {
				st.persist.MustPut("code_scanning_autofixes", newKey, fix)
				st.persist.MustDelete("code_scanning_autofixes", key)
			}
		}
	}
	for _, up := range st.SARIFUploads {
		if up.RepoKey == oldFull {
			up.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("sarif_uploads", up.ID, up)
			}
		}
	}
	if v := st.CodeQLDatabasesByRepo[oldFull]; v != nil {
		st.CodeQLDatabasesByRepo[newFull] = v
		for _, db := range v {
			db.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("codeql_databases", strconv.Itoa(db.ID), db)
			}
		}
		delete(st.CodeQLDatabasesByRepo, oldFull)
	}
	for _, va := range st.CodeQLVariantAnalyses {
		changed := false
		if va.ControllerRepoKey == oldFull {
			va.ControllerRepoKey = newFull
			changed = true
		}
		for i := range va.ScannedRepositories {
			if va.ScannedRepositories[i].FullName == oldFull {
				va.ScannedRepositories[i].FullName = newFull
				changed = true
			}
		}
		for i, name := range va.NotFoundRepos {
			if name == oldFull {
				va.NotFoundRepos[i] = newFull
				changed = true
			}
		}
		if changed && st.persist != nil {
			st.persist.MustPut("codeql_variant_analyses", strconv.Itoa(va.ID), va)
		}
	}
	for _, rs := range st.Rulesets {
		if rs.RepoID == 0 || rs.Source != oldFull {
			continue
		}
		rs.Source = newFull
		if st.persist != nil {
			st.persist.MustPut("repo_rulesets", strconv.Itoa(rs.ID), rs)
		}
	}
	for _, project := range st.ProjectClassic {
		if project.RepoKey == oldFull {
			project.RepoKey = newFull
			if st.persist != nil {
				st.persist.MustPut("projects_classic", strconv.Itoa(project.ID), project)
			}
		}
	}
	for _, cs := range st.Codespaces {
		if cs.RepoKey == oldFull {
			cs.RepoKey = newFull
			cs.UpdatedAt = time.Now().UTC()
			if st.persist != nil {
				st.persist.MustPut("codespaces", strconv.Itoa(cs.ID), cs)
			}
		}
	}
	if pkgs := st.PackagesByOwnerKey[oldFull]; len(pkgs) > 0 {
		st.PackagesByOwnerKey[newFull] = pkgs
		delete(st.PackagesByOwnerKey, oldFull)
		for _, pkg := range pkgs {
			pkg.OwnerKey = newFull
			pkg.UpdatedAt = time.Now().UTC()
			if st.persist != nil {
				st.persist.MustPut("packages", strconv.Itoa(pkg.ID), pkg)
			}
		}
	}

	for k, v := range st.EnvSecrets {
		repoKey, envName, found := strings.Cut(k, "\x1f")
		if found && repoKey == oldFull {
			newKey := newFull + "\x1f" + envName
			st.EnvSecrets[newKey] = v
			delete(st.EnvSecrets, k)
			if st.persist != nil {
				st.persist.MustPut("env_secrets", newKey, v)
				st.persist.MustDelete("env_secrets", k)
			}
		}
	}
	for k, v := range st.EnvVariables {
		repoKey, envName, found := strings.Cut(k, "\x1f")
		if found && repoKey == oldFull {
			newKey := newFull + "\x1f" + envName
			st.EnvVariables[newKey] = v
			delete(st.EnvVariables, k)
			if st.persist != nil {
				st.persist.MustPut("env_variables", newKey, v)
				st.persist.MustDelete("env_variables", k)
			}
		}
	}

	for _, wf := range st.Workflows {
		if wf.RepoFullName == oldFull {
			wf.RepoFullName = newFull
		}
	}
	for _, wf := range st.WorkflowFiles {
		if wf.RepoFullName == oldFull {
			wf.RepoFullName = newFull
			if st.persist != nil {
				st.persist.MustPut("workflow_files", strconv.FormatInt(wf.ID, 10), wf)
			}
		}
	}

	st.Misc.mu.Lock()
	if v := st.Misc.pagesBuilds[oldFull]; v != nil {
		st.Misc.pagesBuilds[newFull] = v
		delete(st.Misc.pagesBuilds, oldFull)
		if st.persist != nil {
			st.persist.MustPut("pages_builds", newFull, v)
			st.persist.MustDelete("pages_builds", oldFull)
		}
	}
	st.Misc.mu.Unlock()
}

// RenameBranch renames a git branch in a repository.
func (st *Store) RenameBranch(repoID int, branch, newName string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return false
	}
	owner, name, ok := splitRepoFullName(repo.FullName)
	if !ok {
		return false
	}
	stor := st.GitStorages[owner+"/"+name]
	if stor == nil {
		return false
	}

	oldRef := plumbing.NewBranchReferenceName(branch)
	newRef := plumbing.NewBranchReferenceName(newName)
	ref, err := stor.Reference(oldRef)
	if err != nil {
		return false
	}
	if _, err := stor.Reference(newRef); err == nil {
		return false
	}
	if err := stor.SetReference(plumbing.NewHashReference(newRef, ref.Hash())); err != nil {
		return false
	}
	if err := stor.RemoveReference(oldRef); err != nil {
		return false
	}
	if repo.DefaultBranch == branch {
		repo.DefaultBranch = newName
	}
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// SetRepoFlag sets a boolean flag field on a repo by name.
func (st *Store) SetRepoFlag(repoID int, field string, value bool) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return false
	}
	switch field {
	case "automated_security_fixes_enabled":
		repo.AutomatedSecurityFixesEnabled = value
	case "private_vulnerability_reporting_enabled":
		repo.PrivateVulnerabilityReportingEnabled = value
	case "vulnerability_alerts_enabled":
		repo.VulnerabilityAlertsEnabled = value
	default:
		return false
	}
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}

// SetRepoInteractionLimit sets the interaction limit for a repo.
func (st *Store) SetRepoInteractionLimit(repoID int, limit string, expiry *time.Time) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo := st.Repos[repoID]
	if repo == nil {
		return false
	}
	repo.InteractionLimit = limit
	repo.InteractionLimitExpiry = expiry
	repo.UpdatedAt = time.Now().UTC()
	if st.persist != nil {
		st.persist.MustPut("repos", strconv.Itoa(repo.ID), repo)
	}
	return true
}
