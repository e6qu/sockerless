package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitHub Projects v2 REST API. Backed by the same ProjectV2Store the
// GraphQL mutations use, so both surfaces see one set of projects,
// items, fields, and views.

func (s *Server) registerGHProjectsV2Routes() {
	// Organization-owned projects.
	s.route("GET /api/v3/orgs/{org}/projectsV2", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectsV2List))
	s.route("GET /api/v3/orgs/{org}/projectsV2/{project_number}", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectV2Get))
	s.route("POST /api/v3/orgs/{org}/projectsV2/{project_number}/drafts", s.requirePerm(scopeProjects, permWrite, s.handleOrgProjectV2CreateDraft))
	s.route("GET /api/v3/orgs/{org}/projectsV2/{project_number}/fields", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectV2ListFields))
	s.route("POST /api/v3/orgs/{org}/projectsV2/{project_number}/fields", s.requirePerm(scopeProjects, permWrite, s.handleOrgProjectV2CreateField))
	s.route("GET /api/v3/orgs/{org}/projectsV2/{project_number}/fields/{field_id}", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectV2GetField))
	s.route("GET /api/v3/orgs/{org}/projectsV2/{project_number}/items", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectV2ListItems))
	s.route("POST /api/v3/orgs/{org}/projectsV2/{project_number}/items", s.requirePerm(scopeProjects, permWrite, s.handleOrgProjectV2AddItem))
	s.route("GET /api/v3/orgs/{org}/projectsV2/{project_number}/items/{item_id}", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectV2GetItem))
	s.route("PATCH /api/v3/orgs/{org}/projectsV2/{project_number}/items/{item_id}", s.requirePerm(scopeProjects, permWrite, s.handleOrgProjectV2UpdateItem))
	s.route("DELETE /api/v3/orgs/{org}/projectsV2/{project_number}/items/{item_id}", s.requirePerm(scopeProjects, permWrite, s.handleOrgProjectV2DeleteItem))
	s.route("POST /api/v3/orgs/{org}/projectsV2/{project_number}/views", s.requirePerm(scopeProjects, permWrite, s.handleOrgProjectV2CreateView))
	s.route("GET /api/v3/orgs/{org}/projectsV2/{project_number}/views/{view_number}/items", s.requirePerm(scopeProjects, permRead, s.handleOrgProjectV2ListViewItems))

	// User-owned projects. The create-view and create-draft routes are
	// keyed by user ID, not login — that is the real GitHub path shape
	// (/users/{user_id}/…/views and /user/{user_id}/…/drafts).
	s.route("GET /api/v3/users/{username}/projectsV2", s.requirePerm(scopeProjects, permRead, s.handleUserProjectsV2List))
	s.route("GET /api/v3/users/{username}/projectsV2/{project_number}", s.requirePerm(scopeProjects, permRead, s.handleUserProjectV2Get))
	s.route("GET /api/v3/users/{username}/projectsV2/{project_number}/fields", s.requirePerm(scopeProjects, permRead, s.handleUserProjectV2ListFields))
	s.route("POST /api/v3/users/{username}/projectsV2/{project_number}/fields", s.requirePerm(scopeProjects, permWrite, s.handleUserProjectV2CreateField))
	s.route("GET /api/v3/users/{username}/projectsV2/{project_number}/fields/{field_id}", s.requirePerm(scopeProjects, permRead, s.handleUserProjectV2GetField))
	s.route("GET /api/v3/users/{username}/projectsV2/{project_number}/items", s.requirePerm(scopeProjects, permRead, s.handleUserProjectV2ListItems))
	s.route("POST /api/v3/users/{username}/projectsV2/{project_number}/items", s.requirePerm(scopeProjects, permWrite, s.handleUserProjectV2AddItem))
	s.route("GET /api/v3/users/{username}/projectsV2/{project_number}/items/{item_id}", s.requirePerm(scopeProjects, permRead, s.handleUserProjectV2GetItem))
	s.route("PATCH /api/v3/users/{username}/projectsV2/{project_number}/items/{item_id}", s.requirePerm(scopeProjects, permWrite, s.handleUserProjectV2UpdateItem))
	s.route("DELETE /api/v3/users/{username}/projectsV2/{project_number}/items/{item_id}", s.requirePerm(scopeProjects, permWrite, s.handleUserProjectV2DeleteItem))
	s.route("POST /api/v3/users/{user_id}/projectsV2/{project_number}/views", s.requirePerm(scopeProjects, permWrite, s.handleUserProjectV2CreateView))
	s.route("GET /api/v3/users/{username}/projectsV2/{project_number}/views/{view_number}/items", s.requirePerm(scopeProjects, permRead, s.handleUserProjectV2ListViewItems))
	s.route("POST /api/v3/user/{user_id}/projectsV2/{project_number}/drafts", s.requirePerm(scopeProjects, permWrite, s.handleAuthenticatedUserProjectV2CreateDraft))
}

// ---------------------------------------------------------------------------
// Owner resolution + access control

// projectV2Owner is the resolved owner (org or user) of a Projects v2
// project addressed by a REST path.
type projectV2Owner struct {
	id        int
	ownerType string // "Organization" or "User"
	login     string
	org       *Org
	user      *User
}

func (s *Server) projectV2OrgOwner(w http.ResponseWriter, r *http.Request) (*projectV2Owner, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return &projectV2Owner{id: org.ID, ownerType: "Organization", login: org.Login, org: org}, true
}

func (s *Server) projectV2UserOwnerByLogin(w http.ResponseWriter, r *http.Request) (*projectV2Owner, bool) {
	u := s.store.LookupUserByLogin(r.PathValue("username"))
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return &projectV2Owner{id: u.ID, ownerType: "User", login: u.Login, user: u}, true
}

func (s *Server) projectV2UserOwnerByID(w http.ResponseWriter, r *http.Request) (*projectV2Owner, bool) {
	id, err := strconv.Atoi(r.PathValue("user_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	u := s.store.GetUserByID(id)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return &projectV2Owner{id: u.ID, ownerType: "User", login: u.Login, user: u}, true
}

// canReadProjectV2: public projects are visible to any caller; private
// projects only to the owning user, active members of the owning org,
// or a site admin.
func (s *Server) canReadProjectV2(user *User, owner *projectV2Owner, p *ProjectV2) bool {
	if p.Public {
		return true
	}
	if user == nil {
		return false
	}
	if user.SiteAdmin {
		return true
	}
	if owner.ownerType == "User" {
		return user.ID == owner.id
	}
	return isActiveOrgMember(s.store, user, owner.login)
}

// canWriteProjectV2: the owning user, active members of the owning org,
// or a site admin.
func (s *Server) canWriteProjectV2(user *User, owner *projectV2Owner) bool {
	if user == nil {
		return false
	}
	if user.SiteAdmin {
		return true
	}
	if owner.ownerType == "User" {
		return user.ID == owner.id
	}
	return isActiveOrgMember(s.store, user, owner.login)
}

// projectV2FromRequest resolves {project_number} for the owner and
// enforces read visibility. Writes 404 (never 403) when the project is
// missing or hidden, matching how GitHub conceals private resources.
func (s *Server) projectV2FromRequest(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) (*ProjectV2, bool) {
	number, err := strconv.Atoi(r.PathValue("project_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	p := s.store.ProjectsV2.GetProjectByOwnerNumber(owner.id, owner.ownerType, number)
	if p == nil || !s.canReadProjectV2(ghUserFromContext(r.Context()), owner, p) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return p, true
}

// requireProjectV2Write enforces write access, writing 403 on denial.
func (s *Server) requireProjectV2Write(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) (*User, bool) {
	user := ghUserFromContext(r.Context())
	if !s.canWriteProjectV2(user, owner) {
		writeGHError(w, http.StatusForbidden, "Must have write access to the project.")
		return nil, false
	}
	return user, true
}

// ---------------------------------------------------------------------------
// Cursor pagination (per_page + after/before), shared with the
// attestations surface.

type cursorPageInfo struct {
	HasNext bool
	HasPrev bool
	Next    string
	Prev    string
}

// cursorPaginate applies GitHub's cursor pagination query parameters
// (per_page, after, before) to items sorted ascending by the stable
// integer identity idOf. Cursors are opaque encodings of that identity.
func cursorPaginate[T any](r *http.Request, items []T, idOf func(T) int) ([]T, cursorPageInfo) {
	perPage := 30
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
			if perPage > 100 {
				perPage = 100
			}
		}
	}
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	var start, end int
	switch {
	case after != "":
		id := decodeCursor(after)
		start = len(items)
		for i, it := range items {
			if idOf(it) > id {
				start = i
				break
			}
		}
		end = start + perPage
	case before != "":
		id := decodeCursor(before)
		end = 0
		for i := len(items) - 1; i >= 0; i-- {
			if idOf(items[i]) < id {
				end = i + 1
				break
			}
		}
		start = end - perPage
	default:
		end = perPage
	}
	if start < 0 {
		start = 0
	}
	if end > len(items) {
		end = len(items)
	}
	if start > end {
		start = end
	}
	page := items[start:end]

	pi := cursorPageInfo{HasPrev: start > 0, HasNext: end < len(items)}
	if len(page) > 0 {
		pi.Next = encodeCursor(idOf(page[len(page)-1]))
		pi.Prev = encodeCursor(idOf(page[0]))
	}
	return page, pi
}

// setCursorLinkHeader emits the RFC 5988 Link header for a
// cursor-paginated response (rel="next" via after, rel="prev" via
// before), preserving the other query parameters.
func setCursorLinkHeader(w http.ResponseWriter, r *http.Request, pi cursorPageInfo) {
	base := r.URL.Path
	mk := func(k, cursor string) string {
		q := r.URL.Query()
		q.Del("after")
		q.Del("before")
		q.Del("page")
		q.Set(k, cursor)
		return "<" + base + "?" + q.Encode() + ">"
	}
	var parts []string
	if pi.HasNext && pi.Next != "" {
		parts = append(parts, mk("after", pi.Next)+`; rel="next"`)
	}
	if pi.HasPrev && pi.Prev != "" {
		parts = append(parts, mk("before", pi.Prev)+`; rel="prev"`)
	}
	if len(parts) > 0 {
		w.Header().Set("Link", strings.Join(parts, ", "))
	}
}

// ---------------------------------------------------------------------------
// JSON rendering

// projectV2APIURL is the project's REST URL, which anchors project_url
// and item_url members.
func (s *Server) projectV2APIURL(r *http.Request, owner *projectV2Owner, number int) string {
	base := s.baseURL(r)
	if owner.ownerType == "Organization" {
		return base + "/api/v3/orgs/" + owner.login + "/projectsV2/" + strconv.Itoa(number)
	}
	return base + "/api/v3/users/" + owner.login + "/projectsV2/" + strconv.Itoa(number)
}

// projectV2CreatorJSON renders the creating user; a creator that no
// longer resolves renders as GitHub's "ghost" placeholder account.
func (s *Server) projectV2CreatorJSON(creatorID int) map[string]interface{} {
	if u := s.store.GetUserByID(creatorID); u != nil {
		return userToJSON(u)
	}
	api := "/api/v3/users/ghost"
	return map[string]interface{}{
		"login":               "ghost",
		"id":                  0,
		"node_id":             "U_kgDOghost0",
		"avatar_url":          "",
		"gravatar_id":         "",
		"url":                 api,
		"html_url":            "/ghost",
		"followers_url":       api + "/followers",
		"following_url":       api + "/following{/other_user}",
		"gists_url":           api + "/gists{/gist_id}",
		"starred_url":         api + "/starred{/owner}{/repo}",
		"subscriptions_url":   api + "/subscriptions",
		"organizations_url":   api + "/orgs",
		"repos_url":           api + "/repos",
		"events_url":          api + "/events{/privacy}",
		"received_events_url": api + "/received_events",
		"type":                "User",
		"site_admin":          false,
		"user_view_type":      "public",
	}
}

func (s *Server) projectV2JSON(p *ProjectV2, owner *projectV2Owner) map[string]interface{} {
	var ownerJSON map[string]interface{}
	if owner.org != nil {
		ownerJSON = orgAsSimpleUserJSON(owner.org)
	} else if owner.user != nil {
		ownerJSON = userToJSON(owner.user)
	}
	state := "open"
	var closedAt interface{}
	if p.Closed {
		state = "closed"
		if p.ClosedAt != nil {
			closedAt = p.ClosedAt.UTC().Format(time.RFC3339)
		}
	}
	return map[string]interface{}{
		"id":                p.ID,
		"node_id":           p.NodeID,
		"owner":             ownerJSON,
		"creator":           s.projectV2CreatorJSON(p.CreatorID),
		"title":             p.Title,
		"description":       nil,
		"public":            p.Public,
		"closed_at":         closedAt,
		"created_at":        p.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        p.UpdatedAt.UTC().Format(time.RFC3339),
		"number":            p.Number,
		"short_description": nil,
		"deleted_at":        nil,
		"deleted_by":        nil,
		"state":             state,
		"is_template":       false,
	}
}

func projectV2FieldJSON(f *ProjectV2Field, projectURL string) map[string]interface{} {
	out := map[string]interface{}{
		"id":          f.ID,
		"node_id":     f.NodeID,
		"name":        f.Name,
		"data_type":   strings.ToLower(string(f.DataType)),
		"project_url": projectURL,
		"created_at":  f.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  f.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if f.DataType == ProjectV2FieldSingleSelect {
		options := make([]map[string]interface{}, 0, len(f.Options))
		for _, o := range f.Options {
			options = append(options, map[string]interface{}{
				"id":          o.ID,
				"name":        map[string]interface{}{"raw": o.Name, "html": o.Name},
				"description": map[string]interface{}{"raw": o.Description, "html": o.Description},
				"color":       o.Color,
			})
		}
		out["options"] = options
	}
	if f.DataType == ProjectV2FieldIteration && f.Iteration != nil {
		out["configuration"] = projectV2IterationConfigJSON(f.Iteration)
	}
	return out
}

func projectV2IterationConfigJSON(cfg *ProjectV2IterationConfiguration) map[string]interface{} {
	iterations := make([]map[string]interface{}, 0, len(cfg.Iterations))
	for _, it := range cfg.Iterations {
		completed := false
		if start, err := time.Parse("2006-01-02", it.StartDate); err == nil {
			completed = time.Now().After(start.AddDate(0, 0, it.Duration))
		}
		iterations = append(iterations, map[string]interface{}{
			"id":         it.ID,
			"start_date": it.StartDate,
			"duration":   it.Duration,
			"title":      map[string]interface{}{"raw": it.Title, "html": it.Title},
			"completed":  completed,
		})
	}
	out := map[string]interface{}{
		"duration":   cfg.Duration,
		"iterations": iterations,
	}
	if start, err := time.Parse("2006-01-02", cfg.StartDate); err == nil {
		// ISO weekday: Monday = 1 … Sunday = 7.
		weekday := int(start.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		out["start_day"] = weekday
	}
	return out
}

func (s *Server) projectV2ItemSimpleJSON(it *ProjectV2Item, projectURL string) map[string]interface{} {
	var archivedAt interface{}
	if it.ArchivedAt != nil {
		archivedAt = it.ArchivedAt.UTC().Format(time.RFC3339)
	}
	return map[string]interface{}{
		"id":           it.ID,
		"node_id":      it.NodeID,
		"content_type": it.ContentType,
		"creator":      s.projectV2CreatorJSON(it.CreatorID),
		"created_at":   it.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   it.UpdatedAt.UTC().Format(time.RFC3339),
		"archived_at":  archivedAt,
		"project_url":  projectURL,
		"item_url":     projectURL + "/items/" + strconv.Itoa(it.ID),
	}
}

func (s *Server) projectV2ItemWithContentJSON(r *http.Request, it *ProjectV2Item, projectURL string, fieldIDs []int) map[string]interface{} {
	out := s.projectV2ItemSimpleJSON(it, projectURL)
	out["content"] = s.projectV2ItemContentJSON(r, it)
	out["fields"] = s.projectV2ItemFieldsJSON(it, fieldIDs)
	return out
}

func (s *Server) projectV2ItemContentJSON(r *http.Request, it *ProjectV2Item) interface{} {
	base := s.baseURL(r)
	switch it.ContentType {
	case "Issue":
		issue := s.store.GetIssue(it.ContentID)
		if issue == nil {
			return nil
		}
		repo := s.store.GetRepoByID(issue.RepoID)
		if repo == nil {
			return nil
		}
		return issueToJSON(issue, s.store, base, repo.FullName)
	case "PullRequest":
		pr := s.store.GetPullRequest(it.ContentID)
		if pr == nil {
			return nil
		}
		repo := s.store.GetRepoByID(pr.RepoID)
		if repo == nil {
			return nil
		}
		return pullRequestToJSON(pr, s.store, base, repo.FullName)
	case "DraftIssue":
		var user interface{}
		if u := s.store.GetUserByID(it.CreatorID); u != nil {
			user = userToJSON(u)
		}
		return map[string]interface{}{
			"id":         it.ID,
			"node_id":    fmt.Sprintf("DI_kgDO%08d", it.ID),
			"title":      it.DraftTitle,
			"body":       it.DraftBody,
			"user":       user,
			"created_at": it.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at": it.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	return nil
}

// projectV2ItemFieldsJSON renders the item's field values. With an
// explicit fieldIDs selection (the `fields` query parameter) every
// requested field is rendered, unset values as null; otherwise every
// field holding a value is rendered.
func (s *Server) projectV2ItemFieldsJSON(it *ProjectV2Item, fieldIDs []int) []map[string]interface{} {
	ids := fieldIDs
	if len(ids) == 0 {
		for fid := range it.FieldValues {
			ids = append(ids, fid)
		}
		sort.Ints(ids)
	}
	out := make([]map[string]interface{}, 0, len(ids))
	for _, fid := range ids {
		f := s.store.ProjectsV2.GetField(fid)
		if f == nil || f.ProjectID != it.ProjectID {
			continue
		}
		entry := map[string]interface{}{
			"id":        f.ID,
			"name":      f.Name,
			"data_type": strings.ToLower(string(f.DataType)),
			"value":     projectV2FieldValueJSON(f, it.FieldValues[fid]),
		}
		out = append(out, entry)
	}
	return out
}

func projectV2FieldValueJSON(f *ProjectV2Field, v *ProjectV2ItemFieldValue) interface{} {
	if v == nil {
		return nil
	}
	switch f.DataType {
	case ProjectV2FieldSingleSelect:
		return map[string]interface{}{"id": v.OptionID, "name": v.OptionName}
	case ProjectV2FieldText:
		return v.TextValue
	case ProjectV2FieldNumber:
		return v.NumberValue
	case ProjectV2FieldDate:
		return v.DateValue
	case ProjectV2FieldIteration:
		if f.Iteration != nil {
			for _, it := range f.Iteration.Iterations {
				if it.ID == v.IterationID {
					return map[string]interface{}{
						"id":         it.ID,
						"title":      it.Title,
						"start_date": it.StartDate,
						"duration":   it.Duration,
					}
				}
			}
		}
		return map[string]interface{}{"id": v.IterationID}
	}
	return nil
}

func (s *Server) projectV2ViewJSON(r *http.Request, v *ProjectV2View, owner *projectV2Owner, project *ProjectV2) map[string]interface{} {
	var filter interface{}
	if v.Filter != nil {
		filter = *v.Filter
	}
	visible := v.VisibleFields
	if visible == nil {
		visible = []int{}
	}
	ownerPath := "/users/" + owner.login
	if owner.ownerType == "Organization" {
		ownerPath = "/orgs/" + owner.login
	}
	return map[string]interface{}{
		"id":                v.ID,
		"number":            v.Number,
		"name":              v.Name,
		"layout":            v.Layout,
		"node_id":           v.NodeID,
		"project_url":       s.projectV2APIURL(r, owner, project.Number),
		"html_url":          s.baseURL(r) + ownerPath + "/projects/" + strconv.Itoa(project.Number) + "/views/" + strconv.Itoa(v.Number),
		"creator":           s.projectV2CreatorJSON(v.CreatorID),
		"created_at":        v.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        v.UpdatedAt.UTC().Format(time.RFC3339),
		"filter":            filter,
		"visible_fields":    visible,
		"sort_by":           []interface{}{},
		"group_by":          []int{},
		"vertical_group_by": []int{},
	}
}

// ---------------------------------------------------------------------------
// Filter queries. The project filter grammar is a token stream of
// `is:` state qualifiers, `field:value` qualifiers matching a field's
// value, and free text matching the title.

func (s *Server) projectV2ItemMatchesFilter(it *ProjectV2Item, filter string) bool {
	for _, tok := range strings.Fields(filter) {
		lower := strings.ToLower(tok)
		switch {
		case lower == "is:issue":
			if it.ContentType != "Issue" {
				return false
			}
		case lower == "is:pr" || lower == "is:pull-request":
			if it.ContentType != "PullRequest" {
				return false
			}
		case lower == "is:draft":
			if it.ContentType != "DraftIssue" {
				return false
			}
		case lower == "is:open":
			if s.projectV2ItemState(it) != "open" {
				return false
			}
		case lower == "is:closed":
			if s.projectV2ItemState(it) == "open" {
				return false
			}
		case strings.Contains(tok, ":"):
			name, value, _ := strings.Cut(tok, ":")
			if !s.projectV2ItemFieldMatches(it, name, value) {
				return false
			}
		default:
			if !strings.Contains(strings.ToLower(s.projectV2ItemTitle(it)), lower) {
				return false
			}
		}
	}
	return true
}

func (s *Server) projectV2ItemFieldMatches(it *ProjectV2Item, fieldName, want string) bool {
	for _, f := range s.store.ProjectsV2.FieldsForProject(it.ProjectID) {
		if !strings.EqualFold(f.Name, fieldName) {
			continue
		}
		v := s.store.ProjectsV2.GetItem(it.ID).FieldValues[f.ID]
		if v == nil {
			return false
		}
		switch f.DataType {
		case ProjectV2FieldSingleSelect:
			return strings.EqualFold(v.OptionName, want)
		case ProjectV2FieldText:
			return strings.EqualFold(v.TextValue, want)
		case ProjectV2FieldNumber:
			num, err := strconv.ParseFloat(want, 64)
			return err == nil && num == v.NumberValue
		case ProjectV2FieldDate:
			return v.DateValue == want
		case ProjectV2FieldIteration:
			if f.Iteration != nil {
				for _, iter := range f.Iteration.Iterations {
					if iter.ID == v.IterationID {
						return strings.EqualFold(iter.Title, want)
					}
				}
			}
			return false
		}
	}
	return false
}

func (s *Server) projectV2ItemState(it *ProjectV2Item) string {
	switch it.ContentType {
	case "Issue":
		if issue := s.store.GetIssue(it.ContentID); issue != nil && issue.State != "OPEN" {
			return "closed"
		}
	case "PullRequest":
		if pr := s.store.GetPullRequest(it.ContentID); pr != nil && pr.State != "OPEN" {
			return "closed"
		}
	}
	return "open"
}

func (s *Server) projectV2ItemTitle(it *ProjectV2Item) string {
	switch it.ContentType {
	case "Issue":
		if issue := s.store.GetIssue(it.ContentID); issue != nil {
			return issue.Title
		}
	case "PullRequest":
		if pr := s.store.GetPullRequest(it.ContentID); pr != nil {
			return pr.Title
		}
	case "DraftIssue":
		return it.DraftTitle
	}
	return ""
}

func projectV2MatchesQuery(p *ProjectV2, q string) bool {
	for _, tok := range strings.Fields(q) {
		switch strings.ToLower(tok) {
		case "is:open":
			if p.Closed {
				return false
			}
		case "is:closed":
			if !p.Closed {
				return false
			}
		default:
			if strings.Contains(tok, ":") {
				return false // unknown qualifier matches no project
			}
			if !strings.Contains(strings.ToLower(p.Title), strings.ToLower(tok)) {
				return false
			}
		}
	}
	return true
}

// parseProjectV2FieldsParam parses the `fields` query parameter (a list
// of field IDs, comma-separated or repeated). Returns ok=false after
// writing a 422 for a non-numeric ID.
func parseProjectV2FieldsParam(w http.ResponseWriter, r *http.Request) ([]int, bool) {
	var out []int
	for _, raw := range r.URL.Query()["fields"] {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.Atoi(part)
			if err != nil {
				writeGHValidationError(w, "ProjectV2Item", "fields", "invalid")
				return nil, false
			}
			out = append(out, id)
		}
	}
	return out, true
}

// ---------------------------------------------------------------------------
// Shared handler cores

func (s *Server) serveProjectsV2List(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	user := ghUserFromContext(r.Context())
	q := r.URL.Query().Get("q")
	all := s.store.ProjectsV2.ListProjectsForOwner(owner.id, owner.ownerType)
	visible := make([]*ProjectV2, 0, len(all))
	for _, p := range all {
		if !s.canReadProjectV2(user, owner, p) {
			continue
		}
		if q != "" && !projectV2MatchesQuery(p, q) {
			continue
		}
		visible = append(visible, p)
	}
	page, pi := cursorPaginate(r, visible, func(p *ProjectV2) int { return p.ID })
	setCursorLinkHeader(w, r, pi)
	out := make([]map[string]interface{}, 0, len(page))
	for _, p := range page {
		out = append(out, s.projectV2JSON(p, owner))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) serveProjectV2Get(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.projectV2JSON(p, owner))
}

func (s *Server) serveProjectV2CreateDraft(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	user, ok := s.requireProjectV2Write(w, r, owner)
	if !ok {
		return
	}
	var req struct {
		Title *string `json:"title"`
		Body  string  `json:"body"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Title == nil || strings.TrimSpace(*req.Title) == "" {
		writeGHValidationError(w, "ProjectV2Item", "title", "missing_field")
		return
	}
	item := s.store.ProjectsV2.AddDraftItem(p.ID, *req.Title, req.Body, user.ID)
	writeJSON(w, http.StatusCreated, s.projectV2ItemSimpleJSON(item, s.projectV2APIURL(r, owner, p.Number)))
}

func (s *Server) serveProjectV2ListFields(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	fields := s.store.ProjectsV2.FieldsForProject(p.ID)
	sort.Slice(fields, func(i, j int) bool { return fields[i].ID < fields[j].ID })
	page, pi := cursorPaginate(r, fields, func(f *ProjectV2Field) int { return f.ID })
	setCursorLinkHeader(w, r, pi)
	projectURL := s.projectV2APIURL(r, owner, p.Number)
	out := make([]map[string]interface{}, 0, len(page))
	for _, f := range page {
		out = append(out, projectV2FieldJSON(f, projectURL))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) serveProjectV2CreateField(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	if _, ok := s.requireProjectV2Write(w, r, owner); !ok {
		return
	}
	var req struct {
		IssueFieldID        *int    `json:"issue_field_id"`
		Name                *string `json:"name"`
		DataType            *string `json:"data_type"`
		SingleSelectOptions []struct {
			Name        string `json:"name"`
			Color       string `json:"color"`
			Description string `json:"description"`
		} `json:"single_select_options"`
		IterationConfiguration *struct {
			StartDate  string `json:"start_date"`
			Duration   int    `json:"duration"`
			Iterations []struct {
				Title     string `json:"title"`
				StartDate string `json:"start_date"`
				Duration  int    `json:"duration"`
			} `json:"iterations"`
		} `json:"iteration_configuration"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.IssueFieldID != nil {
		// bleephub has no organization issue fields, so no issue_field_id
		// can resolve to one.
		writeGHValidationError(w, "ProjectV2Field", "issue_field_id", "invalid")
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		writeGHValidationError(w, "ProjectV2Field", "name", "missing_field")
		return
	}
	if req.DataType == nil {
		writeGHValidationError(w, "ProjectV2Field", "data_type", "missing_field")
		return
	}
	for _, f := range s.store.ProjectsV2.FieldsForProject(p.ID) {
		if strings.EqualFold(f.Name, *req.Name) {
			writeGHValidationError(w, "ProjectV2Field", "name", "already_exists")
			return
		}
	}

	var options []*ProjectV2SingleSelectOption
	var iteration *ProjectV2IterationConfiguration
	switch *req.DataType {
	case "text", "number", "date":
	case "single_select":
		if len(req.SingleSelectOptions) == 0 {
			writeGHValidationError(w, "ProjectV2Field", "single_select_options", "missing_field")
			return
		}
		for _, o := range req.SingleSelectOptions {
			if strings.TrimSpace(o.Name) == "" {
				writeGHValidationError(w, "ProjectV2Field", "single_select_options", "invalid")
				return
			}
			options = append(options, &ProjectV2SingleSelectOption{Name: o.Name, Color: o.Color, Description: o.Description})
		}
	case "iteration":
		if req.IterationConfiguration == nil {
			writeGHValidationError(w, "ProjectV2Field", "iteration_configuration", "missing_field")
			return
		}
		cfg := req.IterationConfiguration
		duration := cfg.Duration
		if duration <= 0 {
			duration = 14 // real GitHub's default iteration length
		}
		startDate := cfg.StartDate
		if startDate == "" {
			startDate = time.Now().UTC().Format("2006-01-02")
		} else if _, err := time.Parse("2006-01-02", startDate); err != nil {
			writeGHValidationError(w, "ProjectV2Field", "iteration_configuration", "invalid")
			return
		}
		iteration = &ProjectV2IterationConfiguration{StartDate: startDate, Duration: duration}
		for _, it := range cfg.Iterations {
			itDuration := it.Duration
			if itDuration <= 0 {
				itDuration = duration
			}
			itStart := it.StartDate
			if itStart != "" {
				if _, err := time.Parse("2006-01-02", itStart); err != nil {
					writeGHValidationError(w, "ProjectV2Field", "iteration_configuration", "invalid")
					return
				}
			} else {
				itStart = startDate
			}
			iteration.Iterations = append(iteration.Iterations, &ProjectV2Iteration{
				Title:     it.Title,
				StartDate: itStart,
				Duration:  itDuration,
			})
		}
	default:
		writeGHValidationError(w, "ProjectV2Field", "data_type", "invalid")
		return
	}

	field := s.store.ProjectsV2.CreateField(p.ID, *req.Name, ProjectV2FieldDataType(strings.ToUpper(*req.DataType)), options, iteration)
	writeJSON(w, http.StatusCreated, projectV2FieldJSON(field, s.projectV2APIURL(r, owner, p.Number)))
}

func (s *Server) serveProjectV2GetField(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	fieldID, err := strconv.Atoi(r.PathValue("field_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	f := s.store.ProjectsV2.GetField(fieldID)
	if f == nil || f.ProjectID != p.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, projectV2FieldJSON(f, s.projectV2APIURL(r, owner, p.Number)))
}

func (s *Server) serveProjectV2ListItems(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	fieldIDs, ok := parseProjectV2FieldsParam(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	items := s.store.ProjectsV2.ListItemsForProject(p.ID)
	if q != "" {
		filtered := items[:0:0]
		for _, it := range items {
			if s.projectV2ItemMatchesFilter(it, q) {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	page, pi := cursorPaginate(r, items, func(it *ProjectV2Item) int { return it.ID })
	setCursorLinkHeader(w, r, pi)
	projectURL := s.projectV2APIURL(r, owner, p.Number)
	out := make([]map[string]interface{}, 0, len(page))
	for _, it := range page {
		out = append(out, s.projectV2ItemWithContentJSON(r, it, projectURL, fieldIDs))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) serveProjectV2AddItem(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	user, ok := s.requireProjectV2Write(w, r, owner)
	if !ok {
		return
	}
	var req struct {
		Type   string  `json:"type"`
		ID     *int    `json:"id"`
		Owner  *string `json:"owner"`
		Repo   *string `json:"repo"`
		Number *int    `json:"number"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Type != "Issue" && req.Type != "PullRequest" {
		writeGHValidationError(w, "ProjectV2Item", "type", "invalid")
		return
	}

	var contentID int
	switch {
	case req.ID != nil:
		contentID = *req.ID
	case req.Owner != nil && req.Repo != nil && req.Number != nil:
		repo := s.store.GetRepo(*req.Owner, *req.Repo)
		if repo == nil {
			writeGHValidationError(w, "ProjectV2Item", "repo", "invalid")
			return
		}
		if req.Type == "Issue" {
			issue := s.store.GetIssueByNumber(repo.ID, *req.Number)
			if issue == nil {
				writeGHValidationError(w, "ProjectV2Item", "number", "invalid")
				return
			}
			contentID = issue.ID
		} else {
			pr := s.store.GetPullRequestByNumber(repo.ID, *req.Number)
			if pr == nil {
				writeGHValidationError(w, "ProjectV2Item", "number", "invalid")
				return
			}
			contentID = pr.ID
		}
	default:
		writeGHValidationError(w, "ProjectV2Item", "id", "missing_field")
		return
	}

	// The database ID path must also resolve to real content.
	if req.Type == "Issue" {
		if s.store.GetIssue(contentID) == nil {
			writeGHValidationError(w, "ProjectV2Item", "id", "invalid")
			return
		}
	} else if s.store.GetPullRequest(contentID) == nil {
		writeGHValidationError(w, "ProjectV2Item", "id", "invalid")
		return
	}

	item := s.store.ProjectsV2.AddItem(p.ID, req.Type, contentID, user.ID)
	writeJSON(w, http.StatusCreated, s.projectV2ItemSimpleJSON(item, s.projectV2APIURL(r, owner, p.Number)))
}

// projectV2ItemFromRequest resolves {item_id} within the project,
// writing 404 when it is absent or belongs elsewhere.
func (s *Server) projectV2ItemFromRequest(w http.ResponseWriter, r *http.Request, p *ProjectV2) (*ProjectV2Item, bool) {
	itemID, err := strconv.Atoi(r.PathValue("item_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	it := s.store.ProjectsV2.GetItem(itemID)
	if it == nil || it.ProjectID != p.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return it, true
}

func (s *Server) serveProjectV2GetItem(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	it, ok := s.projectV2ItemFromRequest(w, r, p)
	if !ok {
		return
	}
	fieldIDs, ok := parseProjectV2FieldsParam(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.projectV2ItemWithContentJSON(r, it, s.projectV2APIURL(r, owner, p.Number), fieldIDs))
}

func (s *Server) serveProjectV2UpdateItem(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	it, ok := s.projectV2ItemFromRequest(w, r, p)
	if !ok {
		return
	}
	if _, ok := s.requireProjectV2Write(w, r, owner); !ok {
		return
	}
	var req struct {
		Fields []struct {
			ID    *int        `json:"id"`
			Value interface{} `json:"value"`
		} `json:"fields"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Fields) == 0 {
		writeGHValidationError(w, "ProjectV2Item", "fields", "missing_field")
		return
	}
	for _, upd := range req.Fields {
		if upd.ID == nil {
			writeGHValidationError(w, "ProjectV2Item", "fields", "missing_field")
			return
		}
		if err := s.store.ProjectsV2.SetFieldValueAny(it.ID, *upd.ID, upd.Value); err != nil {
			writeGHValidationError(w, "ProjectV2Item", "fields", "invalid")
			return
		}
	}
	writeJSON(w, http.StatusOK, s.projectV2ItemWithContentJSON(r, it, s.projectV2APIURL(r, owner, p.Number), nil))
}

func (s *Server) serveProjectV2DeleteItem(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	it, ok := s.projectV2ItemFromRequest(w, r, p)
	if !ok {
		return
	}
	if _, ok := s.requireProjectV2Write(w, r, owner); !ok {
		return
	}
	s.store.ProjectsV2.DeleteItem(it.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveProjectV2CreateView(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	user, ok := s.requireProjectV2Write(w, r, owner)
	if !ok {
		return
	}
	var req struct {
		Name          *string `json:"name"`
		Layout        *string `json:"layout"`
		Filter        *string `json:"filter"`
		VisibleFields []int   `json:"visible_fields"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		writeGHValidationError(w, "ProjectV2View", "name", "missing_field")
		return
	}
	if req.Layout == nil {
		writeGHValidationError(w, "ProjectV2View", "layout", "missing_field")
		return
	}
	switch *req.Layout {
	case "table", "board", "roadmap":
	default:
		writeGHValidationError(w, "ProjectV2View", "layout", "invalid")
		return
	}

	fields := s.store.ProjectsV2.FieldsForProject(p.ID)
	visible := []int{}
	if *req.Layout != "roadmap" { // visible_fields does not apply to roadmap views
		if req.VisibleFields != nil {
			for _, fid := range req.VisibleFields {
				f := s.store.ProjectsV2.GetField(fid)
				if f == nil || f.ProjectID != p.ID {
					writeGHValidationError(w, "ProjectV2View", "visible_fields", "invalid")
					return
				}
				visible = append(visible, fid)
			}
		} else {
			for _, f := range fields {
				visible = append(visible, f.ID)
			}
			sort.Ints(visible)
		}
	}

	view := s.store.ProjectsV2.CreateView(p.ID, *req.Name, *req.Layout, req.Filter, visible, user.ID)
	writeJSON(w, http.StatusCreated, s.projectV2ViewJSON(r, view, owner, p))
}

func (s *Server) serveProjectV2ListViewItems(w http.ResponseWriter, r *http.Request, owner *projectV2Owner) {
	p, ok := s.projectV2FromRequest(w, r, owner)
	if !ok {
		return
	}
	viewNumber, err := strconv.Atoi(r.PathValue("view_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	view := s.store.ProjectsV2.GetViewByNumber(p.ID, viewNumber)
	if view == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	fieldIDs, ok := parseProjectV2FieldsParam(w, r)
	if !ok {
		return
	}
	items := s.store.ProjectsV2.ListItemsForProject(p.ID)
	if view.Filter != nil && *view.Filter != "" {
		filtered := items[:0:0]
		for _, it := range items {
			if s.projectV2ItemMatchesFilter(it, *view.Filter) {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	page, pi := cursorPaginate(r, items, func(it *ProjectV2Item) int { return it.ID })
	setCursorLinkHeader(w, r, pi)
	projectURL := s.projectV2APIURL(r, owner, p.Number)
	out := make([]map[string]interface{}, 0, len(page))
	for _, it := range page {
		out = append(out, s.projectV2ItemWithContentJSON(r, it, projectURL, fieldIDs))
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------------------
// Route-facing wrappers

func (s *Server) handleOrgProjectsV2List(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectsV2List(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2Get(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2Get(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2CreateDraft(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2CreateDraft(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2ListFields(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2ListFields(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2CreateField(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2CreateField(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2GetField(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2GetField(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2ListItems(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2ListItems(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2AddItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2AddItem(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2GetItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2GetItem(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2UpdateItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2UpdateItem(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2DeleteItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2DeleteItem(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2CreateView(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2CreateView(w, r, owner)
	}
}

func (s *Server) handleOrgProjectV2ListViewItems(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2OrgOwner(w, r); ok {
		s.serveProjectV2ListViewItems(w, r, owner)
	}
}

func (s *Server) handleUserProjectsV2List(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectsV2List(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2Get(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2Get(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2ListFields(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2ListFields(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2CreateField(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2CreateField(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2GetField(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2GetField(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2ListItems(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2ListItems(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2AddItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2AddItem(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2GetItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2GetItem(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2UpdateItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2UpdateItem(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2DeleteItem(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2DeleteItem(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2CreateView(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByID(w, r); ok {
		s.serveProjectV2CreateView(w, r, owner)
	}
}

func (s *Server) handleUserProjectV2ListViewItems(w http.ResponseWriter, r *http.Request) {
	if owner, ok := s.projectV2UserOwnerByLogin(w, r); ok {
		s.serveProjectV2ListViewItems(w, r, owner)
	}
}

// handleAuthenticatedUserProjectV2CreateDraft serves the
// authenticated-user draft route (POST /user/{user_id}/…/drafts); the
// addressed user must be the caller.
func (s *Server) handleAuthenticatedUserProjectV2CreateDraft(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.projectV2UserOwnerByID(w, r)
	if !ok {
		return
	}
	user := ghUserFromContext(r.Context())
	if user == nil || (user.ID != owner.id && !user.SiteAdmin) {
		writeGHError(w, http.StatusForbidden, "Must be the addressed user.")
		return
	}
	s.serveProjectV2CreateDraft(w, r, owner)
}
