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
	FieldValues map[int]*ProjectV2ItemFieldValue // fieldID → value
	CreatedAt   time.Time
}

// ProjectV2FieldDataType matches real GitHub's narrow set. Single-select
// + text + number cover what gh CLI / Octokit primarily exercise; date
// + iteration are deferred until a real consumer needs them.
type ProjectV2FieldDataType string

const (
	ProjectV2FieldSingleSelect ProjectV2FieldDataType = "SINGLE_SELECT"
	ProjectV2FieldText         ProjectV2FieldDataType = "TEXT"
	ProjectV2FieldNumber       ProjectV2FieldDataType = "NUMBER"
)

// ProjectV2Field is a column on a project. SINGLE_SELECT carries
// per-option metadata in Options.
type ProjectV2Field struct {
	ID        int
	NodeID    string
	ProjectID int
	Name      string
	DataType  ProjectV2FieldDataType
	Options   []*ProjectV2SingleSelectOption
	CreatedAt time.Time
}

// ProjectV2SingleSelectOption is one selectable value on a
// SINGLE_SELECT field (e.g. Status: Todo / In Progress / Done).
type ProjectV2SingleSelectOption struct {
	ID   string // GitHub uses 8-char alnum IDs ("47fc9ee4"); we generate similar
	Name string
}

// ProjectV2ItemFieldValue is the value an item has for one field. For
// SINGLE_SELECT, OptionID points at one of the field's options. For
// TEXT, TextValue holds the body. For NUMBER, NumberValue.
type ProjectV2ItemFieldValue struct {
	FieldID     int
	OptionID    string  // SINGLE_SELECT
	OptionName  string  // denormalised so reads don't need to chase the field
	TextValue   string  // TEXT
	NumberValue float64 // NUMBER
}

// ProjectV2Store is the in-memory store. Concurrency-safe via mu.
type ProjectV2Store struct {
	mu             sync.RWMutex
	projects       map[int]*ProjectV2
	items          map[int]*ProjectV2Item
	itemsByOwner   map[int][]*ProjectV2Item // contentID → items it appears in
	fields         map[int]*ProjectV2Field
	fieldsByProj   map[int][]*ProjectV2Field
	nextProjectID  int
	nextItemID     int
	nextFieldID    int
	nextOptionSeed int
}

func newProjectV2Store() *ProjectV2Store {
	return &ProjectV2Store{
		projects:       map[int]*ProjectV2{},
		items:          map[int]*ProjectV2Item{},
		itemsByOwner:   map[int][]*ProjectV2Item{},
		fields:         map[int]*ProjectV2Field{},
		fieldsByProj:   map[int][]*ProjectV2Field{},
		nextProjectID:  1,
		nextItemID:     1,
		nextFieldID:    1,
		nextOptionSeed: 1,
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
		FieldValues: map[int]*ProjectV2ItemFieldValue{},
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

// GetItem returns a project item by id.
func (s *ProjectV2Store) GetItem(id int) *ProjectV2Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[id]
}

// LookupItemByNodeID returns the item with the given GraphQL node id.
func (s *ProjectV2Store) LookupItemByNodeID(nodeID string) *ProjectV2Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, it := range s.items {
		if it.NodeID == nodeID {
			return it
		}
	}
	return nil
}

// CreateField adds a field column to a project.
func (s *ProjectV2Store) CreateField(projectID int, name string, dataType ProjectV2FieldDataType, options []string) *ProjectV2Field {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return nil
	}
	id := s.nextFieldID
	s.nextFieldID++
	f := &ProjectV2Field{
		ID:        id,
		NodeID:    fmt.Sprintf("PVTF_kgDO%08d", id),
		ProjectID: projectID,
		Name:      name,
		DataType:  dataType,
		CreatedAt: time.Now(),
	}
	if dataType == ProjectV2FieldSingleSelect {
		for _, optName := range options {
			optID := fmt.Sprintf("%08x", s.nextOptionSeed)
			s.nextOptionSeed++
			f.Options = append(f.Options, &ProjectV2SingleSelectOption{
				ID:   optID,
				Name: optName,
			})
		}
	}
	s.fields[id] = f
	s.fieldsByProj[projectID] = append(s.fieldsByProj[projectID], f)
	return f
}

// GetField returns the field by id.
func (s *ProjectV2Store) GetField(id int) *ProjectV2Field {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fields[id]
}

// LookupFieldByNodeID returns the field with the given GraphQL node id.
func (s *ProjectV2Store) LookupFieldByNodeID(nodeID string) *ProjectV2Field {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.fields {
		if f.NodeID == nodeID {
			return f
		}
	}
	return nil
}

// FieldsForProject returns every field defined on the project.
func (s *ProjectV2Store) FieldsForProject(projectID int) []*ProjectV2Field {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2Field, 0, len(s.fieldsByProj[projectID]))
	out = append(out, s.fieldsByProj[projectID]...)
	return out
}

// FieldByNameOnProject returns the field with the given name on the
// project, or nil. Lookups via gh CLI / GraphQL go through Issue.
// projectItems → ProjectV2Item.fieldValueByName → field name.
func (s *ProjectV2Store) FieldByNameOnProject(projectID int, name string) *ProjectV2Field {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.fieldsByProj[projectID] {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// SetFieldValue writes a value for (item, field). For SINGLE_SELECT,
// optionID must match one of the field's options. For TEXT/NUMBER,
// optionID is ignored. Returns (value, nil) on success.
func (s *ProjectV2Store) SetFieldValue(itemID, fieldID int, optionID, textValue string, numberValue float64) (*ProjectV2ItemFieldValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[itemID]
	if !ok {
		return nil, fmt.Errorf("item %d not found", itemID)
	}
	field, ok := s.fields[fieldID]
	if !ok {
		return nil, fmt.Errorf("field %d not found", fieldID)
	}
	if field.ProjectID != item.ProjectID {
		return nil, fmt.Errorf("field %d belongs to a different project than item %d", fieldID, itemID)
	}
	val := &ProjectV2ItemFieldValue{FieldID: fieldID}
	switch field.DataType {
	case ProjectV2FieldSingleSelect:
		if optionID == "" {
			return nil, fmt.Errorf("optionId is required for SINGLE_SELECT field %q", field.Name)
		}
		var match *ProjectV2SingleSelectOption
		for _, opt := range field.Options {
			if opt.ID == optionID {
				match = opt
				break
			}
		}
		if match == nil {
			return nil, fmt.Errorf("option %q not found on field %q", optionID, field.Name)
		}
		val.OptionID = match.ID
		val.OptionName = match.Name
	case ProjectV2FieldText:
		val.TextValue = textValue
	case ProjectV2FieldNumber:
		val.NumberValue = numberValue
	default:
		return nil, fmt.Errorf("unsupported field data type %q", field.DataType)
	}
	if item.FieldValues == nil {
		item.FieldValues = map[int]*ProjectV2ItemFieldValue{}
	}
	item.FieldValues[fieldID] = val
	return val, nil
}
