package main

import (
	"net/http"
)

// handleConfigMetadata serves the curated annotation table the UI
// renders in the edit modal so each row gets a hot/restart indicator.
func handleConfigMetadata() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"keys": AllAnnotations(),
		})
	}
}

// configUpdateResponse tells the UI which kind of action will pick up
// the change (reload vs restart) so it can prompt the operator.
type configUpdateResponse struct {
	Project                string            `json:"project"`
	Instance               string            `json:"instance"`
	HotReloadableChanges   []string          `json:"hot_reloadable_changes"`
	RestartRequiredChanges []string          `json:"restart_required_changes"`
	Config                 map[string]string `json:"config"`
}

// handleInstanceConfigUpdate replaces Instance.Config in the topology
// and returns the change classification so the UI can decide whether
// to offer Reload or Restart.
//
// PUT body is the full Config map. Partial-update semantics (PATCH-
// style merging) would force admin to track which keys came from
// where; the UI already holds the full map, so a full replace is
// simpler and unambiguous.
func handleInstanceConfigUpdate(mgr *TopologyManager) http.HandlerFunc {
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

		var next map[string]string
		if err := decodeJSON(r.Body, &next); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body: " + err.Error(),
			})
			return
		}
		if next == nil {
			next = map[string]string{}
		}

		hot, restart := ClassifyChanges(ref.Instance.Config, next)

		updated := ref.Instance
		updated.Config = next
		if err := mgr.UpdateInstance(project, updated); err != nil {
			writeJSON(w, crudErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, configUpdateResponse{
			Project:                project,
			Instance:               instance,
			HotReloadableChanges:   hot,
			RestartRequiredChanges: restart,
			Config:                 next,
		})
	}
}

// handleInstanceReload sends SIGHUP to the running component (via
// `make reload-component`). Component-side handling is the
// component's concern — admin provides the signal path; whether the
// component does anything with SIGHUP is per-binary.
//
// Returns 503 when lifecycle is unconfigured (test path), 404 when
// the instance is unknown, 502 when the make target fails.
func handleInstanceReload(mgr *TopologyManager, lifecycle *InstanceLifecycle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if lifecycle == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle not configured"})
			return
		}
		project := r.PathValue("project")
		instance := r.PathValue("instance")
		ref, ok := mgr.FindInstance(project, instance)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "instance " + project + "/" + instance + " not found",
			})
			return
		}
		if err := lifecycle.Reload(r.Context(), ref.Instance); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "reloaded",
			"project": project,
			"name":    instance,
		})
	}
}
