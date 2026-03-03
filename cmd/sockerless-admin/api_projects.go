package main

import (
	"net/http"
	"strconv"
)

// registerProjectAPI registers project management API routes.
func registerProjectAPI(mux *http.ServeMux, projectMgr *ProjectManager) {
	mux.HandleFunc("GET /api/v1/projects", handleProjectList(projectMgr))
	mux.HandleFunc("POST /api/v1/projects", handleProjectCreate(projectMgr))
	mux.HandleFunc("GET /api/v1/projects/{name}", handleProjectGet(projectMgr))
	mux.HandleFunc("POST /api/v1/projects/{name}/start", handleProjectStart(projectMgr))
	mux.HandleFunc("POST /api/v1/projects/{name}/stop", handleProjectStop(projectMgr))
	mux.HandleFunc("DELETE /api/v1/projects/{name}", handleProjectDelete(projectMgr))
	mux.HandleFunc("GET /api/v1/projects/{name}/logs", handleProjectLogs(projectMgr))
	mux.HandleFunc("GET /api/v1/projects/{name}/connection", handleProjectConnection(projectMgr))
}

// handleProjectList returns all projects with status.
func handleProjectList(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects := projectMgr.List()
		if projects == nil {
			projects = []ProjectStatus{}
		}
		writeJSON(w, http.StatusOK, projects)
	}
}

// CreateProjectRequest is the JSON body for creating a project.
type CreateProjectRequest struct {
	Name             string      `json:"name"`
	Cloud            CloudType   `json:"cloud"`
	Backend          BackendType `json:"backend"`
	LogLevel         string      `json:"log_level"`
	SimPort          int         `json:"sim_port"`
	BackendPort      int         `json:"backend_port"`
	FrontendPort     int         `json:"frontend_port"`
	FrontendMgmtPort int         `json:"frontend_mgmt_port"`
}

// handleProjectCreate creates a new project.
func handleProjectCreate(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateProjectRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		cfg := ProjectConfig{
			Name:             req.Name,
			Cloud:            req.Cloud,
			Backend:          req.Backend,
			LogLevel:         req.LogLevel,
			SimPort:          req.SimPort,
			BackendPort:      req.BackendPort,
			FrontendPort:     req.FrontendPort,
			FrontendMgmtPort: req.FrontendMgmtPort,
		}

		if err := projectMgr.Create(cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		status, _ := projectMgr.Get(req.Name)
		writeJSON(w, http.StatusCreated, status)
	}
}

// handleProjectGet returns a single project with status.
func handleProjectGet(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		status, ok := projectMgr.Get(name)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

// handleProjectStart performs orchestrated start.
func handleProjectStart(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := projectMgr.Start(name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		status, _ := projectMgr.Get(name)
		writeJSON(w, http.StatusOK, status)
	}
}

// handleProjectStop performs orchestrated stop.
func handleProjectStop(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := projectMgr.Stop(name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		status, _ := projectMgr.Get(name)
		writeJSON(w, http.StatusOK, status)
	}
}

// handleProjectDelete deletes a project.
func handleProjectDelete(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if err := projectMgr.Delete(name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"deleted": name})
	}
}

// handleProjectLogs returns logs for a project, optionally filtered by component.
func handleProjectLogs(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		component := r.URL.Query().Get("component")
		lines := 200
		if q := r.URL.Query().Get("lines"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 {
				lines = n
			}
		}

		logs, err := projectMgr.Logs(name, component, lines)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if logs == nil {
			logs = []string{}
		}
		writeJSON(w, http.StatusOK, logs)
	}
}

// handleProjectConnection returns Docker/Podman connection info.
func handleProjectConnection(projectMgr *ProjectManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		conn, err := projectMgr.Connection(name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}
