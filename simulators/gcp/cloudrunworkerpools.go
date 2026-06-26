package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	sim "github.com/sockerless/simulator"
)

// Cloud Run v2 Worker Pools slice. Worker pools are the v2 resource for
// pull-based (non-request-serving) workloads — the SDK uses
// run.NewWorkerPoolsRESTClient and the v2 REST surface
// `/v2/projects/{project}/locations/{location}/workerPools`. terraform's
// `google_cloud_run_v2_worker_pool` and `gcloud run worker-pools` wrap the
// same endpoints.
//
// Real API: https://cloud.google.com/run/docs/reference/rest/v2/projects.locations.workerPools

// WorkerPoolV2 mirrors google.cloud.run.v2.WorkerPool (proto-JSON shape).
// Generation is encoded as a JSON string per proto-JSON int64 rules.
type WorkerPoolV2 struct {
	Name                  string                      `json:"name"`
	UID                   string                      `json:"uid,omitempty"`
	Generation            int64                       `json:"generation,string,omitempty"`
	Labels                map[string]string           `json:"labels,omitempty"`
	Annotations           map[string]string           `json:"annotations,omitempty"`
	Description           string                      `json:"description,omitempty"`
	CreateTime            string                      `json:"createTime,omitempty"`
	UpdateTime            string                      `json:"updateTime,omitempty"`
	LaunchStage           enumString                  `json:"launchStage,omitempty"`
	Template              *WorkerPoolRevisionTemplate `json:"template,omitempty"`
	Scaling               *WorkerPoolScaling          `json:"scaling,omitempty"`
	InstanceSplits        []InstanceSplit             `json:"instanceSplits,omitempty"`
	InstanceSplitStatuses []InstanceSplit             `json:"instanceSplitStatuses,omitempty"`
	TerminalCondition     *Condition                  `json:"terminalCondition,omitempty"`
	Conditions            []Condition                 `json:"conditions,omitempty"`
	LatestReadyRevision   string                      `json:"latestReadyRevision,omitempty"`
	LatestCreatedRevision string                      `json:"latestCreatedRevision,omitempty"`
	ObservedGeneration    int64                       `json:"observedGeneration,string,omitempty"`
	Etag                  string                      `json:"etag,omitempty"`
	Reconciling           bool                        `json:"reconciling,omitempty"`
}

// WorkerPoolRevisionTemplate mirrors
// google.cloud.run.v2.WorkerPoolRevisionTemplate — the per-revision spec
// for a worker pool. Unlike a Service's RevisionTemplate there is no
// per-revision scaling (worker-pool scaling is manual, at the pool level).
type WorkerPoolRevisionTemplate struct {
	Labels         map[string]string `json:"labels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	Revision       string            `json:"revision,omitempty"`
	Containers     []Container       `json:"containers,omitempty"`
	Volumes        []Volume          `json:"volumes,omitempty"`
	VpcAccess      *VpcAccess        `json:"vpcAccess,omitempty"`
	ServiceAccount string            `json:"serviceAccount,omitempty"`
	NodeSelector   *NodeSelector     `json:"nodeSelector,omitempty"`
}

// NodeSelector mirrors google.cloud.run.v2.NodeSelector — the GPU
// accelerator selector a worker-pool revision can request.
type NodeSelector struct {
	Accelerator string `json:"accelerator,omitempty"`
}

// WorkerPoolScaling mirrors google.cloud.run.v2.WorkerPoolScaling.
type WorkerPoolScaling struct {
	ManualInstanceCount int32 `json:"manualInstanceCount,omitempty"`
}

// InstanceSplit mirrors google.cloud.run.v2.InstanceSplit (and the
// identical InstanceSplitStatus). Worker pools split instances across
// revisions much as a Service splits request traffic.
type InstanceSplit struct {
	Type     string `json:"type,omitempty"`
	Revision string `json:"revision,omitempty"`
	Percent  int32  `json:"percent,omitempty"`
}

func registerCloudRunWorkerPoolsV2(srv *sim.Server) {
	pools := sim.MakeStore[WorkerPoolV2](srv.DB(), "crv2_workerpools")
	revisions := sim.MakeStore[RevisionV2](srv.DB(), "crv2_workerpool_revisions")
	if crOperations == nil {
		crOperations = sim.MakeStore[Operation](srv.DB(), "operations")
	}

	const wpType = "type.googleapis.com/google.cloud.run.v2.WorkerPool"

	// CreateWorkerPool: POST .../workerPools?workerPoolId=<id>
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/workerPools", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolID := r.URL.Query().Get("workerPoolId")
		if poolID == "" {
			sim.GCPError(w, http.StatusBadRequest, "workerPoolId query parameter is required", "INVALID_ARGUMENT")
			return
		}
		var pool WorkerPoolV2
		if err := sim.ReadJSON(r, &pool); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		name := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, poolID)
		if _, exists := pools.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "worker pool %q already exists", name)
			return
		}
		pool = seedWorkerPoolV2Defaults(pool, project, location, poolID)
		pools.Put(name, pool)
		reconcileWorkerPoolRevision(revisions, name, poolID+"-00001-abc", pool)
		lro := newLRO(project, location, pool, wpType)
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// GetWorkerPool (the {workerPool} wildcard also carries the GET-side
	// `{workerPool}:getIamPolicy` verb).
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/workerPools/{workerPool}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolParam := sim.PathParam(r, "workerPool")
		if id, action, found := strings.Cut(poolParam, ":"); found {
			if action == "getIamPolicy" {
				handleResourceIAM(w, r, gcpResourceIAMStore(),
					fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, id), action)
				return
			}
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown action %q on worker pool %q", action, id)
			return
		}
		name := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, poolParam)
		pool, ok := pools.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "worker pool %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, pool)
	})

	// ListWorkerPools
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/workerPools", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/workerPools/", project, location)
		result := pools.Filter(func(p WorkerPoolV2) bool { return strings.HasPrefix(p.Name, prefix) })
		if result == nil {
			result = []WorkerPoolV2{}
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		page, next, ok := paginateList(w, r, result)
		if !ok {
			return
		}
		resp := map[string]any{"workerPools": page}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	})

	// UpdateWorkerPool (PATCH, honoring updateMask).
	srv.HandleFunc("PATCH /v2/projects/{project}/locations/{location}/workerPools/{workerPool}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolID := sim.PathParam(r, "workerPool")
		name := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, poolID)
		existing, ok := pools.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "worker pool %q not found", name)
			return
		}
		var update WorkerPoolV2
		if err := sim.ReadJSON(r, &update); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}
		if mask := r.URL.Query().Get("updateMask"); mask != "" {
			fields := strings.Split(mask, ",")
			has := func(p string) bool {
				for _, f := range fields {
					f = strings.TrimSpace(f)
					if f == p || strings.HasPrefix(f, p+".") {
						return true
					}
				}
				return false
			}
			merged := existing
			if has("template") {
				merged.Template = update.Template
			}
			if has("labels") {
				merged.Labels = update.Labels
			}
			if has("annotations") {
				merged.Annotations = update.Annotations
			}
			if has("scaling") {
				merged.Scaling = update.Scaling
			}
			if has("instanceSplits") || has("instance_splits") {
				merged.InstanceSplits = update.InstanceSplits
			}
			if has("description") {
				merged.Description = update.Description
			}
			if has("launchStage") || has("launch_stage") {
				merged.LaunchStage = update.LaunchStage
			}
			update = merged
		}
		update.Name = existing.Name
		update.UID = existing.UID
		update.CreateTime = existing.CreateTime
		update.Generation = existing.Generation + 1
		update.UpdateTime = nowTimestamp()
		if update.LaunchStage == "" {
			update.LaunchStage = existing.LaunchStage
		}
		update.TerminalCondition = &Condition{
			Type:               "Ready",
			State:              "CONDITION_SUCCEEDED",
			LastTransitionTime: update.UpdateTime,
		}
		revName := fmt.Sprintf("%s-%05d-abc", poolID, update.Generation)
		update.LatestCreatedRevision = fmt.Sprintf("%s/revisions/%s", name, revName)
		update.LatestReadyRevision = update.LatestCreatedRevision
		pools.Put(name, update)
		reconcileWorkerPoolRevision(revisions, name, revName, update)
		lro := newLRO(project, location, update, wpType)
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// DeleteWorkerPool
	srv.HandleFunc("DELETE /v2/projects/{project}/locations/{location}/workerPools/{workerPool}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolID := sim.PathParam(r, "workerPool")
		name := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, poolID)
		pool, ok := pools.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "worker pool %q not found", name)
			return
		}
		pools.Delete(name)
		revPrefix := name + "/revisions/"
		for _, rev := range revisions.Filter(func(rv RevisionV2) bool { return strings.HasPrefix(rv.Name, revPrefix) }) {
			revisions.Delete(rev.Name)
		}
		lro := newLRO(project, location, pool, wpType)
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// --- Worker Pool Revisions (get/list/delete) ---
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/workerPools/{workerPool}/revisions/{revision}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolID := sim.PathParam(r, "workerPool")
		revisionID := sim.PathParam(r, "revision")
		name := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s/revisions/%s", project, location, poolID, revisionID)
		rev, ok := revisions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "revision %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, rev)
	})
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/workerPools/{workerPool}/revisions", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolID := sim.PathParam(r, "workerPool")
		prefix := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s/revisions/", project, location, poolID)
		result := revisions.Filter(func(rev RevisionV2) bool { return strings.HasPrefix(rev.Name, prefix) })
		if result == nil {
			result = []RevisionV2{}
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
		page, next, ok := paginateList(w, r, result)
		if !ok {
			return
		}
		resp := map[string]any{"revisions": page}
		if next != "" {
			resp["nextPageToken"] = next
		}
		sim.WriteJSON(w, http.StatusOK, resp)
	})
	srv.HandleFunc("DELETE /v2/projects/{project}/locations/{location}/workerPools/{workerPool}/revisions/{revision}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolID := sim.PathParam(r, "workerPool")
		revisionID := sim.PathParam(r, "revision")
		name := fmt.Sprintf("projects/%s/locations/%s/workerPools/%s/revisions/%s", project, location, poolID, revisionID)
		rev, ok := revisions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "revision %q not found", name)
			return
		}
		revisions.Delete(name)
		lro := newLRO(project, location, rev, "type.googleapis.com/google.cloud.run.v2.Revision")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// --- Worker Pool IAM verbs (setIamPolicy / testIamPermissions) ---
	// getIamPolicy rides the GET handler's colon-split above.
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/workerPools/{workerPoolAction}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		poolAction := sim.PathParam(r, "workerPoolAction")
		id, action, found := strings.Cut(poolAction, ":")
		if !found {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown action on worker pool %q", poolAction)
			return
		}
		switch action {
		case "setIamPolicy", "testIamPermissions":
			handleResourceIAM(w, r, gcpResourceIAMStore(),
				fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, id), action)
		default:
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "unknown action %q on worker pool %q", action, id)
		}
	})
}

func seedWorkerPoolV2Defaults(pool WorkerPoolV2, project, location, poolID string) WorkerPoolV2 {
	now := nowTimestamp()
	pool.Name = fmt.Sprintf("projects/%s/locations/%s/workerPools/%s", project, location, poolID)
	pool.UID = generateUUID()
	pool.Generation = 1
	pool.ObservedGeneration = 1
	pool.CreateTime = now
	pool.UpdateTime = now
	if pool.LaunchStage == "" {
		pool.LaunchStage = "GA"
	}
	pool.TerminalCondition = &Condition{
		Type:               "Ready",
		State:              "CONDITION_SUCCEEDED",
		LastTransitionTime: now,
	}
	pool.Conditions = []Condition{
		{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
	}
	revName := fmt.Sprintf("%s-00001-abc", poolID)
	pool.LatestReadyRevision = fmt.Sprintf("%s/revisions/%s", pool.Name, revName)
	pool.LatestCreatedRevision = pool.LatestReadyRevision
	pool.InstanceSplitStatuses = []InstanceSplit{
		{Type: "INSTANCE_SPLIT_ALLOCATION_TYPE_LATEST", Percent: 100, Revision: revName},
	}
	return pool
}

// reconcileWorkerPoolRevision materializes the immutable Revision a worker
// pool deploy produces, mirroring reconcileServiceRevision for services.
func reconcileWorkerPoolRevision(store sim.Store[RevisionV2], poolName, revName string, pool WorkerPoolV2) {
	now := nowTimestamp()
	full := poolName + "/revisions/" + revName
	rev := RevisionV2{
		Name:        full,
		UID:         generateUUID(),
		Generation:  pool.Generation,
		CreateTime:  now,
		UpdateTime:  now,
		LaunchStage: pool.LaunchStage,
		Conditions: []Condition{
			{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
		},
	}
	if pool.Template != nil {
		rev.Labels = pool.Template.Labels
		rev.Annotations = pool.Template.Annotations
		rev.Containers = pool.Template.Containers
		rev.Volumes = pool.Template.Volumes
		rev.VpcAccess = pool.Template.VpcAccess
	}
	store.Put(full, rev)
}
