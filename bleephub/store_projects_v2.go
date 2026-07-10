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
	CreatorID int    // user who created the project
	Title     string
	Closed    bool
	ClosedAt  *time.Time
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
	CreatorID   int
	DraftTitle  string
	DraftBody   string
	FieldValues map[int]*ProjectV2ItemFieldValue // fieldID → value
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
}

// ProjectV2FieldDataType is the custom-field data type. The values are
// the REST enum spelled uppercase, matching the GraphQL
// ProjectV2CustomFieldType enum spellings for the types both surfaces
// share; REST handlers lowercase them on the wire.
type ProjectV2FieldDataType string

const (
	ProjectV2FieldSingleSelect ProjectV2FieldDataType = "SINGLE_SELECT"
	ProjectV2FieldText         ProjectV2FieldDataType = "TEXT"
	ProjectV2FieldNumber       ProjectV2FieldDataType = "NUMBER"
	ProjectV2FieldDate         ProjectV2FieldDataType = "DATE"
	ProjectV2FieldIteration    ProjectV2FieldDataType = "ITERATION"
)

// ProjectV2Field is a column on a project. SINGLE_SELECT carries
// per-option metadata in Options; ITERATION carries its schedule in
// Iteration.
type ProjectV2Field struct {
	ID        int
	NodeID    string
	ProjectID int
	Name      string
	DataType  ProjectV2FieldDataType
	Options   []*ProjectV2SingleSelectOption
	Iteration *ProjectV2IterationConfiguration
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProjectV2SingleSelectOption is one selectable value on a
// SINGLE_SELECT field (e.g. Status: Todo / In Progress / Done).
type ProjectV2SingleSelectOption struct {
	ID          string // GitHub uses 8-char alnum IDs ("47fc9ee4"); we generate similar
	Name        string
	Color       string // GitHub's option color enum (BLUE, GRAY, GREEN, ...)
	Description string
}

// ProjectV2IterationConfiguration is the schedule of an ITERATION
// field: a default duration plus the concrete iterations.
type ProjectV2IterationConfiguration struct {
	StartDate  string // date of the first iteration, YYYY-MM-DD
	Duration   int    // default iteration length in days
	Iterations []*ProjectV2Iteration
}

// ProjectV2Iteration is one concrete iteration on an ITERATION field.
type ProjectV2Iteration struct {
	ID        string // same 8-char ID space as single-select options
	Title     string
	StartDate string // YYYY-MM-DD
	Duration  int    // days
}

// ProjectV2ItemFieldValue is the value an item has for one field. For
// SINGLE_SELECT, OptionID points at one of the field's options. For
// TEXT, TextValue holds the body. For NUMBER, NumberValue. For DATE,
// DateValue. For ITERATION, IterationID points at one of the field's
// iterations.
type ProjectV2ItemFieldValue struct {
	FieldID     int
	OptionID    string  // SINGLE_SELECT
	OptionName  string  // denormalised so reads don't need to chase the field
	TextValue   string  // TEXT
	NumberValue float64 // NUMBER
	DateValue   string  // DATE, YYYY-MM-DD
	IterationID string  // ITERATION
}

// ProjectV2View is a board/table/roadmap view inside a project.
type ProjectV2View struct {
	ID            int
	NodeID        string
	ProjectID     int
	Number        int // per-project sequential
	Name          string
	Layout        string // "table", "board", or "roadmap"
	CreatorID     int
	Filter        *string // the view's filter query, nil when unset
	VisibleFields []int   // field IDs shown in the view
	CreatedAt     time.Time
	UpdatedAt     time.Time
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

// CreateProject creates a new ProjectV2 owned by the given user or org,
// recording the creating user.
func (s *ProjectV2Store) CreateProject(ownerID int, ownerType, title string, creatorID int) *ProjectV2 {
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
		CreatorID: creatorID,
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
func (s *ProjectV2Store) AddItem(projectID int, contentType string, contentID, creatorID int) *ProjectV2Item {
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
	now := time.Now()
	it := &ProjectV2Item{
		ID:          id,
		NodeID:      fmt.Sprintf("PVTI_kgDO%08d", id),
		ProjectID:   projectID,
		ContentType: contentType,
		ContentID:   contentID,
		CreatorID:   creatorID,
		FieldValues: map[int]*ProjectV2ItemFieldValue{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.items[id] = it
	s.itemsByOwner[contentID] = append(s.itemsByOwner[contentID], it)
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(id), it)
	}
	return it
}

// AddDraftItem adds a draft issue to a project.
func (s *ProjectV2Store) AddDraftItem(projectID int, title, body string, creatorID int) *ProjectV2Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return nil
	}
	id := s.nextItemID
	s.nextItemID++
	now := time.Now()
	it := &ProjectV2Item{
		ID:          id,
		NodeID:      fmt.Sprintf("PVTI_kgDO%08d", id),
		ProjectID:   projectID,
		ContentType: "DraftIssue",
		CreatorID:   creatorID,
		DraftTitle:  title,
		DraftBody:   body,
		FieldValues: map[int]*ProjectV2ItemFieldValue{},
		CreatedAt:   now,
		UpdatedAt:   now,
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

// CreateField adds a field column to a project. options applies to
// SINGLE_SELECT fields (IDs are assigned here; Name/Color/Description
// are caller-supplied) and iteration to ITERATION fields (iteration IDs
// are assigned here too).
func (s *ProjectV2Store) CreateField(projectID int, name string, dataType ProjectV2FieldDataType, options []*ProjectV2SingleSelectOption, iteration *ProjectV2IterationConfiguration) *ProjectV2Field {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[projectID]; !ok {
		return nil
	}
	id := s.nextFieldID
	s.nextFieldID++
	now := time.Now()
	f := &ProjectV2Field{
		ID:        id,
		NodeID:    fmt.Sprintf("PVTF_kgDO%08d", id),
		ProjectID: projectID,
		Name:      name,
		DataType:  dataType,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if dataType == ProjectV2FieldSingleSelect {
		for _, opt := range options {
			color := opt.Color
			if color == "" {
				color = "GRAY" // real GitHub's default option color
			}
			f.Options = append(f.Options, &ProjectV2SingleSelectOption{
				ID:          s.nextOptionIDLocked(),
				Name:        opt.Name,
				Color:       color,
				Description: opt.Description,
			})
		}
	}
	if dataType == ProjectV2FieldIteration && iteration != nil {
		cfg := &ProjectV2IterationConfiguration{
			StartDate: iteration.StartDate,
			Duration:  iteration.Duration,
		}
		for _, it := range iteration.Iterations {
			cfg.Iterations = append(cfg.Iterations, &ProjectV2Iteration{
				ID:        s.nextOptionIDLocked(),
				Title:     it.Title,
				StartDate: it.StartDate,
				Duration:  it.Duration,
			})
		}
		f.Iteration = cfg
	}
	s.fields[id] = f
	s.fieldsByProj[projectID] = append(s.fieldsByProj[projectID], f)
	if s.persist != nil {
		s.persist.MustPut("project_v2_fields", strconv.Itoa(id), f)
	}
	return f
}

// nextOptionIDLocked mints the next 8-char hex ID shared by
// single-select options and iterations. Callers must hold s.mu.
func (s *ProjectV2Store) nextOptionIDLocked() string {
	id := fmt.Sprintf("%08x", s.nextOptionSeed)
	s.nextOptionSeed++
	return id
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
		if *closed && !p.Closed {
			now := time.Now()
			p.ClosedAt = &now
		}
		if !*closed {
			p.ClosedAt = nil
		}
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
			s.unindexItemLocked(it)
			if s.persist != nil {
				s.persist.MustDelete("project_v2_items", strconv.Itoa(iid))
			}
		}
	}
	delete(s.viewsByProj, id)
	for vid := range s.views {
		if s.views[vid].ProjectID == id {
			delete(s.views, vid)
			if s.persist != nil {
				s.persist.MustDelete("project_v2_views", strconv.Itoa(vid))
			}
		}
	}
	if s.persist != nil {
		s.persist.MustDelete("projects_v2", strconv.Itoa(id))
	}
	return true
}

// DeleteContentItems removes every ProjectV2 item whose content points at
// one of the supplied issue or pull request database IDs.
func (s *ProjectV2Store) DeleteContentItems(contentType string, contentIDs map[int]bool) {
	if len(contentIDs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, it := range s.items {
		if it.ContentType != contentType || !contentIDs[it.ContentID] {
			continue
		}
		delete(s.items, id)
		s.unindexItemLocked(it)
		if s.persist != nil {
			s.persist.MustDelete("project_v2_items", strconv.Itoa(id))
		}
	}
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
	it.UpdatedAt = time.Now()
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(id), it)
	}
	return it
}

// SetFieldValueAny writes a field value from a REST update, dispatching
// on the field's data type: string for TEXT/DATE, float64 for NUMBER,
// option ID string for SINGLE_SELECT, iteration ID string for
// ITERATION. A nil value clears the field.
func (s *ProjectV2Store) SetFieldValueAny(itemID, fieldID int, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[itemID]
	if !ok {
		return fmt.Errorf("item %d not found", itemID)
	}
	field, ok := s.fields[fieldID]
	if !ok {
		return fmt.Errorf("field %d not found", fieldID)
	}
	if field.ProjectID != item.ProjectID {
		return fmt.Errorf("field %d belongs to a different project than item %d", fieldID, itemID)
	}
	if item.FieldValues == nil {
		item.FieldValues = map[int]*ProjectV2ItemFieldValue{}
	}
	if value == nil {
		delete(item.FieldValues, fieldID)
		item.UpdatedAt = time.Now()
		if s.persist != nil {
			s.persist.MustPut("project_v2_items", strconv.Itoa(itemID), item)
		}
		return nil
	}
	val := &ProjectV2ItemFieldValue{FieldID: fieldID}
	switch field.DataType {
	case ProjectV2FieldText:
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %q expects a string value", field.Name)
		}
		val.TextValue = str
	case ProjectV2FieldDate:
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %q expects a date string value", field.Name)
		}
		if _, err := time.Parse("2006-01-02", str); err != nil {
			return fmt.Errorf("field %q expects a YYYY-MM-DD date, got %q", field.Name, str)
		}
		val.DateValue = str
	case ProjectV2FieldNumber:
		num, ok := value.(float64)
		if !ok {
			return fmt.Errorf("field %q expects a number value", field.Name)
		}
		val.NumberValue = num
	case ProjectV2FieldSingleSelect:
		optionID, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %q expects a single select option ID", field.Name)
		}
		var match *ProjectV2SingleSelectOption
		for _, opt := range field.Options {
			if opt.ID == optionID {
				match = opt
				break
			}
		}
		if match == nil {
			return fmt.Errorf("option %q not found on field %q", optionID, field.Name)
		}
		val.OptionID = match.ID
		val.OptionName = match.Name
	case ProjectV2FieldIteration:
		iterationID, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %q expects an iteration ID", field.Name)
		}
		found := false
		if field.Iteration != nil {
			for _, it := range field.Iteration.Iterations {
				if it.ID == iterationID {
					found = true
					break
				}
			}
		}
		if !found {
			return fmt.Errorf("iteration %q not found on field %q", iterationID, field.Name)
		}
		val.IterationID = iterationID
	default:
		return fmt.Errorf("field %q of type %q cannot be set directly", field.Name, field.DataType)
	}
	item.FieldValues[fieldID] = val
	item.UpdatedAt = time.Now()
	if s.persist != nil {
		s.persist.MustPut("project_v2_items", strconv.Itoa(itemID), item)
	}
	return nil
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
	s.unindexItemLocked(it)
	if s.persist != nil {
		s.persist.MustDelete("project_v2_items", strconv.Itoa(id))
	}
	return true
}

func (s *ProjectV2Store) unindexItemLocked(it *ProjectV2Item) {
	if it == nil || it.ContentID == 0 {
		return
	}
	owner := s.itemsByOwner[it.ContentID]
	kept := owner[:0]
	for _, x := range owner {
		if x.ID != it.ID {
			kept = append(kept, x)
		}
	}
	if len(kept) == 0 {
		delete(s.itemsByOwner, it.ContentID)
		return
	}
	s.itemsByOwner[it.ContentID] = kept
}

// UpdateField patches a field's name/options.
func (s *ProjectV2Store) UpdateField(id int, name *string, options []*ProjectV2SingleSelectOption) *ProjectV2Field {
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
		for _, opt := range options {
			color := opt.Color
			if color == "" {
				color = "GRAY"
			}
			f.Options = append(f.Options, &ProjectV2SingleSelectOption{
				ID:          s.nextOptionIDLocked(),
				Name:        opt.Name,
				Color:       color,
				Description: opt.Description,
			})
		}
	}
	f.UpdatedAt = time.Now()
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
func (s *ProjectV2Store) CreateView(projectID int, name, layout string, filter *string, visibleFields []int, creatorID int) *ProjectV2View {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projects[projectID] == nil {
		return nil
	}
	id := s.nextViewID
	s.nextViewID++
	number := 1
	for _, v := range s.viewsByProj[projectID] {
		if v.Number >= number {
			number = v.Number + 1
		}
	}
	now := time.Now()
	v := &ProjectV2View{
		ID:            id,
		NodeID:        fmt.Sprintf("PVTV_kgDO%08d", id),
		ProjectID:     projectID,
		Number:        number,
		Name:          name,
		Layout:        layout,
		CreatorID:     creatorID,
		Filter:        filter,
		VisibleFields: visibleFields,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	s.views[id] = v
	s.viewsByProj[projectID] = append(s.viewsByProj[projectID], v)
	if s.persist != nil {
		s.persist.MustPut("project_v2_views", strconv.Itoa(id), v)
	}
	return v
}

// GetView returns a view by id.
func (s *ProjectV2Store) GetView(id int) *ProjectV2View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.views[id]
}

// GetViewByNumber returns the project's view with the given per-project
// number, or nil.
func (s *ProjectV2Store) GetViewByNumber(projectID, number int) *ProjectV2View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.viewsByProj[projectID] {
		if v.Number == number {
			return v
		}
	}
	return nil
}

// ViewsForProject returns every view on a project.
func (s *ProjectV2Store) ViewsForProject(projectID int) []*ProjectV2View {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ProjectV2View, 0, len(s.viewsByProj[projectID]))
	out = append(out, s.viewsByProj[projectID]...)
	return out
}

// GetProjectByOwnerNumber returns the owner's project with the given
// per-owner number, or nil.
func (s *ProjectV2Store) GetProjectByOwnerNumber(ownerID int, ownerType string, number int) *ProjectV2 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.OwnerID == ownerID && p.OwnerType == ownerType && p.Number == number {
			return p
		}
	}
	return nil
}
