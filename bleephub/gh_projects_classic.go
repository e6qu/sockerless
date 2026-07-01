package bleephub

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHProjectsClassicRoutes() {
	// Project-level
	s.route("GET /api/v3/repos/{owner}/{repo}/projects", s.handleListProjectsClassic)
	s.route("POST /api/v3/repos/{owner}/{repo}/projects", s.handleCreateProjectClassic)
	s.route("GET /api/v3/projects/{project_id}", s.handleGetProjectClassic)
	s.route("PATCH /api/v3/projects/{project_id}", s.handleUpdateProjectClassic)
	s.route("DELETE /api/v3/projects/{project_id}", s.handleDeleteProjectClassic)

	// Column-level: two real GitHub path shapes share the two-segment slot
	// after /projects/ — Go's mux cannot distinguish them, so dispatch.
	s.route("GET /api/v3/projects/{p1}/{p2}", s.handleProjectTwoSegDispatch("GET"))
	s.route("POST /api/v3/projects/{p1}/{p2}", s.handleProjectTwoSegDispatch("POST"))
	s.route("PATCH /api/v3/projects/{p1}/{p2}", s.handleProjectTwoSegDispatch("PATCH"))
	s.route("DELETE /api/v3/projects/{p1}/{p2}", s.handleProjectTwoSegDispatch("DELETE"))

	// Column moves are unambiguous.
	s.route("POST /api/v3/projects/columns/{column_id}/moves", s.handleMoveProjectColumn)

	// Card-level: two real GitHub path shapes share the two-segment slot
	// after /projects/columns/ — Go's mux cannot distinguish them, so dispatch.
	s.route("GET /api/v3/projects/columns/{p1}/{p2}", s.handleColumnTwoSegDispatch("GET"))
	s.route("POST /api/v3/projects/columns/{p1}/{p2}", s.handleColumnTwoSegDispatch("POST"))
	s.route("PATCH /api/v3/projects/columns/{p1}/{p2}", s.handleColumnTwoSegDispatch("PATCH"))
	s.route("DELETE /api/v3/projects/columns/{p1}/{p2}", s.handleColumnTwoSegDispatch("DELETE"))

	// Card moves are unambiguous.
	s.route("POST /api/v3/projects/columns/cards/{card_id}/moves", s.handleMoveProjectCard)
}

// handleProjectTwoSegDispatch resolves:
//
//	GET/POST /projects/{project_id}/columns
//	GET/PATCH/DELETE /projects/columns/{column_id}
func (s *Server) handleProjectTwoSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1, p2 := r.PathValue("p1"), r.PathValue("p2")
		switch {
		case p2 == "columns" && (method == "GET" || method == "POST"):
			r.SetPathValue("project_id", p1)
			if method == "GET" {
				s.handleListProjectColumns(w, r)
			} else {
				s.handleCreateProjectColumn(w, r)
			}
		case p1 == "columns" && (method == "GET" || method == "PATCH" || method == "DELETE"):
			r.SetPathValue("column_id", p2)
			switch method {
			case "GET":
				s.handleGetProjectColumn(w, r)
			case "PATCH":
				s.handleUpdateProjectColumn(w, r)
			case "DELETE":
				s.handleDeleteProjectColumn(w, r)
			}
		default:
			writeGHError(w, http.StatusNotFound, "Not Found")
		}
	}
}

// handleColumnTwoSegDispatch resolves:
//
//	GET/POST /projects/columns/{column_id}/cards
//	GET/PATCH/DELETE /projects/columns/cards/{card_id}
func (s *Server) handleColumnTwoSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1, p2 := r.PathValue("p1"), r.PathValue("p2")
		switch {
		case p2 == "cards" && (method == "GET" || method == "POST"):
			r.SetPathValue("column_id", p1)
			if method == "GET" {
				s.handleListProjectCards(w, r)
			} else {
				s.handleCreateProjectCard(w, r)
			}
		case p1 == "cards" && (method == "GET" || method == "PATCH" || method == "DELETE"):
			r.SetPathValue("card_id", p2)
			switch method {
			case "GET":
				s.handleGetProjectCard(w, r)
			case "PATCH":
				s.handleUpdateProjectCard(w, r)
			case "DELETE":
				s.handleDeleteProjectCard(w, r)
			}
		default:
			writeGHError(w, http.StatusNotFound, "Not Found")
		}
	}
}

func (s *Server) handleListProjectsClassic(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	state := strings.ToLower(r.URL.Query().Get("state"))
	projects := s.store.ListProjectClassicsForRepo(repo.FullName)
	out := make([]map[string]interface{}, 0, len(projects))
	for _, p := range projects {
		if state != "" && state != strings.ToLower(p.State) {
			continue
		}
		out = append(out, projectClassicToJSON(p, s.store, s.baseURL(r), repo.FullName))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateProjectClassic(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Name  string `json:"name"`
		Body  string `json:"body"`
		State string `json:"state"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeGHValidationError(w, "Project", "name", "missing_field")
		return
	}
	proj := s.store.CreateProjectClassic(repo, user.ID, body.Name, body.Body, body.State)
	s.recordAuditEvent("project.create", user.Login, "", map[string]interface{}{"repo": repo.FullName, "project_id": proj.ID})
	writeJSON(w, http.StatusCreated, projectClassicToJSON(proj, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleGetProjectClassic(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	proj, repo := s.resolveProjectClassic(w, r)
	if proj == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, projectClassicToJSON(proj, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleUpdateProjectClassic(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	proj, repo := s.resolveProjectClassic(w, r)
	if proj == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Name  *string `json:"name"`
		Body  *string `json:"body"`
		State *string `json:"state"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	updated := s.store.UpdateProjectClassic(proj, body.Name, body.Body, body.State)
	writeJSON(w, http.StatusOK, projectClassicToJSON(updated, s.store, s.baseURL(r), repo.FullName))
}

func (s *Server) handleDeleteProjectClassic(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	proj, repo := s.resolveProjectClassic(w, r)
	if proj == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.DeleteProjectClassic(proj.ID)
	s.recordAuditEvent("project.destroy", user.Login, "", map[string]interface{}{"repo": repo.FullName, "project_id": proj.ID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListProjectColumns(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	proj, repo := s.resolveProjectClassic(w, r)
	if proj == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	cols := s.store.ListProjectColumns(proj.ID)
	out := make([]map[string]interface{}, len(cols))
	for i, c := range cols {
		out[i] = projectColumnToJSON(c, s.store, s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateProjectColumn(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	proj, repo := s.resolveProjectClassic(w, r)
	if proj == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeGHValidationError(w, "ProjectColumn", "name", "missing_field")
		return
	}
	col := s.store.CreateProjectColumn(proj.ID, body.Name)
	writeJSON(w, http.StatusCreated, projectColumnToJSON(col, s.store, s.baseURL(r)))
}

func (s *Server) handleGetProjectColumn(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	col, repo := s.resolveProjectColumn(w, r)
	if col == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, projectColumnToJSON(col, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateProjectColumn(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	col, repo := s.resolveProjectColumn(w, r)
	if col == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeGHValidationError(w, "ProjectColumn", "name", "missing_field")
		return
	}
	updated := s.store.UpdateProjectColumn(col, body.Name)
	writeJSON(w, http.StatusOK, projectColumnToJSON(updated, s.store, s.baseURL(r)))
}

func (s *Server) handleDeleteProjectColumn(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	col, repo := s.resolveProjectColumn(w, r)
	if col == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.DeleteProjectColumn(col.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMoveProjectColumn(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	col, repo := s.resolveProjectColumn(w, r)
	if col == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Position string `json:"position"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if err := s.store.MoveProjectColumn(col, body.Position); err != nil {
		writeGHValidationError(w, "ProjectColumn", "position", "invalid")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": col.ID, "url": projectColumnURL(col, s.baseURL(r))})
}

func (s *Server) handleListProjectCards(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	col, repo := s.resolveProjectColumn(w, r)
	if col == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	cards := s.store.ListProjectCards(col.ID)
	out := make([]map[string]interface{}, len(cards))
	for i, c := range cards {
		out[i] = projectCardToJSON(c, s.store, s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateProjectCard(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	col, repo := s.resolveProjectColumn(w, r)
	if col == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Note        string `json:"note"`
		ContentID   int    `json:"content_id"`
		ContentType string `json:"content_type"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}

	issueID := 0
	if body.ContentType != "" || body.ContentID != 0 {
		if strings.EqualFold(body.ContentType, "Issue") {
			issue := s.store.GetIssue(body.ContentID)
			if issue == nil || issue.RepoID != repo.ID {
				writeGHValidationError(w, "ProjectCard", "content_id", "invalid")
				return
			}
			issueID = issue.ID
		} else {
			writeGHValidationError(w, "ProjectCard", "content_type", "invalid")
			return
		}
	}
	if issueID == 0 && body.Note == "" {
		writeGHValidationError(w, "ProjectCard", "note", "missing_field")
		return
	}

	card := s.store.CreateProjectCard(col.ID, user.ID, body.Note, issueID)
	writeJSON(w, http.StatusCreated, projectCardToJSON(card, s.store, s.baseURL(r)))
}

func (s *Server) handleGetProjectCard(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	card, repo := s.resolveProjectCard(w, r)
	if card == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, projectCardToJSON(card, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateProjectCard(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	card, repo := s.resolveProjectCard(w, r)
	if card == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	updated := s.store.UpdateProjectCard(card, body.Note)
	writeJSON(w, http.StatusOK, projectCardToJSON(updated, s.store, s.baseURL(r)))
}

func (s *Server) handleDeleteProjectCard(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	card, repo := s.resolveProjectCard(w, r)
	if card == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.DeleteProjectCard(card.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMoveProjectCard(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	card, repo := s.resolveProjectCard(w, r)
	if card == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body struct {
		Position string `json:"position"`
		ColumnID int    `json:"column_id"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.ColumnID != 0 && s.store.GetProjectColumn(body.ColumnID) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if err := s.store.MoveProjectCard(card, body.ColumnID, body.Position); err != nil {
		writeGHValidationError(w, "ProjectCard", "position", "invalid")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"id": card.ID, "url": projectCardURL(card, s.baseURL(r))})
}

func (s *Server) resolveProjectClassic(w http.ResponseWriter, r *http.Request) (*ProjectClassic, *Repo) {
	idStr := r.PathValue("project_id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	proj := s.store.GetProjectClassic(id)
	if proj == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	repo := s.store.GetRepoByName(proj.RepoKey)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return proj, repo
}

func (s *Server) resolveProjectColumn(w http.ResponseWriter, r *http.Request) (*ProjectColumn, *Repo) {
	idStr := r.PathValue("column_id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	col := s.store.GetProjectColumn(id)
	if col == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	proj := s.store.GetProjectClassic(col.ProjectID)
	if proj == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	repo := s.store.GetRepoByName(proj.RepoKey)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return col, repo
}

func (s *Server) resolveProjectCard(w http.ResponseWriter, r *http.Request) (*ProjectCard, *Repo) {
	idStr := r.PathValue("card_id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	card := s.store.GetProjectCard(id)
	if card == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	col := s.store.GetProjectColumn(card.ColumnID)
	if col == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	proj := s.store.GetProjectClassic(col.ProjectID)
	if proj == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	repo := s.store.GetRepoByName(proj.RepoKey)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return card, repo
}

func projectClassicToJSON(p *ProjectClassic, st *Store, baseURL, repoFullName string) map[string]interface{} {
	var creator map[string]interface{}
	st.mu.RLock()
	if u := st.Users[p.CreatorID]; u != nil {
		creator = userToJSON(u)
	}
	st.mu.RUnlock()

	api := baseURL + "/api/v3/projects/" + strconv.Itoa(p.ID)
	return map[string]interface{}{
		"id":          p.ID,
		"node_id":     p.NodeID,
		"name":        p.Name,
		"body":        p.Body,
		"state":       p.State,
		"number":      p.Number,
		"creator":     creator,
		"created_at":  p.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  p.UpdatedAt.UTC().Format(time.RFC3339),
		"url":         api,
		"html_url":    baseURL + "/" + repoFullName + "/projects/" + strconv.Itoa(p.Number),
		"columns_url": api + "/columns",
		"owner_url":   baseURL + "/api/v3/repos/" + repoFullName,
	}
}

func projectColumnToJSON(c *ProjectColumn, st *Store, baseURL string) map[string]interface{} {
	api := projectColumnURL(c, baseURL)
	return map[string]interface{}{
		"id":          c.ID,
		"node_id":     c.NodeID,
		"name":        c.Name,
		"created_at":  c.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  c.UpdatedAt.UTC().Format(time.RFC3339),
		"url":         api,
		"project_url": baseURL + "/api/v3/projects/" + strconv.Itoa(c.ProjectID),
		"cards_url":   api + "/cards",
	}
}

func projectColumnURL(c *ProjectColumn, baseURL string) string {
	return baseURL + "/api/v3/projects/columns/" + strconv.Itoa(c.ID)
}

func projectCardToJSON(c *ProjectCard, st *Store, baseURL string) map[string]interface{} {
	var creator map[string]interface{}
	var contentURL interface{}
	st.mu.RLock()
	if u := st.Users[c.CreatorID]; u != nil {
		creator = userToJSON(u)
	}
	col := st.ProjectColumns[c.ColumnID]
	var proj *ProjectClassic
	if col != nil {
		proj = st.ProjectClassic[col.ProjectID]
	}
	if c.IssueID != 0 && proj != nil {
		if issue := st.Issues[c.IssueID]; issue != nil {
			contentURL = baseURL + "/api/v3/repos/" + proj.RepoKey + "/issues/" + strconv.Itoa(issue.Number)
		}
	}
	st.mu.RUnlock()

	api := projectCardURL(c, baseURL)
	return map[string]interface{}{
		"id":          c.ID,
		"node_id":     c.NodeID,
		"note":        nullIfEmpty(c.Note),
		"creator":     creator,
		"created_at":  c.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  c.UpdatedAt.UTC().Format(time.RFC3339),
		"url":         api,
		"column_url":  projectColumnURL(col, baseURL),
		"project_url": baseURL + "/api/v3/projects/" + strconv.Itoa(col.ProjectID),
		"content_url": contentURL,
	}
}

func projectCardURL(c *ProjectCard, baseURL string) string {
	return baseURL + "/api/v3/projects/columns/cards/" + strconv.Itoa(c.ID)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (st *Store) GetRepoByName(fullName string) *Repo {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.ReposByName[fullName]
}
