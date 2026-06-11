package bleephub

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gitStorage "github.com/go-git/go-git/v5/storage"
)

type Repo struct {
	ID                  int       `json:"id"`
	NodeID              string    `json:"node_id"`
	Name                string    `json:"name"`
	FullName            string    `json:"full_name"`
	Description         string    `json:"description"`
	DefaultBranch       string    `json:"default_branch"`
	Visibility          string    `json:"visibility"`
	Language            string    `json:"language"`
	Owner               *User     `json:"-"`
	OwnerID             int       `json:"owner_id"` // serialized so Owner can be relinked on reload
	Private             bool      `json:"private"`
	Fork                bool      `json:"fork"`
	Archived            bool      `json:"archived"`
	StargazersCount     int       `json:"stargazers_count"`
	Topics              []string  `json:"topics"`
	NextIssueNumber     int       `json:"-"`
	NextMilestoneNumber int       `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	PushedAt            time.Time `json:"pushed_at"`
}

func (st *Store) CreateRepo(owner *User, name, description string, private bool) *Repo {
	st.mu.Lock()
	defer st.mu.Unlock()

	fullName := owner.Login + "/" + name
	if _, exists := st.ReposByName[fullName]; exists {
		return nil
	}

	now := time.Now().UTC()
	visibility := "public"
	if private {
		visibility = "private"
	}

	repo := &Repo{
		ID:                  st.NextRepo,
		NodeID:              fmt.Sprintf("R_kgDO%08d", st.NextRepo),
		Name:                name,
		FullName:            fullName,
		Description:         description,
		DefaultBranch:       "main",
		Visibility:          visibility,
		Owner:               owner,
		OwnerID:             owner.ID,
		Private:             private,
		Topics:              []string{},
		NextIssueNumber:     1,
		NextMilestoneNumber: 1,
		CreatedAt:           now,
		UpdatedAt:           now,
		PushedAt:            now,
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

func (st *Store) GetGitStorage(owner, name string) gitStorage.Storer {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.GitStorages[owner+"/"+name]
}
