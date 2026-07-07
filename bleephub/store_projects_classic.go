package bleephub

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// ProjectClassic is a GitHub Projects classic (v1) project, scoped to a
// repository. It contains columns, which in turn contain cards.
type ProjectClassic struct {
	ID        int       `json:"id"`
	NodeID    string    `json:"node_id"`
	RepoKey   string    `json:"repo_key"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // "open" or "closed"
	Number    int       `json:"number"`
	CreatorID int       `json:"creator_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectColumn is a column inside a ProjectClassic.
type ProjectColumn struct {
	ID        int       `json:"id"`
	NodeID    string    `json:"node_id"`
	ProjectID int       `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Position  int64     `json:"-"` // ordering within the project
}

// ProjectCard is a card inside a ProjectColumn. It is either a note card
// (Note non-empty, IssueID 0) or an issue card (IssueID set, Note empty).
type ProjectCard struct {
	ID        int       `json:"id"`
	NodeID    string    `json:"node_id"`
	ColumnID  int       `json:"column_id"`
	Note      string    `json:"note"`
	IssueID   int       `json:"issue_id"`
	CreatorID int       `json:"creator_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Position  int64     `json:"-"` // ordering within the column
}

const (
	projectClassicPositionStep int64 = 1 << 40
)

func projectClassicNodeID(id int) string {
	return "MDExOlByb2plY3Q" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", id)))
}

func projectColumnNodeID(id int) string {
	return "MDEzOlByb2plY3RDb2x1bW4" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", id)))
}

func projectCardNodeID(id int) string {
	return "MDE1OlByb2plY3RDYXJk" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", id)))
}

// CreateProjectClassic creates a new repo-scoped project.
func (st *Store) CreateProjectClassic(repo *Repo, creatorID int, name, body, state string) *ProjectClassic {
	st.mu.Lock()
	defer st.mu.Unlock()

	if state == "" {
		state = "open"
	}
	now := time.Now().UTC()
	proj := &ProjectClassic{
		ID:        st.NextProjectClassicID,
		NodeID:    projectClassicNodeID(st.NextProjectClassicID),
		RepoKey:   repo.FullName,
		Name:      name,
		Body:      body,
		State:     state,
		Number:    st.NextProjectClassicID,
		CreatorID: creatorID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	st.ProjectClassic[proj.ID] = proj
	st.NextProjectClassicID++
	st.persistProjectClassic(proj)
	return proj
}

// GetProjectClassic returns a project by ID.
func (st *Store) GetProjectClassic(id int) *ProjectClassic {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ProjectClassic[id]
}

// ListProjectClassicsForRepo returns all projects in a repo, newest first.
func (st *Store) ListProjectClassicsForRepo(repoKey string) []*ProjectClassic {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*ProjectClassic
	for _, p := range st.ProjectClassic {
		if p.RepoKey == repoKey {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// UpdateProjectClassic applies updates to a project.
func (st *Store) UpdateProjectClassic(proj *ProjectClassic, name, body, state *string) *ProjectClassic {
	st.mu.Lock()
	defer st.mu.Unlock()
	if name != nil {
		proj.Name = *name
	}
	if body != nil {
		proj.Body = *body
	}
	if state != nil {
		proj.State = *state
	}
	proj.UpdatedAt = time.Now().UTC()
	st.persistProjectClassic(proj)
	return proj
}

// DeleteProjectClassic deletes a project and all its columns and cards.
func (st *Store) DeleteProjectClassic(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	proj := st.ProjectClassic[id]
	if proj == nil {
		return false
	}
	for _, col := range st.ProjectColumns {
		if col.ProjectID == id {
			for _, card := range st.ProjectCards {
				if card.ColumnID == col.ID {
					st.deleteProjectCardLocked(card.ID)
				}
			}
			st.deleteProjectColumnLocked(col.ID)
		}
	}
	delete(st.ProjectClassic, id)
	if st.persist != nil {
		st.persist.MustDelete("projects_classic", strconv.Itoa(id))
	}
	return true
}

// CreateProjectColumn creates a column in a project, appending it last.
func (st *Store) CreateProjectColumn(projectID int, name string) *ProjectColumn {
	st.mu.Lock()
	defer st.mu.Unlock()

	pos := projectClassicPositionStep
	for _, col := range st.ProjectColumns {
		if col.ProjectID == projectID && col.Position >= pos {
			pos = col.Position + projectClassicPositionStep
		}
	}

	now := time.Now().UTC()
	col := &ProjectColumn{
		ID:        st.NextProjectColumnID,
		NodeID:    projectColumnNodeID(st.NextProjectColumnID),
		ProjectID: projectID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		Position:  pos,
	}
	st.ProjectColumns[col.ID] = col
	st.NextProjectColumnID++
	st.persistProjectColumn(col)
	return col
}

// GetProjectColumn returns a column by ID.
func (st *Store) GetProjectColumn(id int) *ProjectColumn {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ProjectColumns[id]
}

// ListProjectColumns returns columns for a project in visual order.
func (st *Store) ListProjectColumns(projectID int) []*ProjectColumn {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*ProjectColumn
	for _, col := range st.ProjectColumns {
		if col.ProjectID == projectID {
			out = append(out, col)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// UpdateProjectColumn renames a column.
func (st *Store) UpdateProjectColumn(col *ProjectColumn, name string) *ProjectColumn {
	st.mu.Lock()
	defer st.mu.Unlock()
	col.Name = name
	col.UpdatedAt = time.Now().UTC()
	st.persistProjectColumn(col)
	return col
}

// DeleteProjectColumn deletes a column and all its cards.
func (st *Store) DeleteProjectColumn(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	col := st.ProjectColumns[id]
	if col == nil {
		return false
	}
	for _, card := range st.ProjectCards {
		if card.ColumnID == id {
			st.deleteProjectCardLocked(card.ID)
		}
	}
	st.deleteProjectColumnLocked(id)
	return true
}

func (st *Store) deleteProjectColumnLocked(id int) {
	delete(st.ProjectColumns, id)
	if st.persist != nil {
		st.persist.MustDelete("project_columns", strconv.Itoa(id))
	}
}

// MoveProjectColumn repositions a column within its project.
func (st *Store) MoveProjectColumn(col *ProjectColumn, position string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	cols := make([]*ProjectColumn, 0)
	for _, c := range st.ProjectColumns {
		if c.ProjectID == col.ProjectID {
			cols = append(cols, c)
		}
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].Position < cols[j].Position })

	switch position {
	case "first":
		min := int64(0)
		for _, c := range cols {
			if c.ID != col.ID && (min == 0 || c.Position < min) {
				min = c.Position
			}
		}
		col.Position = min - projectClassicPositionStep
	case "last":
		max := int64(0)
		for _, c := range cols {
			if c.ID != col.ID && c.Position > max {
				max = c.Position
			}
		}
		col.Position = max + projectClassicPositionStep
	default:
		var afterID int
		if _, err := fmt.Sscanf(position, "after:%d", &afterID); err != nil {
			return fmt.Errorf("invalid position")
		}
		var afterPos, nextPos int64
		found := false
		for i, c := range cols {
			if c.ID == afterID {
				afterPos = c.Position
				found = true
				if i+1 < len(cols) {
					nextPos = cols[i+1].Position
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("after card not found")
		}
		if nextPos == 0 {
			col.Position = afterPos + projectClassicPositionStep
		} else {
			col.Position = (afterPos + nextPos) / 2
		}
	}
	col.UpdatedAt = time.Now().UTC()
	st.persistProjectColumn(col)
	return nil
}

// CreateProjectCard creates a card in a column. Exactly one of note or
// issueID must be provided.
func (st *Store) CreateProjectCard(columnID, creatorID int, note string, issueID int) *ProjectCard {
	st.mu.Lock()
	defer st.mu.Unlock()

	pos := projectClassicPositionStep
	for _, card := range st.ProjectCards {
		if card.ColumnID == columnID && card.Position >= pos {
			pos = card.Position + projectClassicPositionStep
		}
	}

	now := time.Now().UTC()
	card := &ProjectCard{
		ID:        st.NextProjectCardID,
		NodeID:    projectCardNodeID(st.NextProjectCardID),
		ColumnID:  columnID,
		Note:      note,
		IssueID:   issueID,
		CreatorID: creatorID,
		CreatedAt: now,
		UpdatedAt: now,
		Position:  pos,
	}
	st.ProjectCards[card.ID] = card
	st.NextProjectCardID++
	st.persistProjectCard(card)
	return card
}

// GetProjectCard returns a card by ID.
func (st *Store) GetProjectCard(id int) *ProjectCard {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ProjectCards[id]
}

// ListProjectCards returns cards in a column in visual order.
func (st *Store) ListProjectCards(columnID int) []*ProjectCard {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*ProjectCard
	for _, card := range st.ProjectCards {
		if card.ColumnID == columnID {
			out = append(out, card)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}

// UpdateProjectCard updates a card's note. Converting a note card to an
// issue card is not supported by PATCH; real GitHub uses a separate flow.
func (st *Store) UpdateProjectCard(card *ProjectCard, note string) *ProjectCard {
	st.mu.Lock()
	defer st.mu.Unlock()
	card.Note = note
	card.UpdatedAt = time.Now().UTC()
	st.persistProjectCard(card)
	return card
}

// DeleteProjectCard deletes a card.
func (st *Store) DeleteProjectCard(id int) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.deleteProjectCardLocked(id)
}

func (st *Store) deleteProjectCardLocked(id int) bool {
	if st.ProjectCards[id] == nil {
		return false
	}
	delete(st.ProjectCards, id)
	if st.persist != nil {
		st.persist.MustDelete("project_cards", strconv.Itoa(id))
	}
	return true
}

// MoveProjectCard moves a card to a column and/or a new position within it.
func (st *Store) MoveProjectCard(card *ProjectCard, targetColumnID int, position string) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if targetColumnID != 0 && targetColumnID != card.ColumnID {
		if st.ProjectColumns[targetColumnID] == nil {
			return fmt.Errorf("target column not found")
		}
		card.ColumnID = targetColumnID
	}

	columnID := card.ColumnID
	cards := make([]*ProjectCard, 0)
	for _, c := range st.ProjectCards {
		if c.ColumnID == columnID && c.ID != card.ID {
			cards = append(cards, c)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Position < cards[j].Position })

	switch position {
	case "first":
		min := int64(0)
		for _, c := range cards {
			if min == 0 || c.Position < min {
				min = c.Position
			}
		}
		card.Position = min - projectClassicPositionStep
	case "last":
		max := int64(0)
		for _, c := range cards {
			if c.Position > max {
				max = c.Position
			}
		}
		card.Position = max + projectClassicPositionStep
	default:
		var afterID int
		if _, err := fmt.Sscanf(position, "after:%d", &afterID); err != nil {
			return fmt.Errorf("invalid position")
		}
		var afterPos, nextPos int64
		found := false
		for i, c := range cards {
			if c.ID == afterID {
				afterPos = c.Position
				found = true
				if i+1 < len(cards) {
					nextPos = cards[i+1].Position
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("after card not found")
		}
		if nextPos == 0 {
			card.Position = afterPos + projectClassicPositionStep
		} else {
			card.Position = (afterPos + nextPos) / 2
		}
	}
	card.UpdatedAt = time.Now().UTC()
	st.persistProjectCard(card)
	return nil
}

// ConvertProjectCardToIssue replaces a note card with an issue card in the
// same column/position, preserving the card ID.
func (st *Store) ConvertProjectCardToIssue(card *ProjectCard, issueID int) *ProjectCard {
	st.mu.Lock()
	defer st.mu.Unlock()
	card.Note = ""
	card.IssueID = issueID
	card.UpdatedAt = time.Now().UTC()
	st.persistProjectCard(card)
	return card
}

func (st *Store) persistProjectClassic(p *ProjectClassic) {
	if st.persist != nil {
		st.persist.MustPut("projects_classic", strconv.Itoa(p.ID), p)
	}
}

func (st *Store) persistProjectColumn(c *ProjectColumn) {
	if st.persist != nil {
		st.persist.MustPut("project_columns", strconv.Itoa(c.ID), c)
	}
}

func (st *Store) persistProjectCard(c *ProjectCard) {
	if st.persist != nil {
		st.persist.MustPut("project_cards", strconv.Itoa(c.ID), c)
	}
}
