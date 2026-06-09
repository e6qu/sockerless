package bleephub

import (
	"context"
	"fmt"
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

	now := time.Now()
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
		Private:             private,
		Topics:              []string{},
		NextIssueNumber:     1,
		NextMilestoneNumber: 1,
		CreatedAt:           now,
		UpdatedAt:           now,
		PushedAt:            now,
	}
	st.NextRepo++

	st.Repos[repo.ID] = repo
	st.ReposByName[fullName] = repo

	stor, err := openOrInitGitStorage(context.Background(), fullName)
	if err != nil {
		return nil
	}
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
	repo.UpdatedAt = time.Now()
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

	gitDir := GitDataDir()
	if gitDir != "" {
		repoDir := filepath.Join(gitDir, filepath.FromSlash(fullName))
		_ = os.RemoveAll(repoDir)
	}
	if IsS3GitStorage() {
		s3fs, err := getS3FS(context.Background())
		if err == nil && s3fs != nil {
			s3fs.deleteRepoPrefix(fullName)
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
