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

	gitStorage "github.com/go-git/go-git/v5/storage"
)

type Repo struct {
	ID                        int       `json:"id"`
	NodeID                    string    `json:"node_id"`
	Name                      string    `json:"name"`
	FullName                  string    `json:"full_name"`
	Description               string    `json:"description"`
	Homepage                  string    `json:"homepage"`
	DefaultBranch             string    `json:"default_branch"`
	Visibility                string    `json:"visibility"`
	Language                  string    `json:"language"`
	Owner                     *User     `json:"-"`
	OwnerID                   int       `json:"owner_id"`   // serialized so Owner can be relinked on reload
	OwnerType                 string    `json:"owner_type"` // "User" or "Organization"; empty means User for backwards compatibility
	Private                   bool      `json:"private"`
	Fork                      bool      `json:"fork"`
	Archived                  bool      `json:"archived"`
	IsTemplate                bool      `json:"is_template"`
	WebCommitSignoffRequired  bool      `json:"web_commit_signoff_required"`
	HasIssues                 bool      `json:"has_issues"`
	HasProjects               bool      `json:"has_projects"`
	HasWiki                   bool      `json:"has_wiki"`
	HasPullRequests           bool      `json:"has_pull_requests"`
	AllowSquashMerge          bool      `json:"allow_squash_merge"`
	AllowMergeCommit          bool      `json:"allow_merge_commit"`
	AllowRebaseMerge          bool      `json:"allow_rebase_merge"`
	AllowAutoMerge            bool      `json:"allow_auto_merge"`
	AllowUpdateBranch         bool      `json:"allow_update_branch"`
	DeleteBranchOnMerge       bool      `json:"delete_branch_on_merge"`
	UseSquashPRTitleAsDefault bool      `json:"use_squash_pr_title_as_default"`
	SquashMergeCommitTitle    string    `json:"squash_merge_commit_title"`
	SquashMergeCommitMessage  string    `json:"squash_merge_commit_message"`
	MergeCommitTitle          string    `json:"merge_commit_title"`
	MergeCommitMessage        string    `json:"merge_commit_message"`
	PullRequestCreationPolicy string    `json:"pull_request_creation_policy"`
	LicenseKey                string    `json:"license_key"`
	LicenseName               string    `json:"license_name"`
	LicenseSPDX               string    `json:"license_spdx"`
	StargazersCount           int       `json:"stargazers_count"`
	Topics                    []string  `json:"topics"`
	NextIssueNumber           int       `json:"-"`
	NextMilestoneNumber       int       `json:"-"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
	PushedAt                  time.Time `json:"pushed_at"`
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
		HasPullRequests:           true,
		AllowSquashMerge:          true,
		AllowMergeCommit:          true,
		AllowRebaseMerge:          true,
		PullRequestCreationPolicy: "all",
		Topics:                    []string{},
		NextIssueNumber:           1,
		NextMilestoneNumber:       1,
		CreatedAt:                 now,
		UpdatedAt:                 now,
		PushedAt:                  now,
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
	delete(st.CheckSuitePrefs, fullName)
	if st.persist != nil {
		st.persist.MustDelete("hooks", fullName)
		st.persist.MustDelete("repo_secrets", fullName)
		st.persist.MustDelete("check_suite_prefs", fullName)
	}
	for id, issue := range st.Issues {
		if issue.RepoID == repo.ID {
			delete(st.Issues, id)
			if st.persist != nil {
				st.persist.MustDelete("issues", strconv.Itoa(id))
			}
		}
	}
	for id, pr := range st.PullRequests {
		if pr.RepoID == repo.ID {
			delete(st.PullRequests, id)
			if st.persist != nil {
				st.persist.MustDelete("pull_requests", strconv.Itoa(id))
			}
		}
	}
	st.Releases.DeleteAllForRepo(repo.ID)

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
	// a collaborator. bleephub does not model per-repo collaborators yet, so
	// this set is empty.
	_ = includeCollab

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
