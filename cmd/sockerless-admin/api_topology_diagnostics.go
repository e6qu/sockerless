package main

import (
	"net/http"
	"strconv"
)

// diagnosticResponse bundles status + tail of recent logs in one
// shot so the UI's diagnostic panel doesn't have to chain two
// fetches when an instance goes unhealthy.
type diagnosticResponse struct {
	Status   InstanceStatus `json:"status"`
	LogLines []string       `json:"log_lines"`
	LogPath  string         `json:"log_path"`
}

const diagnosticDefaultLines = 50

// handleInstanceDiagnostics serves
// GET /api/v1/topology/projects/{p}/instances/{i}/diagnostics?lines=N.
//
// Returns the same InstanceStatus the polling status endpoint serves,
// plus the last N lines of `.stack-pids/<n>.log` for the operator to
// glance at without opening the full log tail. N defaults to 50,
// capped at 1000.
func handleInstanceDiagnostics(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instance := r.PathValue("instance")
		ref, ok := mgr.FindInstance(project, instance)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "instance " + project + "/" + instance + " not found",
			})
			return
		}

		lines := diagnosticDefaultLines
		if raw := r.URL.Query().Get("lines"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				if n > 1000 {
					n = 1000
				}
				lines = n
			}
		}

		status := readInstanceStatus(ref.Instance)
		status.Project = ref.Project

		path := instanceLogPath(ref.Instance.Name)
		tail, err := readLastLines(path, lines)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "read log: " + err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, diagnosticResponse{
			Status:   status,
			LogLines: tail,
			LogPath:  path,
		})
	}
}
