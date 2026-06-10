package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

type dataflowJob struct {
	ID               string            `json:"id,omitempty"`
	ProjectID        string            `json:"projectId,omitempty"`
	Location         string            `json:"location,omitempty"`
	Name             string            `json:"name,omitempty"`
	Type             string            `json:"type,omitempty"`
	CurrentState     string            `json:"currentState,omitempty"`
	CurrentStateTime string            `json:"currentStateTime,omitempty"`
	CreateTime       string            `json:"createTime,omitempty"`
	Steps            []map[string]any  `json:"steps,omitempty"`
	Environment      map[string]any    `json:"environment,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
}

var dataflowJobs sim.Store[dataflowJob]

func registerDataflow(srv *sim.Server) {
	dataflowJobs = sim.MakeStore[dataflowJob](srv.DB(), "dataflow_jobs")
	srv.HandleFunc("POST /v1b3/projects/{project}/locations/{location}/jobs", handleDataflowCreateJob)
	srv.HandleFunc("GET /v1b3/projects/{project}/locations/{location}/jobs", handleDataflowListJobs)
	srv.HandleFunc("GET /v1b3/projects/{project}/locations/{location}/jobs/{job}", handleDataflowGetJob)
	srv.HandleFunc("PUT /v1b3/projects/{project}/locations/{location}/jobs/{job}", handleDataflowUpdateJob)
}

func dataflowJobKey(project, location, id string) string {
	return fmt.Sprintf("projects/%s/locations/%s/jobs/%s", project, location, id)
}

func handleDataflowCreateJob(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	var req dataflowJob
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	if req.Name == "" {
		sim.GCPError(w, http.StatusBadRequest, "job name is required", "INVALID_ARGUMENT")
		return
	}
	id := req.ID
	if id == "" {
		id = generateUUID()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	req.ID = id
	req.ProjectID = project
	req.Location = location
	if req.Type == "" {
		req.Type = "JOB_TYPE_BATCH"
	}
	req.CurrentState = "JOB_STATE_RUNNING"
	req.CreateTime = now
	req.CurrentStateTime = now
	dataflowJobs.Put(dataflowJobKey(project, location, id), req)
	sim.WriteJSON(w, http.StatusOK, req)
}

func handleDataflowListJobs(w http.ResponseWriter, r *http.Request) {
	project := sim.PathParam(r, "project")
	location := sim.PathParam(r, "location")
	prefix := fmt.Sprintf("projects/%s/locations/%s/jobs/", project, location)
	out := dataflowJobs.Filter(func(job dataflowJob) bool {
		return strings.HasPrefix(dataflowJobKey(job.ProjectID, job.Location, job.ID), prefix)
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	page, next, ok := paginateList(w, r, out)
	if !ok {
		return
	}
	resp := map[string]any{"jobs": page}
	if next != "" {
		resp["nextPageToken"] = next
	}
	sim.WriteJSON(w, http.StatusOK, resp)
}

func handleDataflowGetJob(w http.ResponseWriter, r *http.Request) {
	key := dataflowJobKey(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "job"))
	job, ok := dataflowJobs.Get(key)
	if !ok {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "job %q not found", key)
		return
	}
	sim.WriteJSON(w, http.StatusOK, job)
}

func handleDataflowUpdateJob(w http.ResponseWriter, r *http.Request) {
	key := dataflowJobKey(sim.PathParam(r, "project"), sim.PathParam(r, "location"), sim.PathParam(r, "job"))
	var req dataflowJob
	if err := sim.ReadJSON(r, &req); err != nil {
		sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
		return
	}
	if !dataflowJobs.Update(key, func(job *dataflowJob) {
		if req.CurrentState != "" {
			job.CurrentState = req.CurrentState
			job.CurrentStateTime = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}) {
		sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "job %q not found", key)
		return
	}
	job, _ := dataflowJobs.Get(key)
	sim.WriteJSON(w, http.StatusOK, job)
}
