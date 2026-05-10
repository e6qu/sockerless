package main

import (
	"net/http"
	"strings"
	"time"
)

// registerTopologyAPI wires the sockerless.yaml topology surface.
//
// Routes:
//
//	GET    /api/v1/topology
//	PUT    /api/v1/topology
//	GET    /api/v1/topology/instances
//	GET    /api/v1/topology/projects/{project}/instances/{instance}
//	GET    /api/v1/topology/projects/{project}/instances/{instance}/logs
//	POST   /api/v1/topology/projects/{project}/instances/{instance}/proxy
//	GET    /api/v1/topology/resources
//	POST   /api/v1/topology/projects/{project}/instances/{instance}/start
//	POST   /api/v1/topology/projects/{project}/instances/{instance}/stop
//	POST   /api/v1/topology/projects/{project}/instances/{instance}/rebuild
//	POST   /api/v1/topology/allocate-port?kind=<sim|backend|bleephub>
//
// Lifecycle endpoints shell `make {start|stop|rebuild}-component` (see
// make/components.mk). Components stay decoupled — the make targets
// invoke the binaries with their normal env vars; admin doesn't talk
// to running components beyond the public surface (/v1/health etc).
//
// `lifecycle` may be nil in tests that only exercise the read +
// replace + allocate-port surface; lifecycle handlers fail with 503
// when it is.
func registerTopologyAPI(mux *http.ServeMux, mgr *TopologyManager, lifecycle *InstanceLifecycle) {
	mux.HandleFunc("GET /api/v1/topology", handleTopologyGet(mgr))
	mux.HandleFunc("PUT /api/v1/topology", handleTopologyPut(mgr))
	mux.HandleFunc("GET /api/v1/topology/instances", handleTopologyInstances(mgr))
	mux.HandleFunc("GET /api/v1/topology/projects/{project}/instances/{instance}", handleInstanceGet(mgr))
	mux.HandleFunc("POST /api/v1/topology/projects/{project}/instances/{instance}/start", handleInstanceStart(mgr, lifecycle))
	mux.HandleFunc("POST /api/v1/topology/projects/{project}/instances/{instance}/stop", handleInstanceStop(mgr, lifecycle))
	mux.HandleFunc("POST /api/v1/topology/projects/{project}/instances/{instance}/rebuild", handleInstanceRebuild(mgr, lifecycle))
	mux.HandleFunc("POST /api/v1/topology/allocate-port", handleAllocatePort(mgr))
	mux.HandleFunc("POST /api/v1/topology/projects", handleProjectAdd(mgr))
	mux.HandleFunc("DELETE /api/v1/topology/projects/{project}", handleProjectRemove(mgr))
	mux.HandleFunc("POST /api/v1/topology/projects/{project}/instances", handleInstanceAdd(mgr))
	mux.HandleFunc("PUT /api/v1/topology/projects/{project}/instances/{instance}", handleInstanceUpdate(mgr))
	mux.HandleFunc("DELETE /api/v1/topology/projects/{project}/instances/{instance}", handleInstanceRemove(mgr))
	mux.HandleFunc("GET /api/v1/topology/projects/{project}/instances/{instance}/status", handleInstanceStatus(mgr))
	mux.HandleFunc("GET /api/v1/topology/projects/{project}/instances/{instance}/logs", handleInstanceLogs(mgr))
	proxyClient := &http.Client{Timeout: proxyTimeout + 5*time.Second}
	mux.HandleFunc("POST /api/v1/topology/projects/{project}/instances/{instance}/proxy", handleInstanceProxy(mgr, proxyClient))
	rollupClient := &http.Client{Timeout: 5 * time.Second}
	mux.HandleFunc("GET /api/v1/topology/resources", handleTopologyResources(mgr, rollupClient))
}

func handleTopologyGet(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := mgr.Get()
		writeJSON(w, http.StatusOK, t)
	}
}

func handleTopologyPut(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var next Topology
		if err := decodeJSON(r.Body, &next); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
			return
		}
		if err := mgr.Replace(next); err != nil {
			writeJSON(w, topologyReplaceStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, mgr.Get())
	}
}

func handleTopologyInstances(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, mgr.Instances())
	}
}

func handleInstanceGet(mgr *TopologyManager) http.HandlerFunc {
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
		writeJSON(w, http.StatusOK, ref)
	}
}

// topologyReplaceStatus maps Replace errors to HTTP status codes.
func topologyReplaceStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := err.Error()
	// Validation errors (duplicate / unknown / out-of-range) are caller
	// errors. Anything else (file write, validation that wasn't possible
	// to surface) is a server error.
	if strings.HasPrefix(msg, "validate:") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func resolveInstanceForLifecycle(mgr *TopologyManager, w http.ResponseWriter, r *http.Request) (InstanceRef, int, bool) {
	project := r.PathValue("project")
	instance := r.PathValue("instance")
	ref, ok := mgr.FindInstance(project, instance)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "instance " + project + "/" + instance + " not found",
		})
		return InstanceRef{}, 0, false
	}
	// Backend instances with a Sim ref need the linked sim's port to
	// fill SOCKERLESS_ENDPOINT_URL at start time.
	simPort := 0
	if ref.Instance.Kind == InstanceKindBackend && ref.Instance.Sim != "" {
		linked, ok := mgr.FindInstance(ref.Project, ref.Instance.Sim)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "backend " + ref.Instance.Name + " references unknown sim " + ref.Instance.Sim,
			})
			return InstanceRef{}, 0, false
		}
		simPort = linked.Instance.Port
	}
	return ref, simPort, true
}

func handleInstanceStart(mgr *TopologyManager, lifecycle *InstanceLifecycle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if lifecycle == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle not configured"})
			return
		}
		ref, simPort, ok := resolveInstanceForLifecycle(mgr, w, r)
		if !ok {
			return
		}
		if err := lifecycle.Start(r.Context(), ref.Instance, simPort); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "started",
			"project": ref.Project,
			"name":    ref.Instance.Name,
		})
	}
}

func handleInstanceStop(mgr *TopologyManager, lifecycle *InstanceLifecycle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if lifecycle == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle not configured"})
			return
		}
		ref, _, ok := resolveInstanceForLifecycle(mgr, w, r)
		if !ok {
			return
		}
		if err := lifecycle.Stop(r.Context(), ref.Instance); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "stopped",
			"project": ref.Project,
			"name":    ref.Instance.Name,
		})
	}
}

func handleInstanceRebuild(mgr *TopologyManager, lifecycle *InstanceLifecycle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if lifecycle == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle not configured"})
			return
		}
		ref, _, ok := resolveInstanceForLifecycle(mgr, w, r)
		if !ok {
			return
		}
		if err := lifecycle.Rebuild(r.Context(), ref.Instance); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "rebuilt",
			"project": ref.Project,
			"name":    ref.Instance.Name,
		})
	}
}

func handleAllocatePort(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind := InstanceKind(r.URL.Query().Get("kind"))
		if !IsValidInstanceKind(kind) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "kind query param must be one of sim, backend, bleephub",
			})
			return
		}
		port, err := mgr.AllocatePort(kind)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "port": port})
	}
}

// crudErrorStatus maps TopologyManager mutation errors to HTTP statuses.
// Validation = 400, conflict (already exists) = 409, missing = 404.
func crudErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "already exists"):
		return http.StatusConflict
	case strings.Contains(msg, "not found"):
		return http.StatusNotFound
	case strings.HasPrefix(msg, "validate:"):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func handleProjectAdd(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p ProjectConfig
		if err := decodeJSON(r.Body, &p); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
			return
		}
		if err := mgr.AddProject(p); err != nil {
			writeJSON(w, crudErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}
}

func handleProjectRemove(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		if err := mgr.RemoveProject(project); err != nil {
			writeJSON(w, crudErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"removed": project})
	}
}

func handleInstanceAdd(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		var inst Instance
		if err := decodeJSON(r.Body, &inst); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
			return
		}
		if err := mgr.AddInstance(project, inst); err != nil {
			writeJSON(w, crudErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, InstanceRef{Project: project, Instance: inst})
	}
}

func handleInstanceUpdate(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instanceName := r.PathValue("instance")
		var inst Instance
		if err := decodeJSON(r.Body, &inst); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
			return
		}
		if inst.Name != instanceName {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "body name " + inst.Name + " must equal path name " + instanceName + " (renames go through delete + add)",
			})
			return
		}
		if err := mgr.UpdateInstance(project, inst); err != nil {
			writeJSON(w, crudErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, InstanceRef{Project: project, Instance: inst})
	}
}

func handleInstanceRemove(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instanceName := r.PathValue("instance")
		if err := mgr.RemoveInstance(project, instanceName); err != nil {
			writeJSON(w, crudErrorStatus(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"removed": project + "/" + instanceName})
	}
}

func handleInstanceStatus(mgr *TopologyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		instanceName := r.PathValue("instance")
		ref, ok := mgr.FindInstance(project, instanceName)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "instance " + project + "/" + instanceName + " not found",
			})
			return
		}
		status := readInstanceStatus(ref.Instance)
		status.Project = ref.Project
		writeJSON(w, http.StatusOK, status)
	}
}
