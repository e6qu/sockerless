package bleephub

import (
	"fmt"
	"sort"
	"strconv"
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

// ProjectV2View is a board/table view inside a project.
type ProjectV2View struct {
	ID        int
	NodeID    string
	ProjectID int
	Name      string
	Layout    string
	Filters   map[string]interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProjectV2Store is the in-memory store. Concurrency-safe via mu.
type ProjectV2Store struct {
	mu             sync.RWMutex
	projects       map[int]*ProjectV2
	items          map[int]*ProjectV2Item
	itemsByOwner   map[int][]*ProjectV2Item // contentID → items it appears in
	fields         map[int]*ProjectV2Field
	fieldsByProj   map[int][]*ProjectV2Field
	views          map[int]*ProjectV2View
	viewsByProj    map[int][]*ProjectV2View
	nextProjectID  int
	nextItemID     int
	nextFieldID    int
	nextOptionSeed int
	nextViewID     int
	persist        *Persistence
}

func newProjectV2Store(p *Persistence) *ProjectV2Store {
	return &ProjectV2Store{
		projects:       map[int]*ProjectV2{},
		items:          map[int]*ProjectV2Item{},
		itemsByOwner:   map[int][]*ProjectV2Item{},
		fields:         map[int]*ProjectV2Field{},
		fieldsByProj:   map[int][]*ProjectV2Field{},
		views:          map[int]*ProjectV2View{},
		viewsByProj:    map[int][]*ProjectV2View{},
		nextProjectID:  1,
		nextItemID:     1,
		nextFieldID:    1,
		nextOptionSeed: 1,
		nextViewID:     1,
		persist:        p,
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
	if s.persist != nil {
		s.persist.MustPut("projects_v2", strconv.Itoa(id), p)
	}
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
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(id), it)
	}
	return it
}

// AddDraftItem adds a draft issue to a project.
func (s *ProjectV2Store) AddDraftItem(projectID int, title, body string) *ProjectV2Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return nil
	}
	id := s.nextItemID
	s.nextItemID++
	it := &ProjectV2Item{
		ID:          id,
		NodeID:      fmt.Sprintf("PVTI_kgDO%08d", id),
		ProjectID:   projectID,
		ContentType: "DraftIssue",
		DraftTitle:  title,
		DraftBody:   body,
		FieldValues: map[int]*ProjectV2ItemFieldValue{},
		CreatedAt:   time.Now(),
	}
	s.items[id] = it
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(id), it)
	}
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
	if s.persist != nil {
		s.persist.MustPut("project_v2_fields", strconv.Itoa(id), f)
	}
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
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(itemID), item)
	}
	return val, nil
}

// ListProjectsForOwner returns all projects owned by a user or organization.
func (s *ProjectV2Store) ListProjectsForOwner(ownerID int, ownerType string) []*ProjectV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2, 0)
	for _, p := range s.projects {
		if p.OwnerID == ownerID && p.OwnerType == ownerType {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// UpdateProject patches a project's title/closed/public fields.
func (s *ProjectV2Store) UpdateProject(id int, title *string, closed, public *bool) *ProjectV2 {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.projects[id]
	if p == nil {
		return nil
	}
	if title != nil {
		p.Title = *title
	}
	if closed != nil {
		p.Closed = *closed
	}
	if public != nil {
		p.Public = *public
	}
	p.UpdatedAt = time.Now()
	if s.persist != nil {
		s.persist.MustPut("projects_v2", strconv.Itoa(id), p)
	}
	return p
}

// DeleteProject removes a project and its fields/items/views.
func (s *ProjectV2Store) DeleteProject(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projects[id] == nil {
		return false
	}
	delete(s.projects, id)
	for fid := range s.fields {
		if s.fields[fid].ProjectID == id {
			delete(s.fields, fid)
			if s.persist != nil {
				s.persist.MustDelete("project_v2_fields", strconv.Itoa(fid))
			}
		}
	}
	delete(s.fieldsByProj, id)
	for iid, it := range s.items {
		if it.ProjectID == id {
			delete(s.items, iid)
			if s.persist != nil {
				s.persist.MustDelete("project_v2_items", strconv.Itoa(iid))
			}
		}
	}
	delete(s.viewsByProj, id)
	for vid := range s.views {
		if s.views[vid].ProjectID == id {
			delete(s.views, vid)
		}
	}
	if s.persist != nil {
		s.persist.MustDelete("projects_v2", strconv.Itoa(id))
	}
	return true
}

// ListItemsForProject returns every item on a project.
func (s *ProjectV2Store) ListItemsForProject(projectID int) []*ProjectV2Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2Item, 0)
	for _, it := range s.items {
		if it.ProjectID == projectID {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// UpdateItem patches an item's draft title/body or field values.
func (s *ProjectV2Store) UpdateItem(id int, draftTitle, draftBody *string) *ProjectV2Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := s.items[id]
	if it == nil {
		return nil
	}
	if draftTitle != nil {
		it.DraftTitle = *draftTitle
	}
	if draftBody != nil {
		it.DraftBody = *draftBody
	}
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(id), it)
	}
	return it
}

// DeleteItem removes an item from a project.
func (s *ProjectV2Store) DeleteItem(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := s.items[id]
	if it == nil {
		return false
	}
	delete(s.items, id)
	if it.ContentID != 0 {
		owner := s.itemsByOwner[it.ContentID]
		filtered := make([]*ProjectV2Item, 0, len(owner))
		for _, x := range owner {
			if x.ID != id {
				filtered = append(filtered, x)
			}
		}
		s.itemsByOwner[it.ContentID] = filtered
	}
	if s.persist != nil {
		s.persist.MustDelete("project_v2_items", strconv.Itoa(id))
	}
	return true
}

// UpdateField patches a field's name/options.
func (s *ProjectV2Store) UpdateField(id int, name *string, options []string) *ProjectV2Field {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.fields[id]
	if f == nil {
		return nil
	}
	if name != nil {
		f.Name = *name
	}
	if options != nil && f.DataType == ProjectV2FieldSingleSelect {
		f.Options = nil
		for _, optName := range options {
			optID := fmt.Sprintf("%08x", s.nextOptionSeed)
			s.nextOptionSeed++
			f.Options = append(f.Options, &ProjectV2SingleSelectOption{
				ID:   optID,
				Name: optName,
			})
		}
	}
	if s.persist != nil {
		s.persist.MustPut("project_v2_fields", strconv.Itoa(id), f)
	}
	return f
}

// DeleteField removes a field from a project.
func (s *ProjectV2Store) DeleteField(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.fields[id]
	if f == nil {
		return false
	}
	delete(s.fields, id)
	projFields := s.fieldsByProj[f.ProjectID]
	filtered := make([]*ProjectV2Field, 0, len(projFields))
	for _, x := range projFields {
		if x.ID != id {
			filtered = append(filtered, x)
		}
	}
	s.fieldsByProj[f.ProjectID] = filtered
	if s.persist != nil {
		s.persist.MustDelete("project_v2_fields", strconv.Itoa(id))
	}
	return true
}

// CreateView adds a view to a project.
func (s *ProjectV2Store) CreateView(projectID int, name, layout string, filters map[string]interface{}) *ProjectV2View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projects[projectID] == nil {
		return nil
	}
	id := s.nextViewID
	s.nextViewID++
	now := time.Now()
	v := &ProjectV2View{
		ID:        id,
		NodeID:    fmt.Sprintf("PVTV_kgDO%08d", id),
		ProjectID: projectID,
		Name:      name,
		Layout:    layout,
		Filters:   filters,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.views[id] = v
	s.viewsByProj[projectID] = append(s.viewsByProj[projectID], v)
	return v
}

// GetView returns a view by id.
func (s *ProjectV2Store) GetView(id int) *ProjectV2View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.views[id]
}

// ViewsForProject returns every view on a project.
func (s *ProjectV2Store) ViewsForProject(projectID int) []*ProjectV2View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2View, 0, len(s.viewsByProj[projectID]))
	out = append(out, s.viewsByProj[projectID]...)
	return out
}
