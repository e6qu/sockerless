package bleephub

import (
	"fmt"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Repo represents a GitHub repository.
type Repo struct {
	ID              int       `json:"id"`
	NodeID          string    `json:"node_id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Description     string    `json:"description"`
	DefaultBranch   string    `json:"default_branch"`
	Visibility      string    `json:"visibility"`
	Language        string    `json:"language"`
	Owner           *User     `json:"-"`
	Private         bool      `json:"private"`
	Fork            bool      `json:"fork"`
	Archived        bool      `json:"archived"`
	StargazersCount     int       `json:"stargazers_count"`
	Topics              []string  `json:"topics"`
	NextIssueNumber     int       `json:"-"`
	NextMilestoneNumber int       `json:"-"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	PushedAt            time.Time `json:"pushed_at"`
}

// CreateRepo creates a new repository with an initialized bare git storage.
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

	// Initialize bare go-git in-memory storage
	storer := memory.NewStorage()
	st.GitStorages[fullName] = storer

	// Init bare repo
	_, _ = git.Init(storer, nil)

	return repo
}

// GetRepo returns a repository by owner login and name.
func (st *Store) GetRepo(owner, name string) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ReposByName[owner+"/"+name]
}

// UpdateRepo applies a mutation function to a repository.
func (st *Store) UpdateRepo(owner, name string, fn func(*Repo)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	repo, ok := st.ReposByName[owner+"/"+name]
	if !ok {
		return false
	}
	fn(repo)
	repo.UpdatedAt = time.Now()
	return true
}

// DeleteRepo removes a repository and its git storage.
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
	return true
}

// ListReposByOwner returns all repositories owned by the given login.
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

// GetGitStorage returns the go-git memory storage for a repository.
func (st *Store) GetGitStorage(owner, name string) *memory.Storage {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.GitStorages[owner+"/"+name]
}
