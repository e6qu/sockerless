package bleephub

import (
	"fmt"
	"sync"
	"time"
)

// ProjectsV2 — minimum-viable GitHub Projects v2 store. Real GitHub's
// ProjectV2 has a rich schema (fields, iterations, automations); this
// implementation covers what `gh project create`, `gh project item-add`,
// and `gh issue view --json projectItems` actually exercise.

// ProjectV2 is a Projects v2 project. Per real GH: each project belongs
// to a user or organization (the owner) and has a stable per-owner
// `number` plus a globally unique `nodeID`.
type ProjectV2 struct {
	ID        int
	NodeID    string
	Number    int    // per-owner sequential
	OwnerID   int    // user/org ID
	OwnerType string // "User" or "Organization"
	Title     string
	Closed    bool
	Public    bool
	URL       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProjectV2Item links an issue or PR (or a draft issue) to a project.
// ContentType is "Issue", "PullRequest", or "DraftIssue".
type ProjectV2Item struct {
	ID          int
	NodeID      string
	ProjectID   int
	ContentType string
	ContentID   int // 0 for DraftIssue
	DraftTitle  string
	DraftBody   string
	CreatedAt   time.Time
}

// ProjectV2Store is the in-memory store. Concurrency-safe via mu.
type ProjectV2Store struct {
	mu            sync.RWMutex
	projects      map[int]*ProjectV2
	items         map[int]*ProjectV2Item
	itemsByOwner  map[int][]*ProjectV2Item // contentID → items it appears in
	nextProjectID int
	nextItemID    int
}

func newProjectV2Store() *ProjectV2Store {
	return &ProjectV2Store{
		projects:      map[int]*ProjectV2{},
		items:         map[int]*ProjectV2Item{},
		itemsByOwner:  map[int][]*ProjectV2Item{},
		nextProjectID: 1,
		nextItemID:    1,
	}
}

// CreateProject creates a new ProjectV2 owned by the given user or org.
func (s *ProjectV2Store) CreateProject(ownerID int, ownerType, title string) *ProjectV2 {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextProjectID
	s.nextProjectID++
	// Per-owner sequential number.
	number := 1
	for _, p := range s.projects {
		if p.OwnerID == ownerID && p.OwnerType == ownerType && p.Number >= number {
			number = p.Number + 1
		}
	}
	now := time.Now()
	p := &ProjectV2{
		ID:        id,
		NodeID:    fmt.Sprintf("PVT_kgDO%08d", id),
		Number:    number,
		OwnerID:   ownerID,
		OwnerType: ownerType,
		Title:     title,
		Public:    false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.projects[id] = p
	return p
}

// GetProject returns a project by ID or nil.
func (s *ProjectV2Store) GetProject(id int) *ProjectV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projects[id]
}

// LookupProjectByNodeID returns the project with the given global node id.
func (s *ProjectV2Store) LookupProjectByNodeID(nodeID string) *ProjectV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.NodeID == nodeID {
			return p
		}
	}
	return nil
}

// AddItem adds an Issue or PullRequest to the given project. contentID is
// the issue or PR database ID; contentType is "Issue" or "PullRequest".
func (s *ProjectV2Store) AddItem(projectID int, contentType string, contentID int) *ProjectV2Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return nil
	}
	// Avoid duplicate item for the same (project, content).
	for _, it := range s.itemsByOwner[contentID] {
		if it.ProjectID == projectID && it.ContentType == contentType {
			return it
		}
	}
	id := s.nextItemID
	s.nextItemID++
	it := &ProjectV2Item{
		ID:          id,
		NodeID:      fmt.Sprintf("PVTI_kgDO%08d", id),
		ProjectID:   projectID,
		ContentType: contentType,
		ContentID:   contentID,
		CreatedAt:   time.Now(),
	}
	s.items[id] = it
	s.itemsByOwner[contentID] = append(s.itemsByOwner[contentID], it)
	return it
}

// ListItemsForIssue returns every project item that wraps the issue with
// the given database ID. Used by Issue.projectItems GraphQL resolver.
func (s *ProjectV2Store) ListItemsForIssue(issueID int) []*ProjectV2Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2Item, 0)
	for _, it := range s.itemsByOwner[issueID] {
		if it.ContentType == "Issue" {
			out = append(out, it)
		}
	}
	return out
}

// ListItemsForPR returns every project item that wraps the PR with the
// given database ID.
func (s *ProjectV2Store) ListItemsForPR(prID int) []*ProjectV2Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2Item, 0)
	for _, it := range s.itemsByOwner[prID] {
		if it.ContentType == "PullRequest" {
			out = append(out, it)
		}
	}
	return out
}
