package bleephub

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// DiscussionCategory is a repository discussion category.
type DiscussionCategory struct {
	ID           int       `json:"id"`
	NodeID       string    `json:"node_id"`
	RepoID       int       `json:"repo_id"`
	Name         string    `json:"name"`
	Emoji        string    `json:"emoji"`
	Description  string    `json:"description"`
	IsAnswerable bool      `json:"is_answerable"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Discussion is a repository discussion.
type Discussion struct {
	ID           int        `json:"id"`
	NodeID       string     `json:"node_id"`
	RepoID       int        `json:"repo_id"`
	CategoryID   int        `json:"category_id"`
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	AuthorID     int        `json:"author_id"`
	Locked       bool       `json:"locked"`
	LockedReason string     `json:"locked_reason"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastEditedAt *time.Time `json:"last_edited_at"`
	PublishedAt  *time.Time `json:"published_at"`
	Deleted      bool       `json:"deleted"`
}

// DiscussionComment is a comment on a discussion (top-level or reply).
type DiscussionComment struct {
	ID           int        `json:"id"`
	NodeID       string     `json:"node_id"`
	DiscussionID int        `json:"discussion_id"`
	AuthorID     int        `json:"author_id"`
	Body         string     `json:"body"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastEditedAt *time.Time `json:"last_edited_at"`
	IsAnswer     bool       `json:"is_answer"`
	ParentID     int        `json:"parent_id"`
	Deleted      bool       `json:"deleted"`
}

func discussionCategoryNodeID(id int) string {
	return fmt.Sprintf("DGC_kgDO%08d", id)
}

func discussionNodeID(id int) string {
	return fmt.Sprintf("D_kgDO%08d", id)
}

func discussionCommentNodeID(id int) string {
	return fmt.Sprintf("DC_kgDO%08d", id)
}

// ensureDefaultDiscussionCategoriesLocked creates default categories while the
// caller already holds st.mu.
func (st *Store) ensureDefaultDiscussionCategoriesLocked(repoID int) {
	defaults := []struct {
		name         string
		emoji        string
		description  string
		isAnswerable bool
	}{
		{"General", ":speech_balloon:", "Chat about anything and everything here", false},
		{"Ideas", ":bulb:", "Share ideas for new features or improvements", false},
		{"Q&A", ":question:", "Ask the community for help", true},
		{"Show and tell", ":raised_hands:", "Show off something you've made", false},
		{"Polls", ":bar_chart:", "Take a vote from the community", false},
	}
	for _, d := range defaults {
		st.createDiscussionCategoryLocked(repoID, d.name, d.emoji, d.description, d.isAnswerable)
	}
}

// CreateDiscussionCategory creates a new discussion category in the given repository.
func (st *Store) CreateDiscussionCategory(repoID int, name, emoji, description string, isAnswerable bool) *DiscussionCategory {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.createDiscussionCategoryLocked(repoID, name, emoji, description, isAnswerable)
}

// createDiscussionCategoryLocked creates a category while the caller already holds st.mu.
func (st *Store) createDiscussionCategoryLocked(repoID int, name, emoji, description string, isAnswerable bool) *DiscussionCategory {
	now := time.Now().UTC()
	cat := &DiscussionCategory{
		ID:           st.NextDiscussionCategoryID,
		NodeID:       discussionCategoryNodeID(st.NextDiscussionCategoryID),
		RepoID:       repoID,
		Name:         name,
		Emoji:        emoji,
		Description:  description,
		IsAnswerable: isAnswerable,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.DiscussionCategories[cat.ID] = cat
	st.NextDiscussionCategoryID++
	st.persistDiscussionCategory(cat)
	return cat
}

// GetDiscussionCategory returns a category by global ID.
func (st *Store) GetDiscussionCategory(id int) *DiscussionCategory {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.DiscussionCategories[id]
}

// GetDiscussionCategoryByName returns a category by repo and name.
func (st *Store) GetDiscussionCategoryByName(repoID int, name string) *DiscussionCategory {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, cat := range st.DiscussionCategories {
		if cat.RepoID == repoID && cat.Name == name {
			return cat
		}
	}
	return nil
}

// ListDiscussionCategories returns all categories for a repository.
func (st *Store) ListDiscussionCategories(repoID int) []*DiscussionCategory {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*DiscussionCategory
	for _, cat := range st.DiscussionCategories {
		if cat.RepoID == repoID {
			out = append(out, cat)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// nextDiscussionNumber returns the next per-repo discussion number.
func (st *Store) nextDiscussionNumber(repoID int) int {
	max := 0
	for _, d := range st.Discussions {
		if d.RepoID == repoID && d.Number > max {
			max = d.Number
		}
	}
	return max + 1
}

// CreateDiscussion creates a new discussion in the given repository.
func (st *Store) CreateDiscussion(repoID, categoryID, authorID int, title, body string) *Discussion {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	d := &Discussion{
		ID:          st.NextDiscussionID,
		NodeID:      discussionNodeID(st.NextDiscussionID),
		RepoID:      repoID,
		CategoryID:  categoryID,
		Number:      st.nextDiscussionNumber(repoID),
		Title:       title,
		Body:        body,
		AuthorID:    authorID,
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishedAt: &now,
	}
	st.Discussions[d.ID] = d
	st.NextDiscussionID++
	st.persistDiscussion(d)
	return d
}

// GetDiscussion returns a discussion by global ID.
func (st *Store) GetDiscussion(id int) *Discussion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.Discussions[id]
}

// GetDiscussionByNumber returns a discussion by repo and number.
func (st *Store) GetDiscussionByNumber(repoID, number int) *Discussion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	for _, d := range st.Discussions {
		if d.RepoID == repoID && d.Number == number && !d.Deleted {
			return d
		}
	}
	return nil
}

// ListDiscussions returns discussions for a repository, optionally filtered by category.
func (st *Store) ListDiscussions(repoID, categoryID int) []*Discussion {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*Discussion
	for _, d := range st.Discussions {
		if d.RepoID != repoID || d.Deleted {
			continue
		}
		if categoryID != 0 && d.CategoryID != categoryID {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// UpdateDiscussion applies a mutation function to a discussion.
func (st *Store) UpdateDiscussion(id int, fn func(*Discussion)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	d, ok := st.Discussions[id]
	if !ok || d.Deleted {
		return false
	}
	fn(d)
	now := time.Now().UTC()
	if d.LastEditedAt == nil {
		d.LastEditedAt = &now
	} else {
		*d.LastEditedAt = now
	}
	d.UpdatedAt = now
	st.persistDiscussion(d)
	return true
}

// DeleteDiscussion soft-deletes a discussion.
func (st *Store) DeleteDiscussion(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	d, ok := st.Discussions[id]
	if !ok || d.Deleted {
		return false
	}
	d.Deleted = true
	d.UpdatedAt = time.Now().UTC()
	st.persistDiscussion(d)
	return true
}

// CreateDiscussionComment creates a new top-level comment or reply on a discussion.
func (st *Store) CreateDiscussionComment(discussionID, authorID int, body string, parentID int) *DiscussionComment {
	st.mu.Lock()
	defer st.mu.Unlock()

	now := time.Now().UTC()
	c := &DiscussionComment{
		ID:           st.NextDiscussionCommentID,
		NodeID:       discussionCommentNodeID(st.NextDiscussionCommentID),
		DiscussionID: discussionID,
		AuthorID:     authorID,
		Body:         body,
		CreatedAt:    now,
		UpdatedAt:    now,
		ParentID:     parentID,
	}
	st.DiscussionComments[c.ID] = c
	st.NextDiscussionCommentID++
	st.persistDiscussionComment(c)
	return c
}

// GetDiscussionComment returns a comment by global ID.
func (st *Store) GetDiscussionComment(id int) *DiscussionComment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.DiscussionComments[id]
}

// ListDiscussionComments returns comments for a discussion, optionally scoped to a parent.
func (st *Store) ListDiscussionComments(discussionID, parentID int) []*DiscussionComment {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*DiscussionComment
	for _, c := range st.DiscussionComments {
		if c.DiscussionID != discussionID || c.Deleted {
			continue
		}
		if c.ParentID != parentID {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// UpdateDiscussionComment applies a mutation function to a comment.
func (st *Store) UpdateDiscussionComment(id int, fn func(*DiscussionComment)) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.DiscussionComments[id]
	if !ok || c.Deleted {
		return false
	}
	fn(c)
	now := time.Now().UTC()
	if c.LastEditedAt == nil {
		c.LastEditedAt = &now
	} else {
		*c.LastEditedAt = now
	}
	c.UpdatedAt = now
	st.persistDiscussionComment(c)
	return true
}

// DeleteDiscussionComment soft-deletes a comment.
func (st *Store) DeleteDiscussionComment(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.DiscussionComments[id]
	if !ok || c.Deleted {
		return false
	}
	c.Deleted = true
	c.UpdatedAt = time.Now().UTC()
	st.persistDiscussionComment(c)
	return true
}

// MarkDiscussionCommentAsAnswer marks a comment as the answer, unmarking any other answer.
func (st *Store) MarkDiscussionCommentAsAnswer(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.DiscussionComments[id]
	if !ok || c.Deleted {
		return false
	}
	for _, other := range st.DiscussionComments {
		if other.DiscussionID == c.DiscussionID && other.IsAnswer {
			other.IsAnswer = false
			other.UpdatedAt = time.Now().UTC()
			st.persistDiscussionComment(other)
		}
	}
	c.IsAnswer = true
	c.UpdatedAt = time.Now().UTC()
	st.persistDiscussionComment(c)
	return true
}

// UnmarkDiscussionCommentAsAnswer unmarks a comment as the answer.
func (st *Store) UnmarkDiscussionCommentAsAnswer(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.DiscussionComments[id]
	if !ok || c.Deleted || !c.IsAnswer {
		return false
	}
	c.IsAnswer = false
	c.UpdatedAt = time.Now().UTC()
	st.persistDiscussionComment(c)
	return true
}

// --- Persistence helpers ---

func (st *Store) persistDiscussionCategory(cat *DiscussionCategory) {
	if st.persist != nil {
		st.persist.MustPut("discussion_categories", strconv.Itoa(cat.ID), cat)
	}
}

func (st *Store) persistDiscussion(d *Discussion) {
	if st.persist != nil {
		st.persist.MustPut("discussions", strconv.Itoa(d.ID), d)
	}
}

func (st *Store) persistDiscussionComment(c *DiscussionComment) {
	if st.persist != nil {
		st.persist.MustPut("discussion_comments", strconv.Itoa(c.ID), c)
	}
}
